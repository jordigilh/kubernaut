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
stores the terminal decision and the operator's partial parameter override.

**Pros**:

- RAR can be reviewed independently of an investigation session.
- RO consumes one approval-scoped source of truth.
- Original parameters cannot drift after request creation.
- Audit reconstruction can distinguish original, override and effective values.

**Cons**:

- The RAR CRD schema stores a second copy of selection data.
- User-input storage and access rules must be explicit.
- A shared schema-validation core and service-specific adapters must be wired and
  proven across KA, AF and AuthWebhook.

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
executionBundle: registry.example/oom-recovery@sha256:...
parameters:
  MEMORY_LIMIT: "512Mi"
  REPLICAS: "3"
```

The canonical typed execution portion of this snapshot is the existing
`sharedtypes.WorkflowSnapshot` from `pkg/shared/types/workflow_snapshot.go`.
It carries the catalog-authoritative workflow identity and execution fields:
`WorkflowID`, `WorkflowName`, `ActionType`, `Version`, `ExecutionBundle`,
`ExecutionBundleDigest`, `ExecutionEngine`, `EngineConfig`,
`ServiceAccountName`, `ExecutionClusterID`, `Dependencies`, `Resources` and
`DeclaredParameterNames`. The selection contract accompanies that type with
`Parameters` and the selected workflow's parameter schema.
The RAR also freezes the target context needed for
locking, resource checks, hashing and WorkflowExecution creation.

The YAML above is illustrative; the shared Go type and generated CRD schemas
are canonical.

The RAR spec is immutable and is the review source of truth; no separate
selection digest is required. Missing original parameter data fails closed; an
explicitly empty map means data was provided with no values and remains subject
to schema validation. Correct AIA's existing `omitempty` parameter representation
and verify it through the generated CRD. Integer parameter text remains exact
and must not be converted to float64. General numeric normalization of
workflow-author configuration is outside the agreed scope.

RO is the authorized creator of the single RAR per RR. If RO's create returns
`AlreadyExists`, never reuse the existing RAR: treat it as suspected tampering,
fail the investigation and RR, and route to manual review. This also applies to
a retry following an earlier successful create whose next step was not recorded;
the safety decision deliberately favors manual review over automatic recovery.
If the RAR is missing after the RR entered approval, fail and route to manual
review rather than creating a replacement. This detects and escalates a collision;
it does not prevent CR creation or guarantee that an independent controller has
not acted. Test the RO outcome through the production path. A separate
reviewer-facing RAR UID or reuse-provenance marker is not part of this contract.

For #2570 the workflow already stored in etcd is the catalog source. Preserve
the execution snapshot through approval; original author input precision before
catalog storage is outside this effort. Reviewer parameter processing must
retain both the original length and exact string value, in addition to passing
schema validation.

The current execution contract remains `map[string]string`. “Generic” means
that the parameter names are not hard-coded by AF or RAR; it does not introduce
typed JSON values into WorkflowExecution.

For schema-directed validation, a `string` value remains exact text; non-string
values are carried as JSON text representing the schema type, such as `3`,
`true` or `["a","b"]`. Invalid type representations are rejected. This is
decoding of string-valued transport, not typed transport (#2575).
Integer numeric text must be preserved without float64 conversion across the
selection handoff. The input contract accepts integer JSON text such as `3`,
not decimal or exponent notation such as `3.0` or `1e3`, even if a generic
parser considers those values mathematically integral. An omitted optional parameter remains omitted in the RAR;
review shows the omission. AW and RO do not insert schema defaults into the
effective parameter map, WE or audit evidence. A workflow may supply its own
runtime default; this contract does not assert that it does. The AF-to-API and
execution/audit integration paths must test omission.
Integer validation uses signed `int64`. Float validation uses finite `float64`
and rejects overflow and nonzero input that underflows to zero. Numeric schema
bounds must be compared without rounding an integer to float64.
The user states that workflow parameters contain no secrets. BR-AUDIT-011
describes KA Secret read permissions and sanitization before LLM output;
runtime isolation must be verified rather than assumed. WorkflowExecution delivers secret dependencies
through backend-specific mechanisms. This contract does not by itself establish
protection of rationale or error text.

Clarification: redacted Secret information may be presented as LLM investigation
context; workflow parameters are operational inputs defined by the discovery
agent, not a channel for consuming secrets. Execution secret dependencies are
delivered separately through WorkflowExecution.

For a new approval snapshot, absent schema-derived declared-parameter metadata
must prevent execution: the executor's nil allowlist otherwise passes all
parameters. An explicitly empty declared map means no parameters are declared.
The existing declaration filter remains authoritative for all executors;
undeclared managed target keys are not passed as executor inputs. The frozen
target safety context remains separately available to RO. This is a fail-closed
safety rule, not legacy RAR compatibility; migration is outside #2570.

### Shared parameter validation

KA, AF and AuthWebhook use one shared, side-effect-free workflow parameter
validation core for the complete effective map and the same schema constraints.
KA and AF adapt their typed inputs; AuthWebhook decodes the string-valued RAR
transport according to the reviewed schema before calling the core. The
transport remains `map[string]string`; typed transport is deferred to #2575.
The core must have one approved specification and common vectors for supported
types, required values, bounds, enum, pattern, defaults, dependencies and
malformed schemas. Exact decoding and edge semantics remain design-review
questions, not implied by an existing validator.

AF must receive the **full** selected workflow parameter schema, including
pattern, minimum, maximum and dependencies as well as type, required, default
and enum. Its current discovery projection omits some constraints. KA retains
its self-correction and managed-key handling; AF supplies preflight feedback;
AuthWebhook retains authoritative admission, overlay, locked-target and
identity checks. The core does not confer authorization or modify parameters.
Its callers must be connected to production paths and independently integration
tested. #2578 addresses the defective existing DataStorage helper without
asserting it currently has production callers.

AIAnalysis is the producer of the original selection. RAR is the approval-time
source of truth after the snapshot is copied into its immutable spec.

The user subsequently clarified that WE must fail closed if the workflow is
not found at runtime, even when the snapshot is complete. This changes the
snapshot-only execution/deletion behavior of DD-WORKFLOW-018 and requires a
trusted-namespace catalog lookup before WE dispatch. AW override identity
checking and WE lookup are point-in-time checks, not an atomic guarantee that
the catalog remains unchanged until a later write or external dispatch. The
proposed trusted-namespace `status.workflowId` selector requires exactly one
matching recomputed content ID. A successful no-match, ambiguity or mismatch
fails closed immediately; transient API/network failures receive at most three
total cancellable attempts, then fail closed. These checks remain subject to
design review. A selector miss alone does not establish why the ID was absent.
External bundle and dependency availability remain execution-time concerns.

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

Effective parameters are the original map overlaid with the submitted changes:

- `workflowOverride == nil` or `parameters == nil`: use the original RAR spec
  parameter map.
- Submitted keys must already exist in the original map. A submitted
  `TARGET_RESOURCE_*` key must have its original value; new keys are rejected.
- Omitted keys retain their original values. An explicit empty map changes
  nothing, including when locked keys exist.
- AuthWebhook validates the complete effective map against the frozen workflow
  schema, after confirming the current catalog object matches the reviewed
  workflow ID. Missing or changed workflows fail closed; transient network
  errors may be retried with bounded backoff.

The wire contract must preserve those states: omitted `parameters` decodes as
nil, while `parameters: {}` decodes as an explicit empty map. The AF patch
builder and CRD serialization must not use an `omitempty` path that drops an
explicit empty override.

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

The Kubernetes API server is the first-line immutability guard. The RAR CRD
schema MUST use CEL transition validation with explicit field-preservation
guards: once `status.decision` is first set, the frozen business fields above
must equal their previous values; once `status.workflowOverride` is first set,
  it must not change, and it cannot be added after a terminal decision. Root-level
  guards must prevent removing the optional status parent after a decision or
  an override is set. This follows the existing field-by-field transition pattern
used by `AIAnalysis.status.selectedWorkflow` after `selectedAt` and by
WorkflowExecution spec immutability. A whole-status equality rule is not used,
because controller-owned observations must remain mutable and nullable fields
must retain their nil-versus-empty meaning.

Controller-owned observations may still mutate under an explicit allowlist:

- Conditions required by DD-CRD-002
- `ObservedGeneration` accompanying condition updates
- `AuditRecorded` condition transitions for enqueue bookkeeping; it does not
  certify DataStorage persistence

`TimeRemaining` may update while pending and stops changing after a terminal
decision. A later status update may not alter any business field.

### 4. Authorization and attribution boundary

- AF authenticates the OAuth user, performs MCP SAR/tool authorization and
  emits the human approval audit event.
- AF writes the RAR status update through its ServiceAccount.
- AuthWebhook authenticates the Kubernetes caller, trusts the AF ServiceAccount
  as the delegated intermediary, preserves `DecidedBy`, and sets `DecidedVia`.
- AuthWebhook is authoritative for RAR mutation validation and terminal business
  field context checks and mirrors the CEL frozen-field allowlist; it does not
  duplicate AF's OAuth/SAR check.
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
  decision event and downstream orchestration context. Emission uses the
  existing fire-and-forget in-memory buffer. Enqueue is not durable persistence;
  this ADR adds no synchronous Flush gate, audit finalizer or outbox. Persistent
  Valkey-backed buffering is a future design, outside #2570.

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

User-entered text is not sanitized or redacted; preserve it exactly in the RAR
and in any audit fields that include it. Limit RAR and audit readback to
authorized callers and test denial for unauthorized callers. Workflow parameters
are not a secret-consumption channel; this contract does not detect or prevent
accidental secret input. KA's tool-output sanitizer is not a protection for RAR
user input or approval audit fields.

The AF review read and approval input use the RR-derived unique RAR name. The
immutable RAR spec is the reviewed selection, and Kubernetes `resourceVersion`
guards concurrent status updates. No separate selection revision is required.

The final field set is derived from ADR-034, DD-AUDIT-003, BR-AUDIT-005,
BR-AUDIT-006 and the applicable AU/AC/SI control objectives.

---

## Consequences

### Positive Consequences

1. RO executes exactly the approval-scoped RAR proposal.
2. Original, changed and effective values remain attributable. Override
   admission requires catalog availability; ordinary AuthWebhook decisions
   without overrides do not. Every WE dispatch, including automatic execution,
   still requires the trusted catalog identity check.
3. Terminal approval decisions cannot be rewritten through later status writes.
4. AF/AuthWebhook/RO responsibilities remain separated.

### Negative Consequences

1. AIA, RAR, AF, AuthWebhook and RO share a larger contract.
   - **Mitigation**: use one shared snapshot type and generated CRD artifacts.
2. RAR stores original selection data in addition to AIA.
   - **Mitigation**: enforce and test the existing RAR and audit authorization boundaries.
3. Existing workflow-name override behavior from #594 is not exposed by the
   current #2570 AF contract.
   - **Mitigation**: keep workflow switching deferred to #2572.

### Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| AuthWebhook admits a post-terminal business-field mutation | Medium | High | Compare old/new objects and enforce a frozen-field allowlist. |
| Original or override user input contains sensitive values | Medium | High | Restrict RAR/audit access and test unauthorized read denial; input is not redacted or automatically classified. |
| AIA/RAR snapshot shapes diverge | Medium | Medium | Shared type and serialization tests across both CRDs. |
| Editable metadata is missing before #2571 lands | High | Medium | Default catalog fields editable; keep KA-owned target parameters locked. |

---

## Validation Strategy

1. **UT-AA-2570-067/068; IT-AA-2570-001; IT-AF-2570-003 / BR-ORCH-025**:
   validate AIA-to-RAR nil/empty original maps and AF-to-API-server omitted,
   empty and partial override serialization without clearing omitted values.
2. **UT-AW-2570-021/087; IT-AW-2570-004/005/011 / BR-AUTH-001, SI-10**:
   validate one terminal business update, rejection of later business-field
   mutations and rejection of a non-empty legacy workflow name before approval.
3. **IT-AW-2570-005 / BR-ORCH-026**: validate controller observation updates
   remain possible after a decision.
4. **IT-AF-2570-003; E2E-AUD-2570-008 / BR-AUTH-001, AU-3**: validate AF
   authorization, human audit and AuthWebhook delegation attribution.
5. **IT-RO-2570-006 / BR-ORCH-025/026**: validate RO creates WE from RAR only
   and passes the complete effective map to actual executors.
6. **E2E-AUD-2570-008 / BR-AUDIT-005/006, CC7.2, AU-3**: validate audit
   payload reconstruction and applicable control-objective coverage.
7. **CHECKPOINT W — IT-2570-020/021/022**: validate trusted namespace/RBAC,
   pre-change RAR failure handling and RAR creation conflict handling.

The implementation and TDD plans are proposed in `docs/tests/2570/`; feature
implementation remains deferred until the documents are reviewed and approved.

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
