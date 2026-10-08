package sessions

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ConfigFileName is the name of the configuration file inside the OmniDevX
// directory, ~/.plexusone/omnidevx.
const ConfigFileName = "config.json"

// Config is the user's session-catalog configuration. Every field is
// optional: an empty Config, or no file at all, gives usable defaults.
type Config struct {
	// WorkspaceRoots are directories that hold repositories, such as ~/go/src. The repository
	// index scans them for repositories. A leading ~ means the home directory.
	WorkspaceRoots []string `json:"workspaceRoots,omitempty"`
	// WorkRefRules replace the default work-reference rules when present. Omit it to match
	// initiative and roadmap-item IDs.
	WorkRefRules []WorkRefRule `json:"workRefRules,omitempty"`
}

// DefaultConfigPath returns ~/.plexusone/omnidevx/config.json.
func DefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".plexusone", "omnidevx", ConfigFileName), nil
}

// LoadConfig reads the configuration at path. A file that does not exist is
// not an error and yields the zero Config. A file that cannot be read, is
// not valid JSON, or has an unknown key is an error that names the path, so
// a typo cannot silently turn a setting off.
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is the caller's configuration file
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var c Config
	if err := dec.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := c.validate(); err != nil {
		return Config{}, fmt.Errorf("config %s: %w", path, err)
	}
	return c, nil
}

func (c Config) validate() error {
	for _, r := range c.WorkspaceRoots {
		if strings.TrimSpace(r) == "" {
			return errors.New("workspaceRoots contains an empty entry")
		}
	}
	if len(c.WorkRefRules) > 0 {
		if _, err := NewWorkRefExtractor(c.WorkRefRules); err != nil {
			return err
		}
	}
	return nil
}

// ExpandedRoots returns the workspace roots as absolute, cleaned paths with
// a leading ~ replaced by home. A relative path that is not ~-prefixed is
// an error, because it would depend on where the program happened to run.
func (c Config) ExpandedRoots(home string) ([]string, error) {
	out := make([]string, 0, len(c.WorkspaceRoots))
	for _, r := range c.WorkspaceRoots {
		p := r
		switch {
		case p == "~":
			p = home
		case strings.HasPrefix(p, "~/"):
			p = filepath.Join(home, p[2:])
		}
		if !filepath.IsAbs(p) {
			return nil, fmt.Errorf("workspace root %q must be absolute or start with ~/", r)
		}
		out = append(out, filepath.Clean(p))
	}
	return out, nil
}

// Extractor returns a work-reference extractor over the configured rules,
// or over DefaultWorkRefRules when none are configured.
func (c Config) Extractor() (*WorkRefExtractor, error) {
	rules := c.WorkRefRules
	if len(rules) == 0 {
		rules = DefaultWorkRefRules()
	}
	return NewWorkRefExtractor(rules)
}
