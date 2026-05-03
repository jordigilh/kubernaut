# ADR-010: Load Testing Tool Selection

**Status:** Accepted
**Date:** 2026-05-03
**Deciders:** AF team
**Source:** #43 design

## Context

AF needs load/stress testing to validate SLO targets under realistic workloads. The service is SSE-heavy (long-lived connections for investigation streaming), uses JSON-RPC protocols (MCP, A2A), and has LLM-dependent latency characteristics.

## Decision

Use **k6 (Grafana)** as the primary load testing tool, supplemented by a thin Go test harness for protocol-specific edge cases.

## Consequences

- k6 has native SSE support via `k6/experimental/streams` — critical for testing AF's streaming architecture
- Prometheus remote write exports metrics directly to the same observability stack
- JavaScript scripts are accessible to the entire team (low learning curve)
- k6-operator enables execution on K8s when dedicated hardware is available
- Thresholds map directly to SLO definitions (same metric names, same percentiles)
- Go supplement handles MCP/A2A JSON-RPC session management edge cases
- `experimental/streams` API may change — acceptable risk given k6's release cadence

## Alternatives Considered

| Alternative | Why rejected |
|-------------|-------------|
| vegeta (Go) | No SSE support; HTTP-only |
| locust (Python) | SSE support is manual; team is Go-native |
| Custom Go harness (only) | Full framework maintenance burden; reinventing k6's VU scheduling |
| Gatling | JVM-based; heavy for the team's stack; licensing concerns |
