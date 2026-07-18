package omnidevx

import "time"

// EventType identifies the kind of activity an Event records.
//
// The "ai." namespace covers coding-agent session activity; the "devx."
// namespace covers development-work semantics (commits, reviews, delivery).
type EventType string

// Coding-agent session event types.
const (
	EventSessionStarted  EventType = "ai.session.started"
	EventSessionEnded    EventType = "ai.session.ended"
	EventPromptSubmitted EventType = "ai.prompt.submitted"
	// EventMessageCompleted records one assistant turn, with model and
	// token-usage attributes.
	EventMessageCompleted EventType = "ai.message.completed"
	EventToolCompleted    EventType = "ai.tool.completed"
	EventPatchApplied     EventType = "ai.patch.applied"
	EventTaskStarted      EventType = "ai.task.started"
	EventTaskCompleted    EventType = "ai.task.completed"
	// EventUsageRecorded reports incremental token usage for sources that
	// emit usage separately from assistant messages.
	EventUsageRecorded EventType = "ai.usage.recorded"
)

// Development-work event types ("devx." namespace).
const (
	// EventChangeCommitted records one git commit, with attribution
	// attributes including AI co-author detection.
	EventChangeCommitted EventType = "devx.change.committed"
	// EventContributionRecorded is one calendar day of platform-reported
	// contribution activity (e.g. the GitHub contribution graph).
	EventContributionRecorded EventType = "devx.contribution.recorded"
	// EventContributionSnapshot is a per-repository contribution summary
	// observed for a period.
	EventContributionSnapshot EventType = "devx.contribution.snapshot"
	// EventProfileSnapshot is a whole-profile contribution summary observed
	// for a period.
	EventProfileSnapshot EventType = "devx.profile.snapshot"
)

// Common attribute keys used in Event.Attributes. Providers should prefer
// these keys over ad-hoc names so metrics can be computed across sources.
const (
	AttrModel               = "model"
	AttrInputTokens         = "input_tokens"
	AttrOutputTokens        = "output_tokens"
	AttrCacheReadTokens     = "cache_read_tokens"
	AttrCacheCreationTokens = "cache_creation_tokens"
	AttrReasoningTokens     = "reasoning_tokens"
	AttrTotalTokens         = "total_tokens"
	AttrTool                = "tool"
	AttrSuccess             = "success"
	AttrDurationMS          = "duration_ms"
	AttrTimeToFirstTokenMS  = "time_to_first_token_ms"
	AttrSidechain           = "sidechain"
	AttrClientVersion       = "client_version"
	AttrReasoningEffort     = "reasoning_effort"
	AttrSessionSource       = "session_source"
	AttrSessionTotalTokens  = "session_total_tokens"
	AttrCommitHash          = "commit_hash"
	AttrAuthorEmail         = "author_email"
	AttrAIAssisted          = "ai_assisted"
	AttrAITools             = "ai_tools"
	AttrInsertions          = "insertions"
	AttrDeletions           = "deletions"
	AttrFilesChanged        = "files_changed"
	AttrCommits             = "commits"
	AttrContributions       = "contributions"
	AttrPullRequests        = "pull_requests"
	AttrIssues              = "issues"
	AttrReviews             = "reviews"
	AttrRepositories        = "repositories"
	AttrReposCreated        = "repos_created"
	AttrReleases            = "releases"
	AttrPrivateRepo         = "private"
	AttrCostUSD             = "cost_usd"
)

// CollectionMode describes how an event was obtained.
type CollectionMode string

const (
	ModeHistory CollectionMode = "history" // reconstructed from local session records
	ModeOTel    CollectionMode = "otel"    // observed via OpenTelemetry export
	ModeHooks   CollectionMode = "hooks"   // observed via lifecycle hooks
	ModeAPI     CollectionMode = "api"     // fetched from a service API
	ModeSurvey  CollectionMode = "survey"  // self-reported by the developer
)

// Source identifies the system an event came from. Provider and Product use
// canonical names (e.g. provider "anthropic", product "claude-code") even
// when Go package names are compressed.
type Source struct {
	Provider string `json:"provider"`
	Product  string `json:"product"`
	Version  string `json:"version,omitempty"`
}

// SubjectRef identifies whose activity an event describes. PersonID is the
// canonical identity; account fields support later identity resolution.
type SubjectRef struct {
	PersonID     string `json:"personId,omitempty"`
	LocalAccount string `json:"localAccount,omitempty"`
	DeviceID     string `json:"deviceId,omitempty"`
}

// EventContext situates an event in a session and a workspace.
type EventContext struct {
	SessionID  string `json:"sessionId,omitempty"`
	Repository string `json:"repository,omitempty"`
	Workspace  string `json:"workspace,omitempty"`
	GitBranch  string `json:"gitBranch,omitempty"`
}

// Provenance records how an event was collected and how reliable it is.
// Confidence is in [0, 1]; directly observed events are 1.0, historical
// reconstruction is lower.
type Provenance struct {
	CollectionMode CollectionMode `json:"collectionMode"`
	Confidence     float64        `json:"confidence"`
}

// Event is the canonical observation. IDs are deterministic per source
// record so re-importing the same history deduplicates naturally.
//
// Attributes hold metadata only: never prompt text, model responses, or
// file contents.
type Event struct {
	ID         string         `json:"id"`
	Type       EventType      `json:"type"`
	Timestamp  time.Time      `json:"timestamp"`
	Subject    SubjectRef     `json:"subject,omitzero"`
	Source     Source         `json:"source"`
	Context    EventContext   `json:"context,omitzero"`
	Attributes map[string]any `json:"attributes,omitempty"`
	Provenance Provenance     `json:"provenance"`
}
