// Package sessions defines a provider-neutral catalog of coding-agent
// sessions: what exists on this machine, what each was doing, when a human
// last interacted with it, and how to resume it.
//
// Content-access contract: unlike the telemetry collectors in this module,
// session readers read prompt text and titles from local harness storage so
// a developer can recognize a session. That content is held in memory and
// rendered; it is never written to the telemetry event store. Readers do
// not make network calls.
package sessions

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	omnidevx "github.com/plexusone/omnidevx-core"
)

// Harness identifies the coding-agent tool that owns a session.
type Harness string

// Known harnesses.
const (
	HarnessClaudeCode Harness = "claude-code"
	HarnessCodex      Harness = "codex"
)

// State is whether a session currently has a live process.
type State string

// Session states.
const (
	// StateResumable means the session exists on disk and no live process
	// was found for it.
	StateResumable State = "resumable"
	// StateRunning means a live process is attached to the session.
	StateRunning State = "running"
	// StateUnknown means liveness could not be determined.
	StateUnknown State = "unknown"
)

// TitleSource records where a session's title came from.
const (
	TitleHarness     = "harness"
	TitleFirstPrompt = "first-prompt"
	TitleCWD         = "cwd"
)

// Session is one resumable harness session. (Harness, ID) is its identity;
// everything else describes it.
type Session struct {
	// Harness is the coding-agent tool that owns the session: claude-code or codex.
	Harness Harness `json:"harness"`
	// ID is the harness's own session identifier, durable across process exit and reboot.
	ID string `json:"id"`

	// CWD is the directory the session was started in. Resuming must happen from here.
	CWD string `json:"cwd"`
	// GitBranch is the git branch recorded for the session, when the harness records one.
	GitBranch string `json:"gitBranch,omitempty"`
	// GitOrigin is the normalized origin remote, such as github.com/org/repo, when recorded.
	GitOrigin string `json:"gitOrigin,omitempty"`

	// CreatedAt is when the session started.
	CreatedAt time.Time `json:"createdAt"`
	// LastActivityAt is the last activity of any kind, including agent work.
	LastActivityAt time.Time `json:"lastActivityAt"`
	// LastHumanActivityAt is the last prompt a person typed. It is absent when none was found.
	// A large gap after LastActivityAt means the agent kept working unattended.
	LastHumanActivityAt time.Time `json:"lastHumanActivityAt,omitzero"`

	// Title names the session. It comes from the harness when it records one.
	Title string `json:"title,omitempty"`
	// TitleSource says where Title came from: harness, first-prompt, or cwd.
	TitleSource string `json:"titleSource,omitempty"`
	// RecentPrompts are the latest prompts a person typed, truncated. They are omitted when
	// prompt content is suppressed.
	RecentPrompts []Prompt `json:"recentPrompts,omitempty"`

	// Archived is set when the harness marks the session archived.
	Archived bool `json:"archived,omitempty"`
	// State is running, resumable, or unknown.
	State State `json:"state"`
	// Runtime describes the live process when State is running.
	Runtime *Runtime `json:"runtime,omitempty"`

	// Resume says how to resume the session.
	Resume ResumeSpec `json:"resume"`

	// Evidence is what the session touched. It is populated only when requested, because it
	// needs a full transcript parse.
	Evidence *Evidence `json:"evidence,omitempty"`
}

// Prompt is a human-authored message, truncated for display.
type Prompt struct {
	// At is when the prompt was sent.
	At time.Time `json:"at"`
	// Text is the prompt text, truncated.
	Text string `json:"text"`
}

// Runtime describes the live process attached to a running session.
type Runtime struct {
	// PID is the operating-system process ID of the harness.
	PID int `json:"pid,omitempty"`
}

// ResumeSpec says how to resume a session. Readers return it; callers
// decide how to run it (replace the process, new terminal, tmux).
type ResumeSpec struct {
	// Argv is the command and arguments that resume the session.
	Argv []string `json:"argv"`
	// Dir is the directory to run the command from.
	Dir string `json:"dir"`
}

// Command renders the spec as a shell command line for display.
func (r ResumeSpec) Command() string {
	parts := make([]string, 0, len(r.Argv)+2)
	if r.Dir != "" {
		parts = append(parts, "cd", shellQuote(r.Dir), "&&")
	}
	for _, a := range r.Argv {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}

func shellQuote(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\n'\"\\$`!*?()[]{}<>|&;#~") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ListOptions scopes a Reader.List call.
type ListOptions struct {
	// Since drops sessions whose last activity is before this time.
	Since time.Time
	// IncludeArchived includes sessions the harness marks archived.
	IncludeArchived bool
	// NoContent suppresses prompt-derived fields (title text, prompts).
	NoContent bool
}

// Reader lists the sessions of one harness. List must be cheap: it may use
// file metadata, indexes, and bounded head/tail reads, but not full
// transcript parses. Unparseable sessions become diagnostics, not errors;
// only failure to read the harness store at all is an error.
type Reader interface {
	Harness() Harness
	List(ctx context.Context, opts ListOptions) ([]Session, []omnidevx.Diagnostic, error)
}

// Catalog merges the sessions of several readers.
type Catalog struct {
	readers []Reader
}

// NewCatalog returns a Catalog over the given readers.
func NewCatalog(readers ...Reader) *Catalog {
	return &Catalog{readers: readers}
}

// List returns sessions from every reader, newest activity first. One
// reader failing does not hide the others: the result is returned together
// with a joined error naming each failing harness.
func (c *Catalog) List(ctx context.Context, opts ListOptions) ([]Session, []omnidevx.Diagnostic, error) {
	var (
		all   []Session
		diags []omnidevx.Diagnostic
		errs  []string
	)
	for _, r := range c.readers {
		if err := ctx.Err(); err != nil {
			return all, diags, err
		}
		s, d, err := r.List(ctx, opts)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", r.Harness(), err))
		}
		all = append(all, s...)
		diags = append(diags, d...)
	}
	sort.SliceStable(all, func(i, j int) bool {
		return all[i].LastActivityAt.After(all[j].LastActivityAt)
	})
	if len(errs) > 0 {
		return all, diags, fmt.Errorf("list sessions: %s", strings.Join(errs, "; "))
	}
	return all, diags, nil
}

// maxAmbiguousShown caps how many candidates an ambiguous-ID error lists.
const maxAmbiguousShown = 8

// Resolve finds one session by full ID or unique prefix of at least four
// characters. A "harness:" qualifier (e.g. "codex:0199") narrows the search.
// An ambiguous prefix returns an error listing the candidates.
func Resolve(all []Session, query string) (*Session, error) {
	var harness Harness
	if h, rest, ok := strings.Cut(query, ":"); ok {
		harness, query = Harness(h), rest
	}
	if len(query) < 4 {
		return nil, fmt.Errorf("session id %q too short: need at least 4 characters", query)
	}
	var matches []int
	for i := range all {
		if harness != "" && all[i].Harness != harness {
			continue
		}
		if strings.HasPrefix(all[i].ID, query) {
			if all[i].ID == query {
				return &all[i], nil
			}
			matches = append(matches, i)
		}
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no session matches %q", query)
	case 1:
		return &all[matches[0]], nil
	}
	var b strings.Builder
	for n, i := range matches {
		if n == maxAmbiguousShown {
			fmt.Fprintf(&b, "\n  … and %d more; use a longer prefix", len(matches)-n)
			break
		}
		fmt.Fprintf(&b, "\n  %s:%s  %s", all[i].Harness, all[i].ID, Truncate(all[i].Title, 70))
	}
	return nil, fmt.Errorf("%q is ambiguous (%d matches):%s", query, len(matches), b.String())
}

// Truncate shortens s to at most n runes, collapsing whitespace, for
// single-line display.
func Truncate(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}
