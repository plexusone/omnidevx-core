package sessions

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	omnidevx "github.com/plexusone/omnidevx-core"
)

type fakeReader struct {
	harness  Harness
	sessions []Session
	err      error
}

func (f fakeReader) Harness() Harness { return f.harness }

func (f fakeReader) List(context.Context, ListOptions) ([]Session, []omnidevx.Diagnostic, error) {
	return f.sessions, nil, f.err
}

func TestCatalogListSortsNewestFirst(t *testing.T) {
	base := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	c := NewCatalog(
		fakeReader{harness: HarnessClaudeCode, sessions: []Session{
			{Harness: HarnessClaudeCode, ID: "old1", LastActivityAt: base.Add(-2 * time.Hour)},
			{Harness: HarnessClaudeCode, ID: "new1", LastActivityAt: base},
		}},
		fakeReader{harness: HarnessCodex, sessions: []Session{
			{Harness: HarnessCodex, ID: "mid1", LastActivityAt: base.Add(-time.Hour)},
		}},
	)
	got, _, err := c.List(context.Background(), ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, s := range got {
		ids = append(ids, s.ID)
	}
	if strings.Join(ids, ",") != "new1,mid1,old1" {
		t.Fatalf("order = %v", ids)
	}
}

func TestCatalogListIsolatesReaderFailure(t *testing.T) {
	c := NewCatalog(
		fakeReader{harness: HarnessClaudeCode, err: errors.New("no store")},
		fakeReader{harness: HarnessCodex, sessions: []Session{{Harness: HarnessCodex, ID: "keep"}}},
	)
	got, _, err := c.List(context.Background(), ListOptions{})
	if err == nil || !strings.Contains(err.Error(), "claude-code") {
		t.Fatalf("expected error naming claude-code, got %v", err)
	}
	if len(got) != 1 || got[0].ID != "keep" {
		t.Fatalf("surviving sessions = %+v", got)
	}
}

func TestCatalogListCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := NewCatalog(fakeReader{harness: HarnessCodex}).List(ctx, ListOptions{}); err == nil {
		t.Fatal("expected context error")
	}
}

func TestResolve(t *testing.T) {
	all := []Session{
		{Harness: HarnessClaudeCode, ID: "a83f1111-aaaa"},
		{Harness: HarnessClaudeCode, ID: "a83f2222-bbbb"},
		{Harness: HarnessCodex, ID: "0199abcd-cccc"},
		{Harness: HarnessCodex, ID: "a83f3333-dddd"},
	}
	tests := []struct {
		name    string
		query   string
		wantID  string
		wantErr string
	}{
		{"unique prefix", "0199", "0199abcd-cccc", ""},
		{"exact id", "a83f1111-aaaa", "a83f1111-aaaa", ""},
		{"ambiguous", "a83f", "", "ambiguous"},
		{"harness qualifier disambiguates", "codex:a83f", "a83f3333-dddd", ""},
		{"too short", "a8", "", "too short"},
		{"no match", "ffff", "", "no session matches"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(all, tt.query)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.ID != tt.wantID {
				t.Fatalf("id = %s, want %s", got.ID, tt.wantID)
			}
		})
	}
}

func TestResumeSpecCommand(t *testing.T) {
	r := ResumeSpec{Argv: []string{"claude", "--resume", "abc"}, Dir: "/Users/example/my repo"}
	want := "cd '/Users/example/my repo' && claude --resume abc"
	if got := r.Command(); got != want {
		t.Fatalf("Command() = %q, want %q", got, want)
	}
}

func TestTruncate(t *testing.T) {
	if got := Truncate("  hello \n  world ", 50); got != "hello world" {
		t.Fatalf("got %q", got)
	}
	if got := Truncate("abcdefghij", 5); got != "abcd…" {
		t.Fatalf("got %q", got)
	}
}
