# ADR-076: RAR Approval Snapshot and Terminal Decision Contract

**Status**: 📋 Proposed — design-only, pending document review and merge
**Date**: 2026-10-29
**Version**: 1.0
**Confidence**: 90%
**Deciders**: Project owner; API Frontend, Auth Webhook, AIAnalysis and RemediationOrchestrator maintainers

## Context

Issue #2570 extends the standalone `RemediationApprovalRequest` (RAR) approval
flow with parameter overrides. The existing #594 implementation proves that
Remediation Orchestrator can consume a `WorkflowOverride`, but it does not yet
provide a complete approval-scoped selection snapshot for an independent
reviewer.

The approval path must preserve three properties:

1. The proposal reviewed by the operator is the proposal executed by
   WorkflowExecution.
2. A terminal approval decision cannot be changed through a later status update.
3. The original selection, the complete override, and the effective execution
   set remain attributable for audit and reconstruction.

The current scope is **parameter-only**. Selecting a different workflow is
deferred to #2572 and is not part of #2570, #2564 or #2565 unless those issues
explicitly expand their scope.

## Related Business Requirements

- BR-ORCH-025/026: workflow data pass-through and approval orchestration
- BR-AUDIT-005: correlated audit reconstruction
- BR-AUDIT-006: RemediationApprovalRequest decision audit trail
- BR-KA-191: workflow parameter validation
- BR-AUTH-001: authenticated operator attribution

## Related Decisions

- ADR-034: Unified Audit Table Design
- ADR-040: RemediationApprovalRequest CRD Architecture
- ADR-022: API Frontend ServiceAccount security model
- DD-AF-006: Approval Consent Guard
- DD-WEBHOOK-003: Webhook-Complete Audit Pattern
- DD-WORKFLOW-018: etcd/CRD workflow snapshot authority
- DD-AF-017: Standalone RAR Parameter Override Contract

---

## Decision Drivers

1. RAR must be the approval-time source of truth.
2. AIA and RAR must use the same typed selection snapshot shape.
3. RO must not reconstruct approval intent from live AIA or catalog state.
4. Business decision fields must be terminal and immutable.
5. Existing human-consent, delegation and audit boundaries must remain intact.
6. Parameter names must remain workflow-generic while preserving the current
   string-valued execution contract.

---

## Alternatives Considered

### Alternative A: RAR-scoped immutable selection snapshot ✅ CHOSEN

**Approach**: AIAnalysis produces a shared selection snapshot. When RO creates
the RAR, it copies that snapshot into the immutable RAR spec. The RAR status
stores the terminal decision and complete parameter replacement override.

**Pros**:

- RAR can be reviewed independently of an investigation session.
- RO consumes one approval-scoped source of truth.
- Original parameters and selection revision cannot drift after request creation.
- Audit reconstruction can distinguish original, override and effective values.

**Cons**:

- The RAR CRD schema stores a second copy of selection data.
- Sensitive-value storage and redaction rules must be explicit.

**Confidence**: 95% (chosen)

### Alternative B: RO reads AIA and live catalog after approval ❌ REJECTED

**Approach**: Keep the current RAR summary and let RO read AIA or the workflow
catalog when it handles an Approved RAR.

**Pros**:

- Smaller RAR schema change.
- Less data duplicated in the approval request.

**Cons**:

- The executed proposal can differ from what the reviewer saw.
- AIA or catalog retention/availability becomes an approval dependency.
- Live catalog data can replace recorded selection data.
- It violates the RAR source-of-truth requirement.

**Confidence**: 95% (rejected)

### Alternative C: Reconstruct the proposal from audit events ❌ REJECTED

**Approach**: AF or RO reconstructs the original selection from DataStorage audit
events at approval time.

**Pros**:

- Durable historical source when CRDs are deleted.

**Cons**:

- Current approval audit projections do not contain the complete selection
  packet and parameter metadata needed by the editor.
- Introduces paging, redaction and unavailable-data failure modes into the
  approval write path.
- Makes a successful approval depend on audit-read availability.

**Confidence**: 90% (rejected)

---

## Decision

### 1. Shared AIA/RAR selection snapshot

AIAnalysis and RAR use the same logical selection snapshot contract:

```yaml
workflowId: oom-recovery
workflowName: oom-recovery-v1
workflowVersion: 1.2.0
workflowRevision: sha256:...
executionBundle: registry.example/oom-recovery@sha256:...
parameters:
  MEMORY_LIMIT: "512Mi"
  REPLICAS: "3"
selectionRevision: rar-selection-v1
selectionDigest: sha256:...
```

The canonical typed execution portion of this snapshot is the existing
`sharedtypes.WorkflowSnapshot` from `pkg/shared/types/workflow_snapshot.go`.
It carries the catalog-authoritative workflow identity and execution fields:
`WorkflowID`, `WorkflowName`, `ActionType`, `Version`, `ExecutionBundle`,
`ExecutionBundleDigest`, `ExecutionEngine`, `EngineConfig`,
`ServiceAccountName`, `ExecutionClusterID`, `Dependencies`, `Resources` and
`DeclaredParameterNames`. The selection contract accompanies that type with
`Parameters`, `SelectionRevision` and `SelectionDigest`.

The YAML above is illustrative; the shared Go type and generated CRD schemas
are canonical.

`SelectionDigest` is the SHA-256 digest of a canonical JSON preimage containing
the complete original selection snapshot, excluding the digest field itself.
The canonicalizer MUST use the fixed struct field order, lexicographically
sorted map keys, UTF-8 JSON without insignificant whitespace, and explicit
presence markers for nil versus empty maps. This preserves the distinction
between an absent parameter map and an explicit empty map. A shared helper and
fixed test vectors will be required by implementation.

The current execution contract remains `map[string]string`. “Generic” means
that the parameter names are not hard-coded by AF or RAR; it does not introduce
typed JSON values into WorkflowExecution.

AIAnalysis is the producer of the original selection. RAR is the approval-time
source of truth after the snapshot is copied into its immutable spec.

### 2. RAR status override semantics

The current parameter-only override is represented in RAR status:

```yaml
decision: Approved
workflowOverride:
  parameters:
    MEMORY_LIMIT: "768Mi"
    REPLICAS: "3"
  rationale: Increase memory for the observed workload
```

Effective parameter selection is not a per-key merge:

- `workflowOverride == nil` or `parameters == nil`: use the original RAR spec
  parameter map.
- For a non-nil replacement map, AuthWebhook requires every locked
  `TARGET_RESOURCE_*` key from the original RAR snapshot to be present with
  its original value. Missing or changed locked keys are rejected. Any new
  `TARGET_RESOURCE_*` key absent from the original snapshot is also rejected.
- After locked-key validation, use the non-nil map as the complete replacement
  map.
- Reject an explicit empty map when the original snapshot contains a locked
  `TARGET_RESOURCE_*` key; otherwise execute with no parameters.

The wire contract must preserve those states: omitted `parameters` decodes as
nil, while `parameters: {}` decodes as an explicit empty map. The AF patch
builder and CRD serialization must not use an `omitempty` path that drops an
explicit empty replacement.

For the current parameter-only AF contract, a non-empty `workflowName` in the
RAR status override is rejected rather than stripped. The existing #594
workflow-name field remains outside this scope for deferred/legacy handling;
workflow switching is tracked separately by #2572.

Workflow selection changes are excluded from this ADR's current scope. If that
capability is later approved, it must carry a complete resolved workflow
snapshot or immutable revision in RAR status; RO must not perform an arbitrary
live workflow lookup.

### 3. Terminal business fields

After `Approved`, `Rejected` or `Expired`, these fields are immutable:

- RAR spec and original selection snapshot
- `Decision`, `DecidedBy`, `DecidedVia`, `DecidedAt`
- `DecisionMessage`, `WorkflowOverride`, `Expired`
- top-level terminal `Reason` and `Message`

Controller-owned observations may still mutate under an explicit allowlist:

- Conditions required by DD-CRD-002
- `ObservedGeneration` accompanying condition updates
- `AuditRecorded` condition transitions while audit persistence retries

`TimeRemaining` may update while pending and stops changing after a terminal
decision. A later status update may not alter any business field.

### 4. Authorization and attribution boundary

- AF authenticates the OAuth user, performs MCP SAR/tool authorization and
  emits the human approval audit event.
- AF writes the RAR status update through its ServiceAccount.
- AuthWebhook authenticates the Kubernetes caller, trusts the AF ServiceAccount
  as the delegated intermediary, preserves `DecidedBy`, and sets `DecidedVia`.
- AuthWebhook is authoritative for RAR mutation validation and terminal business
  field immutability; it does not duplicate AF's OAuth/SAR check.
- RO consumes the admitted RAR snapshot and does not select a workflow or infer
  approval intent.

The audit roles are distinct:

- AF's tool-execution audit records the authenticated human's approval intent
  and request details. It is not proof that the RAR mutation was admitted.
- AuthWebhook's `webhook.remediationapprovalrequest.decided` event is the
  authoritative admission-attribution event. It records the authenticated
  identity and accepted business fields after validation; rejected mutations
  must not produce a successful event. It is not a transactionally durable
  proof that the API server has persisted the object.
- RO observes the persisted terminal RAR and owns the committed business
  decision event and downstream orchestration context. If a control objective
  later requires atomic audit persistence with the CRD write, the implementation
  plan must add an outbox/reconciliation mechanism; this ADR does not claim
  synchronous audit storage.

These events are correlated but must not be treated as duplicate committed
decisions.

### 5. Audit contract

Existing typed approval audit/readback payload types are extended. No new
override-specific event taxonomy is introduced. The extended representation
must support, subject to SOC2/FedRAMP control mapping:

- original selection and parameters
- complete override parameters and rationale
- effective execution selection
- authenticated human actor and AF delegation identity
- selection/review revision references
- sensitive-value redaction or references

The AF review read returns the RAR UID, immutable `selectionRevision` and
canonical `selectionDigest`. The approval input echoes those values. AuthWebhook
rejects the mutation when any binding differs from the current pending RAR.
`resourceVersion` remains the Kubernetes API concurrency guard; it is not the
business review revision.

The final field set is derived from ADR-034, DD-AUDIT-003, BR-AUDIT-005,
BR-AUDIT-006 and the applicable AU/AC/SI control objectives.

---

## Consequences

### Positive Consequences

1. RO executes exactly the approval-scoped RAR proposal.
2. Original and overridden values remain attributable without a live catalog
   dependency.
3. Terminal approval decisions cannot be rewritten through later status writes.
4. AF/AuthWebhook/RO responsibilities remain separated.

### Negative Consequences

1. AIA, RAR, AF, AuthWebhook and RO share a larger contract.
   - **Mitigation**: use one shared snapshot type and generated CRD artifacts.
2. RAR stores original selection data in addition to AIA.
   - **Mitigation**: define field-level sensitivity and redaction rules.
3. Existing workflow-name override behavior from #594 is not exposed by the
   current #2570 AF contract.
   - **Mitigation**: keep workflow switching deferred to #2572.

### Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| AuthWebhook admits a post-terminal business-field mutation | Medium | High | Compare old/new objects and enforce a frozen-field allowlist. |
| Original and override parameter evidence leaks sensitive values | Medium | High | Control-mapped redaction/reference policy and audit tests. |
| AIA/RAR snapshot shapes diverge | Medium | Medium | Shared type and serialization tests across both CRDs. |
| Editable metadata is missing before #2571 lands | High | Medium | Default catalog fields editable; keep KA-owned target parameters locked. |

---

## Validation Strategy

1. **UT-AF-2570-001 / BR-ORCH-025**: validate AIA-to-RAR snapshot
   serialization and nil/empty parameter semantics.
2. **UT-AW-2570-002 / BR-AUTH-001, SI-10**: validate AuthWebhook accepts one
   terminal business update and rejects later business-field mutations.
3. **IT-AW-2570-003 / BR-ORCH-026**: validate controller-only condition and
   audit bookkeeping remains possible.
4. **IT-AF-2570-004 / BR-AUTH-001, AU-3**: validate AF human audit plus
   AuthWebhook delegation attribution.
5. **IT-RO-2570-005 / BR-ORCH-025/026**: validate RO creates WE from RAR only
   and passes the complete effective map.
6. **E2E-AUD-2570-006 / BR-AUDIT-005/006, CC7.2, AU-3**: validate audit
   payload reconstruction and control-objective coverage.

Implementation planning and TDD scenarios are intentionally deferred until
this ADR/DD set is merged.

---

## References

- Issue #2570: standalone RAR parameter overrides
- Issue #2564: conditional RAR decisions for independent reviewers
- Issue #2565: RAR-scoped review evidence
- Issue #2571: RemediationWorkflow parameter editability metadata
- Issue #2572: deferred standalone RAR workflow-selection overrides
- ADR-034, ADR-040, ADR-022
- DD-AF-006, DD-WEBHOOK-003, DD-WORKFLOW-018

---

**Document Version**: 1.0
**Last Updated**: 2026-10-29
