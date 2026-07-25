package report

import (
	"sort"
	"strings"
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
// "claude-opus-4". The [1m] suffix (extended context) uses the same rates.
//
// Prices from Anthropic's published API pricing (per 1M tokens):
// https://www.anthropic.com/pricing
var modelPricingTable = map[string]ModelPricing{
	// Claude 4 Opus family
	"claude-opus-4-8": {
		InputPerMillion:         15.00,
		OutputPerMillion:        75.00,
		CacheReadPerMillion:     1.50,
		CacheCreationPerMillion: 18.75,
	},
	"claude-opus-4-6": {
		InputPerMillion:         15.00,
		OutputPerMillion:        75.00,
		CacheReadPerMillion:     1.50,
		CacheCreationPerMillion: 18.75,
	},
	"claude-opus-4-5": {
		InputPerMillion:         15.00,
		OutputPerMillion:        75.00,
		CacheReadPerMillion:     1.50,
		CacheCreationPerMillion: 18.75,
	},

	// Claude 5 Sonnet
	"claude-sonnet-5": {
		InputPerMillion:         3.00,
		OutputPerMillion:        15.00,
		CacheReadPerMillion:     0.30,
		CacheCreationPerMillion: 3.75,
	},

	// Claude 5 Fable (research/creative model)
	"claude-fable-5": {
		InputPerMillion:         3.00,
		OutputPerMillion:        15.00,
		CacheReadPerMillion:     0.30,
		CacheCreationPerMillion: 3.75,
	},

	// Claude 4.5 Haiku
	"claude-haiku-4-5": {
		InputPerMillion:         0.80,
		OutputPerMillion:        4.00,
		CacheReadPerMillion:     0.08,
		CacheCreationPerMillion: 1.00,
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
// then longest-prefix match (so "claude-opus-4-8[1m]" matches "claude-opus-4-8").
// Returns false if no matching pricing is found.
func LookupPricing(model string) (ModelPricing, bool) {
	// Exact match
	if p, ok := modelPricingTable[model]; ok {
		return p, true
	}
	// Longest-prefix match
	for _, prefix := range sortedPrefixes {
		if strings.HasPrefix(model, prefix) {
			return modelPricingTable[prefix], true
		}
	}
	return ModelPricing{}, false
}

// EstimateCost computes the USD cost from token counts using model pricing.
func EstimateCost(p ModelPricing, input, output, cacheRead, cacheCreation int64) float64 {
	const perMillion = 1_000_000.0
	return float64(input)/perMillion*p.InputPerMillion +
		float64(output)/perMillion*p.OutputPerMillion +
		float64(cacheRead)/perMillion*p.CacheReadPerMillion +
		float64(cacheCreation)/perMillion*p.CacheCreationPerMillion
}
