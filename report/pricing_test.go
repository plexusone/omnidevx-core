package report

import (
	"math"
	"testing"
)

func TestLookupPricing_ExactMatch(t *testing.T) {
	p, ok := LookupPricing("claude-opus-4-8")
	if !ok {
		t.Fatal("expected exact match for claude-opus-4-8")
	}
	if p.InputPerMillion != 5.00 {
		t.Errorf("InputPerMillion = %v, want 5.00", p.InputPerMillion)
	}
	if p.OutputPerMillion != 25.00 {
		t.Errorf("OutputPerMillion = %v, want 25.00", p.OutputPerMillion)
	}
}

func TestLookupPricing_PrefixMatch(t *testing.T) {
	// The [1m] suffix indicates extended context window, same pricing
	p, ok := LookupPricing("claude-opus-4-8[1m]")
	if !ok {
		t.Fatal("expected prefix match for claude-opus-4-8[1m]")
	}
	if p.InputPerMillion != 5.00 {
		t.Errorf("InputPerMillion = %v, want 5.00", p.InputPerMillion)
	}

	// Version suffix should also match
	p2, ok := LookupPricing("claude-haiku-4-5-20251001")
	if !ok {
		t.Fatal("expected prefix match for claude-haiku-4-5-20251001")
	}
	if p2.InputPerMillion != 1.00 {
		t.Errorf("InputPerMillion = %v, want 1.00", p2.InputPerMillion)
	}
}

func TestLookupPricing_LongestPrefixWins(t *testing.T) {
	// claude-opus-4-8 should match with -YYYYMMDD suffix
	p, ok := LookupPricing("claude-opus-4-8-20260101")
	if !ok {
		t.Fatal("expected match for claude-opus-4-8-20260101")
	}
	// Should match claude-opus-4-8 (Opus pricing)
	if p.InputPerMillion != 5.00 {
		t.Errorf("InputPerMillion = %v, want 5.00 (claude-opus-4-8)", p.InputPerMillion)
	}

	// claude-opus-4-7 exists with same pricing, verify it matches correctly
	p2, ok := LookupPricing("claude-opus-4-7[1m]")
	if !ok {
		t.Fatal("expected match for claude-opus-4-7[1m]")
	}
	if p2.InputPerMillion != 5.00 {
		t.Errorf("InputPerMillion = %v, want 5.00 (claude-opus-4-7)", p2.InputPerMillion)
	}
}

func TestLookupPricing_BoundaryValidation(t *testing.T) {
	// claude-opus-4-80 should NOT match claude-opus-4-8 (no valid boundary)
	_, ok := LookupPricing("claude-opus-4-80")
	if ok {
		t.Error("claude-opus-4-80 should NOT match claude-opus-4-8")
	}

	// claude-opus-4-8x should NOT match
	_, ok = LookupPricing("claude-opus-4-8x")
	if ok {
		t.Error("claude-opus-4-8x should NOT match claude-opus-4-8")
	}

	// claude-haiku-4-50 should NOT match claude-haiku-4-5
	_, ok = LookupPricing("claude-haiku-4-50")
	if ok {
		t.Error("claude-haiku-4-50 should NOT match claude-haiku-4-5")
	}
}

func TestLookupPricing_NoMatch(t *testing.T) {
	_, ok := LookupPricing("gpt-4")
	if ok {
		t.Error("expected no match for gpt-4")
	}

	_, ok = LookupPricing("")
	if ok {
		t.Error("expected no match for empty string")
	}
}

func TestEstimateCost_Basic(t *testing.T) {
	p := ModelPricing{
		InputPerMillion:         15.00,
		OutputPerMillion:        75.00,
		CacheReadPerMillion:     1.50,
		CacheCreationPerMillion: 18.75,
	}

	// 1M tokens of each type
	cost := EstimateCost(p, 1_000_000, 1_000_000, 1_000_000, 1_000_000)
	expected := 15.00 + 75.00 + 1.50 + 18.75
	if math.Abs(cost-expected) > 0.001 {
		t.Errorf("EstimateCost = %v, want %v", cost, expected)
	}
}

func TestEstimateCost_FractionalTokens(t *testing.T) {
	p := ModelPricing{
		InputPerMillion:  15.00,
		OutputPerMillion: 75.00,
	}

	// 1000 input tokens = $0.015, 500 output tokens = $0.0375
	cost := EstimateCost(p, 1000, 500, 0, 0)
	expected := 0.015 + 0.0375
	if math.Abs(cost-expected) > 0.0001 {
		t.Errorf("EstimateCost = %v, want %v", cost, expected)
	}
}

func TestEstimateCost_ZeroTokens(t *testing.T) {
	p, _ := LookupPricing("claude-opus-4-8")
	cost := EstimateCost(p, 0, 0, 0, 0)
	if cost != 0 {
		t.Errorf("EstimateCost with zero tokens = %v, want 0", cost)
	}
}

func TestEstimateCost_RealisticSession(t *testing.T) {
	// Typical Claude Code session: 50k input (with cache), 5k output
	p, ok := LookupPricing("claude-opus-4-6")
	if !ok {
		t.Fatal("expected to find claude-opus-4-6")
	}

	// 50k input, 5k output, 45k cache read, 5k cache creation
	cost := EstimateCost(p, 50_000, 5_000, 45_000, 5_000)

	// Expected (Opus 4.6 pricing: $5 input, $25 output, $0.50 cache read, $6.25 cache creation):
	// input: 50k/1M * $5 = $0.25
	// output: 5k/1M * $25 = $0.125
	// cache_read: 45k/1M * $0.50 = $0.0225
	// cache_creation: 5k/1M * $6.25 = $0.03125
	expected := 0.25 + 0.125 + 0.0225 + 0.03125
	if math.Abs(cost-expected) > 0.0001 {
		t.Errorf("EstimateCost = %v, want %v", cost, expected)
	}
}

func TestAllModelsHaveValidPricing(t *testing.T) {
	models := []string{
		"claude-opus-4-8",
		"claude-opus-4-6",
		"claude-opus-4-5",
		"claude-sonnet-5",
		"claude-fable-5",
		"claude-haiku-4-5",
	}
	for _, m := range models {
		p, ok := LookupPricing(m)
		if !ok {
			t.Errorf("missing pricing for %s", m)
			continue
		}
		if p.InputPerMillion <= 0 {
			t.Errorf("%s: InputPerMillion should be positive", m)
		}
		if p.OutputPerMillion <= 0 {
			t.Errorf("%s: OutputPerMillion should be positive", m)
		}
	}
}
