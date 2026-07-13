// Package stream routes messages into named categories using per-stream match
// rules, with a built-in default stream that catches everything unrouted.
package stream

import (
	"sync"

	"github.com/t0mer/tollan/internal/schema"
)

const (
	// DefaultID is the id of the built-in catch-all stream.
	DefaultID = "default"
	// DefaultName is the display name of the default stream.
	DefaultName = "All messages"
)

// Router assigns each message to the first stream whose rules match, falling
// back to the default stream. It is safe for concurrent use and hot-reloadable.
type Router struct {
	mu      sync.RWMutex
	streams []*Compiled
}

// NewRouter returns a router with no user streams (everything → default).
func NewRouter() *Router { return &Router{} }

// SetStreams replaces the router's ordered stream set.
func (r *Router) SetStreams(streams []*Compiled) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.streams = streams
}

// Route assigns a stream to the message. A stream already set by a pipeline
// action is respected; otherwise the first matching stream wins, else default.
func (r *Router) Route(m *schema.Message) {
	if m.Stream != "" {
		return
	}
	r.mu.RLock()
	for _, c := range r.streams {
		if c.Matches(m) {
			m.Stream = c.Stream.ID
			r.mu.RUnlock()
			return
		}
	}
	r.mu.RUnlock()
	m.Stream = DefaultID
}
