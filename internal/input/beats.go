package input

import (
	"context"
	"encoding/json"
	"io"
	stdlog "log"
	"log/slog"

	"github.com/elastic/go-lumber/server"
)

// go-lumber emits verbose debug output through the standard library logger;
// Tollan logs exclusively via slog, so route that noise to /dev/null.
func init() { stdlog.SetOutput(io.Discard) }

// Beats receives events from Filebeat/Winlogbeat over the Lumberjack v2 protocol
// (with acks). Each event is a JSON object journaled with type "beats".
type Beats struct {
	cfg  Config
	pub  Publisher
	log  *slog.Logger
	srv  server.Server
	done chan struct{}
}

// NewBeats builds a Beats input.
func NewBeats(cfg Config, pub Publisher, log *slog.Logger) *Beats {
	return &Beats{cfg: cfg, pub: pub, log: log, done: make(chan struct{})}
}

func (b *Beats) ID() string   { return b.cfg.ID }
func (b *Beats) Type() string { return "beats" }

func (b *Beats) Start() error {
	ln, err := listen(b.cfg.Bind)
	if err != nil {
		return err
	}
	opts := []server.Option{server.V2(true), server.V1(true)}
	if b.cfg.Protocol == TLS {
		tlsCfg, err := loadTLS(b.cfg.TLSCertFile, b.cfg.TLSKeyFile)
		if err != nil {
			ln.Close()
			return err
		}
		opts = append(opts, server.TLS(tlsCfg))
	}
	s, err := server.NewWithListener(ln, opts...)
	if err != nil {
		ln.Close()
		return err
	}
	b.srv = s
	go b.consume()
	return nil
}

func (b *Beats) consume() {
	for {
		select {
		case <-b.done:
			return
		case batch := <-b.srv.ReceiveChan():
			if batch == nil {
				return
			}
			for _, evt := range batch.Events {
				payload, err := json.Marshal(evt)
				if err != nil {
					continue
				}
				_ = b.pub.Publish(RawMessage{
					InputID:    b.cfg.ID,
					InputType:  "beats",
					Source:     "",
					ReceivedAt: nowUTC(),
					Payload:    payload,
				})
			}
			batch.ACK()
		}
	}
}

func (b *Beats) Stop(ctx context.Context) error {
	if b.srv == nil {
		return nil
	}
	close(b.done)
	return b.srv.Close()
}
