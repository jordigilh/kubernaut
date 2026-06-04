# DD-INT-020: ServiceNow Signal Target Type

**Status**: Proposed
**Decision Date**: 2026-06-03
**Version**: 1.2
**Confidence**: 94%
**Deciders**: Architecture Team
**Applies To**: API Frontend, Signal Processing, Remediation Orchestrator, AI Analysis, Kubernaut Agent, Workflow Execution

**Related Business Requirements**:
- BR-INT-004: ServiceNow ticket creation/tracking
- BR-INT-020: ServiceNow as signal target type

**Related Design Decisions**:
- DD-HAPI-019: KA Go rewrite design (prompt builder, parser, tool registry)
- DD-WORKFLOW-001: Mandatory label schema (workflow contract description)
- DD-RO-002: Centralized routing responsibility (scope, blocking conditions)

**Related ADRs**:
- ADR-063: ServiceNow Signal Integration Architecture
- ADR-041: LLM Prompt Response Contract

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.2 | 2026-06-04 | Architecture Team | B2 clarified: KA injects full ticket data (not summaries) as raw context; LLM triages related tickets against resource state. Workflow selection is mandatory unless ticket is already closed (mirrors K8s flow). |
| 1.1 | 2026-06-04 | Architecture Team | Dropped Part D (EM verification) and B6 (KA verification endpoint). ServiceNow is its own audit trail -- WFE success/failure is sufficient. Removed C3 (ProviderData to EA) since EM no longer consumes it. Simplified scope and file count. |
| 1.0 | 2026-06-03 | Architecture Team | Initial design: pipeline plumbing, KA enrichment/prompt/tools, RO guards, EM contract-driven verification, ProviderData schema |

---

## Context & Problem

### Current State

Kubernaut processes signals exclusively from Kubernetes-originated sources (AlertManager, proactive predictors). The pipeline assumes K8s semantics throughout: enrichment queries the K8s API, prompts reference K8s resources, EM verifies K8s resource state via spec hashing and pod health checks.

### Problem Statement

Customers need to investigate ServiceNow tickets that reference cloud objects (cluster status, workloads). The system must:

1. Accept signals derived from ServiceNow tickets
2. Correlate maintenance tickets with reported issues (false alarm detection)
3. Choose between closing the alert (maintenance-caused) or escalating with a new ticket

### Constraints

- `TargetType` and `ProviderData` are dropped at the AA boundary (9-hop plumbing gap)
- KA enrichment assumes K8s API access (would attempt `Pod/<ticket-number>` lookup without gating)
- Scope is customer evaluation readiness (POC/demo), not full production GA
- CMDB CIs are at cluster/node level only (no application-level CIs)

---

## Decision Drivers

1. **Customer demand**: Evaluation deployment requires ServiceNow integration end-to-end
2. **Pipeline reuse**: Maximize reuse of existing RR -> SP -> AA -> KA -> WFE pipeline
3. **Isolation**: ServiceNow-specific logic must not pollute K8s signal processing
4. **Minimal blast radius**: Prefer composition over modification of existing components

---

## Alternatives Considered

### Alternative A: New SignalSource="servicenow" -- REJECTED

**Approach**: Use `SignalSource` to distinguish ServiceNow signals.

**Cons**:
- `SignalSource` indicates origin (AlertManager, a2a-agent), not target system
- AF-originated ServiceNow signals should be `SignalSource="a2a-agent"`
- Would conflate two orthogonal dimensions

**Confidence**: 0% (rejected)

### Alternative B: TargetType="servicenow" with pipeline plumbing -- CHOSEN

**Approach**: Add `servicenow` to the existing `TargetType` enum. Propagate `TargetType` and `ProviderData` through the full pipeline. Gate K8s-specific logic on `TargetType`.

**Pros**:
- Reuses existing discriminator field
- Clean separation: `SignalSource` = origin, `TargetType` = target system
- K8s remains default behavior, ServiceNow is additive

**Cons**:
- 9-file plumbing change to close the AA boundary gap

**Confidence**: 95%

### Alternative C: LLM client in EM -- REJECTED

**Approach**: Embed an LLM client directly in the EM controller for ServiceNow verification.

**Cons**:
- Duplicates LLM infrastructure (client, prompt builder, parser) already in KA
- Makes EM non-deterministic for all target types
- Large blast radius in a controller that is currently purely deterministic

**Confidence**: 0% (rejected)

### Alternative D: KA verification endpoint for EM -- DEFERRED

**Approach**: Expose `POST /verify-effectiveness` in KA. EM calls it as an HTTP client. KA reuses existing LLM infrastructure for a completely independent reasoning session.

**Rationale for deferral**: ServiceNow is its own audit trail. Every ticket state change is recorded with timestamps, actors, and reasons. Unlike K8s resources (where we have no visibility into what changed and must verify post-remediation state), ServiceNow verification would be validating that a ticket did what it was supposed to do -- which the ticket history already confirms. WFE success/failure is sufficient for the POC. If a future need arises (e.g., complex multi-step ServiceNow workflows where partial success is possible), the verification endpoint can be revisited as a new requirement.

**Confidence**: N/A (deferred)

---

## Decision

### Chosen: Alternative B (TargetType) + No EM verification for ServiceNow (POC)

### Architecture

```
                         ┌──────────────────────────────────┐
                         │          ServiceNow API           │
                         └──────┬───────────────┬───────────┘
                                │               │
                           AF fetches      KA queries
                           originating     related
                           ticket          tickets
                                │               │
  ┌─────┐   ┌────┐   ┌────┐   ▼   ┌────┐      ▼   ┌─────┐        ┌─────┐
  │  AF │──▶│ RR │──▶│ SP │──────▶│ AA │─────────▶│ KA  │───────▶│ WFE │
  └─────┘   └────┘   └────┘       └────┘          └─────┘        └─────┘
              │                                                      │
              │  targetType + ProviderData                           │
              │  propagated through pipeline                  WFE success/failure
              │                                              is sufficient for EM
              ▼
           ┌────┐
           │ EA │  EM skips ServiceNow signals (no K8s checks apply,
           └────┘  ServiceNow audit trail is self-documenting)
```

**EM note**: For the POC, EM does not perform ServiceNow-specific verification. ServiceNow is its own audit trail -- every ticket state change is recorded with timestamps, actors, and reasons. WFE success/failure provides sufficient signal. Contract-driven verification via a KA endpoint could be added as a future requirement if complex multi-step workflows require partial-success detection.

### Part A: Pipeline Plumbing (9 files + codegen)

`TargetType` and `ProviderData` flow from RR through SP but are dropped at the AA boundary. The fix propagates both fields through 9 hops:

| Hop | File | Change |
|-----|------|--------|
| A1 | `api/aianalysis/v1alpha1/aianalysis_types.go` | Add `TargetType` and `ProviderData` to `SignalContextInput` |
| A2 | `pkg/remediationorchestrator/creator/aianalysis.go` | Copy from `rr.Spec` / `sp.Spec.Signal` in `buildSignalContext()` |
| A3 | `internal/kubernautagent/api/openapi.json` | Add `target_type` and `provider_data` to `IncidentRequest` (nullable optional string, `anyOf: [{type: string}, {type: null}]` pattern) |
| A3b | `pkg/agentclient/` | Regenerate with `make generate-agentclient` (produces `OptNilString` fields) |
| A4 | `pkg/aianalysis/handlers/request_builder.go` | Map AA CRD fields to `IncidentRequest` via `.SetTo()` |
| A5 | `pkg/kubernautagent/types/types.go` | Add `TargetType` and `ProviderData` to `SignalContext` |
| A6 | `internal/kubernautagent/server/handler.go` | Map `IncidentRequest` to `SignalContext` via `.Get()` pattern |
| A7 | `internal/kubernautagent/mcp/adapters/signal_resolver.go` | Read `rr.Spec.TargetType` and `rr.Spec.ProviderData` in RR fallback path |
| A8 | `internal/kubernautagent/prompt/builder.go` | Add to `SignalData`, `investigationTemplateData` |

### Part B: KA ServiceNow-Aware Logic

#### B1: Enrichment Gating

Gate K8s enrichment in `Investigate()` before `resolveEnrichmentCached()`:

```go
if signal.TargetType != "" && signal.TargetType != "kubernetes" {
    // skip K8s enrichment -- enrichData stays nil
} else {
    enrichData = inv.resolveEnrichmentCached(...)
}
```

Also skip re-enrichment (post-RCA target resolution). Downstream consumers handle nil enrichment gracefully (validated by spike).

#### B2: Prompt Templates

**Phase 1** (`incident_investigation.tmpl`): Add `{{ if eq .TargetType "servicenow" }}` conditional block (mirrors existing `{{ if eq .SignalMode "proactive" }}` pattern) containing:

- **Full original ticket data** from ProviderData (number, description, state, priority, CMDB CI, timestamps, assignment details) -- injected as raw context, not pre-digested summaries
- **Full related open ticket data** for the same CMDB CI (fetched by KA pre-enrichment) -- each ticket's complete details, not summaries
- **Investigation instructions** directing the LLM to triage the related tickets against the state of the resource: the LLM must determine whether the symptoms in the original ticket are explained by what the related tickets describe (scheduled maintenance, known changes, etc.) or whether the problem is independent
- **`submit_result` schema guidance** including `is_false_alarm`, `explained_by_ticket`, `correlation_reasoning` fields

The key design principle: KA provides the raw evidence (original ticket + related tickets + CMDB CI context). The LLM does the triage -- cross-referencing related tickets against the actual resource state to determine causality. KA does not pre-process or summarize the tickets.

**Phase 3** (`phase3_workflow_selection.tmpl`): Add ServiceNow action-type selection rules (CloseAlert vs EscalateTicket) alongside existing K8s/GitOps rules.

**Workflow selection is mandatory** unless the ticket is already closed at investigation time (early exit with "no action needed"). The RCA outcome (explained by maintenance or not) informs which workflow is selected but never skips selection. This mirrors the K8s flow:

| RCA Outcome | Workflow Selection |
|---|---|
| Explained by related tickets (maintenance) | CloseServiceNowAlert |
| Not explained by related tickets | EscalateServiceNowTicket |
| Ticket already closed at investigation time | No workflow -- early exit ("nothing to do") |

#### B3: Dynamic Tool Gating

Thread `phaseTools` as a parameter through the call chain (1 function reads `inv.phaseTools`, minimal refactor):

1. `toolDefinitionsForPhase(phase, phaseTools)` -- replace `inv.phaseTools` with parameter
2. `runLLMLoop(..., phaseTools)` -- pass through
3. 4 call sites pass the appropriate map based on `signal.TargetType`

Entry points select between `DefaultPhaseToolMap()` (K8s) and `ServiceNowPhaseToolMap()` (ServiceNow).

#### B4: Workflow Selection Component Key

`listActionsTool.Execute()` uses `signal.TargetType` to query the DS catalog with ServiceNow-specific component keys.

#### B5: Parser + InvestigationResult for Correlation Fields

Add to `InvestigationResult`:
- `IsFalseAlarm *bool` -- maintenance explains the incident
- `ExplainedByTicket string` -- ServiceNow ticket number that explains the issue
- `CorrelationReasoning string` -- LLM's reasoning about maintenance correlation

Add corresponding fields to `llmResponse` in parser, JSON schema in `schema.go`, and response mapper in `handler.go`.

#### B6: KA Verification Endpoint -- DEFERRED (not in POC)

Dropped from POC scope. ServiceNow's native audit trail makes contract-driven verification redundant for the demo. See "Decision Drivers" and "Alternatives Considered > Alternative D" for full rationale.

If revisited for production GA, the design would expose a `POST /verify-effectiveness` endpoint in KA for EM to call as an HTTP client. The endpoint would use the workflow's `RemediationWorkflowDescription.What` field as the agentic contract and evaluate each claim against post-remediation ServiceNow state.

### Part C: RO Guardrails

#### C1-C2: CapturePreRemediationHash Guard

Guard at both call sites in `analyzing_handler.go` and `awaiting_approval_handler.go` where `rr` is in scope:

```go
if rr.Spec.TargetType != "" && rr.Spec.TargetType != "kubernetes" {
    // skip hash capture -- preHash stays ""
} else {
    preHash, degradedReason, hashErr = h.callbacks.CapturePreRemediationHash(...)
}
```

Skip semantics: `preHash = ""` causes downstream EA creation to have empty `PreRemediationSpecHash`, which EM handles gracefully (hash comparison returns "no pre-hash for comparison").

#### C3: ProviderData into EA Spec -- DEFERRED (not in POC)

Originally planned so EM could read the pre-remediation ticket snapshot. With EM verification dropped, there is no consumer of `ProviderData` on the EA. Deferred to GA if/when EM verification is introduced.

#### CheckUnmanagedResource (no change needed for POC)

`scope.Manager.IsManaged` skips resource-level label checks for unknown K8s kinds and falls back to namespace label. ServiceNow signals in a managed namespace pass scope checks without code changes.

### Part D: EM ServiceNow Verification -- DROPPED FROM POC

**Decision**: EM does not perform ServiceNow-specific verification for the POC.

**Rationale**: ServiceNow is its own audit trail. Every ticket state change (creation, resolution, escalation, comment) is recorded with timestamps, actors, and reasons. Unlike K8s resources -- where Kubernaut has no visibility into what changed and must verify post-remediation state via spec hashing and health checks -- ServiceNow ticket history already confirms what happened. Verifying that the workflow "did what it was supposed to do" adds little value when the ticket history already documents the outcome.

For the POC, WFE success/failure is the only signal EM needs. If WFE reports success, the ServiceNow CLI container executed its API calls successfully (ticket closed, escalation created, etc.). If WFE reports failure, the Job failed.

**What this means for EM**: ServiceNow EAs will complete through the standard K8s-skip path with no ServiceNow-specific components. The existing `scopeNoExecution` / degraded mode patterns in EM handle this gracefully.

**Deferred items** (if revisited for production GA):
- D1: EA CRD extension (`TargetType`, `ProviderData`, `WorkflowID`, `ServiceNowAssessed`, `ServiceNowScore`)
- D2: EA creator populating ServiceNow fields
- D3: ServiceNow scorer as HTTP client to KA verification endpoint
- D5: Early-exit branch in `runComponentPipeline`
- D6: Completion logic for `servicenow_verified` assessment reason

### ProviderData Schema

```json
{
  "instance_url": "https://customer.service-now.com",
  "ticket": {
    "sys_id": "abc123def456",
    "number": "INC0067890",
    "short_description": "Application X unresponsive in cluster prod-east-1",
    "description": "Users reporting 503 errors since 14:30 UTC...",
    "state": "active",
    "priority": "2",
    "impact": "2",
    "urgency": "2",
    "assigned_to": "sre-team-alpha",
    "assignment_group": "Cloud Operations",
    "category": "Infrastructure",
    "subcategory": "Kubernetes",
    "cmdb_ci": {
      "sys_id": "ci_node_789",
      "name": "prod-east-1-node-03",
      "ci_class": "cmdb_ci_linux_server"
    },
    "opened_at": "2026-06-03T14:32:00Z",
    "sys_updated_on": "2026-06-03T14:35:12Z"
  }
}
```

Serves as KA investigation context. If EM verification is introduced in the future, this same snapshot serves as the pre-remediation baseline.

---

## Consequences

### Positive Consequences

1. End-to-end ServiceNow signal support (AF -> RR -> SP -> AA -> KA -> WFE) reusing existing pipeline
2. EM remains untouched for the POC -- zero regression risk for K8s signals
3. Minimal blast radius: ServiceNow-specific logic isolated to KA (tools, prompts, gating)
4. WFE success/failure + ServiceNow audit trail provide sufficient verification

### Negative Consequences

1. 9-file plumbing change for AA boundary gap
   - **Mitigation**: Mechanical field propagation, validated by spikes
2. Two prompt templates need ServiceNow awareness (Phase 1 + Phase 3)
   - **Mitigation**: Conditional blocks mirror existing `SignalMode` pattern
3. No automated verification of ServiceNow workflow outcomes beyond WFE success/failure
   - **Mitigation**: ServiceNow's native audit trail documents what changed. For the POC, this is acceptable. For GA, a KA verification endpoint could be added.

### Risks

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Phase 3 template ServiceNow action types not matched by DS catalog | Low | High | Ensure ServiceNow RemediationWorkflow CRDs deployed before demo |
| WFE reports success but API call partially failed | Low | Medium | ServiceNow API returns errors on partial failure; CLI container exits non-zero |

---

## Compliance

| Requirement | Status | Notes |
|-------------|--------|-------|
| BR-INT-004 | Partial | ServiceNow ticket creation via WFE Job executor |
| BR-INT-020 | Full | ServiceNow as signal target type end-to-end (AF through WFE) |

---

## Validation Strategy

1. **Unit tests**: Per-component logic (parser, prompt rendering, tool gating) with mock dependencies
2. **Integration tests**: Pipeline plumbing (RR -> SP -> AA -> KA) with test CRDs
3. **E2E tests**: Full pipeline with mock ServiceNow API + mock LLM
4. **Spike validation**: 11 spikes executed pre-implementation confirming all assumptions

---

## References

- Issue #1338: feat: Add ServiceNow as a signal target type
- `api/remediationworkflow/v1alpha1/remediationworkflow_types.go`: Workflow description contract
- `internal/kubernautagent/api/openapi.json`: KA OpenAPI specification

---

**Document Version**: 1.2
**Last Updated**: 2026-06-04
