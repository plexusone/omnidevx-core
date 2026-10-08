# Canonical Events

Every observation is an `Event` (schema `omnidevx.event/v1`):

```go
type Event struct {
	ID         string         // deterministic per source record
	Type       EventType      // ai.* or devx.* namespace
	Timestamp  time.Time
	Subject    SubjectRef     // whose activity (personId + accounts)
	Source     Source         // provider/product/version, canonical names
	Context    EventContext   // sessionId, repository, workspace, gitBranch
	Attributes map[string]any // metadata only — never content
	Provenance Provenance     // collectionMode + confidence
}
```

## Event types

**`ai.*` — coding-agent session activity:**

| Type | Meaning |
|------|---------|
| `ai.session.started` / `ai.session.ended` | Session span bounds |
| `ai.prompt.submitted` | A human prompt (no text captured) |
| `ai.message.completed` | One assistant turn, with model + token usage |
| `ai.tool.completed` | Tool execution, with tool name and success |
| `ai.patch.applied` | A patch entered the working tree |
| `ai.task.started` / `ai.task.completed` | Agent task spans, with durations |
| `ai.usage.recorded` | Token/cost usage reported separately from messages |

**`devx.*` — development-work semantics:**

| Type | Meaning |
|------|---------|
| `devx.change.committed` | One git commit, with AI attribution |
| `devx.contribution.recorded` | One day of platform-reported contributions |
| `devx.contribution.snapshot` | Per-repository period summary |
| `devx.profile.snapshot` | Whole-profile period summary |

## Shared attribute keys

Providers use the exported `Attr*` constants (`AttrModel`,
`AttrInputTokens`, `AttrOutputTokens`, `AttrCacheReadTokens`, `AttrTool`,
`AttrSuccess`, `AttrDurationMS`, `AttrAIAssisted`, `AttrAITools`,
`AttrInsertions`, `AttrDeletions`, `AttrCostUSD`, …) so metrics compute
across sources without per-provider key mapping.

## Provenance

```go
type Provenance struct {
	CollectionMode CollectionMode // history | otel | hooks | api | survey
	Confidence     float64        // 1.0 = directly observed
}
```

Historical reconstruction (parsing local session files) is inherently less
certain than live observation; downstream metrics must be able to
distinguish observed values from reconstruction. Collectors set reduced
confidence for history mode.

## Deterministic IDs

Event IDs are stable functions of the source record (a session record
UUID, a commit hash, an OTLP datapoint timestamp). Re-importing overlapping
history produces identical IDs, and the [store](store.md) deduplicates on
them — including the same commit reached through multiple local clones.
