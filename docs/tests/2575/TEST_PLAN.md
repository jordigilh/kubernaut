# Test Plan: #2575 Typed Workflow Parameter Contract

**Test Plan Identifier:** TP-2575-v1.1
**Feature:** Typed workflow parameter transport with the `RemediationWorkflow`
CRD as the sole schema authority
**Version:** 1.1
**Created:** 2026-11-06
**Author:** Kubernaut development team
**Status:** Implementation complete — local evidence passes; production-path acceptance pending
**Branch:** `plan/2575-parameter-contract`

**Implementation plan:** [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md)

---

## 1. Introduction

### 1.1 Purpose

This approved plan defines the tests for replacing string-valued workflow parameters with
one normalized typed JSON representation across AIA, RAR, and WE while removing
parameter-schema copies from those and other persisted CRDs. It proves that
every schema-dependent operation reads and validates the exact trusted
`RemediationWorkflow` content identified by the content-derived
`status.workflowId`, preserves #2570 approval safety, converts values correctly
for each execution engine, and reconstructs correlated audit evidence without
embedding another schema authority.

This plan records the approved contract and the evidence available in the current
working tree. `Pass` is limited to the named local or disposable evidence;
`Blocked` means the required production-path environment was unavailable;
`Deferred` means the scenario is an operational gate outside this code change.
Neither `Blocked` nor `Deferred` is passing acceptance evidence.

### 1.2 Objectives

1. Prove AIA, RAR, and WE use the same typed parameter representation and
   presence contract across generated CRD round trips.
2. Prove all typed value and presence semantics, including explicit zero,
    false, empty string, empty array, null, absent, and empty-map behavior.
3. Prove live schema authority and content-derived revision fencing at KA, AIA,
   RO, AF, AW, and WE boundaries, including every schema-dependent reconcile
   state before it completes a transition or creates a resource.
4. Prove typed engine output for Job, Tekton, and Ansible without heuristic
    string coercion or schema-derived CRD snapshots.
5. Prove actor-attributed, correlated, typed audit readback and safe failure
    when audit enqueue or authorization fails.
6. Prove breaking-cutover rejection behavior and zero regression against the #2570/
    #2601 approval, locking, collision, and fail-closed safety baseline.

### 1.3 Success metrics

| Metric | Target | Measurement |
|---|---:|---|
| Critical unit scenarios | 100% | Passing scenarios in Section 8 against the declared denominator |
| All in-scope unit scenarios | ≥90% | Requirements-based scenario coverage; structural coverage is diagnostic |
| Wiring integration rows | 100% | Section 14 wiring matrix and real production-path tests |
| Applicable control objectives | 100% | IT/E2E evidence for SI-10, AC-4, AC-6, AU-2, AU-3, AU-9, AU-11, CC7.2 |
| Cross-CR representation drift | 0 | AIA/RAR/WE generated CRD round trips and production handoff assertions |
| Typed transport regressions | 0 | Existing affected tests plus #2570/#2601 regression set |
| Schema copies in persisted CRDs | 0 | Generated CRD inspection and API-server round-trip assertions |
| External execution on invalid/stale input | 0 | WE tests inspect Job/PipelineRun/AWX calls/resources |

---

## 2. References

### 2.1 Authority

- [BR-WORKFLOW-004](../../requirements/BR-WORKFLOW-004-workflow-schema-format.md)
- [BR-WORKFLOW-006](../../requirements/BR-WORKFLOW-006-remediation-workflow-crd.md)
- [BR-KA-191](../../requirements/BR-KA-191-workflow-parameter-validation.md)
- [BR-ORCH-025/026](../../requirements/BR-ORCH-025-026-workflow-approval-orchestration.md)
- [BR-WE-015](../../requirements/BR-WE-015-ansible-execution-engine.md)
- [BR-AUDIT-005 v2.0](../../requirements/11_SECURITY_ACCESS_CONTROL.md#411-comprehensive-audit-logging)
- [BR-AUDIT-006](../../requirements/BR-AUDIT-006-remediation-approval-audit-trail.md)
- [BR-SECURITY-016](../../requirements/BR-SECURITY-016-kubernetes-rbac-enforcement.md)
- Issue [#2575](https://github.com/jordigilh/kubernaut/issues/2575)

### 2.2 Cross-references

- [#2570 implementation plan](../2570/IMPLEMENTATION_PLAN.md)
- [#2570 test plan](../2570/TEST_PLAN.md)
- [#1172 — prior string-transport normalization proposal](https://github.com/jordigilh/kubernaut/issues/1172)
- [ADR-076 — RAR approval snapshot](../../architecture/decisions/ADR-076-rar-approval-snapshot-and-terminal-decision-contract.md)
- [DD-WORKFLOW-018 — etcd catalog authority](../../architecture/decisions/DD-WORKFLOW-018-etcd-single-source-of-truth.md)
- [Test-plan template](../../testing/TEST_PLAN_TEMPLATE.md)
- [DD-AUDIT-003](../../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md)
- [ADR-034](../../architecture/decisions/ADR-034-unified-audit-table-design.md)

ADR-076, DD-WORKFLOW-018, and #2570 documents are references only in this
planning change. They are not modified here.

---

## 3. Risks and mitigations

| ID | Risk | Impact | Probability | Affected tests | Mitigation |
|---|---|---|---|---|---|
| R1 | API-server/codegen handling of raw JSON map values is not structural | Values are pruned, rejected, or read back with a different shape | Medium | UT-TYP-2575-001, IT-CRD-2575-001 | Require a disposable codegen/envtest round trip before implementation proceeds. |
| R2 | A stale, foreign, duplicate, or modified workflow supplies the schema | Validation can authorize the wrong parameters or revision | High | UT-ID-2575-011/012, IT-AW-2575-007, IT-WE-2575-011, E2E-WE-2575-003 | Exact trusted-namespace lookup, one-match rule, content-derived ID recomputation, fail closed. |
| R3 | Cutover starts while a pre-cutover object or legacy writer is still active | Values may be silently converted or unsafe execution may occur | High | UT-CUT-2575-013/014, IT-CUT-2575-013 | Quiesce legacy writers/controllers, inventory active AgentSession/AIA/RAR/WE objects, and block the release when the cutover precondition is not met. |
| R4 | Numeric canonicalization or JSONB storage changes reviewed evidence | Audit loses operator intent or #2570 exact-input guarantees regress | High | UT-TYP-2575-004/007, UT-AUD-2575-018, IT-AUD-2575-012 | Preserve raw numeric tokens in separate text/byte evidence; canonicalize only for deterministic comparison; never treat JSONB as lexical evidence. |
| R5 | Audit/OpenAPI generation is updated incompletely | Typed decisions cannot be persisted or reconstructed | High | UT-AUD-2575-018/019, IT-AUD-2575-012, E2E-AUD-2575-004 | Update source OpenAPI and generated clients together; query real Data Storage. |

### 3.1 Risk-to-test traceability

High-risk behavior has local unit, compile, or disposable-spike evidence where
available. Production-path integration and E2E gaps remain explicitly listed in
Sections 9, 10, 13, and 14. A missing environment is a blocked gate, not a
passing result.

---

## 4. Scope

### 4.1 Features to be tested

- **Typed parameter contract** (`api/*`, shared typed-parameter package): raw
  JSON union values, presence semantics, numeric safety, canonical comparison,
  and no `map[string]any` in the persisted CRD/audit parameter contract.
- **Raw LLM ingress fidelity** (KA parser and AgentSession handoff): preserve
  numeric tokens before typed-value construction and `parameter_evidence`
  capture; this tests an input boundary, not a persisted LLM DTO authority.
- **Cross-CR normalization** (AIA, RAR, WE, AgentSession and generated assets):
  one typed parameter representation for selected, original, override, and
  effective values, with explicit absence/empty semantics preserved.
- **Live workflow authority** (`pkg/shared/workflowidentity`, KA, AIA, RO,
  AF, AW, WE): exact trusted workflow lookup and schema validation.
- **CRD transport** (AIA, RAR, WE, AgentSession and generated assets): typed
  round trips, explicit empty preservation, and zero schema copies.
- **Approval path** (`pkg/apifrontend`, `pkg/authwebhook`, RO): typed original,
  override, effective values and all #2570 safety rules.
- **Execution path** (`internal/controller/workflowexecution` and executors):
  live-schema filtering and typed Job/Tekton/Ansible conversion.
- **Audit path** (RO/AF/AW/WE and Data Storage): typed correlated readback,
  actor attribution, error details, authorization, and enqueue observability.
- **Breaking rollout follow-up**: coordinated typed-only cutover with a clean
  active-object gate; no runtime compatibility conversion. The inventory and
  quiescence gate is operational follow-up, not implemented in this branch.

### 4.2 Features not to be tested

- New workflow selection algorithms, workflow switching, or editability
  metadata; those remain separate issues.
- A new schema registry, DS-owned mutable workflow schema, or database catalog.
- Automatic secret detection/redaction of parameter or rationale text.
- Atomic prevention of workflow deletion after a successful point-in-time read.
- Seven-year time passage or crash durability as evidence of AU-9/AU-11.
- Changes to ADR-076, DD-WORKFLOW-018, or the #2570 documents.

### 4.3 Design decisions under test

| Decision | Rationale |
|---|---|
| AgentSession/AIA/RAR/WE share one named structural typed union, not `map[string]string` or persisted `map[string]any` | Removes representation drift while preserving JSON types without exposing an unbounded public Go union; internal LLM/tool parsing DTOs remain ordinary JSON outside the persisted contract, while their number-safe input boundary remains tested. |
| Workflow CRD is the only schema authority | Prevents AIA/RAR/WE schema drift and makes revision identity verifiable. |
| Content-derived `status.workflowId` is the revision fence | A spec change yields a new identity; name/version fallback could accept stale content. |
| Raw submitted values plus separate canonical comparison encoding | Preserves #2570 audit evidence while making map equality deterministic. |
| Coordinated breaking cutover | Legacy writers/controllers are quiesced and active pre-cutover objects block the release; immutable RAR/WFE objects are not rewritten or converted at runtime. |

---

## 5. Approach

### 5.1 Coverage policy

- **Unit:** Ginkgo/Gomega BDD, all 21 in-scope critical implementation scenarios
  in Section 8 must pass. The two cutover-inventory rows remain in the approved
  plan as owner-dispositioned operational gates and are excluded from the unit
  denominator; any legitimate additional implementation scenario increases the
  denominator and cannot be removed to improve the percentage.
- **Integration:** Ginkgo/Gomega with real API-server/envtest, generated CRDs,
  webhook/controller dispatch, and real Data Storage for audit persistence.
  Every wiring row must pass.
- **E2E:** Ginkgo/Gomega on the deployed full pipeline. Prove the user journey,
  external execution result, access-control boundary, and audit reconstruction.
- Structural line coverage is diagnostic. The merged all-tier gate remains the
  repository CI policy; it is not substituted for requirements coverage.

### 5.2 Two-tier minimum

Every in-scope business requirement has unit and integration coverage. E2E is
required for the end-to-end execution, authorization, and audit control
objectives. Unit tests do not count as wiring proof.

### 5.3 Mock policy

- Mock only external boundaries: LLM, AWX/Ansible server, and external network
  failures where a real service is not available.
- Use real internal typed validation, workflow identity, API-server objects,
  generated CRDs, webhook/controller code, executor builders, and audit models.
- Data Storage audit tests use real PostgreSQL/HTTP persistence and an
  independent readback client; an in-memory event capture is not the result
  oracle.

### 5.4 Pass/fail criteria

**Final PASS** requires:

1. All 21 in-scope P0 unit scenarios pass; all in-scope critical scenarios
   remain 100% passing.
2. At least 90% of the final requirements-based unit denominator passes.
3. Every wiring row and applicable control objective has passing IT/E2E proof.
4. No existing affected test regresses, including the #2570/#2601 safety set.
5. No invalid, stale, foreign, or unverifiable workflow reaches an external
   execution resource.
6. Real audit readback reconstructs typed original/override/effective values,
   actor, correlation, and workflow content identity.

**FAIL** includes any P0 failure, schema-copy persistence, silent coercion,
missing actor/correlation, authorization fail-open, a cutover proceeding with
active pre-cutover objects, or any external execution after a failed
identity/schema/contract check.

### 5.5 Suspension and resumption

Suspend when the code does not compile, generated CRDs are unavailable, the
trusted API-server test environment cannot start, real Data Storage/audit
readback is unavailable, or more than three failures share one unknown root
cause. Resume after the dependency is restored or the root cause is isolated.
Do not mark unavailable tests as passed.

---

## 6. Test items

### 6.1 Unit-testable code

| File/package | Functions or behavior | Approximate scope |
|---|---|---|
| `pkg/shared/types/typed_parameter.go` and `typed_parameter_contract_test.go` | typed union marshal/unmarshal, presence, numeric safety, evidence, and engine conversion | Implemented; affected local suite passes |
| `pkg/shared/workflowidentity/verify.go` and WE identity callers | exact lookup, content hash/UUID check, and error classification | Implemented; controller/unit paths pass; dedicated production IT remains blocked |
| `pkg/remediationapprovalrequest/parameters.go` | live-schema effective-map validation and override policy | Implemented; affected local suite passes |
| `pkg/aianalysis/handlers/` | typed selection parsing and schema-copy removal | Implemented; affected local suite passes |
| `pkg/workflowexecution/executor/` | filtering and Job/Tekton/Ansible conversion | Implemented; affected local suite passes, including mixed engine-string coverage |
| audit builders and OpenAPI adapters | typed payload and presence mapping | Implemented; local/repository precision tests pass; production readback remains blocked |

### 6.2 Integration-testable code

| Production path | Test environment | Current evidence |
|---|---|---|
| AIA/RAR/WE CRD serialization | envtest API server and generated CRDs | Disposable Kubernetes 1.37 round trip passed; current runtime rerun blocked by Podman storage |
| KA → AgentSession → AIA | real CRDs and production adapters | Focused local unit/compile evidence passes; production IT blocked |
| AF → API server → AW | real MCP/API/webhook path | Focused local unit/compile evidence passes; production IT blocked |
| RO → RAR → WE | real controller clients and status transitions | Affected tests compile and local suite passes; production IT blocked |
| WE → executor | real controller and Job/Tekton builders; external AWX boundary isolated | Executor unit coverage passes; production IT blocked |
| audit → Data Storage → query | real authenticated HTTP/PostgreSQL | Repository precision test and disposable PostgreSQL spike passed; current production readback IT blocked |
| trusted namespace/RBAC | real service accounts, Roles/RoleBindings, and API server | Not proven in this branch; operator RBAC work is delegated to #624 |

### 6.3 Version identification

| Item | Version/commit | Notes |
|---|---|---|
| Code under test | `plan/2575-parameter-contract` working tree | Implementation present; changes remain uncommitted and unpushed |
| Go | `1.26.10` | From `go.mod` |
| Ginkgo | `v2.33.1` | From `go.mod`; verify at implementation time |
| Existing safety baseline | #2570/#2601 and #2589-related tests | Must remain green; no inferred evidence |

### 6.4 Current local evidence snapshot

| Gate | Evidence | Result and limitation |
|---|---|---|
| Build | `go build ./...` | Pass |
| Compile-only regression | `go test ./... -run '^$'` | Pass |
| Lint | `make lint` | Pass; 0 issues |
| Generation | `make generate`, repeated generation comparison | Pass; generated CRDs/OpenAPI/Ogen assets are stable |
| Affected behavior | Shared types, AIA, AgentSession, AF, AW, RAR, RO, WE, audit, and executor suites | Pass locally |
| Repository test target | `make test` | Exit 0; one unrelated DataStorage flake and two unrelated kubeconfig-dependent skips are recorded separately |
| Runtime integration/E2E | envtest/controller, RBAC, deployed pipeline, and production Data Storage readback | Blocked or not re-run; prior disposable spike evidence is not production certification |

---

## 7. BR coverage matrix

| BR ID | Business outcome | Priority | Tier | Test ID | Status |
|---|---|---|---|---|---|
| BR-WORKFLOW-004 | Values are validated against live CRD parameter constraints | P0 | Unit | `UT-VAL-2575-008` | Pass locally; production IT remains blocked |
| BR-WORKFLOW-004 | No persisted component creates a second schema authority | P0 | Integration | `IT-CRD-2575-001` | Disposable CRD proof passed; current runtime IT blocked |
| BR-WORKFLOW-005 | Float values remain finite and typed | P0 | Unit/IT | `UT-TYP-2575-005`, `IT-WE-2575-010` | Unit pass; production IT blocked |
| BR-WORKFLOW-006 | Trusted CRD content and status ID fence workflow identity | P0 | Unit/IT | `UT-ID-2575-011`, `IT-AW-2575-007`, `IT-INV-2575-016` | Controller/unit paths pass; production IT blocked |
| BR-KA-191 | KA validates typed selection and self-corrects before handoff | P0 | Unit/IT/E2E | `UT-VAL-2575-008`, `IT-KA-2575-002`, `E2E-KA-2575-001` | Unit pass; IT/E2E blocked |
| BR-ORCH-025 | Typed original values survive AIA → RO → WE | P0 | Unit/IT/E2E | `UT-RO-2575-021`, `IT-RO-2575-004/008`, `E2E-KA-2575-001` | Local regression pass; IT/E2E blocked |
| BR-ORCH-026 | Approved typed override changes only allowed existing keys | P0 | Unit/IT/E2E | `UT-VAL-2575-010`, `IT-AF-2575-005`, `E2E-AF-2575-002` | Local approval-safety pass; IT/E2E blocked |
| BR-WE-015 | Ansible receives typed `extra_vars` without heuristic coercion | P0 | Unit/IT/E2E | `UT-WE-2575-016`, `IT-WE-2575-010`, `E2E-WE-2575-006` | Unit pass; IT/E2E blocked |
| BR-AUDIT-005 | Correlated typed decision evidence is queryable and reconstructable | P0 | Unit/IT/E2E | `UT-AUD-2575-018/019`, `IT-AUD-2575-012`, `E2E-AUD-2575-004` | Local/repository and disposable-DB pass; production IT/E2E blocked |
| BR-AUDIT-006 | Approval actor, rationale, original/override/effective values are captured | P0 | Unit/IT/E2E | `UT-AUD-2575-018`, `IT-AUD-2575-012`, `E2E-AUD-2575-004` | Local payload pass; production IT/E2E blocked |
| BR-SECURITY-016 | Unauthorized/foreign access and authorization-service errors fail closed | P0 | Unit/IT/E2E | `UT-SEC-2575-022`, `IT-SEC-2575-014`, `E2E-SEC-2575-005` | Unit fail-closed paths pass; RBAC IT/E2E blocked |
| #2570/#2601 baseline | Approval, locking, concurrency, collision, and no-default safety remains intact | P0 | Unit/IT/E2E | `UT-REG-2575-023`, `IT-REG-2575-015`, `E2E-AF-2575-002` | Affected local regression pass; IT/E2E not re-run |

---

## 8. Test scenarios — unit

**Critical unit denominator:** 21 in-scope implementation scenarios. Every
in-scope scenario below is P0. `UT-CUT-2575-013` and `UT-CUT-2575-014` remain
listed for traceability but are deferred operational gates, not implementation
scenarios in this branch.

| ID | Observable business outcome | BR/control | Phase |
|---|---|---|---|
| `UT-TYP-2575-001` | String, integer, float, boolean, and array values survive a CRD JSON round trip as their declared JSON types; no persisted CRD/audit `map[string]any` is required. | BR-WORKFLOW-004, SI-10 | Pass — shared typed-contract suite |
| `UT-TYP-2575-002` | Absent/null selected parameters fail closed; present-empty `{}` survives as present-empty; override absent/null/`{}` is a no-op. | BR-ORCH-025/026 | Pass — codec and approval/override suites |
| `UT-TYP-2575-003` | Explicit `0`, `false`, `""`, and `[]` remain present values and are not mistaken for absence. | BR-KA-191, SI-10 | Pass — shared typed-contract and mapping suites |
| `UT-TYP-2575-004` | Large integers are preserved from the number-safe KA input boundary and validated without float64 rounding; unsafe, fractional, or disallowed integer spellings fail closed. | BR-WORKFLOW-004, SI-10 | Pass — shared codec and KA/AF boundary suites |
| `UT-TYP-2575-005` | Finite float values pass; NaN, infinity, overflow, and nonzero underflow fail. | BR-WORKFLOW-005, SI-10 | Pass — shared codec suite |
| `UT-TYP-2575-006` | Malformed JSON, trailing tokens, top-level null, and unsupported object values are rejected before persistence or execution. | BR-WORKFLOW-004, SI-10 | Pass — strict codec suite |
| `UT-TYP-2575-007` | Canonical comparison is deterministic by sorted keys and compact JSON, preserves array order, and does not rewrite separate lossless submitted evidence; `1`, `1.0`, and `1e0` remain distinguishable there. | BR-AUDIT-005, CC7.2 | Pass — evidence/override and repository precision suites |
| `UT-VAL-2575-008` | Live schema required/type/enum/pattern/bounds/dependsOn validation returns actionable errors and never mutates inputs. | BR-KA-191, SI-10 | Pass — KA live-schema unit suite |
| `UT-VAL-2575-009` | Schema defaults are not inserted into the effective map, WE values, or audit evidence. | BR-ORCH-025, BR-AUDIT-006 | Pass — approval and regression suites |
| `UT-VAL-2575-010` | Partial overrides preserve omitted keys, reject new keys and changed managed target keys, and leave status unchanged on rejection. | BR-ORCH-026, AC-4/SI-10 | Pass — AF/AW/RO approval-safety suites |
| `UT-ID-2575-011` | Exactly one trusted workflow with matching status ID and recomputed content-derived ID is accepted; name/version-only matches are rejected. | BR-WORKFLOW-006, AC-4 | Pass locally — WE/controller identity paths; production IT blocked |
| `UT-ID-2575-012` | Missing, ambiguous, foreign, content-mismatched, malformed, unauthorized, and transient lookup outcomes use the correct no-retry/bounded-retry behavior. | BR-WORKFLOW-006, AC-4/AC-6 | Pass locally — WE/controller identity paths; RBAC IT blocked |
| `UT-CUT-2575-013` | The cutover gate rejects activation while legacy writers/controllers or active pre-cutover AgentSession/AIA/RAR/WE objects remain. | Breaking contract, SI-10 | Deferred — operational inventory gate, not implemented in this branch |
| `UT-CUT-2575-014` | The cutover gate permits typed activation only after legacy writers/controllers are quiesced and the active-object inventory is clean; no runtime conversion path is used. | Breaking contract, CC7.2 | Deferred — operational deployment gate, not implemented in this branch |
| `UT-WE-2575-015` | Job and Tekton adapters serialize typed values to their string-only engine contract without heuristic type inference. | BR-WE-014, SI-10 | Pass — mixed typed conversion unit test; production IT blocked |
| `UT-WE-2575-016` | Ansible adapter sends raw typed JSON `extra_vars`; invalid values cannot degrade to strings through `coerceValue`. | BR-WE-015, SI-10 | Pass — Ansible executor unit suite |
| `UT-WE-2575-017` | Executor declaration filtering uses the ephemeral live schema view; no `DeclaredParameterNames` snapshot is trusted. | BR-WORKFLOW-004, AC-4 | Pass — live metadata/filter unit suites; production IT blocked |
| `UT-AUD-2575-018` | Original, submitted override, and effective typed maps preserve values and absent/null/empty presence needed for reconstruction, while separate audit evidence preserves raw numeric lexemes beyond JSONB normalization. | BR-AUDIT-005/006, AU-3 | Pass — audit payload and repository precision suites |
| `UT-AUD-2575-019` | Audit builders include actor, correlation, workflow identity, outcome, error details, and expose enqueue failures without claiming durable success. | BR-AUDIT-005/006, AU-2/CC7.2 | Pass — affected audit manager/builder suites |
| `UT-CRD-2575-020` | AIA/RAR/WE/AgentSession projections contain no parameter schema or schema-derived allowlist; non-schema execution metadata remains intact. | BR-WORKFLOW-006, SI-7 | Pass — reflection, response, and generated-schema suites |
| `UT-RO-2575-021` | RAR and WE typed values are immutable/frozen according to the approved path; RO cannot replace the selected workflow or invent defaults. | BR-ORCH-025/026, AC-6 | Pass — RO override and creator suites |
| `UT-SEC-2575-022` | Empty trusted namespace, foreign namespace, invalid identity, and authorization failures fail closed and produce observable reasons. | BR-SECURITY-016, AC-4/AC-6 | Pass locally — fail-closed unit paths; RBAC IT/E2E blocked |
| `UT-REG-2575-023` | The #2570/#2601 regression vectors remain green: target locking, first decision wins, collision/manual review, no default insertion, exact evidence, and status immutability. | BR-ORCH-025/026, BR-AUDIT-006 | Pass — affected local regression suites |

### Common unit vectors

Use the same logical vectors through the shared core and KA/AF/AW/WE adapters.
The live workflow schema, not a copied fixture schema inside AIA/RAR/WE, is the
source of the constraints.

| Vector | Schema/input | Expected outcome |
|---|---|---|
| V-01 | `P:string`, values `{"P":"safe"}` | Preserve exact string. |
| V-02 | `P:integer`, value `3` | Preserve an integer, not a string. |
| V-03 | `P:integer`, value `9007199254740993` | Validate exact integer or reject by schema bounds; never round. |
| V-04 | `P:float`, value `1.0` | Preserve typed finite float and approved raw evidence. |
| V-05 | `P:boolean`, values `true`, `false` | Both are real values; quoted forms are not booleans. |
| V-06 | `P:array`, values `[]`, `["a",2]` | Preserve empty/order/type; reject malformed array JSON. |
| V-07 | Any current type, value `null` | Reject as a value; do not treat as absent inside a present map. |
| V-08 | Optional `P` with default, omitted | Effective map remains omitted; no default insertion. |
| V-09 | Override omitted, null, `{}` | No-op; original/effective values remain unchanged. |
| V-10 | Override `{"NEW":"x"}` or changed `TARGET_RESOURCE_*` | Reject with no terminal status mutation. |
| V-11 | Valid trusted CRD then changed/deleted/foreign CRD | The old ID fails verification; no fallback to name/version. |
| V-12 | Audit original/override/effective with `0`, false, `""`, `[]`, `{}` | Readback preserves typed values and presence semantics. |

---

## 9. Test scenarios — integration

| ID | Production-path outcome | Environment | Phase |
|---|---|---|---|
| `IT-CRD-2575-001` | Generated AIA/RAR/WE/AgentSession CRDs accept typed values, preserve `{}`, and contain no schema-copy fields after API-server round trip. | Real envtest API server and generated CRDs | Partial — disposable Kubernetes 1.37 round trip passed; current runtime rerun blocked by Podman storage |
| `IT-KA-2575-002` | KA reads the trusted workflow CRD, validates typed selection values, self-corrects invalid output, and persists only the selection—not its schema; raw LLM parameters including `9007199254740993`, `1.0`, and `1e0` retain number-safe tokens before typed construction. | KA/informer catalog, real CRD | Partial — focused unit/compile evidence passes; production IT blocked |
| `IT-AA-2575-003` | AgentSession → AIAnalysis production handoff preserves typed values and the raw-value evidence needed downstream, and rejects a stale/foreign/unverifiable workflow before AIA persistence. | AIA controller and envtest | Partial — local handoff tests pass; production IT blocked |
| `IT-RO-2575-004` | RO creates a RAR from verified AIA data with typed original values, immutable non-schema execution snapshot, and no `ParameterSchema`. | RO and real Kubernetes API | Partial — local creator/approval tests pass; production IT blocked |
| `IT-AF-2575-005` | MCP/API review carries absent/null/empty/partial typed overrides through the real API path; reviewer-visible values match the submitted request. | AF, API server, auth/webhook fixtures | Partial — local API/audit tests pass; production IT blocked |
| `IT-AW-2575-006` | AW validates the effective typed map against the live CRD schema, enforces managed-key locking, preserves status on denial, and applies the conditional decision write. | AW webhook and real API server | Partial — local admission tests pass; production IT blocked |
| `IT-AW-2575-007` | AW accepts only one exact trusted workflow content match; missing, changed, ambiguous, foreign, and name/version-only matches are denied with correct retry behavior. | Two namespaces, indexed status ID, API failure fixtures | Partial — fail-closed unit paths pass; production IT/RBAC blocked |
| `IT-RO-2575-008` | Approved RAR → WE uses typed effective values and frozen non-schema execution metadata, with no live-schema copy in the WE CRD. | RO/WE controllers and envtest | Partial — local creator tests pass; production IT blocked |
| `IT-WE-2575-009` | WE Pending reconcile obtains the exact trusted workflow, validates typed values, filters declared names from live content, and dispatches only after verification. | WE controller, real API server, executor fixture | Partial — controller/unit paths pass; production IT blocked |
| `IT-WE-2575-010` | Job, Tekton, and Ansible production executor paths receive correct engine representations for string, integer, float, boolean, and array values. | Real Job/Tekton object builders; isolated AWX HTTP server | Partial — executor unit coverage passes; production IT blocked |
| `IT-WE-2575-011` | Updating/deleting the workflow after selection but before Pending dispatch makes the old content-derived ID unverifiable; WE marks failure and creates no new external resource. | Real API server and production WE reconcile | Partial — fail-closed controller tests pass; production IT blocked |
| `IT-AUD-2575-012` | Approval intent, admission, committed decision, and execution audit events persist to real Data Storage and reconstruct typed values by correlation ID and workflow content identity. | Real Data Storage/PostgreSQL/HTTP and authorized query client | Partial — repository test and disposable PostgreSQL spike pass; current production readback IT blocked |
| `IT-INV-2575-016` | Production AIA, RO, AW, and WE reconcile paths verify the exact trusted workflow before persistence, decision mutation, child-CR creation, every Running/terminal status transition or reconcile completion, or external-resource creation; the inventory covers WFE fetch, no-finalizer addition (observable error/requeue with no mutation), finalizer-present Pending failure (Failed status with no resource), Running, terminal audit/cleanup, and deletion/finalizer removal, with explicit owner-approved cleanup exceptions. | Real API server, controller/webhook harness, and executor fixture | Blocked — production-path invariant matrix not run |
| `IT-CUT-2575-013` | The production cutover gate inventories AgentSession/AIA/RAR/WE objects and legacy writers/controllers, blocks activation when the active-object precondition fails, and permits the typed-only deployment only after the gate passes. | Real API server, generated CRDs, deployment/controller harness | Deferred — operational cutover gate, not implemented in this branch |
| `IT-SEC-2575-014` | Trusted namespace Role/RoleBinding permits required reads only; foreign namespace and unauthorized/status failures cannot satisfy verification. | Real service accounts and API-server RBAC | Blocked — operator RBAC change is delegated to #624; runtime proof not available |
| `IT-REG-2575-015` | Existing #2570/#2601 approval, collision, conditional-write, lock, audit, and no-default integration paths remain green after the type change. | Affected existing suites plus real API server | Partial — local regression/build evidence passes; runtime IT blocked |

---

## 10. Test scenarios — E2E

| ID | Deployed journey and expected outcome | Controls/BRs | Phase |
|---|---|---|---|
| `E2E-KA-2575-001` | An autonomous investigation selects a workflow with integer, float, boolean, string, and array inputs; the workload receives the intended typed semantics and the remediation completes. | BR-KA-191, BR-ORCH-025, BR-WE-015, SI-10 | Blocked — deployed full-pipeline environment unavailable |
| `E2E-AF-2575-002` | An authorized reviewer changes one existing typed value; AW admits it, omitted values remain unchanged, the locked target cannot change, and the workload receives the effective map. | BR-ORCH-026, AC-4/AC-6/SI-10 | Blocked — deployed full-pipeline environment unavailable |
| `E2E-WE-2575-003` | After selection but before WE dispatch, the trusted workflow is changed or deleted; the WFE fails closed and no Job/PipelineRun/AWX execution starts. | BR-WORKFLOW-006, AC-4, CC7.2 | Blocked — deployed full-pipeline environment unavailable |
| `E2E-WE-2575-006` | An Ansible/AWX workflow executes with integer, float, boolean, string, and array parameters; the AWX request receives the intended raw typed `extra_vars`, including a large integer without float64 rounding, and no heuristic string coercion occurs. | BR-WE-015, SI-10, CC7.2 | Blocked — deployed full-pipeline environment unavailable |
| `E2E-AUD-2575-004` | Querying Data Storage by remediation correlation reconstructs selection, approval, actor, typed original/override/effective values, workflow content hash, execution outcome, and failure evidence. | BR-AUDIT-005/006, AU-2/AU-3, CC7.2 | Blocked — deployed Data Storage/audit journey unavailable |
| `E2E-SEC-2575-005` | A foreign-namespace or unauthorized workflow cannot satisfy a live lookup; the denied path is observable and no execution occurs. | BR-SECURITY-016, AC-4/AC-6 | Blocked — deployed RBAC environment unavailable |

---

## 11. Detailed P0 test cases

### UT-TYP-2575-002: Presence semantics

**BR:** BR-ORCH-025/026, BR-KA-191
**Priority:** P0
**Type:** Unit
**Evidence files:** `pkg/shared/types/typed_parameter_contract_test.go`,
related approval/override suites

**Preconditions:** A live schema with optional string/integer/boolean/array
parameters and at least one required parameter.

**Steps:**

1. **Given** selected parameters are absent, then decode them and assert a
   missing-data failure.
2. **Given** selected parameters are `null`, then decode them and assert the
   same fail-closed result.
3. **Given** selected parameters are `{}`, then assert the map is present and
   schema validation—not presence logic—decides whether it is valid.
4. **Given** override parameters are omitted, `null`, and `{}`, then assert
   each is a no-op and the effective map equals the original map.
5. **Given** values `0`, `false`, `""`, and `[]`, then assert each key remains
   present with its original typed value.

**Expected results:** No value is converted to an absent state; no default is
inserted; invalid required-value cases fail before persistence.

**Acceptance criteria:** API-equivalent JSON for `{}` survives; null/absent
selected input fails; override no-op semantics match #2570; explicit zero-like
values remain real values.

### UT-ID-2575-011: Exact content-derived workflow identity

**BR:** BR-WORKFLOW-006, BR-SECURITY-016
**Priority:** P0
**Type:** Unit
**Evidence files:** `pkg/workflowexecution/reconcile_pending_orchestration_test.go`
and affected controller suites; no dedicated `workflowidentity` #2575 spec was
added

**Preconditions:** A trusted namespace, expected workflow ID, and workflow
fixtures whose clean CRD content can be hashed with the existing helper.

**Steps:**

1. **Given** one workflow with matching namespace/status ID/content-derived ID,
   then assert verification returns the exact workflow object/view.
2. **Given** a same-name or same-version workflow with a different content
   derived ID, then assert verification fails.
3. **Given** zero or multiple status-ID matches, then assert verification fails
   without retrying a successful no-match/ambiguous read.
4. **Given** a foreign-namespace match, then assert it cannot satisfy lookup.
5. **Given** content hash recomputation fails or differs, then assert fail closed.

**Expected results:** Only the exact trusted content is usable for schema
validation. No caller receives a name/version fallback.

### IT-WE-2575-009: WE live schema gate before external dispatch

**BR:** BR-WORKFLOW-006, BR-ORCH-025, BR-WE-015
**Priority:** P0
**Type:** Integration
**Runtime target:** `test/integration/workflowexecution/typed_parameters_test.go`
(not run or added; production integration gate remains blocked)

**Preconditions:** Real API server, trusted workflow CRD, WFE with typed
effective parameters and its finalizer already present, and an executor fixture
that records resource creation.

**Steps:**

1. **Case A — valid Pending:** Given a finalizer-present Pending WFE whose ID
   matches one valid trusted workflow, reconcile Pending. When WE verifies the
   content and validates the effective map, assert the executor receives a live
   ephemeral schema view and creates the expected resource.
2. **Case B — invalid Pending:** Given a fresh finalizer-present Pending WFE
   whose workflow is deleted, modified, foreign-only, ambiguous, or malformed,
   invoke the production Pending reconcile path. Assert WFE enters an observable
   failed state and the executor fixture records no new external resource.
3. **Case C — invalid Running:** Given a separate WFE already in Running with
   the same identity failure, invoke the production Running reconcile path.
   Assert no status transition, successful reconcile completion, or external
   resource recreation occurs; the failure is observable and retriable according
   to the classified lookup outcome.

**Acceptance criteria:** The WFE CRD has no schema copy; valid Pending execution
is typed; every identity/schema failure blocks dispatch; Running failure does not
silently advance status or recreate a resource.

### IT-AUD-2575-012: Correlated typed audit readback

**BR:** BR-AUDIT-005/006
**Priority:** P0
**Type:** Integration
**Runtime target:** `test/integration/datastorage/typed_parameter_audit_test.go`
(not run or added; repository precision and disposable PostgreSQL evidence are
recorded separately)

**Preconditions:** Real authenticated Data Storage/PostgreSQL, unique
remediation correlation ID, and production audit emitters.

**Steps:**

1. **Given** a raw LLM response containing parameter tokens `9007199254740993`,
   `1.0`, and `1e0`, pass it through the production KA parser and
   AgentSession → AIA → approval → WE path, with an approved partial override
   containing the large integer.
2. **When** the asynchronous audit buffer is explicitly flushed, query the
   remediation lifecycle events using the remediation correlation ID, exercising
   both single-event and batch persistence paths.
3. **Then** extract the executed/selected `workflow_id` from those events and
   join to `remediationworkflow.admitted.create`/`update` events by
   `event_data.workflow_id`, as specified by DD-AUDIT-004. Do not assume the
   workflow admission event uses the remediation correlation ID.
4. **Then** assert common event ID/type/action/outcome, actor, resource, and
   correlation fields; assert event-specific ownership: intent has the
   submitted override/rationale, admission has accepted original/override/
   effective values plus actor/delegation, the committed decision has the full
   original/override/effective trace and workflow identity, execution events
   have effective values/workflow identity/outcome, and failure events have
   `error_details`. Every event carrying parameter values also carries the
   separate lossless `parameter_evidence` field.
5. **Then** assert the joined workflow admission content and content hash are
   sufficient to reconstruct the authoritative schema without a parameter
   schema copy in the RAR or WE; missing admission evidence is an explicit
    reconstruction failure, not a fallback to a live CRD or snapshot. Assert
    that the semantic readback preserves `9007199254740993` without float64
    rounding and that raw evidence distinguishes numeric lexemes such as `1`,
    `1.0`, and `1e0` even when semantic typed maps are stored in JSONB.
6. **Given** enqueue failure, assert the failure is observable and no durable
   success claim is emitted.

**Acceptance criteria:** Real persisted readback, not an in-memory capture,
proves the full typed decision trace.

### E2E-KA-2575-001: Typed end-to-end execution

**BR:** BR-KA-191, BR-ORCH-025, BR-WE-015
**Priority:** P0
**Type:** End-to-end
**Runtime target:** `test/e2e/fullpipeline/typed_parameter_contract_2575_test.go`
(not run or added; deployed E2E environment is unavailable)

**Preconditions:** Deployed KA, AIA, RO, WE, trusted workflow CRD, real API
server, an execution fixture that records inputs, and an isolated audit query.

**Steps:**

1. **Given** the CRD declares string, integer, float, boolean, and array
   parameters, trigger the normal investigation and selection path.
2. **When** the selected values cross AIA, RO, and WE, inspect each API-server
   object and assert no parameter schema is copied.
3. **When** the workload executes, assert it receives the intended values and
   that `0`, `false`, `""`, and `[]` are not omitted.
4. **Then** query the audit trail by correlation ID and assert typed selection,
   execution, actor, outcome, and workflow content identity.

**Acceptance criteria:** The deployed journey proves semantic value fidelity,
not merely that a CRD field was populated.

---

## 12. Environment and tooling

### 12.1 Unit tests

- Framework: Ginkgo/Gomega BDD.
- Real logic: typed JSON parsing, validation, canonical comparison, identity,
  audit mapping, and engine conversion.
- External mocks only: AWX/network failure seams.
- Native Go fuzz tests are allowed only in dedicated fuzz files for untrusted
  JSON parsing; they supplement, not replace, the BDD cases.

### 12.2 Integration tests

- Framework: Ginkgo/Gomega BDD.
- Kubernetes: real envtest/API server with generated CRDs and selectable
  `status.workflowId` behavior.
- Audit: real Data Storage/PostgreSQL/HTTP and an independently authenticated
  query client.
- Authorization: real service-account Roles/RoleBindings where the harness
  supports them; use mock SAR only for isolated error classification, not as
  the sole control proof.
- No mocks for internal controllers, validators, CRD serialization, or audit
  persistence.

### 12.3 E2E tests

- Deployed Kind/full-pipeline environment with trusted and foreign namespaces.
- Real API-server CRD admission and controller reconciliation.
- Mock LLM is permitted as an external model boundary.
- Job/Tekton execution fixtures must expose actual received inputs; observing
  only the WFE spec is insufficient.

### 12.4 Tools and versions

| Tool | Version | Purpose |
|---|---|---|
| Go | 1.26.10 | Build and test |
| Ginkgo/Gomega | v2.33.1 / repository-pinned | BDD execution |
| Kubernetes envtest/Kind | repository-supported | CRD and controller wiring |
| PostgreSQL/Data Storage | repository integration harness | Audit persistence/readback |

---

## 13. Dependencies and execution order

### 13.1 Blocking dependencies

| Dependency | Type | Status | Impact | Workaround |
|---|---|---|---|---|
| Owner approval of typed contract | Decision | Complete on 2026-11-08 | Implementation may proceed under the approved typed-only contract | Preserve the decision and keep runtime gates explicit |
| Raw-JSON CRD codegen spike | Technical | Complete in disposable workspace | Contract and generated schema round trips are locally qualified | Keep generated source/assets synchronized |
| #2570/#2601 safety baseline | Code/contract | Local affected suites pass; runtime integration pending | Typed change may regress approval safety in a deployed path | Re-run the named integration/E2E regression set when infrastructure is available |
| Real Data Storage audit harness | Infrastructure | Blocked for current production-path evidence | Audit persistence cannot be certified from local capture alone | Re-run `IT-AUD-2575-012` with authenticated HTTP/PostgreSQL readback |
| Trusted namespace RBAC/selectable field | Deployment | Blocked/delegated to #624 | Identity fence cannot be proven in production path | Install the operator-owned Role/RoleBinding and run `IT-SEC-2575-014` |
| Podman storage for envtest/integration images | Infrastructure | Blocked during prior runtime attempt (`no space left on device`) | Controller/webhook integration and E2E execution cannot be rerun | Restore container storage, then rerun affected suites |

### 13.2 Execution order

1. **Contract spike:** raw JSON map, presence, generated CRD, content identity,
   and OpenAPI round trips.
2. **RED:** all unit cases, then integration cases for every wiring row.
3. **GREEN:** production types, live verification, adapters, audit, and
   generated artifacts; run IT continuously.
4. **REFACTOR:** remove duplicate conversion/snapshot logic; rerun build and
   compile-only test gate.
5. **Breaking-cutover verification:** quiesce legacy writers/controllers,
   inventory active pre-cutover objects, block activation when the inventory is
   not clean, and prove the typed-only readers/writers are then deployed without
   a conversion path.
6. **E2E/control verification:** execute full journeys and record environment,
   result, commit, and any unrelated baseline failure separately.

### 13.3 Current evidence disposition

- **Complete locally:** typed union and number-safe codec, CRD/OpenAPI/Ogen
  generation, affected unit/regression suites, build, compile-only regression,
  lint, and the mixed Job/Tekton engine-string test.
- **Qualified by disposable spike only:** Kubernetes 1.37 CRD/status round trips,
  approval-safety envtest, and real PostgreSQL create/batch/query readback. These
  results are evidence for the contract, not current production-path certification.
- **Blocked:** runtime controller/webhook integration, authenticated production
  Data Storage readback, deployed E2E journeys, and RBAC verification. The prior
  runtime attempt stopped at a Podman image build with `no space left on device`.
- **Deferred:** active-object inventory and legacy-writer/controller quiescence
  gates (`UT/IT-CUT-2575-013/014`). No cutover implementation or test is added in
  this branch; rollout ownership remains operational.

---

## 14. Wiring verification matrix

| Code path | Production entry point | Exit point | Wiring IT | Status |
|---|---|---|---|---|
| Live schema and typed validation | KA/AF/AW/WE production adapters | Accepted/rejected typed map | `IT-KA-2575-002`, `IT-AW-2575-006`, `IT-WE-2575-009` | Production callers present; runtime IT blocked |
| KA selection | AgentSession workflow selection | Typed AIA handoff | `IT-KA-2575-002` | Production caller present; runtime IT blocked |
| AgentSession → AIA | AIA response processing | Persisted AIA selection without schema | `IT-AA-2575-003` | Production caller present; runtime IT blocked |
| AIA → RAR | RO approval creation | Immutable typed RAR | `IT-RO-2575-004` | Production caller present; runtime IT blocked |
| AF review → AW | MCP/API approval | Conditional RAR decision | `IT-AF-2575-005`, `IT-AW-2575-006` | Production callers present; runtime IT blocked |
| Approved RAR → WE | RO approved-decision reconcile | Typed WE CRD without schema | `IT-RO-2575-008` | Production caller present; runtime IT blocked |
| WE live gate | WFE Pending reconcile | Job/PipelineRun/AWX creation or fail-closed status | `IT-WE-2575-009`, `IT-WE-2575-011` | Production caller present; runtime IT blocked |
| General reconcile invariant | AIA, RO, AW, and schema-dependent WE state handlers | Workflow verified before transition, child creation, or external dispatch | `IT-INV-2575-016` | Production callers present; runtime IT blocked |
| Engine conversion | Executor registry/strategy dispatch | Actual backend input | `IT-WE-2575-010` | Production callers and unit proof present; runtime IT blocked |
| Audit emission | RO/AF/AW/WE audit builders | Data Storage readback | `IT-AUD-2575-012` | Production callers present; readback IT blocked |
| Namespace/RBAC | Service-account API access | Allowed trusted read or observable denial | `IT-SEC-2575-014` | Runtime RBAC proof blocked; operator work delegated to #624 |
| Breaking contract gate | Coordinated deployment and updated readers | Typed-only activation only after a clean active-object inventory | `IT-CUT-2575-013` | Deferred operational gate; not implemented in this branch |

CHECKPOINT W passes only when every row has a production caller and a passing
integration test. Unit tests do not count as wiring proof.

---

## 15. Evidence and follow-up test obligations

| Location | Current local evidence | Remaining obligation |
|---|---|---|---|
| `pkg/remediationorchestrator/approval_orchestration_test.go` | Typed AIA/RAR values and no schema snapshot assertions pass locally | Re-run controller integration path when envtest is available |
| `pkg/remediationapprovalrequest/parameters_test.go` | Live-workflow typed validation and managed-key rejection pass locally | Re-run AuthWebhook production admission path |
| `pkg/authwebhook/rar_override_validation_test.go` | Typed approval-safety regression suite passes locally | Complete real API-server/RBAC admission proof |
| `pkg/aianalysis/handlers/response_processor_workflow_snapshot_test.go` | Schema-copy removal and typed handoff assertions pass locally | Complete AgentSession → AIA production handoff IT |
| `internal/controller/workflowexecution/workflowexecution_pending_test.go` | Live metadata/filtering options pass locally | Complete WE Pending/Running invariant IT |
| `pkg/workflowexecution/executor/*_test.go` | Job/Tekton mixed conversion and Ansible native JSON unit coverage pass | Complete production executor IT and E2E input observation |
| `api/openapi/data-storage-v1.yaml` and generated Ogen assets | Source/generated parity and repository precision tests pass | Complete authenticated Data Storage readback |
| `test/e2e/fullpipeline/22_*`, `23_*`, `24_*` | Typed regression changes compile; deployed journeys were not re-run | Run full approval/execution/audit E2E set |

No in-scope test may be skipped or left pending in final acceptance. The
`Blocked` and `Deferred` labels above are evidence dispositions, not passing
tests; they must be resolved or explicitly accepted before release. A scenario
that cannot be supported is removed from the approved plan with owner rationale,
not hidden with `XIt` or `Skip()`.

---

## 16. Deliverables and changelog

### 16.1 Deliverables

| Deliverable | Location | Description |
|---|---|---|
| Implementation plan | `docs/tests/2575/IMPLEMENTATION_PLAN.md` | Contract, rollout, wiring, and approval gate |
| This test plan | `docs/tests/2575/TEST_PLAN.md` | IEEE 829/Kubernaut test strategy and scenarios |
| Typed unit suites | affected `pkg/**` packages | BDD proof of values, identity, conversion, and audit mapping |
| Integration suites | `test/integration/**` and affected controller suites | Real CRD/controller/webhook/audit wiring proof |
| E2E suites | `test/e2e/**` | Deployed journey and control-objective proof |
| Coverage/evidence report | CI artifacts and issue/PR record | Requirements coverage, structural diagnostics, and environment results |

### 16.2 Changelog

| Version | Date | Change |
|---|---|---|
| 1.0 | 2026-11-06 | Initial docs-only draft; implementation and contract remain unapproved |
| 1.1 | 2026-11-08 | Recorded local implementation evidence, disposable-spike qualification, blocked runtime gates, and deferred operational cutover gates |

**Current disposition:** The typed representation, lossless
`parameter_evidence`, canonicalization, content-derived revision fence, and
reconcile-state contract are approved and implemented. Local quality gates pass.
Production-path integration/E2E, RBAC, authenticated audit readback, and the
operational cutover inventory remain open; no commit or push has been performed.
