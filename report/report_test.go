package report

import (
	"testing"
	"time"

	omnidevx "github.com/plexusone/omnidevx-core"
)

var claudeSource = omnidevx.Source{Provider: "anthropic", Product: "claude-code"}
var gitSource = omnidevx.Source{Provider: "git", Product: "git"}

func historyEvent(id string, typ omnidevx.EventType, ts time.Time, src omnidevx.Source, sessionID string, attrs map[string]any) omnidevx.Event {
	return omnidevx.Event{
		ID:         id,
		Type:       typ,
		Timestamp:  ts,
		Source:     src,
		Context:    omnidevx.EventContext{SessionID: sessionID},
		Attributes: attrs,
		Provenance: omnidevx.Provenance{CollectionMode: omnidevx.ModeHistory, Confidence: 0.9},
	}
}

func TestBuildSumsAdditiveMetrics(t *testing.T) {
	day := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	events := []omnidevx.Event{
		historyEvent("p:1", omnidevx.EventPromptSubmitted, day.Add(time.Hour), claudeSource, "s1", nil),
		historyEvent("p:2", omnidevx.EventPromptSubmitted, day.Add(2*time.Hour), claudeSource, "s1", nil),
		historyEvent("m:1", omnidevx.EventMessageCompleted, day.Add(time.Hour), claudeSource, "s1", map[string]any{
			omnidevx.AttrInputTokens: 100, omnidevx.AttrOutputTokens: 50,
		}),
		historyEvent("c:1", omnidevx.EventChangeCommitted, day.Add(3*time.Hour), gitSource, "", map[string]any{
			omnidevx.AttrInsertions: 20, omnidevx.AttrDeletions: 5, omnidevx.AttrFilesChanged: 2,
			omnidevx.AttrAIAssisted: true,
		}),
	}
	period := omnidevx.Period{Start: day, End: day.Add(24 * time.Hour)}
	report := Build(events, Subject{PersonID: "person:john"}, period)

	if got := report.Metrics.Combined["prompts"].Value; got != 2 {
		t.Errorf("prompts: got %v, want 2", got)
	}
	if got := report.Metrics.Combined["input_tokens"].Value; got != 100 {
		t.Errorf("input_tokens: got %v, want 100", got)
	}
	if got := report.Metrics.Combined["commits"].Value; got != 1 {
		t.Errorf("commits: got %v, want 1", got)
	}
	if got := report.Metrics.Combined["ai_assisted_commits"].Value; got != 1 {
		t.Errorf("ai_assisted_commits: got %v, want 1", got)
	}
	if got := report.Metrics.Combined["insertions"].Value; got != 20 {
		t.Errorf("insertions: got %v, want 20", got)
	}

	// bySource retains the per-source breakdown.
	claudeKey := sourceKey(claudeSource)
	if got := report.Metrics.BySource[claudeKey]["prompts"].Value; got != 2 {
		t.Errorf("bySource prompts: got %v, want 2", got)
	}
	gitKey := sourceKey(gitSource)
	if _, ok := report.Metrics.BySource[gitKey]["prompts"]; ok {
		t.Error("git source should not carry a prompts metric")
	}
}

func TestBuildDeduplicatesSessionsAcrossDayBoundary(t *testing.T) {
	day1 := time.Date(2026, 7, 6, 23, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 7, 7, 1, 0, 0, 0, time.UTC) // same session, next calendar day
	events := []omnidevx.Event{
		historyEvent("m:1", omnidevx.EventMessageCompleted, day1, claudeSource, "s1", nil),
		historyEvent("m:2", omnidevx.EventMessageCompleted, day2, claudeSource, "s1", nil),
		historyEvent("m:3", omnidevx.EventMessageCompleted, day2, claudeSource, "s2", nil),
	}
	period := omnidevx.Period{
		Start: time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC),
	}
	report := Build(events, Subject{PersonID: "person:john"}, period)

	// Naively summing daily distinct counts would double-count s1 (seen on
	// both days); the correct answer dedupes across the whole period.
	if got := report.Metrics.Combined["sessions"].Value; got != 2 {
		t.Errorf("sessions: got %v, want 2 (deduplicated across day boundary)", got)
	}
}

func TestBuildTasksNotCombinedAcrossSources(t *testing.T) {
	day := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	events := []omnidevx.Event{
		historyEvent("t:1", omnidevx.EventTaskCompleted, day.Add(time.Hour), claudeSource, "s1", nil),
	}
	period := omnidevx.Period{Start: day, End: day.Add(24 * time.Hour)}
	report := Build(events, Subject{PersonID: "person:john"}, period)

	if _, ok := report.Metrics.Combined["tasks"]; ok {
		t.Error("tasks should not appear in Combined (only after cross-source normalization, per package doc)")
	}
	if got := report.Metrics.BySource[sourceKey(claudeSource)]["tasks"].Value; got != 1 {
		t.Errorf("bySource tasks: got %v, want 1", got)
	}
}

func TestBuildSnapshotEventsExcludedWithWarning(t *testing.T) {
	day := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	githubSource := omnidevx.Source{Provider: "github", Product: "github"}
	events := []omnidevx.Event{
		historyEvent("snap:1", omnidevx.EventProfileSnapshot, day.Add(time.Hour), githubSource, "", map[string]any{
			omnidevx.AttrCommits: 500,
		}),
	}
	period := omnidevx.Period{Start: day, End: day.Add(24 * time.Hour)}
	report := Build(events, Subject{PersonID: "person:john"}, period)

	if len(report.Metrics.Combined) != 0 {
		t.Errorf("snapshot events should not contribute additive metrics: %+v", report.Metrics.Combined)
	}
	if len(report.Quality.Warnings) == 0 {
		t.Error("expected a data-quality warning about unmerged snapshot events")
	}
	// But the source is still counted in coverage — it did observe the subject.
	if len(report.Sources) != 1 || report.Sources[0].EventCount != 1 {
		t.Errorf("expected snapshot source counted in coverage: %+v", report.Sources)
	}
}

func TestBuildCoverageScore(t *testing.T) {
	weekStart := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	period := omnidevx.Period{Start: weekStart, End: weekStart.Add(7 * 24 * time.Hour)}

	// Events on 3 of the 7 days.
	events := []omnidevx.Event{
		historyEvent("m:1", omnidevx.EventMessageCompleted, weekStart.Add(time.Hour), claudeSource, "s1", nil),
		historyEvent("m:2", omnidevx.EventMessageCompleted, weekStart.Add(25*time.Hour), claudeSource, "s2", nil),
		historyEvent("m:3", omnidevx.EventMessageCompleted, weekStart.Add(49*time.Hour), claudeSource, "s3", nil),
	}
	report := Build(events, Subject{PersonID: "person:john"}, period)

	want := 3.0 / 7.0
	if got := report.Quality.CoverageScore; got != want {
		t.Errorf("coverage score: got %v, want %v", got, want)
	}
}

func TestBuildUnboundedPeriodWarns(t *testing.T) {
	report := Build(nil, Subject{PersonID: "person:john"}, omnidevx.Period{})
	if report.Quality.CoverageScore != 0 {
		t.Errorf("unbounded period coverage score: got %v, want 0", report.Quality.CoverageScore)
	}
	if len(report.Quality.Warnings) == 0 {
		t.Error("expected a warning for an unbounded period")
	}
}

func TestBuildToolFailureCounts(t *testing.T) {
	day := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	events := []omnidevx.Event{
		historyEvent("tc:1", omnidevx.EventToolCompleted, day.Add(time.Hour), claudeSource, "s1",
			map[string]any{omnidevx.AttrSuccess: true}),
		historyEvent("tc:2", omnidevx.EventToolCompleted, day.Add(2*time.Hour), claudeSource, "s1",
			map[string]any{omnidevx.AttrSuccess: false}),
	}
	period := omnidevx.Period{Start: day, End: day.Add(24 * time.Hour)}
	report := Build(events, Subject{PersonID: "person:john"}, period)

	if got := report.Metrics.Combined["tool_calls"].Value; got != 2 {
		t.Errorf("tool_calls: got %v, want 2", got)
	}
	if got := report.Metrics.Combined["tool_calls_failed"].Value; got != 1 {
		t.Errorf("tool_calls_failed: got %v, want 1", got)
	}
}

func TestBuildDailyIgnoresEventsOutsideDay(t *testing.T) {
	day := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	events := []omnidevx.Event{
		historyEvent("in", omnidevx.EventPromptSubmitted, day.Add(time.Hour), claudeSource, "s1", nil),
		historyEvent("out", omnidevx.EventPromptSubmitted, day.Add(25*time.Hour), claudeSource, "s1", nil),
	}
	summary := BuildDaily(events, day)
	if got := summary.Combined["prompts"]; got != 1 {
		t.Errorf("prompts: got %v, want 1 (event outside the day should be excluded)", got)
	}
}

func TestBuildConfidenceReflectsLowestObservedSource(t *testing.T) {
	day := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	lowConfidence := omnidevx.Event{
		ID: "m:1", Type: omnidevx.EventMessageCompleted, Timestamp: day.Add(time.Hour),
		Source: claudeSource, Context: omnidevx.EventContext{SessionID: "s1"},
		Provenance: omnidevx.Provenance{CollectionMode: omnidevx.ModeHistory, Confidence: 0.5},
	}
	events := []omnidevx.Event{lowConfidence}
	period := omnidevx.Period{Start: day, End: day.Add(24 * time.Hour)}
	report := Build(events, Subject{PersonID: "person:john"}, period)

	if got := report.Metrics.Combined["messages"].Measurement.Confidence; got != 0.5 {
		t.Errorf("confidence: got %v, want 0.5", got)
	}
}
