// Package store persists canonical OmniDevX events as daily JSONL files:
//
//	<dir>/events/YYYY/MM/DD/<product>.jsonl
//
// Plain text keeps the store inspectable — a developer can read exactly what
// has been recorded about them — and reprocessable when metric formulas
// change. Writes are idempotent: an event whose deterministic ID already
// exists in its day file is skipped, so re-importing overlapping history
// does not duplicate.
//
// The default root is ~/.plexusone/omnidevx/data. Files are created with
// owner-only permissions.
package store

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	omnidevx "github.com/plexusone/omnidevx-core"
)

// Options configures a Store.
type Options struct {
	// Dir is the store root. Defaults to ~/.plexusone/omnidevx/data.
	Dir string
}

// Store reads and writes canonical events under a root directory.
type Store struct {
	dir string
}

// Open returns a Store rooted at opts.Dir, creating the directory if needed.
func Open(opts Options) (*Store, error) {
	dir := opts.Dir
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("store: resolve home directory: %w", err)
		}
		dir = filepath.Join(home, ".plexusone", "omnidevx", "data")
	}
	if err := os.MkdirAll(filepath.Join(dir, "events"), 0o700); err != nil {
		return nil, fmt.Errorf("store: create %s: %w", dir, err)
	}
	return &Store{dir: dir}, nil
}

// Dir returns the store root.
func (s *Store) Dir() string { return s.dir }

// WriteResult reports the outcome of a Write.
type WriteResult struct {
	Written    int `json:"written"`
	Duplicates int `json:"duplicates"`
}

// Write persists events, grouping them into day files by timestamp and
// product. Events whose ID already exists in their day file are counted as
// duplicates and skipped. Events without an ID, type, or timestamp are
// rejected.
func (s *Store) Write(ctx context.Context, events []omnidevx.Event) (*WriteResult, error) {
	byFile := map[string][]omnidevx.Event{}
	for _, e := range events {
		if e.ID == "" || e.Type == "" || e.Timestamp.IsZero() {
			return nil, fmt.Errorf("store: event missing id, type, or timestamp: %+v", e)
		}
		path, err := s.eventFile(e)
		if err != nil {
			return nil, err
		}
		byFile[path] = append(byFile[path], e)
	}

	// Deterministic file order keeps failures reproducible.
	paths := make([]string, 0, len(byFile))
	for path := range byFile {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	result := &WriteResult{}
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		written, dupes, err := appendEvents(path, byFile[path])
		if err != nil {
			return result, err
		}
		result.Written += written
		result.Duplicates += dupes
	}
	return result, nil
}

// eventFile maps an event to its day file. The product name is sanitized so
// it is always a single path element.
func (s *Store) eventFile(e omnidevx.Event) (string, error) {
	product := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		default:
			return '-'
		}
	}, e.Source.Product)
	if product == "" {
		return "", fmt.Errorf("store: event %s has no source product", e.ID)
	}
	day := e.Timestamp.UTC()
	return filepath.Join(s.dir, "events", day.Format("2006"), day.Format("01"), day.Format("02"), product+".jsonl"), nil
}

// appendEvents appends events not already present in the file (by ID).
func appendEvents(path string, events []omnidevx.Event) (written, duplicates int, err error) {
	existing, err := existingIDs(path)
	if err != nil {
		return 0, 0, err
	}

	var buf []byte
	for _, e := range events {
		if existing[e.ID] {
			duplicates++
			continue
		}
		line, err := json.Marshal(e)
		if err != nil {
			return 0, 0, fmt.Errorf("store: marshal event %s: %w", e.ID, err)
		}
		buf = append(buf, line...)
		buf = append(buf, '\n')
		existing[e.ID] = true
		written++
	}
	if len(buf) == 0 {
		return 0, duplicates, nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return 0, 0, fmt.Errorf("store: create day directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return 0, 0, fmt.Errorf("store: open %s: %w", path, err)
	}
	if _, err := f.Write(buf); err != nil {
		_ = f.Close() // the write error is the one worth reporting
		return 0, 0, fmt.Errorf("store: append %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return 0, 0, fmt.Errorf("store: close %s: %w", path, err)
	}
	return written, duplicates, nil
}

// existingIDs reads the IDs already present in a day file. A missing file is
// an empty set. Unparseable lines (e.g. a torn final line from a crashed
// write) are ignored here; Read surfaces them as diagnostics.
func existingIDs(path string) (map[string]bool, error) {
	ids := map[string]bool{}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return ids, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}
	defer f.Close() //nolint:errcheck // read-only file

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		var idOnly struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(scanner.Bytes(), &idOnly) == nil && idOnly.ID != "" {
			ids[idOnly.ID] = true
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("store: scan %s: %w", path, err)
	}
	return ids, nil
}

// Query scopes a Read. A zero Query reads everything.
type Query struct {
	// Period filters by event timestamp (half-open, zero bounds open).
	Period omnidevx.Period
	// Product, when set, reads only that source product's files.
	Product string
}

// ReadResult carries events plus diagnostics for damaged lines.
type ReadResult struct {
	Events      []omnidevx.Event
	Diagnostics []omnidevx.Diagnostic
}

// Read returns stored events matching the query, ordered by timestamp.
// Damaged lines become diagnostics, never silent drops.
func (s *Store) Read(ctx context.Context, q Query) (*ReadResult, error) {
	result := &ReadResult{}
	eventsRoot := filepath.Join(s.dir, "events")

	err := filepath.WalkDir(eventsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		if d.IsDir() {
			if skip, serr := skipDay(eventsRoot, path, q.Period); serr != nil {
				return serr
			} else if skip {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		if q.Product != "" && strings.TrimSuffix(filepath.Base(path), ".jsonl") != q.Product {
			return nil
		}
		return readFile(path, q, result)
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(result.Events, func(i, j int) bool {
		return result.Events[i].Timestamp.Before(result.Events[j].Timestamp)
	})
	return result, nil
}

// skipDay reports whether a fully-formed YYYY/MM/DD directory lies entirely
// outside the query period. Partial paths (year, month) are never skipped.
func skipDay(root, path string, p omnidevx.Period) (bool, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false, err
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) != 3 {
		return false, nil
	}
	day, err := time.Parse("2006/01/02", strings.Join(parts, "/"))
	if err != nil {
		return false, nil // unrecognized layout: read rather than skip
	}
	if !p.Start.IsZero() && day.Add(24*time.Hour).Before(p.Start.UTC()) {
		return true, nil
	}
	if !p.End.IsZero() && !day.Before(p.End.UTC()) {
		return true, nil
	}
	return false, nil
}

func readFile(path string, q Query, result *ReadResult) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("store: open %s: %w", path, err)
	}
	defer f.Close() //nolint:errcheck // read-only file

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		var e omnidevx.Event
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			result.Diagnostics = append(result.Diagnostics, omnidevx.Diagnostic{
				Severity: omnidevx.SeverityWarning,
				Message:  fmt.Sprintf("skipping damaged line %d: %v", lineNo, err),
				Path:     path,
			})
			continue
		}
		if q.Period.Contains(e.Timestamp) {
			result.Events = append(result.Events, e)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("store: scan %s: %w", path, err)
	}
	return nil
}
