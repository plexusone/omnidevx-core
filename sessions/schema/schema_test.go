package schema_test

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/plexusone/omnidevx-core/sessions"
	"github.com/plexusone/omnidevx-core/sessions/schema"
)

type def struct {
	Type       string                     `json:"type"`
	Required   []string                   `json:"required"`
	Properties map[string]json.RawMessage `json:"properties"`
}

type document struct {
	ID    string         `json:"$id"`
	Ref   string         `json:"$ref"`
	Title string         `json:"title"`
	Defs  map[string]def `json:"$defs"`
}

// covered maps each schema definition to the Go type it describes. A new
// type added to the schema must be added here, which forces the drift
// check below to cover it.
var covered = map[string]reflect.Type{
	"Session":      reflect.TypeOf(sessions.Session{}),
	"Prompt":       reflect.TypeOf(sessions.Prompt{}),
	"Runtime":      reflect.TypeOf(sessions.Runtime{}),
	"ResumeSpec":   reflect.TypeOf(sessions.ResumeSpec{}),
	"Evidence":     reflect.TypeOf(sessions.Evidence{}),
	"RepoActivity": reflect.TypeOf(sessions.RepoActivity{}),
	"FileActivity": reflect.TypeOf(sessions.FileActivity{}),
	"CommitRef":    reflect.TypeOf(sessions.CommitRef{}),
	"WorkRef":      reflect.TypeOf(sessions.WorkRef{}),
}

func load(t *testing.T) document {
	t.Helper()
	var d document
	if err := json.Unmarshal(schema.Session(), &d); err != nil {
		t.Fatalf("embedded schema is not valid JSON: %v", err)
	}
	return d
}

func TestSchemaIdentity(t *testing.T) {
	d := load(t)
	if !strings.HasSuffix(d.ID, "/sessions/schema/session.schema.json") {
		t.Errorf("$id = %q", d.ID)
	}
	if d.Ref != "#/$defs/Session" {
		t.Errorf("$ref = %q, want the Session definition", d.Ref)
	}
	if d.Title == "" {
		t.Error("schema has no title")
	}
}

func TestSchemaReturnsACopy(t *testing.T) {
	a := schema.Session()
	a[0] = 'X'
	if schema.Session()[0] == 'X' {
		t.Fatal("Session() must return a copy the caller can modify")
	}
}

// jsonFields returns the JSON property names of a struct and which of them
// are required (not marked omitempty or omitzero).
func jsonFields(typ reflect.Type) (names, required []string) {
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if !f.IsExported() {
			continue
		}
		name, opts, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		names = append(names, name)
		if !strings.Contains(opts, "omitempty") && !strings.Contains(opts, "omitzero") {
			required = append(required, name)
		}
	}
	sort.Strings(names)
	sort.Strings(required)
	return names, required
}

// TestSchemaMatchesGoTypes is the drift guard: if a struct changes and the
// schema is not regenerated, a property or its required status differs.
func TestSchemaMatchesGoTypes(t *testing.T) {
	d := load(t)
	for name := range d.Defs {
		if _, ok := covered[name]; !ok {
			t.Errorf("schema defines %q, which this test does not map to a Go type; add it to covered", name)
		}
	}
	for name, typ := range covered {
		sd, ok := d.Defs[name]
		if !ok {
			t.Errorf("schema has no definition for %s; regenerate with go generate ./sessions/schema", name)
			continue
		}
		wantNames, wantRequired := jsonFields(typ)
		gotNames := make([]string, 0, len(sd.Properties))
		for p := range sd.Properties {
			gotNames = append(gotNames, p)
		}
		sort.Strings(gotNames)
		gotRequired := append([]string(nil), sd.Required...)
		sort.Strings(gotRequired)

		if !reflect.DeepEqual(gotNames, wantNames) {
			t.Errorf("%s properties differ from the Go type\n  schema: %v\n  go:     %v\nregenerate with go generate ./sessions/schema", name, gotNames, wantNames)
		}
		if !reflect.DeepEqual(gotRequired, wantRequired) {
			t.Errorf("%s required fields differ from the Go type\n  schema: %v\n  go:     %v\nregenerate with go generate ./sessions/schema", name, gotRequired, wantRequired)
		}
	}
}

// TestSessionJSONMatchesSchemaShape marshals a fully populated Session and
// checks that every key it emits is a property the schema declares.
func TestSessionJSONMatchesSchemaShape(t *testing.T) {
	d := load(t)
	s := sessions.Session{
		Harness: sessions.HarnessCodex, ID: "x", CWD: "/w", State: sessions.StateRunning,
		Title: "t", RecentPrompts: []sessions.Prompt{{Text: "p"}},
		Runtime:  &sessions.Runtime{PID: 1},
		Resume:   sessions.ResumeSpec{Argv: []string{"codex"}, Dir: "/w"},
		Evidence: &sessions.Evidence{Repos: []sessions.RepoActivity{{Root: "/w"}}},
	}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatal(err)
	}
	for key := range top {
		if _, ok := d.Defs["Session"].Properties[key]; !ok {
			t.Errorf("Session JSON emits %q, which the schema does not declare", key)
		}
	}
	for _, req := range d.Defs["Session"].Required {
		if _, ok := top[req]; !ok {
			t.Errorf("Session JSON omits required property %q", req)
		}
	}
}
