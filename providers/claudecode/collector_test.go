package claudecode

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	omnidevx "github.com/plexusone/omnidevx-core"
)

func TestSourceDescriptor(t *testing.T) {
	c, err := New(Options{Dir: "testdata"})
	if err != nil {
		t.Fatal(err)
	}
	want := omnidevx.Source{Provider: "anthropic", Product: "claude-code"}
	if got := c.Source(); got != want {
		t.Errorf("Source(): got %+v, want %+v", got, want)
	}
}

func TestCollectMissingProjectsDir(t *testing.T) {
	c, err := New(Options{Dir: t.TempDir()}) // exists, but has no projects/
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Collect(context.Background(), omnidevx.CollectRequest{}); err == nil {
		t.Fatal("expected error for missing projects directory, got nil")
	}
}

func TestCollectEmptyStore(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "projects"), 0o750); err != nil {
		t.Fatal(err)
	}
	c, err := New(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.Collect(context.Background(), omnidevx.CollectRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Events) != 0 || len(result.Diagnostics) != 0 {
		t.Errorf("empty store: got %d events, %d diagnostics; want 0/0",
			len(result.Events), len(result.Diagnostics))
	}
}

func TestCollectCanceledContext(t *testing.T) {
	c, err := New(Options{Dir: "testdata"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Collect(ctx, omnidevx.CollectRequest{}); err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
}
