package identity

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// Identity types.
const (
	TypeGitHub       = "github"
	TypeGitEmail     = "git_email"
	TypeLocalAccount = "local_account"
)

// Identity links one account or device-scoped account to a person.
//
// Value holds the identifier for GitHub and local-account identities;
// git-email identities are stored hashed (ValueHash) so raw emails never
// need to be committed to a config file. DeviceID scopes local-account
// identities, since OS usernames collide across machines.
type Identity struct {
	Type      string `json:"type"`
	Value     string `json:"value,omitempty"`
	ValueHash string `json:"valueHash,omitempty"`
	DeviceID  string `json:"deviceId,omitempty"`
}

// Person links one canonical identity to every account it maps to.
type Person struct {
	PersonID   string     `json:"personId"`
	Identities []Identity `json:"identities"`
}

// HashEmail returns the canonical hash for a git-email identity: lowercased
// and trimmed, then SHA-256, formatted as "sha256:<hex>". Callers store this
// in Identity.ValueHash instead of the raw email, and pass raw addresses to
// Map.ResolveGitEmail, which hashes them the same way before lookup.
func HashEmail(email string) string {
	normalized := strings.ToLower(strings.TrimSpace(email))
	sum := sha256.Sum256([]byte(normalized))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Map resolves identities to person IDs.
type Map struct {
	byGitHub    map[string]string // github username -> personId
	byEmailHash map[string]string // "sha256:..." -> personId
	byLocal     map[string]string // "deviceId/account" -> personId
}

// NewMap builds a Map from a set of people. A duplicate identity (the same
// account claimed by two people) is a configuration error, not a runtime
// fallback decision, and is rejected.
func NewMap(people []Person) (*Map, error) {
	m := &Map{
		byGitHub:    map[string]string{},
		byEmailHash: map[string]string{},
		byLocal:     map[string]string{},
	}
	for _, p := range people {
		if p.PersonID == "" {
			return nil, fmt.Errorf("identity: person has no personId")
		}
		for _, id := range p.Identities {
			if err := m.add(p.PersonID, id); err != nil {
				return nil, err
			}
		}
	}
	return m, nil
}

func (m *Map) add(personID string, id Identity) error {
	switch id.Type {
	case TypeGitHub:
		if id.Value == "" {
			return fmt.Errorf("identity: %s identity for %s has no value", TypeGitHub, personID)
		}
		return insert(m.byGitHub, id.Value, personID, TypeGitHub)
	case TypeGitEmail:
		if id.ValueHash == "" {
			return fmt.Errorf("identity: %s identity for %s has no valueHash", TypeGitEmail, personID)
		}
		return insert(m.byEmailHash, id.ValueHash, personID, TypeGitEmail)
	case TypeLocalAccount:
		if id.DeviceID == "" || id.Value == "" {
			return fmt.Errorf("identity: %s identity for %s needs both deviceId and value", TypeLocalAccount, personID)
		}
		return insert(m.byLocal, localKey(id.DeviceID, id.Value), personID, TypeLocalAccount)
	default:
		return fmt.Errorf("identity: unknown identity type %q for %s", id.Type, personID)
	}
}

func insert(index map[string]string, key, personID, typ string) error {
	if existing, ok := index[key]; ok && existing != personID {
		return fmt.Errorf("identity: %s %q claimed by both %s and %s", typ, key, existing, personID)
	}
	index[key] = personID
	return nil
}

func localKey(deviceID, value string) string { return deviceID + "/" + value }

// ResolveGitHub returns the personId for a GitHub username, if known.
func (m *Map) ResolveGitHub(username string) (string, bool) {
	id, ok := m.byGitHub[username]
	return id, ok
}

// ResolveGitEmail returns the personId for a git commit email, if known. The
// email is hashed before lookup so callers pass raw addresses.
func (m *Map) ResolveGitEmail(email string) (string, bool) {
	id, ok := m.byEmailHash[HashEmail(email)]
	return id, ok
}

// ResolveLocalAccount returns the personId for a device-scoped local
// account, if known.
func (m *Map) ResolveLocalAccount(deviceID, account string) (string, bool) {
	id, ok := m.byLocal[localKey(deviceID, account)]
	return id, ok
}
