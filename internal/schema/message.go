// Package schema defines Tollan's canonical log message and the well-known field
// names that inputs normalize toward. Inputs produce a Message; pipelines,
// streams, the log store and search all operate on it.
package schema

import (
	"strconv"
	"time"
)

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

// StringField returns a message attribute as a string. It resolves the promoted
// columns (message/source/stream/input_id) and otherwise looks in Fields. The
// bool reports whether the value was present and non-empty.
func (m *Message) StringField(name string) (string, bool) {
	switch name {
	case FieldMessage, "body":
		return m.Body, m.Body != ""
	case FieldSource:
		return m.Source, m.Source != ""
	case "stream":
		return m.Stream, m.Stream != ""
	case "input_id":
		return m.InputID, m.InputID != ""
	}
	v, ok := m.Fields[name]
	if !ok || v == nil {
		return "", false
	}
	return stringifyField(v), true
}

// stringifyField renders a field value as a string for matching/serialization.
func stringifyField(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	default:
		return ""
	}
}

// NumberField returns a message field as a float64 if it is numeric.
func (m *Message) NumberField(name string) (float64, bool) {
	s, ok := m.StringField(name)
	if !ok {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}
