package input

import (
	"context"
	"log/slog"
	"net"
)

// CEF receives ArcSight CEF messages (often over a syslog envelope): one message
// per UDP datagram or per newline over TCP. Parsing happens at decode time.
type CEF struct {
	cfg Config
	pub Publisher
	log *slog.Logger
	srv *netServer
}

// NewCEF builds a CEF input.
func NewCEF(cfg Config, pub Publisher, log *slog.Logger) *CEF {
	return &CEF{cfg: cfg, pub: pub, log: log}
}

func (c *CEF) ID() string   { return c.cfg.ID }
func (c *CEF) Type() string { return "cef" }

func (c *CEF) Start() error {
	c.srv = &netServer{
		proto:    c.cfg.Protocol,
		bind:     c.cfg.Bind,
		log:      c.log,
		onPacket: func(p []byte, src string) { c.emit(p, src) },
		onConn:   c.handleConn,
	}
	if c.cfg.Protocol == TLS {
		tlsCfg, err := loadTLS(c.cfg.TLSCertFile, c.cfg.TLSKeyFile)
		if err != nil {
			return err
		}
		c.srv.tlsCfg = tlsCfg
	}
	return c.srv.start()
}

func (c *CEF) Stop(ctx context.Context) error {
	if c.srv == nil {
		return nil
	}
	return c.srv.stop(ctx)
}

func (c *CEF) handleConn(ctx context.Context, conn net.Conn, source string) {
	scanLines(ctx, conn, func(line []byte) { c.emit(line, source) })
}

func (c *CEF) emit(payload []byte, source string) {
	if len(payload) == 0 {
		return
	}
	_ = c.pub.Publish(RawMessage{
		InputID:    c.cfg.ID,
		InputType:  "cef",
		Source:     source,
		ReceivedAt: nowUTC(),
		Payload:    payload,
	})
}
