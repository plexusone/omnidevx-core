# Privacy Model

OmniDevX collects **metadata only**. Canonical events carry event types,
durations, counts, models, token and cost figures, and repository/branch
identifiers — never:

- prompt text or assistant responses
- tool inputs, outputs, stdout/stderr
- file contents or diffs
- commit subjects or bodies (hashes and attribution only)
- OTel log/event bodies (`POST /v1/logs` is acknowledged and discarded)

## Enforcement

The rule is structural, not aspirational:

- Provider parsers never decode content fields — the wire structs simply
  omit them.
- Test suites assert that content-like attribute keys (`content`, `text`,
  `stdout`, `arguments`, `message`, …) fail the build if they ever appear
  in events.
- Resource attributes such as `user.email` from OTel exporters are read
  for routing but never stored on events.

## Local by default

The event store lives on the developer's machine with owner-only
permissions. Individual raw events are designed never to leave it: team
aggregation (when it arrives) consumes shared **period reports** — already
metadata-level summaries — not raw events.

Collection-side privacy and publication-side disclosure are separate
concerns: what a portfolio publishes is governed by explicit publication
profiles downstream (see the DevFolio specs), while this module guarantees
there is no content to leak in the first place.
