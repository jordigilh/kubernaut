# ADR-063: ServiceNow Signal Integration Architecture

**Status**: PROPOSED
**Date**: 2026-06-04
**Decision Makers**: Architecture Team
**Confidence**: 94%

**Related Decisions**:
- [DD-INT-020](DD-INT-020-servicenow-signal-target-type.md) -- Detailed design for ServiceNow signal target type
- [ADR-041](adr-041-llm-contract/ADR-041-llm-prompt-response-contract.md) -- LLM prompt response contract (extended for ServiceNow investigation)
- [ADR-001](ADR-001-crd-microservices-architecture.md) -- CRD microservices architecture

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.1 | 2026-06-04 | Architecture Team | Dropped decisions 3 (KA verification endpoint), 4 (contract-driven verification), and 5 (EM early-exit). ServiceNow audit trail sufficient for POC. |
| 1.0 | 2026-06-03 | Architecture Team | Initial ADR: four architectural decisions for ServiceNow signal integration |

---

## 1. Context and Problem Statement

Kubernaut's remediation pipeline is designed around Kubernetes-originated signals. Every component in the chain -- from enrichment through investigation to effectiveness verification -- assumes K8s API access and K8s resource semantics.

Extending the pipeline to support ServiceNow tickets as signal sources requires two architectural decisions:

1. How to discriminate ServiceNow signals from K8s signals throughout the pipeline
2. How to gate K8s-specific behavior (enrichment, hashing, scope) for non-K8s targets

Additionally, KA investigation (tools, prompts, correlation reasoning) is covered in DD-INT-020 as an implementation detail rather than an architectural decision.

EM verification for ServiceNow signals has been deferred -- ServiceNow's native audit trail makes automated effectiveness assessment redundant for the POC scope.

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

## 3. Decision: No EM Verification for ServiceNow (POC Scope)

### Context

EM currently uses four deterministic scorers (hash, health, alert, metrics) that assume K8s API access. The question is whether ServiceNow signals need an analogous verification mechanism.

Three approaches were considered:

| Approach | Description | Verdict |
|----------|-------------|---------|
| **Deterministic rules** | Check specific field values (state=resolved) | Insufficient -- verifies state change, not intent fulfillment |
| **LLM client in EM** | Embed LLM client directly in EM controller | Rejected -- duplicates KA infrastructure, makes EM non-deterministic |
| **KA verification endpoint** | New REST endpoint in KA; EM calls as HTTP client | Deferred -- solves a problem that doesn't exist for ServiceNow |
| **No EM verification** | WFE success/failure is sufficient; ServiceNow audit trail self-documents | **Chosen for POC** |

### Decision

**EM does not perform ServiceNow-specific verification for the POC.** WFE success/failure is the only signal EM needs.

### Rationale

- **ServiceNow is its own audit trail**: Every ticket state change is recorded with timestamps, actors, and reasons. This is fundamentally different from K8s resources, where Kubernaut has no visibility into what changed.
- **WFE binary outcome is sufficient**: If the ServiceNow CLI container exits 0, the API calls succeeded (ticket closed, escalation created, etc.). If it exits non-zero, the Job failed. The ServiceNow audit trail documents exactly what happened in either case.
- **Avoid speculative complexity**: Contract-driven verification via a KA endpoint is a sophisticated mechanism that solves a problem that doesn't exist for ServiceNow tickets. Building it now would be speculative engineering.
- **Clear upgrade path**: If a future requirement emerges (e.g., complex multi-step workflows with partial-success semantics), the KA verification endpoint can be introduced as a new feature, not retrofitted into a codebase that was designed around it.

### Consequences

- EM is untouched for the POC -- zero regression risk for K8s signals
- No EA CRD changes needed (no `TargetType`, `ProviderData`, `ServiceNowAssessed` fields)
- No new `AssessmentReason` enum values
- ServiceNow EAs complete through the standard degraded/no-execution path

---

## 6. ProviderData Schema

ServiceNow `ProviderData` serves dual purpose: KA investigation context AND EM pre-remediation baseline. Schema defined in DD-INT-020.

Key fields: `instance_url`, `ticket.sys_id`, `ticket.number`, `ticket.short_description`, `ticket.description`, `ticket.state`, `ticket.priority`, `ticket.assigned_to`, `ticket.assignment_group`, `ticket.category`, `ticket.cmdb_ci.*`, timestamps.

---

## 7. Scope and Deferrals

### In Scope (Customer Evaluation)

- Pipeline plumbing (Part A): propagate `TargetType` + `ProviderData` through AA boundary
- KA investigation + prompt + tools (Part B, B1-B5): enrichment gating, prompt templates, dynamic tool registration, parser/types for correlation fields
- RO guardrails (Part C, C1-C2): `CapturePreRemediationHash` guard for non-K8s targets
- ServiceNow Go REST API client with mock httptest server
- Two RemediationWorkflow CRDs (close + escalate)
- E2E test with mock ServiceNow + mock LLM

### Deferred to Production GA

- **CHG (Change Request) use case**: Post-change validation -- after a change is deployed, investigate whether affected resources are healthy. Same pipeline, new prompt blocks and workflow CRDs (confirm success / roll back / escalate incident). No architecture changes required.
- **PRB (Problem) use case**: Problem investigation -- correlate multiple incidents on the same CI, find common root cause. Same pipeline, new prompt blocks and workflow CRDs (link RCA to problem / escalate to engineering). No architecture changes required.
- EM ServiceNow verification (Part D): contract-driven verification via KA endpoint
- KA verification endpoint (Part B6): `POST /verify-effectiveness`
- ProviderData propagation to EA (Part C3): only needed if EM verification is introduced
- ServiceNow API circuit breaker and rate-limit handling
- CMDB response caching
- Multi-instance ServiceNow support
- Shared `ResourceResolver` interface (K8s + ServiceNow)
- Credential rotation runbook
- `targetType`-based scope bypass in `scope.Manager.IsManaged` (namespace fallback sufficient for POC)

**Extensibility note**: The POC targets INC (incident) tickets only. The architecture is ticket-type-agnostic by design: `TargetType="servicenow"` is not incident-specific, `ProviderData` holds any ServiceNow ticket type (INC, CHG, PRB all share the same core fields), ServiceNow tools are generic, and workflow selection is driven by RemediationWorkflow CRDs. Extending to CHG and PRB is additive -- new prompt conditional blocks + new workflow CRDs, no pipeline or architecture changes.

---

## 8. References

- [DD-INT-020](DD-INT-020-servicenow-signal-target-type.md): Detailed design document
- Issue #1338: feat: Add ServiceNow as a signal target type
- `api/remediationworkflow/v1alpha1/remediationworkflow_types.go`: `RemediationWorkflowDescription`

---

**Document Version**: 1.1
**Last Updated**: 2026-06-04
