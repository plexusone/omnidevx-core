# Maintaining

How to work on `omnidevx-core`: the docs layout, the everyday checks, the
recurring tasks, and the release process. For how the code is organized, see
[Architecture](architecture.md).

## Documentation layout

Each directory under `docs/` has one purpose and its own rule for when it
changes.

| Path | Purpose | Update when |
|------|---------|-------------|
| `docs/index.md` | Site home and overview | The project's scope or principles change |
| `docs/guides/` | User documentation: getting started, concepts, provider guides | User-visible behavior changes (in the same change) |
| `docs/design/` | Living design documents: architecture and this page | Internals, boundaries, or process change (in the same change) |
| `docs/specs/` | Specifications for initiatives whose home repository is this one, at `docs/specs/initiatives/<INIT-ID>/` | Before and during the initiative; frozen after release |
| `docs/releases/` | Release notes, one file per tag | Each release |

Design documents describe current behavior only. Planned work belongs in a
specification, not in `docs/design/`. Every page must appear in the
`mkdocs.yml` navigation, and `mkdocs build --strict` must pass.

## Everyday checks

```bash
go test ./...            # add -race before a release
golangci-lint run
mkdocs build --strict    # docs
```

Tests use fabricated fixtures only. Never commit real session transcripts,
prompts, or paths from a personal machine; use placeholders such as
`/Users/example/...`.

## Dependencies

The module is standard-library only apart from `gogit`, which the git
provider uses. Keep it that way: an integration that needs a database driver
or a vendor API client belongs in its own repository. Verify the latest
version of any dependency before adding it.

## Recurring tasks

**Update model pricing.** Edit `report/pricing.json`, which is embedded with
`//go:embed`, and run the tests.

**Change a session type.** Every exported field in `sessions` needs a doc
comment, because the comments become the schema's property descriptions.
After changing a type, regenerate the embedded schema and commit it with the
change:

```bash
go install github.com/grokify/schemakit/cmd/schemakit@latest   # v0.6.0 or later
go generate ./sessions/schema
```

The tests in `sessions/schema` compare the schema's properties and required
fields to the Go structs and fail when they differ.

**Add a collector or a session reader.** Implement the contract in the
repository that owns the vendor format (here only if it is standard-library
only), add tests with fabricated fixtures, and document it in
`docs/guides/providers/`. A session reader also needs a case in the shared
reader contract test in `omnidevx`.

**Review checklist for session changes.** The session catalog is the one
place content is read, so check that a change does not produce `Event`
values from session data, write session content to the event store, make
network calls, or add content to `Event`. See the
[Privacy Model](../guides/concepts/privacy.md).

## Releases

The changelog is generated from `CHANGELOG.json`:

```bash
schangelog parse-commits --since=<previous-tag>   # review what changed
schangelog validate CHANGELOG.json
schangelog generate CHANGELOG.json -o CHANGELOG.md
```

To release a version:

1. Move the `unreleased` entries in `CHANGELOG.json` into a release entry with
   the version and date, validate it, and regenerate `CHANGELOG.md`.
2. Write `docs/releases/vX.Y.Z.md` and add it to the `mkdocs.yml` navigation.
3. Run the tests with `-race`, the linter, and `mkdocs build --strict`.
4. Commit, push, and wait for CI to pass.
5. Tag the release, `git tag vX.Y.Z && git push origin vX.Y.Z`.

Tag only after CI has passed on the pushed commit. A published version is
immutable: the Go module proxy and checksum database record it permanently,
so correct a mistake with a new version, never by moving a tag.

When this module releases, update `omni-openai` and `omnidevx`, which depend
on it, in that order: `omni-openai` first, then `omnidevx`.
