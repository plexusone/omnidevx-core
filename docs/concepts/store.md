# Event Store

The `store` package persists canonical events as daily JSONL files:

```text
~/.plexusone/omnidevx/data/
└── events/
    └── 2026/07/17/
        ├── claude-code.jsonl
        ├── codex-cli.jsonl
        └── git.jsonl
```

One file per source product per day, owner-only permissions (0700
directories, 0600 files).

## Why JSONL, not a database

- **Inspectable.** A developer can `grep` exactly what has been recorded
  about them — central to the [privacy model](privacy.md).
- **Reprocessable.** Raw events are the durable source of truth; when
  metric formulas change, reports regenerate without recollection.
- **Dependency-light.** No driver in core. A derived SQLite/DuckDB index
  remains a reserved escape hatch if query patterns demand it — always
  regenerable, never authoritative.

## Writing

```go
s, _ := store.Open(store.Options{}) // default root
result, err := s.Write(ctx, events)
// result.Written, result.Duplicates
```

Writes are **idempotent**: an event whose deterministic ID already exists
in its day file is counted as a duplicate and skipped. Events missing an
ID, type, or timestamp are rejected.

## Reading

```go
read, err := s.Read(ctx, store.Query{
	Period:  omnidevx.Period{Start: start, End: end}, // half-open
	Product: "claude-code",                            // optional
})
// read.Events (timestamp-ordered), read.Diagnostics
```

Day directories entirely outside the period are pruned without opening
files. Damaged lines (e.g. a torn final line from an interrupted write)
surface as diagnostics rather than silent drops, and the next write to
that file dedups against the intact lines and appends normally.
