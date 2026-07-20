package report

import (
	"fmt"
	"sort"
	"time"

	omnidevx "github.com/plexusone/omnidevx-core"
)

// Build reduces events into a DeveloperPeriodReport for one subject and
// period. It buckets events by UTC calendar day, reduces each day with
// BuildDaily, and rolls the summaries up with Rollup — the same path a
// caller replaying cached DailySummary artifacts would take, so
// reprocessing with a changed formula never requires recollection.
func Build(events []omnidevx.Event, subject Subject, period omnidevx.Period) *DeveloperPeriodReport {
	byDay := map[time.Time][]omnidevx.Event{}
	for _, e := range events {
		if !period.Contains(e.Timestamp) {
			continue
		}
		day := e.Timestamp.UTC().Truncate(24 * time.Hour)
		byDay[day] = append(byDay[day], e)
	}

	dailies := make([]*DailySummary, 0, len(byDay))
	for day, dayEvents := range byDay {
		dailies = append(dailies, BuildDaily(dayEvents, day))
	}
	return Rollup(dailies, subject, period)
}

// Rollup combines DailySummary values into one DeveloperPeriodReport.
// Summaries outside period are ignored so callers can pass a cached
// superset without re-filtering first.
func Rollup(dailies []*DailySummary, subject Subject, period omnidevx.Period) *DeveloperPeriodReport {
	combined := map[string]float64{}
	bySource := map[string]map[string]float64{}
	sessionIDs := map[string]bool{}
	sessionIDsBySource := map[string]map[string]bool{}
	sources := map[string]*sourceAccum{}
	snapshotCount := 0
	dayCount := 0

	for _, d := range dailies {
		if d == nil || !period.Contains(d.Date) {
			continue
		}
		dayCount++
		snapshotCount += d.Snapshots

		for metric, v := range d.Combined {
			combined[metric] += v
		}
		for src, metrics := range d.BySource {
			if bySource[src] == nil {
				bySource[src] = map[string]float64{}
			}
			for metric, v := range metrics {
				bySource[src][metric] += v
			}
		}
		for src, ids := range d.sessionIDs {
			if sessionIDsBySource[src] == nil {
				sessionIDsBySource[src] = map[string]bool{}
			}
			for id := range ids {
				sessionIDsBySource[src][id] = true
				sessionIDs[id] = true
			}
		}
		for src, acc := range d.sources {
			if sources[src] == nil {
				sources[src] = newSourceAccum(acc.source)
			}
			sources[src].merge(acc)
		}
	}

	// Sessions are deduplicated across day boundaries, not summed per day.
	// Only reported where session-scoped activity was actually observed —
	// like every other metric, absence means no entry, not a manufactured
	// zero borrowing confidence from an unrelated source.
	if len(sessionIDs) > 0 {
		combined["sessions"] = float64(len(sessionIDs))
	}
	for src, ids := range sessionIDsBySource {
		if len(ids) == 0 {
			continue
		}
		if bySource[src] == nil {
			bySource[src] = map[string]float64{}
		}
		bySource[src]["sessions"] = float64(len(ids))
	}

	return &DeveloperPeriodReport{
		SchemaVersion: SchemaVersion,
		Subject:       subject,
		Period:        period,
		Sources:       coverageList(sources),
		Metrics:       buildMetricSet(combined, bySource, sources),
		Quality:       quality(sources, dayCount, period, snapshotCount),
	}
}

func buildMetricSet(combined map[string]float64, bySource map[string]map[string]float64, sources map[string]*sourceAccum) MetricSet {
	ms := MetricSet{
		Combined: map[string]Metric{},
		BySource: map[string]map[string]Metric{},
	}
	for src, metrics := range bySource {
		conf := 1.0
		if acc, ok := sources[src]; ok && acc.hasConfidence {
			conf = acc.minConfidence
		}
		out := map[string]Metric{}
		for metric, v := range metrics {
			out[metric] = Metric{
				Value:       v,
				Measurement: Measurement{Kind: KindObserved, Method: "sum", Confidence: conf},
			}
		}
		ms.BySource[src] = out
	}
	overallConf := minConfidenceAcross(sources)
	for metric, v := range combined {
		if !combinedMetricKeys[metric] {
			continue
		}
		ms.Combined[metric] = Metric{
			Value:       v,
			Measurement: Measurement{Kind: KindObserved, Method: "sum", Confidence: overallConf},
		}
	}
	return ms
}

func minConfidenceAcross(sources map[string]*sourceAccum) float64 {
	min := 1.0
	found := false
	for _, acc := range sources {
		if acc.hasConfidence && (!found || acc.minConfidence < min) {
			min = acc.minConfidence
			found = true
		}
	}
	return min
}

func coverageList(sources map[string]*sourceAccum) []SourceCoverage {
	keys := make([]string, 0, len(sources))
	for k := range sources {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	list := make([]SourceCoverage, 0, len(keys))
	for _, k := range keys {
		list = append(list, sources[k].coverage())
	}
	return list
}

func quality(sources map[string]*sourceAccum, dayCount int, period omnidevx.Period, snapshotCount int) DataQuality {
	var warnings []string
	if snapshotCount > 0 {
		warnings = append(warnings, fmt.Sprintf("%d period-total snapshot event(s) present but not yet merged into this report", snapshotCount))
	}
	if len(sources) == 0 {
		warnings = append(warnings, "no events observed for this subject and period")
	}

	expected := expectedDays(period)
	if expected == 0 {
		warnings = append(warnings, "period has no bounded start and end; coverage score is not meaningful")
		return DataQuality{CoverageScore: 0, Warnings: warnings}
	}
	score := float64(dayCount) / float64(expected)
	if score > 1 {
		score = 1
	}
	return DataQuality{CoverageScore: score, Warnings: warnings}
}

// expectedDays counts the calendar days covered by the half-open period
// [Start, End). Returns 0 for an unbounded period.
func expectedDays(p omnidevx.Period) int {
	if p.Start.IsZero() || p.End.IsZero() {
		return 0
	}
	start := p.Start.UTC().Truncate(24 * time.Hour)
	end := p.End.UTC()
	count := 0
	for d := start; d.Before(end); d = d.Add(24 * time.Hour) {
		count++
	}
	return count
}
