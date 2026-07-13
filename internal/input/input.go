// Package input defines Tollan's ingest inputs: listeners that receive log data
// over a transport, frame it into individual messages, and publish the raw
// bytes to the journal. Per-protocol decoding into schema.Message happens later,
// in the processing workers — inputs stay thin so they can ack quickly.
package input

import (
	"context"
	"fmt"
	"time"
)

// RawMessage is a single framed message as received by an input, before decode.
type RawMessage struct {
	InputID    string
	InputType  string
	Source     string
	ReceivedAt time.Time
	Payload    []byte
}

// Publisher receives raw messages from inputs and durably enqueues them. The
// journal-backed implementation appends to the ingest journal.
type Publisher interface {
	Publish(RawMessage) error
}

// Input is a runnable ingest listener.
type Input interface {
	// ID is the unique input identifier.
	ID() string
	// Type is the protocol type (syslog, gelf, raw, ...).
	Type() string
	// Start begins listening. It returns once listeners are bound (or on bind
	// error) and runs until Stop is called.
	Start() error
	// Stop stops the input and releases its listeners.
	Stop(ctx context.Context) error
}

// Protocol identifies the transport for an input.
type Protocol string

const (
	UDP  Protocol = "udp"
	TCP  Protocol = "tcp"
	TLS  Protocol = "tls"
	HTTP Protocol = "http"
)

// Config declares an input to start. Inputs are bootstrapped from configuration
// in this phase; runtime CRUD via the API arrives in a later phase.
type Config struct {
	ID       string   `mapstructure:"id"`
	Type     string   `mapstructure:"type"`
	Bind     string   `mapstructure:"bind"`
	Protocol Protocol `mapstructure:"protocol"`
	// TLSCertFile / TLSKeyFile enable TLS for tcp inputs when set.
	TLSCertFile string `mapstructure:"tls_cert_file"`
	TLSKeyFile  string `mapstructure:"tls_key_file"`
	// Token authenticates HTTP-JSON submissions when set.
	Token string `mapstructure:"token"`
}

// Validate checks a Config for the common required fields.
func (c Config) Validate() error {
	if c.ID == "" {
		return fmt.Errorf("input: id is required")
	}
	if c.Type == "" {
		return fmt.Errorf("input %q: type is required", c.ID)
	}
	if c.Bind == "" {
		return fmt.Errorf("input %q: bind address is required", c.ID)
	}
	return nil
}
