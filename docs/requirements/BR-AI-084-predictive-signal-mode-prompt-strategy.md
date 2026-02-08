# BR-AI-084: Predictive Signal Mode Prompt Strategy

**Document Version**: 1.0
**Date**: February 8, 2026
**Status**: ✅ APPROVED
**Category**: HolmesGPT Integration
**Priority**: P1 (High)
**Service**: AIAnalysis + HolmesGPT API
**GitHub Issue**: [#55](https://github.com/jordigilh/kubernaut/issues/55)
**Related**: BR-SP-106, DD-WORKFLOW-001, ADR-045

---

## Business Context

### Problem Statement

When Signal Processing classifies a signal as `predictive` (BR-SP-106), the downstream AI investigation must adapt its strategy. A reactive signal requires root cause analysis ("what happened and why?"), while a predictive signal requires environment evaluation ("is this prediction valid and should we act preemptively?").

Without this distinction, HolmesGPT would perform RCA on an incident that hasn't occurred, producing irrelevant or misleading results (e.g., "no error logs found" — because nothing has failed yet).

### Business Value

1. **Accurate AI investigation**: LLM receives the correct investigation directive
2. **Preemptive recommendations**: AI can recommend scaling, eviction, or other preemptive actions
3. **Valid "no action" outcomes**: Predictive mode legitimately allows "no action needed" — the trend may reverse or the prediction may be based on a temporary spike
4. **Audit differentiation**: Reactive vs. predictive remediation outcomes tracked separately for Effectiveness Monitor

---

## Requirements

### R1: SignalMode in AA Spec

The AIAnalysis CRD spec MUST include a `SignalMode` field in `SignalContextInput`, populated by RO from the SP status (same copy pattern as severity, environment, priority).

### R2: Request Builder Passes SignalMode to HAPI

The AA request builder (`pkg/aianalysis/handlers/request_builder.go`) MUST include `signalMode` in the `IncidentRequest` sent to HAPI.

### R3: HAPI OpenAPI Spec Update

The HAPI OpenAPI spec MUST include `signal_mode` as a field in the `IncidentRequest` schema. Both Go and Python clients MUST be regenerated.

### R4: HAPI Prompt Strategy and Workflow Search

HAPI MUST switch its investigation prompt based on the `signal_mode` value. The prompt MUST also guide the agent's workflow catalog search behavior.

**Reactive** (default):
> Perform root cause analysis. The incident has occurred. Investigate logs, events, and resource state to determine why this happened and recommend remediation. Search for a remediation workflow matching the signal type.

**Predictive**:
> Evaluate current environment. This incident is predicted but has not occurred yet. Assess resource trends, recent deployments, and current state to determine if preemptive action is warranted. "No action needed" is a valid outcome if the prediction is unlikely to materialize. Search for a **predictive** remediation workflow matching the signal type (e.g., `PredictedOOMKill`). If no predictive-specific workflow exists, fall back to the base reactive workflow (e.g., `OOMKilled`) but adapt execution for preemptive context.

**Critical**: The agent must be aware that the workflow catalog may contain **both** reactive and predictive workflows for the same underlying condition. The `signal_type` field carries the full type (e.g., `PredictedOOMKill`, not `OOMKilled`), enabling the agent to search for predictive-specific workflows first.

### R5: Valid "No Action" Outcome

In predictive mode, the LLM MUST be allowed to conclude that no preemptive action is needed. This is a valid outcome that:
- Sets `needs_human_review: false`
- Sets `selected_workflow: null`
- Provides reasoning in the analysis summary (e.g., "Temporary memory spike from batch job, trend reversing")

### R6: Audit Event Recording

Audit events for AI analysis MUST include the `signalMode` value, enabling the Effectiveness Monitor to differentiate predictive vs. reactive remediation outcomes.

---

## Data Flow

```
RO copies sp.Status.SignalMode + sp.Status.SignalType → aa.Spec.SignalContext
  → AA request builder includes signalMode + signalType in IncidentRequest
    → HAPI reads signal_mode, switches prompt strategy
      → LLM investigates with correct directive
        → Agent searches workflow catalog using signal_type (e.g., PredictedOOMKill)
          → If no predictive workflow found, agent falls back to base type (e.g., OOMKilled)
            → Response: workflow recommendation OR "no action needed"
```

---

## Acceptance Criteria

- [ ] `SignalMode` field in `SignalContextInput` (`api/aianalysis/v1alpha1/aianalysis_types.go`)
- [ ] RO copies `SignalMode` from SP status to AA spec (`pkg/remediationorchestrator/creator/aianalysis.go`, `buildSignalContext()`)
- [ ] AA request builder passes `signalMode` to HAPI (`pkg/aianalysis/handlers/request_builder.go`)
- [ ] HAPI OpenAPI spec includes `signal_mode` in `IncidentRequest`
- [ ] Go client regenerated (`make generate-holmesgpt-client`)
- [ ] Python client regenerated
- [ ] HAPI prompt switches based on `signal_mode`
- [ ] Predictive mode allows "no action" as valid LLM outcome
- [ ] Audit events include `signalMode`
- [ ] `make generate` regenerates deepcopy successfully

---

## Implementation Points

| Component | File(s) | Change |
|---|---|---|
| AA CRD spec | `api/aianalysis/v1alpha1/aianalysis_types.go` | Add `SignalMode` to `SignalContextInput` |
| RO creator | `pkg/remediationorchestrator/creator/aianalysis.go` | Copy `sp.Status.SignalMode` in `buildSignalContext()` |
| AA request builder | `pkg/aianalysis/handlers/request_builder.go` | Pass `SignalMode` in `BuildIncidentRequest()` |
| HAPI OpenAPI | `holmesgpt-api/openapi.yaml` | Add `signal_mode` to `IncidentRequest` |
| HAPI prompt | `holmesgpt-api/src/` | Conditional prompt strategy |
| Client regen | Generated clients | `make generate-holmesgpt-client` |
| Deepcopy | `api/aianalysis/v1alpha1/zz_generated.deepcopy.go` | `make generate` |

---

## Test Plan

### Unit Tests
- AA request builder passes `signalMode` correctly for both values
- HAPI prompt content differs for `reactive` vs. `predictive`
- "No action" outcome accepted in predictive mode

### Integration Tests
- RO copies `signalMode` from SP to AA spec
- HAPI mock LLM validates prompt contains correct investigation directive
- Predictive mode with mock LLM returns valid "no action" response

### E2E Tests
- Full pipeline: predictive alert → SP → RO → AA → HAPI → workflow selection (or "no action")

---

## Approval Gate Considerations

Operators may want different approval thresholds for predictive vs. reactive remediations:
- **Reactive**: Auto-approve at 80%+ confidence (existing behavior)
- **Predictive**: Potentially require higher confidence or always require human approval

This is a **future enhancement** — v1.0 uses the same approval thresholds regardless of signal mode. The `signalMode` field in audit events enables this differentiation in v1.1+.

---

## References

### Prometheus Documentation

- [predict_linear() function reference](https://prometheus.io/docs/prometheus/latest/querying/functions/#predict_linear) — The PromQL function that generates the predictive signals consumed by this feature. Understanding its linear regression model helps inform prompt design: the AI agent should know that predictions are based on recent linear trends, not seasonal patterns.
- [Prometheus Alerting Rules](https://prometheus.io/docs/prometheus/latest/configuration/alerting_rules/) — Defines how predictive alerts are generated and delivered to Kubernaut's Gateway.

### Related Documents

- [BR-SP-106: Predictive Signal Mode Classification](BR-SP-106-predictive-signal-mode-classification.md)
- [Issue #55: Predictive remediation pipeline](https://github.com/jordigilh/kubernaut/issues/55)
- [ADR-045: AIAnalysis ↔ HolmesGPT API Contract](../architecture/decisions/ADR-045-aianalysis-holmesgpt-api-contract.md)
- [AA Business Requirements](../services/crd-controllers/02-aianalysis/BUSINESS_REQUIREMENTS.md)
