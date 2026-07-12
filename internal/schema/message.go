// Package schema defines Tollan's canonical log message and the well-known field
// names that inputs normalize toward. Inputs produce a Message; pipelines,
// streams, the log store and search all operate on it.
package schema

import "time"

// Canonical field names (§5). Inputs and extractors map raw data onto these so
// search and widgets have a stable vocabulary. Values live in Message.Fields;
// Timestamp, Source and Body are promoted to dedicated columns by the store.
const (
	FieldSource      = "source"
	FieldTimestamp   = "timestamp"
	FieldMessage     = "message"
	FieldLevel       = "level"
	FieldFacility    = "facility"
	FieldSrcIP       = "src_ip"
	FieldDstIP       = "dst_ip"
	FieldSrcPort     = "src_port"
	FieldDstPort     = "dst_port"
	FieldUser        = "user"
	FieldEventAction = "event_action"
	FieldProgram     = "program"
	FieldHost        = "host"
)

// Message is a single normalized log record flowing through Tollan.
type Message struct {
	// ID is a unique identifier assigned at ingest.
	ID string `json:"id"`
	// Timestamp is the event time (from the log, or ReceivedAt if absent).
	Timestamp time.Time `json:"timestamp"`
	// ReceivedAt is when Tollan received the message.
	ReceivedAt time.Time `json:"received_at"`
	// Source is the originating host/source identifier.
	Source string `json:"source"`
	// Body is the full human-readable message text (indexed for full-text search).
	Body string `json:"message"`
	// Stream is the id of the stream this message was routed to ("" until routed).
	Stream string `json:"stream,omitempty"`
	// InputID identifies the input that received the message.
	InputID string `json:"input_id,omitempty"`
	// Fields holds all other extracted/parsed fields keyed by canonical name.
	Fields map[string]any `json:"fields,omitempty"`
}

// NewMessage returns a Message with an initialized Fields map and ReceivedAt set
// to now (in UTC).
func NewMessage(now time.Time) *Message {
	return &Message{
		ReceivedAt: now.UTC(),
		Fields:     make(map[string]any),
	}
}

// SetField sets a field, allocating the map if needed.
func (m *Message) SetField(key string, value any) {
	if m.Fields == nil {
		m.Fields = make(map[string]any)
	}
	m.Fields[key] = value
}

// GetField returns a field value and whether it was present.
func (m *Message) GetField(key string) (any, bool) {
	v, ok := m.Fields[key]
	return v, ok
}

// EnsureTimestamp defaults an unset event Timestamp to ReceivedAt.
func (m *Message) EnsureTimestamp() {
	if m.Timestamp.IsZero() {
		m.Timestamp = m.ReceivedAt
	}
}
