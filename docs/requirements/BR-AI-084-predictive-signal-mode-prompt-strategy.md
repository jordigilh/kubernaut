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

### R1.1: RO SignalType Source Change

The RO creator's `buildSignalContext()` currently copies `SignalType` from `rr.Spec.SignalType` (RemediationRequest). This MUST change to `sp.Status.SignalType` (SP status) so the AA receives the **normalized** signal type. This is a behavioral change to existing code — the RR spec still contains the raw incoming type, but downstream consumers need the normalized value.

```go
// BEFORE (current):
SignalType: rr.Spec.SignalType,

// AFTER (required):
SignalType: sp.Status.SignalType,  // Normalized type from SP status (BR-SP-106)
SignalMode: sp.Status.SignalMode,  // NEW: Copy signal mode from SP status
```

### R2: Request Builder Passes SignalMode to HAPI

The AA request builder (`pkg/aianalysis/handlers/request_builder.go`) MUST include `signalMode` in the `IncidentRequest` sent to HAPI.

### R3: HAPI OpenAPI Spec Update

The HAPI OpenAPI spec MUST include `signal_mode` as a field in the `IncidentRequest` schema. Both Go and Python clients MUST be regenerated.

### R4: HAPI Prompt Strategy

HAPI MUST switch its investigation prompt based on the `signal_mode` value. The current prompt (`holmesgpt-api/src/extensions/incident/prompt_builder.py`) uses a 5-phase investigation workflow. The predictive variant must adapt the relevant phases:

| Phase | Reactive (current) | Predictive (new) |
|---|---|---|
| **Phase 1: Investigate** | Investigate the incident: check pod status, events, logs, resource usage | Evaluate current environment: check resource utilization trends, recent deployments, current state |
| **Phase 2: Root Cause** | Determine root cause — what happened and why? | Determine prevention strategy — is the prediction valid? Should we act preemptively? |
| **Phase 3: Signal Type** | Identify signal type for workflow search | _(unchanged — uses normalized base signal type)_ |
| **Phase 4: Search Workflow** | Search workflow catalog | _(unchanged — same catalog, same normalized type)_ |
| **Phase 5: Summary** | Return RCA summary + workflow recommendation | Return prevention assessment + workflow recommendation OR "no action needed" |

**Reactive** (default — Phases 1-2 investigation directive):
> Perform root cause analysis. The incident has occurred. Investigate logs, events, and resource state to determine why this happened and recommend remediation.

**Predictive** (Phases 1-2 investigation directive):
> Evaluate current environment. This incident is **predicted** based on resource trend analysis but has not occurred yet. Assess resource utilization trends, recent deployments, and current state to determine if preemptive action is warranted and how to **prevent** this incident. "No action needed" is a valid outcome if the prediction is unlikely to materialize.

**Why this is clean**: Because SP normalizes the signal type (BR-SP-106), the agent receives `signal_type = "OOMKilled"` in both modes. It searches the same workflow catalog entry regardless of mode. Phases 3-4 are unchanged. The only difference is the investigation directive (Phases 1-2) and the summary format (Phase 5). The LLM never needs to deal with the `Predicted` prefix — that's entirely handled by SP normalization.

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
RO copies sp.Status.SignalMode → aa.Spec.SignalContext.SignalMode
  → AA request builder includes signalMode in IncidentRequest
    → HAPI reads signal_mode, switches prompt strategy
      → LLM receives normalized signal_type (e.g., "OOMKilled") + mode context
        → Agent searches workflow catalog for "OOMKilled" (standard search, no special logic)
          → Prompt directs: RCA (reactive) or predict & prevent (predictive)
            → Response: workflow recommendation OR "no action needed"
```

---

## Acceptance Criteria

- [ ] `SignalMode` field in `SignalContextInput` (`api/aianalysis/v1alpha1/aianalysis_types.go`)
- [ ] RO copies `SignalMode` from SP status to AA spec (`pkg/remediationorchestrator/creator/aianalysis.go`, `buildSignalContext()`)
- [ ] RO `SignalType` source changed from `rr.Spec.SignalType` to `sp.Status.SignalType` (normalized)
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
| RO creator | `pkg/remediationorchestrator/creator/aianalysis.go` | Change `SignalType` source from `rr.Spec` → `sp.Status` + copy `SignalMode` in `buildSignalContext()` |
| AA request builder | `pkg/aianalysis/handlers/request_builder.go` | Pass `SignalMode` in `BuildIncidentRequest()` |
| HAPI OpenAPI | `holmesgpt-api/api/openapi.json` | Add `signal_mode` to `IncidentRequest` schema |
| HAPI Python model | `holmesgpt-api/src/models/incident_models.py` | Add `signal_mode: Optional[str]` to `IncidentRequest` class |
| HAPI prompt builder | `holmesgpt-api/src/extensions/incident/prompt_builder.py` | Conditional prompt strategy in `create_incident_investigation_prompt()` (Phases 1-2, 5) |
| HAPI LLM integration | `holmesgpt-api/src/extensions/incident/llm_integration.py` | Pass `signal_mode` to prompt builder in `analyze_incident()` |
| Go client regen | `pkg/holmesgpt/client/oas_schemas_gen.go` | `make generate-holmesgpt-client` |
| Mock LLM | `test/services/mock-llm/src/server.py` | Add predictive scenario variants + detection logic (see Test Plan) |
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

> **Note — Mock LLM Enhancement Required**: The Mock LLM (`test/services/mock-llm/src/server.py`) needs a small enhancement to support predictive signal testing:
> 1. **New predictive scenario variants**: Add predictive variants for existing scenarios (e.g., `oomkilled_predictive`) in the `MOCK_SCENARIOS` dict. These use the same `workflow_id` (same catalog entry, since SP normalizes the signal type) but return `root_cause` text reflecting prediction/prevention rather than reactive RCA.
> 2. **Detection logic update**: In `_detect_scenario()`, detect `"predictive"` or `"signal_mode"` in the message content to select the predictive variant of a scenario.
> 3. **"No action" scenario**: Add a predictive-specific scenario that returns `selected_workflow: null` with reasoning like "trend reversing, no preemptive action needed" — validates R5.
>
> The mock's architecture (two-phase response: tool call → final analysis) and config loading (YAML scenarios file) remain unchanged. This is a scenario addition, not a structural change.

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
