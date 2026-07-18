package claudecode

import (
	"context"
	"testing"
	"time"

	omnidevx "github.com/plexusone/omnidevx-core"
)

func collectTestdata(t *testing.T, req omnidevx.CollectRequest) *omnidevx.CollectionResult {
	t.Helper()
	c, err := New(Options{Dir: "testdata"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.Collect(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func countByType(events []omnidevx.Event) map[omnidevx.EventType]int {
	counts := map[omnidevx.EventType]int{}
	for _, e := range events {
		counts[e.Type]++
	}
	return counts
}

func TestCollectSessionFile(t *testing.T) {
	subject := omnidevx.SubjectRef{PersonID: "person:test"}
	result := collectTestdata(t, omnidevx.CollectRequest{Subject: subject})

	counts := countByType(result.Events)
	want := map[omnidevx.EventType]int{
		omnidevx.EventSessionStarted:   1,
		omnidevx.EventSessionEnded:     1,
		omnidevx.EventPromptSubmitted:  1, // sidechain prompt excluded
		omnidevx.EventMessageCompleted: 2,
		omnidevx.EventToolCompleted:    1,
	}
	for eventType, n := range want {
		if counts[eventType] != n {
			t.Errorf("event type %s: got %d, want %d", eventType, counts[eventType], n)
		}
	}

	// The unparseable line must surface as a diagnostic, not vanish.
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics: got %d, want 1: %+v", len(result.Diagnostics), result.Diagnostics)
	}
	if result.Diagnostics[0].Severity != omnidevx.SeverityWarning {
		t.Errorf("diagnostic severity: got %s, want %s", result.Diagnostics[0].Severity, omnidevx.SeverityWarning)
	}

	for _, e := range result.Events {
		if e.Subject != subject {
			t.Errorf("event %s: subject not stamped: %+v", e.ID, e.Subject)
		}
		if e.Provenance.CollectionMode != omnidevx.ModeHistory {
			t.Errorf("event %s: collection mode: got %s", e.ID, e.Provenance.CollectionMode)
		}
		if e.Source != (omnidevx.Source{Provider: "anthropic", Product: "claude-code"}) {
			t.Errorf("event %s: unexpected source %+v", e.ID, e.Source)
		}
	}
}

func TestToolAttributionAndUsage(t *testing.T) {
	result := collectTestdata(t, omnidevx.CollectRequest{})

	var tool, message *omnidevx.Event
	for i, e := range result.Events {
		switch e.Type {
		case omnidevx.EventToolCompleted:
			tool = &result.Events[i]
		case omnidevx.EventMessageCompleted:
			if message == nil {
				message = &result.Events[i]
			}
		}
	}
	if tool == nil {
		t.Fatal("no tool.completed event")
	}
	if got := tool.Attributes[omnidevx.AttrTool]; got != "Bash" {
		t.Errorf("tool name: got %v, want Bash", got)
	}
	if got := tool.Attributes[omnidevx.AttrSuccess]; got != true {
		t.Errorf("tool success: got %v, want true", got)
	}

	if message == nil {
		t.Fatal("no message.completed event")
	}
	if got := message.Attributes[omnidevx.AttrModel]; got != "claude-opus-4-5" {
		t.Errorf("model: got %v", got)
	}
	if got := message.Attributes[omnidevx.AttrInputTokens]; got != int64(100) {
		t.Errorf("input tokens: got %v (%T), want 100", got, got)
	}
	if got := message.Attributes[omnidevx.AttrCacheCreationTokens]; got != int64(2000) {
		t.Errorf("cache creation tokens: got %v, want 2000", got)
	}

	// Privacy: no attribute may carry content-like keys.
	for _, e := range result.Events {
		for key := range e.Attributes {
			switch key {
			case "content", "text", "prompt", "output", "stdout", "stderr":
				t.Errorf("event %s leaks content attribute %q", e.ID, key)
			}
		}
	}
}

func TestPeriodFilter(t *testing.T) {
	// Period covering only the first two records.
	result := collectTestdata(t, omnidevx.CollectRequest{
		Period: omnidevx.Period{
			Start: time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC),
			End:   time.Date(2026, 7, 1, 10, 0, 6, 0, time.UTC),
		},
	})
	counts := countByType(result.Events)
	if counts[omnidevx.EventPromptSubmitted] != 1 {
		t.Errorf("prompts in window: got %d, want 1", counts[omnidevx.EventPromptSubmitted])
	}
	if counts[omnidevx.EventMessageCompleted] != 1 {
		t.Errorf("messages in window: got %d, want 1", counts[omnidevx.EventMessageCompleted])
	}
	if counts[omnidevx.EventToolCompleted] != 0 {
		t.Errorf("tools in window: got %d, want 0", counts[omnidevx.EventToolCompleted])
	}
	// Session end (10:05) is outside the window; session start is inside.
	if counts[omnidevx.EventSessionStarted] != 1 || counts[omnidevx.EventSessionEnded] != 0 {
		t.Errorf("session events: got start=%d end=%d, want 1/0",
			counts[omnidevx.EventSessionStarted], counts[omnidevx.EventSessionEnded])
	}
}
