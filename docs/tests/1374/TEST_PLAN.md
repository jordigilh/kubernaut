# Test Plan — #1374: SyncSignalFromRCA GVK Sync for Cross-Resource RCA

**IEEE 829 Compliant** | **Issue**: [#1374](https://github.com/jordigilh/kubernaut/issues/1374) | **Depends on**: #1051 (GVK component matching), #1064 (label override consistency)

## 1. Test Plan Identifier

TP-1374-SIGNAL-GVK-SYNC

## 2. Introduction

`SyncSignalAPIVersionFromRCA` only syncs `ResourceAPIVersion` from the RCA target to the
signal context, but does not sync `ResourceKind`. When the RCA identifies a different
resource kind than the original alert target (e.g., `AuthorizationPolicy` vs `Deployment`),
the resulting `ComponentGVK()` produces an invalid combination
(`security.istio.io/v1/Deployment`) that fails to match any workflow in the DS catalog.

This test plan covers replacing `SyncSignalAPIVersionFromRCA` with `SyncSignalFromRCA` that
syncs Kind, Name, Namespace, and APIVersion from the RCA target, with a stale GVK guard and
namespace normalization for cluster-scoped kinds.

**Business Requirements**:
- BR-WORKFLOW-004: DS catalog labels use GVK format (`apiVersion/Kind`); discovery filters must match
- BR-INTERACTIVE-010 SC-6: MCP `discover_workflows` must have parity with autonomous Phase 2/3
- BR-HAPI-261: Cross-resource root cause must be correctly resolved for workflow discovery
- BR-AUDIT-025: DS audit events must capture correct `signalContext.component`

**FedRAMP Control Mapping**:
| Control | Objective | Behavioral Assurance |
|---------|-----------|---------------------|
| SI-10   | Information input validation | Correct GVK prevents invalid DS catalog queries; stale GVK guard prevents nonsensical combinations (e.g. `security.istio.io/v1/Deployment`). Namespace normalization via RESTMapper prevents cluster-scoped kinds from carrying stale namespace values |
| AU-2    | Auditable events defined | `component_gvk` in structured logs reflects actual RCA target, not original alert target. Workflow discovery logs carry pre/post Kind for audit traceability |
| AU-12   | Audit generation | Every workflow discovery invocation logs the resolved `component_gvk`, enabling post-incident audit of which GVK was used for catalog queries |

## 3. Test Items

| Item | File | Description |
|------|------|-------------|
| `SyncSignalFromRCA` | `internal/kubernautagent/investigator/investigator_phases.go` | Renamed from `SyncSignalAPIVersionFromRCA`; now syncs Kind, Name, Namespace, APIVersion with stale GVK guard |
| `RunWorkflowDiscoveryFromRCA` (updated) | `internal/kubernautagent/investigator/investigator.go` | Updated call site + namespace normalization via `normalizeNamespace` |

## 4. Features to Be Tested

### 4.1 SyncSignalFromRCA — Signal/RCA Target Reconciliation (UT tier)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| UT-KA-1374-001 | BR-WORKFLOW-004 | SI-10 | Cross-resource RCA: Deployment signal + AuthorizationPolicy/security.istio.io/v1 RCA target | `signal.ResourceKind == "AuthorizationPolicy"`, `ComponentGVK() == "security.istio.io/v1/AuthorizationPolicy"` |
| UT-KA-1374-002 | BR-WORKFLOW-004 | — | Cross-resource RCA: Name and Namespace synced from RCA target | `signal.ResourceName == target.Name`, `signal.Namespace == target.Namespace` |
| UT-KA-1374-003 | BR-WORKFLOW-004 | SI-10 | Stale GVK guard: Kind changes but RCA has no apiVersion | `signal.ResourceAPIVersion == ""`, `ComponentGVK() == ""` |
| UT-KA-1374-004 | BR-WORKFLOW-004 | — | Same-resource RCA: apiVersion synced from RCA (parity with autonomous L505) | `signal.ResourceAPIVersion == target.APIVersion` |
| UT-KA-1374-005 | BR-WORKFLOW-004 | — | RCA target with empty Kind: signal unchanged | All signal fields unchanged |
| UT-KA-1374-006 | BR-WORKFLOW-004 | — | Cluster-scoped kind (Node): Namespace cleared when RCA target has empty namespace | `signal.Namespace == ""` |
| UT-KA-1374-007 | BR-WORKFLOW-004 | SI-10 | Namespace normalization in RunWorkflowDiscoveryFromRCA: cluster-scoped Kind clears stale namespace via RESTMapper | `signal.Namespace == ""` after normalization |

### 4.2 Existing Tests — Updated for Renamed Function

| ID | Change | Asserts |
|----|--------|---------|
| UT-KA-WD-001 | Call `SyncSignalFromRCA` instead of `SyncSignalAPIVersionFromRCA` | Unchanged: apiVersion copied when signal empty |
| UT-KA-WD-002 | Updated behavior: RCA apiVersion takes precedence (parity with autonomous L505) | `signal.ResourceAPIVersion == target.APIVersion` |
| UT-KA-WD-003 | Call renamed function | Unchanged: both empty stays empty |
| UT-KA-WD-004 | Call renamed function | Unchanged: Node apiVersion synced |

### 4.3 MCP discover_workflows Wiring (IT tier)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| IT-KA-DISC-009 | BR-INTERACTIVE-010, BR-WORKFLOW-004 | AU-2, SI-10 | Cross-resource RCA: Deployment signal, AuthorizationPolicy RCA target; `discover_workflows` produces correct GVK for DS query | `component_gvk` log contains `security.istio.io/v1/AuthorizationPolicy`; workflow found |
| IT-KA-DISC-010 | BR-INTERACTIVE-010 | — | Same-resource RCA: signal and RCA agree on Kind/APIVersion | GVK unchanged from pre-fix behavior |

### 4.4 Full Discovery Lifecycle (E2E tier)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| E2E-KA-DISC-005 | BR-INTERACTIVE-010, BR-WORKFLOW-004, BR-HAPI-261 | SI-10, AU-12 | Full discovery lifecycle with cross-resource RCA: alert targets Deployment, RCA identifies AuthorizationPolicy, DS has matching workflow | Workflow discovered and returned; no `submit_result_no_workflow` sentinel |

## 5. Features Not to Be Tested

- Autonomous `Investigate()` path — unchanged, uses inline logic at L492-509
- AF MCP bridge — not involved in KA-internal signal reconciliation
- `ResolvePostRCAEnrichment` stub — out of scope; tracked separately
- `DetectedLabelsJSON` propagation — out of scope; tracked separately

## 6. Approach

### 6.1 TDD Methodology

Each implementation phase follows strict TDD RED-GREEN-REFACTOR:

1. **RED**: Write failing Ginkgo/Gomega tests defining the business contract
2. **GREEN**: Minimal implementation to pass all tests
3. **REFACTOR**: Improve code quality; validate against [100 Go Mistakes](https://github.com/teivah/100-go-mistakes)

### 6.2 Test Pyramid (Pyramid Invariant)

| Tier | Count | Target Coverage | Strategy |
|------|-------|-----------------|----------|
| Unit (UT) | 11 scenarios (7 new + 4 updated) | >= 80% of `SyncSignalFromRCA` | Table-driven, value semantics |
| Integration (IT) | 2 scenarios | >= 80% of `RunWorkflowDiscoveryFromRCA` changes | MCP dispatch via `BootstrapMCP` stack |
| E2E | 1 scenario | Full journey proof | Kind + mock LLM + DS + real KA |

### 6.3 Checkpoints

#### CP-1: After TDD Red

| Dimension | Gate |
|-----------|------|
| All new tests fail with expected errors | Function `SyncSignalFromRCA` undefined or wrong behavior |
| Build succeeds | `go build ./...` green (production code unchanged) |

#### CP-2: After TDD Green

| Dimension | Gate | Evidence |
|-----------|------|----------|
| Build | `go build ./...` green | CI output |
| UT | UT-KA-1374-001..007 + UT-KA-WD-001..004 pass | Test output |
| IT | IT-KA-DISC-009..010 pass | Test output |
| E2E | E2E-KA-DISC-005 passes | Test output |
| Coverage | >=80% of changed code per tier | `go test -cover` |
| FedRAMP SI-10 | GVK input validated before DS query | UT-KA-1374-003 |
| FedRAMP AU-2/AU-12 | `component_gvk` log reflects RCA target | IT-KA-DISC-009 |
| Pyramid Invariant | UT proves logic, IT proves wiring, E2E proves journey | All tiers green |
| Wiring | `SyncSignalFromRCA` called from production code | grep evidence |
| Backward compat | Autonomous path unchanged | `Investigate()` untouched |
| No regressions | IT-KA-DISC-001..008 pass | CI output |

#### CP-3: After TDD Refactor

| Dimension | Gate | Evidence |
|-----------|------|----------|
| Build + Lint | `go build ./...` + `golangci-lint` clean | CI output |
| All tiers green | UT + IT + E2E pass, no flakes | Full CI pipeline green |
| 100 Go Mistakes | No violations in changed code | Audit checklist |
| No orphaned code | `SyncSignalAPIVersionFromRCA` fully removed | grep returns 0 |
| Live cluster | Trigger IstioHighDenyRate, verify workflow discovered | Manual test |
| Confidence >=90% | Grounded in test results + live evidence | Report |

## 7. Test Deliverables

| Deliverable | Location |
|-------------|----------|
| This test plan | `docs/tests/1374/TEST_PLAN.md` |
| UT: SyncSignalFromRCA | `internal/kubernautagent/investigator/signal_label_override_test.go` |
| IT: Cross-resource discovery | `test/integration/kubernautagent/mcp/discovery_flow_test.go` |
| E2E: Cross-resource lifecycle | `test/e2e/kubernautagent/interactive_discovery_e2e_test.go` |
| E2E infra: mock LLM scenario | `test/services/mock-llm/scenarios/scenario_istio_authz.go` |
| E2E infra: workflow fixture | `test/fixtures/workflows/istio-authz-fix/workflow-schema.yaml` |

## 8. Test Environment

| Component | Tool |
|-----------|------|
| Go version | 1.24+ |
| Test framework | Ginkgo v2 / Gomega |
| K8s testing | envtest (kubebuilder) for IT tier; Kind for E2E |
| CI | GitHub Actions (`ci-pipeline.yml`) |

## 9. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|-----------|------------------------|----------------------|------------|
| `SyncSignalFromRCA` | `RunWorkflowDiscoveryFromRCA()` | `investigator.go:338` | IT-KA-DISC-009 |
| `normalizeNamespace` in discovery | `RunWorkflowDiscoveryFromRCA()` | `investigator.go:339` (new) | IT-KA-DISC-009 |

## 10. Risks and Mitigations

| Risk | Impact | Likelihood | Mitigation | Test Coverage |
|------|--------|------------|------------|---------------|
| UT-KA-WD-002 behavioral change (RCA apiVersion now takes precedence) | Low | Low | Autonomous path already overrides (L505); signal apiVersion usually empty at discovery time | UT-KA-WD-002 updated |
| Namespace cleared for namespaced resource when RCA has empty namespace | Medium | Low | `normalizeNamespace` only clears cluster-scoped kinds; namespaced resources preserve namespace | UT-KA-1374-006, UT-KA-1374-007 |
| E2E infra: new mock LLM scenario + DS fixture | Low | Medium | Follows existing DISC-001 pattern; component-specific (not wildcard) | E2E-KA-DISC-005 |
| LLM hallucinates wrong apiVersion, override regresses | Low | Low | RCA goes through `apiVersionValidationGate` upstream; stale GVK guard clears invalid combos | UT-KA-1374-003 |

## 11. Schedule

| Phase | Tests | Checkpoint |
|-------|-------|------------|
| TDD Red | UT-KA-1374-001..007, UT-KA-WD-001..004, IT-KA-DISC-009..010, E2E-KA-DISC-005 | **CP-1** |
| TDD Green | Implement `SyncSignalFromRCA` + update `RunWorkflowDiscoveryFromRCA` | **CP-2** |
| TDD Refactor | 100 Go Mistakes audit, lint, docs | **CP-3** |

## 12. Approvals

| Role | Name | Date |
|------|------|------|
| Author | AI Agent | 2026-06-06 |
| Reviewer | — | — |
