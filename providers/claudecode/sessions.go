package claudecode

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	omnidevx "github.com/plexusone/omnidevx-core"
	"github.com/plexusone/omnidevx-core/sessions"
)

const (
	// headBytes is how much of a transcript is read from the start to find
	// the creation time, working directory, and first prompt.
	headBytes = 256 << 10
	// tailBytes is the initial tail read for last activity and the latest
	// title. It grows up to maxTailBytes while no human prompt is found,
	// because a long agent run is mostly tool results.
	tailBytes    = 256 << 10
	maxTailBytes = 8 << 20
	// recentPrompts is how many human prompts are kept per session.
	recentPrompts = 3
	// promptWidth truncates prompt text kept on a session.
	promptWidth = 300
	// startTolerance is how far a live-session record's start time may
	// differ from the process's actual start time.
	startTolerance = 60 * time.Second
)

// SessionReader lists Claude Code sessions from the local store. It reads
// prompt text and titles, so it is separate from the metadata-only
// Collector; see the sessions package for the content-access contract.
type SessionReader struct {
	dir string
	// processStart returns when a process started, or an error if it is not
	// running. Replaceable in tests.
	processStart func(pid int) (time.Time, error)
}

var _ sessions.Reader = (*SessionReader)(nil)

// NewSessionReader returns a SessionReader for the given options.
func NewSessionReader(opts Options) (*SessionReader, error) {
	dir := opts.Dir
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("claudecode: resolve home directory: %w", err)
		}
		dir = filepath.Join(home, ".claude")
	}
	return &SessionReader{dir: dir, processStart: psStart}, nil
}

// Harness implements sessions.Reader.
func (r *SessionReader) Harness() sessions.Harness { return sessions.HarnessClaudeCode }

// sessionRecord is the subset of a transcript line the reader uses.
type sessionRecord struct {
	Type         string    `json:"type"`
	Timestamp    time.Time `json:"timestamp"`
	CWD          string    `json:"cwd"`
	GitBranch    string    `json:"gitBranch"`
	IsSidechain  bool      `json:"isSidechain"`
	IsMeta       bool      `json:"isMeta"`
	PromptSource string    `json:"promptSource"`
	AITitle      string    `json:"aiTitle"`
	Message      *struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// liveRecord is one ~/.claude/sessions/<pid>.json file.
type liveRecord struct {
	PID       int    `json:"pid"`
	SessionID string `json:"sessionId"`
	CWD       string `json:"cwd"`
	StartedAt int64  `json:"startedAt"` // epoch milliseconds
	Name      string `json:"name"`
}

// List implements sessions.Reader.
func (r *SessionReader) List(ctx context.Context, opts sessions.ListOptions) ([]sessions.Session, []omnidevx.Diagnostic, error) {
	projectsDir := filepath.Join(r.dir, "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil, nil, fmt.Errorf("claudecode: read projects directory: %w", err)
	}

	live, diags := r.liveSessions()

	var out []sessions.Session
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		files, err := filepath.Glob(filepath.Join(projectsDir, entry.Name(), "*.jsonl"))
		if err != nil {
			return nil, diags, fmt.Errorf("claudecode: glob %s: %w", entry.Name(), err)
		}
		for _, file := range files {
			if err := ctx.Err(); err != nil {
				return out, diags, err
			}
			s, err := readSession(file, opts.NoContent)
			if err != nil {
				diags = append(diags, omnidevx.Diagnostic{
					Severity: omnidevx.SeverityWarning,
					Message:  fmt.Sprintf("read session: %v", err),
					Path:     file,
				})
				continue
			}
			if s == nil {
				continue // no timestamps: not a real conversation
			}
			if !opts.Since.IsZero() && s.LastActivityAt.Before(opts.Since) {
				continue
			}
			if rec, ok := live[s.ID]; ok {
				s.State = sessions.StateRunning
				s.Runtime = &sessions.Runtime{PID: rec.PID}
				if rec.Name != "" && !opts.NoContent {
					s.Title, s.TitleSource = rec.Name, sessions.TitleHarness
				}
			}
			out = append(out, *s)
		}
	}
	return out, diags, nil
}

// liveSessions maps session ID to the live process record, keeping only
// records whose PID is alive and whose start time matches the record. A
// stale file left by a crash or reboot may name a reused PID, so liveness
// alone is not trusted.
func (r *SessionReader) liveSessions() (map[string]liveRecord, []omnidevx.Diagnostic) {
	live := map[string]liveRecord{}
	var diags []omnidevx.Diagnostic
	files, err := filepath.Glob(filepath.Join(r.dir, "sessions", "*.json"))
	if err != nil {
		return live, nil
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				diags = append(diags, omnidevx.Diagnostic{
					Severity: omnidevx.SeverityWarning,
					Message:  fmt.Sprintf("read live-session record: %v", err),
					Path:     file,
				})
			}
			continue
		}
		var rec liveRecord
		if err := json.Unmarshal(data, &rec); err != nil || rec.SessionID == "" || rec.PID <= 0 {
			continue
		}
		started, err := r.processStart(rec.PID)
		if err != nil {
			continue
		}
		recorded := time.UnixMilli(rec.StartedAt)
		if d := started.Sub(recorded); d > startTolerance || d < -startTolerance {
			continue
		}
		live[rec.SessionID] = rec
	}
	return live, diags
}

// psStart returns the start time of a running process via ps. It returns an
// error when the process does not exist.
func psStart(pid int) (time.Time, error) {
	if err := syscall.Kill(pid, 0); err != nil && !errors.Is(err, syscall.EPERM) {
		return time.Time{}, err
	}
	out, err := exec.Command("ps", "-o", "lstart=", "-p", strconv.Itoa(pid)).Output() //nolint:gosec // pid is an int
	if err != nil {
		return time.Time{}, err
	}
	// lstart is local time, e.g. "Wed Oct  7 10:04:54 2026".
	return time.ParseInLocation("Mon Jan _2 15:04:05 2006", strings.TrimSpace(string(out)), time.Local)
}

// readSession builds a Session from one transcript using bounded head and
// tail reads. It returns nil for a file with no timestamped records.
func readSession(path string, noContent bool) (*sessions.Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // read-only file
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()

	s := &sessions.Session{
		Harness: sessions.HarnessClaudeCode,
		ID:      strings.TrimSuffix(filepath.Base(path), ".jsonl"),
		State:   sessions.StateResumable,
	}

	// Head: creation time, working directory, branch, first prompt.
	var firstPrompt string
	err = scanRecords(io.NewSectionReader(f, 0, min(size, headBytes)), false, func(rec sessionRecord) bool {
		if s.CreatedAt.IsZero() && !rec.Timestamp.IsZero() {
			s.CreatedAt = rec.Timestamp
		}
		if s.CWD == "" && rec.CWD != "" {
			s.CWD = rec.CWD
		}
		if s.GitBranch == "" && rec.GitBranch != "" {
			s.GitBranch = rec.GitBranch
		}
		if firstPrompt == "" && isHumanPrompt(rec) {
			if text := promptText(rec); text != "" && !strings.HasPrefix(text, "<") {
				firstPrompt = text
			}
		}
		return s.CreatedAt.IsZero() || s.CWD == "" || firstPrompt == ""
	})
	if err != nil {
		return nil, err
	}
	if s.CreatedAt.IsZero() {
		return nil, nil
	}

	// Tail: last activity, latest title, last human prompts. Grow the window
	// until a human prompt is found or the file/limit is exhausted.
	var (
		aiTitle     string
		prompts     []sessions.Prompt
		lastHuman   time.Time
		lastRecTime time.Time
	)
	for window := int64(tailBytes); ; window *= 4 {
		window = min(window, maxTailBytes)
		start := max(size-window, 0)
		aiTitle, prompts, lastHuman, lastRecTime = "", nil, time.Time{}, time.Time{}
		err = scanRecords(io.NewSectionReader(f, start, size-start), start > 0, func(rec sessionRecord) bool {
			if rec.Type == "ai-title" && rec.AITitle != "" {
				aiTitle = rec.AITitle
			}
			if !rec.Timestamp.IsZero() && !rec.IsSidechain {
				lastRecTime = rec.Timestamp
			}
			if isHumanPrompt(rec) {
				lastHuman = rec.Timestamp
				if text := promptText(rec); text != "" && !strings.HasPrefix(text, "<") {
					prompts = append(prompts, sessions.Prompt{At: rec.Timestamp, Text: sessions.Truncate(text, promptWidth)})
				}
			}
			return true
		})
		if err != nil {
			return nil, err
		}
		if !lastHuman.IsZero() || start == 0 || window >= maxTailBytes {
			break
		}
	}

	s.LastActivityAt = lastRecTime
	if s.LastActivityAt.IsZero() {
		s.LastActivityAt = info.ModTime().UTC()
	}
	s.LastHumanActivityAt = lastHuman
	if len(prompts) > recentPrompts {
		prompts = prompts[len(prompts)-recentPrompts:]
	}

	if !noContent {
		s.RecentPrompts = prompts
		switch {
		case aiTitle != "":
			s.Title, s.TitleSource = aiTitle, sessions.TitleHarness
		case firstPrompt != "":
			s.Title, s.TitleSource = sessions.Truncate(firstPrompt, 80), sessions.TitleFirstPrompt
		}
	}
	if s.Title == "" {
		s.Title, s.TitleSource = filepath.Base(s.CWD), sessions.TitleCWD
	}

	// Claude Code finds a session by the project directory it started in,
	// so the resume command must run from the recorded working directory.
	s.Resume = sessions.ResumeSpec{Argv: []string{"claude", "--resume", s.ID}, Dir: s.CWD}
	return s, nil
}

// scanRecords calls fn for each parseable JSON line until fn returns false.
// When skipPartial is set the first line is dropped, because the window
// began mid-line. Unparseable lines are skipped.
func scanRecords(r io.Reader, skipPartial bool, fn func(sessionRecord) bool) error {
	br := bufio.NewReaderSize(r, 1<<20)
	first := true
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 && !(first && skipPartial) {
			var rec sessionRecord
			if json.Unmarshal(bytes.TrimSpace(line), &rec) == nil {
				if !fn(rec) {
					return nil
				}
			}
		}
		first = false
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

// isHumanPrompt reports whether a record is a message the human typed, as
// opposed to a tool result, a harness-injected message, or subagent
// traffic. Current Claude Code versions mark typed and queued prompts with
// promptSource; older records are classified from their content.
func isHumanPrompt(rec sessionRecord) bool {
	if rec.Type != "user" || rec.IsSidechain || rec.Message == nil {
		return false
	}
	switch rec.PromptSource {
	case "typed", "queued":
		return true
	case "":
		// Older record: no marker, so classify from content below.
	default:
		return false // e.g. "system"
	}
	if rec.IsMeta {
		return false
	}
	text := promptText(rec)
	if text == "" || hasToolResult(rec.Message.Content) {
		return false
	}
	for _, prefix := range []string{"<command-name>", "<local-command", "<system-reminder", "Caveat:"} {
		if strings.HasPrefix(text, prefix) {
			return false
		}
	}
	return true
}

// promptText extracts the text of a user message, or "" if it has none.
func promptText(rec sessionRecord) string {
	if rec.Message == nil {
		return ""
	}
	var s string
	if json.Unmarshal(rec.Message.Content, &s) == nil {
		return strings.TrimSpace(s)
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(rec.Message.Content, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
			parts = append(parts, strings.TrimSpace(b.Text))
		}
	}
	return strings.Join(parts, "\n")
}

func hasToolResult(content json.RawMessage) bool {
	var blocks []struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(content, &blocks) != nil {
		return false
	}
	for _, b := range blocks {
		if b.Type == "tool_result" {
			return true
		}
	}
	return false
}
