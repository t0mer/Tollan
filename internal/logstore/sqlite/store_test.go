package sqlite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/t0mer/tollan/internal/logstore"
	"github.com/t0mer/tollan/internal/schema"
)

func msg(id string, ts time.Time, body, stream string) *schema.Message {
	return &schema.Message{
		ID:         id,
		Timestamp:  ts,
		ReceivedAt: ts,
		Source:     "host1",
		Body:       body,
		Stream:     stream,
		Fields:     map[string]any{"level": "info"},
	}
}

func TestStoreAndSearchBasic(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	base := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	in := []*schema.Message{
		msg("a", base, "user alice logged in", "default"),
		msg("b", base.Add(time.Minute), "user bob failed password", "default"),
		msg("c", base.Add(2*time.Minute), "connection from 10.0.0.5", "net"),
	}
	if err := s.Store(ctx, in); err != nil {
		t.Fatalf("Store: %v", err)
	}

	// Full range, newest first.
	res, err := s.Search(ctx, logstore.Query{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res.Total != 3 {
		t.Fatalf("Total = %d, want 3", res.Total)
	}
	if len(res.Messages) != 3 || res.Messages[0].ID != "c" {
		t.Fatalf("expected newest-first, got %v", ids(res.Messages))
	}

	// FTS term.
	res, err = s.Search(ctx, logstore.Query{Text: "password"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Messages) != 1 || res.Messages[0].ID != "b" {
		t.Fatalf("FTS password: got %v", ids(res.Messages))
	}

	// Stream filter.
	res, err = s.Search(ctx, logstore.Query{Stream: "net"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Messages) != 1 || res.Messages[0].ID != "c" {
		t.Fatalf("stream net: got %v", ids(res.Messages))
	}

	// Fields round-trip.
	if lvl, _ := res.Messages[0].GetField("level"); lvl != "info" {
		t.Fatalf("level field = %v, want info", lvl)
	}
}

func TestSearchAscendingAndPaging(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()
	ctx := context.Background()
	base := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	var in []*schema.Message
	for i := 0; i < 10; i++ {
		in = append(in, msg(fmt.Sprintf("m%02d", i), base.Add(time.Duration(i)*time.Minute), "line", "default"))
	}
	if err := s.Store(ctx, in); err != nil {
		t.Fatal(err)
	}

	res, err := s.Search(ctx, logstore.Query{Order: logstore.Ascending, Limit: 3, Offset: 2})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 10 {
		t.Fatalf("Total = %d, want 10", res.Total)
	}
	if got := ids(res.Messages); len(got) != 3 || got[0] != "m02" || got[2] != "m04" {
		t.Fatalf("asc page = %v, want m02..m04", got)
	}
}

func TestCrossDayFanout(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()
	ctx := context.Background()

	d1 := time.Date(2026, 7, 11, 23, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 7, 12, 1, 0, 0, 0, time.UTC)
	if err := s.Store(ctx, []*schema.Message{
		msg("x", d1, "yesterday event", "default"),
		msg("y", d2, "today event", "default"),
	}); err != nil {
		t.Fatal(err)
	}

	days, _ := s.Days(ctx)
	if len(days) != 2 {
		t.Fatalf("days = %v, want 2 partitions", days)
	}

	res, err := s.Search(ctx, logstore.Query{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 2 || len(res.Messages) != 2 {
		t.Fatalf("cross-day total = %d", res.Total)
	}
	// Newest first: today before yesterday.
	if res.Messages[0].ID != "y" {
		t.Fatalf("order = %v, want y first", ids(res.Messages))
	}

	// Range restricted to yesterday only.
	res, err = s.Search(ctx, logstore.Query{
		From: time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 7, 11, 23, 59, 59, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Messages) != 1 || res.Messages[0].ID != "x" {
		t.Fatalf("range = %v, want only x", ids(res.Messages))
	}
}

func TestDropBefore(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()
	ctx := context.Background()
	old := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	recent := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	if err := s.Store(ctx, []*schema.Message{
		msg("old", old, "old", "default"),
		msg("new", recent, "new", "default"),
	}); err != nil {
		t.Fatal(err)
	}

	dropped, err := s.DropBefore(ctx, time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if dropped != 1 {
		t.Fatalf("dropped = %d, want 1", dropped)
	}
	days, _ := s.Days(ctx)
	if len(days) != 1 || days[0] != "2026-07-12" {
		t.Fatalf("remaining days = %v", days)
	}
}

func TestStoreDeduplicates(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()
	ctx := context.Background()
	ts := time.Date(2026, 7, 12, 5, 0, 0, 0, time.UTC)
	m := msg("dup", ts, "only once", "default")
	if err := s.Store(ctx, []*schema.Message{m}); err != nil {
		t.Fatal(err)
	}
	if err := s.Store(ctx, []*schema.Message{m}); err != nil {
		t.Fatal(err)
	}
	res, _ := s.Search(ctx, logstore.Query{})
	if res.Total != 1 {
		t.Fatalf("Total = %d, want 1 (dedup by id)", res.Total)
	}
}

func ids(msgs []*schema.Message) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		out[i] = m.ID
	}
	return out
}
