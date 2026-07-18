// Package claudecode collects developer-experience events from Claude Code's
// local session history (~/.claude/projects/<project>/<session>.jsonl).
//
// The session files are an internal Claude Code format, not a stable public
// contract: parsing is defensive, unrecognized records are skipped, and all
// events carry history-mode provenance with reduced confidence.
//
// Only metadata is extracted — models, token usage, tool names, timestamps,
// workspace and branch identifiers. Prompt text, assistant output, and tool
// results are never read into events.
package claudecode
