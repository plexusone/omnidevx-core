package omnidevx

import (
	"context"
	"time"
)

// Period is a half-open time range [Start, End). A zero Start or End leaves
// that bound open.
type Period struct {
	Start time.Time `json:"start,omitzero"`
	End   time.Time `json:"end,omitzero"`
}

// Contains reports whether t falls within the period.
func (p Period) Contains(t time.Time) bool {
	if !p.Start.IsZero() && t.Before(p.Start) {
		return false
	}
	if !p.End.IsZero() && !t.Before(p.End) {
		return false
	}
	return true
}

// CollectRequest scopes a collection run. The Subject, when set, is stamped
// onto every event the collector emits.
type CollectRequest struct {
	Period  Period     `json:"period,omitzero"`
	Subject SubjectRef `json:"subject,omitzero"`
}

// Diagnostic severities.
const (
	SeverityWarning = "warning"
	SeverityError   = "error"
)

// Diagnostic reports a non-fatal problem encountered during collection,
// such as an unparseable record. Collectors surface these rather than
// silently dropping data.
type Diagnostic struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Path     string `json:"path,omitempty"`
}

// CollectionResult is the output of one collection run.
type CollectionResult struct {
	Source      Source       `json:"source"`
	Subject     SubjectRef   `json:"subject,omitzero"`
	Period      Period       `json:"period,omitzero"`
	Events      []Event      `json:"events"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
	CollectedAt time.Time    `json:"collectedAt"`
}

// Collector is implemented by every source provider. Implementations
// normalize provider-native records into canonical events; they never
// compute framework metrics.
type Collector interface {
	Source() Source
	Collect(ctx context.Context, req CollectRequest) (*CollectionResult, error)
}
