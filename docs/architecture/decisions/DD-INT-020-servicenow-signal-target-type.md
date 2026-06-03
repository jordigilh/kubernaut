# DD-INT-020: ServiceNow Signal Target Type

**Status**: Proposed
**Decision Date**: 2026-06-03
**Version**: 1.0
**Confidence**: 92%
**Deciders**: Architecture Team
**Applies To**: API Frontend, Signal Processing, Remediation Orchestrator, AI Analysis, Kubernaut Agent, Workflow Execution, Effectiveness Monitor

**Related Business Requirements**:
- BR-INT-004: ServiceNow ticket creation/tracking
- BR-INT-020: ServiceNow as signal target type

**Related Design Decisions**:
- DD-EM-002: Canonical spec hash (pre/post comparison model)
- DD-HAPI-019: KA Go rewrite design (prompt builder, parser, tool registry)
- DD-WORKFLOW-001: Mandatory label schema (workflow contract description)
- DD-RO-002: Centralized routing responsibility (scope, blocking conditions)

**Related ADRs**:
- ADR-063: ServiceNow Signal Integration Architecture
- ADR-EM-001: Effectiveness Monitor Service Integration
- ADR-041: LLM Prompt Response Contract

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
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
4. Verify that the chosen workflow actually achieved its intended outcome

### Constraints

- `TargetType` and `ProviderData` are dropped at the AA boundary (9-hop plumbing gap)
- KA enrichment assumes K8s API access (would attempt `Pod/<ticket-number>` lookup without gating)
- EM is purely deterministic (no LLM client) -- cannot reason about ServiceNow ticket outcomes
- Scope is customer evaluation readiness (POC/demo), not full production GA
- CMDB CIs are at cluster/node level only (no application-level CIs)

---

## Decision Drivers

1. **Customer demand**: Evaluation deployment requires ServiceNow integration end-to-end
2. **Pipeline reuse**: Maximize reuse of existing RR -> SP -> AA -> KA -> WFE -> EM pipeline
3. **Isolation**: ServiceNow-specific logic must not pollute K8s signal processing
4. **Verification depth**: "Did the workflow complete?" is insufficient -- need "Did the workflow do what it promised?"
5. **Minimal blast radius**: Prefer composition over modification of existing components

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

**Confidence**: 0% (rejected in favor of KA verification endpoint)

### Alternative D: KA verification endpoint for EM -- CHOSEN

**Approach**: Expose `POST /verify-effectiveness` in KA. EM calls it as an HTTP client. KA reuses existing LLM infrastructure for a completely independent reasoning session.

**Pros**:
- Zero duplication of LLM infrastructure
- EM stays thin (HTTP client, same pattern as Prometheus/AlertManager)
- Clean separation: KA = reasoning engine, EM = orchestrator
- Independent session with dedicated prompt and guardrails

**Cons**:
- KA availability becomes a dependency for EM ServiceNow verification

**Confidence**: 92%

---

## Decision

### Chosen: Alternative B (TargetType) + Alternative D (KA verification endpoint)

### Architecture

```
                         ┌──────────────────────────────────────────────┐
                         │              ServiceNow API                  │
                         └──────┬───────────────┬───────────────┬──────┘
                                │               │               │
                           AF fetches      KA queries      EM fetches
                           originating     related         current
                           ticket          tickets         ticket
                                │               │               │
  ┌─────┐   ┌────┐   ┌────┐   ▼   ┌────┐      ▼   ┌─────┐    ▼   ┌────┐
  │  AF │──▶│ RR │──▶│ SP │──────▶│ AA │─────────▶│ KA  │───────▶│ WFE│
  └─────┘   └────┘   └────┘       └────┘          └──┬──┘        └──┬─┘
              │                                       │              │
              │  targetType + ProviderData             │              │
              │  propagated through pipeline           │              │
              │                                       │              │
              ▼                                       │              ▼
           ┌────┐                                     │           ┌────┐
           │ EA │◀────────────────────────────────────┘           │ EM │
           └────┘  RO copies ProviderData into EA                └──┬─┘
              ▲                                                     │
              │                          POST /verify-effectiveness │
              │                          (workflow contract +       │
              │                           pre/post ticket state)    │
              └─────────────────────────────────────────────────────┘
```

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
- ServiceNow ticket context from ProviderData (number, description, state, CMDB CI)
- Pre-fetched related active ticket summaries
- Tool guidance for ServiceNow-specific investigation tools
- Modified `submit_result` schema guidance including `is_false_alarm`, `explained_by_ticket`, `correlation_reasoning`

**Phase 3** (`phase3_workflow_selection.tmpl`): Add ServiceNow action-type selection rules (CloseAlert vs EscalateTicket) alongside existing K8s/GitOps rules.

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

#### B6: KA Verification Endpoint

New `POST /verify-effectiveness` endpoint for contract-driven EM verification.

**Request** (`VerifyEffectivenessRequest`):
```json
{
  "correlation_id": "rr-abc-123",
  "target_type": "servicenow",
  "workflow_id": "servicenow-close-maintenance-v1",
  "workflow_contract": {
    "what": "Closes the ServiceNow incident as false alarm...",
    "when_to_use": "When RCA determines incident is caused by maintenance...",
    "preconditions": "Active maintenance change request exists..."
  },
  "rca_summary": "Alert caused by scheduled maintenance CHG0012345...",
  "pre_remediation_state": { "ticket": { "...ProviderData snapshot..." } },
  "post_remediation_state": {
    "tickets": [
      { "sys_id": "abc123", "number": "INC0067890", "state": "resolved", "..." : "..." }
    ]
  }
}
```

**Response** (`VerifyEffectivenessResponse`):
```json
{
  "effective": true,
  "confidence": 0.93,
  "contract_evaluation": [
    {"criterion": "Sets ticket state to Resolved", "met": true, "evidence": "state: active -> resolved"},
    {"criterion": "Adds close_notes referencing maintenance CR", "met": true, "evidence": "close_notes contains CHG0012345"}
  ],
  "reasoning": "All contract criteria met."
}
```

The workflow contract comes from the `RemediationWorkflow` CRD's `description.what` field. EM fetches the CRD and sends the contract in the request. KA evaluates each claim against post-remediation evidence using a dedicated `verify_effectiveness.tmpl` prompt template.

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

#### C3: ProviderData into EA Spec

RO copies `rr.Spec.ProviderData` into `EA.Spec.ProviderData` at creation time, so EM can read the pre-remediation ticket snapshot directly from the EA.

#### CheckUnmanagedResource (no change needed for POC)

`scope.Manager.IsManaged` skips resource-level label checks for unknown K8s kinds and falls back to namespace label. ServiceNow signals in a managed namespace pass scope checks without code changes.

### Part D: EM ServiceNow Verification

#### D1: EA CRD Extension

Add optional fields to `EffectivenessAssessmentSpec`:
- `TargetType string` (discriminator for EM pipeline)
- `ProviderData string` (pre-remediation ServiceNow ticket snapshot)
- `WorkflowID string` (for contract lookup)

Add to `EAComponents`:
- `ServiceNowAssessed bool`
- `ServiceNowScore *float64`

Add `AssessmentReason` enum value: `"servicenow_verified"`.

CEL `self == oldSelf` on spec is unaffected (new optional fields are additive).

#### D2: EA Creator

RO populates new EA spec fields from RR and AA at creation time.

#### D3: ServiceNow Scorer

HTTP client calling KA `/verify-effectiveness`. Flow:

1. Read `ea.Spec.ProviderData` (pre-remediation state)
2. Fetch current ticket from ServiceNow API (post-remediation state)
3. Fetch `RemediationWorkflow` CRD for `ea.Spec.WorkflowID` (get `description` contract)
4. Build `VerifyEffectivenessRequest` with contract + pre/post state + RCA summary
5. POST to KA `/verify-effectiveness`
6. Map `VerifyEffectivenessResponse` to `ComponentResult{Assessed: true, Score: &confidence}`

#### D5: Pipeline Integration (Early-Exit Branch)

At top of `runComponentPipeline` (after scope determination):

```go
if ea.Spec.TargetType == "servicenow" {
    r.runServiceNowCheck(ctx, rctx)
    result, err := r.completeAssessmentWithReason(ctx, ea, eav1.AssessmentReasonServiceNowVerified)
    return result, true, err
}
```

This skips all K8s components (hash, health, alert, metrics) and runs only the ServiceNow scorer. Pattern matches existing `scopeNoExecution` early exit.

#### D6: Completion Logic

`allComponentsDone` and `determineAssessmentReason` updated to handle `targetType == "servicenow"` (only `ServiceNowAssessed` required, K8s components treated as N/A).

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

Serves dual purpose: KA investigation context AND EM pre-remediation baseline.

---

## Consequences

### Positive Consequences

1. End-to-end ServiceNow signal support reusing existing pipeline
2. Contract-driven verification is workflow-agnostic -- works for any future ServiceNow workflow
3. KA verification endpoint is reusable for other non-K8s target types
4. EM remains deterministic for K8s signals (zero regression risk)

### Negative Consequences

1. 9-file plumbing change for AA boundary gap
   - **Mitigation**: Mechanical field propagation, validated by spikes
2. KA availability becomes EM dependency for ServiceNow verification
   - **Mitigation**: Same graceful degradation pattern as Prometheus/AlertManager (assessed-as-skipped, requeue)
3. Two prompt templates need ServiceNow awareness (Phase 1 + Phase 3)
   - **Mitigation**: Conditional blocks mirror existing `SignalMode` pattern

### Risks

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Verification prompt produces inconsistent results | Medium | Low | Structured output schema constrains LLM; per-criterion evaluation provides transparency |
| ServiceNow API rate limiting during EM verification | Low | Medium | Graceful degradation; requeue with backoff |
| Phase 3 template ServiceNow action types not matched by DS catalog | Low | High | Ensure ServiceNow RemediationWorkflow CRDs deployed before demo |

---

## Compliance

| Requirement | Status | Notes |
|-------------|--------|-------|
| BR-INT-004 | Partial | ServiceNow ticket creation via WFE Job executor |
| BR-INT-020 | Full | ServiceNow as signal target type end-to-end |

---

## Validation Strategy

1. **Unit tests**: Per-component logic (parser, prompt rendering, scorer) with mock dependencies
2. **Integration tests**: Pipeline plumbing (RR -> SP -> AA -> KA) with test CRDs
3. **E2E tests**: Full pipeline with mock ServiceNow API + mock LLM
4. **Contract verification tests**: Verify `description.what` parsing and per-criterion evaluation
5. **Spike validation**: 11 spikes executed pre-implementation confirming all assumptions

---

## References

- Issue #1338: feat: Add ServiceNow as a signal target type
- `api/remediationworkflow/v1alpha1/remediationworkflow_types.go`: Workflow description contract
- `internal/kubernautagent/api/openapi.json`: KA OpenAPI specification
- `internal/controller/effectivenessmonitor/reconcile_components.go`: EM component pipeline

---

**Document Version**: 1.0
**Last Updated**: 2026-06-03
