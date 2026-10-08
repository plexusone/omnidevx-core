# Sessions

Coding agents such as Claude Code and Codex CLI keep each conversation as a
resumable session on disk. After a reboot, or once a dozen terminals have
piled up, the hard part is not resuming a session but recognizing which one
you want. The `sessions` package builds one catalog across harnesses with
enough context to decide, and says how to resume the one you pick.

```text
DISCOVER → UNDERSTAND → SELECT → RESUME
```

The catalog is a separate read path from the [event stream](events.md). It
reads titles and prompts, which events never carry; see
[Privacy Model](privacy.md#session-catalog-the-one-place-content-is-read).

## A session

A `Session` is identified by its harness and the harness's own ID, which
survive process exit and reboot. Process IDs and terminals are runtime
details, not identity.

| Field | Meaning |
|-------|---------|
| `harness`, `id` | Identity: `claude-code` or `codex`, and the harness's session ID |
| `cwd` | Directory the session started in. Resuming must happen from here |
| `createdAt`, `lastActivityAt` | When it started and when anything last happened |
| `lastHumanActivityAt` | When a person last typed a prompt |
| `title`, `titleSource` | A name for the session and where it came from |
| `recentPrompts` | The latest prompts a person typed, truncated |
| `state`, `runtime` | `running`, `resumable`, or `unknown`, and the live process if any |
| `resume` | The command and directory that resume it |
| `evidence` | What the session touched; populated on request |

`lastHumanActivityAt` is kept apart from `lastActivityAt` on purpose. A large
gap means the agent kept working unattended, which is the signal that tells
an interactive session from one that ran on its own.

The JSON form is described by a generated
[JSON Schema](https://github.com/plexusone/omnidevx-core/blob/main/sessions/schema/session.schema.json),
embedded in the `sessions/schema` package.

## Readers and the catalog

Each harness has a `Reader` that lists its sessions cheaply: it uses file
metadata, indexes, and bounded reads of the start and end of each transcript,
not a full parse. A `Catalog` merges readers newest-first. One reader failing
does not hide the others; the sessions that were read come back together with
an error naming each failure.

| Harness | Reader | Where it reads | Title | Running state |
|---------|--------|----------------|-------|---------------|
| Claude Code | `providers/claudecode` | `~/.claude/projects` | The harness's own title, else the first prompt | A live-process record whose process exists and started when the record says |
| Codex CLI | `omni-openai/omnidevx` | `~/.codex` state database and rollout files | Thread name, title, first message | Unknown unless a `codex` process names the thread on its command line |

A human prompt means a message a person typed. Tool results, injected
context, and subagent traffic do not count, even though harnesses record them
as user messages. Without that rule the last-human time would track the
agent's tool calls instead of the person.

The Claude reader trusts a live-process record only if the process is alive
and started when the record says it did. A record left behind by a crash or a
reboot can name a process ID that has since been reused. On Windows the
reader cannot read a process's start time, so it never reports a session as
running; check for an open Claude Code window before resuming.

## Resuming

A reader returns a `ResumeSpec`, the command and directory, rather than
running anything. The caller decides how to run it. A session that is already
running should be refused unless the caller overrides, because resuming it
would attach a second agent to the same conversation.

An ID can be given as any unique prefix of four or more characters, optionally
qualified with its harness, for example `codex:0199`. An ambiguous prefix
lists a few candidates and how many matched.

## Evidence

`Evidence` records what a session touched: repositories, files, commits, and
work-tracking IDs. It is a **lower bound**. A harness records reads and edits
made through its own tools, but a shell command can change files without
leaving a path to extract, so something missing from evidence was not
observed, not proven untouched.

The repository index and the work-reference extractor that feed it are
available now; the readers populate evidence in a later release.

### Repository index

`RepoIndex` maps a path to the git repository that holds it. It scans the
configured workspace roots to a bounded depth, skipping `node_modules`,
`vendor`, and hidden directories, and resolves a path by the deepest known
repository. A path outside every root is searched upward for a `.git` entry
and remembered. Repository IDs such as `github.com/org/repo` come from the
origin remote, read directly from the git config and normalized across https,
ssh, and scp-style remotes. Linked worktrees and submodules are followed.

### Work references

`WorkRefExtractor` finds work-tracking identifiers using named regular
expressions. The defaults match initiative IDs (`INIT-<SLUG>-NNN`) and
roadmap-item IDs (`RMI-<REPOSLUG>-NNN`). An identifier found in several
places keeps one entry that lists each place. An optional `WorkRefResolver`
supplies titles; nothing implements it by default.

## Configuration

Settings live in `~/.plexusone/omnidevx/config.json`. Every setting is
optional and a missing file gives the defaults. A file that is unreadable,
is not valid JSON, has an unknown key, or has an invalid rule is an error
that names the file.

```json
{
  "workspaceRoots": ["~/go/src"],
  "workRefRules": [
    { "name": "initiative", "pattern": "\\bINIT-[A-Z0-9]+-\\d{3,}\\b" },
    { "name": "rmi", "pattern": "\\bRMI-[A-Z0-9]+-\\d{3,}\\b" }
  ]
}
```

| Key | Meaning |
|-----|---------|
| `workspaceRoots` | Directories that hold repositories. A leading `~` is the home directory; other paths must be absolute |
| `workRefRules` | Replaces the default work-reference rules when present |
