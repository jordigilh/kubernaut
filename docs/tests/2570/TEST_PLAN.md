# Test Plan: #2570 RAR Parameter Overrides

**Status:** Proposed; awaiting user approval.

**Implementation:** [Implementation Plan](IMPLEMENTATION_PLAN.md)
**Scope:** BR-ORCH-025/026, BR-KA-191, BR-AUTH-001, BR-AUDIT-005/006; #2577 and #2578.

## Acceptance

Use Ginkgo/Gomega business-outcome tests. The 44 scenarios below are the proposed in-scope critical UT denominator: **0/44 implemented or passing**. All 44 must pass; add any consequential scenario found during implementation rather than weakening the denominator. Integration tests must pass for every wiring point, and E2E/control tests must prove each applicable objective. Structural line coverage is diagnostic; follow the separate merged-tier CI gate. No skipped or pending executable tests.

The pyramid invariant is mandatory: UT proves business logic, IT proves real production wiring, and E2E proves the deployed user journey. Across all defined in-scope UT business scenarios require at least 90% passing and 100% of critical scenarios passing. Because all 44 current scenarios are critical, the current gate is 44/44, not 90% of 44. If legitimate non-critical cases are added, include them in the overall 90% denominator while keeping every critical case mandatory. Higher meaningful coverage is preferred when justified by additional business cases; do not add duplicate or coverage-only tests. Use Engram MCP for recall and code navigation; if disabled, stop and escalate.

## Unit scenarios

| # | ID | Observable business outcome | Planned package / requirement |
|---|---|---|---|
| 1 | UT-RO-2570-076 | Frozen snapshot survives later AIA changes; WE uses the admitted selection. | RO creator; ORCH-025 |
| 2 | UT-AA-2570-067 | Missing original parameter data blocks approval. | AIA/RO creator; ORCH-025 |
| 3 | UT-AA-2570-068 | Explicit empty original map remains empty when schema permits. | AIA/RO creator; ORCH-025 |
| 4 | UT-RAR-2570-002 | The real RAR spec immutability rule rejects a change to the frozen selection. | in-process RAR/CEL validation; ORCH-025 |
| 5 | UT-AW-2570-001 | Partial edit changes submitted existing key and preserves omitted keys. | AW/shared validation; ORCH-025 |
| 6 | UT-AW-2570-003 | Empty reviewer override changes no values. | AW; ORCH-025 |
| 7 | UT-AW-2570-004 | A new key is rejected without a partial decision. | AW; KA-191 / SI-10 |
| 8 | UT-AW-2570-005 | A changed managed target key is rejected. | AW; KA-191 / AC-4 |
| 9 | UT-AW-2570-066 | Integer `1.0` is rejected, never rewritten. | shared validation/AW; KA-191 |
| 10 | UT-AW-2570-065 | Valid float `1.0` is retained exactly. | shared validation/AW; KA-191 |
| 11 | UT-VAL-2570-077 | A processed candidate differing in length or text from the reviewer-submitted value is rejected; a changed override preserved exactly from the submission is accepted. | shared validation/AW; KA-191 / SI-10 |
| 12 | UT-AW-2570-078 | Boolean text is accepted or rejected according to its schema. | shared validation; KA-191 / SI-10 |
| 13 | UT-AW-2570-079 | A missing required effective parameter is rejected. | shared validation; KA-191 / SI-10 |
| 14 | UT-AW-2570-015 | Malformed schema fails closed. | shared validation/AW; KA-191 / SI-10 |
| 15 | UT-VAL-2570-041 | Integer above 2^53 survives KA/AIA handoff without rounding. | KA/AIA parsing; KA-191 |
| 16 | UT-KA-2577-033 | Trusted-namespace discovery excludes foreign workflows. | KA catalog; WORKFLOW-006 / AC-4 |
| 17 | UT-AW-2570-017 | Missing or changed same-ID trusted workflow rejects an override. | AW; AUTH-001 / SI-10 |
| 18 | UT-AW-2570-019 | Ordinary no-override decision is independent of catalog availability. | AW; AUTH-001 |
| 19 | UT-WE-2570-060 | Missing workflow at dispatch produces no external execution. | WE reconciler; ORCH-025 / AC-4 |
| 20 | UT-AF-2570-024 | Unauthorized or forged reviewer is denied. | AF/AW; AUTH-001 / AC-6 |
| 21 | UT-AF-2570-025 | First concurrent decision wins; the other cannot overwrite it. | AF/AW; AUTH-001 / AC-6 |
| 22 | UT-AW-2570-021 | Terminal decision and override cannot be modified or removed. | AW/RAR; AUTH-001 / SI-10 |
| 23 | UT-RO-2570-027 | Absent remediation target preserves the frozen RR fallback behavior. | RO target/WE creator; ORCH-026 / AC-4 |
| 24 | UT-AUD-2570-030 | Decision audit distinguishes original, changed and effective values and actor. | RO audit; AUDIT-005/006 / AU-3 / CC7.2 |
| 25 | UT-AUD-2570-031 | Enqueue failure is observable and not reported as durable success. | RO audit; AUDIT-006 / AU-2 |
| 26 | UT-VAL-2570-080 | A string is preserved as entered and constrained by its schema. | shared validation; KA-191 / SI-10 |
| 27 | UT-VAL-2570-081 | Array input accepts JSON arrays and rejects non-arrays. | shared validation; KA-191 / SI-10 |
| 28 | UT-VAL-2570-082 | Numeric values outside schema bounds are rejected. | shared validation; KA-191 / SI-10 |
| 29 | UT-VAL-2570-083 | Values outside a declared enum are rejected. | shared validation; KA-191 / SI-10 |
| 30 | UT-VAL-2570-084 | Values violating a declared pattern are rejected. | shared validation; KA-191 / SI-10 |
| 31 | UT-VAL-2570-085 | A present parameter with a missing dependency is rejected. | shared validation; KA-191 / SI-10 |
| 32 | UT-KA-2577-034 | Trusted-namespace discovery excludes foreign ActionTypes. | KA catalog; WORKFLOW-007 / AC-4 |
| 33 | UT-RO-2570-028 | A valid frozen remediation target drives safety and execution identity. | RO target/WE creator; ORCH-026 / AC-4 |
| 34 | UT-DS-2578-001 | DataStorage helper rejects malformed typed parameter values. | DataStorage models; KA-191 / SI-10 |
| 35 | UT-RO-2570-086 | A conflicting pre-created RAR is not reused; suspected tampering fails the investigation and RR and routes to manual review. | RO creator/reconciler; ORCH-026 / AC-6 |
| 36 | UT-AW-2570-087 | Non-empty legacy `workflowName` is rejected before decision or execution. | AW; ORCH-025 / SI-10 |
| 37 | UT-VAL-2570-088 | Omitted optional parameter with a schema default remains absent in the effective map; no default is silently inserted. | shared validation/RO; KA-191 |
| 38 | UT-RO-2570-089 | After a successful RAR create but before the next step is recorded, an RO retry receives `AlreadyExists` and fails closed to manual review rather than adopting the RAR. | RO creator/reconciler; ORCH-026 / AC-6 |
| 39 | UT-VAL-2570-090 | A non-finite float value is rejected. | shared validation; KA-191 / SI-10 |
| 40 | UT-VAL-2570-091 | A float beyond the finite float64 range is rejected as overflow. | shared validation; KA-191 / SI-10 |
| 41 | UT-VAL-2570-092 | A nonzero float that underflows to zero is rejected. | shared validation; KA-191 / SI-10 |
| 42 | UT-VAL-2570-093 | Integers above signed int64 maximum or below its minimum are rejected without a tighter schema bound. | shared validation; KA-191 / SI-10 |
| 43 | UT-AW-2570-094 | An override on Pending, Rejected or Expired is rejected with RAR status unchanged. | AW; AUTH-001 / SI-10 |
| 44 | UT-AW-2570-095 | A rationale-only Approved override retains the original effective parameter map. | AW; ORCH-025 / KA-191 |

## Integration and end-to-end proof

| ID | Production path and expected evidence | Environment |
|---|---|---|
| IT-AA-2570-001 / IT-RO-2570-002 / BR-ORCH-025/026 | KA selection/schema → AgentSession/AIA → RAR; typed nil/empty maps, frozen selection, confidence, rationale and provenance survive generated CRDs. WE confidence/rationale and audit provenance use the frozen values. | Real Kubernetes API server and generated CRDs. |
| IT-KA-2577-001/002 / BR-WORKFLOW-006/007 | KA bootstrap list/lookup/watch excludes foreign workflows and ActionTypes, including same-name/ID objects. | Real API server, two namespaces. |
| IT-2570-020 / BR-WORKFLOW-006/007, BR-AUTH-001 | KA, AW and WE use their configured trusted namespace; missing namespace fails closed, namespace-scoped Role/RoleBinding grants required access including AW/WE `list` on `remediationworkflows`, and foreign catalog objects cannot satisfy lookup. | Real API server with service identities and at least two namespaces. |
| IT-2570-021 / BR-ORCH-025/026 | Pending and already-approved pre-change RARs lacking required selection/declaration data fail closed with an observable reason; no execution is created. | Real API server and RO/AW integration. |
| IT-2570-022 / BR-ORCH-026, BR-AUTH-001 | A pre-existing RAR causes `AlreadyExists`; RO does not reuse it and fails/escalates the RR, including on a retry after a prior create succeeded but its next step was not recorded. A RAR missing after the RR enters approval is not recreated and causes the same safe outcome. Transient API failures remain distinct. This does not claim to prevent CR creation or independent controller actions. | Real API server and RO reconciliation. |
| IT-KA-2570-008 / BR-WORKFLOW-006 | Generated RemediationWorkflow CRD supports `status.workflowId` server-side selection within a namespace and returns the matching workflow. | Real Kubernetes API server with generated CRD. |
| IT-KA-2570-009 / BR-KA-191 | Production KA validation invokes shared core and returns actionable self-correction. | KA integration with schema fixture. |
| IT-AF-2570-003/010 / BR-KA-191, BR-AUTH-001 | Actual MCP read/approve exposes full schema; omitted override, explicit `{}` and partial override survive the AF→API-server path without clearing omitted values; a rationale-only Approved override preserves the effective map; omitted optional defaults stay absent in WE and audit evidence. | AF, API server, authorization, webhook and execution/audit fixtures. |
| IT-AW-2570-004/011 / BR-AUTH-001, BR-KA-191 | Real AW admission enforces same-ID lookup, rejects multiple same-ID matches, schema violations, locked-key changes and processing that alters reviewer-submitted text; rejects a non-empty legacy workflowName and overrides on Pending/Rejected/Expired with status unchanged; and covers a rationale-only Approved override. Verify no retry on a successful no-match, 403 or invalid namespace; cancellation/deadline stops retries and transient API/network errors use at most three total attempts. | API server, webhook, trusted/foreign namespaces. |
| IT-AW-2570-005 / BR-AUTH-001 | Generated RAR CEL blocks terminal mutation and whole-status removal while permitting observation updates; webhook bypass included. | API server with generated RAR CRD. |
| IT-AF-2570-012 / BR-AUTH-001 | Concurrent approve/reject updates: exactly one commits; conflict differs from network error; ambiguous failures re-read before retry. | Actual AF/AW path and API server. |
| IT-RO-2570-006 / BR-ORCH-025/026 | Approved RAR → frozen target/lock/hash → WE → actual filtered inputs for Job, Tekton and Ansible; nil declaration metadata fails closed and explicit empty remains empty and passes no inputs. | RO/WE integration, real API and executor fixtures. |
| IT-WE-2570-012 / BR-ORCH-025 | WE trusted-ID lookup on approval and non-approval execution: same-ID dispatch succeeds; a previously selected ID with no match fails immediately after one lookup; ambiguous, stale/mismatched or foreign-only result never dispatches. Transient API/network failures make at most three total attempts; 403 or invalid namespace configuration is not retried, and cancellation/deadline stops retries. | API server and backend fixtures. |
| IT-AUD-2570-007 / BR-AUDIT-005/006, BR-AUTH-001 | RO decision → shared buffer → DataStorage HTTP/PostgreSQL; healthy authorized correlated readback preserves included user text, unauthorized RAR/audit reads are denied, and enqueue failure is observable. | Real service and isolated database with authorized/unauthorized identities. |
| E2E-AUD-2570-008 / BR-AUDIT-005/006 | Authenticated review → admitted RAR → committed decision → correlated audit reconstruction; execute applicable safety and denial journeys. | Deployed AF, AW, RO, WE and DataStorage with TLS. |

### End-to-end business journeys

These are required E2E tests of deployed components, not substitutes for unit or
integration tests. Use an execution fixture that records the actual inputs
received by its workload; observing only the WE spec does not prove execution.

| ID | Journey and required outcome | Requirements / objectives |
|---|---|---|
| E2E-2570-001 | Create an approval-required RR with a selected workflow and at least two original parameters. An authorized reviewer changes one valid value through AF; AW admits it; RO creates WE from the RAR; WE dispatches the workflow. Verify the workload receives the exact changed text and the untouched value, and the RR completes. | ORCH-025/026, KA-191, AC-4, SI-10 |
| E2E-2570-002 | On a fresh pending RAR, submit an invalid or unauthorized parameter change through AF. Verify rejection, unchanged decision, and no WE or external execution. | AUTH-001, KA-191, AC-6, SI-10 |
| E2E-2570-003 | On a fresh approved RAR, use an isolated controlled barrier to remove the matching trusted workflow before WE's runtime check, then release reconciliation. Verify WE fails closed and creates no external execution resource; do not rely on timing or sleeps. | ORCH-025, AC-4 |

E2E-AUD-2570-008 additionally proves correlated intent, admission and committed
decision readback under healthy delivery. Each journey uses isolated fixtures and
the actual service path. None is implemented or passing evidence yet.

The existing WE workflow-deletion E2E test must be updated: a missing same-ID workflow before dispatch now prevents a new execution. Test the separate point-in-time race without claiming atomic deletion prevention. A previously running execution is not canceled merely because a later catalog check fails.

## Frozen selection fields

The frozen selection includes `workflow`, `parameters`, `parameterSchema` and `targetContext`. The workflow snapshot includes `workflowId`, `workflowName`, `actionType`, `version`, `executionBundle`, `executionBundleDigest`, `executionEngine`, `engineConfig`, `serviceAccountName`, `executionClusterId`, `dependencies`, `resources` and `declaredParameterNames`. Schema entries include `name`, `type`, `required`, `description`, `enum`, `pattern`, `minimum`, `maximum`, `default` and `dependsOn`. Target context includes remediation target, signal target and signal cluster ID; each target includes `apiVersion`, `kind`, `namespace`, `name`. Selected-workflow confidence stays in existing RAR `spec.confidence`, rationale in `spec.recommendedWorkflow.rationale`; provenance uses existing `spec.remediationRequestRef` and `spec.aiAnalysisRef` plus frozen workflow ID/name/version. Preserve explicit empty maps and the original stored values. The workflow bundle digest is an existing execution field, not a selection digest.

Test actual AIA/RAR CRD round trips and exact reviewer strings. No selection SHA-256 digest, general numeric-value normalization or workflow-author input validation is in scope.

## Control evidence and environment

- **SI-10 / ASVS input validation:** invalid types, constraints or keys do not commit a decision; AW is authoritative.
- **AC-4 / AC-6:** unauthorized or forged callers are denied; namespace isolation and target safety hold; denied actions do not execute.
- **AU-2 / AU-3 / SOC2 CC7.2:** under healthy delivery, query correlated intent, admission and committed decision with actor and original/changed/effective values, preserving user-entered text without sanitization or redaction. Verify unauthorized callers cannot read protected RAR/audit data; no automatic secret-detection guarantee is implied.
- **AU-9 / AU-11:** use applicable existing integrity, retention and hold tests or source-attributed inherited evidence. No seven-year passage or crash-safe delivery is inferred from a stored event.
- **ASVS 5.0 V16.2/V16.3:** assess relevant correlation, sensitive logging, denied authorization and error visibility with the actual configured log processor; mark missing evidence unverified rather than claiming compliance.

| Objective | Planned proving behavior |
|---|---|
| SI-10 and ASVS input validation | UT validation scenarios, IT-AW-2570-004/011 and E2E-2570-002 reject invalid input before a committed decision. Verify the precise applicable ASVS 5.0 clause against the standard before reporting it. |
| AC-4/AC-6 and ASVS access control | IT-KA-2577-001/002, IT-AF-2570-003 and E2E-2570-002/003 prove namespace/authorization boundaries and absence of execution on denial. |
| AU-2/AU-3, SOC2 CC7.2 and ASVS 5.0 V16.2/V16.3 | IT-AUD-2570-007 and E2E-AUD-2570-008 prove correlated actor, decision and safe readback under healthy delivery; test denial/error logging through the actual configured processor. |
| AU-9/AU-11 | Identify source-attributed inherited evidence or run applicable integrity, retention and legal-hold tests against the supported DataStorage configuration; if unavailable, record as unverified and block the applicable control gate. |

Use isolated resources. Mock only external dependencies; use real internal validation, API server and webhook for wiring, and real DataStorage/DB for audit delivery. A missing environment blocks the corresponding gate. Run affected unit/integration/E2E suites, build, lint, artifact generation and post-refactor checks specified in the implementation plan. Record results and unrelated baseline failures separately.
