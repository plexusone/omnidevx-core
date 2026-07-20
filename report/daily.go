package report

import (
	"slices"
	"time"

	omnidevx "github.com/plexusone/omnidevx-core"
)

// combinedMetricKeys lists the metric keys computed here that combine
// safely across sources (see package doc's safe-to-combine notes) and
// therefore appear in MetricSet.Combined as well as BySource. Metrics
// omitted here (e.g. "tasks" — autonomous task completion needs
// cross-source normalization first) remain visible in BySource only.
var combinedMetricKeys = map[string]bool{
	"prompts":               true,
	"messages":              true,
	"tool_calls":            true,
	"tool_calls_failed":     true,
	"patches_applied":       true,
	"input_tokens":          true,
	"output_tokens":         true,
	"cache_read_tokens":     true,
	"cache_creation_tokens": true,
	"reasoning_tokens":      true,
	"total_tokens":          true,
	"cost_usd":              true,
	"commits":               true,
	"ai_assisted_commits":   true,
	"insertions":            true,
	"deletions":             true,
	"files_changed":         true,
	"contributions":         true,
	"sessions":              true, // dedup handled at Rollup, not summed per day
}

// sourceAccum tracks one source's contribution within a day or a rolled-up
// period.
type sourceAccum struct {
	source        omnidevx.Source
	eventCount    int
	modes         map[omnidevx.CollectionMode]bool
	minConfidence float64
	hasConfidence bool
}

func newSourceAccum(s omnidevx.Source) *sourceAccum {
	return &sourceAccum{source: s, modes: map[omnidevx.CollectionMode]bool{}}
}

func (a *sourceAccum) observe(p omnidevx.Provenance) {
	a.eventCount++
	a.modes[p.CollectionMode] = true
	if !a.hasConfidence || p.Confidence < a.minConfidence {
		a.minConfidence = p.Confidence
		a.hasConfidence = true
	}
}

func (a *sourceAccum) merge(other *sourceAccum) {
	a.eventCount += other.eventCount
	for m := range other.modes {
		a.modes[m] = true
	}
	if other.hasConfidence && (!a.hasConfidence || other.minConfidence < a.minConfidence) {
		a.minConfidence = other.minConfidence
		a.hasConfidence = true
	}
}

func (a *sourceAccum) coverage() SourceCoverage {
	modes := make([]omnidevx.CollectionMode, 0, len(a.modes))
	for m := range a.modes {
		modes = append(modes, m)
	}
	slices.Sort(modes)
	conf := a.minConfidence
	if !a.hasConfidence {
		conf = 1
	}
	return SourceCoverage{
		Source:          a.source,
		EventCount:      a.eventCount,
		CollectionModes: modes,
		MinConfidence:   conf,
	}
}

// DailySummary is one day's reduction of canonical events: additive metric
// counts plus enough per-source bookkeeping for Rollup to merge many days
// into a period report without re-reading raw events.
type DailySummary struct {
	Date       time.Time
	Combined   map[string]float64
	BySource   map[string]map[string]float64
	Snapshots  int // period-total events seen but not summarized (see package doc)
	sessionIDs map[string]map[string]bool
	sources    map[string]*sourceAccum
}

func sourceKey(s omnidevx.Source) string { return s.Provider + "/" + s.Product }

// BuildDaily reduces one day's events into a DailySummary. day identifies
// the UTC calendar day being summarized; events outside [day, day+24h) are
// ignored so callers can pass a pre-bucketed slice or the full event set
// interchangeably.
func BuildDaily(events []omnidevx.Event, day time.Time) *DailySummary {
	day = day.UTC().Truncate(24 * time.Hour)
	next := day.Add(24 * time.Hour)

	d := &DailySummary{
		Date:       day,
		Combined:   map[string]float64{},
		BySource:   map[string]map[string]float64{},
		sessionIDs: map[string]map[string]bool{},
		sources:    map[string]*sourceAccum{},
	}

	for _, e := range events {
		ts := e.Timestamp.UTC()
		if ts.Before(day) || !ts.Before(next) {
			continue
		}
		key := sourceKey(e.Source)
		if d.sources[key] == nil {
			d.sources[key] = newSourceAccum(e.Source)
		}
		d.sources[key].observe(e.Provenance)

		if isSnapshotEvent(e.Type) {
			d.Snapshots++
			continue
		}

		for metric, delta := range metricDeltas(e) {
			d.add(key, metric, delta)
		}
		if e.Context.SessionID != "" && isSessionScoped(e.Type) {
			if d.sessionIDs[key] == nil {
				d.sessionIDs[key] = map[string]bool{}
			}
			d.sessionIDs[key][e.Context.SessionID] = true
		}
	}
	return d
}

func (d *DailySummary) add(srcKey, metric string, delta float64) {
	d.Combined[metric] += delta
	if d.BySource[srcKey] == nil {
		d.BySource[srcKey] = map[string]float64{}
	}
	d.BySource[srcKey][metric] += delta
}

func isSnapshotEvent(t omnidevx.EventType) bool {
	return t == omnidevx.EventProfileSnapshot || t == omnidevx.EventContributionSnapshot
}

func isSessionScoped(t omnidevx.EventType) bool {
	switch t {
	case omnidevx.EventSessionStarted, omnidevx.EventSessionEnded, omnidevx.EventPromptSubmitted,
		omnidevx.EventMessageCompleted, omnidevx.EventToolCompleted, omnidevx.EventPatchApplied,
		omnidevx.EventTaskStarted, omnidevx.EventTaskCompleted, omnidevx.EventUsageRecorded:
		return true
	default:
		return false
	}
}

// metricDeltas returns the additive metric increments one event
// contributes.
func metricDeltas(e omnidevx.Event) map[string]float64 {
	deltas := map[string]float64{}
	switch e.Type {
	case omnidevx.EventPromptSubmitted:
		deltas["prompts"] = 1
	case omnidevx.EventMessageCompleted:
		deltas["messages"] = 1
		addTokenDeltas(deltas, e.Attributes)
	case omnidevx.EventUsageRecorded:
		addTokenDeltas(deltas, e.Attributes)
	case omnidevx.EventToolCompleted:
		deltas["tool_calls"] = 1
		if success, ok := e.Attributes[omnidevx.AttrSuccess].(bool); ok && !success {
			deltas["tool_calls_failed"] = 1
		}
	case omnidevx.EventPatchApplied:
		deltas["patches_applied"] = 1
	case omnidevx.EventTaskCompleted:
		deltas["tasks"] = 1
	case omnidevx.EventChangeCommitted:
		deltas["commits"] = 1
		if assisted, ok := e.Attributes[omnidevx.AttrAIAssisted].(bool); ok && assisted {
			deltas["ai_assisted_commits"] = 1
		}
		addFloatDelta(deltas, "insertions", e.Attributes[omnidevx.AttrInsertions])
		addFloatDelta(deltas, "deletions", e.Attributes[omnidevx.AttrDeletions])
		addFloatDelta(deltas, "files_changed", e.Attributes[omnidevx.AttrFilesChanged])
	case omnidevx.EventContributionRecorded:
		addFloatDelta(deltas, "contributions", e.Attributes[omnidevx.AttrContributions])
	}
	if cost, ok := numeric(e.Attributes[omnidevx.AttrCostUSD]); ok {
		deltas["cost_usd"] += cost
	}
	return deltas
}

func addTokenDeltas(deltas map[string]float64, attrs map[string]any) {
	pairs := []struct{ attr, metric string }{
		{omnidevx.AttrInputTokens, "input_tokens"},
		{omnidevx.AttrOutputTokens, "output_tokens"},
		{omnidevx.AttrCacheReadTokens, "cache_read_tokens"},
		{omnidevx.AttrCacheCreationTokens, "cache_creation_tokens"},
		{omnidevx.AttrReasoningTokens, "reasoning_tokens"},
		{omnidevx.AttrTotalTokens, "total_tokens"},
	}
	for _, p := range pairs {
		addFloatDelta(deltas, p.metric, attrs[p.attr])
	}
}

func addFloatDelta(deltas map[string]float64, metric string, v any) {
	if n, ok := numeric(v); ok {
		deltas[metric] += n
	}
}

// numeric converts an attribute value — set directly by a collector (any Go
// numeric kind) or decoded from stored JSON (always float64) — to float64.
func numeric(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}
