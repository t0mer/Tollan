// Package agentfleet implements the tollan-agent collector: it enrolls with a
// Tollan server, tails files (and journald on Linux), ships events over GELF
// TCP, sends heartbeats, and applies server-pushed collector configuration.
package agentfleet

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// Config configures the agent.
type Config struct {
	ServerURL       string
	EnrollmentToken string
	GELFAddr        string // host:port; derived from ServerURL if empty
	Tags            []string
	Files           []string // glob patterns (local, merged with server config)
	DataDir         string
	Version         string
}

// Agent is the collector runner.
type Agent struct {
	cfg     Config
	log     *slog.Logger
	client  *http.Client
	id      string
	shipped atomic.Int64

	mu      sync.Mutex
	tailing map[string]bool // files currently being tailed

	connMu sync.Mutex
	conn   net.Conn
}

// New builds an agent.
func New(cfg Config, log *slog.Logger) (*Agent, error) {
	if cfg.ServerURL == "" {
		return nil, fmt.Errorf("--server is required")
	}
	if cfg.GELFAddr == "" {
		u, err := url.Parse(cfg.ServerURL)
		if err != nil {
			return nil, fmt.Errorf("invalid server url: %w", err)
		}
		cfg.GELFAddr = net.JoinHostPort(u.Hostname(), "12201")
	}
	if cfg.DataDir == "" {
		cfg.DataDir = "./agent-data"
	}
	return &Agent{
		cfg:     cfg,
		log:     log,
		client:  &http.Client{Timeout: 20 * time.Second},
		tailing: map[string]bool{},
	}, nil
}

// Run enrolls and collects until ctx is cancelled.
func (a *Agent) Run(ctx context.Context) error {
	if err := os.MkdirAll(a.cfg.DataDir, 0o750); err != nil {
		return err
	}
	a.id = a.loadOrCreateID()

	cfg, err := a.register(ctx)
	if err != nil {
		a.log.Warn("initial enrollment failed; will retry", "error", err)
	} else {
		a.applyConfig(ctx, cfg)
	}

	// Local file globs from the CLI are always tailed.
	for _, g := range a.cfg.Files {
		a.startGlob(ctx, g)
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			a.heartbeat(ctx)
		}
	}
}

func (a *Agent) loadOrCreateID() string {
	path := filepath.Join(a.cfg.DataDir, "agent-id")
	if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
		return string(bytes.TrimSpace(b))
	}
	id := uuid.NewString()
	_ = os.WriteFile(path, []byte(id), 0o640)
	return id
}

// register enrolls the agent and returns the server-pushed config.
func (a *Agent) register(ctx context.Context) (agentConfig, error) {
	host, _ := os.Hostname()
	body, _ := json.Marshal(map[string]any{
		"enrollment_token": a.cfg.EnrollmentToken,
		"id":               a.id,
		"hostname":         host,
		"os":               runtime.GOOS,
		"version":          a.cfg.Version,
		"tags":             a.cfg.Tags,
	})
	var resp struct {
		Config agentConfig `json:"config"`
	}
	if err := a.postJSON(ctx, "/api/v1/agents/register", body, &resp); err != nil {
		return agentConfig{}, err
	}
	a.log.Info("enrolled with server", "id", a.id, "server", a.cfg.ServerURL)
	return resp.Config, nil
}

func (a *Agent) heartbeat(ctx context.Context) {
	body, _ := json.Marshal(map[string]any{"shipped": a.shipped.Load()})
	var resp struct {
		ConfigVersion int `json:"config_version"`
	}
	if err := a.postJSON(ctx, "/api/v1/agents/"+a.id+"/heartbeat", body, &resp); err != nil {
		// Server may have restarted or forgotten us — re-enroll.
		if cfg, err := a.register(ctx); err == nil {
			a.applyConfig(ctx, cfg)
		}
		return
	}
	// Poll for a newer config and apply it.
	if cfg, err := a.fetchConfig(ctx); err == nil {
		a.applyConfig(ctx, cfg)
	}
}

func (a *Agent) fetchConfig(ctx context.Context) (agentConfig, error) {
	var cfg agentConfig
	err := a.getJSON(ctx, "/api/v1/agents/"+a.id+"/config", &cfg)
	return cfg, err
}

// agentConfig mirrors the server's collector config.
type agentConfig struct {
	Version         int          `json:"version"`
	Files           []fileSource `json:"files"`
	Journald        bool         `json:"journald"`
	WindowsEventLog bool         `json:"windows_event_log"`
}

type fileSource struct {
	Paths            []string `json:"paths"`
	MultilinePattern string   `json:"multiline_pattern"`
}

// applyConfig starts collectors described by the server config.
func (a *Agent) applyConfig(ctx context.Context, cfg agentConfig) {
	for _, fs := range cfg.Files {
		for _, g := range fs.Paths {
			a.startGlob(ctx, g)
		}
	}
	if cfg.Journald && runtime.GOOS == "linux" {
		a.startJournald(ctx)
	}
}

// startGlob tails all files matching a glob (each file once).
func (a *Agent) startGlob(ctx context.Context, glob string) {
	matches, _ := filepath.Glob(glob)
	for _, path := range matches {
		a.mu.Lock()
		if a.tailing[path] {
			a.mu.Unlock()
			continue
		}
		a.tailing[path] = true
		a.mu.Unlock()
		go a.tailFile(ctx, path)
	}
}

// tailFile follows a file, shipping new lines as they appear.
func (a *Agent) tailFile(ctx context.Context, path string) {
	host, _ := os.Hostname()
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return
	}
	reader := bufio.NewReader(f)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for {
				line, err := reader.ReadString('\n')
				if len(line) > 0 {
					a.ship(host, path, bytes.TrimRight([]byte(line), "\r\n"))
				}
				if err != nil {
					break
				}
			}
		}
	}
}

// startJournald follows journald via journalctl -f -o json (Linux).
func (a *Agent) startJournald(ctx context.Context) {
	a.mu.Lock()
	if a.tailing["__journald__"] {
		a.mu.Unlock()
		return
	}
	a.tailing["__journald__"] = true
	a.mu.Unlock()

	go func() {
		host, _ := os.Hostname()
		cmd := exec.CommandContext(ctx, "journalctl", "-f", "-o", "json", "--no-pager", "-n", "0")
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return
		}
		if err := cmd.Start(); err != nil {
			return
		}
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for sc.Scan() {
			var rec map[string]any
			if json.Unmarshal(sc.Bytes(), &rec) != nil {
				continue
			}
			if msg, ok := rec["MESSAGE"].(string); ok {
				a.ship(host, "journald", []byte(msg))
			}
		}
		_ = cmd.Wait()
	}()
}

// ship sends one line to the server over GELF.
func (a *Agent) ship(host, sourceFile string, line []byte) {
	if len(line) == 0 {
		return
	}
	g := map[string]any{
		"version":       "1.1",
		"host":          host,
		"short_message": string(line),
		"timestamp":     float64(time.Now().UnixMilli()) / 1000.0,
		"_source_file":  sourceFile,
		"_agent_id":     a.id,
	}
	payload, _ := json.Marshal(g)
	if err := a.sendGELF(append(payload, 0)); err != nil {
		a.log.Debug("gelf ship failed", "error", err)
		return
	}
	a.shipped.Add(1)
}

// sendGELF writes a null-framed payload over a persistent GELF TCP connection,
// redialing on failure.
func (a *Agent) sendGELF(payload []byte) error {
	a.connMu.Lock()
	defer a.connMu.Unlock()
	for attempt := 0; attempt < 2; attempt++ {
		if a.conn == nil {
			c, err := net.DialTimeout("tcp", a.cfg.GELFAddr, 10*time.Second)
			if err != nil {
				return err
			}
			a.conn = c
		}
		_ = a.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if _, err := a.conn.Write(payload); err != nil {
			_ = a.conn.Close()
			a.conn = nil
			continue
		}
		return nil
	}
	return fmt.Errorf("gelf write failed after reconnect")
}

func (a *Agent) postJSON(ctx context.Context, path string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.ServerURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return a.do(req, out)
}

func (a *Agent) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.cfg.ServerURL+path, nil)
	if err != nil {
		return err
	}
	return a.do(req, out)
}

func (a *Agent) do(req *http.Request, out any) error {
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
