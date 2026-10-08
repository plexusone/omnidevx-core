# Claude Code Provider

`providers/claudecode` imports Claude Code's local session history —
per-project JSONL transcripts under `~/.claude/projects/` — into canonical
events. Stdlib only.

```go
c, err := claudecode.New(claudecode.Options{
	Dir: "", // default ~/.claude
})
result, err := c.Collect(ctx, omnidevx.CollectRequest{Period: week})
```

## Events emitted

| Event | From |
|-------|------|
| `ai.session.started` / `ai.session.ended` | First/last record per session, with client version |
| `ai.prompt.submitted` | Human prompts (sidechain/subagent prompts excluded) |
| `ai.message.completed` | Assistant turns: model + input/output/cache-read/cache-creation tokens |
| `ai.tool.completed` | Tool results, with tool name attributed via `tool_use` → `tool_result` ID correlation and success from `is_error` |

Context carries the session ID, workspace path, and git branch.

## Characteristics and limits

- **Internal format.** Session files are not a stable Claude Code
  contract; parsing is defensive, unparseable lines become diagnostics,
  and provenance is `history` mode at reduced confidence.
- **Retention.** Claude Code prunes local session history (weeks, not
  months). Historical import cannot reach past retention — pair with the
  [OTel receiver](genericotel.md) for durable live capture.
- **Resumed sessions duplicate records.** Claude Code copies conversation
  prefixes into resumed/branched session files (~3% duplicate IDs in
  practice); the store's ID dedup absorbs this by design.
- **No costs.** Token counts are captured; USD cost needs a pricing table
  (deliberately not maintained here) or the OTel cost metric.

## Session reader

Besides the collector above, the package has a `SessionReader` that lists
Claude Code sessions for the [session catalog](../concepts/sessions.md). It
is separate from the collector because it reads titles and the prompts a
person typed, which events never carry; see the
[Privacy Model](../concepts/privacy.md#session-catalog-the-one-place-content-is-read).

```go
r, err := claudecode.NewSessionReader(claudecode.Options{})
sessions, diags, err := r.List(ctx, sessions.ListOptions{})
```

- **Cheap listing.** Each transcript is read from its start (creation time,
  working directory, first prompt) and from the end (last activity, latest
  title, recent prompts). The end window grows while no human prompt is
  found, because a long agent run is mostly tool results.
- **Human prompts.** A record counts when Claude Code marks it as typed or
  queued. Older records without that marker are classified from their
  content: tool results, injected context, meta messages, and subagent
  records do not count.
- **Title.** The title Claude Code records for the session, else the first
  prompt, else the directory name.
- **Running state.** `~/.claude/sessions/<pid>.json` records a live process.
  A session is `running` only if that process exists and started when the
  record says it did, so a stale record that names a reused process ID is
  ignored. On Windows the start time cannot be read using only the standard
  library, so sessions are never reported as `running` there; they show as
  `resumable`, and you should check that Claude Code is not already open in
  that session before resuming it.
- **Resume.** `claude --resume <id>` from the session's original directory,
  because Claude Code finds a session by the project directory it started in.
