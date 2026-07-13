package input

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// defaultDockerEndpoint is used when the input's Bind is empty.
const defaultDockerEndpoint = "unix:///var/run/docker.sock"

// Docker streams logs from the Docker Engine API: it discovers running
// containers (and newly-started ones via the event stream), follows each
// container's stdout/stderr, and journals each line enriched with container
// metadata. Requires access to the Docker socket (mount it into the container).
type Docker struct {
	cfg  Config
	pub  Publisher
	log  *slog.Logger
	http *http.Client
	base string

	cancel    context.CancelFunc
	wg        sync.WaitGroup
	mu        sync.Mutex
	following map[string]bool
}

// NewDocker builds a Docker input.
func NewDocker(cfg Config, pub Publisher, log *slog.Logger) *Docker {
	return &Docker{cfg: cfg, pub: pub, log: log, following: map[string]bool{}}
}

func (d *Docker) ID() string   { return d.cfg.ID }
func (d *Docker) Type() string { return "docker" }

func (d *Docker) Start() error {
	endpoint := d.cfg.Bind
	if endpoint == "" {
		endpoint = defaultDockerEndpoint
	}
	client, base, err := dockerClient(endpoint)
	if err != nil {
		return err
	}
	d.http = client
	d.base = base

	ctx, cancel := context.WithCancel(context.Background())
	d.cancel = cancel
	// Connectivity is checked in the loop so a missing socket doesn't abort the
	// other inputs; the loop keeps retrying and logs when it connects.
	d.wg.Add(1)
	go d.watch(ctx)
	return nil
}

func (d *Docker) Stop(ctx context.Context) error {
	if d.cancel != nil {
		d.cancel()
	}
	done := make(chan struct{})
	go func() { d.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// dockerClient builds an HTTP client (without a timeout, for streaming) bound to
// a unix socket or tcp endpoint.
func dockerClient(endpoint string) (*http.Client, string, error) {
	switch {
	case strings.HasPrefix(endpoint, "unix://"):
		sock := strings.TrimPrefix(endpoint, "unix://")
		tr := &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", sock)
			},
		}
		return &http.Client{Transport: tr}, "http://docker", nil
	case strings.HasPrefix(endpoint, "tcp://"):
		return &http.Client{}, "http://" + strings.TrimPrefix(endpoint, "tcp://"), nil
	case strings.HasPrefix(endpoint, "http://"), strings.HasPrefix(endpoint, "https://"):
		return &http.Client{}, strings.TrimRight(endpoint, "/"), nil
	default:
		return nil, "", fmt.Errorf("docker: unsupported endpoint %q", endpoint)
	}
}

// watch discovers containers and follows the event stream, reconnecting on
// failure.
func (d *Docker) watch(ctx context.Context) {
	defer d.wg.Done()
	connected := false
	for ctx.Err() == nil {
		if err := d.followExisting(ctx); err != nil {
			if connected {
				d.log.Warn("docker connection lost", "error", err)
				connected = false
			} else {
				d.log.Debug("docker not reachable, retrying", "error", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}
		if !connected {
			d.log.Info("docker connected", "endpoint", d.cfg.Bind)
			connected = true
		}
		// followEvents blocks until the stream ends or ctx is cancelled.
		if err := d.followEvents(ctx); err != nil && ctx.Err() == nil {
			d.log.Debug("docker event stream ended", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

type containerSummary struct {
	ID    string   `json:"Id"`
	Names []string `json:"Names"`
	Image string   `json:"Image"`
}

// followExisting starts following all currently-running containers.
func (d *Docker) followExisting(ctx context.Context) error {
	resp, err := d.get(ctx, "/containers/json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("containers list: status %d", resp.StatusCode)
	}
	var list []containerSummary
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return err
	}
	for _, c := range list {
		d.startFollow(ctx, c.ID, cleanName(c.Names), c.Image)
	}
	return nil
}

type eventMsg struct {
	Type   string `json:"Type"`
	Action string `json:"Action"`
	Actor  struct {
		ID         string            `json:"ID"`
		Attributes map[string]string `json:"Attributes"`
	} `json:"Actor"`
}

// followEvents follows container start events to pick up new containers.
func (d *Docker) followEvents(ctx context.Context) error {
	resp, err := d.get(ctx, `/events?filters={"type":["container"],"event":["start"]}`)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	dec := json.NewDecoder(resp.Body)
	for {
		var e eventMsg
		if err := dec.Decode(&e); err != nil {
			return err
		}
		if e.Type == "container" && e.Action == "start" {
			name := e.Actor.Attributes["name"]
			image := e.Actor.Attributes["image"]
			d.startFollow(ctx, e.Actor.ID, name, image)
		}
	}
}

// startFollow begins following a container's logs (once).
func (d *Docker) startFollow(ctx context.Context, id, name, image string) {
	d.mu.Lock()
	if d.following[id] {
		d.mu.Unlock()
		return
	}
	d.following[id] = true
	d.mu.Unlock()

	tty := d.inspectTTY(ctx, id)
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer func() {
			d.mu.Lock()
			delete(d.following, id)
			d.mu.Unlock()
		}()
		d.followLogs(ctx, id, name, image, tty)
	}()
}

// inspectTTY reports whether a container was started with a TTY (raw log stream).
func (d *Docker) inspectTTY(ctx context.Context, id string) bool {
	resp, err := d.get(ctx, "/containers/"+id+"/json")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var info struct {
		Config struct{ Tty bool } `json:"Config"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&info)
	return info.Config.Tty
}

// followLogs streams and demultiplexes a container's logs.
func (d *Docker) followLogs(ctx context.Context, id, name, image string, tty bool) {
	resp, err := d.get(ctx, "/containers/"+id+"/logs?follow=1&stdout=1&stderr=1&tail=0&timestamps=1")
	if err != nil {
		return
	}
	defer resp.Body.Close()

	emit := func(stream, line string) { d.emit(id, name, image, stream, line) }
	if tty {
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
		for sc.Scan() {
			if ctx.Err() != nil {
				return
			}
			emit("tty", sc.Text())
		}
		return
	}
	demux(ctx, resp.Body, emit)
}

// demux reads Docker's multiplexed stdout/stderr framing and emits lines.
func demux(ctx context.Context, r io.Reader, emit func(stream, line string)) {
	header := make([]byte, 8)
	bufs := map[byte]*strings.Builder{1: {}, 2: {}}
	for {
		if ctx.Err() != nil {
			return
		}
		if _, err := io.ReadFull(r, header); err != nil {
			return
		}
		streamType := header[0]
		size := binary.BigEndian.Uint32(header[4:8])
		if size == 0 {
			continue
		}
		payload := make([]byte, size)
		if _, err := io.ReadFull(r, payload); err != nil {
			return
		}
		b := bufs[streamType]
		if b == nil {
			b = &strings.Builder{}
			bufs[streamType] = b
		}
		streamName := "stdout"
		if streamType == 2 {
			streamName = "stderr"
		}
		flushLines(b, string(payload), func(line string) { emit(streamName, line) })
	}
}

// flushLines appends data to buf, emitting complete lines and keeping the
// trailing partial line buffered.
func flushLines(buf *strings.Builder, data string, emit func(string)) {
	buf.WriteString(data)
	s := buf.String()
	for {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimRight(s[:i], "\r")
		if line != "" {
			emit(line)
		}
		s = s[i+1:]
	}
	buf.Reset()
	buf.WriteString(s)
}

// emit builds an enriched JSON message from a log line. timestamps=1 prefixes
// each line with an RFC3339Nano timestamp.
func (d *Docker) emit(id, name, image, stream, line string) {
	ts := time.Time{}
	msg := line
	if idx := strings.IndexByte(line, ' '); idx > 0 {
		if t, err := time.Parse(time.RFC3339Nano, line[:idx]); err == nil {
			ts = t.UTC()
			msg = line[idx+1:]
		}
	}
	if msg == "" {
		return
	}
	shortID := id
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}
	obj := map[string]any{
		"host":             name,
		"message":          msg,
		"container_name":   name,
		"container_id":     shortID,
		"image":            image,
		"container_stream": stream,
	}
	if !ts.IsZero() {
		obj["timestamp"] = ts.Format(time.RFC3339Nano)
	}
	payload, err := json.Marshal(obj)
	if err != nil {
		return
	}
	_ = d.pub.Publish(RawMessage{
		InputID:    d.cfg.ID,
		InputType:  "docker",
		Source:     name,
		ReceivedAt: nowUTC(),
		Payload:    payload,
	})
}

// get issues a GET against the Docker API.
func (d *Docker) get(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.base+path, nil)
	if err != nil {
		return nil, err
	}
	return d.http.Do(req)
}

// cleanName picks the primary container name and strips the leading slash.
func cleanName(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return strings.TrimPrefix(names[0], "/")
}
