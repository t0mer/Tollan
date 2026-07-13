// Package processing consumes the ingest journal, decodes each raw message into
// a canonical schema.Message, routes it to a stream, and persists it to the log
// store, committing journal progress after each batch. This is the bridge
// between ingest (inputs → journal) and storage/search.
package processing

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/t0mer/tollan/internal/decode"
	"github.com/t0mer/tollan/internal/journal"
	"github.com/t0mer/tollan/internal/logstore"
	"github.com/t0mer/tollan/internal/metrics"
	"github.com/t0mer/tollan/internal/output"
	"github.com/t0mer/tollan/internal/pipeline"
	"github.com/t0mer/tollan/internal/schema"
)

// Defaults for batching.
const (
	defaultBatchSize = 500
	defaultBatchWait = 25 * time.Millisecond
	storeRetries     = 3
	storeRetryWait   = 100 * time.Millisecond
)

// Processor drives journal → decode → pipeline (route + enrich) → store.
type Processor struct {
	journal *journal.Journal
	store   logstore.Store
	engine  *pipeline.Engine
	outputs *output.Manager
	log     *slog.Logger
	metrics *metrics.Metrics

	batchSize int
	batchWait time.Duration
}

// Options configures a Processor.
type Options struct {
	Journal   *journal.Journal
	Store     logstore.Store
	Engine    *pipeline.Engine
	Outputs   *output.Manager
	Logger    *slog.Logger
	Metrics   *metrics.Metrics
	BatchSize int
	BatchWait time.Duration
}

// New builds a Processor.
func New(opts Options) *Processor {
	if opts.BatchSize <= 0 {
		opts.BatchSize = defaultBatchSize
	}
	if opts.BatchWait <= 0 {
		opts.BatchWait = defaultBatchWait
	}
	return &Processor{
		journal:   opts.Journal,
		store:     opts.Store,
		engine:    opts.Engine,
		outputs:   opts.Outputs,
		log:       opts.Logger,
		metrics:   opts.Metrics,
		batchSize: opts.BatchSize,
		batchWait: opts.BatchWait,
	}
}

type item struct {
	seq uint64
	rec journal.Record
}

// Run consumes the journal until it is closed and drained, or ctx is cancelled.
func (p *Processor) Run(ctx context.Context) error {
	r := p.journal.NewReader()
	defer r.Close()

	for {
		seq, rec, err := r.Next(ctx)
		if err != nil {
			if errors.Is(err, journal.ErrClosed) || errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}
		batch := []item{{seq, rec}}
		batch = p.gather(r, batch)
		p.processBatch(ctx, batch)
	}
}

// gather greedily collects more records (up to batchSize) that are already
// available, waiting at most batchWait for each, to amortize store writes.
func (p *Processor) gather(r *journal.Reader, batch []item) []item {
	for len(batch) < p.batchSize {
		bctx, cancel := context.WithTimeout(context.Background(), p.batchWait)
		seq, rec, err := r.Next(bctx)
		cancel()
		if err != nil {
			break
		}
		batch = append(batch, item{seq, rec})
	}
	return batch
}

func (p *Processor) processBatch(ctx context.Context, batch []item) {
	msgs := make([]*schema.Message, 0, len(batch))
	var maxSeq uint64
	var lastReceived time.Time
	for _, it := range batch {
		if it.seq > maxSeq {
			maxSeq = it.seq
		}
		lastReceived = it.rec.ReceivedAt
		m, err := decode.Decode(it.rec.InputType, it.rec.Source, it.rec.ReceivedAt, it.rec.Payload)
		if err != nil {
			p.log.Warn("decode failed", "input", it.rec.InputID, "type", it.rec.InputType, "error", err)
			continue
		}
		m.ID = uuid.NewString()
		m.InputID = it.rec.InputID
		if m.ReceivedAt.IsZero() {
			m.ReceivedAt = it.rec.ReceivedAt
		}
		m.EnsureTimestamp()
		// Pipelines normalize/enrich, route to a stream, and may drop the message.
		if p.engine.Process(m) {
			continue
		}
		m.EnsureTimestamp() // pipelines may have set/changed the timestamp
		msgs = append(msgs, m)
	}

	if err := p.storeWithRetry(ctx, msgs); err != nil {
		// After exhausting retries we skip the batch to avoid a poison-pill
		// stall; the loss is logged and counted.
		p.log.Error("dropping batch after store failures", "count", len(msgs), "error", err)
	} else if p.outputs != nil {
		// Forward stored messages to configured outputs (best-effort).
		for _, m := range msgs {
			p.outputs.Dispatch(m)
		}
	}

	// Advance the journal past this batch regardless, so we make progress.
	if err := p.journal.Commit(maxSeq); err != nil {
		p.log.Error("journal commit failed", "seq", maxSeq, "error", err)
	}
	p.updateMetrics(lastReceived)
}

func (p *Processor) storeWithRetry(ctx context.Context, msgs []*schema.Message) error {
	if len(msgs) == 0 {
		return nil
	}
	var err error
	for attempt := 0; attempt < storeRetries; attempt++ {
		if err = p.store.Store(ctx, msgs); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(storeRetryWait):
		}
	}
	return err
}

func (p *Processor) updateMetrics(lastReceived time.Time) {
	if p.metrics == nil {
		return
	}
	p.metrics.JournalDepth.Set(float64(p.journal.Depth()))
	p.metrics.JournalUtilization.Set(p.journal.Utilization())
	if !lastReceived.IsZero() {
		lag := time.Since(lastReceived).Seconds()
		if lag < 0 {
			lag = 0
		}
		p.metrics.ProcessingLag.Set(lag)
	}
}
