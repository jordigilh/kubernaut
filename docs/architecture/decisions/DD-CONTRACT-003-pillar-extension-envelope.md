# DD-CONTRACT-003: Pillar Extension Envelope

**Status**: Proposed; pending cross-team review
**Decision Date**: 2026-09-14
**Version**: 1.0
**Confidence**: 95%
**Deciders**: Architecture Team
**Applies To**: Gateway, RemediationRequest, Signal Processing, AI Analysis, Kubernaut Agent, Remediation Orchestrator, Workflow Execution, Effectiveness Monitor, and future pillar services

**Related Business Requirements**:
- `BR-ORCH-025`: workflow data pass-through to child CRDs
- `BR-AUDIT-005`: complete remediation lifecycle reconstruction
- `TR-REQ-012`: multi-pillar extensibility discovery requirement; formal BR pending

**Related Design Decisions**:
- [ADR-075: Multi-Pillar Data Contract Extensibility](ADR-075-multi-pillar-data-contract-extensibility.md)
- [DD-CONTRACT-002: Service Integration Contracts](DD-CONTRACT-002-service-integration-contracts.md)
- [DD-GATEWAY-010: Adapter Naming Convention](DD-GATEWAY-010-adapter-naming-convention.md)

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-09-14 | Jordi Gil | Initial proposed pillar extension envelope |

---

## Context & Problem

### Current State

Kubernaut currently carries common signal fields and provider-specific source content through typed CRDs. `signalType` is normalized to the generic `alert` value, while `signalSource` identifies the adapter or external system. `ProviderData` is a JSON string and remains a compatibility boundary.

The platform is expected to support Alert Remediation, Threat Remediation, Supply Chain Security, Compliance Remediation, Cost Optimization, and future capabilities. Their domain data is not interchangeable:

- Threat Remediation needs findings, principals, attack stage, indicators, blast radius, containment constraints, and forensic requirements.
- Supply Chain Security needs SBOM and advisory identity, exploitability, reachability, affected artifact and workload scope, patch or backport state, and residual exposure.
- Compliance Remediation needs framework and control identifiers, violations, evidence, exceptions, compensating controls, deadlines, and evidence validity.
- Cost Optimization needs utilization, allocation, cost attribution, budgets, forecasts, rightsizing, and savings verification.

Existing `AgentSession` and `RemediationWorkflow` APIs use narrowly scoped preserved JSON fields for schemas that evolve independently from their CRDs.

### Problem Statement

There is no explicit common contract for pillar identity, pillar-local kind, schema version, and pillar-specific data. Adding pillar-specific fields directly to every shared CRD would couple unrelated services. Making entire CRDs unstructured would weaken validation, discoverability, policy safety, and typed service contracts.

### Constraints

- `signalType` must retain its existing signal-shape semantics.
- Existing `ProviderData` and `OriginalPayload` serialization must not change in the first implementation.
- Unknown optional fields may be preserved, but unknown required schema versions must not be interpreted silently.
- Raw pillar evidence must not be propagated to every downstream service.
- A new pillar must be additive at its boundary and must not require broad refactoring.

---

## Decision Drivers

1. Stable universal fields and generated clients.
2. Additive support for new pillars and pillar-local schema evolution.
3. Explicit validation and ownership boundaries.
4. Safe handling of untrusted and sensitive evidence.
5. Compatibility with current CRD and audit behavior.

---

## Alternatives Considered

### Alternative A: Per-pillar typed union in shared CRDs - Rejected

Add a dedicated typed branch or field for every pillar to each shared CRD.

**Pros**:
- Strong compile-time and admission validation.
- Excellent API discoverability.

**Cons**:
- Every new pillar changes shared CRDs and generated clients.
- Unrelated services become coupled to every pillar.
- Large unions create long-term migration and maintenance pressure.

**Confidence**: 95% (rejected as the only mechanism)

### Alternative B: Fully unstructured shared CRDs - Rejected

Use arbitrary maps for signal and remediation content and interpret them in each service.

**Pros**:
- New pillars can transport arbitrary fields immediately.

**Cons**:
- Runtime failures replace schema-time failures.
- Policy and security review become harder.
- Typed contracts, generated clients, and API discoverability lose value.
- Parsing and validation logic gets duplicated across services.

**Confidence**: 99% (rejected as the default)

### Alternative C: One typed common envelope with a versioned pillar extension - CHOSEN

Keep universal fields typed and add one optional, narrowly scoped extension object:

```yaml
pillarData:
  pillar: supply-chain-security
  kind: artifact-vulnerability
  schemaVersion: v1
  data:
    artifactDigest: sha256:...
    advisoryId: CVE-...
```

**Pros**:
- New pillars evolve without one shared field per pillar attribute.
- Pillar ownership, kind, and schema version are explicit.
- Preserved JSON bridges independently evolving schemas.
- Downstream services can use typed projections.

**Cons**:
- Requires decoder registration and runtime semantic validation.
- Preserved JSON is not directly suitable as a CEL policy input.
- Each pillar needs fixtures, redaction rules, and migration tests.

**Confidence**: 95% (proposed)

---

## Decision

### Chosen: Alternative C - One typed common envelope with a versioned pillar extension

The proposed envelope contains:

- `pillar`: capability profile, not a replacement for `signalType` or `signalSource`.
- `kind`: pillar-local signal or evidence shape, such as `artifact-vulnerability`, `security-finding`, `compliance-violation`, or `cost-observation`.
- `schemaVersion`: version selected by the owning decoder.
- `data`: optional preserved JSON for small, inline pillar context.
- `reference`: optional child-resource or evidence-store reference for large, sensitive, asynchronous, or independently retained data.

`data` and `reference` are mutually exclusive. The final top-level field name and exact placement are subject to API-owner review; this DD uses `pillarData` as the baseline proposal.

### Architecture

```text
Gateway
  -> RemediationRequest
       common typed fields + legacy ProviderData + optional PillarData
  -> SignalProcessing
       decode, validate, normalize, enrich
  -> AIAnalysis / AgentSession
       typed pillar investigation projection
  -> Remediation Orchestrator
       pillar-aware policy and approval
  -> WorkflowExecution
       governed action plan only
  -> Effectiveness Monitor
       pillar-specific verification plan or evidence reference
```

### Implementation Details

The conceptual shape is:

```go
type PillarData struct {
    Pillar        string                `json:"pillar"`
    Kind          string                `json:"kind"`
    SchemaVersion string                `json:"schemaVersion"`
    Data          *apiextensionsv1.JSON `json:"data,omitempty"`
    Reference     *PillarDataReference  `json:"reference,omitempty"`
}
```

The implementation must validate in this order:

1. Envelope presence and structural validity.
2. Pillar and pillar-local kind vocabulary.
3. Schema-version support in the decoder registry.
4. Inline versus reference exclusivity and size limits.
5. Pillar-specific semantic validation.
6. Redaction and sensitivity classification before downstream propagation.

The decoder registry is keyed by `(pillar, kind, schemaVersion)`. A new pillar adds a registry entry and pillar-owned validation/projection code; it does not add a new branch to every shared CRD consumer.

---

## Consequences

### Positive Consequences

1. New pillars can be introduced without broad shared-CRD refactoring.
2. Universal lifecycle and routing fields remain typed and discoverable.
3. Pillar schemas can evolve independently behind explicit versions.
4. Large or sensitive evidence can have an independent lifecycle.

### Negative Consequences

1. Runtime decoder and semantic validation infrastructure is required.
   - **Mitigation**: registry completeness tests, fail-closed version handling, and per-pillar fixtures.
2. API owners must maintain typed projections in services that consume pillar context.
   - **Mitigation**: define ownership and projection contracts before implementation.
3. Preserved JSON cannot be used directly by policy expressions.
   - **Mitigation**: convert only validated fields into typed policy inputs.

### Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| `pillarData` becomes an unstructured payload bus | Medium | High | Single decoder owner, typed projections, size limits |
| Unsupported versions are interpreted as current | Low | High | Registry lookup and fail-closed behavior |
| Sensitive evidence crosses an unnecessary boundary | Medium | High | Reference-based storage, redaction, and access controls |

---

## Compliance

| Requirement | Status | Notes |
|-------------|--------|-------|
| `BR-AUDIT-005` | Proposed | Pillar, kind, schema version, and correlation must be reconstructable. |
| FedRAMP SI-10 | Proposed | Structural and semantic validation precede interpretation. |
| FedRAMP AC-4/AC-6 | Proposed | Typed projections limit information flow and privilege. |
| SOC 2 CC7.2 | Proposed | Correlated pillar lifecycle events support reconstruction. |

---

## Validation Strategy

1. Generate the selected CRD schema and validate the envelope with envtest or a Kubernetes API server.
2. Test JSON/YAML round trips and unknown-field preservation for every initial pillar.
3. Test malformed envelopes, missing fields, unsupported versions, duplicate data/reference, and size limits.
4. Assert decoder registry completeness for all supported `(pillar, kind, schemaVersion)` tuples.
5. Add contract tests proving a synthetic new pillar does not modify unrelated projections.

---

## References

- [ADR-075: Multi-Pillar Data Contract Extensibility](ADR-075-multi-pillar-data-contract-extensibility.md)
- [Multi-Pillar Data Contract Spike](../../spikes/multi-pillar-data-contract/README.md)
- [Threat Remediation Product Discovery](../../requirements/enhancements/THREAT_REMEDIATION_PRODUCT_DISCOVERY.md)
- [DD-CONTRACT-002: Service Integration Contracts](DD-CONTRACT-002-service-integration-contracts.md)
- [AgentSession API types](../../../api/agentsession/v1alpha1/agentsession_types.go)

---

**Document Version**: 1.0
**Last Updated**: 2026-09-14
