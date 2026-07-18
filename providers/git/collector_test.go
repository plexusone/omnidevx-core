package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	omnidevx "github.com/plexusone/omnidevx-core"
)

// gitRun executes git in a fixture directory, isolated from the developer's
// global/system config (signing, hooks, templates).
func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	//nolint:gosec // G204: test helper; dir is t.TempDir(), args are literals
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_DATE=2026-07-01T10:00:00Z",
		"GIT_COMMITTER_DATE=2026-07-01T10:00:00Z",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

// fixtureRepo builds a repo with one AI-assisted and one plain commit.
func fixtureRepo(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "init", "-q", "-b", "main")
	gitRun(t, dir, "config", "user.name", "Test User")
	gitRun(t, dir, "config", "user.email", "test@example.com")
	gitRun(t, dir, "remote", "add", "origin", "https://github.com/example/"+name+".git")

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\ntwo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", "a.txt")
	gitRun(t, dir, "commit", "-q", "-m",
		"feat: ai assisted\n\nCo-Authored-By: Claude Fable 5 <noreply@anthropic.com>")

	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", "b.txt")
	gitRun(t, dir, "commit", "-q", "-m", "fix: human only")
	return dir
}

func TestCollectCommits(t *testing.T) {
	root := t.TempDir()
	fixtureRepo(t, root, "repo1")
	fixtureRepo(t, root, "repo2")

	c, err := New(Options{Roots: []string{root}, MaxDepth: 1})
	if err != nil {
		t.Fatal(err)
	}
	subject := omnidevx.SubjectRef{PersonID: "person:test"}
	result, err := c.Collect(context.Background(), omnidevx.CollectRequest{Subject: subject})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics: %+v", result.Diagnostics)
	}
	if len(result.Events) != 4 { // 2 repos x 2 commits
		t.Fatalf("events: got %d, want 4", len(result.Events))
	}

	aiCount := 0
	for _, e := range result.Events {
		if e.Type != omnidevx.EventChangeCommitted {
			t.Errorf("type: got %s", e.Type)
		}
		if e.Subject != subject {
			t.Errorf("subject not stamped on %s", e.ID)
		}
		if e.Context.Repository == "" || e.Context.GitBranch != "main" {
			t.Errorf("context incomplete: %+v", e.Context)
		}
		if e.Attributes[omnidevx.AttrAuthorEmail] != "test@example.com" {
			t.Errorf("author email: got %v", e.Attributes[omnidevx.AttrAuthorEmail])
		}
		if assisted, _ := e.Attributes[omnidevx.AttrAIAssisted].(bool); assisted {
			aiCount++
			tools, _ := e.Attributes[omnidevx.AttrAITools].([]string)
			if len(tools) != 1 || tools[0] != "Claude Code" {
				t.Errorf("ai tools: got %v", tools)
			}
		}
		// Privacy: commit subjects must not leak into attributes.
		for key, v := range e.Attributes {
			if s, ok := v.(string); ok && (s == "feat: ai assisted" || s == "fix: human only") {
				t.Errorf("subject leaked via attribute %q", key)
			}
		}
	}
	if aiCount != 2 { // one AI-assisted commit per repo
		t.Errorf("ai-assisted commits: got %d, want 2", aiCount)
	}
}

func TestCollectPeriodFilter(t *testing.T) {
	root := t.TempDir()
	fixtureRepo(t, root, "repo1")

	c, err := New(Options{Roots: []string{root}, MaxDepth: 1})
	if err != nil {
		t.Fatal(err)
	}
	// Fixture commits are at 2026-07-01T10:00:00Z; window excludes them.
	result, err := c.Collect(context.Background(), omnidevx.CollectRequest{
		Period: omnidevx.Period{
			Start: time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Events) != 0 {
		t.Errorf("out-of-window: got %d events, want 0", len(result.Events))
	}
}

func TestCloneDedupByHash(t *testing.T) {
	// The same commit reached via two clones must produce identical IDs.
	root := t.TempDir()
	repo := fixtureRepo(t, root, "orig")
	gitRun(t, root, "clone", "-q", repo, filepath.Join(root, "clone"))

	c, err := New(Options{Roots: []string{root}, MaxDepth: 1})
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.Collect(context.Background(), omnidevx.CollectRequest{})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]int{}
	for _, e := range result.Events {
		ids[e.ID]++
	}
	if len(ids) != 2 { // 2 unique commits, each seen twice
		t.Errorf("unique ids: got %d, want 2 (%v)", len(ids), ids)
	}
	for id, n := range ids {
		if n != 2 {
			t.Errorf("id %s seen %d times, want 2 (store dedups)", id, n)
		}
	}
}

func TestNewRequiresRoots(t *testing.T) {
	if _, err := New(Options{}); err == nil {
		t.Fatal("expected error for missing roots")
	}
}

func TestAIToolByEmail(t *testing.T) {
	cases := map[string]string{
		"noreply@anthropic.com":                      "Claude Code",
		"NoReply@Anthropic.com":                      "Claude Code",
		"999999+gemini-cli@users.noreply.github.com": "Gemini CLI",
		"copilot@github.com":                         "GitHub Copilot",
		"aider@aider.chat":                           "Aider",
		"human@example.com":                          "",
		"gemini-cli@elsewhere.com":                   "",
	}
	for email, want := range cases {
		if got := AIToolByEmail(email); got != want {
			t.Errorf("AIToolByEmail(%q): got %q, want %q", email, got, want)
		}
	}
}
