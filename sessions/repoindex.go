package sessions

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// DefaultRepoScanDepth is how many directory levels below a workspace root
// the index searches for repositories. It is enough for the common
// <root>/<host>/<org>/<repo> layout.
const DefaultRepoScanDepth = 5

// Repo is a git repository known to a RepoIndex.
type Repo struct {
	// Root is the repository's root directory, the one that holds .git.
	Root string `json:"root"`
	// ID identifies the repository, such as github.com/org/repo, derived from its origin
	// remote. It is empty when the repository has no usable remote.
	ID string `json:"id,omitempty"`
}

// RepoIndexOptions tunes NewRepoIndex.
type RepoIndexOptions struct {
	// MaxDepth is how many levels below each root to search. Zero means DefaultRepoScanDepth.
	MaxDepth int
}

// RepoIndex maps filesystem paths to the git repository that holds them. It
// is safe for concurrent use.
type RepoIndex struct {
	mu    sync.RWMutex
	repos map[string]Repo // by root directory
	miss  map[string]bool // directories known to be outside any repository
}

// skipDirs are directory names the scan never enters. Hidden directories
// are skipped too.
var skipDirs = map[string]bool{"node_modules": true, "vendor": true}

// NewRepoIndex scans each root for repositories. A root that cannot be read
// is skipped and reported in the returned error, which may accompany a
// usable index, so one stale entry in a configuration does not hide every
// other repository.
func NewRepoIndex(roots []string, opts RepoIndexOptions) (*RepoIndex, error) {
	depth := opts.MaxDepth
	if depth <= 0 {
		depth = DefaultRepoScanDepth
	}
	x := &RepoIndex{repos: map[string]Repo{}, miss: map[string]bool{}}
	var errs []error
	for _, root := range roots {
		root = filepath.Clean(root)
		if _, err := os.Stat(root); err != nil {
			errs = append(errs, fmt.Errorf("workspace root %s: %w", root, err))
			continue
		}
		x.scan(root, 0, depth)
	}
	return x, errors.Join(errs...)
}

// scan records repo roots under dir. A directory that holds .git is a repo
// and is not searched further, so nested checkouts such as vendored copies
// are not mistaken for separate repositories.
func (x *RepoIndex) scan(dir string, level, maxDepth int) {
	if hasGit(dir) {
		x.add(dir)
		return
	}
	if level >= maxDepth {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() || skipDirs[name] || strings.HasPrefix(name, ".") {
			continue // files, symlinks (IsDir is false for them), and skipped names
		}
		x.scan(filepath.Join(dir, name), level+1, maxDepth)
	}
}

func hasGit(dir string) bool {
	_, err := os.Lstat(filepath.Join(dir, ".git"))
	return err == nil
}

func (x *RepoIndex) add(root string) Repo {
	r := Repo{Root: root, ID: NormalizeRemote(readOriginURL(root))}
	x.mu.Lock()
	x.repos[root] = r
	x.mu.Unlock()
	return r
}

// Lookup returns the repository that holds path, which may be a file or a
// directory and need not exist. Known repositories are matched by their
// deepest root. A path outside every known repository is searched upward for
// a .git entry, and a repository found that way is remembered. Relative
// paths are not resolved, because the right base is the caller's.
func (x *RepoIndex) Lookup(path string) (Repo, bool) {
	if !filepath.IsAbs(path) {
		return Repo{}, false
	}
	path = filepath.Clean(path)

	x.mu.RLock()
	for p := path; ; p = filepath.Dir(p) {
		if r, ok := x.repos[p]; ok {
			x.mu.RUnlock()
			return r, true
		}
		if p == filepath.Dir(p) {
			break
		}
	}
	missed := x.miss[path]
	x.mu.RUnlock()
	if missed {
		return Repo{}, false
	}

	for p := path; ; p = filepath.Dir(p) {
		if hasGit(p) {
			return x.add(p), true
		}
		if p == filepath.Dir(p) {
			break
		}
	}
	x.mu.Lock()
	x.miss[path] = true
	x.mu.Unlock()
	return Repo{}, false
}

// Repos returns every known repository sorted by root.
func (x *RepoIndex) Repos() []Repo {
	x.mu.RLock()
	defer x.mu.RUnlock()
	out := make([]Repo, 0, len(x.repos))
	for _, r := range x.repos {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Root < out[j].Root })
	return out
}

// readOriginURL returns the url of the origin remote from the repository's
// git config, or "" when there is none. It reads the file directly rather
// than running git, so indexing hundreds of repositories stays fast and
// needs no git binary.
func readOriginURL(root string) string {
	cfg := gitConfigPath(root)
	if cfg == "" {
		return ""
	}
	f, err := os.Open(cfg) //nolint:gosec // path derived from a repository's own .git
	if err != nil {
		return ""
	}
	defer f.Close() //nolint:errcheck // read-only file

	inOrigin := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			inOrigin = line == `[remote "origin"]`
			continue
		}
		if !inOrigin {
			continue
		}
		if key, val, ok := strings.Cut(line, "="); ok && strings.TrimSpace(key) == "url" {
			return strings.TrimSpace(val)
		}
	}
	return ""
}

// gitConfigPath finds the config file that holds a repository's remotes.
// For an ordinary repository .git is a directory. For a linked worktree or
// a submodule .git is a file naming the real git directory, and a worktree's
// remotes live in the common directory its commondir file points at.
func gitConfigPath(root string) string {
	dotGit := filepath.Join(root, ".git")
	info, err := os.Lstat(dotGit)
	if err != nil {
		return ""
	}
	if info.IsDir() {
		return filepath.Join(dotGit, "config")
	}
	data, err := os.ReadFile(dotGit) //nolint:gosec // .git of a repository being indexed
	if err != nil {
		return ""
	}
	gitdir, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:")
	if !ok {
		return ""
	}
	gitdir = strings.TrimSpace(gitdir)
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(root, gitdir)
	}
	if common, err := os.ReadFile(filepath.Join(gitdir, "commondir")); err == nil { //nolint:gosec // inside the git directory
		c := strings.TrimSpace(string(common))
		if !filepath.IsAbs(c) {
			c = filepath.Join(gitdir, c)
		}
		return filepath.Join(filepath.Clean(c), "config")
	} else if !errors.Is(err, fs.ErrNotExist) {
		return ""
	}
	return filepath.Join(gitdir, "config")
}

var scpRemote = regexp.MustCompile(`^(?:[^@/\s]+@)?([^:/\s]+):(.+)$`)

// NormalizeRemote turns a git remote URL into a stable repository ID of the
// form host/path, such as github.com/org/repo. It accepts https, ssh, and
// scp-style remotes, drops credentials, ports, and a trailing .git, and
// lower-cases the host. It returns "" for an empty remote or a remote that
// is a local path, which does not identify a repository to anyone else.
func NormalizeRemote(remote string) string {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return ""
	}
	var host, path string
	if strings.Contains(remote, "://") {
		u, err := url.Parse(remote)
		if err != nil || u.Hostname() == "" {
			return ""
		}
		host, path = u.Hostname(), u.Path
	} else if m := scpRemote.FindStringSubmatch(remote); m != nil {
		host, path = m[1], m[2]
	} else {
		return "" // a local path
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	if host == "" || path == "" {
		return ""
	}
	return strings.ToLower(host) + "/" + path
}
