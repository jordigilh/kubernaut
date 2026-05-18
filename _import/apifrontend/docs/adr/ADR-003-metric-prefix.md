# ADR-003: Prometheus Metric Prefix

**Status:** Accepted
**Date:** 2026-05-03
**Deciders:** AF team
**Source:** #41 SLO reconciliation

## Context

The issue body (#41) originally used `apifrontend_*` as the metric prefix. QE-3 performance targets used `af_*`. We needed a canonical prefix for all Prometheus metrics.

## Decision

Use `af_` as the canonical Prometheus metric prefix for all API Frontend metrics.

## Consequences

- Short prefix is easier in PromQL queries (`af_triage_duration_seconds` vs `apifrontend_triage_duration_seconds`)
- Consistent with kubernaut ecosystem patterns (kubernaut uses short prefixes)
- Less truncation risk in Grafana panel legends
- Unique enough within a cluster (no collision with other `af_` metrics in the k8s ecosystem)
- All metrics registered with this prefix: histograms, counters, gauges (~20 metrics total)

## Alternatives Considered

| Alternative | Why rejected |
|-------------|-------------|
| `apifrontend_` | Too long for PromQL ergonomics; truncated in Grafana legends |
| `kaf_` (kubernaut-apifrontend) | Unfamiliar abbreviation; not self-evident |
| `kubernaut_af_` | Redundant given cluster context; very long |
