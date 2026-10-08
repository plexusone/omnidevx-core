# Architecture

How `omnidevx-core` is built today: the packages and what may depend on what,
the two separate read paths, and the constraints that keep it small. This page
describes current behavior. For how to use it, see the [guides](../guides/getting-started.md).

## Role

`omnidevx-core` is the vendor-neutral foundation of OmniDevX, PlexusOne's
developer-experience telemetry domain. It defines the canonical event model,
the collector contract, the event store, the report pipeline, and the session
catalog. Thick integrations live in the repositories that own the vendor
formats; `omnidevx` composes them.

| Repository | Role |
|------------|------|
| `omnidevx-core` (this repo) | Canonical types, contracts, store, reports, session catalog, and the stdlib-only Claude Code and git providers |
| [`omni-openai`](https://github.com/plexusone/omni-openai) | Codex CLI collector and session reader |
| [`omni-aws`](https://github.com/plexusone/omni-aws) | Kiro CLI collector |
| [`omni-github`](https://github.com/plexusone/omni-github) | GitHub contribution collector |
| [`omnidevx`](https://github.com/plexusone/omnidevx) | Batteries-included distribution and CLI |
| [`devfolio`](https://github.com/plexusone/devfolio) | Consumes period reports for dashboards |

## Two read paths

The same local harness files are read two ways, under different contracts.

```text
                      ┌─→ Collector ─→ Event ─→ store ─→ report      metadata only
Harness files ────────┤
                      └─→ SessionReader ─→ Session ─→ Catalog        titles and typed prompts
```

- **Events** are metadata only: types, timestamps, counts, models, token and
  cost figures, repository identifiers. The `Event` type has no content
  fields, and the Claude collector's tests fail if content-like attribute
  keys (such as `content`, `text`, or `stdout`) appear on its events.
- **Sessions** exist so a person can recognize and resume a session. Readers
  decode titles and the prompts a person typed. That content is held in
  memory, never written to the event store, and never turned into an `Event`.

Keeping the paths separate is the design, not an accident of layout: the
event stream can be shared and aggregated precisely because it carries no
content, and the catalog can read content precisely because it stays local.
See the [Privacy Model](../guides/concepts/privacy.md).

## Packages and dependency direction

| Package | Purpose | Depends on |
|---------|---------|------------|
| `omnidevx` (root) | `Event`, `Collector`, `Period`, `Source`, `Provenance`, attribute keys | stdlib only |
| `identity` | Resolves usernames, emails, and device accounts to a person | stdlib only |
| `store` | Daily-file JSONL event store, idempotent writes | root |
| `report` | Daily summaries, period rollups, model pricing and cost estimation | root |
| `sessions` | `Session`, `Reader`, `Catalog`, `Evidence`, repository index, work references, config | root |
| `sessions/schema` | Generated, embedded JSON Schema for `sessions.Session` | nothing |
| `providers/claudecode` | Claude Code event collector and session reader | root, `sessions` |
| `providers/git` | Git commit collector with AI co-author attribution | root, `gogit` |
| `providers/genericotel` | OTLP/JSON metrics receiver | root, `store` |

The rules this graph encodes:

- **The root package depends on nothing.** Everything else may import it;
  it imports no sibling package.
- **`gogit` is the only third-party dependency** of the module, used by the
  git provider. Everything else is the standard library. Integrations that
  need a driver or an API client belong in their own repository, not here.
- **Providers do not depend on each other.** They depend on the root, and
  the two that need more depend on exactly one more package (`sessions` for
  the session reader, `store` for the receiver).
- **`report` and `store` never import providers.** Collection and analysis
  stay decoupled; the only contract between them is the `Event`.

## Event pipeline

1. A `Collector` reads a source for a `Period` and returns a `CollectionResult`
   of normalized events plus `Diagnostic`s for anything it could not parse.
   Collectors normalize; they never compute framework metrics.
2. `store` writes events to daily files, `<dir>/events/YYYY/MM/DD/<product>.jsonl`.
   Event IDs are deterministic per source record, so re-importing overlapping
   history deduplicates.
3. `report` builds daily summaries and rolls them up into a
   `DeveloperPeriodReport`: combined, per-source, and per-model metrics, source
   coverage, and data-quality warnings. Cost not reported by a source is
   estimated from an embedded pricing table and labelled as estimated.

## Session subsystem

A `Reader` lists one harness's sessions cheaply: file metadata, indexes, and
bounded reads of a transcript's start and end, never a full parse. A `Catalog`
merges readers newest-first and keeps one failing reader from hiding the rest.
Readers return what to run to resume (`ResumeSpec`) and leave running it to
the caller.

Alongside the readers, the package provides the pieces that turn a listing
into understanding:

- **`RepoIndex`** maps a path to its git repository, deriving a stable ID from
  the origin remote by reading the git config directly.
- **`WorkRefExtractor`** finds work-tracking identifiers using named,
  validated rules.
- **`Evidence`** is the data model for what a session touched. It is a lower
  bound by construction, and the readers do not populate it yet.
- **`Config`** loads optional settings from `~/.plexusone/omnidevx/config.json`.
- **`sessions/schema`** publishes the session document's contract, generated
  from the Go structs, which stay the source of truth.

## Cross-cutting conventions

- **Failures are data.** Parse problems become diagnostics, and one failing
  source returns partial results with an error naming it.
- **Wire naming follows the surface.** Document formats, including the session
  schema and config, use camelCase. Event attribute keys are snake_case, which
  matches the model vendors' own field names.
- **Go structs are the source of truth** for every published schema. Schemas
  are generated, committed, and guarded by tests against drift.

## Data paths

| Path | Contents |
|------|----------|
| `~/.plexusone/omnidevx/data/` | JSONL event store |
| `~/.plexusone/omnidevx/reports/` | Generated period reports |
| `~/.plexusone/omnidevx/config.json` | Optional session-catalog settings |
| `~/.claude/projects/`, `~/.claude/sessions/` | Claude Code transcripts and live-process records, read by the Claude provider |

## Extending

To add an event source, implement `Collector` in the repository that owns the
format, emit the shared attribute keys with deterministic IDs and honest
provenance, and add tests with fabricated fixtures. To add a session source,
implement `sessions.Reader` the same way, and add a case to the shared reader
contract test in `omnidevx`.
