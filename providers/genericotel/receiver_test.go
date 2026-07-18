package genericotel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	omnidevx "github.com/plexusone/omnidevx-core"
	"github.com/plexusone/omnidevx-core/store"
)

const sampleMetrics = `{
  "resourceMetrics": [{
    "resource": {"attributes": [
      {"key": "service.name", "value": {"stringValue": "claude-code"}},
      {"key": "user.email", "value": {"stringValue": "someone@example.com"}}
    ]},
    "scopeMetrics": [{
      "metrics": [
        {"name": "claude_code.token.usage", "sum": {"aggregationTemporality": 1, "dataPoints": [
          {"attributes": [
             {"key": "type", "value": {"stringValue": "input"}},
             {"key": "model", "value": {"stringValue": "claude-fable-5"}},
             {"key": "session.id", "value": {"stringValue": "sess-1"}}],
           "timeUnixNano": "1783000000000000000", "asInt": "1234"},
          {"attributes": [
             {"key": "type", "value": {"stringValue": "cacheRead"}},
             {"key": "model", "value": {"stringValue": "claude-fable-5"}},
             {"key": "session.id", "value": {"stringValue": "sess-1"}}],
           "timeUnixNano": "1783000000000000001", "asInt": "99000"}
        ]}},
        {"name": "claude_code.cost.usage", "sum": {"aggregationTemporality": 1, "dataPoints": [
          {"attributes": [
             {"key": "model", "value": {"stringValue": "claude-fable-5"}},
             {"key": "session.id", "value": {"stringValue": "sess-1"}}],
           "timeUnixNano": "1783000000000000002", "asDouble": 0.42}
        ]}},
        {"name": "claude_code.lines_of_code.count", "sum": {"aggregationTemporality": 1, "dataPoints": [
          {"attributes": [], "timeUnixNano": "1783000000000000003", "asInt": "10"}
        ]}}
      ]
    }]
  }]
}`

func newTestReceiver(t *testing.T) (*Receiver, *store.Store) {
	t.Helper()
	s, err := store.Open(store.Options{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	r, err := New(Options{Store: s, Subject: omnidevx.SubjectRef{PersonID: "person:test"}})
	if err != nil {
		t.Fatal(err)
	}
	return r, s
}

func post(t *testing.T, handler http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

func TestMetricsIngestion(t *testing.T) {
	r, s := newTestReceiver(t)
	w := post(t, r.Handler(), "/v1/metrics", sampleMetrics)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d: %s", w.Code, w.Body.String())
	}

	read, err := s.Read(context.Background(), store.Query{})
	if err != nil {
		t.Fatal(err)
	}
	// token input + token cacheRead + cost; lines_of_code ignored.
	if len(read.Events) != 3 {
		t.Fatalf("events: got %d, want 3: %+v", len(read.Events), read.Events)
	}
	for _, e := range read.Events {
		if e.Type != omnidevx.EventUsageRecorded {
			t.Errorf("type: got %s", e.Type)
		}
		if e.Provenance.CollectionMode != omnidevx.ModeOTel || e.Provenance.Confidence != 1.0 {
			t.Errorf("provenance: %+v", e.Provenance)
		}
		if e.Context.SessionID != "sess-1" {
			t.Errorf("session: got %q", e.Context.SessionID)
		}
		if e.Source.Product != "claude-code" {
			t.Errorf("source: %+v", e.Source)
		}
		if e.Subject.PersonID != "person:test" {
			t.Errorf("subject: %+v", e.Subject)
		}
		// Privacy: resource attributes like user.email must not leak.
		for k, v := range e.Attributes {
			if s, ok := v.(string); ok && strings.Contains(s, "@example.com") {
				t.Errorf("email leaked via %q", k)
			}
		}
	}

	var sawInput, sawCache, sawCost bool
	for _, e := range read.Events {
		if v, ok := e.Attributes[omnidevx.AttrInputTokens]; ok && v == float64(1234) {
			sawInput = true
		}
		if _, ok := e.Attributes[omnidevx.AttrCacheReadTokens]; ok {
			sawCache = true
		}
		if v, ok := e.Attributes[omnidevx.AttrCostUSD]; ok {
			sawCost = true
			if f, isF := v.(float64); !isF || f != 0.42 {
				t.Errorf("cost: got %v", v)
			}
		}
	}
	if !sawInput || !sawCache || !sawCost {
		t.Errorf("missing expected attrs: input=%v cache=%v cost=%v", sawInput, sawCache, sawCost)
	}
}

func TestIngestionIsIdempotent(t *testing.T) {
	r, s := newTestReceiver(t)
	post(t, r.Handler(), "/v1/metrics", sampleMetrics)
	post(t, r.Handler(), "/v1/metrics", sampleMetrics) // retried export

	read, err := s.Read(context.Background(), store.Query{})
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Events) != 3 {
		t.Errorf("after retry: got %d events, want 3 (dedup)", len(read.Events))
	}
}

func TestLogsAcceptedAndDiscarded(t *testing.T) {
	r, s := newTestReceiver(t)
	w := post(t, r.Handler(), "/v1/logs", `{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"body":{"stringValue":"secret prompt text"}}]}]}]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("logs status: got %d", w.Code)
	}
	read, err := s.Read(context.Background(), store.Query{})
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Events) != 0 {
		t.Errorf("logs must not be stored: got %d events", len(read.Events))
	}
}

func TestMalformedPayload(t *testing.T) {
	r, _ := newTestReceiver(t)
	w := post(t, r.Handler(), "/v1/metrics", "not json")
	if w.Code != http.StatusBadRequest {
		t.Errorf("malformed: got %d, want 400", w.Code)
	}
}
