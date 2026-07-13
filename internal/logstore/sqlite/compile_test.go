package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/t0mer/tollan/internal/logstore"
	"github.com/t0mer/tollan/internal/schema"
	"github.com/t0mer/tollan/internal/search/query"
)

func seedForQuery(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC)
	msgs := []*schema.Message{
		{ID: "1", Timestamp: base, Source: "web01", Body: "GET /login 200",
			Fields: map[string]any{"level": "info", "status": 200}},
		{ID: "2", Timestamp: base.Add(time.Minute), Source: "web01", Body: "failed password for alice",
			Fields: map[string]any{"level": "error", "status": 401, "user": "alice"}},
		{ID: "3", Timestamp: base.Add(2 * time.Minute), Source: "web02", Body: "GET /orders 500 upstream timeout",
			Fields: map[string]any{"level": "critical", "status": 500}},
		{ID: "4", Timestamp: base.Add(3 * time.Minute), Source: "db01", Body: "connection established",
			Fields: map[string]any{"level": "debug"}},
	}
	if err := s.Store(context.Background(), msgs); err != nil {
		t.Fatal(err)
	}
	return s
}

func runQuery(t *testing.T, s *Store, q string) []string {
	t.Helper()
	node, err := query.Parse(q)
	if err != nil {
		t.Fatalf("parse %q: %v", q, err)
	}
	res, err := s.Search(context.Background(), logstore.Query{Expr: node, Order: logstore.Ascending})
	if err != nil {
		t.Fatalf("search %q: %v", q, err)
	}
	out := make([]string, len(res.Messages))
	for i, m := range res.Messages {
		out[i] = m.ID
	}
	return out
}

func TestQuerySearch(t *testing.T) {
	s := seedForQuery(t)
	defer s.Close()

	cases := []struct {
		q    string
		want []string
	}{
		{"", []string{"1", "2", "3", "4"}},
		{"level:error", []string{"2"}},
		{"level:error OR level:critical", []string{"2", "3"}},
		{"source:web01", []string{"1", "2"}},
		{"source:web*", []string{"1", "2", "3"}},
		{"status:>=500", []string{"3"}},
		{"status:[400 TO 499]", []string{"2"}},
		{"status:>200", []string{"2", "3"}},
		{"_exists_:user", []string{"2"}},
		{"password", []string{"2"}},
		{"GET", []string{"1", "3"}},
		{"password AND source:web01", []string{"2"}},
		{"NOT level:debug", []string{"1", "2", "3"}},
		{"level:error OR source:web02", []string{"2", "3"}},
		{`user:alice`, []string{"2"}},
	}
	for _, c := range cases {
		t.Run(c.q, func(t *testing.T) {
			got := runQuery(t, s, c.q)
			if !equalIDs(got, c.want) {
				t.Errorf("query %q = %v, want %v", c.q, got, c.want)
			}
		})
	}
}

func TestQueryInvalidFieldNameRejected(t *testing.T) {
	s := seedForQuery(t)
	defer s.Close()
	// A field name with an illegal char must be rejected, not injected.
	_, err := s.Search(context.Background(), logstore.Query{
		Expr: &query.FieldEq{Field: "a'b", Value: "x"},
	})
	if err == nil {
		t.Fatal("expected error for invalid field name")
	}
}

func equalIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
