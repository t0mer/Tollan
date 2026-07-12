package input

import (
	"bufio"
	"context"
	"log/slog"
	"net"
)

// maxLineBytes caps a single stream-framed message (1 MiB).
const maxLineBytes = 1 << 20

// Raw is a plain-text input: one message per UDP datagram, or one message per
// newline-delimited line over TCP.
type Raw struct {
	cfg Config
	pub Publisher
	log *slog.Logger
	srv *netServer
}

// NewRaw builds a raw input.
func NewRaw(cfg Config, pub Publisher, log *slog.Logger) *Raw {
	return &Raw{cfg: cfg, pub: pub, log: log}
}

func (r *Raw) ID() string   { return r.cfg.ID }
func (r *Raw) Type() string { return "raw" }

func (r *Raw) Start() error {
	r.srv = &netServer{
		proto:    r.cfg.Protocol,
		bind:     r.cfg.Bind,
		log:      r.log,
		onPacket: func(p []byte, src string) { r.emit(p, src) },
		onConn:   r.handleConn,
	}
	if r.cfg.Protocol == TLS {
		tlsCfg, err := loadTLS(r.cfg.TLSCertFile, r.cfg.TLSKeyFile)
		if err != nil {
			return err
		}
		r.srv.tlsCfg = tlsCfg
	}
	return r.srv.start()
}

func (r *Raw) Stop(ctx context.Context) error {
	if r.srv == nil {
		return nil
	}
	return r.srv.stop(ctx)
}

func (r *Raw) handleConn(ctx context.Context, conn net.Conn, source string) {
	scanLines(ctx, conn, func(line []byte) { r.emit(line, source) })
}

func (r *Raw) emit(payload []byte, source string) {
	if len(payload) == 0 {
		return
	}
	_ = r.pub.Publish(RawMessage{
		InputID:    r.cfg.ID,
		InputType:  "raw",
		Source:     source,
		ReceivedAt: nowUTC(),
		Payload:    payload,
	})
}

// scanLines reads newline-delimited messages from conn, invoking emit for each
// non-empty line, until ctx is cancelled or the connection closes.
func scanLines(ctx context.Context, conn net.Conn, emit func([]byte)) {
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
	for sc.Scan() {
		if ctx.Err() != nil {
			return
		}
		line := sc.Bytes()
		out := make([]byte, len(line))
		copy(out, line)
		emit(out)
	}
}
