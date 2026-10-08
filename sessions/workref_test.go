package sessions

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func mustExtractor(t *testing.T, rules []WorkRefRule) *WorkRefExtractor {
	t.Helper()
	e, err := NewWorkRefExtractor(rules)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestDefaultRulesMatch(t *testing.T) {
	e := mustExtractor(t, DefaultWorkRefRules())
	tests := []struct {
		name string
		text string
		want []string
	}{
		{"initiative and rmi", "work on INIT-EXAMPLE-001 via RMI-EXAMPLECORE-012", []string{"INIT-EXAMPLE-001", "RMI-EXAMPLECORE-012"}},
		{"four-digit numbers", "see RMI-EXAMPLE-1042", []string{"RMI-EXAMPLE-1042"}},
		{"in a path", "docs/specs/initiatives/INIT-ABC2-003/PRD.md", []string{"INIT-ABC2-003"}},
		{"in a branch", "feat/RMI-EXAMPLE-007-session-catalog", []string{"RMI-EXAMPLE-007"}},
		{"trailer", "Refs: RMI-EXAMPLE-004", []string{"RMI-EXAMPLE-004"}},
		{"too few digits", "RMI-EXAMPLE-01 and INIT-X-1", nil},
		{"lower case is not matched", "rmi-example-001", nil},
		{"embedded in a longer word", "XRMI-EXAMPLE-001Y", nil},
		{"no match", "nothing to see", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set := e.NewSet()
			set.Add(WorkRefPrompt, tt.text)
			var got []string
			for _, r := range set.Refs() {
				got = append(got, r.ID)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSetMergesSourcesAndSorts(t *testing.T) {
	set := mustExtractor(t, DefaultWorkRefRules()).NewSet()
	set.Add(WorkRefPath, "docs/INIT-EXAMPLE-001/PLAN.md")
	set.Add(WorkRefPrompt, "continue INIT-EXAMPLE-001 and RMI-EXAMPLE-002")
	set.Add(WorkRefCommit, "feat: x\n\nRefs: RMI-EXAMPLE-002")
	set.Add(WorkRefPrompt, "again INIT-EXAMPLE-001") // repeat from the same source

	got := set.Refs()
	want := []WorkRef{
		{ID: "INIT-EXAMPLE-001", Kind: "initiative", Sources: []WorkRefSource{WorkRefPrompt, WorkRefPath}},
		{ID: "RMI-EXAMPLE-002", Kind: "rmi", Sources: []WorkRefSource{WorkRefPrompt, WorkRefCommit}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestFirstMatchingRuleNamesTheKind(t *testing.T) {
	e := mustExtractor(t, []WorkRefRule{
		{Name: "specific", Pattern: `\bRMI-EXAMPLE-\d+\b`},
		{Name: "any-rmi", Pattern: `\bRMI-[A-Z]+-\d+\b`},
	})
	set := e.NewSet()
	set.Add(WorkRefPrompt, "RMI-EXAMPLE-5 and RMI-OTHER-6")
	kinds := map[string]string{}
	for _, r := range set.Refs() {
		kinds[r.ID] = r.Kind
	}
	if kinds["RMI-EXAMPLE-5"] != "specific" || kinds["RMI-OTHER-6"] != "any-rmi" {
		t.Fatalf("kinds = %v", kinds)
	}
}

func TestCustomRule(t *testing.T) {
	e := mustExtractor(t, []WorkRefRule{{Name: "ticket", Pattern: `\bTICKET-\d+\b`}})
	set := e.NewSet()
	set.Add(WorkRefBranch, "fix/TICKET-42-login")
	refs := set.Refs()
	if len(refs) != 1 || refs[0].ID != "TICKET-42" || refs[0].Kind != "ticket" {
		t.Fatalf("refs = %+v", refs)
	}
}

func TestNewWorkRefExtractorRejectsBadRules(t *testing.T) {
	tests := []struct {
		name    string
		rules   []WorkRefRule
		wantErr string
	}{
		{"empty name", []WorkRefRule{{Pattern: `x`}}, "no name"},
		{"duplicate name", []WorkRefRule{{Name: "a", Pattern: `x`}, {Name: "a", Pattern: `y`}}, "duplicate"},
		{"bad pattern", []WorkRefRule{{Name: "a", Pattern: `(`}}, `rule "a"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewWorkRefExtractor(tt.rules)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

type fakeResolver struct {
	titles map[string]string
	err    error
	asked  []string
}

func (f *fakeResolver) Titles(_ context.Context, ids []string) (map[string]string, error) {
	f.asked = ids
	return f.titles, f.err
}

func TestResolveTitles(t *testing.T) {
	refs := []WorkRef{{ID: "INIT-A-001", Kind: "initiative"}, {ID: "RMI-A-002", Kind: "rmi"}}

	r := &fakeResolver{titles: map[string]string{"INIT-A-001": "First"}}
	got, err := ResolveTitles(context.Background(), refs, r)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Title != "First" || got[1].Title != "" {
		t.Fatalf("titles = %q / %q, want only the known one filled", got[0].Title, got[1].Title)
	}
	if refs[0].Title != "" {
		t.Error("input slice must not be modified")
	}
	if !reflect.DeepEqual(r.asked, []string{"INIT-A-001", "RMI-A-002"}) {
		t.Errorf("resolver asked %v", r.asked)
	}

	// A failing resolver keeps the references and reports the error.
	failing := &fakeResolver{err: errors.New("backend down")}
	got, err = ResolveTitles(context.Background(), refs, failing)
	if err == nil || !strings.Contains(err.Error(), "backend down") {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("references lost on resolver failure: %+v", got)
	}

	// No resolver is not an error.
	if got, err := ResolveTitles(context.Background(), refs, nil); err != nil || len(got) != 2 {
		t.Fatalf("nil resolver: %v, %v", got, err)
	}
}
