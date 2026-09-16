# BR-SECURITY-554: Threat Remediation

**Business Requirement ID**: BR-SECURITY-554
**Category**: Security
**Priority**: P1 - Proposed
**Target Version**: TBD after product discovery
**Status**: Proposed; not approved or shipped
**Date**: September 15, 2026
**GitHub Issue**: [#554](https://github.com/jordigilh/kubernaut/issues/554)
**Related domain**: Compliance Continuous Remediation ([#669](https://github.com/jordigilh/kubernaut/issues/669))

---

## Business Need

Enterprise Kubernetes and OpenShift environments receive security findings from
runtime detectors, vulnerability scanners, policy engines, Kubernetes audit
pipelines, identity and secrets systems, and SIEM or SOAR platforms. Those
systems remain authoritative for detection, but responders still need to
correlate signals, determine effective risk and blast radius, choose a
proportional response, preserve evidence, and verify that the threat is gone.

The current workaround combines security consoles, static playbooks, tickets,
and manual investigation. Fixed responses can destroy forensic evidence,
interrupt healthy service, or fail to contain a self-recreating or node-level
foothold. Uncorrelated tooling also makes it difficult to reconstruct what was
observed, decided, approved, changed, and verified.

## Business Requirement

Kubernaut SHALL provide a Kubernetes-aware, closed-loop Threat Remediation
capability that turns authoritative security findings into explainable,
policy-governed, auditable, and verified remediation outcomes.

The investigation model SHALL recommend and explain. Independently governed,
registered workflows SHALL be the only mechanism allowed to make state-changing
remediation changes.

This requirement authorizes product discovery and pilot validation only. It
does not claim that Threat Remediation is currently available as an end-to-end
product capability.

---

## Functional Requirements

### BR-SECURITY-554-001: Authoritative Finding Intake

Kubernaut SHALL accept security findings through explicit, versioned source
contracts. Each accepted finding SHALL preserve source identity, source finding
identifier, observed timestamp, cluster and resource identity, principal when
available, source confidence, and the source payload or an immutable evidence
reference.

Finding content SHALL be treated as untrusted input and validated and
sanitized before it is used for investigation or workflow selection.

**Acceptance criteria**:

- A finding can be traced back to its authoritative source and original evidence.
- Invalid or malformed source data is rejected or isolated without being
  represented as a successful remediation.
- Adding a source does not make Kubernaut the authoritative detector for that
  source.

### BR-SECURITY-554-002: Incident Identity and Correlation

Kubernaut SHALL assign or propagate one incident identifier across related
findings, investigation, policy evaluation, approval, workflow execution,
observed state changes, notification, and verification.

Correlation SHALL retain source-specific provenance and SHALL not discard
independent observations when they are associated with one incident.

**Acceptance criteria**:

- An operator can reconstruct the lifecycle of a correlated incident using its
  incident identifier.
- Duplicate, stale, contradictory, and unrelated observations are handled
  explicitly and remain distinguishable in the record.

### BR-SECURITY-554-003: Kubernetes-Aware Investigation Context

The investigation SHALL be able to obtain relevant, read-oriented context for
the affected environment, including workload ownership, image provenance,
network paths, service-account and RBAC grants, secret references, node
placement, recent events, change history, dependent services, fleet identity,
and related security findings.

Context access SHALL respect configured cluster, namespace, resource, and
identity scope.

**Acceptance criteria**:

- The investigation identifies affected workloads and principals using current
  platform context rather than the source finding alone.
- Context retrieval failures and missing data are exposed as uncertainty.
- Read-oriented investigation access cannot directly mutate infrastructure.

### BR-SECURITY-554-004: Explainable Risk Assessment

Kubernaut SHALL produce an explainable assessment containing, where supported,
effective severity, confidence, affected resources and identities, likely
attack stage, supporting evidence, blast radius, lateral-movement paths,
availability and forensic constraints, ranked response options, and residual
risk.

The assessment SHALL distinguish what the source observed from what Kubernaut
inferred.

**Acceptance criteria**:

- A responder can understand why a response was recommended and what evidence
  supports it.
- Low-confidence, incomplete, or conflicting evidence is visible and affects
  the response path.
- A recommendation does not imply that the threat has been contained.

### BR-SECURITY-554-005: Independent Policy and Approval Gate

Recommendations SHALL be evaluated against customer-owned policy and approval
rules independently of the investigation model. Recommendation, policy
evaluation, approval or rejection, execution, and verification SHALL remain
distinct lifecycle states.

The initial product behavior SHALL support the existing auto-approval and
manual-review model. An automatic policy-deny outcome SHALL require a separate
approved product and API design; it SHALL not be implied by a constant or
unwired code path.

**Acceptance criteria**:

- The investigation model cannot grant itself permission to change state.
- High-impact responses default to human review unless a customer explicitly
  enables a tested automation policy.
- Approval and rejection are attributable and visible in the incident record.

### BR-SECURITY-554-006: Registered Deterministic Response Workflows

State-changing remediation SHALL be performed only by registered, versioned,
deterministic workflows. The initial catalog SHOULD prioritize:

- Reversible workload or network isolation that preserves forensic access.
- Credential revocation or rotation.
- Egress or cloud-metadata access restriction.
- Suspected-node cordon with replacement-capacity handling.
- Scoped RBAC tightening.
- Image or workload-revision rollback or replacement.
- Notification and external incident-record updates.

Each workflow SHALL declare and be tested with scoped service accounts,
Kubernetes RBAC, fleet scope, restricted execution context, timeouts, recovery
behavior, and forensic-preservation constraints.

**Acceptance criteria**:

- An unregistered workflow, arbitrary model-generated command, or investigation
  transcript cannot directly execute a state-changing action.
- Each supported workflow has explicit scope, permission, timeout, failure,
  and recovery behavior.
- Workflow registration is not treated as proof of least privilege or safety.

### BR-SECURITY-554-007: Fail-Safe and Evidence-Preserving Response

Threat Remediation SHALL prefer proportional, reversible, and non-destructive
containment where possible. Missing, stale, contradictory, or unverifiable
evidence SHALL result in escalation or review rather than an unqualified
success path.

Response decisions SHALL account for service availability, replacement
capacity, credential propagation, affected identity scope, reversibility, and
forensic preservation.

**Acceptance criteria**:

- High-impact, low-confidence, and contradictory cases are reviewed or
  escalated according to policy.
- Containment does not intentionally erase evidence needed for approved
  forensics.
- Failed, timed-out, blocked, and degraded paths are observable and are not
  reported as successful remediation.

### BR-SECURITY-554-008: Security Outcome Verification

Kubernaut SHALL verify the security outcome independently of workflow
completion. Verification SHALL establish, as applicable, that:

- The original finding cleared or moved to an explicitly accepted residual-risk
  state.
- Suspicious behavior and egress did not recur during a defined observation
  window.
- Revoked credentials no longer authenticate and intended replacement
  credentials work.
- A quarantined workload cannot reach protected systems while approved forensic
  access remains available.
- Replacement capacity and dependent services remain healthy.
- Rescans confirm the intended image, posture, or policy state.

Unverifiable outcomes SHALL escalate instead of being presented as resolved.

**Acceptance criteria**:

- A completed workflow without a verified security outcome is not marked as
  resolved.
- Verification evidence is correlated to the incident and identifies the
  observation window and remaining uncertainty.
- Verification failures and recurrence trigger the configured escalation path.

### BR-SECURITY-554-009: Independent Incident Record and Audit Reconstruction

Kubernaut SHALL independently capture and correlate the source findings,
investigation inputs and outputs, policy version, approval decision, actor,
workflow identity, observed state changes, notifications, and verification
evidence.

The record SHALL not depend solely on a transcript produced inside the workload
under investigation. Retention, immutability or integrity protection,
sensitive-data handling, access control, and export to the customer's system of
record SHALL be defined before production use.

**Acceptance criteria**:

- A complete incident lifecycle is reconstructable by incident identifier
  without manual stitching of unrelated logs.
- Audit events contain the required event identity, action, outcome, actor, and
  correlation fields.
- Security and Compliance approve the retention, integrity, redaction, and
  export behavior for the selected pilot scope.

### BR-SECURITY-554-010: Pilot Validation and Operational Feedback

The initial validation SHALL use one authoritative source and two or three
deterministic response journeys. The first phase SHALL be recommendation-only;
state-changing automation SHALL follow only after analyst agreement, workflow
safety, approval behavior, and verification quality are demonstrated.

The pilot SHALL establish baselines and measure, at minimum:

- Time from finding to trusted decision.
- Time from approved decision to safe containment.
- Analyst acceptance, modification, rejection, and escalation rates.
- Workflow success, recovery, and verification completeness.
- Recurrence and unintended availability or forensic impact.
- Incident reconstruction completeness.

Numeric targets SHALL be agreed with the design partner before they are treated
as product commitments.

**Acceptance criteria**:

- A named pilot scope, source, response journeys, owners, and baselines are
  approved before implementation.
- Recommendation quality is evaluated against analyst decisions before
  enabling state-changing automation.
- Pilot results support an evidence-based packaging and roadmap decision.

---

## Product Boundaries

- Detection, identity, network, admission, secrets, sandbox, SIEM, and SOAR
  products remain authoritative within their domains.
- Threat Remediation complements those products; it does not replace them.
- Kubernaut is not a guarantee against zero-day exploitation, model
  misalignment, insider activity, or every covert channel.
- Agent-runtime supervision is a related expansion, not a prerequisite for the
  narrower Kubernetes threat-response pilot. External LLM, MCP, network,
  filesystem, and kernel observations require their own integrations and
  security review.
- Compliance Continuous Remediation may share technical foundations but remains
  a separately positioned product domain.
- Threat Remediation is a proposed capability and must not be described as
  released until the requirements, implementation, integrations, controls, and
  acceptance evidence are approved.

## Control Mapping

| Control | Requirement application |
|---|---|
| SOC 2 CC7.2 | Monitoring support, investigation evidence, incident reconstruction, and outcome verification |
| SOC 2 CC8.1 | Product change-management process for approving and implementing this capability; not a substitute for event-level audit controls |
| FedRAMP AU-2 / AU-3 | Audit event coverage and structured attribution for the complete lifecycle |
| FedRAMP AU-9 / AU-11 | Integrity protection and retention of incident and audit evidence |
| FedRAMP AC-4 / AC-6 | Information-flow boundaries, scoped execution, and least-privilege responsibility |
| FedRAMP SC-8 | Confidential transport for findings, context, workflow, and evidence exchanges |
| FedRAMP SI-10 | Validation and sanitization of untrusted findings and investigation inputs |

## Implementation Gates

Before implementation begins:

- Product Management, Security, Compliance, Platform, and SRE approve the
  initial scope and ownership.
- Each child requirement is mapped to an owner, success measure, and `UT-`,
  `IT-`, and `E2E-` scenario where implementation is authorized.
- A threat model covers malicious source data, prompt injection, forged or
  stale findings, conflicting telemetry, excessive scope, approval bypass,
  workflow compromise, and unavailable verification.
- A wiring manifest identifies every new production entry point and its
  integration test.
- The recommendation-only journey passes integration and end-to-end evidence
  review before any state-changing automation is enabled.
- Supported sources, workflows, configuration responsibilities, limitations,
  operating procedures, and escalation paths are documented.

## Related Documentation

- [Remediation Approval Architecture](../architecture/decisions/ADR-040-remediation-approval-request-architecture.md)
- [Unified Audit Table Design](../architecture/decisions/ADR-034-unified-audit-table-design.md)
- [Service Audit Trace Requirements](../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md)
- [LLM Input Sanitization](./BR-KA-211-llm-input-sanitization.md)

The companion product-discovery record and multi-pillar data-contract
proposal are reviewed separately in [PR #2406](https://github.com/jordigilh/kubernaut/pull/2406).

## Source and Confidentiality Note

This public requirement is a sanitized abstraction of the Threat Remediation
business-case analysis. It intentionally excludes private blueprint templates,
visual assets, customer or design-partner identity, internal ownership
placeholders, and non-public operational details. Public incident reporting is
treated only as a design case and is not evidence that this capability was
deployed or would have guaranteed prevention.

**Document Version**: 1.0-proposed
**Maintained By**: Kubernaut Product and Architecture teams
