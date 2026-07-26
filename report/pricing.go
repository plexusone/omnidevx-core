package report

import (
	_ "embed"
	"encoding/json"
	"sort"
	"strings"
	"unicode"
)

//go:embed pricing.json
var pricingJSON []byte

// ModelPricing holds per-million-token prices (USD) for one model.
type ModelPricing struct {
	InputPerMillion         float64 `json:"inputPerMillion"`
	OutputPerMillion        float64 `json:"outputPerMillion"`
	CacheReadPerMillion     float64 `json:"cacheReadPerMillion"`
	CacheCreationPerMillion float64 `json:"cacheCreationPerMillion"`
}

// PricingData is the top-level structure of pricing.json.
type PricingData struct {
	Version string                  `json:"version"`
	Source  string                  `json:"source"`
	Note    string                  `json:"note"`
	Models  map[string]ModelPricing `json:"models"`
}

// modelPricingTable is loaded from embedded pricing.json at init time.
var modelPricingTable map[string]ModelPricing

// pricingMeta holds version and source info from embedded JSON.
var pricingMeta PricingData

// sortedPrefixes holds model prefixes sorted longest-first for prefix matching.
var sortedPrefixes []string

func init() {
	if err := json.Unmarshal(pricingJSON, &pricingMeta); err != nil {
		panic("report: failed to parse embedded pricing.json: " + err.Error())
	}
	modelPricingTable = pricingMeta.Models

	sortedPrefixes = make([]string, 0, len(modelPricingTable))
	for k := range modelPricingTable {
		sortedPrefixes = append(sortedPrefixes, k)
	}
	sort.Slice(sortedPrefixes, func(i, j int) bool {
		return len(sortedPrefixes[i]) > len(sortedPrefixes[j])
	})
}

// PricingVersion returns the version string from embedded pricing.json (e.g., "2026-07").
func PricingVersion() string {
	return pricingMeta.Version
}

// PricingSource returns the source URL from embedded pricing.json.
func PricingSource() string {
	return pricingMeta.Source
}

// AllPricing returns a copy of the full pricing table.
func AllPricing() map[string]ModelPricing {
	result := make(map[string]ModelPricing, len(modelPricingTable))
	for k, v := range modelPricingTable {
		result[k] = v
	}
	return result
}

// LookupPricing returns pricing for a model name. It tries exact match first,
// then longest-prefix match with boundary validation (so "claude-opus-4-8[1m]"
// matches "claude-opus-4-8" but "claude-opus-4-80" does not).
//
// Valid suffixes after a prefix: [1m], -YYYYMMDD, or end of string.
// Returns false if no matching pricing is found.
func LookupPricing(model string) (ModelPricing, bool) {
	// Exact match
	if p, ok := modelPricingTable[model]; ok {
		return p, true
	}
	// Longest-prefix match with boundary check
	for _, prefix := range sortedPrefixes {
		if strings.HasPrefix(model, prefix) && isValidSuffix(model[len(prefix):]) {
			return modelPricingTable[prefix], true
		}
	}
	return ModelPricing{}, false
}

// isValidSuffix returns true if suffix is empty or starts with a valid
// model ID boundary: "[" (for [1m]), "-" followed by digit (for -YYYYMMDD).
func isValidSuffix(suffix string) bool {
	if suffix == "" {
		return true
	}
	if suffix[0] == '[' {
		return true
	}
	if suffix[0] == '-' && len(suffix) > 1 && unicode.IsDigit(rune(suffix[1])) {
		return true
	}
	return false
}

// EstimateCost computes the USD cost from token counts using model pricing.
func EstimateCost(p ModelPricing, input, output, cacheRead, cacheCreation int64) float64 {
	const perMillion = 1_000_000.0
	return float64(input)/perMillion*p.InputPerMillion +
		float64(output)/perMillion*p.OutputPerMillion +
		float64(cacheRead)/perMillion*p.CacheReadPerMillion +
		float64(cacheCreation)/perMillion*p.CacheCreationPerMillion
}
