// Package git collects development-work events from local git repositories:
// one devx.change.committed event per commit, with AI co-author attribution
// detected from Co-authored-by trailers.
//
// Repository discovery and log parsing come from github.com/grokify/gogit;
// this package adds the DevX semantics (canonical events, the known-AI-tool
// registry, provenance).
//
// Only metadata is extracted — hashes, timestamps, author identities, change
// stats, and AI-tool attribution. Commit subjects and bodies are never read
// into events.
package git
