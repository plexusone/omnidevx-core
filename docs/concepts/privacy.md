# Privacy Model

OmniDevX collects **metadata only**. Canonical events carry event types,
durations, counts, models, token and cost figures, and repository/branch
identifiers — never:

- prompt text or assistant responses
- tool inputs, outputs, stdout/stderr
- file contents or diffs
- commit subjects or bodies (hashes and attribution only)
- OTel log/event bodies (`POST /v1/logs` is acknowledged and discarded)

This applies to everything that produces events: collectors, the event
store, and reports. The one place content is read is the
[session catalog](sessions.md), which is separate from the event stream
and described [below](#session-catalog-the-one-place-content-is-read).

## Enforcement

The rule is structural, not aspirational:

- Provider parsers never decode content fields — the wire structs simply
  omit them.
- Test suites assert that content-like attribute keys (`content`, `text`,
  `stdout`, `arguments`, `message`, …) fail the build if they ever appear
  in events.
- Resource attributes such as `user.email` from OTel exporters are read
  for routing but never stored on events.

## Session catalog: the one place content is read

The `sessions` package lists coding-agent sessions so a developer can tell
them apart and resume the right one. That needs titles and the prompts the
developer typed, so its readers do decode them. The guarantees are narrower
than "metadata only" and are kept structurally separate from the event
stream:

- **Never in events.** Session readers do not produce `Event` values, and
  nothing they return is written to the event store.
- **On demand and in memory.** Titles and prompts are read when a command
  asks, held in memory, and printed. They are not cached on disk by
  OmniDevX.
- **Local only.** Readers make no network calls and read only the harness's
  own local files.
- **Suppressible.** `NoContent` removes titles derived from prompts and all
  recent prompts from a listing, falling back to the directory name.
- **Evidence is structural by design.** The evidence model, which the
  readers do not populate yet, records what a session touched (repositories,
  files, commits) from the harness's structured tool records and from git,
  not from text the model wrote. Work-reference IDs are matched in the
  prompts the developer typed, branch names, commit messages, and paths.
- **Prompt text is truncated.** Prompts kept on a session are cut to a short
  length for display.

The event-store tests that reject content-like attribute keys still apply
unchanged, because the catalog does not use the event types.

## Local by default

The event store lives on the developer's machine with owner-only
permissions. Individual raw events are designed never to leave it: team
aggregation (when it arrives) consumes shared **period reports** — already
metadata-level summaries — not raw events.

Collection-side privacy and publication-side disclosure are separate
concerns: what a portfolio publishes is governed by explicit publication
profiles downstream (see the DevFolio specs), while this module guarantees
there is no content to leak in the first place.
