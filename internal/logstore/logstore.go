// Package logstore defines the interface Tollan uses to persist and search log
// messages, decoupled from any particular backend. The default implementation
// (internal/logstore/sqlite) stores one SQLite database per UTC day with an
// FTS5 index; a future backend (bleve, ClickHouse) can drop in behind this
// interface.
package logstore

import (
	"context"
	"time"

	"github.com/t0mer/tollan/internal/schema"
	"github.com/t0mer/tollan/internal/search/query"
)

// Direction is the sort order of search results by event timestamp.
type Direction string

const (
	// Descending returns newest messages first (the default).
	Descending Direction = "desc"
	// Ascending returns oldest messages first.
	Ascending Direction = "asc"
)

// Query describes a search over a time range. Phase 2 supports a full-text term
// and a stream filter; the structured query language compiles into the richer
// fields added in a later phase.
type Query struct {
	From time.Time
	To   time.Time
	// Expr is the parsed query AST. When nil, Text (if any) is used as a simple
	// full-text term; when both are empty, all messages in range match.
	Expr query.Node
	// Text is a plain full-text match over the message body. Ignored when Expr
	// is set. Convenient for internal callers and tests.
	Text string
	// Stream, if set, restricts results to that stream id.
	Stream string
	// Limit caps returned messages (0 → a sensible default applied by the store).
	Limit int
	// Offset skips the first N matching messages.
	Offset int
	// Order controls sort direction (default Descending).
	Order Direction
}

// Result is a page of search results.
type Result struct {
	// Messages is the page, ordered per Query.Order.
	Messages []*schema.Message
	// Total is the number of messages matching the query across the range.
	Total int
}

// Store persists and searches log messages.
type Store interface {
	// Store persists a batch of messages. Messages are partitioned by the UTC
	// day of their event timestamp.
	Store(ctx context.Context, msgs []*schema.Message) error
	// Search returns messages matching the query.
	Search(ctx context.Context, q Query) (Result, error)
	// Days returns the UTC day identifiers (YYYY-MM-DD) that have partitions,
	// oldest first.
	Days(ctx context.Context) ([]string, error)
	// DaySizes returns the on-disk byte size of each day partition.
	DaySizes(ctx context.Context) (map[string]int64, error)
	// DropBefore deletes whole day partitions strictly older than the cutoff
	// day (UTC) and returns the number of partitions removed.
	DropBefore(ctx context.Context, cutoff time.Time) (int, error)
	// Close releases all open partition handles.
	Close() error
}
