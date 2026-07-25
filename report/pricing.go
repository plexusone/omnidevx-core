package report

import (
	"sort"
	"strings"
	"unicode"
)

// ModelPricing holds per-million-token prices (USD) for one model.
type ModelPricing struct {
	InputPerMillion         float64
	OutputPerMillion        float64
	CacheReadPerMillion     float64
	CacheCreationPerMillion float64
}

// modelPricingTable maps model name prefixes to pricing. Entries are checked
// in longest-prefix-first order so "claude-opus-4-8" matches before
// "claude-opus-4". Suffixes like [1m] (extended context) or -YYYYMMDD (version)
// are allowed after the prefix.
//
// Prices from Anthropic's published API pricing (per 1M tokens) as of July 2026:
// https://platform.claude.com/docs/en/about-claude/pricing
//
// Cache pricing uses the 5-minute TTL rates (1.25x input for write, 0.1x for read).
var modelPricingTable = map[string]ModelPricing{
	// Claude 4 Opus family (legacy, same pricing as 4.8)
	"claude-opus-4-8": {
		InputPerMillion:         5.00,
		OutputPerMillion:        25.00,
		CacheReadPerMillion:     0.50, // 0.1x input
		CacheCreationPerMillion: 6.25, // 1.25x input
	},
	"claude-opus-4-7": {
		InputPerMillion:         5.00,
		OutputPerMillion:        25.00,
		CacheReadPerMillion:     0.50,
		CacheCreationPerMillion: 6.25,
	},
	"claude-opus-4-6": {
		InputPerMillion:         5.00,
		OutputPerMillion:        25.00,
		CacheReadPerMillion:     0.50,
		CacheCreationPerMillion: 6.25,
	},
	"claude-opus-4-5": {
		InputPerMillion:         5.00,
		OutputPerMillion:        25.00,
		CacheReadPerMillion:     0.50,
		CacheCreationPerMillion: 6.25,
	},

	// Claude 5 Fable
	"claude-fable-5": {
		InputPerMillion:         10.00,
		OutputPerMillion:        50.00,
		CacheReadPerMillion:     1.00,
		CacheCreationPerMillion: 12.50,
	},

	// Claude 5 Sonnet (introductory pricing through Aug 2026)
	"claude-sonnet-5": {
		InputPerMillion:         2.00,
		OutputPerMillion:        10.00,
		CacheReadPerMillion:     0.20,
		CacheCreationPerMillion: 2.50,
	},

	// Claude 4.5 Haiku
	"claude-haiku-4-5": {
		InputPerMillion:         1.00,
		OutputPerMillion:        5.00,
		CacheReadPerMillion:     0.10,
		CacheCreationPerMillion: 1.25,
	},
}

// sortedPrefixes holds model prefixes sorted longest-first for prefix matching.
var sortedPrefixes []string

func init() {
	sortedPrefixes = make([]string, 0, len(modelPricingTable))
	for k := range modelPricingTable {
		sortedPrefixes = append(sortedPrefixes, k)
	}
	sort.Slice(sortedPrefixes, func(i, j int) bool {
		return len(sortedPrefixes[i]) > len(sortedPrefixes[j])
	})
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
