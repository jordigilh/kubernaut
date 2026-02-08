# ADR-054: Predictive Signal Mode Classification

**Status**: ✅ APPROVED
**Date**: February 8, 2026
**Deciders**: Platform Team, SignalProcessing Team, AIAnalysis Team
**Confidence**: 85%

---

## Context

### Problem Statement

Kubernaut is reactive by design: it processes signals representing incidents that have **already occurred** (Prometheus alerts, Kubernetes events). Enterprise environments need preemptive remediation for **predicted** incidents — for example, Prometheus `predict_linear()` alerts that fire before resource exhaustion.

Two problems block predictive signal support today:

1. **Workflow catalog mismatch**: Predictive signal types (e.g., `PredictedOOMKill`) don't match any workflow in the catalog, which registers workflows under base signal types (e.g., `OOMKilled`). The workflow catalog uses `signal_type` as a mandatory label filter (DD-WORKFLOW-001).

2. **Wrong investigation strategy**: HolmesGPT performs root cause analysis ("what happened?") for all signals. For a predicted incident that hasn't occurred, RCA produces irrelevant results ("no error logs found"). The AI agent needs a different directive: "evaluate current environment and determine if preemptive action is warranted."

### Business Requirements

This ADR implements two Business Requirements:
- **BR-SP-106**: Predictive Signal Mode Classification (Signal Processing)
- **BR-AI-084**: Predictive Signal Mode Prompt Strategy (AIAnalysis + HAPI)

### Constraints

- No CRD label changes — `signalMode` lives in status (SP) and spec (AA), not labels
- Signal type normalization must preserve the original signal type for audit trail
- Predictive mode must allow "no action needed" as a valid LLM outcome
- v1.0 — no backwards compatibility concerns (not yet released)

---

## Decision

### 1. Signal Mode Classification in Signal Processing

**Chosen**: SP classifies all signals as `reactive` (default) or `predictive` based on a configurable signal type mapping, and normalizes predictive signal types to their base type.

**Classification Logic**:
```
Input: PredictedOOMKill
  → Lookup in predictive-signal-mappings.yaml
  → Found: PredictedOOMKill → OOMKilled
  → Set: status.signalType = "OOMKilled" (normalized for workflow catalog)
  → Set: status.signalMode = "predictive"
  → Set: status.originalSignalType = "PredictedOOMKill" (preserved for audit)

Input: OOMKilled
  → Lookup in predictive-signal-mappings.yaml
  → Not found (not a predictive type)
  → Set: status.signalType = "OOMKilled" (unchanged)
  → Set: status.signalMode = "reactive"
```

**Configuration**:
```yaml
# config/signalprocessing/predictive-signal-mappings.yaml
predictive_signal_mappings:
  PredictedOOMKill: OOMKilled
  PredictedCPUThrottling: CPUThrottling
  PredictedDiskPressure: DiskPressure
  PredictedNodeNotReady: NodeNotReady
```

**Rationale**:
- **Config-driven**: Operators can add new predictive signal types without code changes
- **Hot-reloadable**: Follows BR-SP-072 pattern (fsnotify-based ConfigMap reload)
- **Safe default**: Unknown signal types default to `reactive`, no workflow disruption
- **Audit preserving**: Original signal type retained for full traceability

---

### 2. Pipeline Data Flow

**Chosen**: `signalMode` flows through the existing pipeline using established copy patterns — no new wiring required.

```
Prometheus predict_linear() alert
  → Gateway (receives PredictedOOMKill, passes through unchanged)
    → Signal Processing
        status.signalMode = "predictive"
        status.signalType = "OOMKilled" (normalized)
        status.originalSignalType = "PredictedOOMKill"
      → Remediation Orchestrator
          copies sp.Status.SignalMode → aa.Spec.SignalContext.SignalMode
          (same pattern as severity, environment, priority)
        → AI Analysis
            passes signalMode to HAPI in IncidentRequest
          → HolmesGPT API
              switches prompt strategy based on signal_mode
```

**Rationale**:
- **Zero new wiring**: Every hop already exists for severity/environment/priority
- **RO is the bridge**: RO already copies SP status fields to AA spec in `buildSignalContext()`
- **AA is the caller**: AA already builds `IncidentRequest` from spec fields in `BuildIncidentRequest()`

---

### 3. HAPI Prompt Strategy

**Chosen**: HAPI switches its investigation prompt based on `signal_mode`, with two distinct directives.

**Reactive** (default — incident has occurred):
> Perform root cause analysis. The incident has occurred. Investigate logs, events, and resource state to determine the root cause and recommend remediation.

**Predictive** (incident predicted, not yet occurred):
> Evaluate current environment. This incident is predicted based on resource trend analysis but has not occurred yet. Assess resource utilization trends, recent deployments, and current state to determine if preemptive action is warranted. "No action needed" is a valid outcome if the prediction is unlikely to materialize.

**Rationale**:
- **Clear directive**: The LLM knows exactly what investigation mode to use
- **Valid "no action"**: Predictive mode explicitly allows the LLM to conclude no preemptive action is needed (trend reversal, temporary spike, etc.)
- **Audit differentiation**: Enables Effectiveness Monitor to track predictive vs. reactive outcomes separately

---

### 4. No CRD Label Changes

**Chosen**: `signalMode` is a CRD **status** field (SP) and **spec** field (AA), not a label.

**Rationale**:
- Labels are part of the CRD identity and affect label selectors, informers, and field selectors
- `signalMode` is internal pipeline context, not a selection criterion
- Avoids DD-WORKFLOW-001 label schema changes
- Status/spec fields are simpler to add and don't affect Kubernetes API behavior

---

## Alternatives Considered

### Alternative A: Workflow Catalog Signal Type Aliases

Register workflows with multiple signal types (e.g., `signal-type-alias: PredictedOOMKill`).

**Rejected because**:
- Requires DD-WORKFLOW-001 schema changes
- Every workflow must be updated with predictive aliases
- DataStorage search query changes needed
- Doesn't solve the prompt strategy problem (HAPI still wouldn't know to switch investigation mode)

### Alternative B: Gateway Normalizes Signal Type

Gateway performs the predictive-to-base signal type mapping before creating the SP CRD.

**Rejected because**:
- Gateway's role is signal ingestion and deduplication, not classification
- Classification belongs in Signal Processing (established responsibility boundary)
- Gateway would need to maintain signal type mapping config (wrong layer)
- SP already has the enrichment pipeline infrastructure (hot-reload, Rego engine, etc.)

### Alternative C: HAPI Infers Predictive Mode from Signal Type Name

HAPI checks if the signal type starts with "Predicted" and adjusts its prompt.

**Rejected because**:
- Fragile string-based convention
- No explicit pipeline signal — implicit behavior is error-prone
- Doesn't generalize to non-"Predicted" naming patterns
- Violates separation of concerns (classification is SP's job)

---

## Consequences

### Positive

1. **Immediate value with zero code changes**: Prometheus `predict_linear()` alerting rules generate predictive signals today. Even without the pipeline enhancement, these alerts flow through Kubernaut and trigger standard remediation.
2. **Incremental enhancement**: The pipeline changes (SP → RO → AA → HAPI) follow existing patterns, minimizing implementation risk.
3. **Enterprise ROI proof**: Predictive vs. reactive tracking in audit events enables the Effectiveness Monitor to answer "How often did predictions prevent incidents?"
4. **Extensible**: New predictive signal types added via config, not code.

### Negative

1. **Prompt engineering iteration**: The predictive prompt will need tuning against real scenarios. Mitigated by prompt being a configuration string, not compiled code.
2. **Linear regression limitations**: `predict_linear()` is a simple linear model — poor for periodic metrics (CPU, request rate). Documented in BR-SP-106 as a known constraint. Future enhancement could integrate `double_exponential_smoothing()` (Prometheus 3.x) for seasonal data.
3. **Config maintenance**: Signal type mappings must be maintained as new alert types are added. Mitigated by hot-reload and operator documentation.

### Neutral

- No impact on existing reactive signal processing — `reactive` is the default
- No CRD schema breaking changes
- No new infrastructure dependencies

---

## Implementation

### Estimated Effort: 6-8 days (1 developer), 4-5 days (2 developers)

| Phase | Days | Details |
|---|---|---|
| Production code | 2-3 | SP CRD + enrichment, RO copy, AA builder, HAPI OpenAPI + prompt |
| Testing | 3-4 | Unit (SP, RO, AA, HAPI), integration, E2E full pipeline |
| Config + docs | 0.5 | Mapping config, Prometheus rule examples |
| Buffer | 0.5 | Prompt iteration |

### Files Modified

| Component | File | Change |
|---|---|---|
| SP CRD | `api/signalprocessing/v1alpha1/signalprocessing_types.go` | Add `SignalMode`, `OriginalSignalType` to status |
| SP enrichment | `internal/controller/signalprocessing/signalprocessing_controller.go` | Signal mode classification during enrichment |
| SP classifier | `pkg/signalprocessing/classifier/` (new) | Signal mode mapping logic |
| SP config | `config/signalprocessing/predictive-signal-mappings.yaml` | Mapping file |
| AA CRD | `api/aianalysis/v1alpha1/aianalysis_types.go` | Add `SignalMode` to `SignalContextInput` |
| RO creator | `pkg/remediationorchestrator/creator/aianalysis.go` | Copy `SignalMode` in `buildSignalContext()` |
| AA builder | `pkg/aianalysis/handlers/request_builder.go` | Pass `SignalMode` in `BuildIncidentRequest()` |
| HAPI OpenAPI | `holmesgpt-api/openapi.yaml` | Add `signal_mode` to `IncidentRequest` |
| HAPI prompt | `holmesgpt-api/src/` | Conditional prompt strategy |
| Deepcopy | `zz_generated.deepcopy.go` | `make generate` |

---

## References

### Prometheus Documentation

- [predict_linear() function](https://prometheus.io/docs/prometheus/latest/querying/functions/#predict_linear) — PromQL function using simple linear regression to predict future metric values
- [Alerting Rules](https://prometheus.io/docs/prometheus/latest/configuration/alerting_rules/) — Configuration for generating predictive alerts
- [Alerting Best Practices](https://prometheus.io/docs/practices/alerting/) — Guidelines for effective alerting, including predictive patterns

### Kubernaut Documents

- [BR-SP-106: Predictive Signal Mode Classification](../../requirements/BR-SP-106-predictive-signal-mode-classification.md)
- [BR-AI-084: Predictive Signal Mode Prompt Strategy](../../requirements/BR-AI-084-predictive-signal-mode-prompt-strategy.md)
- [Issue #55: Predictive remediation pipeline](https://github.com/jordigilh/kubernaut/issues/55)
- [DD-WORKFLOW-001: Mandatory Label Schema](DD-WORKFLOW-001-mandatory-label-schema.md)
- [ADR-045: AIAnalysis ↔ HolmesGPT API Contract](ADR-045-aianalysis-holmesgpt-api-contract.md)

---

**Document Version**: 1.0
**Last Updated**: February 8, 2026
**Next Review**: May 8, 2026 (3 months)
