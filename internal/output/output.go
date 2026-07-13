// Package output forwards stored messages to downstream systems: GELF (TCP/UDP),
// raw TCP (one message per line), TCP syslog (RFC 5424) and STDOUT. Each output
// runs a buffered worker with reconnect-and-retry; overflow is dropped and
// counted rather than blocking ingest.
package output

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/t0mer/tollan/internal/metrics"
	"github.com/t0mer/tollan/internal/schema"
)

// Type is an output backend.
type Type string

const (
	TypeGELF      Type = "gelf"
	TypeTCPRaw    Type = "tcp_raw"
	TypeTCPSyslog Type = "tcp_syslog"
	TypeStdout    Type = "stdout"
)

// Output is a forwarding target.
type Output struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Enabled  bool   `json:"enabled"`
	Type     Type   `json:"type"`
	Stream   string `json:"stream"`   // "" forwards all streams
	Address  string `json:"address"`  // host:port for network types
	Protocol string `json:"protocol"` // tcp|udp (GELF)
}

const bufferSize = 4096

// Manager runs the configured outputs.
type Manager struct {
	log     *slog.Logger
	metrics *metrics.Metrics

	mu      sync.RWMutex
	running []*worker
}

// NewManager returns an empty output manager.
func NewManager(log *slog.Logger, m *metrics.Metrics) *Manager {
	return &Manager{log: log, metrics: m}
}

// SetOutputs replaces the running outputs.
func (m *Manager) SetOutputs(outs []Output) {
	m.mu.Lock()
	old := m.running
	var running []*worker
	for _, o := range outs {
		if !o.Enabled {
			continue
		}
		w := newWorker(o, m.log, m.metrics)
		w.start()
		running = append(running, w)
	}
	m.running = running
	m.mu.Unlock()

	for _, w := range old {
		w.stop()
	}
}

// Dispatch enqueues a message to every output whose stream matches. It never
// blocks: a full buffer drops the message (counted).
func (m *Manager) Dispatch(msg *schema.Message) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, w := range m.running {
		if w.out.Stream != "" && w.out.Stream != msg.Stream {
			continue
		}
		select {
		case w.ch <- msg:
		default:
			if m.metrics != nil {
				m.metrics.OutputFailures.WithLabelValues(w.out.ID).Inc()
			}
		}
	}
}

// Stop halts all outputs.
func (m *Manager) Stop() {
	m.mu.Lock()
	old := m.running
	m.running = nil
	m.mu.Unlock()
	for _, w := range old {
		w.stop()
	}
}

type worker struct {
	out     Output
	ch      chan *schema.Message
	done    chan struct{}
	log     *slog.Logger
	metrics *metrics.Metrics
	conn    net.Conn
}

func newWorker(o Output, log *slog.Logger, m *metrics.Metrics) *worker {
	return &worker{out: o, ch: make(chan *schema.Message, bufferSize), done: make(chan struct{}), log: log, metrics: m}
}

func (w *worker) start() { go w.run() }

func (w *worker) stop() {
	close(w.done)
	if w.conn != nil {
		_ = w.conn.Close()
	}
}

func (w *worker) run() {
	for {
		select {
		case <-w.done:
			return
		case msg := <-w.ch:
			if err := w.send(msg); err != nil {
				w.log.Warn("output send failed", "output", w.out.Name, "error", err)
				if w.metrics != nil {
					w.metrics.OutputFailures.WithLabelValues(w.out.ID).Inc()
				}
				continue
			}
			if w.metrics != nil {
				w.metrics.MessagesOut.WithLabelValues(w.out.ID, string(w.out.Type)).Inc()
			}
		}
	}
}

func (w *worker) send(msg *schema.Message) error {
	switch w.out.Type {
	case TypeStdout:
		fmt.Println(msg.Body)
		return nil
	case TypeGELF:
		return w.writeBytes(append(gelfBytes(msg), 0))
	case TypeTCPRaw:
		return w.writeBytes([]byte(msg.Body + "\n"))
	case TypeTCPSyslog:
		return w.writeBytes([]byte(syslog5424(msg) + "\n"))
	default:
		return fmt.Errorf("unknown output type %q", w.out.Type)
	}
}

// writeBytes writes to the output's connection, dialing/redialing as needed and
// retrying once on a broken connection.
func (w *worker) writeBytes(b []byte) error {
	for attempt := 0; attempt < 2; attempt++ {
		if w.conn == nil {
			c, err := w.dial()
			if err != nil {
				return err
			}
			w.conn = c
		}
		_ = w.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if _, err := w.conn.Write(b); err != nil {
			_ = w.conn.Close()
			w.conn = nil
			continue // reconnect and retry
		}
		return nil
	}
	return fmt.Errorf("write failed after reconnect")
}

func (w *worker) dial() (net.Conn, error) {
	network := "tcp"
	if w.out.Type == TypeGELF && strings.EqualFold(w.out.Protocol, "udp") {
		network = "udp"
	}
	return net.DialTimeout(network, w.out.Address, 10*time.Second)
}

// gelfBytes renders a message as a GELF 1.1 payload.
func gelfBytes(m *schema.Message) []byte {
	g := map[string]any{
		"version":       "1.1",
		"host":          orValue(m.Source, "tollan"),
		"short_message": m.Body,
		"timestamp":     float64(m.Timestamp.UnixMilli()) / 1000.0,
	}
	for k, v := range m.Fields {
		g["_"+k] = v
	}
	b, _ := json.Marshal(g)
	return b
}

// syslog5424 renders a message as an RFC 5424 line.
func syslog5424(m *schema.Message) string {
	ts := m.Timestamp.UTC().Format(time.RFC3339)
	host := orValue(m.Source, "-")
	app := "-"
	if v, ok := m.StringField(schema.FieldProgram); ok {
		app = v
	}
	return fmt.Sprintf("<134>1 %s %s %s - - - %s", ts, host, app, m.Body)
}

func orValue(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
