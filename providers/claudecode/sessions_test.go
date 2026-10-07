package claudecode

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/plexusone/omnidevx-core/sessions"
)

var t0 = time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)

func at(min int) string { return t0.Add(time.Duration(min) * time.Minute).Format(time.RFC3339Nano) }

// line renders one JSONL record from alternating key/value pairs.
func line(t *testing.T, kv ...any) string {
	t.Helper()
	m := map[string]any{}
	if len(kv)%2 != 0 {
		t.Fatalf("line: odd number of key/value arguments: %d", len(kv))
	}
	for i := 0; i+1 < len(kv); i += 2 {
		key, ok := kv[i].(string)
		if !ok {
			t.Fatalf("line: key %v is not a string", kv[i])
		}
		m[key] = kv[i+1]
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func userText(t *testing.T, ts, text string, extra ...any) string {
	t.Helper()
	kv := append([]any{"type", "user", "timestamp", ts, "cwd", "/Users/example/src/app",
		"message", map[string]any{"role": "user", "content": text}}, extra...)
	return line(t, kv...)
}

func toolResult(t *testing.T, ts string) string {
	t.Helper()
	return line(t, "type", "user", "timestamp", ts, "cwd", "/Users/example/src/app",
		"message", map[string]any{"role": "user", "content": []any{
			map[string]any{"type": "tool_result", "tool_use_id": "t1", "content": "ok"},
		}})
}

func assistant(t *testing.T, ts string) string {
	t.Helper()
	return line(t, "type", "assistant", "timestamp", ts, "cwd", "/Users/example/src/app",
		"message", map[string]any{"role": "assistant", "content": []any{
			map[string]any{"type": "text", "text": "working"},
		}})
}

const testProject = "-Users-example-src-app"

func writeSession(t *testing.T, root, id string, lines ...string) string {
	t.Helper()
	dir := filepath.Join(root, "projects", testProject)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, id+".jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func newReader(t *testing.T, root string) *SessionReader {
	t.Helper()
	r, err := NewSessionReader(Options{Dir: root})
	if err != nil {
		t.Fatal(err)
	}
	r.processStart = func(int) (time.Time, error) { return time.Time{}, os.ErrProcessDone }
	return r
}

func listOne(t *testing.T, r *SessionReader, opts sessions.ListOptions) sessions.Session {
	t.Helper()
	got, diags, err := r.List(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %+v", diags)
	}
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1: %+v", len(got), got)
	}
	return got[0]
}

func TestSessionReaderHumanVsAgentActivity(t *testing.T) {
	root := t.TempDir()
	writeSession(t, root, "11111111-aaaa-bbbb-cccc-000000000001",
		userText(t, at(0), "add a session catalog", "promptSource", "typed", "gitBranch", "feat/sessions"),
		assistant(t, at(1)),
		toolResult(t, at(2)),
		userText(t, at(10), "now resume them", "promptSource", "typed"),
		assistant(t, at(11)),
		toolResult(t, at(50)), // agent kept working for 40 minutes
		assistant(t, at(60)),
		line(t, "type", "ai-title", "aiTitle", "Session catalog"),
	)

	s := listOne(t, newReader(t, root), sessions.ListOptions{})

	if s.Harness != sessions.HarnessClaudeCode || s.ID != "11111111-aaaa-bbbb-cccc-000000000001" {
		t.Fatalf("identity = %s/%s", s.Harness, s.ID)
	}
	if s.CWD != "/Users/example/src/app" || s.GitBranch != "feat/sessions" {
		t.Fatalf("cwd/branch = %q/%q", s.CWD, s.GitBranch)
	}
	if !s.CreatedAt.Equal(t0) {
		t.Errorf("CreatedAt = %v", s.CreatedAt)
	}
	if want := t0.Add(60 * time.Minute); !s.LastActivityAt.Equal(want) {
		t.Errorf("LastActivityAt = %v, want %v", s.LastActivityAt, want)
	}
	if want := t0.Add(10 * time.Minute); !s.LastHumanActivityAt.Equal(want) {
		t.Errorf("LastHumanActivityAt = %v, want %v (tool results must not count)", s.LastHumanActivityAt, want)
	}
	if s.Title != "Session catalog" || s.TitleSource != sessions.TitleHarness {
		t.Errorf("title = %q (%s)", s.Title, s.TitleSource)
	}
	if len(s.RecentPrompts) != 2 || s.RecentPrompts[1].Text != "now resume them" {
		t.Errorf("RecentPrompts = %+v", s.RecentPrompts)
	}
	if s.State != sessions.StateResumable {
		t.Errorf("State = %s", s.State)
	}
	wantArgv := []string{"claude", "--resume", s.ID}
	if strings.Join(s.Resume.Argv, " ") != strings.Join(wantArgv, " ") || s.Resume.Dir != s.CWD {
		t.Errorf("Resume = %+v", s.Resume)
	}
}

func TestSessionReaderLegacyAndInjectedRecords(t *testing.T) {
	root := t.TempDir()
	writeSession(t, root, "22222222-aaaa-bbbb-cccc-000000000002",
		// Legacy human prompt: no promptSource.
		userText(t, at(0), "fix the flaky test"),
		// Injected / meta / system / sidechain messages are not human.
		userText(t, at(5), "<system-reminder>ignore</system-reminder>"),
		userText(t, at(6), "meta note", "isMeta", true),
		userText(t, at(7), "injected", "promptSource", "system"),
		userText(t, at(8), "subagent prompt", "isSidechain", true),
		assistant(t, at(9)),
	)

	s := listOne(t, newReader(t, root), sessions.ListOptions{})

	if want := t0; !s.LastHumanActivityAt.Equal(want) {
		t.Errorf("LastHumanActivityAt = %v, want %v", s.LastHumanActivityAt, want)
	}
	if s.Title != "fix the flaky test" || s.TitleSource != sessions.TitleFirstPrompt {
		t.Errorf("title = %q (%s), want first prompt fallback", s.Title, s.TitleSource)
	}
}

func TestSessionReaderNoContent(t *testing.T) {
	root := t.TempDir()
	writeSession(t, root, "33333333-aaaa-bbbb-cccc-000000000003",
		userText(t, at(0), "secret plan", "promptSource", "typed"),
		line(t, "type", "ai-title", "aiTitle", "Secret plan"),
	)

	s := listOne(t, newReader(t, root), sessions.ListOptions{NoContent: true})

	if s.Title != "app" || s.TitleSource != sessions.TitleCWD {
		t.Errorf("title = %q (%s), want cwd fallback", s.Title, s.TitleSource)
	}
	if len(s.RecentPrompts) != 0 {
		t.Errorf("RecentPrompts = %+v, want none", s.RecentPrompts)
	}
}

func TestSessionReaderTailGrowsToFindHumanPrompt(t *testing.T) {
	root := t.TempDir()
	lines := []string{userText(t, at(0), "kick off a long run", "promptSource", "typed")}
	pad := strings.Repeat("x", 1<<10)
	for i := 0; i < 1200; i++ { // ~1.2 MB of agent traffic after the last prompt
		lines = append(lines, line(t, "type", "assistant", "timestamp", at(1),
			"message", map[string]any{"role": "assistant", "content": pad}))
	}
	writeSession(t, root, "44444444-aaaa-bbbb-cccc-000000000004", lines...)

	s := listOne(t, newReader(t, root), sessions.ListOptions{})

	if !s.LastHumanActivityAt.Equal(t0) {
		t.Errorf("LastHumanActivityAt = %v, want %v (tail window must grow past agent output)", s.LastHumanActivityAt, t0)
	}
}

func TestSessionReaderSinceAndSkips(t *testing.T) {
	root := t.TempDir()
	writeSession(t, root, "55555555-aaaa-bbbb-cccc-000000000005",
		userText(t, at(0), "old work", "promptSource", "typed"))
	writeSession(t, root, "66666666-aaaa-bbbb-cccc-000000000006",
		line(t, "type", "file-history-snapshot")) // no timestamps: not a conversation
	// Subagent transcripts live in nested directories and are not sessions.
	nested := filepath.Join(root, "projects", testProject, "sub")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "agent.jsonl"), []byte(userText(t, at(0), "x")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	r := newReader(t, root)
	got, _, err := r.List(context.Background(), sessions.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "55555555-aaaa-bbbb-cccc-000000000005" {
		t.Fatalf("sessions = %+v", got)
	}
	got, _, err = r.List(context.Background(), sessions.ListOptions{Since: t0.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("Since filter kept %+v", got)
	}
}

func TestSessionReaderLiveState(t *testing.T) {
	root := t.TempDir()
	const live, stale, reused = "77777777-aaaa-bbbb-cccc-000000000007", "88888888-aaaa-bbbb-cccc-000000000008", "99999999-aaaa-bbbb-cccc-000000000009"
	for _, id := range []string{live, stale, reused} {
		writeSession(t, root, id, userText(t, at(0), "hello", "promptSource", "typed"))
	}
	writeLive := func(pid int, id string, started time.Time, name string) {
		t.Helper()
		dir := filepath.Join(root, "sessions")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		data := line(t, "pid", pid, "sessionId", id, "startedAt", started.UnixMilli(), "name", name)
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%d.json", pid)), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeLive(101, live, t0, "my-session")
	writeLive(102, stale, t0, "")                     // process is gone
	writeLive(103, reused, t0.Add(-48*time.Hour), "") // PID reused by a newer process

	r := newReader(t, root)
	r.processStart = func(pid int) (time.Time, error) {
		switch pid {
		case 101:
			return t0.Add(5 * time.Second), nil
		case 103:
			return t0.Add(24 * time.Hour), nil
		}
		return time.Time{}, os.ErrProcessDone
	}

	got, _, err := r.List(context.Background(), sessions.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	state := map[string]sessions.State{}
	for _, s := range got {
		state[s.ID] = s.State
		if s.ID == live {
			if s.Runtime == nil || s.Runtime.PID != 101 {
				t.Errorf("live Runtime = %+v", s.Runtime)
			}
			if s.Title != "my-session" {
				t.Errorf("live Title = %q, want registry name", s.Title)
			}
		}
	}
	if state[live] != sessions.StateRunning {
		t.Errorf("live = %s", state[live])
	}
	if state[stale] != sessions.StateResumable {
		t.Errorf("stale = %s, want resumable", state[stale])
	}
	if state[reused] != sessions.StateResumable {
		t.Errorf("reused PID = %s, want resumable", state[reused])
	}
}

func TestSessionReaderMissingStore(t *testing.T) {
	r := newReader(t, t.TempDir())
	if _, _, err := r.List(context.Background(), sessions.ListOptions{}); err == nil {
		t.Fatal("expected error for missing projects directory")
	}
}

func TestSessionReaderUnreadableFileBecomesDiagnostic(t *testing.T) {
	root := t.TempDir()
	path := writeSession(t, root, "aaaaaaaa-aaaa-bbbb-cccc-00000000000a",
		userText(t, at(0), "hi", "promptSource", "typed"))
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) }) //nolint:errcheck // best-effort cleanup

	got, diags, err := newReader(t, root).List(context.Background(), sessions.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 || len(diags) != 1 || diags[0].Path != path {
		t.Fatalf("sessions=%d diags=%+v, want 0 sessions and one diagnostic for %s", len(got), diags, path)
	}
}
