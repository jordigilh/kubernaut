# DD-AF-017: Standalone RAR Parameter Override Contract

**Status**: Proposed — design-only, pending review and merge
**Decision Date**: 2026-10-29
**Version**: 1.0
**Confidence**: 90%
**Deciders**: Project owner; API Frontend, Auth Webhook and RemediationOrchestrator maintainers
**Applies To**: API Frontend, Auth Webhook, RemediationApprovalRequest, RemediationOrchestrator, WorkflowExecution

**Related Business Requirements**:

- BR-ORCH-025/026: workflow data pass-through and approval orchestration
- BR-AUDIT-005: correlated audit reconstruction
- BR-AUDIT-006: approval decision audit trail
- BR-KA-191: workflow parameter validation
- BR-AUTH-001: authenticated operator attribution

**Related Design Decisions**:

- ADR-076: RAR Approval Snapshot and Terminal Decision Contract
- ADR-040: RemediationApprovalRequest CRD Architecture
- ADR-022: API Frontend ServiceAccount security model
- DD-AF-006: Approval Consent Guard
- DD-WEBHOOK-003: Webhook-Complete Audit Pattern
- DD-WORKFLOW-018: etcd/CRD workflow snapshot authority

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-10-29 | Kubernaut Architecture | Initial design for #2570 |

---

## Context & Problem

### Current State

The standalone AF approval path exposes `kubernaut_approve`, but its
`workflow_override` input is currently a workflow-name string. The existing
#594 core already supports a string-valued parameter map in RAR status and RO
consumption, but AF does not expose a structured parameter override contract.

The existing approval read also does not expose the complete original selection
snapshot required for a safe independent review.

### Problem Statement

An authorized reviewer must be able to approve the exact RAR proposal, with an
optional complete parameter replacement, without starting an investigation or
using `kubernaut_select_workflow`. AF must provide the input shape and UX
preflight, while AuthWebhook remains the authoritative CRD admission boundary.

### Constraints

- Current #2570 scope is parameter-only; workflow selection is deferred to #2572.
- Backward compatibility with the current AF input shape is not required.
- Parameter values remain `map[string]string` to match AIA/RAR/WE execution
  contracts.
- RAR is the approval-time source of truth.
- AF OAuth/SAR and human audit remain the authorization boundary.
- AuthWebhook trusts AF delegation and enforces RAR mutation rules.
- `TARGET_RESOURCE_*` parameters remain locked by a runtime/system rule even
  while catalog parameters default to editable until #2571 is implemented.

---

## Decision Drivers

1. No browser-only validation for security or safety decisions.
2. Exact original-versus-override history.
3. Generic parameter names without workflow-specific AF code.
4. Strict terminal decision immutability.
5. No new MCP endpoint or interactive-session dependency.

---

## Alternatives Considered

### Alternative A: Structured object on existing `kubernaut_approve` ✅ CHOSEN

**Approach**: Replace the current string-valued workflow override input with a
structured parameter-only object.

```json
{
  "rar_name": "rar-example",
  "decision": "Approved",
  "reason": "Reviewed the proposed parameter change",
  "workflow_override": {
    "rar_uid": "<RAR UID>",
    "selection_revision": "<selection revision>",
    "selection_digest": "<canonical digest>",
    "parameters": {
      "MEMORY_LIMIT": "768Mi",
      "REPLICAS": "3"
    },
    "rationale": "Increase memory for the observed workload"
  }
}
```

**Pros**:

- One coherent approval contract.
- Generic parameter names.
- No new MCP endpoint.
- Explicit nil-versus-empty semantics.

**Cons**:

- Existing callers must migrate to the new shape.

**Confidence**: 95% (chosen)

### Alternative B: New `kubernaut_approve_with_override` endpoint ❌ REJECTED

**Approach**: Keep ordinary approval unchanged and add a second MCP tool.

**Pros**:

- Separate permission surface.
- No ambiguity in tool schema.

**Cons**:

- Duplicates approval lifecycle behavior.
- Adds a new MCP surface without current need.
- Splits audit/readback and error semantics.

**Confidence**: 90% (rejected)

### Alternative C: Flat parallel input fields ❌ REJECTED

**Approach**: Add `parameter_overrides` and `override_rationale` alongside
the current string field.

**Pros**:

- Small local schema change.

**Cons**:

- Ambiguous precedence between fields.
- Harder to extend for review revision and future structured metadata.
- Preserves a contract that incorrectly implies workflow selection is in scope.

**Confidence**: 90% (rejected)

---

## Decision

### AF input contract

`kubernaut_approve` accepts one structured parameter-only override object. The
object contains:

- `parameters: map[string]string` — complete replacement map
- `rationale: string` — human explanation
- `rar_uid: string` — immutable RAR instance identity
- `selection_revision: string` — immutable revision of the RAR snapshot
- `selection_digest: string` — canonical digest of the reviewed snapshot

The `workflow_override` object is valid only when `decision` is `Approved`.
An ordinary approval or rejection may omit it and is not gated on optional
parameter-edit metadata. When `workflow_override` is present on an `Approved`
decision—including a rationale-only override that preserves the original
parameter map—all three review bindings are required. AuthWebhook rejects
`workflow_override` on a `Rejected` decision.

The bindings map to the RAR source of truth as follows:

- `rar_uid` is compared with the RAR object's `metadata.uid`.
- `selection_revision` is compared with the RAR spec's
  `originalSelection.selectionRevision`.
- `selection_digest` is compared with the RAR spec's
  `originalSelection.selectionDigest`.

These are the canonical AF wire names; the RAR spec uses the camelCase field
names shown above.

The current AF contract does not expose a workflow-selection field. A workflow
selection submitted through this parameter-only path is rejected.

AuthWebhook compares `rar_uid`, `selection_revision` and `selection_digest` to
the current pending RAR before admitting the decision. A mismatch is a stale
review and produces no partial status mutation. Kubernetes `resourceVersion`
remains an API concurrency guard, not the business review revision.

### Effective parameter semantics

- Omitted override or nil parameters: preserve the original RAR spec map.
- For a non-nil replacement map, AuthWebhook requires every locked
  `TARGET_RESOURCE_*` key from the original RAR snapshot to be present with
  its original value. Missing or changed locked keys are rejected.
- After locked-key validation, replace the entire original map.
- Reject an explicit empty map when the original snapshot contains a locked
  `TARGET_RESOURCE_*` key; otherwise execute with no parameters.
- The wire representation must preserve omitted versus explicit-empty maps;
  an explicit `{}` must reach the status patch as an empty replacement map.
- AF may validate shape and required input presence for user feedback.
- AuthWebhook validates the admitted business result and locks the final RAR.

### Validation and authorization boundary

1. AF authenticates the OAuth user, performs the existing SAR/tool permission
   check, and emits the human approval audit event.
2. AF submits the RAR status mutation through its ServiceAccount with the
   human identity for delegation.
3. AuthWebhook authenticates the Kubernetes caller, recognizes AF as the
   trusted intermediary, preserves `DecidedBy`, and sets `DecidedVia`.
4. AuthWebhook validates parameter keys, schema constraints, target-parameter
   protection, decision eligibility and terminal immutability.
5. RO consumes the admitted RAR snapshot and creates WE; it does not perform
   user selection or infer approval intent.

### Audit and readback

Extend existing typed approval payload types. Do not create a new override
event taxonomy. AF records approval intent; AuthWebhook records the admitted
business fields and delegation attribution; RO records the persisted committed
decision and business-context/orchestration events. The final field set is
selected from the SOC2/FedRAMP control mapping and must support original,
override and effective parameter evidence, actor/rationale attribution, review
revision and sensitive-value handling.

---

## Consequences

### Positive Consequences

1. AF exposes a clean, workflow-generic contract.
2. Existing #594 RO semantics remain reusable.
3. Workflow selection is not accidentally reintroduced into the parameter-only
   feature.
4. AuthWebhook remains the server-side safety boundary.

### Negative Consequences

1. Existing AF callers must migrate to the structured input.
   - **Mitigation**: compatibility is explicitly out of scope; update callers
     and tests in the implementation PR.
2. The RAR/AIA snapshot contract grows.
   - **Mitigation**: use a shared type and generated serialization tests.
3. Explicit editability metadata is not available until #2571.
   - **Mitigation**: default catalog parameters to editable; keep
     `TARGET_RESOURCE_*` locked by the runtime rule.

### Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| AF and AuthWebhook disagree on input semantics | Medium | High | AuthWebhook is authoritative; add admission serialization tests. |
| A stale terminal status update changes the override | Medium | High | Old/new object comparison and frozen business-field allowlist. |
| Sensitive parameters enter audit payloads | Medium | High | Control-mapped redaction/reference policy. |

---

## Compliance

| Requirement | Status | Notes |
|-------------|--------|-------|
| BR-ORCH-025/026 | 📋 Planned | RAR snapshot reaches RO/WE without live selection. |
| BR-AUDIT-005/006 | 📋 Planned | Extended typed audit/readback preserves decision provenance. |
| BR-KA-191 | 📋 Planned | AuthWebhook reuses workflow parameter constraints. |
| AC-6 / AU-2 / AU-3 / SI-10 | 📋 Planned | AF SAR, AuthWebhook attribution/validation and typed audit payloads. |

---

## Validation Strategy

1. **UT-AF-2570-001 / BR-ORCH-025**: JSON/schema tests for structured input,
   review bindings, omitted parameters and empty maps.
2. **UT-AW-2570-002 / BR-AUTH-001, SI-10**: AuthWebhook tests for invalid keys,
   schema violations, target locks and post-terminal business-field mutation.
3. **IT-RO-2570-005 / BR-ORCH-025/026**: AF-to-AuthWebhook-to-RO-to-WE test
   proving the complete map.
4. **E2E-AUD-2570-006 / BR-AUDIT-005/006, CC7.2, AU-3**: audit/readback tests
   after the control-objective field mapping is approved.
5. No production implementation begins until this DD and ADR-076 are merged.

---

## References

- ADR-076: RAR Approval Snapshot and Terminal Decision Contract
- ADR-040: RemediationApprovalRequest CRD Architecture
- ADR-022: API Frontend ServiceAccount security model
- DD-AF-006: Approval Consent Guard
- DD-WEBHOOK-003: Webhook-Complete Audit Pattern
- DD-WORKFLOW-018: etcd/CRD workflow snapshot authority
- Issues #2570, #2564, #2565, #2571 and deferred #2572

---

**Document Version**: 1.0
**Last Updated**: 2026-10-29
