package report

import omnidevx "github.com/plexusone/omnidevx-core"

// SchemaVersion is the schema this package's DeveloperPeriodReport
// implements.
const SchemaVersion = "omnidevx.developer-period/v1"

// Measurement kinds.
const (
	KindObserved  = "observed"
	KindEstimated = "estimated"
)

// Subject identifies whose activity a report describes.
type Subject struct {
	PersonID string `json:"personId"`
}

// Measurement records how a metric value was derived. Every metric in this
// package is a direct sum of observed event attributes, so Kind is always
// KindObserved; estimated metrics (e.g. inferred AI contribution ratios)
// belong to a future analytics layer built on top of these reports.
type Measurement struct {
	Kind       string  `json:"kind"`
	Method     string  `json:"method,omitempty"`
	Confidence float64 `json:"confidence"`
}

// Metric is one named value with the provenance behind it.
type Metric struct {
	Value       float64     `json:"value"`
	Unit        string      `json:"unit,omitempty"`
	Measurement Measurement `json:"measurement"`
}

// MetricSet holds the cross-source combined view alongside the per-source
// breakdown. A metric that does not combine safely across sources (see
// package doc) appears in BySource only.
type MetricSet struct {
	Combined map[string]Metric            `json:"combined"`
	BySource map[string]map[string]Metric `json:"bySource"`
}

// SourceCoverage summarizes what one source contributed to a report.
type SourceCoverage struct {
	Source          omnidevx.Source           `json:"source"`
	EventCount      int                       `json:"eventCount"`
	CollectionModes []omnidevx.CollectionMode `json:"collectionModes"`
	MinConfidence   float64                   `json:"minConfidence"`
}

// DataQuality reports how complete a report's coverage is.
type DataQuality struct {
	CoverageScore float64  `json:"coverageScore"`
	Warnings      []string `json:"warnings,omitempty"`
}

// DeveloperPeriodReport is the analytical source of truth for one person's
// activity over one period, combining every source that observed them. It
// is a presentation-agnostic artifact: DevFolio's contributor profile
// summarizes and links to this, never recomputes it.
type DeveloperPeriodReport struct {
	SchemaVersion string           `json:"schemaVersion"`
	Subject       Subject          `json:"subject"`
	Period        omnidevx.Period  `json:"period"`
	Sources       []SourceCoverage `json:"sources"`
	Metrics       MetricSet        `json:"metrics"`
	Quality       DataQuality      `json:"quality"`
}
