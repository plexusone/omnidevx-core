package claudecode

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	omnidevx "github.com/plexusone/omnidevx-core"
)

// DefaultConfidence is the provenance confidence for events reconstructed
// from local session history. Session files are complete transcripts, so
// confidence is high, but the format is internal and undocumented.
const DefaultConfidence = 0.9

var source = omnidevx.Source{
	Provider: "anthropic",
	Product:  "claude-code",
}

// Options configures the collector.
type Options struct {
	// Dir is the Claude Code home directory. Defaults to ~/.claude.
	Dir string
}

// Collector reads Claude Code local session history.
type Collector struct {
	dir string
}

var _ omnidevx.Collector = (*Collector)(nil)

// New returns a Collector for the given options.
func New(opts Options) (*Collector, error) {
	dir := opts.Dir
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("claudecode: resolve home directory: %w", err)
		}
		dir = filepath.Join(home, ".claude")
	}
	return &Collector{dir: dir}, nil
}

// Source implements omnidevx.Collector.
func (c *Collector) Source() omnidevx.Source { return source }

// Collect implements omnidevx.Collector. It scans every project's session
// files, emitting events within the requested period. Unparseable lines and
// files are reported as diagnostics, never silently dropped.
func (c *Collector) Collect(ctx context.Context, req omnidevx.CollectRequest) (*omnidevx.CollectionResult, error) {
	projectsDir := filepath.Join(c.dir, "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil, fmt.Errorf("claudecode: read projects directory: %w", err)
	}

	result := &omnidevx.CollectionResult{
		Source:      source,
		Subject:     req.Subject,
		Period:      req.Period,
		Events:      []omnidevx.Event{},
		CollectedAt: time.Now().UTC(),
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		projectDir := filepath.Join(projectsDir, entry.Name())
		files, err := filepath.Glob(filepath.Join(projectDir, "*.jsonl"))
		if err != nil {
			return nil, fmt.Errorf("claudecode: glob %s: %w", projectDir, err)
		}
		for _, file := range files {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			events, diags := parseSessionFile(file, req)
			result.Events = append(result.Events, events...)
			result.Diagnostics = append(result.Diagnostics, diags...)
		}
	}
	return result, nil
}
