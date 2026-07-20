package identity

import "testing"

func TestNewMapAndResolve(t *testing.T) {
	people := []Person{
		{
			PersonID: "person:john",
			Identities: []Identity{
				{Type: TypeGitHub, Value: "grokify"},
				{Type: TypeGitEmail, ValueHash: HashEmail("john@example.com")},
				{Type: TypeLocalAccount, DeviceID: "device:mac-studio", Value: "john"},
			},
		},
	}
	m, err := NewMap(people)
	if err != nil {
		t.Fatal(err)
	}

	if id, ok := m.ResolveGitHub("grokify"); !ok || id != "person:john" {
		t.Errorf("ResolveGitHub: got %q, %v", id, ok)
	}
	if id, ok := m.ResolveGitEmail("John@Example.com "); !ok || id != "person:john" {
		t.Errorf("ResolveGitEmail: got %q, %v (expected case/whitespace-insensitive match)", id, ok)
	}
	if id, ok := m.ResolveLocalAccount("device:mac-studio", "john"); !ok || id != "person:john" {
		t.Errorf("ResolveLocalAccount: got %q, %v", id, ok)
	}
	if _, ok := m.ResolveGitHub("someone-else"); ok {
		t.Error("expected unknown github username to be unresolved")
	}
	if _, ok := m.ResolveLocalAccount("device:other", "john"); ok {
		t.Error("local account should be device-scoped")
	}
}

func TestNewMapRejectsDuplicateIdentity(t *testing.T) {
	people := []Person{
		{PersonID: "person:a", Identities: []Identity{{Type: TypeGitHub, Value: "shared"}}},
		{PersonID: "person:b", Identities: []Identity{{Type: TypeGitHub, Value: "shared"}}},
	}
	if _, err := NewMap(people); err == nil {
		t.Fatal("expected error for identity claimed by two people")
	}
}

func TestNewMapAllowsSameIdentityTwiceForSamePerson(t *testing.T) {
	people := []Person{
		{PersonID: "person:a", Identities: []Identity{
			{Type: TypeGitHub, Value: "dup"},
			{Type: TypeGitHub, Value: "dup"},
		}},
	}
	if _, err := NewMap(people); err != nil {
		t.Fatalf("re-declaring the same identity for the same person should not error: %v", err)
	}
}

func TestNewMapValidation(t *testing.T) {
	cases := []struct {
		name   string
		people []Person
	}{
		{"missing personId", []Person{{Identities: []Identity{{Type: TypeGitHub, Value: "x"}}}}},
		{"github missing value", []Person{{PersonID: "p", Identities: []Identity{{Type: TypeGitHub}}}}},
		{"git_email missing hash", []Person{{PersonID: "p", Identities: []Identity{{Type: TypeGitEmail}}}}},
		{"local_account missing device", []Person{{PersonID: "p", Identities: []Identity{{Type: TypeLocalAccount, Value: "x"}}}}},
		{"local_account missing value", []Person{{PersonID: "p", Identities: []Identity{{Type: TypeLocalAccount, DeviceID: "d"}}}}},
		{"unknown type", []Person{{PersonID: "p", Identities: []Identity{{Type: "carrier-pigeon", Value: "x"}}}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := NewMap(c.people); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestHashEmailNormalizes(t *testing.T) {
	if HashEmail("Foo@Bar.com") != HashEmail(" foo@bar.com ") {
		t.Error("HashEmail should normalize case and whitespace")
	}
	if HashEmail("a@b.com") == HashEmail("c@d.com") {
		t.Error("different emails should hash differently")
	}
}
