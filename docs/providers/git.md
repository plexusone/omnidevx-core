# Git Provider

`providers/git` emits one `devx.change.committed` event per commit across
configured repository roots, built on
[`github.com/grokify/gogit`](https://github.com/grokify/gogit).

```go
c, err := git.New(git.Options{
	Roots:    []string{"/Users/jane/go/src/github.com/myorg"},
	MaxDepth: 1,    // directory levels below each root (default 3)
	NoMerges: false,
})
```

## Event attributes

| Attribute | Meaning |
|-----------|---------|
| `commit_hash` | Full hash (also keys the event ID: same commit in two clones dedups) |
| `author_email` | Author identity, for downstream identity resolution |
| `insertions` / `deletions` / `files_changed` | `--numstat` change stats |
| `ai_assisted` | Any `Co-authored-by` trailer matches a known AI tool |
| `ai_tools` | Which tools (present only when assisted) |

Context carries the repository (normalized origin URL), workspace path,
and current branch. Commit subjects and bodies are never included.

## AI attribution

`KnownAITools` is the canonical registry of AI coding assistants and
their co-author trailer emails:

| Tool | Signature emails |
|------|-----------------|
| Claude Code | `noreply@anthropic.com` |
| GitHub Copilot | `noreply@github.com`, `copilot@github.com` |
| Gemini CLI | `*+gemini-cli@users.noreply.github.com` and variants |
| Cursor | `ai@cursor.sh`, `cursor@cursor.sh` |
| Aider | `aider@aider.chat` |

Matching is case-insensitive with GitHub-noreply suffix support
(`AIToolByEmail`). Trailer-based attribution **understates** AI
involvement — tools that don't sign their work are invisible — which is
why events carry history-mode provenance rather than claiming certainty.

## Caveats

- Local clones only: a stale clone understates activity relative to the
  hosting platform (pair with the GitHub provider in `omni-github`).
- The branch on the event is the repo's *current* branch, an
  approximation for commits made on other branches.
