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

1. **Agent doesn't distinguish signal modes**: Without explicit classification, the agent treats all signals as reactive incidents. For a predicted event that hasn't occurred yet, the agent performs standard RCA — producing irrelevant results ("no error logs found" — because nothing has failed yet). The agent needs to know this is a prediction so it can investigate how to **prevent** the incident rather than diagnose one that already happened.

2. **Workflow catalog search mismatch**: Predictive signal types from Prometheus (e.g., `PredictedOOMKill`) don't match existing workflows in the catalog, which are registered under base signal types (e.g., `OOMKilled`). The signal type must be normalized to the base type so the agent can find the correct workflow, while a separate `signalMode` field tells the agent the investigation context is predictive.

### Business Requirements

This ADR implements two Business Requirements:
- **BR-SP-106**: Predictive Signal Mode Classification (Signal Processing)
- **BR-AI-084**: Predictive Signal Mode Prompt Strategy (AIAnalysis + HAPI)

### Constraints

- No CRD label changes — `signalMode` lives in status (SP) and spec (AA), not labels
- Signal type normalized to base type so the agent can search the existing workflow catalog
- Original signal type preserved in SP status for audit trail
- Predictive mode must allow "no action needed" as a valid LLM outcome
- v1.0 — no backwards compatibility concerns (not yet released)

---

## Decision

### 1. Signal Mode Classification and Signal Type Normalization in Signal Processing

**Chosen**: SP classifies all signals as `reactive` (default) or `predictive` based on configurable pattern matching (e.g., the `Predicted` prefix convention from Prometheus). SP **normalizes** predictive signal types to their base type so the agent can search the existing workflow catalog, while preserving the original signal type in status for audit.

**Classification and Normalization Logic**:
```
Input: PredictedOOMKill
  → Matches predictive pattern (configurable, e.g. "Predicted*" prefix)
  → Set: status.signalMode = "predictive"
  → Set: status.signalType = "OOMKilled" (normalized — matches existing workflow catalog)
  → Set: status.originalSignalType = "PredictedOOMKill" (preserved for audit trail)

Input: OOMKilled
  → Does not match any predictive pattern
  → Set: status.signalMode = "reactive"
  → Set: status.signalType = "OOMKilled" (unchanged)
  → Set: status.originalSignalType = "" (not applicable)
```

**Configuration**:
```yaml
# config/signalprocessing/predictive-signal-mappings.yaml
predictive_signal_mappings:
  PredictedOOMKill: OOMKilled
  PredictedCPUThrottling: CPUThrottling
  PredictedDiskPressure: DiskPressure
  PredictedNodeNotReady: NodeNotReady
  # Operators can add new mappings without code changes
```

**Rationale**:
- **Normalization enables catalog reuse**: The agent searches for `OOMKilled` workflows that already exist — no need to create predictive-specific workflow entries. The `signalMode` field tells the agent the context is predictive.
- **Source-agnostic workflow catalog**: Normalization decouples the workflow catalog from signal source naming conventions. The catalog only deals with base signal types (`OOMKilled`, `DiskPressure`, etc.) regardless of whether the signal came from Prometheus `predict_linear()`, a reactive Prometheus alert, a Kubernetes event, or any future signal source (AWS CloudWatch, Azure Monitor, etc.). Source-specific naming is an SP concern, not a catalog concern.
- **Clean separation**: SP handles signal type normalization (its responsibility), the prompt handles investigation strategy (HAPI's responsibility). The LLM never needs to know about `PredictedOOMKill` as a signal type.
- **Config-driven**: Operators can add new predictive signal type mappings without code changes
- **Hot-reloadable**: Follows BR-SP-072 pattern (fsnotify-based ConfigMap reload)
- **Safe default**: Unmapped signal types default to `reactive`, no workflow disruption
- **Audit trail**: Original signal type preserved for full traceability

---

### 2. Pipeline Data Flow

**Chosen**: `signalMode` flows through the existing pipeline using established copy patterns — no new wiring required.

```
Prometheus predict_linear() alert (signal_type: PredictedOOMKill)
  → Gateway (receives PredictedOOMKill, passes through unchanged)
    → Signal Processing
        status.signalMode = "predictive"
        status.signalType = "OOMKilled" (normalized to base type)
        status.originalSignalType = "PredictedOOMKill" (preserved for audit)
      → Remediation Orchestrator
          copies sp.Status.SignalMode → aa.Spec.SignalContext.SignalMode
          copies sp.Status.SignalType → aa.Spec.SignalContext.SignalType (normalized)
          (same pattern as severity, environment, priority)
        → AI Analysis
            passes signalMode + signalType (normalized) to HAPI in IncidentRequest
          → HolmesGPT API
              1. Agent receives signal_type = "OOMKilled" (searches catalog normally)
              2. Agent receives signal_mode = "predictive" (switches prompt strategy)
              3. Prompt: "This is a predicted incident — investigate how to prevent it"
```

**Rationale**:
- **Zero new wiring**: Every hop already exists for severity/environment/priority
- **RO is the bridge**: RO already copies SP status fields to AA spec in `buildSignalContext()`
- **AA is the caller**: AA already builds `IncidentRequest` from spec fields in `BuildIncidentRequest()`

---

### 3. HAPI Prompt Strategy

**Chosen**: HAPI switches its investigation prompt based on `signal_mode`, with two distinct directives. Because SP normalizes the signal type, the agent always searches the workflow catalog with the base type — no special search logic needed.

**Reactive** (default — incident has occurred):
> Perform root cause analysis. The incident has occurred. Investigate logs, events, and resource state to determine the root cause and recommend remediation.

**Predictive** (incident predicted, not yet occurred):
> Evaluate current environment. This incident is **predicted** based on resource trend analysis but has not occurred yet. Assess resource utilization trends, recent deployments, and current state to determine if preemptive action is warranted and how to **prevent** this incident. "No action needed" is a valid outcome if the prediction is unlikely to materialize.

**Why this works cleanly**: The agent receives `signal_type = "OOMKilled"` in both modes — it searches the same workflow catalog entry. The only difference is the investigation prompt: reactive asks "what happened and why?", predictive asks "this is about to happen, how do we prevent it?". The LLM never needs to deal with the `Predicted` prefix — that's entirely handled by SP.

**Rationale**:
- **Zero workflow search complexity**: The agent always searches by the normalized base signal type — no fallback logic, no dual-search, no prefix handling
- **Clear directive**: The LLM knows exactly what investigation mode to use
- **Valid "no action"**: Predictive mode explicitly allows the LLM to conclude no preemptive action is needed (trend reversal, temporary spike, etc.)
- **Audit differentiation**: Enables Effectiveness Monitor to track predictive vs. reactive outcomes separately

---

### 4. Single HAPI Endpoint (No Separate Predictive Endpoint)

**Chosen**: Reuse the existing HAPI investigation endpoint (`IncidentRequest`), adding `signal_mode` as a field. No new REST endpoint for predictive investigations.

**Rationale**:
- **Identical pipeline**: The investigation infrastructure is the same — same agent, same tools, same workflow catalog search, same response structure (analysis + workflow recommendation or "no action"). The only difference is the prompt preamble.
- **One-field change**: Adding `signal_mode` to the existing `IncidentRequest` schema is minimal — a single `if` switches the prompt. A new endpoint would duplicate the entire handler chain (validation, auth, audit, error handling) for what amounts to a prompt switch.
- **Single code path**: One endpoint means one code path to maintain, test, and version. Two endpoints for the same investigation with different prompts is over-engineering.
- **Future extensibility**: If predictive investigations eventually need fundamentally different inputs (e.g., time-series data, prediction horizon, confidence intervals from Prometheus), a new endpoint can be introduced at that point. For v1.0, it's a prompt context flag.

---

### 5. No CRD Label Changes

**Chosen**: `signalMode` is a CRD **status** field (SP) and **spec** field (AA), not a label.

**Rationale**:
- Labels are part of the CRD identity and affect label selectors, informers, and field selectors
- `signalMode` is internal pipeline context, not a selection criterion
- Avoids DD-WORKFLOW-001 label schema changes
- Status/spec fields are simpler to add and don't affect Kubernetes API behavior

---

## Alternatives Considered

### Alternative A: Preserve Predictive Signal Type (No Normalization)

SP preserves `PredictedOOMKill` as the signal type and expects the agent to search for predictive-specific workflows in the catalog, with fallback to the base type.

**Rejected because**:
- Requires the LLM to understand predictive signal type naming conventions and implement fallback logic
- Requires the workflow catalog to contain separate predictive workflow entries, or the agent must strip the prefix
- Adds workflow search complexity to the LLM prompt — fragile and hard to test reliably
- The LLM's RCA result would need to reference the predictive signal type, adding more LLM-side logic
- Much simpler to normalize in SP (deterministic code) and let the prompt carry the predictive context

### Alternative B: Gateway Classifies Signal Mode

Gateway performs the predictive pattern detection before creating the SP CRD.

**Rejected because**:
- Gateway's role is signal ingestion and deduplication, not classification
- Classification belongs in Signal Processing (established responsibility boundary)
- Gateway would need to maintain signal pattern config (wrong layer)
- SP already has the enrichment pipeline infrastructure (hot-reload, Rego engine, etc.)

### Alternative C: HAPI Infers Predictive Mode from Signal Type Name

HAPI checks if the signal type starts with "Predicted" and adjusts its prompt, without an explicit `signalMode` field from the pipeline.

**Rejected because**:
- Fragile string-based convention in the LLM layer — no guarantee the LLM will reliably parse the prefix
- No explicit pipeline signal — implicit behavior is error-prone
- Doesn't generalize to non-"Predicted" naming patterns
- Violates separation of concerns (classification is SP's job, not the LLM's)

### Alternative D: Separate HAPI REST Endpoint for Predictive Investigations

Expose a new `/api/v1/predictive-investigation` endpoint alongside the existing investigation endpoint.

**Rejected because**:
- The investigation pipeline is identical: same agent, same tools, same workflow catalog search, same response structure. The only difference is the prompt preamble — a single `if` on `signal_mode`
- A new endpoint duplicates the entire handler chain (validation, auth, audit, error handling) for what amounts to a prompt switch
- Two endpoints means two code paths to maintain, test, and version
- AA would need branching logic to call different endpoints based on signal mode, adding wiring complexity
- If predictive investigations need fundamentally different inputs in the future (time-series data, prediction horizon, confidence intervals), a new endpoint can be introduced then. For v1.0, `signal_mode` in the existing `IncidentRequest` is sufficient

---

## Consequences

### Positive

1. **Immediate value with zero code changes**: Prometheus `predict_linear()` alerting rules generate predictive signals today. Even without the pipeline enhancement, these alerts flow through Kubernaut and trigger standard remediation.
2. **Source-agnostic workflow catalog**: Normalization at the SP layer decouples the workflow catalog from signal source naming conventions. Workflows are defined once per base signal type and work for any source — Prometheus predictive alerts, reactive alerts, Kubernetes events, or future integrations (CloudWatch, Azure Monitor, PagerDuty). Adding a new signal source never requires new workflow catalog entries.
3. **Incremental enhancement**: The pipeline changes (SP → RO → AA → HAPI) follow existing patterns, minimizing implementation risk.
4. **Enterprise ROI proof**: Predictive vs. reactive tracking in audit events enables the Effectiveness Monitor to answer "How often did predictions prevent incidents?"
5. **Extensible**: New predictive signal type mappings added via config, not code.

### Negative

1. **Prompt engineering iteration**: The predictive prompt will need tuning against real scenarios. Mitigated by prompt being a configuration string, not compiled code.
2. **Linear regression limitations**: `predict_linear()` is a simple linear model — poor for periodic metrics (CPU, request rate). Documented in BR-SP-106 as a known constraint. Future enhancement could integrate `double_exponential_smoothing()` (Prometheus 3.x) for seasonal data.
3. **Config maintenance**: Signal type mappings must be maintained as new predictive alert types are added. Mitigated by hot-reload and operator documentation.

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
| SP CRD | `api/signalprocessing/v1alpha1/signalprocessing_types.go` | Add `SignalMode`, `SignalType`, `OriginalSignalType` to status |
| SP enrichment | `internal/controller/signalprocessing/signalprocessing_controller.go` | Signal mode classification + signal type normalization in `reconcileClassifying()` |
| SP classifier | `pkg/signalprocessing/classifier/signalmode.go` (new) | Signal mode classification + normalization mapping logic |
| SP config | `config/signalprocessing/predictive-signal-mappings.yaml` | Predictive signal type → base type mapping config |
| SP main | `cmd/signalprocessing/main.go` | Wire classifier, load config, start hot-reload |
| SP audit | `pkg/signalprocessing/audit/client.go` | Populate `signal_mode` in audit payloads |
| DS OpenAPI | `api/openapi/data-storage-v1.yaml` | Add `signal_mode`, `original_signal_type` to `SignalProcessingAuditPayload` |
| AA CRD | `api/aianalysis/v1alpha1/aianalysis_types.go` | Add `SignalMode` to `SignalContextInput` |
| RO creator | `pkg/remediationorchestrator/creator/aianalysis.go` | Change `SignalType` source to `sp.Status` + copy `SignalMode` in `buildSignalContext()` |
| AA builder | `pkg/aianalysis/handlers/request_builder.go` | Pass `SignalMode` in `BuildIncidentRequest()` |
| HAPI OpenAPI | `holmesgpt-api/api/openapi.json` | Add `signal_mode` to `IncidentRequest` |
| HAPI prompt | `holmesgpt-api/src/extensions/incident/prompt_builder.py` | Conditional prompt strategy (Phases 1-2, 5) |
| Mock LLM | `test/services/mock-llm/src/server.py` | Predictive scenario variants + detection logic |
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
