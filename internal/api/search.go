package api

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/t0mer/tollan/internal/logstore"
	"github.com/t0mer/tollan/internal/schema"
	"github.com/t0mer/tollan/internal/search/query"
)

// searchResponse is the JSON body returned by GET /api/v1/search.
type searchResponse struct {
	Total    int             `json:"total"`
	Count    int             `json:"count"`
	Messages []searchMessage `json:"messages"`
}

type searchMessage struct {
	ID         string         `json:"id"`
	Timestamp  time.Time      `json:"timestamp"`
	ReceivedAt time.Time      `json:"received_at"`
	Source     string         `json:"source"`
	Stream     string         `json:"stream"`
	InputID    string         `json:"input_id"`
	Message    string         `json:"message"`
	Fields     map[string]any `json:"fields,omitempty"`
}

// parseSearchQuery builds a logstore.Query from the common query params shared by
// the search, histogram and fields endpoints. It parses the q expression into an
// AST; a bad expression is a client error.
func parseSearchQuery(r *http.Request) (logstore.Query, error) {
	q := r.URL.Query()
	from, err := parseTime(q.Get("from"))
	if err != nil {
		return logstore.Query{}, &clientError{"invalid 'from': " + err.Error()}
	}
	to, err := parseTime(q.Get("to"))
	if err != nil {
		return logstore.Query{}, &clientError{"invalid 'to': " + err.Error()}
	}
	expr, err := query.Parse(q.Get("q"))
	if err != nil {
		return logstore.Query{}, &clientError{err.Error()}
	}
	return logstore.Query{From: from, To: to, Expr: expr, Stream: q.Get("stream")}, nil
}

// clientError marks a 400-worthy error.
type clientError struct{ msg string }

func (e *clientError) Error() string { return e.msg }

func (a *API) handleSearch(w http.ResponseWriter, r *http.Request) {
	if a.deps.Store == nil {
		writeError(w, http.StatusServiceUnavailable, "log store unavailable")
		return
	}
	lq, err := parseSearchQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	q := r.URL.Query()
	lq.Limit = parseInt(q.Get("limit"), 100)
	if lq.Limit > 1000 {
		lq.Limit = 1000
	}
	lq.Offset = parseInt(q.Get("offset"), 0)
	if lq.Offset > 100000 {
		lq.Offset = 100000
	}
	lq.Order = logstore.Descending
	if q.Get("order") == "asc" {
		lq.Order = logstore.Ascending
	}

	res, err := a.deps.Store.Search(r.Context(), lq)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search failed: "+err.Error())
		return
	}

	out := searchResponse{Total: res.Total, Count: len(res.Messages)}
	out.Messages = make([]searchMessage, 0, len(res.Messages))
	for _, m := range res.Messages {
		out.Messages = append(out.Messages, searchMessage{
			ID: m.ID, Timestamp: m.Timestamp, ReceivedAt: m.ReceivedAt,
			Source: m.Source, Stream: m.Stream, InputID: m.InputID,
			Message: m.Body, Fields: m.Fields,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// histogramResponse describes the time histogram of a query.
type histogramResponse struct {
	IntervalMillis int64             `json:"interval_ms"`
	Buckets        []logstore.Bucket `json:"buckets"`
}

func (a *API) handleHistogram(w http.ResponseWriter, r *http.Request) {
	if a.deps.Store == nil {
		writeError(w, http.StatusServiceUnavailable, "log store unavailable")
		return
	}
	lq, err := parseSearchQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Default the range to the last 24h so an interval can always be derived.
	if lq.To.IsZero() {
		lq.To = time.Now().UTC()
	}
	if lq.From.IsZero() {
		lq.From = lq.To.Add(-24 * time.Hour)
	}
	interval := niceInterval(lq.From, lq.To)

	buckets, err := a.deps.Store.Histogram(r.Context(), lq, interval)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "histogram failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, histogramResponse{IntervalMillis: interval, Buckets: buckets})
}

// niceInterval picks a bucket size targeting ~60 bars over the range, rounded to
// a human-friendly step.
func niceInterval(from, to time.Time) int64 {
	spanMs := to.Sub(from).Milliseconds()
	if spanMs <= 0 {
		return 1000
	}
	target := spanMs / 60
	steps := []int64{
		1000, 5000, 10000, 30000, 60000, 300000, 600000, 1800000,
		3600000, 10800000, 21600000, 43200000, 86400000, 604800000,
	}
	for _, s := range steps {
		if s >= target {
			return s
		}
	}
	return steps[len(steps)-1]
}

// aggregateResponse is the JSON body for GET /api/v1/search/aggregate.
type aggregateResponse struct {
	Rows []logstore.AggRow `json:"rows"`
}

func (a *API) handleAggregate(w http.ResponseWriter, r *http.Request) {
	if a.deps.Store == nil {
		writeError(w, http.StatusServiceUnavailable, "log store unavailable")
		return
	}
	lq, err := parseSearchQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	q := r.URL.Query()
	spec := logstore.AggSpec{
		GroupBy:     q.Get("group_by"),
		Metric:      logstore.AggMetric(orDefault(q.Get("metric"), "count")),
		MetricField: q.Get("metric_field"),
		Limit:       parseInt(q.Get("limit"), 50),
	}
	rows, err := a.deps.Store.Aggregate(r.Context(), lq, spec)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if rows == nil {
		rows = []logstore.AggRow{}
	}
	writeJSON(w, http.StatusOK, aggregateResponse{Rows: rows})
}

// handleExport streams the current search results as CSV or JSON.
func (a *API) handleExport(w http.ResponseWriter, r *http.Request) {
	if a.deps.Store == nil {
		writeError(w, http.StatusServiceUnavailable, "log store unavailable")
		return
	}
	lq, err := parseSearchQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	lq.Limit = parseInt(r.URL.Query().Get("limit"), 10000)
	if lq.Limit > 100000 {
		lq.Limit = 100000
	}
	lq.Order = logstore.Descending
	res, err := a.deps.Store.Search(r.Context(), lq)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if r.URL.Query().Get("format") == "json" {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="tollan-export.json"`)
		_ = json.NewEncoder(w).Encode(res.Messages)
		return
	}
	// CSV.
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="tollan-export.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"timestamp", "source", "stream", "level", "message"})
	for _, m := range res.Messages {
		lvl, _ := m.StringField("level")
		_ = cw.Write([]string{
			m.Timestamp.Format(time.RFC3339),
			csvSafe(m.Source), csvSafe(m.Stream), csvSafe(lvl), csvSafe(m.Body),
		})
	}
	cw.Flush()
}

// csvSafe neutralizes spreadsheet formula injection: log content is
// attacker-controlled, so a cell starting with a formula trigger is prefixed
// with a single quote to force it to be treated as text.
func csvSafe(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// fieldFacet lists the top values of a field in the current result sample.
type fieldFacet struct {
	Field  string       `json:"field"`
	Values []facetValue `json:"values"`
}

type facetValue struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

func (a *API) handleFields(w http.ResponseWriter, r *http.Request) {
	if a.deps.Store == nil {
		writeError(w, http.StatusServiceUnavailable, "log store unavailable")
		return
	}
	lq, err := parseSearchQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sample := parseInt(r.URL.Query().Get("sample"), 500)
	if sample > 2000 {
		sample = 2000
	}
	topN := parseInt(r.URL.Query().Get("top"), 8)
	lq.Limit = sample
	lq.Order = logstore.Descending

	res, err := a.deps.Store.Search(r.Context(), lq)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "field summary failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, facetsFrom(res.Messages, topN))
}

// facetsFrom computes per-field top values over a sample of messages.
func facetsFrom(msgs []*schema.Message, topN int) []fieldFacet {
	counts := map[string]map[string]int{}
	bump := func(field, val string) {
		if val == "" {
			return
		}
		if counts[field] == nil {
			counts[field] = map[string]int{}
		}
		counts[field][val]++
	}
	for _, m := range msgs {
		bump("source", m.Source)
		bump("input_id", m.InputID)
		bump("stream", m.Stream)
		for k, v := range m.Fields {
			bump(k, stringifyValue(v))
		}
	}

	facets := make([]fieldFacet, 0, len(counts))
	for field, values := range counts {
		vs := make([]facetValue, 0, len(values))
		for v, c := range values {
			vs = append(vs, facetValue{Value: v, Count: c})
		}
		sort.Slice(vs, func(i, j int) bool {
			if vs[i].Count != vs[j].Count {
				return vs[i].Count > vs[j].Count
			}
			return vs[i].Value < vs[j].Value
		})
		if len(vs) > topN {
			vs = vs[:topN]
		}
		facets = append(facets, fieldFacet{Field: field, Values: vs})
	}
	sort.Slice(facets, func(i, j int) bool { return facets[i].Field < facets[j].Field })
	return facets
}

func stringifyValue(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	case nil:
		return ""
	default:
		return ""
	}
}

// parseTime accepts an empty string (zero time), an RFC3339 timestamp, or a
// relative expression like "now", "now-15m" or "now-24h".
func parseTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	if s == "now" {
		return time.Now().UTC(), nil
	}
	if strings.HasPrefix(s, "now-") {
		d, err := time.ParseDuration(strings.TrimPrefix(s, "now-"))
		if err != nil {
			return time.Time{}, err
		}
		return time.Now().UTC().Add(-d), nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

func parseInt(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return def
	}
	return n
}
