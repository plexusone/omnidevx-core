# OpenTelemetry Receiver

`providers/genericotel` receives OTLP/HTTP **JSON** metrics from AI coding
agents and writes `ai.usage.recorded` events (otel-mode provenance,
confidence 1.0) directly to the event store. It exists because local
session history is subject to retention — live capture is the durable
path for token and cost data.

Only the JSON encoding is supported: protobuf would pull heavy
dependencies into core. Configure exporters with
`OTEL_EXPORTER_OTLP_PROTOCOL=http/json`.

## Usage

```go
r, err := genericotel.New(genericotel.Options{
	Store:   s, // *store.Store, required
	Subject: omnidevx.SubjectRef{PersonID: "person:jane"},
})
http.ListenAndServe("127.0.0.1:4318", r.Handler())
```

`Handler` serves `POST /v1/metrics` (parsed) and `POST /v1/logs`
(acknowledged and discarded — log bodies can contain prompt text and are
never stored). For a ready-made daemon with launchd setup, a query API,
and Grafana dashboards, see
[`omnidevx-otel`](https://github.com/plexusone/omnidevx-otel).

## Recognized metrics

| Metric | Mapped to |
|--------|-----------|
| `claude_code.token.usage` | Token attributes by `type` datapoint attribute (`input`, `output`, `cacheRead`, `cacheCreation`) |
| `claude_code.cost.usage` | `cost_usd` |
| `codex.token.usage`, `codex.cost.usage` | Same mapping for Codex CLI |

Unrecognized metrics are ignored — the receiver is additive, not a
general metrics database. Datapoint `model` and `session.id` attributes
carry through; resource attributes like `user.email` do not.

## Claude Code configuration

`~/.claude/settings.json`:

```json
{
  "env": {
    "CLAUDE_CODE_ENABLE_TELEMETRY": "1",
    "OTEL_METRICS_EXPORTER": "otlp",
    "OTEL_EXPORTER_OTLP_PROTOCOL": "http/json",
    "OTEL_EXPORTER_OTLP_ENDPOINT": "http://127.0.0.1:4318"
  }
}
```

The SDK appends `/v1/metrics` to the endpoint; export interval defaults
to 60s. Retried exports deduplicate via deterministic event IDs.
