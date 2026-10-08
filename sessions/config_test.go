package sessions

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ConfigFileName)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadConfigMissingFileIsNotAnError(t *testing.T) {
	c, err := LoadConfig(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("a missing config must give defaults, got %v", err)
	}
	if len(c.WorkspaceRoots) != 0 || len(c.WorkRefRules) != 0 {
		t.Fatalf("config = %+v, want zero", c)
	}
	if _, err := c.Extractor(); err != nil {
		t.Fatalf("default extractor: %v", err)
	}
}

func TestLoadConfigReadsSettings(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	path := writeConfig(t, `{
		"workspaceRoots": ["~/go/src", `+strconv.Quote(work)+`],
		"workRefRules": [{"name": "ticket", "pattern": "\\bTICKET-\\d+\\b"}]
	}`)
	c, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	roots, err := c.ExpandedRoots(home)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "go", "src") + "," + work; strings.Join(roots, ",") != want {
		t.Fatalf("roots = %v", roots)
	}
	ext, err := c.Extractor()
	if err != nil {
		t.Fatal(err)
	}
	set := ext.NewSet()
	set.Add(WorkRefPrompt, "TICKET-9 and RMI-EXAMPLE-001")
	refs := set.Refs()
	if len(refs) != 1 || refs[0].ID != "TICKET-9" {
		t.Fatalf("configured rules must replace the defaults, got %+v", refs)
	}
}

func TestLoadConfigRejectsBadFiles(t *testing.T) {
	tests := []struct {
		name, body, wantErr string
	}{
		{"not json", `{not json`, "parse config"},
		{"unknown key", `{"workspaceRoot": ["~/x"]}`, "unknown field"},
		{"empty root", `{"workspaceRoots": ["~/x", " "]}`, "empty entry"},
		{"bad pattern", `{"workRefRules": [{"name": "a", "pattern": "("}]}`, `rule "a"`},
		{"unnamed rule", `{"workRefRules": [{"pattern": "x"}]}`, "no name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.body)
			_, err := LoadConfig(path)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
			}
			if !strings.Contains(err.Error(), path) {
				t.Errorf("error should name the file: %v", err)
			}
		})
	}
}

func TestExpandedRootsRejectsRelativePaths(t *testing.T) {
	c := Config{WorkspaceRoots: []string{"relative/dir"}}
	home := t.TempDir()
	if _, err := c.ExpandedRoots(home); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("err = %v", err)
	}
	c = Config{WorkspaceRoots: []string{"~", filepath.Join(home, "a", "b", "..", "c")}}
	roots, err := c.ExpandedRoots(home)
	if err != nil || roots[0] != home || roots[1] != filepath.Join(home, "a", "c") {
		t.Fatalf("roots = %v, err = %v", roots, err)
	}
}

func TestDefaultConfigPath(t *testing.T) {
	p, err := DefaultConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(p, filepath.Join(".plexusone", "omnidevx", "config.json")) {
		t.Fatalf("path = %s", p)
	}
}
