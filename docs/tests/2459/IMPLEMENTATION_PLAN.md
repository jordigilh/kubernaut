# Implementation Plan: Issue #2459 Workflow Discovery Audit Results

**Status**: #2459 and #2466 implementation complete. Focused unit tests, live Data Storage audit IT, live etcd-Catalog IT, and post-fix focused Kind E2E pass. The E2E verifies remediation-ID decision reconstruction and the generic `get_workflow` audit omission boundary.
**Branch**: `fix/2459-workflow-discovery-audit` (based on `origin/main` `faa9468`)
**Business requirement**: BR-AUDIT-023, with BR-AUDIT-025/026 and BR-AUDIT-005/030
**Test plan**: [TP-2459-v1.2](TEST_PLAN.md)

## 1. Preflight findings

- `cmd/kubernautagent/toolregistry.go:84` wires `custom.RegisterAll` with the production `WorkflowCatalog` and audit store.
- `internal/kubernautagent/tools/custom/tools.go` receives both Step 1 action entries and Step 2 workflows, but `discovery_audit.go` currently records only counts, filters, and (in memory only) the Step 2 `action_type`.
- `internal/kubernautagent/audit/ds_workflow_catalog_payloads.go` builds the typed Data Storage payload but does not map `action_type` or result records. The OpenAPI contract currently maps all four discovery event types to one `WorkflowDiscoveryAuditPayload`, despite Step 1 and Step 2 having different result shapes.
- The existing OpenAPI `WorkflowResultAudit` requires `scoring.confidence`, while BR-AUDIT-026 calls for the query-computed `finalScore` and KA keeps it audit-only. Schema prior art in `WorkflowSearchResult` explicitly defines `confidence` and `finalScore` as the same normalized label-ranking score. Preserve the existing nested `scoring.confidence` for compatibility and add explicit audit `final_score`; both must be populated from the same cache-computed value, not from LLM decision confidence.
- `internal/kubernautagent/workflowcatalog/discovery_cache.go` computes `final_score` to sort workflows and then discards it before returning. Capturing the true score requires carrying it across the Catalog/tool boundary; calculating a replacement in the audit layer would be incorrect. The score helper is also used by Step 1 to count matching workflows, so that path must retain its count semantics.
- The current E2E suite has a deterministic three-step OOM discovery scenario in `test/e2e/kubernautagent/three_step_discovery_test.go`. A hard query-by-remediation-ID assertion can replace the best-effort-only coverage in `audit_pipeline_test.go`.
- Existing detected-label support is already present on this branch. The regression suite will retain ArgoCD, Flux, and `failedDetections` payload coverage while adding the missing result data.

### Bounded spike — 2026-09-26

**Question:** Can Ogen generate separate event-specific payload variants while accepting the current legacy Step 1/Step 2 payload shape in both request and query-response unions?

**Prototype:** In an isolated temporary copy of the current full Data Storage OpenAPI document, mapped `actions_listed` and `workflows_listed` to separate schemas, narrowed the legacy shared schema to the two Step 3 event types, and regenerated with the repository-pinned Ogen v1.20.1. A scratch Go test decoded and validated old-shape Step 1/Step 2 data through both generated unions.

**Result: YES.** Ogen generated distinct request/response variants and typed getters. Both old-shape payloads decoded and validated through the new discriminator mapping. The experiment also confirms an important requirement: remove the Step 1/2 enum values from the legacy shared schema, or `oneOf` can match both the legacy and new schema. The spike artifacts are outside the worktree at `/private/var/folders/r7/gktmmltd1zq7wqhsjjslwsm80000gn/T/opencode/ka2459-schema-spike.n6qh4i`; no production OpenAPI or generated files were changed.

**Score-path preflight:** Engram reference lookup confirms `filterAndScoreCachedWorkflows` is called by the Step 1 match-count path and Step 2 list path. It sorts on the internal `scoredWorkflow.score`, then projects to `[]models.RemediationWorkflow`, dropping the score. `ListWorkflowsByActionType` flows through Catalog/LazyCatalog to `listWorkflowsTool.Execute`; the tool separately maps model fields through `convertWorkflowsToDiscoveryEntries`, an allowlist that has no score property. Recommended handoff: retain the existing list API for current callers, add a score-bearing result method for the Step 2 tool, and have that tool build its existing LLM entry from each candidate's workflow while audit serialization consumes the same candidate's score. Step 1 continues to use the scored helper only for its match count. This localizes production changes to Catalog/LazyCatalog, the custom-tool interface and its small set of test doubles, and the tool/audit builder; `cmd` registration remains unchanged. Existing ordering behavior is verified by `go test ./internal/kubernautagent/workflowcatalog -ginkgo.focus=UT-KA-1677-614-003 -count=1` (PASS).

**Split-store integration preflight (Kubernetes etcd catalog + Data Storage audit):** The closest existing harness is `test/integration/kubernautagent/tools/custom/`, not the investigator suite. It starts envtest (Kubernetes API server backed by etcd), seeds ActionType and RemediationWorkflow CRDs into that API server, and constructs KA's informer-backed Catalog over those CRDs. Separately, it starts real Data Storage/PostgreSQL/Redis. `integration.NewAuthenticatedDataStorageClients` exposes both the authenticated audit client and an Ogen query client. Discovery reads the Kubernetes CRDs through the informer cache; Data Storage is the audit sink, not the catalog source. Ran legacy `IT-KA-433-034` (PASS, 100.9s) with cached envtest v1.36 assets. Its title/assertions are stale: the current path proves CRD/etcd → informer Catalog → discovery tool, while the suite's Data Storage service is present but the test passes a `nil` audit store. Thus it does **not** prove audit persistence/query. The #2459 IT must use the production `audit.NewBufferedDSAuditStore` over the suite's real authenticated audit client, register discovery tools through `custom.RegisterAll`, execute Step 1/2 with a unique remediation ID, explicitly flush the async buffer, and query Data Storage through the separate Ogen query client. This proves both independent persistence paths without substituting an in-memory audit capture.

**E2E infrastructure preflight:** The suite provisions Kind (Kubernetes API/etcd for workflow CRDs), Data Storage/PostgreSQL, Mock LLM, and deployed KA. The initial E2E proved the generic `aiagent.llm.tool_call` record persisted the complete `get_workflow` result, exposing #2466. The #2466 fix minimizes the tool-call result and in-memory request-history audit snapshots while retaining the full allowed result for the LLM. Post-fix verification passed against production-written Data Storage records. A later assertion failure was traced through must-gather-exported KA logs to RCA re-enrichment: the original Pod target resolved to `apps/v1/Deployment`, and the tool correctly filtered by that target GVK; the test was corrected to assert the resolved component.

**Schema prior-art triage:** `api/openapi/data-storage-v1.yaml`'s `WorkflowSearchResult` (lines 2437–2517) requires both `confidence` and `finalScore`; `confidence` is documented as the normalized label score and `finalScore` as the same value. `pkg/datastorage/models/workflow.go` mirrors that definition. This is a deprecated search-response schema/model rather than an active endpoint—the OpenAPI comments say discovery ownership moved to KA and the search endpoint was removed—so reuse the established score-field semantics, not the legacy DTO wholesale. The active audit `WorkflowResultAudit` already requires `scoring.confidence`; add explicit audit `final_score`, populate both from the same cache-computed score, and document the nested field as a legacy alias for the catalog ranking score (not the LLM's decision confidence). Do not copy the old schema's execution-bundle or parameter fields into audit output.

**Reassessed preflight confidence: 94% (at plan approval).** The Ogen union/legacy-shape risk was experimentally resolved; the existing schema/model gave direct prior art for score-field compatibility; the baseline ranking test passed; and the focused integration harness proved the etcd-backed CRD catalog and real Data Storage service can coexist. At this checkpoint the production buffered audit path and Kind E2E were still unproven. The user subsequently authorized execution of the plan; current evidence is recorded in §9.

## 2. Architecture alternatives and recommendation

| Option | Approach | Pros | Cons |
|---|---|---|---|
| **A — Audit-only identity/rank** | Persist Step 1 actions and Step 2 action type/candidate identity/order/page context, but do not carry `final_score` out of the Catalog. | Smallest behavior change; still reconstructs which candidates were returned. | Leaves BR-AUDIT-026 unmet and loses the actual ranking signal already computed by KA. |
| **B — One shared typed payload** | Keep all discovery events mapped to one payload and add optional action results, workflow results, action type, and score fields. | Lowest OpenAPI union/codegen change; common envelope is reused. | Permits impossible/cross-step field combinations and grows a shared schema with unrelated optional fields; less precise for generated clients and future event evolution. |
| **C — Event-specific typed result payloads (approved)** | Map `workflow.catalog.actions_listed` and `workflow.catalog.workflows_listed` to distinct typed payload schemas. Reuse common filter, pagination, timing, and envelope components; leave retrieval/validation schemas and all event names unchanged. Carry the actual cache `final_score` only into the Step 2 audit result. | Strong event-to-payload typing; each step can evolve and validate its own required result shape; avoids a cross-step optional-field bag; meets #2459 and BR-AUDIT-025/026 while keeping scores out of the LLM response. | More OpenAPI discriminator/union and generated-client work; requires explicit old/new payload compatibility testing. |
| **D — Untyped extra JSON/map** | Place results in unconstrained generic event data without generated typed schemas. | Superficially fewer generated edits. | Rejected: bypasses DD-AUDIT-004 and weakens validation and consumer contracts. |

**Selected architecture: Option C, approved by the user.** Add distinct payload definitions for the two result-bearing event types and map them through the existing event-type discriminator. Preserve common typed components rather than duplicating filters and timing. The Actions Listed result captures the actual action entries returned; the Workflows Listed query captures `action_type`, and its results capture returned candidate identity/version, 1-based rank, and actual `final_score`. `total_found` and `returned` remain distinct. New result fields stay optional at the wire-schema boundary for historical payload compatibility, while the current producer must populate them. Workflow parameter schemas and execution bundles remain excluded. The architectural direction is recorded in [DD-AUDIT-009](../../architecture/decisions/DD-AUDIT-009-workflow-discovery-event-specific-payloads.md).

## 3. TDD phases and estimates

### RED — 3–4 hours

1. Add failing Ginkgo/Gomega builder tests for each distinct payload variant: Step 1 action entries; Step 2 `action_type`, candidate identity/order/score; equal `final_score` and legacy `scoring.confidence`; page-vs-total counts; old payload compatibility; and label variants.
2. Add failing production-tool tests proving the score appears only in audit data, not the LLM response.
3. Add a failing IT that invokes the production-registered discovery tools against the etcd-backed Catalog with a real `BufferedDSAuditStore`; flush it, then query the real Data Storage service by remediation ID and assert persisted business evidence.
4. Strengthen the deterministic three-step AgentSession E2E scenario with a unique remediation ID and hard persisted-payload assertions from the real Data Storage query API; do not use an in-memory audit capture.

### GREEN — 4–6 hours

1. Add event-specific Step 1/Step 2 payload definitions, explicit `final_score` plus the compatible nested `scoring.confidence` mapping, and event-type discriminator mappings to `api/openapi/data-storage-v1.yaml`; clarify that the nested score is the catalog ranking score, not LLM decision confidence; regenerate the copied middleware schema and ogen client via repository generation targets.
2. Carry `final_score` and order from the cache-backed Catalog through a focused discovery result type; retain current sorting and preserve the exact LLM-facing JSON contract.
3. Capture Step 1 entries and Step 2 action/candidate/page data at the existing tool execution points; map each event into its own generated typed payload variant.
4. Verify `cmd/kubernautagent/datastorage.go` still constructs the production `BufferedDSAuditStore` and `cmd/kubernautagent/toolregistry.go` wires that store into the discovery tools; pass the real-DS integration test before declaring GREEN complete.

### REFACTOR — 1–2 hours

1. Centralize conversion from discovery results to typed audit result records and avoid duplicate mapping between builder/test fixtures.
2. Keep audit-only score data structurally separate from the LLM-facing response.
3. Update the KA audit event catalog and verify it agrees with approved DD-AUDIT-009; document current Go/KA ownership and each event's exact payload.
4. Post-refactor validation (mandatory safety net): `go build ./...`, focused and affected test suites, `make gen-diff`, and lint. REFACTOR's improvement is the explicit conversion boundary and removal of duplicated payload mapping, not the validation commands.

## 4. Wiring manifest

| Component | Production entry point | Wiring code | Integration proof |
|---|---|---|---|
| Step 1 returned action metadata | `list_available_actions` registered by KA | `cmd/kubernautagent/toolregistry.go:84`; `internal/kubernautagent/tools/custom/tools.go`; `discovery_audit.go`; `ds_workflow_catalog_payloads.go` | IT-KA-2459-001; E2E-KA-2459-001 |
| Step 2 action, candidates, rank, and actual score | `list_workflows` → cache-backed Catalog | `cmd/kubernautagent/toolregistry.go:84`; `tools/custom/tools.go`; `workflowcatalog/discovery_cache.go`; `discovery_audit.go` | IT-KA-2459-001; E2E-KA-2459-001 |
| Buffered typed DS persistence/query by remediation ID | Production KA audit path | `cmd/kubernautagent/datastorage.go` → `internal/kubernautagent/audit/ds_buffered_store.go` → typed payload builder/OpenAPI union → Data Storage; query with the authenticated Ogen client | IT-KA-2459-001 (real buffer + explicit flush + DS query); E2E-KA-2459-001 (deployed buffer + DS query) |

## 5. Success criteria

- All TP-2459 unit, integration, and E2E scenarios pass with business-outcome assertions.
- In both IT and E2E, querying real Data Storage by remediation ID alone reconstructs Step 1 choices and Step 2 candidates and proves the selected candidate was returned; neither tier substitutes an in-memory audit store.
- Each workflow result carries its real rank and computed score in audit data; score remains absent from LLM output.
- Filter context and the standard audit envelope remain intact; event-specific typed variants accept prior payloads and query results remain decodable with both explicit and compatibility score fields.
- `make gen-diff`, `go build ./...`, `golangci-lint run --timeout=5m`, and `make test` pass; wiring manifest is complete.

## 6. Pre-implementation readiness audit

This table records the GA-readiness assessment at the pre-implementation planning checkpoint. Current execution evidence and remaining gates are recorded in §9.

| Dimension | Planned evidence / completion gate |
|---|---|
| Build | `go build ./...` after implementation/refactor. |
| Lint | `golangci-lint run --timeout=5m`; no new findings. |
| Unit tests | Ginkgo/Gomega for business behavior and schema mappings; 100% coverage of new unit-testable business logic. |
| Integration tests | Real etcd-backed Catalog → production-registered KA discovery tools → real `BufferedDSAuditStore` → Data Storage persistence; flush and query Data Storage by remediation ID. |
| Wiring | All three manifest rows have passing IT proof; production `cmd/kubernautagent/toolregistry.go` call path is unchanged and verified. |
| BDD framework | New behavior tests use Ginkgo/Gomega; no business `testing.T` tests. |
| Test IDs | All new tests use `UT-KA-2459-*`, `IT-KA-2459-*`, or `E2E-KA-2459-*`. |
| SOC 2 / FedRAMP / ASVS | The BR/control matrix maps AU-2 event generation, AU-3 record content, CC7.2 reconstruction, ASVS V16.2.1 investigation metadata, V16.2.4 machine-readable correlation, and V16.2.5 protection-level handling of logged data to business-level assertions over persisted records. |
| Go anti-patterns | Check changed Go files for parameter count, nesting, ignored errors, shadowing, context storage, and unnecessary interface/type complexity. |
| BR satisfaction | Verify BR-AUDIT-023/025/026 and correlation-query behavior under BR-AUDIT-005/030. |
| Regression | Old payload decode/acceptance tests plus affected KA, Data Storage, and full `make test` suites. |
| Fail-open safety | Audit persistence remains best-effort and must not change discovery result/error semantics; failure behavior remains observable through existing logging. |
| Domain-specific | Event discriminator/payload consistency, event identity/correlation/cluster context, score provenance, and exclusion of parameter schemas/execution bundles. |

## 7. Risks and rollback

- **Schema/score compatibility:** new `final_score` remains optional for historical events; new events populate it and legacy `scoring.confidence` from the same score. Verify old reads and new Data Storage writes/queries in tests. Rollback can revert producer/schema additions together without changing event types or query endpoints.
- **LLM contract drift:** assert byte/field-level response behavior in unit tests; do not place score on `WorkflowDiscoveryEntry`.
- **Score mismatch:** source the score from the exact cache computation used for ordering; test known inputs against the resulting value; never infer it from rank.
- **Test infrastructure:** UTs are local; IT extends the custom-tools envtest etcd + real Data Storage bootstrap and uses `BufferedDSAuditStore.Flush` as a deterministic write barrier before querying; E2E uses the suite-provisioned Kind stack and bounded polling for its asynchronous production audit writes. Polling only waits for the required event types; both tiers must hard-assert complete payloads afterward, never downgrade to best-effort event-presence warnings.

## 8. Approval gate and authorization

The user approved architecture Option C and authorized execution of the implementation/test plan. RED → GREEN → REFACTOR was carried out; each wiring row has focused unit and IT coverage, and the focused post-fix E2E passes.

## 9. Execution and verification record

| Gate | Result | Evidence / limitation |
|---|---|---|
| Focused unit tests | PASS | `go test ./internal/kubernautagent/audit ./internal/kubernautagent/tools/custom ./internal/kubernautagent/workflowcatalog -ginkgo.focus=2459 -count=1`. |
| Focused live Data Storage IT | PASS | Initial `IT-KA-2459-001` passed with the real etcd-backed catalog, buffered DS audit writes, explicit flush, remediation-ID query, and synthetic parameter sentinel check (111.865s). The retry, including raw API checks for excluded fields/sentinel, passed in 129.553s. |
| Pre-fix E2E finding | FIXED | The first focused run found the seeded parameter sentinel in raw Data Storage because generic `aiagent.llm.tool_call` audit serialized the full `get_workflow` result; #2466 now stores the omission marker while retaining the LLM result. |
| Focused post-fix E2E | PASS | `E2E-KA-2459-001` passed (1/1) against deployed KA/Mock LLM/real Data Storage. Assertions cover event envelope, Step 1 actions, Step 2 action/candidates/rank/score, selected candidate membership, the omission marker, and raw sentinel exclusion. The component filter is asserted as the RCA-resolved `apps/v1/Deployment` GVK. |
| #2466 live Catalog IT | PASS | `IT-KA-2466-001` passed (1/1), querying the seeded etcd-backed CRD through the live Catalog and production-registered tool. |
| #2466 focused UTs | PASS | Both discovery projection and investigator audit-boundary unit suites pass; the latter asserts full tool result remains in the model conversation but is omitted from `aiagent.llm.tool_call` and subsequent `aiagent.llm.request` audit history. |
| Full unit regression | PASS (earlier run) | `make test`: 25 unit suites passed; no failures. |
| Post-#2466 all-services unit rerun | PARTIAL | Aggregate `make test` returned non-zero in two timing-sensitive `pkg/signalprocessing/controller_shutdown_test.go` assertions under 12-way load. The `Controller Shutdown` specs pass when run in isolation; no SignalProcessing files are changed in this branch. |
| Full test-package compilation | PASS | `go test ./... -run=^$ -timeout=30s`. |
| Test-package compilation | PASS | `go test ./test/integration/kubernautagent/tools/custom ./test/e2e/kubernautagent -run=^$ -count=1`. |
| Build | PASS | `go build ./...`. |
| Embedded OpenAPI copies | PASS | Both `pkg/audit/openapi_spec_data.yaml` and `pkg/datastorage/server/middleware/openapi_spec_data.yaml` compare byte-for-byte with `api/openapi/data-storage-v1.yaml`; the Ogen client was regenerated from the updated contract. |
| Formatting / diff whitespace | PASS | `gofmt` on edited Go files and `git diff --check`. |
| Lint | PASS (branch delta) | `golangci-lint run --new-from-rev=origin/main --timeout=5m`: 0 issues. Full-repository lint still reports 22 baseline findings outside this change. |
| Generated diff gate | NOT RUN | `make gen-diff` compares the entire worktree to a clean index and cannot pass while this implementation is intentionally uncommitted. The Ogen client was regenerated and both embedded OpenAPI copies compare byte-for-byte with the source contract. |

The local E2E image build initially exhausted the Podman VM disk. With explicit user approval, dangling images were pruned and eight stopped Buildah work containers from the failed run were removed; the active Engram container, two older Buildah containers, volumes, and tagged source images were preserved. To avoid repeating three `--no-cache` builds, the successful E2E reused the already-built images through the harness-supported `KUBERNAUT_CI_ARTIFACT_TAG` path. The new `.containerignore` excludes only host-side generated artifacts/caches from root-context image builds. Untracked CI-image archives and unrelated Kind configs in the worktree were preserved and not staged.
