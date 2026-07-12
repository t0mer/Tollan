// Package metrics owns Tollan's Prometheus registry and the collectors shared
// across subsystems (ingest, journal, store, search, events, outputs).
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/t0mer/tollan/internal/version"
)

// Metrics holds the registry and the shared collectors. Subsystems receive a
// *Metrics and update the relevant collectors; this keeps a single registry
// and avoids global state.
type Metrics struct {
	Registry *prometheus.Registry

	// Ingest.
	MessagesIn  *prometheus.CounterVec // by input id, type
	MessagesOut *prometheus.CounterVec // by output id, type

	// Journal.
	JournalDepth       prometheus.Gauge
	JournalUtilization prometheus.Gauge // 0..1

	// Processing.
	ProcessingLag prometheus.Gauge // seconds

	// Store.
	StoreDaySizeBytes *prometheus.GaugeVec // by day partition

	// Search.
	SearchLatency prometheus.Histogram // seconds

	// Events / outputs.
	EventFirings   *prometheus.CounterVec // by event definition id
	OutputFailures *prometheus.CounterVec // by output id
}

// New builds the registry with process/go collectors, a build-info metric and
// all shared Tollan collectors registered.
func New() *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	buildInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "tollan",
		Name:      "build_info",
		Help:      "Tollan build information.",
	}, []string{"version", "commit", "goversion"})
	buildInfo.WithLabelValues(version.Version, version.Commit, version.Get().Go).Set(1)
	reg.MustRegister(buildInfo)

	m := &Metrics{
		Registry: reg,
		MessagesIn: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "tollan", Subsystem: "ingest", Name: "messages_in_total",
			Help: "Total messages received, by input.",
		}, []string{"input_id", "type"}),
		MessagesOut: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "tollan", Subsystem: "output", Name: "messages_out_total",
			Help: "Total messages forwarded, by output.",
		}, []string{"output_id", "type"}),
		JournalDepth: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "tollan", Subsystem: "journal", Name: "depth_messages",
			Help: "Number of unprocessed messages in the ingest journal.",
		}),
		JournalUtilization: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "tollan", Subsystem: "journal", Name: "utilization_ratio",
			Help: "Journal disk utilization as a fraction of the configured max size.",
		}),
		ProcessingLag: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "tollan", Subsystem: "processing", Name: "lag_seconds",
			Help: "Age of the oldest unprocessed journal message.",
		}),
		StoreDaySizeBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "tollan", Subsystem: "store", Name: "day_size_bytes",
			Help: "On-disk size of each daily log partition.",
		}, []string{"day"}),
		SearchLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "tollan", Subsystem: "search", Name: "latency_seconds",
			Help:    "Search query latency.",
			Buckets: prometheus.DefBuckets,
		}),
		EventFirings: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "tollan", Subsystem: "event", Name: "firings_total",
			Help: "Total event definition firings.",
		}, []string{"definition_id"}),
		OutputFailures: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "tollan", Subsystem: "output", Name: "failures_total",
			Help: "Total output delivery failures.",
		}, []string{"output_id"}),
	}

	reg.MustRegister(
		m.MessagesIn, m.MessagesOut,
		m.JournalDepth, m.JournalUtilization, m.ProcessingLag,
		m.StoreDaySizeBytes, m.SearchLatency,
		m.EventFirings, m.OutputFailures,
	)
	return m
}

// Handler returns the HTTP handler that exposes /metrics.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{})
}
