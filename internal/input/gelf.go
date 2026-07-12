package input

import (
	"bufio"
	"context"
	"log/slog"
	"net"
	"sync"
	"time"
)

// GELF chunk framing: magic 0x1e0f, 8-byte message id, 1-byte seq, 1-byte count.
var gelfChunkMagic = [2]byte{0x1e, 0x0f}

const (
	gelfChunkHeader  = 12
	gelfChunkTimeout = 5 * time.Second
	gelfMaxChunks    = 128
)

// Gelf receives GELF 1.1 messages. UDP handles chunked, (de)compressible
// datagrams and reassembles them; TCP is null-delimited uncompressed JSON. The
// reassembled/framed payload is journaled; JSON decode happens at decode time.
type Gelf struct {
	cfg   Config
	pub   Publisher
	log   *slog.Logger
	srv   *netServer
	reasm *chunkReassembler
}

// NewGelf builds a GELF input.
func NewGelf(cfg Config, pub Publisher, log *slog.Logger) *Gelf {
	return &Gelf{cfg: cfg, pub: pub, log: log, reasm: newChunkReassembler()}
}

func (g *Gelf) ID() string   { return g.cfg.ID }
func (g *Gelf) Type() string { return "gelf" }

func (g *Gelf) Start() error {
	g.srv = &netServer{
		proto:    g.cfg.Protocol,
		bind:     g.cfg.Bind,
		log:      g.log,
		onPacket: g.onPacket,
		onConn:   g.handleConn,
	}
	if g.cfg.Protocol == TLS {
		tlsCfg, err := loadTLS(g.cfg.TLSCertFile, g.cfg.TLSKeyFile)
		if err != nil {
			return err
		}
		g.srv.tlsCfg = tlsCfg
	}
	return g.srv.start()
}

func (g *Gelf) Stop(ctx context.Context) error {
	if g.srv == nil {
		return nil
	}
	return g.srv.stop(ctx)
}

func (g *Gelf) onPacket(p []byte, source string) {
	if len(p) >= 2 && p[0] == gelfChunkMagic[0] && p[1] == gelfChunkMagic[1] {
		if full, ok := g.reasm.add(p, source); ok {
			g.emit(full, source)
		}
		return
	}
	g.emit(p, source)
}

// handleConn frames null-delimited GELF over TCP.
func (g *Gelf) handleConn(ctx context.Context, conn net.Conn, source string) {
	br := bufio.NewReaderSize(conn, 64*1024)
	for {
		if ctx.Err() != nil {
			return
		}
		msg, err := br.ReadBytes(0x00)
		if n := len(msg); n > 0 {
			if msg[n-1] == 0x00 {
				msg = msg[:n-1]
			}
			if len(msg) > 0 {
				g.emit(append([]byte(nil), msg...), source)
			}
		}
		if err != nil {
			return
		}
	}
}

func (g *Gelf) emit(payload []byte, source string) {
	if len(payload) == 0 {
		return
	}
	_ = g.pub.Publish(RawMessage{
		InputID:    g.cfg.ID,
		InputType:  "gelf",
		Source:     source,
		ReceivedAt: nowUTC(),
		Payload:    payload,
	})
}

// chunkReassembler reassembles chunked GELF UDP datagrams by message id.
type chunkReassembler struct {
	mu   sync.Mutex
	msgs map[string]*partialMessage
}

type partialMessage struct {
	count    int
	parts    [][]byte
	received int
	ts       time.Time
}

func newChunkReassembler() *chunkReassembler {
	return &chunkReassembler{msgs: make(map[string]*partialMessage)}
}

// add ingests one chunk and returns the full payload once all chunks arrive.
func (r *chunkReassembler) add(chunk []byte, _ string) ([]byte, bool) {
	if len(chunk) < gelfChunkHeader {
		return nil, false
	}
	id := string(chunk[2:10])
	seq := int(chunk[10])
	count := int(chunk[11])
	if count == 0 || count > gelfMaxChunks || seq >= count {
		return nil, false
	}
	payload := chunk[gelfChunkHeader:]

	r.mu.Lock()
	defer r.mu.Unlock()
	r.evictExpiredLocked()

	p := r.msgs[id]
	if p == nil {
		p = &partialMessage{count: count, parts: make([][]byte, count), ts: nowUTC()}
		r.msgs[id] = p
	}
	if p.parts[seq] == nil {
		p.parts[seq] = append([]byte(nil), payload...)
		p.received++
	}
	if p.received < p.count {
		return nil, false
	}
	delete(r.msgs, id)
	var full []byte
	for _, part := range p.parts {
		full = append(full, part...)
	}
	return full, true
}

func (r *chunkReassembler) evictExpiredLocked() {
	cutoff := nowUTC().Add(-gelfChunkTimeout)
	for id, p := range r.msgs {
		if p.ts.Before(cutoff) {
			delete(r.msgs, id)
		}
	}
}
