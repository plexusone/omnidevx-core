// Package report builds DeveloperPeriodReport (schema
// omnidevx.developer-period/v1) from canonical events.
//
// Aggregation is two-stage: BuildDaily reduces one day's events into a
// DailySummary; Rollup combines summaries covering a period into a report.
// Build is the one-call convenience that buckets events by day and runs
// both stages. The two-stage split matters because not every metric sums
// correctly across a day boundary — a session spanning midnight must be
// deduplicated by ID at rollup time, not double-counted per day — and
// because reprocessing with a changed formula should replay from stored
// daily summaries, never require recollection from raw session files.
//
// Only additive, per-day event types are summarized here: ai.* session
// activity, devx.change.committed, and devx.contribution.recorded.
// Period-total events (devx.profile.snapshot, devx.contribution.snapshot)
// already describe an entire period rather than one day, so they are not
// decomposed into daily buckets; Rollup surfaces their presence as a
// DataQuality warning until a provider-specific merge rule is defined.
//
// Metrics are split into combined and bySource views per source ("some
// metrics combine safely — sessions, cost, tokens with model retained;
// others do not — acceptance rate only with matching definitions"); every
// metric carries a Measurement recording whether it was observed or
// estimated, and from what confidence.
package report
