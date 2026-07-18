package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	omnidevx "github.com/plexusone/omnidevx-core"
)

func event(id string, ts time.Time, product string) omnidevx.Event {
	return omnidevx.Event{
		ID:        id,
		Type:      omnidevx.EventPromptSubmitted,
		Timestamp: ts,
		Source:    omnidevx.Source{Provider: "test", Product: product},
		Provenance: omnidevx.Provenance{
			CollectionMode: omnidevx.ModeHistory,
			Confidence:     0.9,
		},
	}
}

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(Options{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestWriteReadRoundTrip(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	day1 := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 7, 2, 11, 0, 0, 0, time.UTC)

	events := []omnidevx.Event{
		event("a:1", day1, "claude-code"),
		event("a:2", day1.Add(time.Minute), "claude-code"),
		event("b:1", day2, "codex-cli"),
	}
	result, err := s.Write(ctx, events)
	if err != nil {
		t.Fatal(err)
	}
	if result.Written != 3 || result.Duplicates != 0 {
		t.Fatalf("write: got %+v, want 3/0", result)
	}

	// Files land in the documented layout.
	want := filepath.Join(s.Dir(), "events", "2026", "07", "01", "claude-code.jsonl")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected day file %s: %v", want, err)
	}

	read, err := s.Read(ctx, Query{})
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Events) != 3 || len(read.Diagnostics) != 0 {
		t.Fatalf("read: got %d events, %d diagnostics", len(read.Events), len(read.Diagnostics))
	}
	// Ordered by timestamp.
	if read.Events[0].ID != "a:1" || read.Events[2].ID != "b:1" {
		t.Errorf("order: got %s..%s", read.Events[0].ID, read.Events[2].ID)
	}
	// Round-trip fidelity.
	if read.Events[0].Provenance.Confidence != 0.9 || read.Events[0].Source.Product != "claude-code" {
		t.Errorf("round trip mismatch: %+v", read.Events[0])
	}
}

func TestWriteIsIdempotent(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	ts := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	batch := []omnidevx.Event{event("a:1", ts, "claude-code"), event("a:2", ts, "claude-code")}

	if _, err := s.Write(ctx, batch); err != nil {
		t.Fatal(err)
	}
	// Re-import the same batch plus one new event.
	second, err := s.Write(ctx, append(batch, event("a:3", ts, "claude-code")))
	if err != nil {
		t.Fatal(err)
	}
	if second.Written != 1 || second.Duplicates != 2 {
		t.Fatalf("second write: got %+v, want written=1 duplicates=2", second)
	}

	read, err := s.Read(ctx, Query{})
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Events) != 3 {
		t.Fatalf("after re-import: got %d events, want 3", len(read.Events))
	}
}

func TestWriteRejectsIncompleteEvents(t *testing.T) {
	s := open(t)
	bad := omnidevx.Event{ID: "x", Type: omnidevx.EventPromptSubmitted} // no timestamp
	if _, err := s.Write(context.Background(), []omnidevx.Event{bad}); err == nil {
		t.Fatal("expected error for event without timestamp")
	}
	noProduct := event("y", time.Now(), "")
	if _, err := s.Write(context.Background(), []omnidevx.Event{noProduct}); err == nil {
		t.Fatal("expected error for event without source product")
	}
}

func TestReadPeriodAndProductFilters(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	var events []omnidevx.Event
	for day := 0; day < 10; day++ {
		ts := base.AddDate(0, 0, day).Add(12 * time.Hour)
		events = append(events,
			event(time.Duration(day).String()+":claude", ts, "claude-code"),
			event(time.Duration(day).String()+":codex", ts, "codex-cli"))
	}
	if _, err := s.Write(ctx, events); err != nil {
		t.Fatal(err)
	}

	week := omnidevx.Period{Start: base.AddDate(0, 0, 2), End: base.AddDate(0, 0, 5)}
	read, err := s.Read(ctx, Query{Period: week})
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Events) != 6 { // 3 days x 2 products
		t.Errorf("period filter: got %d events, want 6", len(read.Events))
	}

	read, err = s.Read(ctx, Query{Period: week, Product: "codex-cli"})
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Events) != 3 {
		t.Errorf("product filter: got %d events, want 3", len(read.Events))
	}
	for _, e := range read.Events {
		if e.Source.Product != "codex-cli" {
			t.Errorf("wrong product leaked: %s", e.Source.Product)
		}
	}
}

func TestReadSurfacesDamagedLines(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	ts := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	if _, err := s.Write(ctx, []omnidevx.Event{event("a:1", ts, "claude-code")}); err != nil {
		t.Fatal(err)
	}

	// Simulate a torn write.
	path := filepath.Join(s.Dir(), "events", "2026", "07", "01", "claude-code.jsonl")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"id":"torn`); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	read, err := s.Read(ctx, Query{})
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Events) != 1 || len(read.Diagnostics) != 1 {
		t.Fatalf("torn line: got %d events, %d diagnostics; want 1/1", len(read.Events), len(read.Diagnostics))
	}

	// A subsequent write to the damaged file still dedups and appends.
	result, err := s.Write(ctx, []omnidevx.Event{event("a:1", ts, "claude-code"), event("a:2", ts, "claude-code")})
	if err != nil {
		t.Fatal(err)
	}
	if result.Written != 1 || result.Duplicates != 1 {
		t.Fatalf("write after damage: got %+v, want 1/1", result)
	}
}

func TestProductNameSanitized(t *testing.T) {
	s := open(t)
	e := event("weird:1", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), "Weird/../Product Name")
	if _, err := s.Write(context.Background(), []omnidevx.Event{e}); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(s.Dir(), "events", "2026", "07", "01", "weird----product-name.jsonl")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("sanitized file missing (%s): %v", want, err)
	}
}
