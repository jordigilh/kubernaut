# BR-SP-106: Predictive Signal Mode Classification

**Document Version**: 1.0
**Date**: February 8, 2026
**Status**: ✅ APPROVED
**Category**: Classification
**Priority**: P1 (High)
**Service**: SignalProcessing
**GitHub Issue**: [#55](https://github.com/jordigilh/kubernaut/issues/55)
**Related**: BR-AI-084, DD-WORKFLOW-001, DD-WORKFLOW-015

---

## Business Context

### Problem Statement

Kubernaut is reactive by design: it processes signals (Prometheus alerts, Kubernetes events) that represent incidents that have **already occurred**. However, enterprise environments need preemptive remediation for predicted incidents — e.g., Prometheus `predict_linear()` alerts that fire **before** resource exhaustion.

The challenge is that predictive signals (e.g., `PredictedOOMKill`) don't match any workflow in the catalog, because workflows are registered under their base signal type (e.g., `OOMKilled`). Additionally, the downstream AI investigation must know whether to perform root cause analysis (reactive) or evaluate current environment for preemptive action (predictive).

### Business Value

1. **Proactive incident prevention**: Remediate before impact, not after
2. **ROI proof**: Track predictive vs. reactive remediations separately in the Effectiveness Monitor ("How often did predictions prevent incidents?")
3. **Enterprise differentiation**: Closes the "predictive capabilities" gap identified in enterprise feedback
4. **Zero-code Prometheus integration**: `predict_linear()` alerting rules require no Kubernaut code changes to generate signals

---

## Requirements

### R1: Signal Mode Status Field

SignalProcessing CRD status MUST include a `SignalMode` field with values:
- `reactive` — Standard alert processing (incident has occurred)
- `predictive` — Predicted incident (has not yet occurred)

The field is **required** (not optional) — all signals MUST be classified.

### R2: Signal Type Normalization

SignalProcessing MUST normalize predictive signal types to their base type for workflow catalog matching:

| Incoming Signal Type | Normalized Type | Signal Mode |
|---|---|---|
| `PredictedOOMKill` | `OOMKilled` | `predictive` |
| `PredictedCPUThrottling` | `CPUThrottling` | `predictive` |
| `PredictedDiskPressure` | `DiskPressure` | `predictive` |
| `PredictedNodeNotReady` | `NodeNotReady` | `predictive` |
| `OOMKilled` | `OOMKilled` | `reactive` |
| _(any unmapped type)_ | _(unchanged)_ | `reactive` |

The original signal type MUST be preserved in the SP status for audit trail purposes.

### R3: Configurable Signal Type Mappings

The predictive-to-base signal type mappings MUST be loaded from an operator-configurable YAML file, supporting hot-reload (per BR-SP-072 pattern).

```yaml
# config/signalprocessing/predictive-signal-mappings.yaml
predictive_signal_mappings:
  PredictedOOMKill: OOMKilled
  PredictedCPUThrottling: CPUThrottling
  PredictedDiskPressure: DiskPressure
  PredictedNodeNotReady: NodeNotReady
```

### R4: Unknown Predictive Types

If a signal type is not found in the mapping config:
- Classify as `reactive` (default)
- Preserve original signal type unchanged
- Log a warning for operator visibility

### R5: Enrichment Pipeline Integration

`SignalMode` MUST be set during the enrichment phase, alongside severity, environment, and priority classification. It follows the same status update pattern (atomic status update per DD-PERF-001).

---

## Data Flow

```
Prometheus predict_linear() alert
  → Gateway (receives PredictedOOMKill)
    → SignalProcessing (normalizes to OOMKilled + signalMode: predictive)
      → RO (copies signalMode from SP status to AA spec)
        → AIAnalysis (passes signalMode to HAPI)
          → HAPI (switches prompt: RCA vs. environment evaluation)
```

### Key Design Decision: No CRD Labels

`SignalMode` lives in the CRD **status** (SP) and **spec** (AA), NOT in labels. This avoids CRD label schema changes and keeps the classification as internal pipeline context.

---

## Acceptance Criteria

- [ ] CRD status field: `Status.SignalMode` (string: `reactive` | `predictive`) in `api/signalprocessing/v1alpha1/signalprocessing_types.go`
- [ ] Signal type normalization: configurable mapping applied during enrichment
- [ ] Original signal type preserved in `Status.OriginalSignalType` for audit
- [ ] Config file: `predictive-signal-mappings.yaml` (hot-reloadable per BR-SP-072)
- [ ] Default initial mappings: OOMKill, CPUThrottling, DiskPressure, NodeNotReady
- [ ] Unknown predictive types: classify as `reactive`, log warning
- [ ] Enrichment pipeline integration: set alongside severity, environment, priority
- [ ] `make generate` regenerates deepcopy successfully

---

## Implementation Points

| Component | File(s) | Change |
|---|---|---|
| SP CRD status | `api/signalprocessing/v1alpha1/signalprocessing_types.go` | Add `SignalMode`, `OriginalSignalType` fields |
| SP enrichment | `internal/controller/signalprocessing/signalprocessing_controller.go` | Set `SignalMode` during enrichment |
| SP classifier | `pkg/signalprocessing/classifier/` (new file) | Signal mode classification logic |
| SP config | `config/signalprocessing/predictive-signal-mappings.yaml` | Mapping config |
| Deepcopy | `api/signalprocessing/v1alpha1/zz_generated.deepcopy.go` | `make generate` |

---

## Test Plan

### Unit Tests
- Table-driven tests for signal type mapping (known types, unknown types, empty input)
- Classification logic: predictive signals normalized correctly
- Config loading and hot-reload
- Default reactive classification for unmapped types

### Integration Tests
- Extend existing enrichment integration tests with predictive signal scenarios
- Verify `SignalMode` set in SP status after enrichment completes

### E2E Tests
- Full pipeline: predictive alert → SP enrichment → verify normalized signal type + signalMode in status

---

## Example Prometheus Alerting Rules

These rules generate predictive signals that flow into Kubernaut with zero code changes:

```yaml
groups:
  - name: kubernaut-predictive
    rules:
      - alert: PredictedOOMKill
        expr: predict_linear(container_memory_working_set_bytes[1h], 1800) > container_spec_memory_limit_bytes
        for: 5m
        labels:
          severity: warning
          signal_type: PredictedOOMKill
          kubernaut.ai/managed: "true"
        annotations:
          summary: "Container {{ $labels.container }} predicted to OOM in ~30min"

      - alert: PredictedDiskPressure
        expr: predict_linear(node_filesystem_avail_bytes[6h], 14400) < 0
        for: 10m
        labels:
          severity: warning
          signal_type: PredictedDiskPressure
          kubernaut.ai/managed: "true"
        annotations:
          summary: "Node {{ $labels.node }} predicted to exhaust disk in ~4 hours"
```

---

## Audit Trail Integration

- Audit events MUST record `signalMode` to enable Effectiveness Monitor tracking
- Enables answering: "How often did predictive remediations prevent actual incidents?"
- Supports enterprise ROI proof requirements

---

## References

### Prometheus Documentation

- [predict_linear() function reference](https://prometheus.io/docs/prometheus/latest/querying/functions/#predict_linear) — Official PromQL documentation for the `predict_linear(v range-vector, t scalar)` function. Uses simple linear regression to predict future metric values based on a time series range.
- [Prometheus Alerting Rules](https://prometheus.io/docs/prometheus/latest/configuration/alerting_rules/) — Alerting rule configuration that generates the predictive signals consumed by Kubernaut.
- [Prometheus Recording Rules](https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/) — Recording rules can pre-compute `predict_linear()` expressions for efficiency at scale.
- [Prometheus Best Practices: Alerting](https://prometheus.io/docs/practices/alerting/) — Guidelines for designing effective alerting rules, including predictive alerting patterns.

### Key `predict_linear()` Considerations

- **Best for gauge metrics**: `predict_linear()` uses simple linear regression and works best with gauges (memory usage, disk space, connection counts), not counters.
- **Range window selection**: The range window should be approximately 4-5x the prediction horizon. For a 30-minute prediction, use `[2h]` of historical data. For a 4-hour prediction, use `[1d]`.
- **Combine with `for` clause**: Use `for: 5m` or `for: 10m` in alerting rules to avoid false positives from temporary spikes. This ensures the prediction is sustained before firing.
- **Not suitable for periodic metrics**: CPU usage, request rates, and other metrics with periodic patterns (daily/weekly cycles) will produce poor predictions with linear regression. Use `double_exponential_smoothing()` (Prometheus 3.x) for seasonal data.

### Related Documents

- [BR-AI-084: Predictive Signal Mode Prompt Strategy](BR-AI-084-predictive-signal-mode-prompt-strategy.md)
- [Issue #55: Predictive remediation pipeline](https://github.com/jordigilh/kubernaut/issues/55)
- [DD-WORKFLOW-001: Mandatory Label Schema](../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md)
- [SP Business Requirements](../services/crd-controllers/01-signalprocessing/BUSINESS_REQUIREMENTS.md)
