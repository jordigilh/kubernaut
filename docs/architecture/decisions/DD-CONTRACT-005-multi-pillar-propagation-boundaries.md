# DD-CONTRACT-005: Multi-Pillar Propagation Boundaries

**Status**: Proposed; pending cross-team review
**Decision Date**: 2026-09-14
**Version**: 1.0
**Confidence**: 93%
**Deciders**: Architecture Team
**Applies To**: Gateway, Remediation Orchestrator, Signal Processing, AI Analysis, Kubernaut Agent, Workflow Execution, Effectiveness Monitor, Audit, and Notification

**Related Business Requirements**:
- `BR-ORCH-025`: workflow data pass-through to child CRDs
- `BR-AUDIT-005`: complete correlated lifecycle reconstruction
- `TR-REQ-012`: additive multi-pillar extensibility; formal BR pending

**Related Design Decisions**:
- [ADR-075: Multi-Pillar Data Contract Extensibility](ADR-075-multi-pillar-data-contract-extensibility.md)
- [DD-CONTRACT-002: Service Integration Contracts](DD-CONTRACT-002-service-integration-contracts.md)
- [DD-CONTRACT-003: Pillar Extension Envelope](DD-CONTRACT-003-pillar-extension-envelope.md)
- [ADR-016: Validation Responsibility Chain](ADR-016-validation-responsibility-chain.md)

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-09-14 | Jordi Gil | Initial proposed propagation boundaries |

---

## Context & Problem

### Current State

The production lifecycle is already split into service-owned CRDs:

```text
Gateway -> RemediationRequest -> SignalProcessing -> AIAnalysis -> AgentSession
                                      \-> AIAnalysis -> Remediation Orchestrator -> WorkflowExecution
                                                                    \-> EffectivenessAssessment
```

Gateway creates the RemediationRequest and preserves source evidence. Remediation Orchestrator copies the signal into SignalProcessing. Signal Processing produces normalized classifications and enrichment. AIAnalysis receives typed context from Signal Processing and creates an AgentSession. Remediation Orchestrator maps the selected workflow to WorkflowExecution and creates EffectivenessAssessment for verification.

The current path does not have a pillar field. As a result, a future pillar envelope would be dropped when AIAnalysis and AgentSession are built unless a new projection is introduced. Rego policy input also currently lacks a pillar dimension.

### Problem Statement

Passing the same raw pillar payload through every CRD creates coupling, increases data exposure, and makes every consumer responsible for parsing every pillar. Passing no pillar context prevents domain-specific investigation, policy, execution, and verification.

The system needs explicit handoff boundaries where each service receives the minimum validated representation required for its responsibility.

### Constraints

- The parent RemediationRequest remains the correlation and lifecycle anchor.
- RemediationRequest spec data is immutable after creation.
- Signal Processing owns deterministic normalization, not cross-domain reasoning.
- AIAnalysis and Kubernaut Agent investigate and select workflows; they do not execute infrastructure changes.
- Workflow Execution receives governed action data, not raw source evidence by default.
- Effectiveness Monitor verifies the business outcome and may need pillar-specific criteria.
- Audit records must reconstruct the lifecycle using the common correlation ID.

---

## Decision Drivers

1. Preserve existing service ownership and lifecycle state machines.
2. Prevent raw pillar payload propagation and unnecessary sensitive-data exposure.
3. Keep policy and workflow execution inputs typed and bounded.
4. Make a new pillar additive to the boundaries that consume it.
5. Preserve correlation and audit reconstruction across all handoffs.

---

## Alternatives Considered

### Alternative A: Propagate the raw pillar envelope through every CRD - Rejected

Copy the original pillar envelope from RemediationRequest through SignalProcessing, AIAnalysis, AgentSession, WorkflowExecution, and EffectivenessAssessment.

**Pros**:
- Simple pass-through implementation.
- Every service can inspect the original data.

**Cons**:
- Every service becomes coupled to every pillar.
- Sensitive evidence reaches services that do not need it.
- Duplicate parsing, validation, and redaction logic is likely.
- Raw data encourages policy and execution code to depend on unvalidated fields.

**Confidence**: 99% (rejected)

### Alternative B: Keep pillar data only in Signal Processing - Rejected

Signal Processing would retain all domain context and downstream services would only receive generic classifications.

**Pros**:
- Minimal downstream schema changes.
- Centralized data handling.

**Cons**:
- AIAnalysis cannot perform pillar-specific investigation.
- Policy, execution, and effectiveness cannot receive required validated inputs.
- Signal Processing becomes a domain-logic and cross-domain reasoning bottleneck.

**Confidence**: 98% (rejected)

### Alternative C: Typed service projections at explicit boundaries - CHOSEN

Signal Processing validates and normalizes the pillar envelope. Each consuming service receives a typed or validated projection appropriate to its role.

**Pros**:
- Data ownership follows service responsibility.
- Raw evidence exposure is minimized.
- Each new pillar adds only the projections it needs.
- Policy, execution, and verification receive bounded inputs.

**Cons**:
- Multiple projection types and mapping tests are required.
- Cross-service contract coordination is necessary.

**Confidence**: 93% (proposed)

---

## Decision

### Chosen: Alternative C - Typed service projections at explicit boundaries

The common correlation identity is the RemediationRequest name and must be propagated through child references and audit events. Pillar identity, kind, and schema version must be propagated wherever the service needs to interpret or audit pillar-specific behavior.

### Architecture

```text
Gateway
  | common signal + source evidence + optional pillar envelope
  v
RemediationRequest
  | immutable source contract and correlation anchor
  v
SignalProcessing
  | validates and normalizes pillar data
  | emits validated pillar projection
  v
AIAnalysis
  | typed investigation context + pillar-aware policy input
  v
AgentSession / KA
  | curated pillar context; no unbounded source payload
  v
Remediation Orchestrator
  | approval and workflow policy by pillar/action class
  +---------------------> WorkflowExecution
  |                         governed plan and parameters only
  +---------------------> EffectivenessAssessment
                            pillar-specific verification plan/reference
```

### Boundary Contracts

- Gateway to RemediationRequest: preserve `signalSource`, existing `signalType`, target identity, correlation metadata, legacy `ProviderData`, and the optional pillar envelope when available from the source contract.
- RemediationRequest to SignalProcessing: copy the common signal and pillar envelope without changing the existing ProviderData string contract.
- Signal Processing to AIAnalysis: provide normalized severity/classification, deterministic enrichment, and a validated pillar investigation projection containing pillar, kind, schema version, and only the approved context fields or reference.
- AIAnalysis to AgentSession: carry the same validated pillar projection into the immutable investigation request. The projection must be curated before LLM input.
- AIAnalysis to policy evaluation: provide pillar and kind as typed policy inputs plus an allowlisted set of validated pillar attributes. Preserved JSON is not a policy input.
- AIAnalysis to Remediation Orchestrator: return a recommendation, workflow snapshot, action class, policy decision, approval state, and pillar identity. Raw source evidence is not required for workflow creation.
- Remediation Orchestrator to WorkflowExecution: provide the governed workflow reference, target, parameters, execution identity, and required pillar action metadata. Do not pass the original pillar evidence by default.
- Remediation Orchestrator to EffectivenessAssessment: provide the correlation ID, targets, and a pillar-specific verification plan or evidence reference when generic checks are insufficient.
- Audit and notification: record pillar identity, kind, schema version, and decision outcomes while redacting sensitive evidence and avoiding generic raw-payload storage.

### Fan-Out and Correlation

A single source signal may result in more than one pillar-specific remediation request only through explicit fan-out. Each resulting RemediationRequest gets its own lifecycle identity and retains a parent source correlation reference. A polymorphic payload must not silently cause multiple domain workflows.

### Additive Change Rule

Adding a new pillar must touch only the common envelope integration and the boundaries that consume that pillar. A service that does not consume the new pillar must not require a new domain-specific field, parser, or behavior change.

---

## Consequences

### Positive Consequences

1. Each service receives the minimum data required for its responsibility.
2. Raw and sensitive evidence is less likely to reach execution or notification paths.
3. New pillars can add projections without forcing every consumer to understand them.
4. Existing investigation/execution separation remains intact.
5. Correlation and audit reconstruction remain explicit.

### Negative Consequences

1. Projection schemas must be maintained across service boundaries.
   - **Mitigation**: contract tests, shared field ownership, and versioned projection definitions.
2. A pillar may require changes in several consuming services.
   - **Mitigation**: only services that consume the pillar are in scope; unrelated services remain unchanged.
3. Policy inputs need a controlled allowlist for pillar-specific fields.
   - **Mitigation**: validate at SP, map explicitly into policy input, and reject unapproved fields.

### Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| A projection accidentally includes raw sensitive evidence | Medium | High | Allowlist mapping, redaction tests, field-level ownership |
| Pillar context is dropped at a handoff | Medium | High | End-to-end contract test and required correlation metadata |
| Fan-out produces duplicate or conflicting remediation loops | Low | High | Explicit child identities, idempotency, parent correlation, policy gate |
| WFE becomes dependent on domain investigation schema | Low | High | Pass governed action plan only |

---

## Compliance

| Requirement | Status | Notes |
|-------------|--------|-------|
| `BR-ORCH-025` | Proposed | Defines explicit, tested data pass-through and projection boundaries. |
| `BR-AUDIT-005` | Proposed | Correlation and pillar metadata are required across lifecycle events. |
| FedRAMP AC-4/AC-6 | Proposed | Projections enforce bounded information flow and least privilege. |
| FedRAMP SI-10 | Proposed | Pillar data is validated before service projection or LLM use. |
| SOC 2 CC7.2 | Proposed | End-to-end lifecycle remains reconstructable from correlated evidence. |

---

## Validation Strategy

1. Add contract fixtures for all initial pillars and one synthetic future pillar.
2. Verify Gateway, RR, and SP preserve the envelope and correlation identity.
3. Verify AIAnalysis and AgentSession receive the validated typed projection.
4. Verify policy input contains only the allowlisted pillar fields.
5. Verify WorkflowExecution and notification do not receive raw pillar evidence by default.
6. Verify EffectivenessAssessment receives the correct pillar verification reference.
7. Verify one-source-to-one-RR behavior and explicit multi-pillar fan-out behavior.
8. Reconstruct the full lifecycle by correlation ID from audit records.

---

## References

- [ADR-075: Multi-Pillar Data Contract Extensibility](ADR-075-multi-pillar-data-contract-extensibility.md)
- [DD-CONTRACT-003: Pillar Extension Envelope](DD-CONTRACT-003-pillar-extension-envelope.md)
- [DD-CONTRACT-004: ProviderData Compatibility Boundary](DD-CONTRACT-004-provider-data-compatibility.md)
- [Multi-Pillar Data Contract Spike](../../spikes/multi-pillar-data-contract/README.md)
- [SignalProcessing creator](../../../pkg/remediationorchestrator/creator/signalprocessing.go)
- [AIAnalysis creator](../../../pkg/remediationorchestrator/creator/aianalysis.go)
- [AgentSession request builder](../../../pkg/aianalysis/handlers/request_builder.go)
- [WorkflowExecution creator](../../../pkg/remediationorchestrator/creator/workflowexecution.go)
- [EffectivenessAssessment creator](../../../pkg/remediationorchestrator/creator/effectivenessassessment.go)

---

**Document Version**: 1.0
**Last Updated**: 2026-09-14
