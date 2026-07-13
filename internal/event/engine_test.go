package event

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/t0mer/tollan/internal/logstore/sqlite"
	"github.com/t0mer/tollan/internal/meta"
	"github.com/t0mer/tollan/internal/notify"
	"github.com/t0mer/tollan/internal/schema"
	"github.com/t0mer/tollan/internal/testutil"
)

func TestFilterEventFires(t *testing.T) {
	dir := t.TempDir()
	store, err := sqlite.Open(filepath.Join(dir, "logs"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	m, err := meta.Open(filepath.Join(dir, "tollan.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	ctx := context.Background()

	// Store 3 error messages in the window.
	now := time.Now().UTC()
	var msgs []*schema.Message
	for i := 0; i < 3; i++ {
		msgs = append(msgs, &schema.Message{
			ID: testutil.ID(i), Timestamp: now.Add(-time.Duration(i) * time.Second),
			Source: "web01", Body: "failed login", Stream: "default",
			Fields: map[string]any{"level": "error"},
		})
	}
	if err := store.Store(ctx, msgs); err != nil {
		t.Fatal(err)
	}

	def := Definition{
		Name: "Login failures", Enabled: true, Type: TriggerFilter,
		Query: "level:error", WindowSeconds: 60, Threshold: 2, Backlog: 3,
	}
	data, _ := json.Marshal(def)
	if _, err := m.PutEntity(ctx, meta.KindEvent, "e1", def.Name, data); err != nil {
		t.Fatal(err)
	}

	eng := New(store, m, notify.New(), nil, nil, testutil.Logger())
	if err := eng.EvaluateAll(ctx); err != nil {
		t.Fatal(err)
	}

	events, err := m.ListEvents(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if events[0].Count != 3 {
		t.Errorf("count = %d, want 3", events[0].Count)
	}

	// Below threshold: does not fire.
	def2 := def
	def2.Threshold = 10
	data2, _ := json.Marshal(def2)
	_, _ = m.PutEntity(ctx, meta.KindEvent, "e1", def2.Name, data2)
	// reset throttle by using a fresh engine
	eng2 := New(store, m, notify.New(), nil, nil, testutil.Logger())
	_ = eng2.EvaluateAll(ctx)
	events, _ = m.ListEvents(ctx, 10)
	if len(events) != 1 {
		t.Fatalf("events = %d, want still 1 (below threshold)", len(events))
	}
}
