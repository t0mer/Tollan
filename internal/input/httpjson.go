package input

import (
	"bufio"
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// maxHTTPBody caps a single HTTP-JSON request body (16 MiB).
const maxHTTPBody = 16 << 20

// HTTPJSON receives logs over HTTP POST: a single JSON object, or newline-
// delimited JSON (NDJSON) for bulk. Optional token auth guards the endpoint.
type HTTPJSON struct {
	cfg    Config
	pub    Publisher
	log    *slog.Logger
	server *http.Server
}

// NewHTTPJSON builds an HTTP-JSON input.
func NewHTTPJSON(cfg Config, pub Publisher, log *slog.Logger) *HTTPJSON {
	return &HTTPJSON{cfg: cfg, pub: pub, log: log}
}

func (h *HTTPJSON) ID() string   { return h.cfg.ID }
func (h *HTTPJSON) Type() string { return "httpjson" }

func (h *HTTPJSON) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", h.handle)
	h.server = &http.Server{
		Addr:         h.cfg.Bind,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 15 * time.Second,
	}
	ln, err := listen(h.cfg.Bind)
	if err != nil {
		return err
	}
	go func() {
		if err := h.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			h.log.Warn("http-json server stopped", "error", err)
		}
	}()
	return nil
}

func (h *HTTPJSON) Stop(ctx context.Context) error {
	if h.server == nil {
		return nil
	}
	return h.server.Shutdown(ctx)
}

func (h *HTTPJSON) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if !h.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxHTTPBody))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	n := 0
	if isNDJSON(r, body) {
		sc := bufio.NewScanner(bytes.NewReader(body))
		sc.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
		for sc.Scan() {
			line := bytes.TrimSpace(sc.Bytes())
			if len(line) == 0 {
				continue
			}
			h.emit(append([]byte(nil), line...), r)
			n++
		}
	} else if len(bytes.TrimSpace(body)) > 0 {
		h.emit(body, r)
		n++
	}
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]int{"accepted": n})
}

func (h *HTTPJSON) authorized(r *http.Request) bool {
	if h.cfg.Token == "" {
		return true
	}
	got := r.Header.Get("X-API-Token")
	if got == "" {
		got = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(h.cfg.Token)) == 1
}

func (h *HTTPJSON) emit(payload []byte, r *http.Request) {
	_ = h.pub.Publish(RawMessage{
		InputID:    h.cfg.ID,
		InputType:  "httpjson",
		Source:     hostFromRequest(r),
		ReceivedAt: nowUTC(),
		Payload:    payload,
	})
}

func isNDJSON(r *http.Request, body []byte) bool {
	if strings.Contains(r.Header.Get("Content-Type"), "ndjson") {
		return true
	}
	// Heuristic: multiple non-empty lines each starting with '{'.
	return bytes.Count(bytes.TrimSpace(body), []byte("\n")) > 0
}

func hostFromRequest(r *http.Request) string {
	host := r.RemoteAddr
	if i := strings.LastIndexByte(host, ':'); i > 0 {
		host = host[:i]
	}
	return host
}
