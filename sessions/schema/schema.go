// Package schema embeds the generated JSON Schema for sessions.Session, the
// document "omnidevx sessions --json" prints, so tools can validate it
// without a copy of the Go types.
//
// The schema is generated from the Go structs, which are the source of
// truth, and committed. Regenerate it after changing a type in the sessions
// package:
//
//	go generate ./sessions/schema
//
// Check that the committed file is current, for example in CI, by
// regenerating it and failing on any difference. The generate directive
// below holds the id, title, and description, so a bare schemakit --check
// without them would report drift on a current file.
//
//	go generate ./sessions/schema && git diff --exit-code -- sessions/schema/session.schema.json
//
// The tests in this package also compare the schema's properties and
// required fields to the Go structs, so most drift fails go test.
package schema

import _ "embed"

//go:generate schemakit generate --comments --id https://raw.githubusercontent.com/plexusone/omnidevx-core/main/sessions/schema/session.schema.json --title "OmniDevX Session" --description "A resumable coding-agent session as reported by the OmniDevX session catalog." -o session.schema.json github.com/plexusone/omnidevx-core/sessions Session

//go:embed session.schema.json
var session []byte

// Session returns the JSON Schema (draft 2020-12) for sessions.Session. The
// caller may modify the returned slice.
func Session() []byte {
	return append([]byte(nil), session...)
}
