// Package genericotel receives OpenTelemetry metrics over OTLP/HTTP (JSON
// encoding) and converts them to canonical OmniDevX events, written directly
// to the local event store. It exists so AI coding agents with native OTel
// export (Claude Code, Codex CLI) can stream token usage and cost live,
// instead of relying on retrospective session-history imports that are
// subject to local retention limits.
//
// Only the JSON encoding of OTLP is supported (configure exporters with
// OTEL_EXPORTER_OTLP_PROTOCOL=http/json); protobuf would require heavy
// dependencies that do not belong in omnidevx-core.
//
// Metadata only: metric names, numeric values, models, session identifiers.
// No log bodies or content attributes are stored.
package genericotel
