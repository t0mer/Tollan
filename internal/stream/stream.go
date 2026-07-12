// Package stream routes messages into named categories. This phase provides the
// default stream that catches everything unrouted; user-defined streams with
// match rules arrive in a later phase.
package stream

import "github.com/t0mer/tollan/internal/schema"

const (
	// DefaultID is the id of the built-in catch-all stream.
	DefaultID = "default"
	// DefaultName is the display name of the default stream.
	DefaultName = "All messages"
)

// Router assigns each message to a stream. The phase-2 router sends everything
// to the default stream; it is replaced by a rule-driven router later.
type Router struct{}

// NewRouter returns the default router.
func NewRouter() *Router { return &Router{} }

// Route assigns a stream to the message if none is set.
func (r *Router) Route(m *schema.Message) {
	if m.Stream == "" {
		m.Stream = DefaultID
	}
}
