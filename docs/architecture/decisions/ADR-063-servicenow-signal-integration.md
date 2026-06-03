# ADR-063: ServiceNow Signal Integration Architecture

**Status**: PROPOSED
**Date**: 2026-06-03
**Decision Makers**: Architecture Team
**Confidence**: 92%

**Related Decisions**:
- [DD-INT-020](DD-INT-020-servicenow-signal-target-type.md) -- Detailed design for ServiceNow signal target type
- [ADR-EM-001](ADR-EM-001-effectiveness-monitor-service-integration.md) -- EM service integration (extended by this ADR)
- [ADR-041](adr-041-llm-contract/ADR-041-llm-prompt-response-contract.md) -- LLM prompt response contract (extended for verification)
- [ADR-001](ADR-001-crd-microservices-architecture.md) -- CRD microservices architecture

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-06-03 | Architecture Team | Initial ADR: four architectural decisions for ServiceNow signal integration |

---

## 1. Context and Problem Statement

Kubernaut's remediation pipeline is designed around Kubernetes-originated signals. Every component in the chain -- from enrichment through investigation to effectiveness verification -- assumes K8s API access and K8s resource semantics.

Extending the pipeline to support ServiceNow tickets as signal sources requires four architectural decisions:

1. How to discriminate ServiceNow signals from K8s signals throughout the pipeline
2. How to gate K8s-specific behavior (enrichment, hashing, scope) for non-K8s targets
3. How KA should investigate ServiceNow tickets (tools, prompts, correlation reasoning)
4. How EM should verify ServiceNow remediation outcomes without K8s deterministic checks

This ADR captures these four decisions as a cohesive architecture.

---

## 2. Decision: TargetType as Pipeline Discriminator

### Context

The pipeline has two candidate fields for distinguishing signal sources: `SignalSource` (origin system, e.g., `alertmanager`, `a2a-agent`) and `TargetType` (target system, e.g., `kubernetes`, `aws`).

### Decision

**Use `TargetType="servicenow"` as the pipeline-wide discriminator.** `SignalSource` remains `"a2a-agent"` for AF-originated signals.

### Rationale

- `SignalSource` and `TargetType` are orthogonal: a ServiceNow ticket entering via AF has `SignalSource="a2a-agent"` and `TargetType="servicenow"`
- `TargetType` already exists on RR and SP CRDs with an enum (`kubernetes`, `aws`, `azure`, `gcp`, `datadog`)
- Adding `servicenow` to the enum is a single-line change per CRD
- All downstream gating (enrichment, hashing, EM pipeline) naturally keys on `TargetType`

### Consequences

- `TargetType` and `ProviderData` must be propagated through the AA boundary (currently dropped -- 9-file fix detailed in DD-INT-020)
- All components receiving signals must handle non-`kubernetes` `TargetType` gracefully

---

## 3. Decision: KA Verification Endpoint for EM

### Context

EM currently uses four deterministic scorers (hash, health, alert, metrics) that assume K8s API access. For ServiceNow signals, verification requires reasoning about whether a workflow achieved its stated objectives (e.g., "Did it close the ticket with the right close code? Did it create an escalation ticket with RCA details?").

Three approaches were considered:

| Approach | Description | Verdict |
|----------|-------------|---------|
| **Deterministic rules** | Check specific field values (state=resolved) | Insufficient -- verifies state change, not intent fulfillment |
| **LLM client in EM** | Embed LLM client directly in EM controller | Rejected -- duplicates KA infrastructure, makes EM non-deterministic |
| **KA verification endpoint** | New REST endpoint in KA; EM calls as HTTP client | **Chosen** |

### Decision

**Expose `POST /verify-effectiveness` in KA.** EM calls it as an HTTP client (same pattern as Prometheus/AlertManager). KA is reused as infrastructure only -- the verification session is completely independent from investigation (separate prompt, separate guardrails, separate schema).

### Architecture

```
EM ServiceNow Scorer
  │
  ├─1─▶ Read EA.Spec.ProviderData (pre-remediation snapshot)
  ├─2─▶ Fetch current ticket from ServiceNow API (post-remediation)
  ├─3─▶ Fetch RemediationWorkflow CRD (workflow contract)
  │
  └─4─▶ POST /verify-effectiveness to KA
           │
           ├── workflow_contract (description.what/whenToUse/preconditions)
           ├── rca_summary
           ├── pre_remediation_state (ProviderData)
           └── post_remediation_state (ServiceNow API response)
                 │
                 ▼
           KA renders verify_effectiveness.tmpl
           LLM evaluates contract claims vs evidence
                 │
                 ▼
           VerifyEffectivenessResponse
           {effective, confidence, contract_evaluation[], reasoning}
                 │
                 ▼
           EM maps to ComponentResult{Assessed: true, Score: &confidence}
```

### Rationale

- **No duplication**: KA already has LLM client, prompt builder, structured output parser, token tracking, metrics
- **Clean separation**: EM orchestrates *when* to verify; KA handles *how* to reason
- **Testable boundary**: KA endpoint tests with mock LLM; EM scorer tests with mock HTTP server
- **Graceful degradation**: If KA is unavailable, EM treats it like Prometheus/AlertManager being down (assessed-as-skipped, requeue)
- **Not "grading own homework"**: The verification session shares zero context with investigation -- different prompt, different guardrails, different schema. KA is the execution engine, not a biased evaluator

### Consequences

- KA gains a new OpenAPI endpoint (auto-routed by ogen after codegen)
- KA gains `verify_effectiveness.tmpl` + `VerificationResultSchema()` + parser
- EM gains `KAEndpointURL` config (same pattern as `PrometheusURL`)
- EM ServiceNow scorer is an HTTP client, not an LLM integration

---

## 4. Decision: Contract-Driven Verification

### Context

How should the verification LLM know what to check? Hardcoded rules (e.g., "for close workflows, check state=resolved") are brittle and require code changes for each new workflow.

### Decision

**Use the workflow's `RemediationWorkflowDescription.What` field as the agentic contract.** The LLM evaluates each claim in the `what` field against post-remediation evidence.

### Example

Workflow CRD:
```yaml
description:
  what: "Closes the ServiceNow incident as false alarm caused by scheduled maintenance.
         Sets state to Resolved, close_code to 'Solved (Permanently)', adds close_notes
         referencing the maintenance change request."
```

Verification LLM output:
```json
{
  "effective": true,
  "confidence": 0.93,
  "contract_evaluation": [
    {"criterion": "Sets ticket state to Resolved", "met": true, "evidence": "state: active -> resolved"},
    {"criterion": "Sets close_code to Solved (Permanently)", "met": true, "evidence": "close_code='solved'"},
    {"criterion": "Adds close_notes referencing maintenance CR", "met": true, "evidence": "close_notes contains CHG0012345"}
  ]
}
```

### Rationale

- **Workflow authors define acceptance criteria** in natural language via the existing `description.what` field
- **No hardcoded verification logic** -- works for any ServiceNow workflow without code changes
- **Per-criterion transparency** -- each claim has a pass/fail with evidence, valuable for customer demos
- **Reuses existing CRD field** -- `RemediationWorkflowDescription` is already defined and populated

### Consequences

- Verification quality depends on workflow authors writing precise `description.what` fields
- Vague contracts produce vague verdicts (mitigation: documentation + templates for ServiceNow workflows)

---

## 5. Decision: EM Early-Exit Branch for Non-K8s Targets

### Context

EM's component pipeline runs hash, health, alert, and metrics checks in a hardcoded sequence. Health and hash are always required in `allComponentsDone()` (unlike alert/metrics which have config-disabled escape hatches).

### Decision

**Add an early-exit branch at the top of `runComponentPipeline` for non-K8s target types.** This mirrors the existing `scopeNoExecution` pattern.

```go
if ea.Spec.TargetType == "servicenow" {
    r.runServiceNowCheck(ctx, rctx)
    result, err := r.completeAssessmentWithReason(ctx, ea, eav1.AssessmentReasonServiceNowVerified)
    return result, true, err
}
```

### Rationale

- Clean separation: K8s pipeline is untouched for K8s signals
- Avoids modifying `allComponentsDone`/`determineAssessmentReason` branching logic
- Pattern proven by `scopeNoExecution` and `scopePartial` early exits
- ServiceNow assessment has only one component (the KA verification call)

### Consequences

- New `AssessmentReason` enum value: `"servicenow_verified"`
- EA CRD gains `ServiceNowAssessed` and `ServiceNowScore` component fields
- EM completion logic updated to handle the new reason

---

## 6. ProviderData Schema

ServiceNow `ProviderData` serves dual purpose: KA investigation context AND EM pre-remediation baseline. Schema defined in DD-INT-020.

Key fields: `instance_url`, `ticket.sys_id`, `ticket.number`, `ticket.short_description`, `ticket.description`, `ticket.state`, `ticket.priority`, `ticket.assigned_to`, `ticket.assignment_group`, `ticket.category`, `ticket.cmdb_ci.*`, timestamps.

---

## 7. Scope and Deferrals

### In Scope (Customer Evaluation)

- Pipeline plumbing (Part A)
- KA investigation + prompt + tools (Part B)
- RO guardrails (Part C)
- EM ServiceNow verification via KA endpoint (Part D)
- ServiceNow Go REST API client with mock httptest server
- Two RemediationWorkflow CRDs (close + escalate)
- E2E test with mock ServiceNow + mock LLM

### Deferred to Production GA

- ServiceNow API circuit breaker and rate-limit handling
- CMDB response caching
- Multi-instance ServiceNow support
- Shared `ResourceResolver` interface (K8s + ServiceNow)
- SLO definitions for KA verification endpoint
- Credential rotation runbook
- `targetType`-based scope bypass in `scope.Manager.IsManaged` (namespace fallback sufficient for POC)

---

## 8. References

- [DD-INT-020](DD-INT-020-servicenow-signal-target-type.md): Detailed design document
- Issue #1338: feat: Add ServiceNow as a signal target type
- `api/remediationworkflow/v1alpha1/remediationworkflow_types.go`: `RemediationWorkflowDescription`
- `internal/controller/effectivenessmonitor/reconcile_components.go`: EM component pipeline

---

**Document Version**: 1.0
**Last Updated**: 2026-06-03
