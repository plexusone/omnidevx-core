package git

import (
	"context"
	"fmt"
	"time"

	"github.com/grokify/gogit"
	omnidevx "github.com/plexusone/omnidevx-core"
)

// DefaultConfidence is the provenance confidence for events reconstructed
// from git history. Commits are authoritative records; AI attribution rests
// on co-author trailers, which understate AI involvement when tools do not
// sign their work.
const DefaultConfidence = 0.95

// DefaultMaxDepth is how many directory levels below each root are searched
// for repositories, matching a GOPATH-style host/org/repo layout.
const DefaultMaxDepth = 3

var source = omnidevx.Source{
	Provider: "git",
	Product:  "git",
}

// Options configures the collector.
type Options struct {
	// Roots are directories to search for repositories. A root may itself
	// be a repository. Required.
	Roots []string
	// MaxDepth bounds discovery below each root. Defaults to
	// DefaultMaxDepth.
	MaxDepth int
	// NoMerges excludes merge commits.
	NoMerges bool
}

// Collector emits devx.change.committed events from local git history.
type Collector struct {
	opts Options
}

var _ omnidevx.Collector = (*Collector)(nil)

// New returns a Collector for the given options.
func New(opts Options) (*Collector, error) {
	if len(opts.Roots) == 0 {
		return nil, fmt.Errorf("git: at least one root is required")
	}
	if opts.MaxDepth == 0 {
		opts.MaxDepth = DefaultMaxDepth
	}
	return &Collector{opts: opts}, nil
}

// Source implements omnidevx.Collector.
func (c *Collector) Source() omnidevx.Source { return source }

// Collect implements omnidevx.Collector. Repositories that fail to read
// become diagnostics; only discovery errors abort the run.
func (c *Collector) Collect(ctx context.Context, req omnidevx.CollectRequest) (*omnidevx.CollectionResult, error) {
	repos, err := gogit.Discover(c.opts.Roots, c.opts.MaxDepth)
	if err != nil {
		return nil, fmt.Errorf("git: %w", err)
	}

	result := &omnidevx.CollectionResult{
		Source:      source,
		Subject:     req.Subject,
		Period:      req.Period,
		Events:      []omnidevx.Event{},
		CollectedAt: time.Now().UTC(),
	}

	for _, repoPath := range repos {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		events, diags := c.collectRepo(ctx, repoPath, req)
		result.Events = append(result.Events, events...)
		result.Diagnostics = append(result.Diagnostics, diags...)
	}
	return result, nil
}

func (c *Collector) collectRepo(ctx context.Context, repoPath string, req omnidevx.CollectRequest) ([]omnidevx.Event, []omnidevx.Diagnostic) {
	repo, err := gogit.Open(repoPath)
	if err != nil {
		return nil, []omnidevx.Diagnostic{diag(repoPath, "open repository", err)}
	}

	commits, err := repo.Log(ctx, gogit.LogOptions{
		Since:        req.Period.Start,
		Until:        req.Period.End,
		NoMerges:     c.opts.NoMerges,
		IncludeStats: true,
	})
	if err != nil {
		return nil, []omnidevx.Diagnostic{diag(repoPath, "read log", err)}
	}
	if len(commits) == 0 {
		return nil, nil
	}

	var diags []omnidevx.Diagnostic
	branch, err := repo.Branch(ctx)
	if err != nil {
		diags = append(diags, diag(repoPath, "resolve branch", err))
	}
	origin, err := repo.OriginURL(ctx)
	if err != nil {
		diags = append(diags, diag(repoPath, "resolve origin", err))
	}

	evtCtx := omnidevx.EventContext{
		Repository: gogit.NormalizeRemoteURL(origin),
		Workspace:  repoPath,
		GitBranch:  branch,
	}

	events := make([]omnidevx.Event, 0, len(commits))
	for _, commit := range commits {
		// Period.Contains re-checks because git's --since/--until bounds
		// are inclusive while the canonical Period is half-open.
		if !req.Period.Contains(commit.CommitDate) {
			continue
		}
		events = append(events, commitEvent(commit, evtCtx, req.Subject))
	}
	return events, diags
}

// commitEvent maps one commit to a canonical event. Commit subjects and
// bodies are content and are never included.
func commitEvent(commit gogit.Commit, evtCtx omnidevx.EventContext, subject omnidevx.SubjectRef) omnidevx.Event {
	attrs := map[string]any{
		omnidevx.AttrCommitHash:   commit.Hash,
		omnidevx.AttrAuthorEmail:  commit.Author.Email,
		omnidevx.AttrInsertions:   commit.Insertions,
		omnidevx.AttrDeletions:    commit.Deletions,
		omnidevx.AttrFilesChanged: commit.FilesChanged,
	}

	var aiTools []string
	seen := map[string]bool{}
	for _, coAuthor := range commit.CoAuthors() {
		if tool := AIToolByEmail(coAuthor.Email); tool != "" && !seen[tool] {
			aiTools = append(aiTools, tool)
			seen[tool] = true
		}
	}
	attrs[omnidevx.AttrAIAssisted] = len(aiTools) > 0
	if len(aiTools) > 0 {
		attrs[omnidevx.AttrAITools] = aiTools
	}

	return omnidevx.Event{
		// Hash-keyed IDs deduplicate the same commit found in multiple
		// local clones.
		ID:         "git:commit:" + commit.Hash,
		Type:       omnidevx.EventChangeCommitted,
		Timestamp:  commit.CommitDate,
		Subject:    subject,
		Source:     source,
		Context:    evtCtx,
		Attributes: attrs,
		Provenance: omnidevx.Provenance{
			CollectionMode: omnidevx.ModeHistory,
			Confidence:     DefaultConfidence,
		},
	}
}

func diag(path, action string, err error) omnidevx.Diagnostic {
	return omnidevx.Diagnostic{
		Severity: omnidevx.SeverityWarning,
		Message:  fmt.Sprintf("%s: %v", action, err),
		Path:     path,
	}
}
