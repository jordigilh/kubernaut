# Test Plan: af_create_rr Namespace Split — ADR-057 Compliance (Issue #1292)

## 1. Test Plan Identifier

**TP-AF-1292-v1.0**

## 2. References

| Ref | Description |
|-----|-------------|
| [Issue #1292](https://github.com/jordigilh/kubernaut/issues/1292) | af_create_rr creates RR in target workload namespace instead of controller namespace |
| [ADR-057](docs/architecture/decisions/ADR-057-crd-namespace-consolidation.md) | CRD Namespace Consolidation — all CRDs in controller namespace |
| [PR #1284](https://github.com/jordigilh/kubernaut/pull/1284) | AF Agent Quality Fixes (parent PR) |
| [Issue #1282](https://github.com/jordigilh/kubernaut/issues/1282) | AF Agent Quality — namespace, signal grounding, output suppression |
| BR-PLATFORM-057 | CRD namespace consolidation |
| BR-SAFETY-001 | Dedup fingerprint scoped to workload identity |
| BR-AI-056 | Severity triage queries workload namespace |

## 3. Introduction

`HandleCreateRR` uses a single `namespace` parameter for both CRD placement (where the
RemediationRequest Kubernetes object lives) and workload targeting (where the failing
resource resides). After #1282 removed `Namespace` from `CreateRRArgs` and injected
the controller namespace at wiring time, all downstream paths — `targetResource.namespace`,
`deriveSignalName` event queries, severity triage labels, and dedup fingerprint — resolve
to the controller namespace (`kubernaut-system`) instead of the actual workload namespace.

ADR-057 mandates:
- **CRD `metadata.namespace`** = controller namespace (where RO cache watches)
- **`spec.targetResource.namespace`** = workload namespace (where the resource lives)

This test plan defines the behavioral contract for splitting the single namespace into
`controllerNS` (injected at wiring time) and `args.Namespace` (workload NS from the LLM).

## 4. Test Items

| Component | Path | Description |
|-----------|------|-------------|
| HandleCreateRR | `pkg/apifrontend/tools/af_create_rr.go` | RR creation with namespace split |
| HandleCheckExistingRR | `pkg/apifrontend/tools/af_check_existing_rr.go` | Dedup check with controllerNS listing + workload fingerprint |
| NewCreateRRTool | `pkg/apifrontend/tools/af_create_rr.go` | Tool wiring — controllerNS closure |
| NewCheckExistingRRTool | `pkg/apifrontend/tools/af_check_existing_rr.go` | Tool wiring — controllerNS injection |
| BuildInstruction | `pkg/apifrontend/agent/prompt.go` | Prompt update — LLM provides workload namespace |
| deriveSignalName | `pkg/apifrontend/tools/af_create_rr.go` | Event queries in workload namespace |
| rrFingerprint | `pkg/apifrontend/tools/af_create_rr.go` | Fingerprint uses workload namespace |
| mock-LLM af_create_rr | `test/services/mock-llm/response/openai.go` | Restores namespace in tool arguments |
| mock-LLM ConfigMap | `test/infrastructure/shared_e2e.go` | Restores namespace in scenario YAML |

## 5. Software Risk Issues

| Risk | Impact | Mitigation |
|------|--------|------------|
| 32 callers of HandleCreateRR need signature update | Compilation failure across 4 files | Mechanical: add controllerNS param in GREEN, update callers |
| 11 callers of HandleCheckExistingRR need signature update | Compilation failure across 3 files | Same mechanical approach |
| Prompt change breaks 2 existing tests | UT-AF-1282-PROMPT-002 and IT-AF-1282-W05 fail | Update assertions in GREEN phase |
| LLM schema change for af_check_existing_rr | Standalone tool now receives controllerNS at wiring | No schema change — args.Namespace stays workload NS |
| FP E2E: memory-eater in kubernaut-system | Both namespaces equal — split is invisible | Existing E2E still passes; new UT proves the split |

## 6. Features to be Tested

### F-NS-SPLIT: Namespace Split (ADR-057)

| ID | Feature | BR |
|----|---------|-----|
| F-NS-SPLIT-01 | RR CRD created in controllerNS (`metadata.namespace`) | BR-PLATFORM-057 |
| F-NS-SPLIT-02 | `targetResource.namespace` set to workload NS | BR-PLATFORM-057 |
| F-NS-SPLIT-03 | Dedup fingerprint computed from workload NS | BR-SAFETY-001 |
| F-NS-SPLIT-04 | `deriveSignalName` queries events in workload NS | BR-AI-056 |
| F-NS-SPLIT-05 | Severity triage labels use workload NS | BR-AI-056 |
| F-NS-SPLIT-06 | Empty workload namespace rejected | BR-SAFETY-002 |
| F-NS-SPLIT-07 | CheckExistingRR lists in controllerNS, fingerprints with workload NS | BR-SAFETY-001 |
| F-NS-SPLIT-08 | Prompt instructs LLM to provide workload namespace | BR-PLATFORM-057 |

## 7. Features Not to be Tested

| Exclusion | Rationale |
|-----------|-----------|
| Severity triage pipeline internals | Covered by existing UT-AF-1282-SIG-* and triage_test.go |
| E2E full pipeline namespace split | FP workload lives in kubernaut-system; same-NS path tested by existing E2E-FP-1189-* |
| LLM prompt compliance (model behavior) | Out of scope — we test prompt content, not model interpretation |

## 8. Approach

### Test Pyramid (Pyramid Invariant)

| Tier | Scope | Test IDs | Coverage target |
|------|-------|----------|-----------------|
| Unit | Namespace routing logic: CRD placement, targetResource, fingerprint, signal, triage, validation | UT-AF-1292-NS-001..007 | >=80% of changed lines in af_create_rr.go, af_check_existing_rr.go |
| Integration | envtest wiring: CRD in controllerNS, targetResource in workloadNS, prompt content | IT-AF-1292-W01, IT-AF-1292-W02 | >=80% of wiring code |
| E2E | FP pipeline exercises full path (same-NS, pre-existing) | E2E-FP-1189-002/003 (existing) | Covered |

### TDD Phases

| Phase | Description | Checkpoint |
|-------|-------------|------------|
| RED | Write 7 UT + 2 IT that define the ADR-057 contract. All must compile and fail. | CHECKPOINT 1 |
| GREEN | Minimal implementation: split namespace in handler, check-existing, prompt, wiring. All RED tests pass. | CHECKPOINT 2 |
| REFACTOR | 100-go-mistakes fixes (#49, #40, #53), mock-LLM restore, godoc, update existing tests. | CHECKPOINT 3 |

### Anti-Patterns Avoided

| Anti-Pattern | How avoided |
|--------------|-------------|
| Skip() / XIt | All tests fully implemented or not created |
| GREEN Complexity | Minimal implementation — no optimization in GREEN |
| REFACTOR Creation | No new types or features in REFACTOR — only quality improvements |
| Discovery Skip | Existing implementation searched via 4 parallel explore agents before plan |
| Mock Overuse | Only K8s dynamic client mocked (external dependency); business logic is real |
| time.Sleep in tests | Use Eventually/Consistently from Gomega |

## 9. Item Pass/Fail Criteria

| Criterion | Threshold |
|-----------|-----------|
| Unit test pass rate | 100% |
| Integration test pass rate | 100% |
| Unit coverage on changed lines | >=80% |
| Build clean (`go build ./...`) | 0 errors |
| Vet clean (`go vet ./...`) | 0 errors |
| No Skip() or XIt | 0 instances |
| 100-go-mistakes findings addressed | All Medium+ findings |

## 10. Test Deliverables

| Deliverable | Location |
|-------------|----------|
| Test Plan | `docs/tests/1292/TEST_PLAN.md` |
| Unit Tests | `pkg/apifrontend/tools/af_create_rr_test.go` |
| Unit Tests (check) | `pkg/apifrontend/tools/af_check_existing_rr_test.go` |
| Integration Tests | `test/integration/apifrontend/create_rr_wiring_test.go` |

## 11. Test Scenarios

### 11.1 Unit — Namespace Split (af_create_rr)

| ID | Description | Input | Expected | BR |
|----|-------------|-------|----------|-----|
| UT-AF-1292-NS-001 | Cross-namespace RR creation | controllerNS=`kubernaut-system`, args.Namespace=`production` | metadata.namespace=`kubernaut-system`, targetResource.namespace=`production` | BR-PLATFORM-057 |
| UT-AF-1292-NS-002 | Dedup fingerprint uses workload NS | Pre-seed RR with fingerprint(`production/Deployment/web`), create with same workload args | `already_exists=true` | BR-SAFETY-001 |
| UT-AF-1292-NS-003 | deriveSignalName queries workload NS | Events seeded in `production`, controllerNS=`kubernaut-system` | signalName != `unknown` | BR-AI-056 |
| UT-AF-1292-NS-004 | Empty workload namespace rejected | args.Namespace=`""` | ErrInvalidInput | BR-SAFETY-002 |
| UT-AF-1292-NS-005 | Triage labels use workload namespace | controllerNS=`kubernaut-system`, args.Namespace=`production`, triager with mock alert | TriageInput.Labels["namespace"]=`production` | BR-AI-056 |

### 11.2 Unit — Namespace Split (af_check_existing_rr)

| ID | Description | Input | Expected | BR |
|----|-------------|-------|----------|-----|
| UT-AF-1292-NS-006 | CheckExisting lists in controllerNS, fingerprints with workload NS | RR in `kubernaut-system` with fingerprint(`prod/Deploy/web`), check with controllerNS=`kubernaut-system`, args.Namespace=`prod` | `exists=true` | BR-SAFETY-001 |
| UT-AF-1292-NS-007 | CheckExisting returns false when fingerprint uses wrong workload NS | RR with fingerprint(`staging/Deploy/web`), check with args.Namespace=`prod` | `exists=false` | BR-SAFETY-001 |

### 11.3 Integration — Wiring

| ID | Description | Input | Expected | BR |
|----|-------------|-------|----------|-----|
| IT-AF-1292-W01 | envtest: RR created in controllerNS with targetResource in workloadNS | controllerNS=`kubernaut-system`, args.Namespace=`it-workload-ns` | RR fetched from `kubernaut-system`, targetResource.namespace=`it-workload-ns` | BR-PLATFORM-057 |
| IT-AF-1292-W02 | Prompt includes workload namespace instruction | BuildInstruction(`kubernaut-system`) | Contains `namespace` in af_create_rr field list, does NOT say "namespace: from AF's deployment context" | BR-PLATFORM-057 |

## 12. Environmental Needs

| Requirement | Detail |
|-------------|--------|
| Go 1.24+ | Required for module compatibility |
| envtest (controller-runtime) | For IT tests — CRD installation from `config/crd/bases/` |
| Ginkgo v2 / Gomega | BDD test framework (mandatory per project rules) |

## 13. Responsibilities

| Role | Responsibility |
|------|---------------|
| Developer | Write tests, implement fix, run validation |
| Reviewer | Verify TDD compliance, ADR-057 alignment, Pyramid Invariant |

## 14. Schedule

| Phase | Estimate |
|-------|----------|
| Test Plan | 15 min |
| RED phase | 30 min |
| GREEN phase | 45 min |
| REFACTOR phase | 30 min |
| Checkpoints (3x) | 15 min each |

## 15. Approvals

| Role | Name | Date | Status |
|------|------|------|--------|
| Author | AI Agent | 2026-05-26 | Draft |
| Reviewer | — | — | Pending |

## Appendix A: Business Requirements Coverage Matrix

| BR ID | Description | Test Type | Test IDs | Status |
|-------|-------------|-----------|----------|--------|
| BR-PLATFORM-057 | CRD namespace consolidation (ADR-057) | UT, IT | UT-AF-1292-NS-001, IT-AF-1292-W01, IT-AF-1292-W02 | Pending |
| BR-SAFETY-001 | Dedup fingerprint scoped to workload identity | UT | UT-AF-1292-NS-002, UT-AF-1292-NS-006, UT-AF-1292-NS-007 | Pending |
| BR-AI-056 | Severity triage queries workload namespace | UT | UT-AF-1292-NS-003, UT-AF-1292-NS-005 | Pending |
| BR-SAFETY-002 | Invalid workload namespace rejected | UT | UT-AF-1292-NS-004 | Pending |

## Appendix B: 100-Go-Mistakes Findings (REFACTOR phase)

| Finding | Severity | Mistake # | Current | Fix |
|---------|----------|-----------|---------|-----|
| `%v` in error wrap (line 64) | Medium | #49 | `fmt.Errorf("%w: %v", ErrInvalidInput, err)` | `fmt.Errorf("%w: %w", ErrInvalidInput, err)` |
| Unwrapped singleflight errors (lines 106, 183) | Medium | #49 | `return nil, checkErr` | Add operation context |
| Redundant `string(Source)` (line 142) | Low | #40 | `string(triageResult.Source)` | Remove conversion |
| Swallowed list-events errors | Low | #53 | Silent fallthrough | Add debug-level log |

## Appendix C: Namespace Usage Classification

| Line | Code | Correct NS |
|------|------|------------|
| 63 | `validate.Namespace(...)` | controllerNS |
| 81 | `TriageInput{Namespace: ...}` | args.Namespace (workload) |
| 85 | `Labels{"namespace": ...}` | args.Namespace (workload) |
| 97 | `deriveSignalName(ctx, client, ...)` | args.Namespace (workload) |
| 98 | `rrFingerprint(...)` | args.Namespace (workload) |
| 102 | `HandleCheckExistingRR(...)` | controllerNS (list), args.Namespace (fingerprint) |
| 136 | `spec.targetResource.namespace` | args.Namespace (workload) |
| 159 | `metadata.namespace` | controllerNS |
| 165 | `client.Resource().Namespace().Create()` | controllerNS |
