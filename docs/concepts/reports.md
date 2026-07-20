# Period Reports

The `report` package builds a `DeveloperPeriodReport` (schema
`omnidevx.developer-period/v1`) from canonical events — the analytical
source of truth for one person's activity over one period. A contributor
profile summarizes and links to this; it never recomputes it.

## Two-stage aggregation

```go
r := report.Build(events, report.Subject{PersonID: "person:john"}, period)
```

`Build` buckets events by UTC calendar day, reduces each day with
`BuildDaily` into a `DailySummary`, then combines summaries with `Rollup`.
The two stages exist separately because not every metric sums correctly
across a day boundary — a coding session spanning midnight must be
deduplicated by session ID at rollup time, not double-counted per day — and
because reprocessing with a changed formula should replay from cached daily
summaries, never require recollection from raw session files:

```go
daily := report.BuildDaily(dayEvents, day)          // per day, cacheable
r := report.Rollup(dailies, subject, period)         // pure function of the summaries
```

## Combined vs. bySource

Not every metric compares safely across sources. Sessions, tokens (with
model/provider retained), cost, and commit counts combine; autonomous task
completion does not, without cross-source normalization first. `MetricSet`
keeps both views:

```json
{
  "metrics": {
    "combined": { "commits": { "value": 96, "measurement": {"kind": "observed", "confidence": 0.9} } },
    "bySource": {
      "anthropic/claude-code": { "messages": {"value": 8696, "...": "..."} },
      "git/git":                { "commits": {"value": 96, "...": "..."} }
    }
  }
}
```

A metric that doesn't combine safely (e.g. `tasks`) appears in `bySource`
only. Every `Metric` carries a `Measurement` — `kind` (`observed` here;
`estimated` metrics belong to a future analytics layer built on top of
these reports), the method, and a confidence drawn from the least-confident
event that contributed to it.

## Sources and data quality

```go
r.Sources  // []SourceCoverage: which sources contributed, how many events,
           // which collection modes, minimum confidence
r.Quality  // CoverageScore (active days / days in period) + Warnings
```

Period-total events (`devx.profile.snapshot`, `devx.contribution.snapshot`)
describe an entire period rather than one day, so they are not decomposed
into daily buckets — they're counted in `Sources` coverage, but their
presence surfaces as a `Quality.Warnings` entry until a provider-specific
merge rule is defined, rather than being silently dropped or double-counted
by naive daily summation.

## Identity

`DeveloperPeriodReport.Subject.PersonID` is the canonical identity a report
is built for — never a raw GitHub username or git email. See
[Identity](identity.md) for resolving multiple accounts to one person.
