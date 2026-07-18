# OmniDevX Core

Canonical event model and collectors for **OmniDevX** — the PlexusOne
developer-experience telemetry domain. Events from AI coding agents and
development tools are normalized into one vendor-neutral representation so
frameworks such as SPACE, AI SPACE, and DORA can be computed over a single
substrate.

!!! note "Not OmniDXI"
    [`omnidxi`](https://github.com/plexusone/omnidxi) is Digital Experience
    Intelligence — product analytics (Amplitude, Mixpanel, Heap, Pendo).
    OmniDevX observes how developers and AI agents build software. The two
    domains are unrelated despite the similar names.

## Packages

| Package | Purpose |
|---------|---------|
| `omnidevx` (root) | Canonical `Event` IR, `Collector` contract, periods, provenance |
| `store` | Local JSONL event store: idempotent, inspectable, reprocessable |
| `providers/claudecode` | Claude Code session-history importer (stdlib only) |
| `providers/git` | Git commit collector with AI co-author attribution |
| `providers/genericotel` | OTLP/JSON receiver for live token/cost metrics |

Thick providers with heavy dependencies live in vendor repos: the Codex CLI
collector in `omni-openai/omnidevx` (SQLite driver), the GitHub collector
in `omni-github/omnidevx`.

## Install

```bash
go get github.com/plexusone/omnidevx-core
```

## Principles

1. **Collectors normalize; they never compute metrics.** Frameworks are
   downstream projections over canonical events.
2. **Metadata only.** Prompt text, model responses, and file contents are
   never captured. See [Privacy Model](concepts/privacy.md).
3. **Provenance on everything.** Every event records how it was collected
   (`history`, `otel`, `hooks`, `api`, `survey`) and with what confidence.
4. **Deterministic IDs.** Re-importing the same history deduplicates
   naturally in the store.
