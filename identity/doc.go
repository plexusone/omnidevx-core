// Package identity resolves raw account identifiers (GitHub usernames, git
// commit emails, device-scoped local accounts) to a canonical personId, so
// period reports can aggregate one developer's activity across multiple
// accounts and machines without conflating people.
//
// GitHub username is never the canonical identity: a developer may have
// multiple GitHub accounts, several git commit emails, and local OS
// accounts on more than one machine. Git emails are matched by hash so raw
// addresses never need to live in a config file.
package identity
