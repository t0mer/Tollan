package input

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net"
	"strconv"
)

// Syslog receives syslog messages. UDP delivers one message per datagram. TCP
// supports both octet-counting ("123 <PRI>...") and newline framing (RFC 6587).
// The bytes are journaled as-is; RFC 3164/5424 parsing happens at decode time.
type Syslog struct {
	cfg Config
	pub Publisher
	log *slog.Logger
	srv *netServer
}

// NewSyslog builds a syslog input.
func NewSyslog(cfg Config, pub Publisher, log *slog.Logger) *Syslog {
	return &Syslog{cfg: cfg, pub: pub, log: log}
}

func (s *Syslog) ID() string   { return s.cfg.ID }
func (s *Syslog) Type() string { return "syslog" }

func (s *Syslog) Start() error {
	s.srv = &netServer{
		proto:    s.cfg.Protocol,
		bind:     s.cfg.Bind,
		log:      s.log,
		onPacket: func(p []byte, src string) { s.emit(p, src) },
		onConn:   s.handleConn,
	}
	if s.cfg.Protocol == TLS {
		tlsCfg, err := loadTLS(s.cfg.TLSCertFile, s.cfg.TLSKeyFile)
		if err != nil {
			return err
		}
		s.srv.tlsCfg = tlsCfg
	}
	return s.srv.start()
}

func (s *Syslog) Stop(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}
	return s.srv.stop(ctx)
}

func (s *Syslog) emit(payload []byte, source string) {
	if len(payload) == 0 {
		return
	}
	_ = s.pub.Publish(RawMessage{
		InputID:    s.cfg.ID,
		InputType:  "syslog",
		Source:     source,
		ReceivedAt: nowUTC(),
		Payload:    payload,
	})
}

// handleConn frames TCP syslog. It auto-detects octet-counting (a leading
// decimal length followed by a space) versus newline framing per connection.
func (s *Syslog) handleConn(ctx context.Context, conn net.Conn, source string) {
	br := bufio.NewReaderSize(conn, 64*1024)
	for {
		if ctx.Err() != nil {
			return
		}
		b, err := br.Peek(1)
		if err != nil {
			return
		}
		if b[0] >= '0' && b[0] <= '9' {
			if s.readOctetFramed(br, source) {
				continue
			}
			return
		}
		// Newline framing.
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			trimmed := trimCRLF(line)
			if len(trimmed) > 0 {
				s.emit(append([]byte(nil), trimmed...), source)
			}
		}
		if err != nil {
			return
		}
	}
}

// readOctetFramed reads one octet-counted frame ("<len> <bytes>"); returns false
// on error so the caller stops.
func (s *Syslog) readOctetFramed(br *bufio.Reader, source string) bool {
	lenStr, err := br.ReadString(' ')
	if err != nil {
		return false
	}
	n, err := strconv.Atoi(trimSpace(lenStr))
	if err != nil || n <= 0 || n > maxLineBytes {
		return false
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(br, buf); err != nil {
		return false
	}
	s.emit(buf, source)
	return true
}

func trimCRLF(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
