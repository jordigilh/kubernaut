# Test Plan: Target-scoped RCA API type validation

**Test Plan Identifier**: TP-2609-v1 | **Feature**: target-cluster GVK discovery and safe workflow selection
**Version**: 1.8 | **Created**: 2026-11-06 | **Author**: Kubernaut contributors
**Status**: Implementation and local/remote verification complete; final aggregate repository gate passes | **Branch**: `triage/v16-rr-cancellation`

## 1. Introduction

### 1.1 Purpose
Prove that local and fleet investigations use target-scoped discovery, that the LLM chooses a type using evidence and the harness independently verifies it, and that no unverifiable target can select an unsafe workflow. Registered type and named-object existence are distinct.

### 1.2 Objectives and 1.3 success metrics
All 16 scoped UT scenarios are tracked. The focused #2588/#2609 investigator regression set passes **14/14**, the broader `TP-1044|RunWorkflowDiscoveryFromRCA` regression set passes **42/42**, and the complete investigator suite passes **490/490**; the MCP tools suite passes **275/275** and the K8s tools suite **132/132**. The five focused investigator-entry integration specs, two E2E specs, and adapted legacy API-gate integration specs pass remotely. `-race`, `go build ./...`, compile-only validation, and lint also pass. The focused IT specs use controlled external dependencies and target-scoped registry/overlay fixtures; they do not by themselves prove full service bootstrap or gateway deployment wiring. >=80% merged-tier structural coverage is the separate CI metric, not a substitute for scenario proof. No individual tier line-percentage gate is imposed by this plan; AGENTS.md supersedes the older template's line-based policy.

## 2. References

Authority: [#2609](https://github.com/jordigilh/kubernaut/issues/2609), [BR-KA-212](../../requirements/BR-KA-212-rca-target-resource.md), BR-WORKFLOW-004, BR-AUDIT-005, BR-INTEGRATION-054/065, [DD-WORKFLOW-003](../../architecture/decisions/DD-WORKFLOW-003-parameterized-actions.md), [DD-FLEET-005](../../architecture/decisions/DD-FLEET-005-cluster-transparent-tool-exposure.md), ADR-068, AGENTS.md. Cross-reference [implementation plan](IMPLEMENTATION_PLAN.md), [test template](../../testing/TEST_PLAN_TEMPLATE.md), [upstream PR 1395](https://github.com/containers/kubernetes-mcp-server/pull/1395), [#1039](https://github.com/jordigilh/kubernaut/issues/1039), [#2588](https://github.com/jordigilh/kubernaut/issues/2588) (lock design excluded).

## 3. Risks and mitigations

| Risk | Impact / likelihood | Proving scenarios | Mitigation |
|---|---|---|---|
| R1 wrong cluster or group | Critical / medium | UT-001–004, 011, 013; IT-002/004; E2E-001/002 | scoped context, exact GVK, no hub fallback, least privilege |
| R2 incomplete, denied or non-preferred data misread as absence | Critical / high | UT-005–008, 012; IT-005; E2E-002 | exact positive verification, no unsupported absence/uniqueness inference, and review on inconclusive evidence |
| R3 missing named object treated as invalid/stale | Critical / medium | UT-009/010; IT-003; E2E-001 | investigate evidence separately from type registration |
| R4 parameter/selection cross-contamination or race | Critical / medium | UT-011/013/014; IT-003/004; E2E-001 | exact target, verified workflow component, race checks |
| R5 unsupported tool or lost audit evidence | High / medium | UT-006/015/016; IT-001/002/005; E2E-002 | real production wiring, structured result and correlation checks |

All IDs in this section abbreviate the full `UT-KA-2609-NNN`, `IT-KA-2609-NNN`, and `E2E-KA-2609-NNN` IDs below.

## 4. Scope

In scope: RCA tool exposure/call, exact type registration verification, preferred-version discovery interpretation, signal versus remediation target, safe retry/review, workflow component matching/registration and target parameter propagation, local/fleet context, concurrency, audit and authorization. Out of scope: #2588 lock implementation, general cluster provisioning, unsupported historical-version guessing, adding dependencies without approval, arbitrary workflow behavior or production cluster changes. No mixed-version/backward-compatibility requirement is imposed where the user approved its removal; affected security, catalog and behavior regressions remain mandatory.

Resolved design decisions are in [implementation plan §Decision gate](IMPLEMENTATION_PLAN.md#decision-gate--resolved-before-implementation). Upstream returns preferred versions only and silently filters denied resources; no-warning does not establish completeness. Missing non-preferred GVK in preferred-only output is inconclusive, never an automatic invalid verdict. The LLM must provide an exact GVK, and the harness independently verifies that GVK using target-scoped discovery; a missing named object never authorizes historical-version inference or preferred-version substitution. The full verified GVK is used in the existing workflow component selector without a new separate API-version requirement.

## 5. Approach

Ginkgo/Gomega UT prove business scenarios; IT prove real production dispatch and E2E prove control journeys. Use real internal business logic; external LLM/Kubernetes/network dependencies may be controlled in UT, not internal decision logic. IT/E2E use real integrations per repository no-mocks policy; external LLM may use the project's approved deterministic test service if available, with its status disclosed. Scenarios begin as **planned**; existing RED tests establish the starting failures. No pending/skip test placeholders in executable suites.

**Pass**: every P0 scenario passes, >=15/16 scenarios and 14/14 critical UT scenarios proven, 5/5 focused investigator-entry IT pass, applicable control E2E pass, affected build/lint/regressions pass. Full bootstrap/gateway wiring remains a separate manifest gate. **Fail**: any critical gap, invalid validation boolean, wrong target, selection on inconclusive evidence, or unproven required wiring. **Suspend** affected verification when required target transport, image capability or an authorized isolated environment is unavailable; exercise the approved review path when selected GVK verification is inconclusive. Report blocked rather than infer success.

## 6. Test items and versions

| Item | Scope and evidence |
|---|---|
| `internal/kubernautagent/investigator/investigator_gates.go` | Gate must independently verify exact target-scoped GVK and keep inconclusive results in review |
| `internal/kubernautagent/investigator/investigator_phases.go` | Namespace normalization, authoritative target, parameter injection and cache identity |
| `internal/kubernautagent/investigator/fleet_overlay.go` | Per-investigation context, fail-closed overlay |
| `cmd/kubernautagent/bootstrap.go`, `health.go`, registry/bridge | local/fleet production wiring and target-scoped discovery dispatch |
| `internal/kubernautagent/investigator/apiversion_gate_test.go` | Existing tests, including user's two uncommitted safety REDs |
| `github.com/containers/kubernetes-mcp-server` | Uncommitted module v0.1.2, fleet E2E image digest pinned; deployed image/tool exposure unverified |

## 7. BR and control coverage matrix

UT evidence is complete for the local verification scope. Focused investigator-entry IT evidence is complete for 5/5 scenarios; full bootstrap/gateway wiring is not claimed by those fixtures. UT IDs refer to §8; IT/E2E remain independently required at their stated boundary.

| Requirement/control | Objective | UT | IT | E2E |
|---|---|---|---|---|
| BR-KA-212, BR-WORKFLOW-004 | verified target and compatible workflow | 001–014 | 001–004 | 001 |
| BR-INTEGRATION-054/065; AC-6; ASVS 4.0.3 V4.1.5 | correct scoped cluster, deny on failure, least privilege | 001, 005–008, 013 | 002, 004, 005 | 002 |
| BR-AUDIT-005; SOC2 CC7.2; AU-2/AU-3 | correlation-linked event reconstruction, actor/action/outcome, source and failure evidence | 015 | 005 | 002 |
| SI-10 | validate untrusted tool/model values, exact types and malformed data | 003, 004, 006, 012, 016 | 005 | 002 |

Audit assertions cover `event_id`, `event_type`, `event_action`, `event_outcome`, `actor_type`, `actor_id`, `correlation_id`, typed event data, cluster attribution, provenance and standardized error details where applicable. Existing event catalogs/control mappings must be checked and updated for actual changes; do not claim a new event exists yet.

## 8. Test scenarios

### Unit scenarios — implementation/verification tracked

| ID | P | Business outcome |
|---|---|---|
| UT-KA-2609-001 | P0 | Same Kind registered on local and remote with different groups: only requested target's cluster establishes authority, no remote hub fallback. |
| UT-KA-2609-002 | P0 | Ambiguous Kind across groups: LLM calls `api_resources_list`, uses investigation evidence to choose exact group/version; harness independently verifies. Unresolved ambiguity requires review. |
| UT-KA-2609-003 | P0 | Invalid, adversarial or unregistered group/version from initial answer or retry is not marked validated and cannot select workflow; only conclude invalid when evidence permits. |
| UT-KA-2609-004 | P0 | Kind substring results and plural/resource-name collisions are exact-filtered by Kind; scope/name resolution uses discovered resource metadata, not guessed plural. |
| UT-KA-2609-005 | P0 | Partial warning, timeout, authorization error, unavailable tool, nil resolver, malformed response or failed transport yields inconclusive/review; no selected workflow or false validated boolean. |
| UT-KA-2609-006 | P0 | Structured `{resources, warning}` and actual transport text/error envelopes preserve warning and exact source; missing warning is not proof filtering did not occur. |
| UT-KA-2609-007 | P0 | Silent `denied_resources` filtering, including group/version denial, cannot establish uniqueness or absence; no bypass or privilege expansion. |
| UT-KA-2609-008 | P0 | Preferred version is per group/resource; for a missing object, evidence selects the group and its tool-preferred version is used without fabricating historical version. A conflicting proposal is not silently rewritten; absent non-preferred version alone is inconclusive, not invalid. |
| UT-KA-2609-009 | P0 | Registered type plus `NotFound` named object does not itself imply stale alert or invalid type; missing resource can be incident #1039. Where `resources_list` is used for evidence, enforce target scope and distinguish denial/error from absence and from independent type verification. |
| UT-KA-2609-010 | P0 | Evidence of a stale alert can yield existing no-action outcome; it is not inferred solely from named-object NotFound. |
| UT-KA-2609-011 | P0 | Signal target differs from RCA target: discovery, workflow selection and injected parameters use exact RCA identity, not signal. |
| UT-KA-2609-012 | P0 | No candidate under partial/filtered discovery is inconclusive; truly complete authoritative absence is distinguished only if proof exists. |
| UT-KA-2609-013 | P0 | Concurrent contexts for different targets, same Kind/name and divergent registration never leak overlays, cached evidence, selection or API versions; exercise `-race`. |
| UT-KA-2609-014 | P0 | Retry or continuation target identity changes (including namespace or cluster with same kind/name/version), racing discovery changes or unrelated human-review reasons cannot reuse validation, bypass revalidation, clear review or retain stale selected workflow. |
| UT-KA-2609-015 | P1 | Success/failure audit records truthful outcome and correlation/cluster/source provenance, with no sensitive object payload. |
| UT-KA-2609-016 | P1 | Core `v1`, preferred group/version and malformed inputs are represented exactly in result strings and structured provenance. |

### Integration — focused investigator-entry evidence, 5/5 passing; full service wiring separate

| ID | Production path and assertion |
|---|---|
| IT-KA-2609-001 | Investigator production entry with a controlled local registry → LLM-visible tool → exact discovery → harness checks the selected type and structured payload. Full service bootstrap remains outside this fixture. |
| IT-KA-2609-002 | Investigator production entry with a controlled target-scoped overlay and contrasting hub/spoke registries → target-only dispatch; verify identity and no hub fallback. Full gateway authentication/deployment wiring remains outside this fixture. |
| IT-KA-2609-003 | Autonomous RCA → catalog registration and selection with component array containing explicit GVKs → exact `TARGET_RESOURCE_*` propagation, supported/unsupported GVK and missing named object. Verify approved runtime/schema contract; workflow owns compatible handling. |
| IT-KA-2609-004 | Real interactive workflow-discovery entry → scoped gate → session snapshot → `kubernaut_select_workflow` → existing approval/handoff boundary. Verify valid exact target/parameters survive; inconclusive or changed identity cannot be made executable by human selection, stale snapshot, continuation or approval. Include differing signal/RCA target, same-kind/name/version namespace change and concurrent target contexts; verify remote discovery occurs after scoping, never via hub mapper. Preserve existing approval policy. |
| IT-KA-2609-005 | Investigator production entry with a controlled discovery error → review/no selection → correlation-linked audit and error details. Full service persistence wiring remains a separate manifest gate. |

### E2E — all planned

| ID | Journey and controls |
|---|---|
| E2E-KA-2609-001 | Authorized disposable local + fleet journey with target-only registration and registered-but-missing object: evidence-driven RCA, exact supported workflow and parameter handoff; also evidence-backed stale/no-action path. BR-KA-212, BR-WORKFLOW-004. |
| E2E-KA-2609-002 | Remote ambiguous/denied/partial case: no hub fallback or unsafe selection; review and correlation query reconstructs audit with cluster and outcomes. Assess CC7.2, AU-2/AU-3, AC-6, SI-10, ASVS V4.1.5. |

No tier is waived. E2E runs only in an explicitly authorized disposable environment; if unavailable, report blocked.

## 9. Test case specifications

All P0 cases share: **Given** target-scoped, controlled real or external-dependency fixture and named evidence; **When** the applicable RCA/tool/gate or continuation path is exercised through the tier's entry; **Then** assert the outcome in §8 and, where applicable, exact target identity, `APIVersionValidated` truth, review state and absence/presence of selected workflow. A legitimate stale/no-action outcome need not produce a validated executable target. Preconditions must declare cluster identity, tool response provenance, discovery completeness, candidate GVKs, name existence and signal/RCA distinction. Tests must capture actual transport payloads rather than invented strings. Critical case details:

- **UT-002/003**: two `Subscription` groups; a deterministic external-model fixture requests the actual tool and chooses a candidate using supplied evidence. Independently vary the harness's discovery result after the model receives its response: a conflicting or fabricated result must not validate. Assert actual dispatch and tool arguments rather than accepting a model claim that it called the tool. On a fabricated retry or changed target, assert no `APIVersionValidated=true` or workflow; do not suppress unrelated review.
- **UT-005–008/012**: exercise real v0.1.2 payloads: warning prefix `some API groups could not be queried and are omitted from this list: `, error prefix `failed to list API resources: `, `resources` and optional `warning`. Silent denied filtering has no indicator: no absence/uniqueness assertion until approved completeness proof exists. Preferred-only omission of non-preferred version is inconclusive.
- **UT-009/010**: same registered kind and `NotFound` name with different incident evidence yields respectively repair investigation or stale/no-action; neither result follows from NotFound alone. Never hallucinate a historical version for an absent object.
- **UT-011/013/014**: cross target identities and interleaved contexts, changed retry and changed discovery; assert exact cluster, GVK/name/namespace, no stale cached output or workflow and safe handling at the approved execution boundary.
- **IT-001–005/E2E-001–002**: reproduce §8 through real entry, transport, result, catalog, interactive selection/approval handoff and audit boundaries. Verify exact target selection, discovery non-resource URL permissions and separate object permissions, registration and propagation, no mocks of internal logic. Test real configured remote credential propagation and rejection; do not assert that metadata visibility alone authorizes object operations. E2E setup requires disposable authorized infrastructure and separately reviewed RBAC.

## 10. Environmental needs

UT: Go, Ginkgo/Gomega, external LLM/Kubernetes fakes only, real internal logic. IT: existing actual KA service/registry, real local and fleet MCP gateway, Kubernetes discovery and catalog/audit infrastructure; inspect actual repository harness before selecting commands. E2E: explicitly authorized isolated disposable hub/spoke environments, pinned MCP image with verified tool availability and controlled access. **This planning task performs no cluster mutations.** Record image digest, configs, tool name/schema, service accounts, cluster identities, source revision and transport provenance with results.

## 11. Dependencies and schedule

Blocking: proving local/remote routing and the configured test transport. The user approved review when the LLM cannot clarify or the selected GVK cannot be verified, and the existing component selector carries the full GVK. PR 1395 is merged; module/image changes are uncommitted and not proof of deployment. Estimated GREEN 1–2 days, REFACTOR 2–4 h, verification 0.5–2 days. Follow RED → minimal GREEN with production wiring and passing IT/CHECKPOINT W → substantive REFACTOR → E2E. Planning confidence remains 95%.

## 12. Deliverables

This plan, [implementation plan](IMPLEMENTATION_PLAN.md), approved design/DD if required, Ginkgo test implementations, source-attributed CI/coverage and race reports, completed control/wiring evidence and test execution record. Production implementation and test evidence are tracked as execution deliverables, not claimed until verified.

## 13. Execution

Implementation and local verification are complete. Evidence recorded on 2026-11-08:

- `go test ./internal/kubernautagent/investigator ./internal/kubernautagent/mcp/tools ./pkg/kubernautagent/tools/k8s -count=1 -timeout=5m` — PASS; investigator 490/490, MCP tools 275/275, and K8s tools 132/132.
- Focused `UT-KA-(2588|2609)` investigator regression set — PASS, 14/14 specs; broader `TP-1044|RunWorkflowDiscoveryFromRCA` regression set — PASS, 42/42 specs.
- `go test -race ./internal/kubernautagent/investigator ./internal/kubernautagent/mcp/tools ./pkg/kubernautagent/tools/k8s -count=1 -timeout=5m` — PASS.
- `go test ./pkg/kubernautagent/tools/registry -count=1 -timeout=5m` — PASS after updating the production registry expectations to 22 baseline K8s tools and 39 total tools.
- `go build ./...`, `go test ./... -run=^$ -timeout=30s`, `golangci-lint run --timeout=5m`, and `git diff --check` — PASS.

Exact #2609 evidence is now **5/5 IT, 2/2 E2E**. Remote evidence was collected on `helios08` in `/root/kubernaut-2609` without transferring the repository:

- `IT-KA-2609-001` through `IT-KA-2609-005` and `E2E-KA-2609-001`/`002` are executable specs in this checkout.
- Local proxy integration attempt: `make test-integration-kubernautagent GINKGO_FOCUS='IT-KA-1044-00[1-5]' TEST_PROCS=1 RACE_FLAG= TEST_TIMEOUT_INTEGRATION=20m` — blocked in `SynchronizedBeforeSuite` while building DataStorage; Podman reported `no space left on device`; **0/175** specs ran.
- Local proxy E2E attempt: `make test-e2e-kubernautagent GINKGO_FOCUS='E2E-KA-1044-001|E2E-KA-1044-002|E2E-KA-DISC-005' KA_E2E_PROCS=1` — blocked while building the KA image with the same Podman storage error; **0/110** specs ran. Teardown confirmed no Kind cluster remained.

After moving to `helios08` and cleaning the authorized disposable Kind clusters, the exact runs completed:

- Exact focused IT: **5/5 passed** (`IT-KA-2609-001` through `005`) in the investigator integration suite with target-scoped registry/overlay dispatch and controlled external dependencies. These runs prove the investigator entry points and safety behavior, not the full bootstrap/gateway wiring rows in §14.
- Exact E2E: **2/2 passed** (`E2E-KA-2609-001` and `E2E-KA-2609-002`). The disposable `kubernaut-agent-e2e` Kind cluster was created and deleted successfully.
- Legacy integration compatibility rerun after fixture adaptation: **5/5 passed** (`IT-KA-1044-001` through `005`).

No production/shared cluster was mutated. Three authorized disposable `helios08` Kind clusters were deleted per user instruction, and the disposable E2E cluster was deleted by teardown.

An earlier repository-wide `make test` run hit one unrelated `pkg/signalprocessing/evaluator` `rego evaluation failed: eval_cancel_error: context deadline exceeded` assertion under parallel aggregate load; the same package passed in isolation with `go test ./pkg/signalprocessing/evaluator -count=1 -timeout=10m`. The final `make test` rerun passed all 26 suites, including the Kubernaut Agent aggregate at 784/784, so the transient timeout is not attributed to the target-scoped change.

## 14. Wiring verification

Use the five-row [wiring manifest](IMPLEMENTATION_PLAN.md#wiring-manifest-and-checkpoint-w): production caller and real passing IT are required for every local, fleet, autonomous, interactive and audit path. The focused investigator-entry runs above pass **5/5**, but the stronger full bootstrap/gateway wiring manifest remains **0/5 demonstrated** in this checkout. CHECKPOINT W is required during GREEN, not deferred until REFACTOR.

## 15. Existing tests requiring updates

| Test | Current behavior | Required treatment |
|---|---|---|
| `UT-KA-2588-001/002` in `apiversion_gate_test.go` | User's uncommitted RED assertions reject unregistered retry and discovery failure | Preserve; adapt fixtures/IDs to approved target-scoped tool flow without weakening assertions. |
| `UT-KA-1044-003`, `UT-KA-1051-011` | Mapper auto-overwrites or supplies API version | Replace assumptions inconsistent with approved LLM/tool and version policy; retain correct core-group and verified target coverage. |
| `UT-KA-1044-004/009` | nil/error resolver skips gate | Update to fail closed for executable unverified targets. |
| `UT-KA-1044-006/022` | unverified nonempty version bypasses retry / adversarial version accepted | Assert independent exact validation, rejection or inconclusive review, not silent acceptance. |
| Retry/history/keepalive and fleet overlay regression suites | submit-only retry and existing context flow assumptions | Adapt only where necessary, preserve audit, reasoning, keepalive, access control and unrelated review assertions. |

## 16. Changelog

| Version | Date | Change |
|---|---|---|
| 1.0 | 2026-11-06 | Draft test plan; all execution and feasibility evidence outstanding. |
| 1.1 | 2026-11-07 | Recorded resolved exact-GVK, target-scoped retry, independent verification, and fail-closed review decisions; marked implementation active with RED evidence. |
| 1.2 | 2026-11-08 | Recorded passing focused/full local suites, race/build/compile-only/lint evidence, and the unrun live IT/E2E boundary. |
| 1.3 | 2026-11-08 | Recorded blocked proxy IT/E2E attempts: exact #2609 specs are absent and Podman storage exhaustion prevented harness setup before any proxy spec ran. |
| 1.4 | 2026-11-08 | Recorded `helios08` proxy results: 2/5 legacy IT pass, 3/3 legacy E2E pass, exact #2609 rows still unexecuted, and remote Kind cleanup verified. |
| 1.5 | 2026-11-08 | Added executable #2609 IT/E2E coverage; `helios08` passed 5/5 IT and 2/2 E2E. Legacy compatibility rerun remains pending after SSH interruption. |
| 1.6 | 2026-11-08 | Confirmed adapted legacy API-gate compatibility at 5/5; removed the temporary remote worktree and verified no Kind clusters remain. |
| 1.7 | 2026-11-08 | Reconciled remote evidence wording: 5/5 focused investigator-entry IT passed, while full bootstrap/gateway wiring remains unproven; retained the 2/2 E2E and 5/5 legacy compatibility results. |
| 1.8 | 2026-11-08 | Replaced stale local totals with reproducible current counts (490/490 investigator, 14/14 focused, 42/42 broader regression, 275/275 MCP, 132/132 K8s), recorded registry expectations, and documented the transient aggregate OPA timeout plus successful final rerun. |
