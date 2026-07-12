package journal

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func mustOpen(t *testing.T, opts Options) *Journal {
	t.Helper()
	j, err := Open(opts)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return j
}

func rec(i int) Record {
	return Record{
		InputID:    "in1",
		InputType:  "raw",
		Source:     "127.0.0.1",
		ReceivedAt: time.Unix(int64(1700000000+i), 0).UTC(),
		Payload:    []byte(fmt.Sprintf("message-%d", i)),
	}
}

func TestAppendAndReadInOrder(t *testing.T) {
	j := mustOpen(t, Options{Dir: t.TempDir()})
	defer j.Close()

	const n = 50
	for i := 0; i < n; i++ {
		if _, err := j.Append(rec(i)); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}
	if got := j.Depth(); got != n {
		t.Fatalf("Depth = %d, want %d", got, n)
	}

	r := j.NewReader()
	defer r.Close()
	ctx := context.Background()
	for i := 0; i < n; i++ {
		seq, got, err := r.Next(ctx)
		if err != nil {
			t.Fatalf("Next %d: %v", i, err)
		}
		if seq != uint64(i) {
			t.Fatalf("seq = %d, want %d", seq, i)
		}
		if string(got.Payload) != fmt.Sprintf("message-%d", i) {
			t.Fatalf("payload = %q, want message-%d", got.Payload, i)
		}
	}
}

func TestNextBlocksThenWakes(t *testing.T) {
	j := mustOpen(t, Options{Dir: t.TempDir()})
	defer j.Close()
	r := j.NewReader()
	defer r.Close()

	got := make(chan Record, 1)
	go func() {
		_, rc, err := r.Next(context.Background())
		if err == nil {
			got <- rc
		}
	}()

	select {
	case <-got:
		t.Fatal("Next returned before any append")
	case <-time.After(50 * time.Millisecond):
	}

	if _, err := j.Append(rec(1)); err != nil {
		t.Fatal(err)
	}
	select {
	case rc := <-got:
		if string(rc.Payload) != "message-1" {
			t.Fatalf("payload = %q", rc.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("Next did not wake on append")
	}
}

func TestContextCancelUnblocks(t *testing.T) {
	j := mustOpen(t, Options{Dir: t.TempDir()})
	defer j.Close()
	r := j.NewReader()
	defer r.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, _, err := r.Next(ctx); err == nil {
		t.Fatal("expected context error")
	}
}

func TestResumeAfterCommitAndReopen(t *testing.T) {
	dir := t.TempDir()
	j := mustOpen(t, Options{Dir: dir})

	const n = 10
	for i := 0; i < n; i++ {
		if _, err := j.Append(rec(i)); err != nil {
			t.Fatal(err)
		}
	}
	r := j.NewReader()
	ctx := context.Background()
	var last uint64
	for i := 0; i < 5; i++ {
		seq, _, err := r.Next(ctx)
		if err != nil {
			t.Fatal(err)
		}
		last = seq
	}
	if err := j.Commit(last); err != nil {
		t.Fatal(err)
	}
	r.Close()
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen: nextSeq continues, reader resumes at committed+1 (seq 5).
	j2 := mustOpen(t, Options{Dir: dir})
	defer j2.Close()
	if j2.ReadPos() != 5 {
		t.Fatalf("ReadPos = %d, want 5 (resume after committing seq 4)", j2.ReadPos())
	}
	r2 := j2.NewReader()
	defer r2.Close()
	seq, got, err := r2.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if seq != 5 {
		t.Fatalf("resumed seq = %d, want 5", seq)
	}
	if string(got.Payload) != "message-5" {
		t.Fatalf("payload = %q, want message-5", got.Payload)
	}

	// Appends after reopen keep the sequence monotonic.
	s, err := j2.Append(rec(99))
	if err != nil {
		t.Fatal(err)
	}
	if s != 10 {
		t.Fatalf("post-reopen append seq = %d, want 10", s)
	}
}

func TestRotationCreatesMultipleSegments(t *testing.T) {
	dir := t.TempDir()
	// Tiny segments force rotation every couple of records.
	j := mustOpen(t, Options{Dir: dir, MaxSegmentBytes: 120, MaxTotalBytes: 1 << 20})
	defer j.Close()
	for i := 0; i < 20; i++ {
		if _, err := j.Append(rec(i)); err != nil {
			t.Fatal(err)
		}
	}
	// Read all back across segment boundaries.
	r := j.NewReader()
	defer r.Close()
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		seq, _, err := r.Next(ctx)
		if err != nil {
			t.Fatalf("Next %d: %v", i, err)
		}
		if seq != uint64(i) {
			t.Fatalf("seq = %d, want %d", seq, i)
		}
	}
}

func TestEvictionDropsOldestAndReaderJumps(t *testing.T) {
	dir := t.TempDir()
	// Small total cap forces eviction of the oldest segments.
	j := mustOpen(t, Options{Dir: dir, MaxSegmentBytes: 120, MaxTotalBytes: 360})
	defer j.Close()

	for i := 0; i < 60; i++ {
		if _, err := j.Append(rec(i)); err != nil {
			t.Fatal(err)
		}
	}
	// Utilization should be within the cap.
	if u := j.Utilization(); u > 1.05 {
		t.Fatalf("utilization = %.2f, expected <= ~1.0", u)
	}

	// A fresh reader starts at the earliest surviving record, not seq 0.
	r := j.NewReader()
	defer r.Close()
	if r.Seq() == 0 {
		t.Fatalf("reader started at 0 despite eviction")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	seq, _, err := r.Next(ctx)
	if err != nil {
		t.Fatalf("Next after eviction: %v", err)
	}
	if seq != r.Seq()-1 || seq < 1 {
		t.Fatalf("unexpected first post-eviction seq %d", seq)
	}
}

func TestConcurrentAppend(t *testing.T) {
	j := mustOpen(t, Options{Dir: t.TempDir()})
	defer j.Close()

	const writers, each = 4, 25
	done := make(chan struct{})
	for w := 0; w < writers; w++ {
		go func() {
			for i := 0; i < each; i++ {
				if _, err := j.Append(rec(i)); err != nil {
					t.Errorf("append: %v", err)
				}
			}
			done <- struct{}{}
		}()
	}
	for w := 0; w < writers; w++ {
		<-done
	}
	if got := j.Depth(); got != writers*each {
		t.Fatalf("Depth = %d, want %d", got, writers*each)
	}
	// All sequences 0..N-1 present and unique.
	r := j.NewReader()
	defer r.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	seen := make(map[uint64]bool)
	for i := 0; i < writers*each; i++ {
		seq, _, err := r.Next(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if seen[seq] {
			t.Fatalf("duplicate seq %d", seq)
		}
		seen[seq] = true
	}
}
