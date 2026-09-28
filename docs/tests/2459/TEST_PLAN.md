# Test Plan: Workflow Discovery Audit Result Reconstruction

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-2459-v1.1
**Feature**: Persist the action types and workflow candidates returned during KA workflow discovery
**Version**: 1.2
**Created**: 2026-09-26
**Updated**: 2026-09-27
**Author**: Kubernaut Team
**Status**: Focused #2459/#2466 UTs, live Data Storage audit IT, live etcd-Catalog IT, and post-fix focused Kind E2E pass. The E2E queries real Data Storage by remediation ID and verifies typed discovery reconstruction plus the `get_workflow` audit omission marker and raw sentinel exclusion. Build, branch-delta lint, and repository-wide test-package compilation pass. The latest aggregate `make test` rerun reported two unrelated SignalProcessing shutdown timing assertions under 12-way parallel load; those shutdown specs pass in isolation. Generated OpenAPI copies/client are synchronized; `make gen-diff` was not run because the worktree is intentionally uncommitted.
**Branch**: `fix/2459-workflow-discovery-audit`

## 1. Introduction

### 1.1 Purpose

Issue #2459 is partially addressed on `origin/main`: detected-label filters are now represented in the typed audit payload, but the audit trace still drops the action choices returned by Step 1, the selected `action_type` from Step 2, and the actual workflow candidates returned by Step 2. These losses prevent an operator from reconstructing what KA considered for a remediation from the Data Storage audit trail. This plan specifies behavior-focused tests for preserving those results, their order/score, and their remediation correlation without exposing audit-only scoring to the LLM.

### 1.2 Objectives and success metrics

1. A query by remediation/correlation ID returns the `actions_listed` and `workflows_listed` events with the exact action types and workflow candidates returned for the corresponding page.
2. Step 2 audit data includes its `action_type`, each candidate's stable workflow identity/version, returned order/rank, and the catalog's actual `final_score`; it does not add score or other audit-only metadata to the LLM-facing tool response.
3. Action and workflow result counts, pagination context, signal filters, event envelope metadata, and correlation remain consistent with the operation that ran.
4. Existing clients and prior payloads remain compatible with newly added optional typed fields.
5. Unit, integration, and E2E scenarios pass; all new behavior assertions use Ginkgo/Gomega and map to the BR/control matrix below.
6. New unit-testable conversion/serialization business logic reaches 100% line coverage; merged all-tier coverage remains at or above the project gate.

## 2. References and authority

- [Issue #2459](https://github.com/jordigilh/kubernaut/issues/2459)
- [BR-AUDIT-005](../../requirements/11_SECURITY_ACCESS_CONTROL.md) — complete, queryable audit trails; SOC 2 reconstruction objective
- [BR-AUDIT-021-030](../../requirements/BR-AUDIT-021-030-WORKFLOW-SELECTION-AUDIT-TRAIL.md): BR-AUDIT-023 event generation, BR-AUDIT-025 context/action capture, BR-AUDIT-026 per-result score, BR-AUDIT-027 workflow metadata, BR-AUDIT-030 correlation query
- [DD-AUDIT-004](../../architecture/decisions/DD-AUDIT-004-structured-types-for-audit-event-payloads.md) — typed payload requirement
- [DD-AUDIT-009](../../architecture/decisions/DD-AUDIT-009-workflow-discovery-event-specific-payloads.md) — approved event-specific payload design
- [DD-KA-017](../../architecture/decisions/DD-KA-017-three-step-workflow-discovery-integration.md) and [DD-WORKFLOW-016](../../architecture/decisions/DD-WORKFLOW-016-action-type-workflow-indexing.md) — three-step discovery and internal score behavior
- [DD-WORKFLOW-019](../../architecture/decisions/DD-WORKFLOW-019-ka-owned-workflow-discovery.md) — KA discovery ownership
- [ADR-034](../../architecture/decisions/ADR-034-unified-audit-table-design.md) — unified audit record and correlation
- FedRAMP `AU-2` (audit events) and `AU-3` (audit record content)
- SOC 2 `CC7.2` (monitoring and investigation/reconstruction from audit traces)
- OWASP ASVS v5.0.0, V16.2.1 (necessary when/where/who/what metadata), V16.2.4 (logs readable and correlatable by the log processor), and V16.2.5 (protection-level handling when sensitive data is logged). For V16.2.5, this feature's proof is that unapproved workflow implementation/parameter content—including seeded sentinel values—is not written into the audit trace.
- [OWASP ASVS v5.0.0, V16 Security Logging and Error Handling](https://github.com/OWASP/ASVS/blob/master/5.0/en/0x25-V16-Security-Logging-and-Error-Handling.md)

## 3. Risks and mitigations

| ID | Risk | Impact | Probability | Affected tests | Mitigation |
|---|---|---|---|---|---|
| R1 | Catalog ranking computes `final_score` and discards it before the tool receives workflows. | High: audit cannot reproduce ranking evidence. | Medium | UT-002, IT-001, E2E-001 | Add a discovery-only scored result path; assert exact score/rank and never recompute in the audit layer. |
| R2 | Audit-only score accidentally changes the LLM-facing response. | High: changes model input/selection behavior. | Low | UT-002, IT-001 | Assert separately that audit data contains the score and serialized LLM response has no score/audit-only fields. |
| R3 | Typed OpenAPI/client changes break historical payload reads or legacy `scoring.confidence` drifts from explicit `final_score`. | High: audit write/query compatibility regression or misleading ranking evidence. | Medium | UT-002/003, IT-001 | Keep new `final_score` optional for historical records, decode old event shapes through request/response unions, and assert both fields equal the cache's actual ranking score for new events. |
| R4 | Candidate/action metadata includes unnecessary or sensitive content. | High: excess audit data retention or exposure. | Low | UT-002/003, IT-001, E2E-001 | Use allowlisted typed schemas; seed a non-secret sentinel in workflow parameter/execution content and prove it is absent from Data Storage audit records; never log credentials or tokens. |
| R5 | E2E only observes event presence, not persisted decision evidence. | High: false confidence in reconstruction. | Medium | IT-001, E2E-001 | Query Data Storage by remediation ID and hard-assert exact payload evidence and selected-candidate membership. |

### 3.1 Risk-to-test traceability

All high-impact risks have at least one named test. R1 and R3 also have explicit unit and integration coverage; R5 is assessed by both the real Data Storage IT and deterministic E2E. No identified risk is intentionally left without a proving scenario.

## 4. Scope

### In scope

- The actual action-type entries returned by `list_available_actions`, including identifiers, useful taxonomy descriptions, and workflow counts.
- Step 2's `action_type`, returned candidate identities/names/versions, deterministic rank, and the existing catalog `final_score` (audit-only).
- Accurate total/returned counts and the pagination position/continuation information needed to distinguish returned items from the full match set.
- Distinct typed OpenAPI payload variants for `actions_listed` and `workflows_listed`, generated-client serialization, Data Storage persistence/query by correlation ID, and current signal-filter context including detected-label round trips. Event names remain unchanged; common filter/timing components remain shared.
- Preservation of event ID/type/action/outcome/time, actor attribution, remediation correlation, cluster context when present, and workflow `ResourceID` on Step 3 events.
- Unit, integration, and deterministic E2E proving tests; update the KA audit event catalog to match the emitted payload.

### Out of scope

- Changes to action/workflow matching, ranking formula, candidate order, prompt instructions, or selection/security-gate behavior.
- Exposing `final_score` or additional audit-only fields to the LLM.
- Adding or renaming audit event types, changing the Data Storage query API, retention, authorization, or cryptographic integrity mechanisms.
- Claiming full closure of every historical BR-AUDIT-027 metadata item (for example content hash/owner fields not available on the current tool contract); this change captures the decision evidence required by #2459 and the score/context required by BR-AUDIT-025/026.

## 5. Test approach and TDD mapping

Tests use Ginkgo/Gomega and prove business outcomes, not just implementation details. The persistence domains are separate: discovery candidates are read from ActionType/RemediationWorkflow CRDs stored in Kubernetes etcd and cached by the informer; discovery audit events are written to and queried from Data Storage. The pyramid is explicit: UT proves result capture, score fidelity, minimization, and typed mapping; IT exercises the etcd-backed Catalog plus production-registered tools and the real buffered DS audit write/query path; E2E proves the same persisted decision reconstruction through a deployed AgentSession investigation. IT and E2E must query real Data Storage records; neither may use an in-memory audit capture as the result oracle.

### Coverage policy and tier minimum

- New unit-testable business logic (result conversion, score provenance, and typed payload mapping) targets 100% line coverage; the merged all-tier coverage must remain at or above the repository gate.
- Every in-scope BR has a business-level UT for pure logic and an IT proof for each production wiring point; requirements whose business outcome is persistence/correlation are proven by querying real Data Storage. E2E adds the SOC 2 lifecycle reconstruction journey and does not replace the real-Data-Storage IT.
- UT exercises real internal scoring/result-conversion/payload logic. IT/E2E use deterministic ActionType/RemediationWorkflow CRDs in Kubernetes etcd, the real informer-backed Catalog and KA tool path, and real Data Storage/PostgreSQL audit persistence/query; no business or storage component is mocked. The Mock LLM is permitted only as an external E2E dependency boundary.
- PASS requires every P0 scenario to pass, compatibility tests to pass, each wiring manifest row to have IT evidence, all new business logic to meet coverage, and no affected-suite regression. FAIL includes any missing persisted result, score mismatch, correlation failure, or score leakage into LLM output.
- Suspend IT/E2E only when required services/Kind infrastructure are unavailable or the build is broken; record the blocked tier and resume once restored. Do not weaken assertions or convert them to best-effort event-presence checks.

### Pyramid invariant — evidence required at each tier

| Tier | Path exercised | Business-level proof (not implementation-only) | Primary controls |
|---|---|---|---|
| UT | Real Catalog score/result logic and typed payload conversion; no mocked business logic. | Returned Step 1 actions and Step 2 candidate order/rank/score are captured faithfully; legacy score alias equals the actual ranking score; score stays out of the LLM response; old payloads decode; disallowed sensitive fields are excluded. | FedRAMP AU-2/AU-3; ASVS V16.2.5 |
| IT | Seeded CRDs in Kubernetes etcd → real informer Catalog → production-registered KA discovery tools → production `BufferedDSAuditStore` → real Data Storage/PostgreSQL → query API by remediation ID. | The query result reconstructs both actual discovery results, score/rank, event envelope, actor, timestamp, filters, and correlation; explicit flush is only a deterministic barrier for the async store, not an in-memory result oracle. | FedRAMP AU-2/AU-3; SOC 2 CC7.2; ASVS V16.2.1/V16.2.4/V16.2.5 |
| E2E | Deployed KA AgentSession journey → production buffered audit path → real Data Storage → authenticated query by unique remediation ID. | The persisted actions/candidates explain the selected workflow; the selected workflow is a member of the audited candidates; all payload and protection-boundary assertions are made on Data Storage results. Poll only for async arrival, then fail on any missing or incomplete evidence. | FedRAMP AU-2/AU-3; SOC 2 CC7.2; ASVS V16.2.1/V16.2.4/V16.2.5 |

### RED

- `UT-KA-2459-001`: Step 1 audit event contains the action entries actually returned, not just their count, and uses the actual page's returned count.
- `UT-KA-2459-002`: Step 2 audit event preserves `action_type`, ordered candidate identity/version/rank, exact `final_score`, and the compatible `scoring.confidence` mirror; both values equal the cache score and the LLM-facing result remains score-free.
- `UT-KA-2459-003`: the event-type discriminator selects the Step 1 or Step 2 typed variant; marshal/unmarshal preserves the corresponding fields, equal score fields, filter variants (including ArgoCD, Flux, and `failedDetections`), and prior/minimal payload compatibility, while excluding parameter schemas and execution bundles.
- `IT-KA-2459-001`: seed/use the real etcd-backed Catalog; execute the production-registered discovery tools with a real `BufferedDSAuditStore` backed by the integration environment's authenticated DS audit client; explicitly flush, query Data Storage by remediation ID, and prove the persisted action/workflow results and audit envelope are reconstructable.
- `E2E-KA-2459-001`: extend the deterministic three-step OOM AgentSession journey with a unique remediation ID; the deployed KA writes through its production buffered audit path. Query real Data Storage by that ID, wait for both async events, then hard-assert the action/workflow audit results, envelope, exact score/rank, selected-candidate membership, and absence of excluded sensitive workflow content.

### GREEN

- Wire result capture through the existing production tool registry and `AuditStore` path.
- Add the approved event-specific typed schemas/discriminator mappings and regenerate the Data Storage OpenAPI client/spec artifacts.
- Carry the cache-computed score to audit construction without changing the tool's user-visible response.
- Make all RED tests pass before declaring GREEN complete; the IT for the real persistence/query wiring is mandatory.

### REFACTOR

- Keep the conversion from catalog results to typed audit records centralized and explicit; remove duplicated mapping if present.
- Keep audit-only values separate from `WorkflowDiscoveryResponse` so the LLM contract stays unchanged.
- Update the audit event catalog and decision record for the approved schema/data-flow choice.
- Mandatory post-refactor validation: `go build ./...`, affected Ginkgo suites, and generated-artifact consistency.

## 6. BR and control coverage matrix

| Requirement/control | Business-level assertion | Tier / test ID | Priority | Status |
|---|---|---|---|---|
| BR-AUDIT-023 / FedRAMP AU-2 | Each discovery operation actually performed emits and persists exactly one Step 1/Step 2 event; event type/action/outcome identify the audited operation. | UT-001/002; IT-001; E2E-001 | P0 | PASS: UT, IT, E2E |
| BR-AUDIT-025 / FedRAMP AU-3 | Persisted audit records contain event ID, event type/category/action/outcome, UTC timestamp, actor type/ID, remediation correlation, filters, and result/page counts matching the operation. | UT-001/002/003; IT-001; E2E-001 | P0 | PASS: UT, IT, E2E |
| BR-AUDIT-026 / FedRAMP AU-3 | Every returned workflow candidate has its actual cache-computed score and rank, while the LLM result remains score-free. | UT-002; IT-001; E2E-001 | P0 | PASS: UT, IT, E2E |
| BR-AUDIT-005, BR-AUDIT-030 / SOC 2 CC7.2 | A query by remediation ID alone reconstructs action options, the chosen action category, candidates considered, ranking evidence, and selected candidate identity. | IT-001; E2E-001 | P0 | PASS: IT, E2E |
| ADR-034 / ASVS `v5.0.0-16.2.1` | Persisted records provide the necessary when/where/who/what metadata for investigation (timestamp, cluster/context when present, actor, operation/result, correlation). | IT-001; E2E-001 | P0 | PASS: IT, E2E |
| BR-AUDIT-030 / ASVS `v5.0.0-16.2.4` | The machine-readable Data Storage query path returns both typed events correlated by the remediation ID. | IT-001; E2E-001 | P0 | PASS: IT, E2E |
| ASVS `v5.0.0-16.2.5` | Audit persistence enforces the approved protection boundary for logged data: workflow implementation/parameter content is excluded, and seeded sentinel content is absent from persisted records. | UT-003; IT-001; E2E-001 | P0 | PASS: UT, raw-response IT, E2E |

SOC 2 `CC8.1` is not used as an audit-completeness mapping; in this project it denotes change management.

## 7. Test scenarios and cases

Test ID format follows `{TIER}-KA-2459-NNN`; the current outcome for each case is recorded below. Each case uses a business outcome (persisted reconstruction, exact score fidelity, compatibility, or data minimization) as its oracle.

| ID | Preconditions / Given | Steps / When | Expected result / Then | BR / controls | Priority / phase |
|---|---|---|---|---|---|
| UT-KA-2459-001 | Catalog page has a known subset of action entries, total match count, filters, and continuation context. | Run Step 1 result-to-audit conversion. | Typed Actions Listed payload contains exactly returned actions and accurate total/returned/page/filter values, not merely a count. | BR-AUDIT-023/025; AU-2/AU-3 | P0 / PASS |
| UT-KA-2459-002 | Catalog returns ordered candidates with known cache-computed scores for one `action_type`. | Run Step 2 capture, audit conversion, and LLM response projection. | Audit has exact candidate identity/version/order/rank/`final_score`; nested `scoring.confidence` equals that same label-ranking score; LLM JSON has no score or audit-only fields. | BR-AUDIT-025/026; AU-3 | P0 / PASS |
| UT-KA-2459-003 | Historical minimal and current complete Step 1/Step 2 JSON include filter variants (ArgoCD, Flux, `failedDetections`). | Encode/decode request and response event-data unions; inspect allowlisted fields. | Correct event-specific variant validates; historical shapes decode; both score fields round-trip consistently for new records; parameter schemas and execution bundles are absent. | DD-AUDIT-004; ASVS V16.2.5 | P1 / PASS |
| IT-KA-2459-001 | Envtest Kubernetes API/etcd has seeded ActionType/RemediationWorkflow CRDs; real informer Catalog and real Data Storage/PostgreSQL/Redis are running; unique remediation ID; workflow fixture includes a synthetic sensitive-content sentinel in fields excluded from audit. | Invoke production-registered Step 1/Step 2 tools using the etcd-backed Catalog and a real buffered audit store backed by the authenticated DS audit client; call `Flush(ctx)`; query DS through the separate Ogen query client using only the remediation ID. | DS returns both typed events. Assert event envelope/actor/time/outcome, exact action options, filters/counts, `action_type`, ordered candidate identity/version/rank and exact `final_score` (= `scoring.confidence`); selected audit-only/sensitive sentinel fields are absent; all events share the queried correlation. | BR-AUDIT-023/025/026/030; AU-2/AU-3; CC7.2; ASVS V16.2.1/16.2.4/16.2.5 | PASS (initial run and raw-response retry) |
| E2E-KA-2459-001 | Deployed KA/Mock LLM/real Data Storage on the existing Kind stack; seeded workflow CRDs are in Kubernetes etcd; unique per-run OOM AgentSession remediation ID. | Complete the three-step investigation; query the real DS API by that remediation ID, polling only until both async discovery events arrive. | Hard-assert both persisted typed events and envelope; reconstruct `IncreaseMemoryLimits`, returned OOM candidate identity/version/order/rank/exact score and filters; selected workflow is among the candidates; excluded sensitive workflow sentinel/content is absent, including in generic `get_workflow` tool-call audit data. No in-memory audit capture is accepted. | BR-AUDIT-005/023/025/026/030; AU-2/AU-3; CC7.2; ASVS V16.2.1/16.2.4/16.2.5 | PASS post-#2466; 1/1 focused spec |

## 8. Environmental needs, execution, and pass criteria

- Framework: Ginkgo/Gomega BDD for behavior assertions; generated OpenAPI client tests run under the package's existing Ginkgo suite. Any temporary spike-only standard-library test stays outside the repository and is not production/business test coverage.
- UT: local Go toolchain and repository-generated OpenAPI client; test pure result mapping/serialization and existing internal business logic without mocking business components.
- IT: existing KA custom-tools integration harness with envtest Kubernetes API server/etcd (source of ActionType/RemediationWorkflow CRDs), the real informer-backed Catalog, and a separate real Data Storage/Postgres/Redis audit service with test authentication. `IT-KA-2459-001` passed through `BufferedDSAuditStore`, explicit flush, and independent Ogen/raw remediation-ID queries; `IT-KA-2466-001` passed against the live Catalog and verifies the LLM projection.
- E2E: `E2E-KA-2459-001` passed against deployed KA, Mock LLM, and real Data Storage. It queries production-written records by unique remediation ID and validates typed discovery evidence, selected-candidate membership, the omission marker, and raw sentinel exclusion. Local images were reused through the harness's `KUBERNAUT_CI_ARTIFACT_TAG` path after root-context exclusions reduced build context size.
- Test code locations: existing package-level Ginkgo specs for `internal/kubernautagent/audit`, `tools/custom`, and `workflowcatalog`; integration path `test/integration/kubernautagent/tools/custom/`; existing E2E file `test/e2e/kubernautagent/three_step_discovery_test.go`.
- Tools: repository Go toolchain (go.mod `go 1.26.6`), Ginkgo v2 from go.mod, repository-pinned Ogen v1.20.1 through the generation target, and Kind/container tooling for E2E.

Target commands after implementation:

```bash
go test ./internal/kubernautagent/audit/... -ginkgo.focus=2459
go test ./internal/kubernautagent/tools/custom/... -ginkgo.focus=2459
go test ./internal/kubernautagent/workflowcatalog/... -ginkgo.focus=2459
make test-integration-kubernautagent GINKGO_FOCUS=2459
make test-e2e-kubernautagent GINKGO_FOCUS=2459
make gen-diff
go build ./...
golangci-lint run --timeout=5m
```

The live Catalog IT and post-fix E2E are now green; the E2E proves both remediation-scoped discovery reconstruction and the #2466 audit boundary. A prior aggregate `make test` rerun returned non-zero for two SignalProcessing shutdown timing assertions under 12-way parallel load; the focused shutdown specs pass in isolation and no SignalProcessing files changed. Generated artifacts were regenerated and embedded OpenAPI copies match; `make gen-diff` was not run because the worktree is intentionally uncommitted.

## 9. Test deliverables and wiring verification

### Wiring manifest

| Component | Production entry point | Wiring location | IT/E2E proof |
|---|---|---|---|
| Step 1 action result capture | KA tool registry / `list_available_actions` | `cmd/kubernautagent/toolregistry.go` → `internal/kubernautagent/tools/custom/tools.go` → `discovery_audit.go` → typed payload builder | IT-KA-2459-001; E2E-KA-2459-001 |
| Step 2 action/candidate/score capture | KA tool registry / `list_workflows` and cache-backed Catalog | `cmd/kubernautagent/toolregistry.go` → `tools/custom/tools.go` → `workflowcatalog/discovery_cache.go` → `discovery_audit.go` | IT-KA-2459-001; E2E-KA-2459-001 |
| Buffered typed persistence and correlation query | KA production audit path to Data Storage | `cmd/kubernautagent/datastorage.go` → `audit.NewBufferedDSAuditStore` → `ds_buffered_store.go` typed payload builder → Data Storage; separate authenticated Ogen query client | IT-KA-2459-001; E2E-KA-2459-001 |

### Deliverables

- Passing Ginkgo/Gomega test cases above, generated OpenAPI artifacts, updated KA audit event catalog, approved design-decision entry, and recorded build/lint/test results.

## 10. Dependencies and execution order

| Dependency | Type | Status | Impact if unavailable | Workaround |
|---|---|---|---|---|
| Ogen v1.20.1 generation target and OpenAPI linting | Tooling | Available; generation completed and generated union/client packages compile | Generated union/client mismatches could break typed reads/writes. | Re-run generated-artifact consistency when final source is stable. |
| KA custom-tools envtest etcd + Data Storage/Postgres/Redis harness | Test infrastructure | `IT-KA-2459-001` and live Catalog `IT-KA-2466-001` passed | Confirms the CRD source/catalog projection and real Data Storage audit path independently. | Keep live persistence/query assertions as regression gates. |
| KubernautAgent Kind E2E stack | Test infrastructure | `E2E-KA-2459-001` passed after #2466; local artifacts reused through `KUBERNAUT_CI_ARTIFACT_TAG` | Confirms remediation-ID reconstruction and raw audit minimization through production dispatch. | Keep the default CI build path intact; use the supported artifact path for local retries when VM capacity is constrained. |

Execution order follows TDD and the pyramid: (1) write UT RED for result fidelity, sensitive-data exclusion, and typed compatibility; (2) write IT RED through etcd-backed Catalog and production-registered tools to the real buffered DS audit path, flush, then query; (3) add E2E hard assertions over production-written DS records; (4) implement minimum GREEN until UT and IT pass; (5) refactor with tests green; (6) verify all wiring rows and run E2E/regression suites.

## 11. Existing tests requiring updates

| Test location | Current assertion | Required change | Reason |
|---|---|---|---|
| `test/e2e/kubernautagent/three_step_discovery_test.go` | Deterministic three-step discovery journey used a static remediation ID and did not assert the full persisted Step 1/Step 2 result payload. | DONE: uses a unique remediation ID, queries production-written Data Storage records, and hard-asserts typed payload content, score/rank, protection boundary, selected-candidate membership, and the #2466 omission marker. `E2E-KA-2459-001` passes. | Prevents stale-event contamination and proves SOC 2 CC7.2 reconstruction instead of event presence alone. |
| `test/e2e/kubernautagent/audit_pipeline_test.go` | Existing general audit check remains best-effort/event-presence oriented. | The targeted three-step discovery scenario now owns the #2459 hard assertions; this unrelated general audit-pipeline scenario was left unchanged. | Keeps #2459 assertions deterministic and scoped without weakening unrelated scenarios. |

## 12. Changelog

| Version | Date | Changes |
|---|---|---|
| 1.0 | 2026-09-26 | Initial draft, Ogen spike, schema prior-art triage, and test-harness preflight; event-specific payload architecture approved, implementation/test plan awaiting approval. |
| 1.1 | 2026-09-26 | Clarified the two-store data flow; IT uses the production buffered audit store with flush + real DS query; E2E queries production-written DS traces; added explicit pyramid evidence and tightened business/control assertions, including the ASVS V16.2.5 objective. |
| 1.2 | 2026-09-27 | Recorded passing live-Catalog IT and post-#2466 E2E; corrected the expected component filter to the RCA-resolved `apps/v1/Deployment` GVK and documented local image-artifact reuse for Podman-constrained retries. |
