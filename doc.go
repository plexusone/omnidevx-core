// Package omnidevx defines the canonical intermediate representation (IR)
// for developer-experience telemetry: events observed while developers and
// AI coding agents build software, and the collector contract that source
// providers implement.
//
// OmniDevX is not OmniDXI: OmniDXI (github.com/plexusone/omnidxi) is a
// Digital Experience Intelligence facade over product-analytics platforms
// (Amplitude, Mixpanel, Heap, Pendo). OmniDevX observes developer and
// coding-agent activity (sessions, prompts, tool calls, commits, reviews)
// for frameworks such as SPACE, AI SPACE, and DORA.
//
// Privacy: canonical events carry metadata only — event types, durations,
// counts, models, token figures, and repository identifiers. Prompt text,
// model responses, and file contents are never captured by default.
package omnidevx
