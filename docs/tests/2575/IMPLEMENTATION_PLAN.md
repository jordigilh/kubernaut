# Implementation Plan: #2575 — Typed Workflow Parameter Contract

**Status:** Implementation complete — CI integration validation pending
**Issue:** [#2575](https://github.com/jordigilh/kubernaut/issues/2575)
**Branch:** `plan/2575-parameter-contract`
**Created:** 2026-11-06
**Test plan:** [TEST_PLAN.md](TEST_PLAN.md)

## 1. Decision gate

This document records the authorized implementation and its remaining evidence
gates. Production code and generated assets followed the RED → GREEN → REFACTOR
and wiring sequence below.

The owner approved the contract decisions in Section 12 on 2026-11-08. The
implementation and local contract gates are complete; production-path
integration/E2E, RBAC, authenticated audit readback, and operational cutover
evidence remain:

- `RemediationWorkflow.spec.parameters` is the intended single schema authority.
- Cross-CR parameter transport normalization is required scope for #2575:
  AIA-selected parameters, RAR original/override/effective parameters, and WE
  effective parameters use one approved typed representation and one presence
  contract. The implementation now uses the approved typed union; string
  conversion remains only at the Job/Tekton engine boundary.
- Removing the AIA/RAR/WE **parameter-schema and schema-derived allowlist** fields
  is part of that refactor. The immutable reviewed values, workflow identity,
  target safety context, and non-schema execution metadata remain as the
  approval snapshot.
- [ADR-076](../../architecture/decisions/ADR-076-rar-approval-snapshot-and-terminal-decision-contract.md),
  [DD-WORKFLOW-018](../../architecture/decisions/DD-WORKFLOW-018-etcd-single-source-of-truth.md),
  and the #2570 documents are not modified by this planning change.

## 2. Business and architectural contract

### 2.1 Requirements in scope

- **BR-WORKFLOW-004:** workflow parameter types and constraints originate from
  the workflow schema.
- **BR-WORKFLOW-006:** workflow registration and lifecycle are controlled by
  the namespaced `RemediationWorkflow` CRD; etcd is the catalog source of truth.
- **BR-KA-191:** KA validates selections and gives the LLM a bounded
  self-correction opportunity.
- **BR-ORCH-025/026:** RO preserves the selected workflow and approval path.
- **BR-WE-015:** Ansible receives typed `extra_vars` rather than heuristic
  string guesses.
- **BR-AUDIT-005/006:** business-critical decisions remain correlated,
  actor-attributed, queryable, and reconstructable from audit events.
- **BR-SECURITY-016:** authorization and failure outcomes remain observable and
  fail closed.

`BR-WE-001` is cancelled and is not reactivated. The proposed WE check is a
final integrity and schema-consistency gate using the same live workflow
contract, not a second independently maintained schema validator.

### 2.2 Non-negotiable rules

1. The trusted `RemediationWorkflow.spec.parameters` is the only parameter
   schema used for validation, declaration filtering, defaults metadata, and
   execution conversion.
2. AIA, RAR, WE, AgentSession, and other persisted CRDs use one normalized typed
   parameter representation, but do not carry a full parameter schema or a
   schema-derived parameter-name allowlist. The live workflow is read when a
   boundary needs schema information.
3. Workflow identity is fenced by the content-derived `status.workflowId`.
   Name, version, bundle, or display fields are never fallback identity keys.
4. A workflow lookup is accepted only when it is in the configured trusted
   namespace, returns exactly one object, has the expected status ID, and its
   clean CRD content recomputes to that same ID. Missing, changed, ambiguous,
   foreign, malformed, or unverifiable content fails closed.
5. The frozen selection and target safety context still protect the #2570
   approval path. Omitted override keys retain their original values; `{}` is
   a no-op; new keys and changed `TARGET_RESOURCE_*` values are rejected; no
   schema default is inserted implicitly.
6. Every error path is observable through status/conditions, Kubernetes events,
   structured logs, and applicable audit evidence. No failed verification may
   create an external execution resource.

## 3. Proposed typed contract

Cross-CR parameter normalization and removal of schema copies are required
scope. The following concrete transport contract was approved on 2026-11-08 and
is implemented in `pkg/shared/types/typed_parameter.go`. The code-generation
and API-server qualification is recorded in `SPIKE_REPORT.md`.

### 3.1 Public representation

Use the spike-proven named structural discriminated union:

```go
type TypedParameterMap map[string]TypedParameterValue

type TypedParameterValue struct {
    Kind          string
    StringValue   *string
    IntegerValue  *int64
    FloatValue    *float64
    BooleanValue  *bool
    ArrayValue    []TypedPrimitiveParameter
}
```

The persisted map values are structural union objects, not `map[string]any`,
`interface{}`, schema copies, or stringified values. Arrays contain primitive
union elements only; nested arrays and objects are rejected. Ordinary JSON is
used only at LLM/tool ingress and is converted through the strict codec before
typed construction.

The top-level field representation must preserve an explicit empty object.
If a pointer/map field cannot distinguish omitted, `null`, and `{}` through a
real API-server round trip, add a small presence-aware wrapper. The wrapper is
transport metadata, not a schema copy.

### 3.2 Alternatives and #1172 reconciliation

Issue #1172 evaluated tightening the raw LLM-facing
`map[string]interface{}` representation to `map[string]string` across the data
path. It is closed as not planned; #2570 retained the string-valued execution
contract. #2575 chooses neither public shape: it normalizes the persisted
cross-CR contract to typed raw JSON and converts only at an engine boundary.

| Alternative | Strengths | Risks / reason for decision |
|---|---|---|
| Keep `map[string]string` as the normalized CRD contract | Smallest generated-CRD and engine change; compatible with #2570 | Loses native JSON type information, requires schema-directed string decoding, and cannot preserve typed values across AIA/RAR/WE. Rejected for #2575. |
| Named structural discriminated union with primitive-only arrays | Provides CEL-visible typed fields, explicit union validation, zero-like value preservation, and raw evidence through the separate evidence field | Selected and spike-proven; implementation uses the shared typed union. |
| Explicit discriminated union struct per value | Makes the Go/API type tag explicit and can reject unsupported values early | Larger CRD/OpenAPI surface, more conversion code, and a second union grammar to keep aligned with JSON. Fallback only if raw JSON cannot round-trip safely. |
| Public persisted `map[string]any` / `map[string]interface{}` | Easiest raw LLM handoff | Unbounded CRD/audit contract, weak structural guarantees, and the exact ambiguity that #1172 identified. Rejected for the persisted cross-CR contract; internal LLM parsing is outside this gate. |

The approved direction is typed cross-CR normalization. The owner selected the
structural union rather than the rejected raw-JSON map, and its generated-schema
and API-server proof is recorded in the spike report. Production-path evidence
remains a separate release gate.

Internal LLM parsing DTOs remain non-authoritative and are not part of the
persisted cross-CR representation. They still must preserve parameter number
tokens at the input boundary: retain the existing `UseNumber` behavior in
`internal/kubernautagent/parser/parser_llm_types.go` and the selected-workflow
raw-parameter extraction in `pkg/aianalysis/handlers/response_processor.go`.
The implementation must not introduce a default `json.Unmarshal` into
`float64` before typed-value construction or `parameter_evidence` capture.
This is an input-fidelity invariant, not a new persisted LLM DTO contract.

### 3.3 Value grammar

| Workflow schema type | Accepted JSON value | Required behavior |
|---|---|---|
| `string` | JSON string | Preserve the string exactly; no trimming or coercion. |
| `integer` | JSON integer number | Validate without float64 conversion; retain signed-int64 safety. The #2570 recommendation to reject decimal/exponent spellings remains unless the owner explicitly changes it. |
| `float` | JSON number | Reject `NaN`, infinity, overflow, and nonzero underflow. Do not silently convert to a string. |
| `boolean` | JSON boolean | Accept only `true` or `false`; strings such as `"true"` are not typed booleans in the new contract. |
| `array` | JSON array | Preserve element order and validate JSON syntax. `[]` is an explicit value. |
| `null` | None of the current schema types | Reject as a parameter value. `null` is a top-level presence state, not a typed workflow value. |
| object | Not a current workflow parameter type | Reject at the parameter boundary. Nested JSON objects are not exposed as a public parameter type by this issue. |

The live CRD schema remains authoritative for `required`, `enum`, `pattern`,
numeric bounds, and `dependsOn`. Unknown parameter names are not added to a
selection or override. KA retains its existing bounded self-correction and
managed-key behavior; AF/AW/WE retain authoritative safety checks.

### 3.4 Presence semantics

| Boundary | Absent field | Explicit `null` | Explicit `{}` | Explicit zero/false/empty value |
|---|---|---|---|---|
| AIA selected parameters | Missing data; fail closed before persistence/execution | Missing data; fail closed | Present-empty map; validate against the live schema | Real value; preserve it |
| RAR original parameters | Missing frozen selection; fail closed | Missing frozen selection; fail closed | Present-empty map; preserve it | Real value; preserve it |
| `WorkflowOverride.parameters` | No-op | No-op | No-op, while remaining distinguishable on the wire if submitted | Real partial change |
| WE effective parameters | Invalid/missing transport; fail before dispatch | Invalid; fail before dispatch | Valid only when the live schema permits an empty effective map | Real execution input |
| Audit payloads | Preserve omission when it is observable | Preserve `null` when it was submitted | Preserve `{}` | Preserve the typed value |

No boundary inserts a workflow `default`. A default remains workflow-author
metadata; if a workflow runtime applies its own default, that is outside this
transport contract and must not be claimed by these tests.

### 3.5 Canonical serialization recommendation

The recommendation is to separate **submitted evidence** from **comparison
encoding**:

- Persist and audit each validated raw JSON value without changing its semantic
  value or user-entered numeric lexeme.
- For deterministic equality/fingerprinting, serialize the parameter object with
  lexicographically sorted parameter keys and compact valid JSON.
- Preserve array order. Canonicalize nested object key order only if nested
  objects are admitted inside an array by the approved array contract.
- Validate integer and float tokens with `json.Decoder.UseNumber` or equivalent;
  never pass an integer through `float64`.
- Do not use the comparison encoding to insert defaults, reorder arrays, or
  rewrite the value stored in a CRD or audit event.

The exact numeric canonicalization rule is still an approval item. The safest
compatibility choice is to retain raw numeric lexemes for evidence and make the
canonical comparator explicitly lexeme-sensitive. A semantic numeric
normalizer would make `1`, `1.0`, and `1e0` indistinguishable and would weaken
the #2570 exact-input audit guarantee.

### 3.6 Workflow identity and in-flight revision fencing

The implementation should extend the existing trusted lookup helper so callers
can receive the verified workflow object (or an equivalent ephemeral verified
view) without persisting its schema:

1. Resolve the configured trusted namespace; reject empty, malformed, default,
   or cluster-wide fallback configuration.
2. Query `RemediationWorkflow` by `status.workflowId` in that namespace.
3. Require exactly one result and require both namespace and status ID to match.
4. Recompute the clean CRD content hash and content-derived UUID using the
   existing `pkg/shared/contenthash` functions. Require the recomputed UUID to
   equal the frozen ID. If `status.contentHash` is present, require it to equal
    the recomputed hash; the post-cutover policy for a missing status hash is an
   owner decision.
5. Validate the returned `spec.parameters` and use that exact object for the
   current operation. Never substitute a catalog cache, name/version match, or
   a snapshot schema.

The check occurs at each relevant boundary:

| Boundary | Live verification purpose | Fail-closed result |
|---|---|---|
| KA selection | Validate LLM values against the selected CRD | Bounded self-correction; then human review/failure |
| AgentSession → AIA | Do not persist a selection based on stale or foreign content | AIA is not committed as executable selection |
| RO → RAR | Freeze typed original values only after live validation | No RAR approval request from unverifiable data |
| AW override admission | Validate the complete effective map and enforce locked keys | No decision/status mutation |
| RO approved RAR → WE | Revalidate the frozen ID and effective values | No WE from an unverifiable approval |
| WE Pending reconcile | Recheck immediately before external resource creation | WFE fails before Job/PipelineRun/AWX dispatch |

An update to workflow `spec` produces a new content-derived ID; the old frozen
ID therefore fails lookup rather than silently switching revision. Deletion,
foreign-namespace-only matches, duplicate IDs, content-hash mismatch, invalid
schema, or an unverifiable API read all fail closed. Successful no-match,
authorization/configuration errors, and malformed content are not retried.
Transient API/network failures retain the existing maximum of three total
attempts with cancellable bounded backoff. The lookup is point-in-time and does
not claim an atomic guarantee against deletion after a successful read.

### 3.7 Reconcile invariant inventory

Workflow existence and exact-content verification is a general reconcile
invariant, not only a parameter-validation step. Every relevant reconcile path
must verify the trusted workflow before it completes a state transition, writes
an approval decision, creates a child CR, or creates/recreates an external
execution resource. Paths that cannot require the workflow because they only
observe deletion or perform non-execution cleanup must be listed as explicit,
owner-approved lifecycle exceptions rather than silently omitted.

The implementation must inventory and integration-test these paths explicitly:

| Reconcile path | Required verification point | Required outcome on missing/changed/unverifiable workflow |
|---|---|---|
| WFE fetch/not-found path | No workflow object exists to verify; return without a business transition | Do not claim successful workflow processing. |
| WFE finalizer addition/update | Before adding the finalizer or completing the update | If verification fails before a finalizer exists, return/requeue an observable error without mutating finalizer or status; do not claim a terminal failure. If verification succeeds, add the finalizer and requeue. |
| AgentSession/AIA selection persistence | Before AIA selection/status persistence | Do not commit an executable selection. |
| RO approval and non-approval child creation | Before RAR or WE creation | Do not create the child CR. |
| AW override admission | Before the conditional decision/status write | Deny without mutating the decision. |
| WE Pending with finalizer present | Before selection audit completion or external resource creation | Mark WFE failed with an observable reason; create no Job/PipelineRun/AWX resource. |
| WE Running/status progress | Before every status transition, reconcile completion, or resource recreation | Fail closed; do not complete the reconcile, recreate, or dispatch a resource. |
| WE terminal audit/cleanup | Verify before every terminal reconcile completion, audit, or status update; any cleanup-only exception must be owner-approved and tested | Never use a missing workflow to substitute a different workflow or claim successful terminal processing. |
| WE deletion/finalizer removal | Explicit cleanup-only lifecycle exception; verify that no execution or approval mutation occurs | Remove only the finalizer/owned resource required by deletion; never create execution. |

The production implementation must not rely on the Pending check alone. The
state inventory, finalizer/deletion classification, and corresponding
production-path integration test are part of CHECKPOINT W.

### 3.8 Engine conversion

The typed map is converted once at the executor boundary using the verified live
schema; executors do not infer types by trying parsers in sequence.

| Engine | Proposed boundary representation | Safety rule |
|---|---|---|
| Tekton | Send string values as their raw string contents; serialize non-string values as compact JSON text for Tekton's string parameter API | `safe` stays `safe` (not `"safe"`); `3`, `true`, and `[1,2]` are intentional JSON text. Empty string stays empty. |
| Kubernetes Job | Send string values as raw environment-variable contents; serialize non-string values as compact JSON text for the existing environment-variable contract | `safe` stays `safe` (not `"safe"`); preserve empty strings and explicit zero/false; filter names from the live schema. |
| Ansible/AWX | Send raw JSON values in `extra_vars` | Replace `coerceValue` heuristics; do not turn an invalid typed value into a string. |

The executor options must receive an ephemeral verified schema/declaration view
created during reconciliation. `WorkflowSnapshot.DeclaredParameterNames` is a
schema-derived copy and must not remain in any persisted CRD. Dependency and
execution metadata that is not parameter schema may remain in the frozen
execution snapshot.

### 3.9 Audit contract

The typed audit payload must include:

- `workflow_id` and the verified workflow content hash/revision identifier;
- original, submitted override, and effective typed parameter maps;
- field presence needed to distinguish omitted, `null`, and `{}` where the
  source boundary exposed that distinction;
- existing correlation ID, resource references, actor type/ID, action, outcome,
  and standardized `error_details` on failure;
- the override rationale and delegated-user fields already required by the
  approval path.

Field ownership is event-specific; no event is required to contain values that
do not exist at its emission point:

| Event boundary | Required parameter/evidence fields |
|---|---|
| AF approval intent | Submitted override, rationale, actor, correlation, and the reviewed workflow reference available at intent time. Original/effective values are not required before admission. |
| AuthWebhook admission | Accepted original, submitted override, effective values, `parameter_evidence`, authenticated/delegated actor fields, and workflow identity available from the admitted RAR. |
| RO committed decision | Original, submitted override, effective values, `parameter_evidence`, workflow ID, correlation, rationale/business context, and outcome. |
| WE execution events | Effective values, `parameter_evidence`, workflow ID/content identity, correlation, outcome, and `error_details` on failure. |
| Workflow admission event | Workflow ID, content hash, and full admitted workflow content used for later schema reconstruction. |

The correlated event set, rather than any single event, must provide the
complete original/override/effective decision trace.

Parameter audit events do not embed a second schema. Typed parameter maps may be
stored in the semantic JSON/JSONB event payload, but JSONB is not a lossless
numeric-lexeme evidence store. Each audit event must also carry a separate
lossless `parameter_evidence` string/byte field (for example, compact sorted-key
JSON built from raw value tokens) that preserves submitted numeric lexemes such
as `1`, `1.0`, and `1e0`. The OpenAPI and persistence contract must round-trip
that field as text/bytes, not reparse it as JSONB.

Reconstruction first
selects remediation lifecycle events by correlation ID, extracts `workflow_id`,
and joins to the workflow admission events by that ID; the admission content
and content hash remain the reconstruction source for the exact
`RemediationWorkflow.spec.parameters` used by the operation. Audit remains
fire-and-forget: enqueue failure is observable and is not reported as durable
success unless the existing audit contract says so. If the admission event is
missing because best-effort audit delivery failed, reconstruction must return an
explicit incomplete/failure outcome with an operator-visible reason and must not
claim complete reconstruction or silently substitute the live CRD.

## 4. Preflight findings and change inventory

| Current evidence | Required change |
|---|---|
| `AIAnalysis.Status.SelectedWorkflow` has `Parameters map[string]string` and `ParameterSchema`. | Normalize to the shared typed representation; remove `ParameterSchema`; revalidate against the live workflow. |
| RAR spec and `WorkflowOverride` use string maps; RAR freezes `ParameterSchema`. | Normalize original/override/effective values to the shared typed representation; remove the schema snapshot; retain #2570 overlay and immutability rules. |
| `WorkflowExecution.Spec.Parameters` is a string map and `WorkflowRef` includes `DeclaredParameterNames`. | Normalize effective values to the shared typed representation; remove the schema-derived allowlist from the CRD; obtain it from the verified workflow during Pending reconciliation. |
| `pkg/remediationapprovalrequest/parameters.go` builds validation input from the RAR schema copy. | Replace snapshot validation with a live verified workflow input. |
| `pkg/shared/workflowidentity.Verify` validates identity but returns only an error. | Add an internal verified-object/view path while retaining exact ID/content checks and retry classification. |
| KA parser and AgentSession response handling already use number-safe parameter decoding (`UseNumber`) at their raw JSON boundaries. | Preserve that behavior through the typed handoff; add a production-path regression for `9007199254740993`, `1.0`, and `1e0` through AgentSession and audit evidence, without making the internal DTO a persisted authority. |
| Ansible `coerceValue` heuristically parses strings. | Consume raw typed JSON and reject malformed values before launch. |
| Audit OpenAPI schemas and generated Ogen types use `map[string]string`; repository create/batch paths decode JSON numbers through `map[string]interface{}`. | Define typed raw-JSON payloads, regenerate clients, use number-safe decoding in both audit persistence paths, and test real persistence/readback. |
| Current tests assert `ParameterSchema` and `DeclaredParameterNames` round trips. | Replace those assertions with live-schema lookup, no-copy assertions, and typed round trips. |

### 4.1 Expected implementation files

This list was the scope map for the approved implementation; the affected source
and generated files are changed in the current working tree.

- `api/aianalysis/v1alpha1/aianalysis_types.go`
- `api/remediation/v1alpha1/remediationapprovalrequest_types.go`
- `api/workflowexecution/v1alpha1/workflowexecution_types.go`
- `pkg/shared/types/workflow_snapshot.go`
- `pkg/shared/workflowidentity/verify.go` and a shared typed-parameter package
- `pkg/remediationapprovalrequest/parameters.go`
- `pkg/remediationorchestrator/creator/approval.go`
- `pkg/remediationorchestrator/creator/workflowexecution.go`
- `pkg/aianalysis/handlers/response_processor.go` and workflow snapshot parsing
- `internal/kubernautagent/parser/parser_format.go` and
  `parser_llm_types.go` for number-safe raw parameter ingress
- `pkg/apifrontend/tools/crd_tools.go` and approval audit adapters
- `pkg/authwebhook/remediationapprovalrequest_handler.go` and audit builders
- `internal/controller/workflowexecution/workflowexecution_pending.go`
- `internal/controller/workflowexecution/workflowexecution_controller.go` and
  `workflowexecution_lifecycle.go` for the full reconcile-state invariant
- `pkg/workflowexecution/executor/{executor,filter,tekton,job,ansible}.go`
- `api/openapi/data-storage-v1.yaml` and generated `pkg/datastorage/ogen-client/`
- `pkg/datastorage/repository/audit_events_create.go` and
  `audit_events_batch.go` for number-safe audit JSON decoding/readback
- generated CRDs and embedded CRD assets for every affected API type

The parent ADR-076, DD-WORKFLOW-018, and #2570 records remain unchanged. The
approved supersession boundary is recorded in
`ADR-076-ADDENDUM-001-typed-workflow-parameter-contract.md`; it is not a reason
to rewrite the protected historical decisions.

### 4.2 Supersession boundary

#2575 does not preserve the duplicated parameter-schema or declared-name fields
as a compatibility target. Once approved, it supersedes those transport details
from the #2570/ADR-076/DD-WORKFLOW-018 baseline while preserving the approval
snapshot's reviewed values, immutable decision semantics, target locking,
collision handling, audit evidence, and other safety properties.

Because ADR-076 and DD-WORKFLOW-018 are versioned architecture records, and the
#2570 plans are the approved work record for the safety baseline, the
supersession must be recorded in a new owner-approved ADR/DD amendment (or a
new issue-specific decision) before production implementation. This planning
document records the intended boundary; it does not rewrite the historical or
currently approved records.

## 5. Breaking rollout and existing-object policy

Backwards compatibility is not required for #2575. The parameter contract is a
coordinated breaking change across the CRDs, generated clients, controllers,
webhooks, executors, and audit adapters.

### 5.1 Coordinated cutover

1. Update the source API types, generated CRDs, OpenAPI/client artifacts, and all
   production readers and writers in the same release.
2. Install the typed CRD/OpenAPI contract before enabling the updated services;
   typed writes are active immediately after the cutover.
3. Do not add a dual reader, legacy writer, compatibility feature flag, drain
   counter, or runtime conversion path.
4. Quiesce legacy writers/controllers and inventory AgentSession, AIA, RAR, and
   WE objects before cutover. No pre-cutover object may remain in an active
   selection, approval, or execution lifecycle; if the inventory is not clean,
   block the release before enabling the typed readers. This operational gate
   is required because a legacy JSON string such as `{"P":"safe"}` is
   indistinguishable from a typed JSON string after persistence without
   provenance metadata.
5. Rollback is allowed only to another release implementing this same typed
   contract, or before typed objects are created. A rollback to a legacy-only
   reader/writer is not supported.

### 5.2 Existing-object policy

- Do not mutate an immutable RAR or rewrite an existing object in place.
- Legacy string-valued parameter transport is not decoded or adopted by the new
  contract, even when a live workflow schema could theoretically interpret it.
- Existing `ParameterSchema`/`DeclaredParameterNames` data is never trusted as
  an authority and is removed from the new CRD contract.
- A pre-cutover object in an active lifecycle is a cutover failure, not a runtime
  compatibility case. The deployment must remain blocked or be rolled back;
  the new readers must not silently upgrade or execute it.

## 6. TDD implementation sequence

### RED — contract and failure proof

1. Add failing Ginkgo/Gomega unit cases for the typed value grammar,
   presence semantics, canonical comparison, live schema validation, exact
    content identity, cutover-gate verification, engine serialization, and audit
   payloads.
2. Add failing integration cases through AIA, RAR, AW, RO, WE, the generated
   CRDs, and the real Data Storage audit path. Each wiring row below needs its
   own production-dispatch test.
3. Add failing E2E journeys for successful typed execution, approved typed
   override, revision change/deletion, and correlated audit reconstruction.
4. Prove the failure is caused by the missing contract, not by an unrelated
   baseline failure. Record unrelated failures separately.

### GREEN — minimal implementation and wiring

1. Add the approved typed representation and presence-aware serialization.
2. Wire the live workflow verifier into each boundary before adding richer
   conversion behavior.
3. Replace snapshot schema reads with the ephemeral verified workflow view.
4. Make each executor pass typed values without heuristic inference.
5. Update OpenAPI/generated artifacts and production audit adapters.
6. Complete CHECKPOINT W: every production caller and every integration row
   below passes before the phase is called GREEN.

### REFACTOR — behavior-preserving cleanup

Refactor only after GREEN is passing: remove duplicated map conversion, centralize
error classification, improve error context, and simplify executor adapters.
Do not introduce new behavior or new components in REFACTOR. If GREEN leaves no
legitimate cleanup, mark REFACTOR `N/A` with the reason. Run the post-refactor
build and compile-only test gate before declaring completion.

## 7. Wiring manifest

| Component | Production entry point | Wiring location | Proving integration test |
|---|---|---|---|
| Typed value representation and shared validation | KA selection, AF preflight, AW admission, WE pre-dispatch | New shared typed-parameter package plus existing adapters | `IT-KA-2575-002`, `IT-AF-2575-005`, `IT-AW-2575-006`, `IT-WE-2575-009` |
| Trusted workflow verifier | Every schema-dependent boundary | `pkg/shared/workflowidentity/verify.go` and callers | `IT-AW-2575-007`, `IT-WE-2575-011`, `IT-INV-2575-016` |
| General reconcile invariant | AIA, RO, AW, and every schema-dependent WE state | Reconcile state handlers and child/resource creation gates | `IT-INV-2575-016` |
| KA live-schema selection | AgentSession/KA workflow selection | KA catalog/validator and selection response builder | `IT-KA-2575-002` |
| AgentSession → AIAnalysis handoff | AIA reconciliation/response processing | `pkg/aianalysis/handlers/response_processor.go` and snapshot mapping | `IT-AA-2575-003` |
| AIA → RAR typed freeze | RO approval orchestration | `pkg/remediationorchestrator/creator/approval.go` | `IT-RO-2575-004` |
| AF review and override | MCP approval tool/API path | `pkg/apifrontend/tools/crd_tools.go` | `IT-AF-2575-005` |
| AW authoritative override admission | RAR validating webhook | `pkg/authwebhook/remediationapprovalrequest_handler.go` | `IT-AW-2575-006`, `IT-AW-2575-007` |
| Approved RAR → WE | RO approved-decision path | `pkg/remediationorchestrator/creator/workflowexecution.go` | `IT-RO-2575-008` |
| WE live verification and filtering | WFE Pending reconciliation | `internal/controller/workflowexecution/workflowexecution_pending.go` | `IT-WE-2575-009`, `IT-WE-2575-011` |
| Typed engine adapters | Job, Tekton, and Ansible dispatch | `pkg/workflowexecution/executor/{job,tekton,ansible}.go` | `IT-WE-2575-010` |
| Typed correlated audit | approval intent, admission, decision, execution | RO/AF/AW/WE audit builders and Data Storage OpenAPI | `IT-AUD-2575-012` |
| Number-safe audit persistence | Data Storage audit create and batch paths | `pkg/datastorage/repository/audit_events_create.go`, `audit_events_batch.go` | `IT-AUD-2575-012` |
| Generated contract and RBAC | API-server acceptance and trusted namespace reads | generated CRDs, embedded assets, deployment Roles/RoleBindings | `IT-CRD-2575-001`, `IT-SEC-2575-014` |
| Breaking cutover gate | Deployment activation | legacy writers/controllers quiesced and active pre-cutover object inventory is clean | `IT-CUT-2575-013` |

**CHECKPOINT W rule:** a unit test cannot satisfy a wiring row. Each row needs a
real production entry point, a real boundary traversal, and an observable
business result. A missing row is a release blocker.

## 8. Verification and compliance gates

The implementation is complete only when all of the following pass:

- all P0 unit scenarios in [TEST_PLAN.md](TEST_PLAN.md), with at least 90% of
  all defined in-scope unit scenarios and 100% of critical scenarios;
- every wiring row and applicable FedRAMP control has a passing integration test;
- the E2E journeys prove typed value fidelity, no execution on revision failure,
  authorization/namespace isolation, and audit reconstruction;
- no AIA/RAR/WE/AgentSession CRD contains a parameter schema or
  `DeclaredParameterNames` copy;
- no new persisted CRD/audit parameter API uses `map[string]any` or heuristic
  string coercion; existing internal LLM parsing DTOs are outside this gate;
- `go build ./...` passes;
- `go test ./... -run=^$ -timeout=30s` passes after refactor;
- `golangci-lint run --timeout=5m` has no new findings;
- affected `make test`, integration, E2E, CRD-generation, OpenAPI-generation,
  and breaking-cutover checks pass;
- audit events map to AU-2/AU-3/AU-9/AU-11 as applicable, SOC2 CC7.2, SI-10,
  AC-4, AC-6, and applicable OWASP/ASVS logging and authorization objectives;
- #2570/#2601 safety regressions remain green, including locked target keys,
  first-decision-wins, collision handling, no default insertion, audit enqueue
  observability, and trusted namespace behavior.

## 9. Risks and mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| CRD code generation cannot express a raw JSON map safely | API-server pruning or rejected objects | Run a disposable codegen/envtest spike before RED; do not approve a representation without a real round trip. |
| A pre-cutover object or mixed-version binary reaches the new contract | Controller errors, silent conversion, or unsafe execution | Coordinated breaking cutover; include AgentSession/AIA/RAR/WE in the clean inventory; no runtime conversion or legacy rollback. |
| A stale or foreign workflow supplies the schema | Validation against attacker-controlled or wrong content | Trusted namespace lookup, exact status ID, recomputed content-derived ID, exactly-one match, and fail-closed outcomes. |
| Numeric normalization or float64 decoding changes reviewed evidence | Audit non-repudiation and #2570 regression | Preserve submitted raw values in `parameter_evidence`; use number-safe decoding in create and batch audit paths; keep canonical comparison separate; reject unsafe numeric forms. |
| Live lookup race after successful verification | Workflow may be deleted immediately before dispatch | Document point-in-time limits; never claim atomic deletion prevention; ensure no-match before resource creation fails closed. |
| Audit type change breaks generated clients | Lost or unreadable compliance evidence | Update OpenAPI and generated Ogen code together; exercise real Data Storage write/readback and authorization tests. |

## 10. Historical estimate

These estimates were recorded before implementation and exclude unavailable
infrastructure recovery:

1. Contract/codegen spike and final approval: **0.5–1 day**.
2. RED unit and integration specifications: **2–3 days**.
3. GREEN types, live verification, wiring, and generated artifacts: **4–6 days**.
4. REFACTOR, breaking-cutover validation, E2E, compliance evidence, and regression gates:
   **2–4 days**.

Expected total: **8.5–14 engineer-days after approval**. The implementation has
now been completed locally; the estimate is retained for planning history.

## 11. Explicit non-goals

- No workflow switching or override of the selected content-derived workflow ID.
- No second schema registry, DS-owned mutable schema copy, or CRD schema snapshot.
- No general-purpose `map[string]any` in the normalized persisted CRD/audit
  parameter contract; internal LLM parsing may retain its raw-input DTO where
  required by #1172's boundary.
- No implicit default materialization.
- No secret transport through workflow parameters or a claim that audit text is
  automatically secret-redacted.
- No atomic delete barrier after a successful point-in-time lookup.
- No rewrite of ADR-076, DD-WORKFLOW-018, or #2570 documents in this planning
  change; an approved implementation requires a separate versioned amendment
  that records the #2575 supersession boundary.
- No runtime compatibility migration is part of this work. The operational
  active-object inventory and legacy-writer quiescence gate remain outside this
  branch.

## 12. Approved contract decisions and evidence

The owner approved the contract decisions below on 2026-11-08. The checks record
the approved implementation direction; blocked runtime evidence is tracked in
`TEST_PLAN.md` and is not represented as passing acceptance.

- [x] Approve the concrete wire representation for the required cross-CR
      normalization: the named structural discriminated union in
      `pkg/shared/types`, with no public `map[string]any` in persisted CRD/audit
      parameters.
- [x] Reject top-level `null` and object values; preserve explicit zero, false,
      empty string, and empty array values.
- [x] Keep #2570 integer lexical safety and raw submitted-value evidence; do not
      normalize numeric values in persisted CRDs/audit events.
- [x] Approve separate lossless audit parameter evidence (text/bytes preserving
      raw numeric lexemes) alongside semantic JSON/JSONB typed maps.
- [x] Approve the proposed canonical comparison encoding: sorted object keys,
      compact JSON, preserved array order, and no float64 integer conversion.
- [x] No backwards compatibility: use a coordinated breaking cutover with no
      dual-read, legacy drain, contract discriminator, or runtime conversion;
      active pre-cutover objects block activation through the cutover gate.
- [x] Approve content-derived `status.workflowId` as the sole in-flight revision
      fence, with no name/version fallback and only point-in-time race guarantees.
- [x] Approve the reconcile-state invariant and its explicit cleanup-only
      lifecycle exceptions in the inventory above.
- [x] Approve a new versioned ADR/DD amendment that records the #2575
      supersession of schema-copy transport fields without removing the
      #2570 approval-safety snapshot and terminal-decision guarantees.
- [x] Approve the wiring and test gates in [TEST_PLAN.md](TEST_PLAN.md).

**Implementation confidence: 95% for the local contract.** The repository paths,
typed union, identity verifier, executor behavior, audit schemas, generated
artifacts, and #2570 baseline are implemented and locally validated. Remaining
risk is limited to blocked production-path integration/RBAC/E2E evidence and the
deferred operational cutover inventory.
