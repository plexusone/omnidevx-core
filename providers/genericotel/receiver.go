package genericotel

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	omnidevx "github.com/plexusone/omnidevx-core"
	"github.com/plexusone/omnidevx-core/store"
)

// Options configures a Receiver.
type Options struct {
	// Store receives converted events. Required.
	Store *store.Store
	// Subject is stamped onto every event.
	Subject omnidevx.SubjectRef
	// OnWrite, when set, is called after each successful batch write.
	OnWrite func(result *store.WriteResult, service string)
}

// Receiver converts OTLP/JSON metric posts into canonical events.
type Receiver struct {
	opts Options
}

// New returns a Receiver for the given options.
func New(opts Options) (*Receiver, error) {
	if opts.Store == nil {
		return nil, fmt.Errorf("genericotel: store is required")
	}
	return &Receiver{opts: opts}, nil
}

// Handler returns the HTTP handler exposing OTLP/HTTP endpoints:
// POST /v1/metrics (parsed) and POST /v1/logs (accepted and discarded, so
// exporters configured for both do not error).
func (r *Receiver) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/metrics", r.handleMetrics)
	mux.HandleFunc("POST /v1/logs", func(w http.ResponseWriter, req *http.Request) {
		_, _ = io.Copy(io.Discard, req.Body) // acknowledged, not stored
		writeOTLPOK(w)
	})
	return mux
}

func (r *Receiver) handleMetrics(w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, req.Body, 32<<20))
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}

	var payload exportMetrics
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "parse OTLP JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	events := convert(&payload, r.opts.Subject)
	if len(events) > 0 {
		result, err := r.opts.Store.Write(req.Context(), events)
		if err != nil {
			http.Error(w, "store write: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if r.opts.OnWrite != nil {
			r.opts.OnWrite(result, serviceName(&payload))
		}
	}
	writeOTLPOK(w)
}

// writeOTLPOK sends the empty JSON success response the OTLP spec expects.
func writeOTLPOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("{}"))
}

// --- OTLP/JSON wire structures (subset) ---

type exportMetrics struct {
	ResourceMetrics []struct {
		Resource struct {
			Attributes []kv `json:"attributes"`
		} `json:"resource"`
		ScopeMetrics []struct {
			Metrics []metric `json:"metrics"`
		} `json:"scopeMetrics"`
	} `json:"resourceMetrics"`
}

type metric struct {
	Name string `json:"name"`
	Sum  *struct {
		DataPoints             []dataPoint `json:"dataPoints"`
		AggregationTemporality int         `json:"aggregationTemporality"`
	} `json:"sum"`
	Gauge *struct {
		DataPoints []dataPoint `json:"dataPoints"`
	} `json:"gauge"`
}

type dataPoint struct {
	Attributes   []kv            `json:"attributes"`
	TimeUnixNano json.Number     `json:"timeUnixNano"`
	AsInt        json.Number     `json:"asInt"`
	AsDouble     json.RawMessage `json:"asDouble"`
}

type kv struct {
	Key   string `json:"key"`
	Value struct {
		StringValue *string     `json:"stringValue"`
		IntValue    json.Number `json:"intValue"`
	} `json:"value"`
}

func attrsToMap(attrs []kv) map[string]string {
	m := map[string]string{}
	for _, a := range attrs {
		switch {
		case a.Value.StringValue != nil:
			m[a.Key] = *a.Value.StringValue
		case a.Value.IntValue != "":
			m[a.Key] = a.Value.IntValue.String()
		}
	}
	return m
}

func serviceName(p *exportMetrics) string {
	for _, rm := range p.ResourceMetrics {
		if s := attrsToMap(rm.Resource.Attributes)["service.name"]; s != "" {
			return s
		}
	}
	return "unknown"
}

func (d *dataPoint) value() (float64, bool) {
	if d.AsInt != "" {
		if n, err := d.AsInt.Int64(); err == nil {
			return float64(n), true
		}
	}
	if len(d.AsDouble) > 0 {
		var f float64
		if json.Unmarshal(d.AsDouble, &f) == nil {
			return f, true
		}
	}
	return 0, false
}

func (d *dataPoint) time() time.Time {
	if n, err := d.TimeUnixNano.Int64(); err == nil && n > 0 {
		return time.Unix(0, n).UTC()
	}
	return time.Now().UTC()
}

// tokenAttrByType maps Claude Code's token.usage "type" attribute to
// canonical attribute keys.
var tokenAttrByType = map[string]string{
	"input":         omnidevx.AttrInputTokens,
	"output":        omnidevx.AttrOutputTokens,
	"cacheRead":     omnidevx.AttrCacheReadTokens,
	"cacheCreation": omnidevx.AttrCacheCreationTokens,
}

// convert maps recognized metrics to canonical events. Unrecognized metrics
// are ignored (the receiver is additive, not a general metrics database).
func convert(p *exportMetrics, subject omnidevx.SubjectRef) []omnidevx.Event {
	var events []omnidevx.Event
	for _, rm := range p.ResourceMetrics {
		res := attrsToMap(rm.Resource.Attributes)
		source := sourceFor(res["service.name"])
		for _, sm := range rm.ScopeMetrics {
			for _, m := range sm.Metrics {
				var points []dataPoint
				if m.Sum != nil {
					points = m.Sum.DataPoints
				} else if m.Gauge != nil {
					points = m.Gauge.DataPoints
				}
				for i := range points {
					if e, ok := pointEvent(m.Name, &points[i], source, subject); ok {
						events = append(events, e)
					}
				}
			}
		}
	}
	return events
}

func sourceFor(service string) omnidevx.Source {
	switch service {
	case "claude-code":
		return omnidevx.Source{Provider: "anthropic", Product: "claude-code"}
	case "codex", "codex-cli":
		return omnidevx.Source{Provider: "openai", Product: "codex-cli"}
	default:
		if service == "" {
			service = "unknown"
		}
		return omnidevx.Source{Provider: "otel", Product: service}
	}
}

func pointEvent(name string, d *dataPoint, source omnidevx.Source, subject omnidevx.SubjectRef) (omnidevx.Event, bool) {
	value, ok := d.value()
	if !ok {
		return omnidevx.Event{}, false
	}
	attrs := attrsToMap(d.Attributes)

	eventAttrs := map[string]any{"metric": name}
	switch name {
	case "claude_code.token.usage", "codex.token.usage":
		key := tokenAttrByType[attrs["type"]]
		if key == "" {
			key = omnidevx.AttrTotalTokens
		}
		eventAttrs[key] = int64(value)
	case "claude_code.cost.usage", "codex.cost.usage":
		eventAttrs[omnidevx.AttrCostUSD] = value
	default:
		return omnidevx.Event{}, false
	}
	if model := attrs["model"]; model != "" {
		eventAttrs[omnidevx.AttrModel] = model
	}

	sessionID := attrs["session.id"]
	// Deterministic ID: one datapoint = one event; retried exports dedup.
	id := fmt.Sprintf("otel:%s:%s:%s:%s:%s", source.Product, name,
		attrs["type"], sessionID, strconv.FormatInt(d.time().UnixNano(), 10))

	return omnidevx.Event{
		ID:         id,
		Type:       omnidevx.EventUsageRecorded,
		Timestamp:  d.time(),
		Subject:    subject,
		Source:     source,
		Context:    omnidevx.EventContext{SessionID: sessionID},
		Attributes: eventAttrs,
		Provenance: omnidevx.Provenance{
			CollectionMode: omnidevx.ModeOTel,
			Confidence:     1.0,
		},
	}, true
}
