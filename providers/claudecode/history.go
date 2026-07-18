package claudecode

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	omnidevx "github.com/plexusone/omnidevx-core"
)

// record is the subset of a Claude Code session-file line that OmniDevX
// reads. Content payloads are parsed only far enough to classify blocks;
// text is never extracted.
type record struct {
	Type          string          `json:"type"`
	UUID          string          `json:"uuid"`
	Timestamp     time.Time       `json:"timestamp"`
	SessionID     string          `json:"sessionId"`
	CWD           string          `json:"cwd"`
	GitBranch     string          `json:"gitBranch"`
	Version       string          `json:"version"`
	IsSidechain   bool            `json:"isSidechain"`
	ToolUseResult json.RawMessage `json:"toolUseResult"`
	Message       *message        `json:"message"`
}

type message struct {
	Role    string          `json:"role"`
	Model   string          `json:"model"`
	Usage   *usage          `json:"usage"`
	Content json.RawMessage `json:"content"`
}

type usage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
}

// contentBlock classifies one element of a message content array.
type contentBlock struct {
	Type      string `json:"type"`
	ID        string `json:"id"`          // tool_use
	Name      string `json:"name"`        // tool_use
	ToolUseID string `json:"tool_use_id"` // tool_result
	IsError   bool   `json:"is_error"`    // tool_result
}

// sessionSpan tracks the observed bounds of one session within a file.
type sessionSpan struct {
	first, last time.Time
	cwd, branch string
	version     string
}

// parseSessionFile converts one session file into canonical events. Parse
// failures become diagnostics; only I/O errors abort the file.
func parseSessionFile(path string, req omnidevx.CollectRequest) ([]omnidevx.Event, []omnidevx.Diagnostic) {
	f, err := os.Open(path)
	if err != nil {
		return nil, []omnidevx.Diagnostic{{
			Severity: omnidevx.SeverityError,
			Message:  fmt.Sprintf("open session file: %v", err),
			Path:     path,
		}}
	}
	defer f.Close() //nolint:errcheck // read-only file

	var (
		events   []omnidevx.Event
		diags    []omnidevx.Diagnostic
		sessions = map[string]*sessionSpan{}
		// toolNames maps tool_use IDs to tool names so tool_result records
		// (which carry only the ID) can be attributed.
		toolNames = map[string]string{}
		reader    = bufio.NewReaderSize(f, 1<<20)
		lineNo    = 0
	)

	for {
		line, err := readLine(reader)
		if err == io.EOF {
			break
		}
		if err != nil {
			diags = append(diags, omnidevx.Diagnostic{
				Severity: omnidevx.SeverityError,
				Message:  fmt.Sprintf("read line %d: %v", lineNo+1, err),
				Path:     path,
			})
			break
		}
		lineNo++
		if len(line) == 0 {
			continue
		}

		var rec record
		if err := json.Unmarshal(line, &rec); err != nil {
			diags = append(diags, omnidevx.Diagnostic{
				Severity: omnidevx.SeverityWarning,
				Message:  fmt.Sprintf("skipping unparseable line %d: %v", lineNo, err),
				Path:     path,
			})
			continue
		}

		if rec.SessionID != "" && !rec.Timestamp.IsZero() {
			span := sessions[rec.SessionID]
			if span == nil {
				span = &sessionSpan{first: rec.Timestamp}
				sessions[rec.SessionID] = span
			}
			if rec.Timestamp.Before(span.first) {
				span.first = rec.Timestamp
			}
			if rec.Timestamp.After(span.last) {
				span.last = rec.Timestamp
			}
			if span.cwd == "" {
				span.cwd = rec.CWD
			}
			if span.branch == "" {
				span.branch = rec.GitBranch
			}
			if span.version == "" {
				span.version = rec.Version
			}
		}

		switch rec.Type {
		case "user":
			events = append(events, userEvents(&rec, req, toolNames)...)
		case "assistant":
			events = append(events, assistantEvents(&rec, req, toolNames)...)
		}
	}

	for sessionID, span := range sessions {
		events = append(events, sessionEvents(sessionID, span, req)...)
	}
	for i := range events {
		events[i].Subject = req.Subject
	}
	return events, diags
}

// readLine reads one full line without a fixed length cap; session lines can
// exceed bufio.Scanner's default limits (pasted content, large tool results).
func readLine(r *bufio.Reader) ([]byte, error) {
	line, err := r.ReadBytes('\n')
	if err == nil {
		return line[:len(line)-1], nil
	}
	if err == io.EOF && len(line) > 0 {
		return line, nil
	}
	return nil, err
}

func userEvents(rec *record, req omnidevx.CollectRequest, toolNames map[string]string) []omnidevx.Event {
	if rec.Message == nil || !req.Period.Contains(rec.Timestamp) {
		return nil
	}

	// String content is a plain prompt.
	var asString string
	if err := json.Unmarshal(rec.Message.Content, &asString); err == nil {
		if rec.IsSidechain {
			return nil
		}
		return []omnidevx.Event{newEvent(rec, omnidevx.EventPromptSubmitted, rec.UUID, nil)}
	}

	var blocks []contentBlock
	if err := json.Unmarshal(rec.Message.Content, &blocks); err != nil {
		return nil
	}

	var events []omnidevx.Event
	prompt := false
	for _, b := range blocks {
		switch b.Type {
		case "text":
			prompt = true
		case "tool_result":
			attrs := map[string]any{omnidevx.AttrSuccess: !b.IsError}
			if name := toolNames[b.ToolUseID]; name != "" {
				attrs[omnidevx.AttrTool] = name
			}
			if rec.IsSidechain {
				attrs[omnidevx.AttrSidechain] = true
			}
			events = append(events, newEvent(rec, omnidevx.EventToolCompleted, b.ToolUseID, attrs))
		}
	}
	if prompt && !rec.IsSidechain {
		events = append(events, newEvent(rec, omnidevx.EventPromptSubmitted, rec.UUID, nil))
	}
	return events
}

func assistantEvents(rec *record, req omnidevx.CollectRequest, toolNames map[string]string) []omnidevx.Event {
	if rec.Message == nil {
		return nil
	}

	// Register tool_use IDs regardless of period so tool_result records just
	// inside the period boundary still resolve names.
	var blocks []contentBlock
	if err := json.Unmarshal(rec.Message.Content, &blocks); err == nil {
		for _, b := range blocks {
			if b.Type == "tool_use" && b.ID != "" {
				toolNames[b.ID] = b.Name
			}
		}
	}

	if !req.Period.Contains(rec.Timestamp) {
		return nil
	}

	attrs := map[string]any{}
	if rec.Message.Model != "" {
		attrs[omnidevx.AttrModel] = rec.Message.Model
	}
	if u := rec.Message.Usage; u != nil {
		attrs[omnidevx.AttrInputTokens] = u.InputTokens
		attrs[omnidevx.AttrOutputTokens] = u.OutputTokens
		attrs[omnidevx.AttrCacheReadTokens] = u.CacheReadInputTokens
		attrs[omnidevx.AttrCacheCreationTokens] = u.CacheCreationInputTokens
	}
	if rec.IsSidechain {
		attrs[omnidevx.AttrSidechain] = true
	}
	return []omnidevx.Event{newEvent(rec, omnidevx.EventMessageCompleted, rec.UUID, attrs)}
}

func sessionEvents(sessionID string, span *sessionSpan, req omnidevx.CollectRequest) []omnidevx.Event {
	ctx := omnidevx.EventContext{
		SessionID: sessionID,
		Workspace: span.cwd,
		GitBranch: span.branch,
	}
	var events []omnidevx.Event
	if req.Period.Contains(span.first) {
		attrs := map[string]any{}
		if span.version != "" {
			attrs[omnidevx.AttrClientVersion] = span.version
		}
		events = append(events, omnidevx.Event{
			ID:         "claude-code:" + sessionID + ":start",
			Type:       omnidevx.EventSessionStarted,
			Timestamp:  span.first,
			Subject:    req.Subject,
			Source:     source,
			Context:    ctx,
			Attributes: attrs,
			Provenance: provenance(),
		})
	}
	if !span.last.IsZero() && req.Period.Contains(span.last) {
		events = append(events, omnidevx.Event{
			ID:         "claude-code:" + sessionID + ":end",
			Type:       omnidevx.EventSessionEnded,
			Timestamp:  span.last,
			Subject:    req.Subject,
			Source:     source,
			Context:    ctx,
			Provenance: provenance(),
		})
	}
	return events
}

func newEvent(rec *record, eventType omnidevx.EventType, id string, attrs map[string]any) omnidevx.Event {
	if len(attrs) == 0 {
		attrs = nil
	}
	return omnidevx.Event{
		ID:        "claude-code:" + string(eventType) + ":" + id,
		Type:      eventType,
		Timestamp: rec.Timestamp,
		Source:    source,
		Context: omnidevx.EventContext{
			SessionID: rec.SessionID,
			Workspace: rec.CWD,
			GitBranch: rec.GitBranch,
		},
		Attributes: attrs,
		Provenance: provenance(),
	}
}

func provenance() omnidevx.Provenance {
	return omnidevx.Provenance{
		CollectionMode: omnidevx.ModeHistory,
		Confidence:     DefaultConfidence,
	}
}
