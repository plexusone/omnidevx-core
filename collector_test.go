package omnidevx

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestPeriodContains(t *testing.T) {
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name   string
		period Period
		t      time.Time
		want   bool
	}{
		{"inside", Period{Start: start, End: end}, start.Add(time.Hour), true},
		{"start inclusive", Period{Start: start, End: end}, start, true},
		{"end exclusive", Period{Start: start, End: end}, end, false},
		{"before", Period{Start: start, End: end}, start.Add(-time.Second), false},
		{"open start", Period{End: end}, start.Add(-24 * time.Hour), true},
		{"open end", Period{Start: start}, end.Add(24 * time.Hour), true},
		{"zero period contains all", Period{}, time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC), true},
	}
	for _, c := range cases {
		if got := c.period.Contains(c.t); got != c.want {
			t.Errorf("%s: Contains(%s) = %v, want %v", c.name, c.t, got, c.want)
		}
	}
}

func TestEventJSONOmitsZeroStructs(t *testing.T) {
	e := Event{
		ID:        "x:1",
		Type:      EventPromptSubmitted,
		Timestamp: time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC),
		Source:    Source{Provider: "anthropic", Product: "claude-code"},
		Provenance: Provenance{
			CollectionMode: ModeHistory,
			Confidence:     0.9,
		},
	}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, field := range []string{"subject", "context", "attributes"} {
		if strings.Contains(s, `"`+field+`"`) {
			t.Errorf("zero %s should be omitted from JSON: %s", field, s)
		}
	}

	var back Event
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.ID != e.ID || back.Type != e.Type || !back.Timestamp.Equal(e.Timestamp) ||
		back.Source != e.Source || back.Provenance != e.Provenance {
		t.Errorf("round trip mismatch: %+v vs %+v", back, e)
	}
}
