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
