package meta

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestSavedSearchCRUD(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "tollan.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	created, err := s.CreateSavedSearch(ctx, "Auth failures", "level:error AND password", "now-24h")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" || created.Name != "Auth failures" {
		t.Fatalf("bad created: %+v", created)
	}

	list, err := s.ListSavedSearches(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %v, %v", list, err)
	}

	updated, err := s.UpdateSavedSearch(ctx, created.ID, "Renamed", "source:web01", "now-1h")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "Renamed" || updated.Query != "source:web01" || updated.TimeRange != "now-1h" {
		t.Fatalf("bad update: %+v", updated)
	}

	if _, err := s.UpdateSavedSearch(ctx, "missing", "x", "y", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update missing = %v, want ErrNotFound", err)
	}

	if err := s.DeleteSavedSearch(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := s.DeleteSavedSearch(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("double delete = %v, want ErrNotFound", err)
	}

	list, _ = s.ListSavedSearches(ctx)
	if len(list) != 0 {
		t.Fatalf("list after delete = %v", list)
	}
}

func TestCreateRequiresName(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "tollan.db"))
	defer s.Close()
	if _, err := s.CreateSavedSearch(context.Background(), "", "q", ""); err == nil {
		t.Fatal("expected error for empty name")
	}
}
