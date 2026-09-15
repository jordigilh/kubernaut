# DD-CONTRACT-006: Pillar Safety and Audit Governance

**Status**: Proposed; pending cross-team review
**Decision Date**: 2026-09-14
**Version**: 1.0
**Confidence**: 92%
**Deciders**: Architecture Team
**Applies To**: Gateway, Signal Processing, AI Analysis, Kubernaut Agent, Remediation Orchestrator, Workflow Execution, Effectiveness Monitor, Audit, Data Storage, and Notification

**Related Business Requirements**:
- `BR-AUDIT-005`: complete remediation lifecycle reconstruction
- `BR-KA-211`: LLM input sanitization
- `TR-REQ-012`: additive multi-pillar extensibility; formal BR pending

**Related Design Decisions**:
- [ADR-075: Multi-Pillar Data Contract Extensibility](ADR-075-multi-pillar-data-contract-extensibility.md)
- [DD-CONTRACT-003: Pillar Extension Envelope](DD-CONTRACT-003-pillar-extension-envelope.md)
- [DD-CONTRACT-005: Multi-Pillar Propagation Boundaries](DD-CONTRACT-005-multi-pillar-propagation-boundaries.md)
- [ADR-034: Unified Audit Table Design](ADR-034-unified-audit-table-design.md)
- [DD-AUDIT-003: Service Audit Trace Requirements](DD-AUDIT-003-service-audit-trace-requirements.md)
- [DD-ERROR-001: Error Details Standardization](DD-ERROR-001-error-details-standardization.md)

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-09-14 | Jordi Gil | Initial proposed pillar safety and audit governance |

---

## Context & Problem

### Current State

Kubernaut already requires structured audit records, correlated lifecycle events, input sanitization, least-privilege execution, and explicit error outcomes. Gateway emits structured signal events and Data Storage reconstructs remediation fields from correlated events. Existing AgentSession comments identify signal annotations and investigation content as untrusted and require curation before LLM use.

The multi-pillar contract introduces data with materially different sensitivity and retention profiles:

- Threat Remediation may contain runtime indicators, identities, credentials, lateral-movement evidence, and forensic details.
- Supply Chain Security may contain SBOMs, package metadata, advisories, reachability, exploitability, and artifact provenance.
- Compliance Remediation may contain control evidence, policy results, exceptions, and organizational information.
- Cost Optimization may contain allocation, account, pricing, budget, and business-unit information.

### Problem Statement

Preserved JSON provides forward compatibility but does not provide semantic validation, authorization, redaction, audit classification, or safe policy access. If raw pillar content moves through logs, LLM prompts, notifications, or workflow parameters without explicit controls, a data-contract change can become a security and compliance regression.

Audit reconstruction also requires more than a raw payload. It must identify the pillar, schema version, decoder result, policy decision, actor, outcome, and correlated lifecycle while avoiding unnecessary sensitive content.

### Constraints

- Every audit event must carry the project-standard event identity, action, outcome, actor, and correlation fields.
- Audit events must be queryable by the RemediationRequest correlation ID.
- Raw source evidence must be retained only where required and must be protected according to its sensitivity.
- Findings, evidence, and provider content are untrusted input.
- Unsupported versions, malformed data, unavailable validation, and contradictory evidence must not result in silent success.
- Workflow execution must receive only approved, scoped, and governed action data.

---

## Decision Drivers

1. Complete and independently captured lifecycle reconstruction.
2. Least-privilege information flow across service boundaries.
3. Redaction before LLM, logs, notifications, and audit persistence.
4. Fail-closed behavior for invalid or unsupported pillar data.
5. Pillar-specific retention and access without weakening common controls.

---

## Alternatives Considered

### Alternative A: Central generic sanitizer and audit handler - Rejected

One shared service or library would sanitize and classify every pillar payload and emit all audit metadata.

**Pros**:
- Centralized behavior and fewer service-level implementations.
- Consistent baseline controls.

**Cons**:
- A generic sanitizer cannot understand all pillar semantics or sensitive fields.
- Centralization creates a bottleneck and broad blast radius for changes.
- Service ownership and authoritative decision points become unclear.

**Confidence**: 94% (rejected as the only control)

### Alternative B: Each service handles safety independently with no common contract - Rejected

Every service would decide its own redaction, audit metadata, and unsupported-version behavior.

**Pros**:
- Local ownership and domain-specific knowledge.
- No central registry requirement.

**Cons**:
- Inconsistent controls and gaps at handoff boundaries.
- Difficult lifecycle reconstruction.
- Duplicate implementations and incompatible event payloads.

**Confidence**: 98% (rejected)

### Alternative C: Common mandatory controls plus pillar-owned validation - CHOSEN

The platform defines mandatory envelope, correlation, audit, redaction, and failure behavior. Each pillar owner defines semantic validation, sensitivity rules, typed projections, and verification details for its data.

**Pros**:
- Consistent platform-wide safety and audit contract.
- Domain experts own semantic validation and data classification.
- New pillars add controls through a standard checklist without changing unrelated services.
- Invalid or unsupported data cannot silently proceed.

**Cons**:
- Requires both shared control enforcement and pillar-specific implementation.
- New pillar onboarding includes security and compliance review.

**Confidence**: 92% (proposed)

---

## Decision

### Chosen: Alternative C - Common mandatory controls plus pillar-owned validation

Every pillar implementation must provide:

- A schema and versioned decoder.
- An explicit sensitivity and retention classification.
- An allowlist of fields permitted in each downstream projection.
- Redaction rules for secrets, credentials, personal information, and security-sensitive evidence.
- Policy-input mapping from validated fields only.
- Workflow parameter mapping from approved fields only.
- Verification criteria and evidence references.
- Audit event and error mappings.
- Malformed-input, unsupported-version, degraded-mode, and manual-review behavior.

### Required Audit Metadata

Pillar-aware audit events must retain, where applicable:

- `event_id`.
- `event_type`.
- `event_action`.
- `event_outcome`.
- `actor_type` and `actor_id`.
- `correlation_id`, using the RemediationRequest lifecycle identity.
- `pillar`.
- `kind`.
- `schema_version`.
- Source identity and target reference.
- Decoder or validator result.
- Policy and workflow version where a decision or action occurred.
- Redaction or evidence-reference metadata without copying sensitive raw data unnecessarily.

Event payloads must use the existing structured audit event conventions. Pillar-specific event types or typed payload variants require the normal audit catalog and reconstruction review; this DD does not authorize arbitrary `event_data` maps as a shortcut.

### Validation and Failure Policy

1. Validate the envelope structure before decoding data.
2. Resolve the `(pillar, kind, schemaVersion)` decoder.
3. Reject unsupported required versions.
4. Validate semantic fields and sensitivity classification.
5. Redact and project only approved fields.
6. Evaluate policy using typed, allowlisted inputs.
7. Fail closed or transition to explicit degraded/manual review on validation failure.
8. Emit an observable failure event with standardized error details.

No service may treat successful deserialization as evidence that the data is safe, authorized, or semantically correct.

### Retention and Access Boundary

- Small, non-sensitive normalized context may be inline in the owning CRD.
- Large, sensitive, asynchronous, or independently retained evidence should be stored in a referenced child resource or evidence store.
- Raw source evidence must remain at the narrowest boundary that satisfies audit reconstruction and operational needs.
- Compliance-category records follow the applicable retention floor and protection requirements; retention is not inferred from the pillar name alone.
- Notifications and operator summaries receive redacted projections and references rather than raw evidence.

### Threat and Pillar Review Gate

Before a new pillar is implemented, its owner must provide a threat model and answer:

- What is the source of the data and its trust level?
- Which fields can contain credentials, personal information, exploit details, or customer-sensitive data?
- Which services need each field?
- Which fields can enter an LLM prompt or policy input?
- Which workflows can consume derived values?
- What evidence is required for verification and reconstruction?
- What is the retention, deletion, legal-hold, and access model?
- What happens when the source is stale, contradictory, malformed, or unavailable?

---

## Consequences

### Positive Consequences

1. Multi-pillar extensibility does not bypass existing security and audit controls.
2. Audit records identify the pillar and decoder context needed for reconstruction.
3. Domain owners can define semantic controls without changing common safety rules.
4. Raw evidence exposure is limited by explicit projection and retention decisions.
5. Unsupported or unsafe data produces an observable non-success outcome.

### Negative Consequences

1. Pillar onboarding requires Security, Compliance, and service-owner review.
   - **Mitigation**: use a standard threat-model, audit-catalog, and test-plan checklist.
2. Audit payload definitions may need typed additions for new pillar events.
   - **Mitigation**: use the existing audit event catalog and builder-registry patterns.
3. Fail-closed handling can increase manual review and degraded outcomes.
   - **Mitigation**: measure validation failures and improve source contracts without silently relaxing safety.

### Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Sensitive pillar data is logged or persisted in raw form | Medium | High | Field classification, redaction, projection allowlists, audit tests |
| Audit records omit a domain decision or decoder result | Medium | High | Required metadata and correlation reconstruction tests |
| A malformed payload is treated as successful remediation | Low | High | Fail-closed state machine and explicit failure events |
| Pillar policy input expands beyond reviewed fields | Medium | High | Typed policy input allowlist and schema review |

---

## Compliance

| Requirement | Status | Notes |
|-------------|--------|-------|
| `BR-AUDIT-005` | Proposed | Correlated, complete, independently captured pillar lifecycle. |
| `BR-KA-211` | Proposed | Pillar evidence is curated and sanitized before LLM exposure. |
| FedRAMP AU-2/AU-3 | Proposed | Structured pillar-aware events carry required audit content. |
| FedRAMP AU-9/AU-11 | Proposed | Evidence protection and retention are explicitly classified. |
| FedRAMP AC-4/AC-6 | Proposed | Projections and allowlists constrain information flow and privilege. |
| FedRAMP SI-10 | Proposed | Structural and semantic input validation is mandatory. |
| SOC 2 CC7.2 | Proposed | Correlated events support monitoring and reconstruction. |
| SOC 2 CC8.1 | Proposed | Review and approval are part of the change-management path. |

---

## Validation Strategy

1. Add per-pillar redaction and sensitivity fixtures, including adversarial source content.
2. Verify all malformed, unsupported-version, stale, contradictory, and unavailable-validation paths.
3. Verify no raw sensitive field reaches LLM, logs, notifications, policy input, or WorkflowExecution without explicit approval.
4. Verify required audit metadata on success, failure, degraded, approval, execution, and verification events.
5. Query reconstructed lifecycle records by correlation ID and compare against the source journey.
6. Verify retention and access behavior for inline versus referenced evidence.
7. Require Security and Compliance sign-off before enabling state-changing workflows for a new pillar.

---

## References

- [ADR-075: Multi-Pillar Data Contract Extensibility](ADR-075-multi-pillar-data-contract-extensibility.md)
- [DD-CONTRACT-003: Pillar Extension Envelope](DD-CONTRACT-003-pillar-extension-envelope.md)
- [DD-CONTRACT-005: Multi-Pillar Propagation Boundaries](DD-CONTRACT-005-multi-pillar-propagation-boundaries.md)
- [ADR-034: Unified Audit Table Design](ADR-034-unified-audit-table-design.md)
- [DD-AUDIT-003: Service Audit Trace Requirements](DD-AUDIT-003-service-audit-trace-requirements.md)
- [DD-ERROR-001: Error Details Standardization](DD-ERROR-001-error-details-standardization.md)
- [AgentSession API types](../../../api/agentsession/v1alpha1/agentsession_types.go)
- [Gateway audit emission](../../../pkg/gateway/audit_emission.go)

---

**Document Version**: 1.0
**Last Updated**: 2026-09-14
