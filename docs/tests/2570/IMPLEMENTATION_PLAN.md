# Implementation Plan: #2570 — RAR Parameter Overrides

**Status:** Proposed; awaiting user approval.

**Test plan:** [TP-2570](TEST_PLAN.md)

**Requirements:** BR-ORCH-025/026, BR-KA-191, BR-AUTH-001, BR-AUDIT-005/006.
**Related fixes:** #2577 (catalog namespace isolation), #2578 (DataStorage parameter helper).

## Scope and contract

- Only existing parameter **values** may be changed. Omitted keys retain their original values; `{}` is a no-op. New keys and changes to `TARGET_RESOURCE_*` are rejected. Reject a non-empty legacy `WorkflowOverride.WorkflowName` before committing a decision; workflow switching is out of scope.
- Store valid reviewer input exactly as entered in `map[string]string`. Validate against the full effective schema without rewriting: integer `1.0` is rejected; valid float `1.0` stays `1.0`. If processing produces a candidate value, its byte length and exact text must equal the reviewer-submitted override text, not the pre-existing RAR value; legitimate changed values are allowed.
- A nil original parameter map means data was not provided and blocks approval; `{}` means provided with no values and is valid when the schema allows it. Fix AIA's empty-map serialization and verify it with the generated CRD.
- Preserve the selected workflow, complete parameter schema, original parameters, confidence/rationale and provenance, and target/cluster safety context in the immutable RAR spec. Store the partial changes in status. RO creates WE from the admitted RAR, not a live AIA selection. Preserve existing absent/invalid remediation-target and RR fallback behavior.
- KA, AF and AW use one shared constraint core with service-specific adapters. AF receives the full schema; AW independently validates the complete effective map and enforces authorization. Fix #2578 without claiming its currently unused helper protects an existing production path.
- Optional schema defaults are review metadata: neither AW validation nor RO inserts an omitted value into the RAR changes, effective parameter map, WE or audit evidence. A downstream workflow may apply its own default; do not claim that it does without evidence. Test omission through the audit and execution handoff.
- Update the `WorkflowOverride.Parameters` API/CRD representation and its outdated replacement-semantics comments to preserve an explicitly submitted `{}` through AF→API-server serialization; omitted parameters and `{}` are both no-ops. A non-empty map is a partial overlay, not a replacement. Prove all three cases on the real admission path.
- Retain existing executor declaration filtering on all backends. Missing schema-derived declaration data fails closed; an explicit empty declaration map allows no parameter inputs. Show the reviewer which effective parameters will actually be consumed.
- **Out of scope:** workflow switching (#2572), editability metadata (#2571), typed transport (#2575), workflow-author configuration/default precision and general-purpose numeric normalization. No migration of old RARs is required. Preserve unrelated non-approval behavior.
- **Existing RARs:** Pending or approved RARs created without the required original selection or schema-derived declaration metadata fail closed with an observable reason; no approval or execution may proceed from missing data. No compatibility path, migration or pre-upgrade drain is required. Test pending and approved cases; ordinary decisions on complete RARs remain catalog-independent.

## Workflow identity, decisions and audit

- #2577 scopes KA workflow and ActionType discovery/list/watch to the trusted service namespace. AW override admission and WE dispatch perform a direct API-server lookup by `status.workflowId` in that namespace, recompute the content-derived ID, and require exactly one same-ID result. Never substitute live catalog values for the frozen snapshot. Ordinary AW decisions without overrides do not require catalog access.
- **Trusted namespace and RBAC:** Resolve the namespace from trusted service deployment configuration or the pod's Kubernetes service-account namespace; validate a non-empty namespace during KA, AW and WE bootstrap and fail closed if it cannot be established. Do not silently fall back to `default`, `kubernaut-system` or a cluster-wide scope. Deployment RBAC must grant the respective service accounts only the required catalog reads in this namespace: KA `list`/`watch` for workflows and ActionTypes, and AW and WE `list` on `remediationworkflows` for the selector-based lookups. Verify namespace-scoped Role/RoleBinding wiring and prohibit foreign-namespace matches. Changes to AW workflow-registration admission are not part of #2577 or this PR; this requirement applies to KA discovery and the AW/WE approval/execution lookups.
- **Selector prerequisite:** Declare `status.workflowId` under the RemediationWorkflow CRD version's `selectableFields`, regenerate both source and embedded CRDs, and pass real API-server selector test `IT-KA-2570-008` before relying on AW/WE lookup. This selector change is not part of the documentation-only PR and must be included or merged with the implementation.
- For a previously selected workflow, a successful API lookup with no matching ID fails closed immediately and is not retried; report that the workflow ID was not found. Ambiguity or an ID mismatch also fails closed. For transient API or network errors, use up to three total attempts with `pkg/shared/backoff` jitter, respecting context cancellation and admission timeout. Authorization/configuration failures are not blindly retried. A successful read is point-in-time; deletion before dispatch is not atomically prevented.
- First committed terminal decision wins. Use an explicit Kubernetes `resourceVersion` precondition. Report a conflict/already-decided outcome separately from a network failure. On an ambiguous transport failure, re-read before any bounded retry and never overwrite a committed decision. The RR-derived RAR name is unique by convention; no reviewer-facing RAR UID binding is required.
- **RAR creation collision:** RO creates the RR's single RAR. If the API server returns `AlreadyExists`, do not reuse the existing RAR: treat the conflict as suspected tampering, fail the investigation and RR with an observable reason, and route to manual review. A collision is not by itself proof of malicious intent. This deliberately includes a retry after RO successfully created the RAR but failed before recording its next step; do not silently adopt even that object. Distinguish transient API failures from `AlreadyExists`. If the RR has already entered the approval phase and its RAR is missing, fail and route to manual review instead of creating a replacement. No reviewer-facing RAR UID or reuse-provenance marker is required. This is detection and escalation, not prevention of CR creation or a guarantee that another controller cannot act on a pre-existing CR.
- Emit existing typed, correlated AF intent, AW admission and RO committed-decision audit evidence including actor and original/changed/effective values. Preserve user-entered text without sanitization or redaction, including where it is included in these records; use the existing authorization boundaries for RAR and audit readback. Do not assert that accidental secret input is automatically detected. Audit is fire-and-forget: expose enqueue failures and do not claim enqueue is durable persistence. Persistent buffering, synchronous flush gates and audit finalizers are out of scope.

## Selection integrity

The RAR spec is write-once and is the reviewed source of truth; do not add a separate selection SHA-256 digest. Unit-test the real RAR spec immutability rule in process. The general generated-CRD wiring test verifies installation; no dedicated spec-immutability integration test is required. Keep the existing workflow content-derived ID used for catalog lookup: it serves a different purpose from a selection digest. The selection field manifest and control cases are in [TP-2570](TEST_PLAN.md).

## Wiring manifest

| Production path | Proving integration test |
|---|---|
| KA metadata → AgentSession → AIA selection/schema → RO RAR creator | IT-AA-2570-001, IT-RO-2570-002 |
| KA bootstrap → trusted-namespace workflow and ActionType informer/discovery | IT-KA-2577-001/002 |
| KA validation → shared core → self-correction | IT-KA-2570-009 |
| AF MCP review/approval → full schema and shared validation | IT-AF-2570-003/010 |
| AW admission → shared validation, trusted ID lookup, target/key policy | IT-AW-2570-004/011 |
| Generated RAR CRD → immutable decision and override transitions | IT-AW-2570-005 |
| Approved RAR → RO target/lock/hash → WE → actual executor inputs | IT-RO-2570-006 |
| WE reconciliation → trusted workflow-ID check → dispatch | IT-WE-2570-012 |
| AF/AW/RO decision events → shared buffer → DataStorage readback | IT-AUD-2570-007, E2E-AUD-2570-008 |

**Mandatory cross-cutting wiring gates:** CHECKPOINT W also requires IT-2570-020
(trusted namespace and service-account RBAC, including AW/WE selector `list`)
and IT-2570-021 (pre-change RARs fail closed), and IT-2570-022 (RAR creation
conflict handling and manual-review routing) to pass. None may be
omitted when evaluating the affected KA, AW, RO or WE production paths.

## TDD and verification

1. **RED (1–2 days):** Add failing Ginkgo/Gomega unit scenarios and real API/admission integration tests for every wiring row. Prove failures before implementation.
2. **GREEN (3–5 days):** Implement #2577/#2578 and the snapshot, parameter preservation, shared validation, AF/AW, RO/WE, concurrency, audit and generated CRD/OpenAPI wiring. Pass affected UT and IT; complete CHECKPOINT W for every row.
3. **REFACTOR (0.5–1 day):** Remove duplicated validation/snapshot handling and improve error context without adding components. If no improvement is needed, mark N/A with a reason. Run post-refactor build and compile-only checks.
4. **Verify (1–2 days, excluding infrastructure recovery):** Pass all 41 critical scenarios in TP-2570, every wiring row, the successful/denied/missing-workflow end-to-end journeys and applicable control journeys; run affected regressions, `go build ./...`, `go test ./... -run=^$ -timeout=30s`, `golangci-lint run --timeout=5m`, relevant `make test`/integration/E2E targets and generated-artifact checks. Record unrelated baseline or unavailable infrastructure separately; do not claim unavailable tests passed.

Map applicable audit events to AU-2/AU-3 and SOC2 CC7.2, input validation to SI-10, authorization/namespace boundaries to AC-4/AC-6, and applicable ASVS objectives to proving cases. Assess AU-9/AU-11 using source-attributed existing evidence or scoped tests; do not infer retention duration or crash durability from an enqueued or persisted event. Roll back with a reviewed revert, never by discarding unrelated work.

Follow the pyramid invariant: UT proves meaningful business outcomes, IT proves every production wiring point, and E2E proves deployed journeys and applicable control objectives. Require at least 90% of defined in-scope business scenarios and 100% of critical scenarios in UT; add further tests only for legitimate business, safety or control cases, not to inflate coverage. Prefer Engram MCP for recall and code navigation; stop and escalate if the Engram MCP tools are disabled instead of silently switching tools.

**Planning confidence: 93%.** Local preflight exercised the workflow-ID selector with `IT-KA-2570-008` in `test/integration/kubernautagent/workflowcatalog/cache_test.go`, against the locally modified generated CRD; that selector change and test are not part of this documentation-only PR or yet merged. A disposable Kubernetes 1.37 envtest program at `/private/var/folders/r7/gktmmltd1zq7wqhsjjslwsm80000gn/T/opencode/issue2570-digest-crd-spike.go` directly copied one stored workflow configuration into AIA and read it back. That temporary sample is not a committed, reproducible CI test and does not prove the full KA→AIA→RAR path. The new RAR CRD and AW/RO/WE production paths are not yet implemented or proven. The catalog read/dispatch race and fire-and-forget audit limitations remain explicit. User approval is required before RED.
