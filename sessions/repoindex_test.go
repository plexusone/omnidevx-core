package sessions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mkRepo creates <root>/<rel> as a repository whose origin is origin (no
// remote section when origin is empty) and returns its path.
func mkRepo(t *testing.T, root, rel, origin string) string {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "[core]\n\trepositoryformatversion = 0\n"
	if origin != "" {
		cfg += "[remote \"origin\"]\n\turl = " + origin + "\n\tfetch = +refs/heads/*:refs/remotes/origin/*\n"
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "config"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	mkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRepoIndexScansRootsAndDerivesIDs(t *testing.T) {
	root := t.TempDir()
	app := mkRepo(t, root, "github.com/acme/app", "git@github.com:acme/app.git")
	lib := mkRepo(t, root, "github.com/acme/lib", "https://github.com/acme/lib.git")
	noRemote := mkRepo(t, root, "scratch/play", "")
	// Things the scan must not report.
	mkRepo(t, root, "github.com/acme/app/vendor/dep", "https://github.com/other/dep.git") // inside a repo
	mkRepo(t, root, "node_modules/pkg", "https://github.com/other/pkg.git")
	mkRepo(t, root, ".hidden/secret", "https://github.com/other/secret.git")

	x, err := NewRepoIndex([]string{root}, RepoIndexOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, r := range x.Repos() {
		got[r.Root] = r.ID
	}
	want := map[string]string{app: "github.com/acme/app", lib: "github.com/acme/lib", noRemote: ""}
	if len(got) != len(want) {
		t.Fatalf("repos = %v, want exactly %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("ID of %s = %q, want %q", k, got[k], v)
		}
	}
}

func TestRepoIndexLookup(t *testing.T) {
	root := t.TempDir()
	app := mkRepo(t, root, "github.com/acme/app", "git@github.com:acme/app.git")
	mkdir(t, filepath.Join(root, "github.com/acme/app-other")) // shares a name prefix, not a repository
	x, err := NewRepoIndex([]string{root}, RepoIndexOptions{})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
		want string // repo root, or "" for none
	}{
		{"the root itself", app, app},
		{"a file deep inside", filepath.Join(app, "pkg", "x", "y.go"), app},
		{"a file that does not exist yet", filepath.Join(app, "new", "file.go"), app},
		{"trailing elements are cleaned", app + "/pkg/../main.go", app},
		{"a sibling sharing a name prefix", filepath.Join(root, "github.com/acme/app-other/x.go"), ""},
		{"a directory outside every repository", filepath.Join(root, "elsewhere", "x.go"), ""},
		{"a relative path", "pkg/x.go", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, ok := x.Lookup(tt.path)
			if tt.want == "" {
				if ok {
					t.Fatalf("Lookup(%q) = %+v, want no repository", tt.path, r)
				}
				return
			}
			if !ok || r.Root != tt.want {
				t.Fatalf("Lookup(%q) = %+v, %v; want root %s", tt.path, r, ok, tt.want)
			}
		})
	}
	if r, _ := x.Lookup(filepath.Join(app, "main.go")); r.ID != "github.com/acme/app" {
		t.Errorf("ID = %q", r.ID)
	}
}

func TestRepoIndexDiscoversRepositoriesOutsideTheRoots(t *testing.T) {
	root, other := t.TempDir(), t.TempDir()
	x, err := NewRepoIndex([]string{root}, RepoIndexOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(x.Repos()) != 0 {
		t.Fatal("expected an empty index")
	}
	outside := mkRepo(t, other, "proj", "https://example.com/team/proj")
	r, ok := x.Lookup(filepath.Join(outside, "src", "a.go"))
	if !ok || r.Root != outside || r.ID != "example.com/team/proj" {
		t.Fatalf("Lookup = %+v, %v", r, ok)
	}
	if len(x.Repos()) != 1 {
		t.Errorf("a discovered repository should be remembered: %v", x.Repos())
	}
}

func TestRepoIndexLinkedWorktreeAndSubmodule(t *testing.T) {
	root := t.TempDir()
	main := mkRepo(t, root, "main", "git@github.com:acme/app.git")

	// A linked worktree: .git is a file; remotes live in the common directory.
	wt := filepath.Join(root, "wt")
	adminDir := filepath.Join(main, ".git", "worktrees", "wt")
	writeFile(t, filepath.Join(adminDir, "commondir"), "../..\n")
	writeFile(t, filepath.Join(wt, ".git"), "gitdir: "+adminDir+"\n")

	// A submodule: .git is a file with a relative gitdir that has its own config.
	sub := filepath.Join(main, "third_party", "sub")
	writeFile(t, filepath.Join(main, ".git", "modules", "sub", "config"),
		"[remote \"origin\"]\n\turl = https://github.com/acme/sub.git\n")
	writeFile(t, filepath.Join(sub, ".git"), "gitdir: ../../.git/modules/sub\n")

	x, err := NewRepoIndex(nil, RepoIndexOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if r, ok := x.Lookup(filepath.Join(wt, "a.go")); !ok || r.Root != wt || r.ID != "github.com/acme/app" {
		t.Errorf("worktree: %+v, %v (want its own root and the main repo's ID)", r, ok)
	}
	if r, ok := x.Lookup(filepath.Join(sub, "b.go")); !ok || r.Root != sub || r.ID != "github.com/acme/sub" {
		t.Errorf("submodule: %+v, %v (want its own root and its own ID)", r, ok)
	}
}

func TestRepoIndexMissingRootDoesNotHideOthers(t *testing.T) {
	root := t.TempDir()
	app := mkRepo(t, root, "a/b/app", "https://github.com/acme/app")
	x, err := NewRepoIndex([]string{filepath.Join(root, "does-not-exist"), root}, RepoIndexOptions{})
	if err == nil || !strings.Contains(err.Error(), "does-not-exist") {
		t.Fatalf("err = %v, want it to name the missing root", err)
	}
	if x == nil {
		t.Fatal("the index must still be returned")
	}
	if _, ok := x.Lookup(filepath.Join(app, "x")); !ok {
		t.Error("the readable root should still be indexed")
	}
}

func TestRepoIndexMaxDepth(t *testing.T) {
	root := t.TempDir()
	mkRepo(t, root, "a/b/c/d/e/deep", "https://github.com/acme/deep")
	shallow, _ := NewRepoIndex([]string{root}, RepoIndexOptions{MaxDepth: 3})
	if n := len(shallow.Repos()); n != 0 {
		t.Errorf("depth 3 found %d repos, want 0", n)
	}
	deep, _ := NewRepoIndex([]string{root}, RepoIndexOptions{MaxDepth: 6})
	if n := len(deep.Repos()); n != 1 {
		t.Errorf("depth 6 found %d repos, want 1", n)
	}
}

func TestNormalizeRemote(t *testing.T) {
	tests := map[string]string{ //nolint:gosec // "user:token@" is a fabricated credential the test expects to be stripped
		"git@github.com:acme/app.git":              "github.com/acme/app",
		"git@github.com:acme/app":                  "github.com/acme/app",
		"https://github.com/acme/app.git":          "github.com/acme/app",
		"https://github.com/acme/app/":             "github.com/acme/app",
		"https://user:token@github.com/acme/app":   "github.com/acme/app",
		"ssh://git@github.com:22/acme/app.git":     "github.com/acme/app",
		"https://GitHub.com/Acme/App.git":          "github.com/Acme/App",
		"https://gitlab.example.com/group/sub/pkg": "gitlab.example.com/group/sub/pkg",
		"  https://github.com/acme/app  ":          "github.com/acme/app",
		"":                                         "",
		"/srv/git/app.git":                         "",
		"https://github.com":                       "",
		"::::":                                     "",
	}
	for in, want := range tests {
		if got := NormalizeRemote(in); got != want {
			t.Errorf("NormalizeRemote(%q) = %q, want %q", in, got, want)
		}
	}
}
