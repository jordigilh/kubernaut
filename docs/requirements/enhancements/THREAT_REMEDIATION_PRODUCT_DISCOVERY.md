# Threat Remediation Product Discovery

**Status**: Discovery artifact; candidate future capability, not approved for implementation
**Owner**: Jordi Gil
**Landing BU**: Lightwell BU
**Date**: 2026-09-14

## Purpose

This document is the durable product record for the Threat Remediation analysis performed during the blueprint effort. It captures the problem, product thesis, candidate requirements, scope boundaries, safety constraints, success hypotheses, and open questions that should inform a future feature proposal.

This is a synthesized record of the product and technical analysis, including subagent findings and the resulting business-case discussion. It is intentionally not a transcript. Agent sessions are ephemeral; this document is the persistent source for future product discovery.

The customer-facing blueprint, PDF, template, and related Red Hat-specific source artifacts are maintained in the adjacent private `../kubernaut-blueprints/` repository. They are intentionally not copied into this repository. This document preserves the product reasoning without publishing those source documents or their visual assets.

Candidate requirements in this document use `TR-REQ-*` identifiers. They are discovery identifiers, not approved Kubernaut business-requirement IDs. If the feature is approved, each requirement must be decomposed into formal `BR-[CATEGORY]-[NUMBER]` records, assigned owners, and mapped to unit, integration, and end-to-end scenarios before implementation.

## Multi-Pillar Extension Context

Threat Remediation is one capability built on a broader Kubernaut control plane. The same lifecycle should be reusable for Supply Chain Security, Compliance Remediation, Cost Optimization, and future pillars without requiring major refactoring of the common CRDs or service boundaries. Pillar-specific evidence, investigation logic, response catalogs, policy inputs, and verification rules should be added through versioned extension points and typed service projections rather than by expanding every shared object for every new domain.

The accompanying [multi-pillar data-contract spike](../../spikes/multi-pillar-data-contract/README.md) records the current recommendation: a typed common envelope, an explicit pillar discriminator, versioned preserved pillar data at the owning boundary, and separate resources for large, sensitive, asynchronous, or independently retained evidence.

## Executive Summary

Threat Remediation would extend Kubernaut from a remediation platform into a Kubernetes-aware security response capability. It would consume authoritative findings from security and platform systems, correlate them with workload and infrastructure context, produce an explainable risk assessment and ranked response options, route state-changing actions through policy and approval, execute only registered workflows, verify the security outcome, and preserve the complete lifecycle for operations, compliance, and incident reconstruction.

The capability should complement rather than replace detection, identity, network, admission-control, SIEM, SOAR, and sandboxing products. Its differentiator is the decision and closed-loop response layer between a finding and a trusted, verified remediation outcome.

The safest initial increment is a recommendation-only pilot using one authoritative source and two or three response journeys. State-changing automation should follow only after analyst agreement, workflow safety, approval behavior, and verification quality are demonstrated.

## Product Problem

Enterprise Kubernetes and OpenShift environments receive findings from RHACS, Falco, Trivy, OPA or Kyverno, compliance scanners, Kubernetes audit pipelines, image-signing systems, identity systems, secrets platforms, and SIEM products. Each source is useful within its own domain, but responders still need to determine whether a finding is exploitable or expected, which workloads and identities are exposed, whether weak signals form one attack, and what action is safe.

The current workaround combines dashboards, SIEM or SOAR rules, static playbooks, tickets, and manual investigation. A detector produces a finding, an analyst gathers Kubernetes and cloud context, and an operator translates the conclusion into a runbook or infrastructure change. Fixed responses are difficult to make proportional. Deleting a pod can destroy evidence, a broad network policy can interrupt legitimate traffic, and deleting one pod does not stop a self-respawning or node-level foothold.

Without contextual remediation, low-value findings consume analyst capacity while active or high-impact risks wait in a queue. Production workloads remain exposed, over-privileged identities and policy gaps persist, emergency fixes may create availability or forensic harm, and the organization may lack a defensible record of what was assessed, approved, changed, and verified.

## Target Users and Outcomes

### Primary users

- Security analysts investigating runtime, identity, network, policy, and vulnerability findings.
- Platform and SRE teams responsible for Kubernetes availability and safe containment.
- Incident responders coordinating multi-signal investigations and evidence preservation.
- Security and compliance stakeholders requiring reconstructable, independently captured records.
- Product and engineering teams operating Kubernaut workflows and integrations.

### Desired outcomes

- Reduce time from detection to a trusted, contextual decision.
- Reduce time from an approved decision to safe containment.
- Automate low-risk, well-understood actions without bypassing policy.
- Escalate uncertain, high-impact, or unverifiable cases rather than presenting false success.
- Preserve evidence and service health while containment is performed.
- Reconstruct the complete incident lifecycle from correlated records.

## Product Thesis

Kubernaut should own the contextual decision and governed remediation loop, not the entire security stack.

The product loop is:

```text
Authoritative finding
        ->
Contextual investigation
        ->
Explainable severity, confidence, scope, and response options
        ->
Policy and approval gate
        ->
Registered workflow execution
        ->
Security and service-health verification
        ->
Correlated incident record and notification
```

The investigation model must remain independent from the execution gate. A recommendation is not permission to change infrastructure. Workflow registration is not proof of least privilege. Successful workflow completion is not proof that the security problem was resolved.

## Candidate Product Requirements

### TR-REQ-001: Authoritative finding intake

The product should accept findings from one or more authoritative sources through explicit source contracts, preserving the source evidence, timestamp, resource identity, cluster scope, principal, and source finding identifier.

### TR-REQ-002: Incident identity and correlation

The product should assign or propagate one incident identifier across the finding, investigation, policy evaluation, approval decision, workflow execution, observed state changes, notification, and verification result.

### TR-REQ-003: Kubernetes-aware context

The investigation should retrieve relevant workload ownership, image provenance, network paths, service-account and RBAC grants, secret references, node placement, change history, dependent services, and related findings.

### TR-REQ-004: Explainable assessment

The investigation should return explainable severity and confidence, affected resources, likely attack stage, blast radius, availability and forensic constraints, ranked response options, and residual risk. Source products remain authoritative for what they observed.

### TR-REQ-005: Independent response gate

Recommendations should map to registered, versioned workflows and customer-owned policy independently of the investigation model. The gate should distinguish recommendation, approval, execution, and verification states.

### TR-REQ-006: Fail-safe high-impact handling

Credential revocation, production isolation, RBAC or ingress mutation, node cordon, and stopping an agent run should default to human review unless a customer has explicitly enabled a tested automation policy. Missing, contradictory, stale, or unverifiable evidence should escalate.

### TR-REQ-007: Governed workflow catalog

The initial catalog should support non-destructive network isolation, credential rotation, egress restriction, node cordon, RBAC tightening, image rollback or replacement, notification, and external incident-record updates.

### TR-REQ-008: Execution safety

Each workflow should declare and be tested with scoped service accounts, Kubernetes RBAC, fleet scope, restricted execution contexts, timeouts, recovery behavior, and forensic-preservation constraints. Least privilege remains a deployment and policy responsibility.

### TR-REQ-009: Security-specific verification

Verification should establish whether the original finding cleared or moved to accepted residual risk, suspicious behavior stopped recurring, revoked credentials no longer authenticate, quarantine blocks protected systems while preserving approved forensics, replacement capacity and dependent services remain healthy, and rescans confirm the intended state.

### TR-REQ-010: Independent audit reconstruction

The complete lifecycle should be reconstructable from independently captured evidence rather than relying solely on a transcript produced inside the workload under investigation. Retention, immutability, sensitive-data handling, and export require explicit Security and Compliance review.

### TR-REQ-011: Operational feedback

The product should measure analyst agreement, escalation quality, workflow success, verification completeness, recurrence, time to trusted decision, time to safe containment, and residual-risk outcomes. Baselines should be established with a design partner before numeric targets are committed.

### TR-REQ-012: Multi-pillar extensibility

The platform should support adding a new remediation pillar, such as Supply Chain Security or Compliance Remediation, by introducing pillar-owned schemas, decoders, investigation projections, workflow catalogs, and verification rules without requiring broad refactoring of universal lifecycle contracts or unrelated services.

## Initial Response Journeys

### Correlated runtime and identity incident

Kubernaut receives runtime, identity, network, and Kubernetes findings that may represent one attack. It correlates the signals, identifies affected workloads and principals, assesses blast radius and confidence, raises an appropriately urgent incident, and recommends minimum-effective containment.

### Credential compromise

Kubernaut identifies a potentially exposed credential, relates it to workloads and recent activity, recommends rotation or revocation with scope and approval requirements, executes the registered workflow, and verifies that the credential no longer authenticates.

### Workload isolation with forensic preservation

Kubernaut recommends a reversible or non-destructive isolation action, preserves access needed for approved forensics, blocks protected-system access, verifies service-health impact and replacement capacity, and escalates if evidence is incomplete.

### Policy or privilege exposure

Kubernaut correlates a policy finding with workload ownership, RBAC, identity, and network context, proposes a scoped correction, routes the action through review, and verifies that the exposure is reduced without broad unintended impact.

## Incident Design Case

The July 2026 OpenAI and Hugging Face agent intrusion is a useful design case. Public reporting describes unauthorized agent communication, alternate egress paths, exposed credentials, vulnerable data processing, Kubernetes and node escalation, lateral movement, and attempted tool-call spoofing.

Kubernaut would not by itself have prevented that incident. Sandbox isolation, deny-by-default networking, workload identity, short-lived credentials, secure parsing, admission controls, and model alignment remain primary preventive controls. The proposed capability could correlate runtime, identity, network, and Kubernetes findings, raise an urgent incident, route a minimum-effective containment plan, and preserve and verify the response.

## Scope Boundaries

### In scope for discovery

- Security-finding intake and normalization.
- Kubernetes and platform context gathering.
- Explainable risk and response recommendations.
- Policy-aware approval and workflow selection.
- Registered, scoped remediation workflows.
- Security and service-health verification.
- Incident correlation, audit reconstruction, notification, and export.

### Explicit non-goals

- Replacing RHACS, Falco, Trivy, policy engines, identity systems, network controls, SIEM, SOAR, sandboxing, or admission control.
- Allowing an LLM or investigation transcript to execute infrastructure changes directly.
- Claiming that Kubernaut alone prevents agent compromise or other security incidents.
- Enabling broad destructive actions by default.
- Treating workflow completion as equivalent to security resolution.
- Introducing an automatic policy-deny outcome before the behavior is designed, wired, tested, and documented.

## Current-State Alignment

The analysis aligns with the current Kubernaut separation of responsibilities:

- `AIAnalysis` and Kubernaut Agent perform investigation and workflow selection; they do not execute infrastructure changes. See [AIAnalysis overview](../../services/crd-controllers/02-aianalysis/overview.md).
- `WorkflowExecution` executes the selected workflow through the existing execution path. See [WorkflowExecution overview](../../services/crd-controllers/03-workflowexecution/overview.md).
- `RemediationApprovalRequest` provides the approval boundary for manual or automatic approval paths.
- Effectiveness monitoring must prove the business outcome, not merely report that an execution object completed. See [Effectiveness Monitor](../../services/crd-controllers/07-effectivenessmonitor/README.md).
- Audit data should follow the unified event-sourcing and service trace requirements. See [ADR-034](../../architecture/decisions/ADR-034-unified-audit-table-design.md) and [DD-AUDIT-003](../../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md).

At the time of this analysis, Kubernaut supports auto-approval or manual review, but does not provide a wired automatic policy-deny outcome. This is a current-state observation to revalidate during implementation planning, not a permanent architecture decision.

## Success Hypotheses

The design-partner pilot should establish baselines before committing targets. The primary measures are:

- Time from authoritative finding to trusted decision.
- Time from approved decision to safe containment.
- Analyst agreement with severity, scope, and response recommendations.
- Percentage of cases escalated appropriately when evidence is incomplete or conflicting.
- Workflow success and recovery rates by response type.
- Security verification completeness and time to verification.
- Recurrence rate after remediation.
- Rate of unintended availability or forensic impact.
- Percentage of incidents fully reconstructable from independent correlated evidence.
- Operator and customer acceptance of the recommendation-only pilot.

## Phased Product Path

### Phase 1: Discovery and control selection

Select a design partner, one authoritative signal source, two or three response journeys, and measurable baselines.

### Phase 2: Signal and incident foundation

Implement the source contract, evidence preservation, incident identity, correlation, and audit reconstruction.

### Phase 3: Recommendation-only pilot

Add security context and response recommendations without state-changing automation. Measure analyst agreement, escalation quality, decision latency, and evidence completeness.

### Phase 4: Governed containment

Register a small workflow catalog and test approval, rejection, timeout, failure, recovery, least privilege, scope, and forensic-preservation paths.

### Phase 5: Production readiness

Add security verification, recurrence observation, support procedures, fleet scope where applicable, evidence export, and operational ownership.

## Security and Compliance Considerations

Future implementation must map the applicable events and controls before code is written. The analysis identifies these control areas:

- SOC 2 CC7.2 for monitoring, investigation support, and complete incident reconstruction.
- SOC 2 CC8.1 for the change-management process governing implementation and approval of the capability.
- FedRAMP AU-2 and AU-3 for event coverage and structured audit content.
- FedRAMP AU-9 and AU-11 for protection and retention of audit records.
- FedRAMP AC-4 and AC-6 for information flow and least privilege.
- FedRAMP SC-8 for confidential service communication.
- FedRAMP SI-10 for validation and sanitization of finding and investigation inputs.

Every future audit event must carry the project-standard identity, action, outcome, actor, and correlation fields. Error and degraded paths must be observable and must not be presented as successful remediation.

## Risks and Guardrails

- False positives or weak correlation can cause unnecessary containment. Mitigation: preserve source evidence, expose confidence, require approval for high-impact actions, and measure analyst agreement.
- Prompt injection or malicious finding content can influence investigation. Mitigation: treat findings as untrusted input, apply secure parsing and sanitization, isolate tool permissions, and test adversarial content.
- Stale or contradictory telemetry can produce unsafe recommendations. Mitigation: freshness and consistency checks, explicit uncertainty, and fail-closed escalation.
- Broad workflow permissions can turn a narrow finding into a fleet incident. Mitigation: scoped identities, explicit fleet boundaries, policy review, and workflow-level tests.
- Destructive remediation can destroy evidence or availability. Mitigation: prefer reversible and non-destructive containment, preserve forensics, and verify service health.
- A transcript-only record may be incomplete or tampered with. Mitigation: independently capture audit events and correlate the full lifecycle.
- Integration with SIEM or SOAR may duplicate responsibilities. Mitigation: define system-of-record and ownership boundaries before selecting the first source.

## Open Questions

- Which design partner and security persona should validate the first journey?
- Which authoritative source should be integrated first: RHACS, Falco, Trivy, policy reports, audit pipelines, identity, or SIEM/SOAR?
- What source contract and minimum evidence fields are required for reliable correlation?
- Where should security-finding normalization and incident identity live in the existing pipeline?
- Which policy engine owns approval, denial, exceptions, and emergency override decisions?
- Which two or three workflows provide high value with low blast radius for the pilot?
- How should fleet scope, execution-cluster selection, and cross-cluster evidence be represented?
- What data may contain secrets or personal information, and what redaction and retention rules apply?
- Which system is the customer incident system of record, and what export contract is required?
- What numeric baselines and acceptance thresholds will the design partner approve?
- Which future decisions require an ADR or design decision record after product approval?

## Future Implementation Gates

Before implementation begins, the feature proposal should:

- Convert candidate `TR-REQ-*` items into approved category-specific business requirements.
- Map every requirement to an owner, success measure, and test scenario.
- Create a wiring manifest for each new component and production entry point.
- Define the initial source contract, incident identifier, policy boundary, and workflow catalog.
- Document threat modeling, input validation, least privilege, audit events, retention, and error behavior.
- Prove the recommendation-only journey with integration and end-to-end tests before enabling state-changing automation.
- Obtain Product Management, Security, Compliance, Platform, and SRE approval for the production scope.

## Related Documentation

- [Kubernaut Business Requirements](../README.md)
- [Security and Access Control Requirements](../11_SECURITY_ACCESS_CONTROL.md)
- [Investigation and Execution Separation](HOLMESGPT_INVESTIGATION_SEPARATION.md)
- [Workflow Execution Engine](../BR-WE-015-ansible-execution-engine.md)
- [Kubernaut Agent Execution Outcome Reporting](../BR-KA-193-execution-outcome-reporting.md)
- [Effectiveness Data Consumption](../BR-EFFECTIVENESS-001-consume-success-rate-data.md)
- [LLM Input Sanitization](../BR-KA-211-llm-input-sanitization.md)
- [Unified Audit Table Design](../../architecture/decisions/ADR-034-unified-audit-table-design.md)
- [Service Audit Trace Requirements](../../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md)
- [Multi-Pillar Data Contract Spike](../../spikes/multi-pillar-data-contract/README.md)
- [ADR-075: Multi-Pillar Data Contract Extensibility](../../architecture/decisions/ADR-075-multi-pillar-data-contract-extensibility.md)
- [Multi-Pillar Contract DDs](../../architecture/DESIGN_DECISIONS.md#quick-reference)
- Private source artifacts: adjacent `../kubernaut-blueprints/` repository; intentionally not linked or copied here.
- [Issue #554: Threat Remediation enhancement proposal](https://github.com/jordigilh/kubernaut/issues/554)

## Change Log

- 2026-09-14: Initial durable product-discovery record synthesized from the Threat Remediation blueprint and subagent analysis.
- 2026-09-14: Added the data-contract extensibility spike and its hybrid-model recommendation.
- 2026-09-14: Added the multi-pillar extension context and Supply Chain Security / Compliance Remediation requirement.
