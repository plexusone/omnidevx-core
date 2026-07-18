# OmniDevX Core

Core interfaces and canonical types for **OmniDevX** — the PlexusOne
developer-experience telemetry domain. OmniDevX collects and normalizes
events from AI coding agents and development tools so frameworks such as
SPACE, AI SPACE, and DORA can be computed over one vendor-neutral event
model.

> **Not OmniDXI.** [`omnidxi`](https://github.com/plexusone/omnidxi) is
> Digital Experience Intelligence — a facade over product-analytics
> platforms (Amplitude, Mixpanel, Heap, Pendo). OmniDevX observes how
> developers and AI agents build software. The two domains are unrelated.

## Layout

- Root package `omnidevx` — canonical `Event` IR, `Collector` contract,
  periods, provenance.
- `providers/claudecode` — thin provider reading Claude Code local session
  history (`~/.claude/projects/`). Stdlib only.

Thick providers with heavy dependencies live in vendor repos, e.g. the
Codex CLI collector in
[`omni-openai/omnidevx`](https://github.com/plexusone/omni-openai)
(requires a SQLite driver).

## Privacy

Canonical events carry **metadata only** — event types, durations, counts,
models, token figures, and repository identifiers. Prompt text, model
responses, and file contents are never captured by default.

## Usage

```go
collector, err := claudecode.New(claudecode.Options{})
if err != nil {
    // handle
}
result, err := collector.Collect(ctx, omnidevx.CollectRequest{
    Period:  omnidevx.Period{Start: weekStart, End: weekEnd},
    Subject: omnidevx.SubjectRef{PersonID: "person:jane"},
})
```

Provenance on every event records the collection mode (`history`, `otel`,
`hooks`, `api`, `survey`) and a confidence score, so downstream metrics can
distinguish observed values from historical reconstruction.

## Specifications

Ecosystem PRD/TRD/PLAN/ROADMAP currently reside in
[`devfolio/docs/specs`](https://github.com/plexusone/devfolio/tree/main/docs/specs)
and will migrate here.

## License

MIT
