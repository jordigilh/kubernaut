# DD-CONTRACT-004: ProviderData Compatibility Boundary

**Status**: Proposed; pending cross-team review
**Decision Date**: 2026-09-14
**Version**: 1.0
**Confidence**: 97%
**Deciders**: Architecture Team
**Applies To**: Gateway, RemediationRequest, Signal Processing, Data Storage reconstruction, and audit consumers

**Related Business Requirements**:
- `BR-ORCH-025`: workflow data pass-through to child CRDs
- `BR-AUDIT-005`: remediation request reconstruction
- `TR-REQ-012`: additive multi-pillar extensibility; formal BR pending

**Related Design Decisions**:
- [ADR-075: Multi-Pillar Data Contract Extensibility](ADR-075-multi-pillar-data-contract-extensibility.md)
- [DD-CONTRACT-003: Pillar Extension Envelope](DD-CONTRACT-003-pillar-extension-envelope.md)
- [DD-AUDIT-003: Service Audit Trace Requirements](DD-AUDIT-003-service-audit-trace-requirements.md)

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-09-14 | Jordi Gil | Initial proposed compatibility boundary |

---

## Context & Problem

### Current State

`RemediationRequest.Spec.ProviderData` and `SignalProcessing.Spec.Signal.ProviderData` are Go `string` fields containing JSON text. `OriginalPayload` is also stored as a string. Gateway writes these values, Remediation Orchestrator copies `ProviderData` into SignalProcessing, and Data Storage reconstruction maps provider response summaries back into `ProviderData`.

Issue #96 established the string representation to avoid an unwanted base64 layer when JSON is stored in the CRD. Existing Gateway BDD tests and Data Storage integration tests require the value to remain parseable JSON text.

The architecture documentation still describes some provider data as `json.RawMessage`, creating documentation drift. That drift must be resolved, but it is not evidence that a persisted field can be changed mechanically.

### Problem Statement

Using `ProviderData` for the new multi-pillar envelope would conflate source evidence, normalized pillar context, versioning, and downstream projections. Changing its type would also create a migration and audit-reconstruction problem before the first new pillar is delivered.

### Constraints

- Existing serialized objects and audit reconstruction must remain readable.
- Gateway must continue to produce JSON text without base64 encoding.
- The first pillar implementation must not require rewriting existing alert payloads.
- Source evidence and normalized pillar context must have separate ownership semantics.
- Future deprecation of `ProviderData` requires its own reviewed migration decision.

---

## Decision Drivers

1. Backward compatibility for persisted CRDs and audit records.
2. No migration requirement for the first pillar.
3. Clear distinction between raw source evidence and normalized pillar context.
4. Avoiding duplicate or conflicting normalized representations.
5. Preserving existing tests and reconstruction behavior.

---

## Alternatives Considered

### Alternative A: Change `ProviderData` to `json.RawMessage` - Rejected

Replace the string field in the existing APIs and regenerate CRDs.

**Pros**:
- More idiomatic Go representation for JSON.
- Could simplify some internal decoding.

**Cons**:
- Changes the CRD wire and serialization contract.
- May reintroduce base64 encoding behavior in Kubernetes serialization paths.
- Requires migration, reconstruction, and compatibility testing across existing data.
- Does not distinguish provider evidence from pillar context.

**Confidence**: 99% (rejected for the first pillar)

### Alternative B: Encode the pillar envelope inside `ProviderData` - Rejected

Store a structure containing `pillar`, `kind`, `schemaVersion`, and `data` as the value of the existing string field.

**Pros**:
- No new CRD field.
- Existing pass-through path would carry the bytes.

**Cons**:
- Changes the meaning of an existing field and forces every consumer to understand a new envelope.
- Makes source evidence and normalized context ambiguous.
- Creates hidden coupling in audit reconstruction and downstream parsers.
- Does not provide an additive extension boundary for future pillars.

**Confidence**: 98% (rejected)

### Alternative C: Preserve `ProviderData` and add an optional pillar envelope - CHOSEN

Keep `ProviderData` as legacy/source evidence and add one optional `PillarData` extension with explicit pillar, kind, schema version, and preserved data or reference.

**Pros**:
- Existing objects remain compatible.
- New pillar behavior is additive and explicit.
- Source evidence and normalized context have separate ownership.
- Future migration can be designed independently.

**Cons**:
- The first implementation temporarily carries two related fields.
- Consumers must not treat both fields as competing normalized payloads.

**Confidence**: 97% (proposed)

---

## Decision

### Chosen: Alternative C - Preserve `ProviderData` and add an optional pillar envelope

The first multi-pillar implementation shall:

- Leave `ProviderData string` unchanged in `RemediationRequest` and `SignalProcessing`.
- Leave `OriginalPayload string` unchanged.
- Add an optional, explicit pillar envelope as a separate field when the API design is approved.
- Treat `ProviderData` as source/provider evidence and the new envelope as normalized pillar context.
- Never silently prefer one representation over the other based on which consumer happens to read it.
- Avoid copying the raw `ProviderData` into AIAnalysis, AgentSession, WorkflowExecution, or notification unless a documented audit or investigation requirement permits it.

### Compatibility Model

The two fields have different authority:

- `ProviderData`: authoritative source/provider payload preserved for compatibility and reconstruction.
- `PillarData`: authoritative normalized pillar envelope after Signal Processing validation.
- Typed projections: authoritative service-specific representations derived from validated `PillarData`.

The first implementation may populate `PillarData` from source input or from deterministic Signal Processing decoding, but the origin and decoder result must be auditable.

### Migration Policy

No `ProviderData` migration is part of the first pillar. A future migration must separately define:

1. The persisted data versions and conversion rules.
2. Read compatibility for historical audit and CRD records.
3. Write behavior during rolling upgrades.
4. Rollback behavior.
5. CRD conversion/webhook requirements, if any.
6. Test coverage for existing and new payloads.

---

## Consequences

### Positive Consequences

1. Existing alert ingestion and reconstruction remain stable.
2. The first new pillar can be delivered without rewriting existing CRDs.
3. Raw source evidence, normalized context, and typed service projections have distinct authority.
4. Any future `ProviderData` deprecation can be evaluated on its own evidence.

### Negative Consequences

1. Two fields exist during the transition.
   - **Mitigation**: document authority and prohibit duplicate normalized representations.
2. Source payloads may be stored in a less convenient string representation.
   - **Mitigation**: decode only at explicit boundaries and use typed projections downstream.

### Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| A consumer treats raw ProviderData as trusted normalized context | Medium | High | Ownership documentation, decoder boundary, security tests |
| A later migration breaks historical reconstruction | Medium | High | Separate migration DD, dual-read tests, replay fixtures |
| ProviderData and PillarData diverge semantically | Medium | Medium | Record decoder result and source relationship in audit data |

---

## Compliance

| Requirement | Status | Notes |
|-------------|--------|-------|
| `BR-AUDIT-005` | Proposed | Preserve historical source evidence and reconstruction compatibility. |
| FedRAMP AU-3/AU-9/AU-11 | Proposed | Keep structured provenance and retention semantics stable. |
| FedRAMP SI-10 | Proposed | Do not treat raw ProviderData as validated input. |
| SOC 2 CC7.2 | Proposed | Historical lifecycle reconstruction remains queryable by correlation ID. |

---

## Validation Strategy

1. Test JSON and YAML round trips for current `ProviderData` and `OriginalPayload` values.
2. Test that current ProviderData remains JSON text and is not base64 encoded.
3. Test reconstruction of historical audit records containing ProviderData.
4. Test additive serialization of the pillar envelope without modifying ProviderData.
5. Add a migration test plan before any future type change or deprecation.

---

## References

- [ADR-075: Multi-Pillar Data Contract Extensibility](ADR-075-multi-pillar-data-contract-extensibility.md)
- [DD-CONTRACT-003: Pillar Extension Envelope](DD-CONTRACT-003-pillar-extension-envelope.md)
- [Multi-Pillar Data Contract Spike](../../spikes/multi-pillar-data-contract/README.md)
- [Issue #96 ProviderData compatibility tests](../../../pkg/gateway/processing/crd_creation_business_test.go)
- [Data Storage reconstruction parser](../../../pkg/datastorage/reconstruction/parser.go)
- [Data Storage reconstruction mapper](../../../pkg/datastorage/reconstruction/mapper.go)

---

**Document Version**: 1.0
**Last Updated**: 2026-09-14
