# ADR-075: Multi-Pillar Data Contract Extensibility

**Date**: 2026-09-14
**Status**: Proposed; pending cross-team review
**Version**: 1.0
**Deciders**: Architecture Team
**Consulted**: Gateway, Signal Processing, AI Analysis, Kubernaut Agent, Remediation Orchestrator, Workflow Execution, Effectiveness Monitor, Data Storage, Security, Compliance, Platform, and SRE teams

**Related Business Requirements**:
- `BR-ORCH-025`: workflow data pass-through to child CRDs
- `BR-AUDIT-005`: complete remediation lifecycle reconstruction from audit traces
- `BR-KA-211`: LLM input sanitization
- `TR-REQ-012`: discovery requirement for multi-pillar extensibility; formal BR decomposition is pending product approval

**Related Design Decisions**:
- [DD-CONTRACT-003: Pillar Extension Envelope](DD-CONTRACT-003-pillar-extension-envelope.md)
- [DD-CONTRACT-004: ProviderData Compatibility](DD-CONTRACT-004-provider-data-compatibility.md)
- [DD-CONTRACT-005: Multi-Pillar Propagation Boundaries](DD-CONTRACT-005-multi-pillar-propagation-boundaries.md)
- [DD-CONTRACT-006: Pillar Safety and Audit Governance](DD-CONTRACT-006-pillar-safety-audit-governance.md)

---

## Context

Kubernaut's common remediation lifecycle is intended to support more than alert-driven remediation. The platform must be extensible to Threat Remediation, Supply Chain Security, Compliance Remediation, Cost Optimization, and future pillars without requiring broad refactoring of shared CRDs or unrelated services.

The current APIs already separate common signal fields from provider-specific content. `RemediationRequest.Spec.ProviderData` and `SignalProcessing.Spec.Signal.ProviderData` are JSON strings. The current Gateway and Remediation Orchestrator paths preserve that content, while AIAnalysis and AgentSession receive typed signal and enrichment projections. Existing `AgentSession` and `RemediationWorkflow` APIs provide a precedent for narrowly scoped `apiextensionsv1.JSON` fields with preserved unknown fields.

The current `ProviderData` string representation is an intentional compatibility boundary from issue #96. Existing Gateway, integration, and reconstruction tests require parseable JSON text without an extra base64 layer. A mechanical replacement with `json.RawMessage` or `apiextensionsv1.JSON` would change a persisted and audited contract.

The implementation-readiness spike established two facts:

- JSON and YAML round trips preserve the current `ProviderData` string contract.
- `apiextensionsv1.JSON` and `runtime.RawExtension` preserve unknown fields in synthetic envelopes for all four currently identified non-alert pillars.

The production propagation trace also found a gap: current SignalProcessing-to-AIAnalysis, AgentSession, and Rego policy inputs do not carry an explicit pillar dimension or pillar-specific context.

## Problem Statement

Pillar-specific evidence and decisions cannot be placed directly into universal fields because each pillar has a different schema, lifecycle, retention profile, safety model, and verification method. Conversely, making the entire remediation pipeline unstructured would weaken admission validation, typed service contracts, policy safety, API discoverability, and audit controls.

The platform needs one stable extension boundary that allows a new pillar to be added through pillar-owned code and schemas while preserving typed common contracts and explicit service ownership.

## Decision Drivers

1. Add new pillars without changing unrelated pillar schemas or refactoring every downstream CRD.
2. Preserve existing `ProviderData` and `OriginalPayload` serialization behavior.
3. Keep universal lifecycle, routing, identity, target, timing, and correlation fields strongly typed.
4. Validate pillar and schema version before interpreting pillar data.
5. Prevent raw or sensitive pillar evidence from crossing every service boundary.
6. Preserve complete audit reconstruction, redaction, retention, and fail-closed behavior.
7. Maintain the existing separation between investigation, approval, execution, and verification.

## Decision

Kubernaut will use a hybrid multi-pillar data-contract model:

- Existing universal fields remain strongly typed.
- `signalType` continues to describe normalized signal shape and remains compatible with the current generic `alert` value.
- A separate pillar discriminator identifies the Kubernaut capability profile. Initial values are `alert-remediation`, `threat-remediation`, `supply-chain-security`, `compliance-remediation`, and `cost-optimization`.
- An optional pillar envelope carries the pillar-local kind, schema version, and narrowly scoped preserved JSON data or a reference to an external pillar resource.
- Signal Processing owns source decoding, deterministic normalization, and validation of the pillar envelope.
- Downstream services receive typed or validated pillar projections only when they need them.
- Large, sensitive, asynchronous, or independently retained evidence uses a child resource or evidence-store reference instead of inline data.
- The entire CRD and remediation loop must not become unstructured.

### Proposed envelope

The proposed baseline wire shape is one optional `pillarData` extension object. The final field placement and name remain subject to review by the affected API owners.

```yaml
spec:
  signalType: alert
  signalSource: security-scanner
  targetType: kubernetes
  providerData: '{"sourceFindingId":"finding-123"}'
  pillarData:
    pillar: supply-chain-security
    kind: artifact-vulnerability
    schemaVersion: v1
    data:
      artifactDigest: sha256:...
      advisoryId: CVE-...
      reachability: reachable
```

`data` and an external reference are mutually exclusive. `providerData` remains source evidence and is not a second normalized pillar representation.

### Extensibility invariant

Adding a new pillar is considered successful only if the change is additive at the pillar boundary. A new pillar may add its schema, decoder, registry entry, typed investigation and verification projections, policy inputs, workflow catalog, audit/redaction mappings, and focused tests. It must not require:

- A new universal field for each pillar-specific attribute.
- Changes to unrelated pillar schemas or consumers.
- A broad `map[string]any` or fully unstructured CRD.
- Passing the original raw evidence to Workflow Execution by default.
- Replacing the investigation/approval/execution/verification boundaries.

### Service ownership

- Gateway preserves source identity, source payload, common routing fields, and the selected pillar when the source contract supplies it.
- RemediationRequest stores immutable source provenance and the initial pillar envelope or reference.
- Signal Processing validates and normalizes pillar data deterministically; it does not perform cross-domain investigation or LLM reasoning.
- AIAnalysis receives a typed or validated pillar-specific investigation context and produces the recommendation and workflow selection.
- Kubernaut Agent receives the curated investigation context, not unvalidated source evidence.
- Remediation Orchestrator applies pillar-aware policy and approval semantics.
- Workflow Execution receives the approved, governed action plan and execution parameters, not the original pillar evidence by default.
- Effectiveness Monitor receives a pillar-specific verification plan or evidence reference.
- Audit and notification services record or expose the minimum redacted information required for reconstruction and operations.

### Versioning and validation

- The decoder registry is keyed by pillar, pillar-local kind, and schema version.
- A missing, malformed, unsupported, or contradictory envelope fails closed or enters an explicit degraded/manual-review state.
- Unknown required schema versions are rejected; unknown optional fields may be preserved but are not trusted without validation.
- Policy expressions consume validated typed inputs, not arbitrary preserved JSON.
- Schema changes require versioned decoder behavior and migration tests.

## Alternatives Considered

### Alternative A: Fully typed discriminated union - Rejected

Each pillar would receive a dedicated typed field or union branch in every shared CRD.

**Advantages**:
- Strong compile-time contracts and CRD validation.
- Clear generated clients and `kubectl` behavior.

**Disadvantages**:
- Every new pillar changes shared CRDs and generated code.
- Unrelated services become coupled to every pillar.
- Large unions create long-term versioning and maintenance pressure.

This remains appropriate for universal fields and stable pillar projections, but not as the only extension mechanism.

### Alternative B: Fully unstructured CRDs - Rejected

Shared CRDs would carry arbitrary `data` maps and services would interpret them dynamically.

**Advantages**:
- New pillars can transport arbitrary data without immediate CRD changes.

**Disadvantages**:
- Weak admission validation and poor API discoverability.
- Runtime failures replace schema-time failures.
- Security review, policy evaluation, generated clients, and audit curation become harder.
- The implementation would spread untyped maps and duplicate parsing logic.

This is rejected as the default architecture.

### Alternative C: Typed common envelope with versioned pillar extension - Proposed

Universal fields remain typed while a single controlled extension carries versioned pillar data.

**Advantages**:
- Stable common contract.
- Additive pillar evolution.
- Explicit ownership and versioning.
- Preserved JSON can bridge independently evolving schemas.
- Typed downstream projections prevent raw-payload propagation.

**Disadvantages**:
- Requires decoder registration and runtime semantic validation.
- Preserved JSON cannot be used directly in CEL policy expressions.
- Each pillar requires fixtures, schema tests, redaction rules, and migration policy.

This is the proposed decision.

### Alternative D: Separate pillar resource only - Rejected as the default; retained as a complement

Each pillar would use a separate resource such as `ThreatContext`, `ComplianceContext`, `SupplyChainContext`, or `CostContext`.

**Advantages**:
- Independent schema, lifecycle, retention, and access control.
- Suitable for large, sensitive, asynchronous, or independently retained evidence.

**Disadvantages**:
- More objects, watches, reads, references, and failure modes.
- Audit reconstruction and garbage collection require additional design.
- Heavier than an inline extension for small initial contexts.

Use this option together with Alternative C when the evidence characteristics require it.

## Consequences

### Positive Consequences

1. New pillars can be added without expanding every shared CRD with pillar-specific fields.
2. Existing alert behavior and `ProviderData` serialization remain compatible during the first implementation.
3. Investigation, policy, execution, verification, and audit services receive only the data they own.
4. Schema version and pillar ownership become explicit and auditable.
5. Supply Chain Security and Compliance Remediation use the same platform extension model as Threat Remediation and Cost Optimization.

### Negative Consequences

1. Runtime decoder registries and semantic validation are required.
   - **Mitigation**: require pillar/kind/version registration, fail-closed unknown versions, and focused contract tests.
2. Some pillar data will require duplicated typed projections at service boundaries.
   - **Mitigation**: define ownership and projection schemas once per service and avoid copying raw evidence.
3. The current `ProviderData` field remains a legacy source-evidence contract.
   - **Mitigation**: add the new envelope without migration; handle any future deprecation in a separate reviewed decision.

### Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| The extension becomes a de facto unstructured payload bus | Medium | High | One owning decoder, typed projections, no raw propagation by default |
| A pillar-specific secret reaches logs, LLM input, or notifications | Medium | High | Source-boundary redaction, sensitivity classification, audit tests, least-retention storage |
| Unsupported schema versions are interpreted incorrectly | Medium | High | Version-keyed decoder registry and fail-closed handling |
| A new pillar requires unexpected changes to unrelated services | Medium | Medium | Extensibility invariant and additive contract review gate |
| Audit reconstruction omits pillar context | Medium | High | Correlation requirements and explicit pillar/schema/decoder audit fields or references |

## Compliance

This ADR is proposed and does not replace formal BR approval or control assessment.

| Requirement | Status | Notes |
|-------------|--------|-------|
| `BR-AUDIT-005` | Proposed | Correlation, pillar identity, schema version, and decoder outcome must be reconstructable. |
| FedRAMP AU-2/AU-3/AU-9/AU-11 | Proposed | Pillar lifecycle events require structured content, protection, and retention mapping. |
| FedRAMP AC-4/AC-6 | Proposed | Typed projections and policy inputs constrain information flow and privilege. |
| FedRAMP SI-10 | Proposed | Pillar data is validated and sanitized before interpretation or LLM use. |
| SOC 2 CC7.2 | Proposed | Correlated lifecycle records support monitoring and incident reconstruction. |
| SOC 2 CC8.1 | Proposed | Cross-team review, approval, implementation, and testing follow change-management requirements. |

## Validation Strategy

1. Add an envtest or API-server schema test for the selected envelope and generated CRDs.
2. Verify JSON/YAML round trips, unknown-field preservation, size limits, and current `ProviderData` compatibility.
3. Add decoder tests for every initial pillar and schema version, including malformed and unsupported versions.
4. Trace one representative request through Gateway, RR, SP, AIAnalysis, AgentSession, RO, WFE, EM, audit, and reconstruction.
5. Prove that adding a new synthetic pillar does not modify unrelated pillar projections or consumers.
6. Add redaction, policy-input, audit reconstruction, degraded-mode, and manual-review tests.
7. Complete formal BR, wiring manifest, threat model, and test-plan mapping before implementation.

## References

- [Multi-Pillar Data Contract Spike](../../spikes/multi-pillar-data-contract/README.md)
- [Threat Remediation Product Discovery](../../requirements/enhancements/THREAT_REMEDIATION_PRODUCT_DISCOVERY.md)
- [ADR-001: CRD-Based Microservices Architecture](ADR-001-crd-microservices-architecture.md)
- [ADR-016: Validation Responsibility Chain](ADR-016-validation-responsibility-chain.md)
- [ADR-034: Unified Audit Table Design](ADR-034-unified-audit-table-design.md)
- [DD-CONTRACT-002: Service Integration Contracts](DD-CONTRACT-002-service-integration-contracts.md)
- [DD-GATEWAY-010: Adapter Naming Convention](DD-GATEWAY-010-adapter-naming-convention.md)
- [DD-AUDIT-003: Service Audit Trace Requirements](DD-AUDIT-003-service-audit-trace-requirements.md)
- [DD-ERROR-001: Error Details Standardization](DD-ERROR-001-error-details-standardization.md)

---

**Document Version**: 1.0
**Last Updated**: 2026-09-14
