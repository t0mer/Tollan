// Package event evaluates event definitions on a schedule and fans firings out
// to notification channels. Two trigger types are supported (Graylog "basic
// triggers & aggregations"): filter (query matches ≥N in a window) and
// aggregation (a grouped metric crosses a threshold in a window).
package event

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/t0mer/tollan/internal/crypto"
	"github.com/t0mer/tollan/internal/logstore"
	"github.com/t0mer/tollan/internal/meta"
	"github.com/t0mer/tollan/internal/metrics"
	"github.com/t0mer/tollan/internal/notify"
	"github.com/t0mer/tollan/internal/search/query"
)

// TriggerType is the kind of event trigger.
type TriggerType string

const (
	TriggerFilter      TriggerType = "filter"
	TriggerAggregation TriggerType = "aggregation"
)

// Definition is an event definition (stored as a meta KindEvent entity).
type Definition struct {
	ID              string      `json:"id"`
	Name            string      `json:"name"`
	Enabled         bool        `json:"enabled"`
	Type            TriggerType `json:"type"`
	Query           string      `json:"query"`
	WindowSeconds   int         `json:"window_seconds"`
	Threshold       float64     `json:"threshold"`
	GroupBy         string      `json:"group_by"`
	Metric          string      `json:"metric"`
	MetricField     string      `json:"metric_field"`
	Channels        []string    `json:"channels"`
	MessageTemplate string      `json:"message_template"`
	GraceSeconds    int         `json:"grace_seconds"`
	Backlog         int         `json:"backlog"`
}

// Engine evaluates definitions and dispatches notifications.
type Engine struct {
	store    logstore.Store
	meta     *meta.Store
	notifier *notify.Notifier
	cipher   *crypto.Cipher
	metrics  *metrics.Metrics
	log      *slog.Logger

	interval time.Duration
	cron     *cron.Cron

	mu        sync.Mutex
	lastFired map[string]time.Time
}

// New builds an event engine.
func New(store logstore.Store, m *meta.Store, notifier *notify.Notifier, cipher *crypto.Cipher, mx *metrics.Metrics, log *slog.Logger) *Engine {
	return &Engine{
		store: store, meta: m, notifier: notifier, cipher: cipher, metrics: mx, log: log,
		interval:  30 * time.Second,
		lastFired: map[string]time.Time{},
	}
}

// Start begins periodic evaluation.
func (e *Engine) Start() {
	e.cron = cron.New()
	_, _ = e.cron.AddFunc("@every 30s", func() {
		if err := e.EvaluateAll(context.Background()); err != nil {
			e.log.Error("event evaluation failed", "error", err)
		}
	})
	e.cron.Start()
}

// Stop halts evaluation.
func (e *Engine) Stop() {
	if e.cron != nil {
		e.cron.Stop()
	}
}

// EvaluateAll evaluates every enabled definition once.
func (e *Engine) EvaluateAll(ctx context.Context) error {
	ents, err := e.meta.ListEntities(ctx, meta.KindEvent)
	if err != nil {
		return err
	}
	for _, ent := range ents {
		var def Definition
		if err := json.Unmarshal(ent.Data, &def); err != nil {
			continue
		}
		def.ID = ent.ID
		if !def.Enabled {
			continue
		}
		if err := e.Evaluate(ctx, def); err != nil {
			e.log.Error("evaluating event", "definition", def.Name, "error", err)
		}
	}
	return nil
}

// Evaluate runs one definition and fires if its condition is met.
func (e *Engine) Evaluate(ctx context.Context, def Definition) error {
	if e.throttled(def) {
		return nil
	}
	window := time.Duration(def.WindowSeconds) * time.Second
	if window <= 0 {
		window = 5 * time.Minute
	}
	expr, err := query.Parse(def.Query)
	if err != nil {
		return fmt.Errorf("bad query: %w", err)
	}
	now := time.Now().UTC()
	lq := logstore.Query{From: now.Add(-window), To: now, Expr: expr}

	switch def.Type {
	case TriggerAggregation:
		return e.evalAggregation(ctx, def, lq)
	default:
		return e.evalFilter(ctx, def, lq, now)
	}
}

func (e *Engine) evalFilter(ctx context.Context, def Definition, lq logstore.Query, now time.Time) error {
	backlog := def.Backlog
	if backlog <= 0 {
		backlog = 5
	}
	lq.Limit = backlog
	res, err := e.store.Search(ctx, lq)
	if err != nil {
		return err
	}
	if float64(res.Total) < def.Threshold {
		return nil
	}
	samples := make([]string, 0, len(res.Messages))
	for _, m := range res.Messages {
		samples = append(samples, m.Body)
	}
	return e.fire(ctx, def, res.Total, "", samples)
}

func (e *Engine) evalAggregation(ctx context.Context, def Definition, lq logstore.Query) error {
	spec := logstore.AggSpec{
		GroupBy:     def.GroupBy,
		Metric:      logstore.AggMetric(orDefault(def.Metric, "count")),
		MetricField: def.MetricField,
		Limit:       50,
	}
	rows, err := e.store.Aggregate(ctx, lq, spec)
	if err != nil {
		return err
	}
	fired := false
	for _, row := range rows {
		if row.Value > def.Threshold {
			if err := e.fire(ctx, def, int(row.Value), row.Key, nil); err != nil {
				e.log.Error("firing aggregation event", "error", err)
			}
			fired = true
		}
	}
	_ = fired
	return nil
}

func (e *Engine) throttled(def Definition) bool {
	grace := time.Duration(def.GraceSeconds) * time.Second
	if grace <= 0 {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if last, ok := e.lastFired[def.ID]; ok && time.Since(last) < grace {
		return true
	}
	return false
}

func (e *Engine) markFired(def Definition) {
	e.mu.Lock()
	e.lastFired[def.ID] = time.Now()
	e.mu.Unlock()
}

func (e *Engine) fire(ctx context.Context, def Definition, count int, groupKey string, samples []string) error {
	e.markFired(def)
	if e.metrics != nil {
		e.metrics.EventFirings.WithLabelValues(def.ID).Inc()
	}
	msg := e.render(def, count, groupKey, samples)

	if _, err := e.meta.InsertEvent(ctx, meta.Event{
		DefinitionID:   def.ID,
		DefinitionName: def.Name,
		FiredAt:        time.Now().UTC(),
		Message:        msg,
		Count:          count,
		GroupKey:       groupKey,
	}); err != nil {
		e.log.Error("storing event", "error", err)
	}

	e.log.Info("event fired", "definition", def.Name, "count", count, "group", groupKey)
	e.dispatch(ctx, def, msg)
	return nil
}

// dispatch sends the message to the definition's channels (best-effort).
func (e *Engine) dispatch(ctx context.Context, def Definition, msg string) {
	for _, chID := range def.Channels {
		ent, err := e.meta.GetEntity(ctx, meta.KindChannel, chID)
		if err != nil {
			continue
		}
		var ch notify.Channel
		if err := json.Unmarshal(ent.Data, &ch); err != nil {
			continue
		}
		if !ch.Enabled {
			continue
		}
		e.decryptChannel(&ch)
		if err := e.notifier.Send(ctx, ch, msg); err != nil {
			e.log.Warn("notification send failed", "channel", ch.Name, "error", err)
			if e.metrics != nil {
				e.metrics.OutputFailures.WithLabelValues(ch.ID).Inc()
			}
		}
	}
}

func (e *Engine) decryptChannel(ch *notify.Channel) {
	if e.cipher == nil {
		return
	}
	ch.URL, _ = e.cipher.Decrypt(ch.URL)
	ch.Token, _ = e.cipher.Decrypt(ch.Token)
	ch.Password, _ = e.cipher.Decrypt(ch.Password)
}

const defaultTemplate = `🚨 {{.Definition}} fired: {{.Count}} match(es){{if .GroupKey}} for "{{.GroupKey}}"{{end}} (threshold {{.Threshold}}) over {{.WindowSeconds}}s.
{{range .Samples}}• {{.}}
{{end}}`

type templateData struct {
	Definition    string
	Count         int
	Threshold     float64
	WindowSeconds int
	GroupKey      string
	Samples       []string
}

func (e *Engine) render(def Definition, count int, groupKey string, samples []string) string {
	tmplText := def.MessageTemplate
	if strings.TrimSpace(tmplText) == "" {
		tmplText = defaultTemplate
	}
	tmpl, err := template.New("evt").Parse(tmplText)
	if err != nil {
		return fmt.Sprintf("%s fired: %d", def.Name, count)
	}
	var buf bytes.Buffer
	_ = tmpl.Execute(&buf, templateData{
		Definition:    def.Name,
		Count:         count,
		Threshold:     def.Threshold,
		WindowSeconds: def.WindowSeconds,
		GroupKey:      groupKey,
		Samples:       samples,
	})
	return buf.String()
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
