# Identity

The `identity` package resolves raw account identifiers — GitHub usernames,
git commit emails, device-scoped local accounts — to a canonical
`personId`, so [period reports](reports.md) can aggregate one developer's
activity across multiple accounts and machines without conflating people.

GitHub username is never the canonical identity: a developer may have
multiple GitHub accounts, several git commit emails, and local OS accounts
on more than one machine.

## Building a map

```go
people := []identity.Person{{
	PersonID: "person:01J...",
	Identities: []identity.Identity{
		{Type: identity.TypeGitHub, Value: "grokify"},
		{Type: identity.TypeGitEmail, ValueHash: identity.HashEmail("john@example.com")},
		{Type: identity.TypeLocalAccount, DeviceID: "device:mac-studio", Value: "john"},
	},
}}
m, err := identity.NewMap(people)
```

`NewMap` rejects a duplicate identity claimed by two different people —
ambiguous resolution is a configuration error, not a runtime fallback
decision.

## Resolving

```go
m.ResolveGitHub("grokify")                          // (personId, ok)
m.ResolveGitEmail("john@example.com")                // hashed before lookup
m.ResolveLocalAccount("device:mac-studio", "john")   // device-scoped
```

## Why git emails are hashed

`Identity.ValueHash` stores `sha256:<hex>` of the lowercased, trimmed
email — never the raw address — so an identity map can be committed to a
config file without leaking email addresses. `HashEmail` normalizes the
same way `ResolveGitEmail` does internally, so callers pass raw addresses
on both sides.
