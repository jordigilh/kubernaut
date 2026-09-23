# Test Plan: AIAnalysis Confidence Histogram Integration

> Hybrid IEEE 829-2008 + Kubernaut test plan.

**Test Plan Identifier**: TP-2443-v1.1
**Feature**: Exercise the confidence-distribution metric on a successful, catalog-grounded AIAnalysis flow
**Version**: 1.1
**Created**: 2026-09-23
**Author**: OpenCode agent
**Status**: Complete
**Branch**: `fix/2442-workflow-discovery-membership`

---

## 1. Introduction

### 1.1 Purpose

Repair the AIAnalysis integration test that currently expects a workflow selection for an ImagePullBackOff signal whose target Pod is absent and whose default Mock LLM workflow is not eligible under the real catalog filters. The revised test proves the confidence histogram is recorded as a side effect of a successful AIAnalysis completion with a selected workflow.

### 1.2 Objectives

1. The test workflow is returned by `list_workflows` for the test's action type and signal-context filters.
2. The target Pod exists during investigation and the AIAnalysis reaches `Completed` with a non-nil `SelectedWorkflow`.
3. The `ImagePullBackOff` confidence histogram sample count increases during that successful business flow.
4. The workflow-membership validation remains fail-closed; no production catalog, validator, or metrics behavior is weakened.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Mock LLM scenario routing | 100% | Ginkgo UT verifies signal match, expected workflow, and seeded UUID override |
| AIAnalysis integration outcome | 100% | `Completed` and `SelectedWorkflow != nil` in IT-AA-004-001 |
| Histogram emission | Sample count increases by at least 1 | Gather the `ImagePullBackOff` histogram before and after the test flow |
| Regression | 0 failures | Full `make test-integration-aianalysis` passes |

---

## 2. References

### 2.1 Authority

- **BR-AI-OBSERVABILITY-004**: Confidence-score distribution metric; documented in `docs/services/crd-controllers/02-aianalysis/metrics-slos.md`.
- **DD-KA-017**: Three-step workflow discovery; `list_workflows` results establish discovery membership.
- **DD-TEST-010**: Per-process AIAnalysis integration test isolation and reconciler access.
- **BR-AI-022**: Confidence thresholds for automated decisions (separate behavior; not the histogram metric requirement).

### 2.2 Cross-References

- `docs/testing/TEST_PLAN_TEMPLATE.md`
- `docs/services/crd-controllers/02-aianalysis/metrics-slos.md`
- `internal/controller/aianalysis/metrics_recorder.go`
- `internal/kubernautagent/tools/custom/tools.go`
- `internal/kubernautagent/workflowcatalog/cache_filter.go`

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Workflow fixture YAML does not match all mandatory catalog filters | Mock workflow is absent from `list_workflows`; analysis cannot complete | Medium | UT-MOCK-004-001, IT-AA-004-001 | Set `actionType: RestartPod`, severity `critical`, component `v1/Pod`, environment `staging`, priority `P2`, and wildcard cluster in the actual seeded YAML. |
| R2 | Mock LLM scenario uses a deterministic UUID instead of the UUID seeded from the fixture | Membership validation correctly rejects the candidate | Medium | UT-MOCK-004-001, IT-AA-004-001 | Reuse the existing workflow-name/environment UUID override path and assert the resolved ID. |
| R3 | The Pod is missing or crosses the test's process namespace boundary | Enrichment becomes sparse; the analysis terminates without a workflow | Medium | IT-AA-004-001 | Create the target Pod in `staging` with the existing fixture helper and defer cleanup. |
| R4 | A prior flake attempt leaves a histogram observation in the process registry | A later retry could pass without recording a new observation | Medium | IT-AA-004-001 | Capture the per-attempt baseline and assert the sample-count delta is positive. |

### 3.1 Risk-to-Test Traceability

R1/R2 are covered by the Mock LLM routing/UUID unit test and the real catalog integration path. R3/R4 are covered by IT-AA-004-001.

---

## 4. Scope

### 4.1 Features to be Tested

- `test/services/mock-llm/scenarios/scenario_aianalysis_fixtures.go`: signal-specific scenario selection and workflow UUID override.
- `test/integration/aianalysis/test_workflows.go` and `test/fixtures/workflows/imagepullbackoff-metrics/workflow-schema.yaml`: test workflow seed data.
- `test/integration/aianalysis/metrics_integration_test.go`: real reconciliation, successful selected-workflow outcome, and histogram sample-count delta.
- `internal/controller/aianalysis/metrics_recorder.go`: existing recording contract, exercised without changing production behavior.

### 4.2 Features Not to be Tested

- Confidence-threshold decision behavior under BR-AI-022; it is a separate business outcome.
- The HTTP `/metrics` scrape endpoint; that is covered by the AIAnalysis E2E metrics suite.
- Any change to production workflow membership, catalog filtering, or metric-recording code.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Add a dedicated test workflow schema instead of broadening `generic-restart` labels | The generic restart fixture intentionally excludes `critical`; changing it would broaden product catalog eligibility to solve a test-data mismatch. |
| Reuse the existing AIAnalysis fixture scenario registry and seeded-UUID override path | This exercises the same scenario registration and discovery membership behavior used by the integration suite. |
| Measure histogram sample-count delta | It proves this completed analysis emitted an observation and avoids false positives from existing metric state or retries. |

---

## 5. Approach

### 5.1 Coverage Policy

The change adds no production business logic. The Mock LLM scenario receives Ginkgo unit coverage; the controller, DataStorage catalog, Pod API, and metric recorder are exercised through the real integration test.

### 5.2 Two-Tier Minimum

- **Unit**: Verify the ImagePullBackOff scenario is selected and the scenario's workflow ID resolves to the test-seeded workflow UUID.
- **Integration**: Verify the business flow completes with the discovered workflow and records a histogram observation.

### 5.3 Business Outcome Quality Bar

The test passes only when the AIAnalysis is `Completed`, has a selected workflow discovered through `list_workflows`, and increases the histogram sample count for the signal. Accessing the metric vector alone is not sufficient.

### 5.4 Pass/Fail Criteria

**PASS**:

1. UT-MOCK-004-001 and IT-AA-004-001 pass.
2. AIAnalysis reaches `Completed` with `SelectedWorkflow != nil`.
3. Histogram sample count after the flow is greater than its per-attempt baseline.
4. The entire `make test-integration-aianalysis` lane passes with no regressions.

**FAIL**:

1. The expected workflow is absent from the filtered `list_workflows` response.
2. The analysis is `Failed`, remains non-terminal, or completes without `SelectedWorkflow`.
3. The histogram sample count does not increase for the signal.
4. Any previously passing AIAnalysis integration spec regresses.

### 5.5 Suspension & Resumption Criteria

Suspend if the envtest API server, Podman service dependencies, or workflow catalog fixture seeding fails before the business flow runs. Resume after the infrastructure cause is resolved; do not weaken workflow-membership validation to bypass a fixture mismatch.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File | Functions/Methods | Notes |
|------|-------------------|-------|
| `test/services/mock-llm/scenarios/scenario_aianalysis_fixtures.go` | `aiAnalysisFixtureScenarios`, `matchAIAnalysisFixture`, `aiAnalysisFixtureConfig` | Existing test-harness logic extended for ImagePullBackOff. |

### 6.2 Integration-Testable Code

| File | Functions/Methods | Notes |
|------|-------------------|-------|
| `test/integration/aianalysis/metrics_integration_test.go` | Confidence score metric spec | Exercises the real reconciliation and metric-recording path. |
| `test/integration/aianalysis/test_workflows.go` | `GetAIAnalysisTestWorkflows`, `SeedTestWorkflowsViaDirectCRDCreation` | Seeds workflow UUID and environment mapping from the fixture schema. |
| `test/fixtures/workflows/imagepullbackoff-metrics/workflow-schema.yaml` | RemediationWorkflow fixture | Supplies the catalog labels used by discovery filters. |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | PR #2443 merge commit `7065d8d` baseline | Test was reproduced locally from this CI merge commit. |
| Go | `go.mod` toolchain requirement | Local runs use `GOTOOLCHAIN=auto` if the installed toolchain is older. |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-AI-OBSERVABILITY-004 | Record confidence score distribution for completed workflow selections | P1 | Unit | UT-MOCK-004-001 | Pass |
| BR-AI-OBSERVABILITY-004 | Record an observation on the real AIAnalysis success path | P1 | Integration | IT-AA-004-001 | Pass |

---

## 8. Test Scenarios

### Test ID Naming Convention

Use the project form `{TIER}-{SERVICE}-{BR_NUMBER}-{SEQUENCE}`.

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| UT-MOCK-004-001 | An ImagePullBackOff prompt resolves to the dedicated scenario and the actual seeded workflow UUID | Pass |

### Tier 2: Integration Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| IT-AA-004-001 | A seeded, eligible workflow is discovered, selected, and produces a confidence histogram observation after successful completion | Pass |

### Tier 3: E2E Tests

No new E2E scenario is required. The existing E2E metrics suite covers operator scrape access; this plan proves recording during the real reconciliation business flow.

### Tier Skip Rationale

E2E is not expanded because this change corrects the existing integration test fixture and verifies the recording call site; scrape endpoint behavior remains in the existing E2E suite.

---

## 9. Test Cases

### UT-MOCK-004-001: ImagePullBackOff scenario selects the seeded workflow

**BR**: BR-AI-OBSERVABILITY-004
**Priority**: P1
**Type**: Unit (Ginkgo/Gomega)
**File**: `test/services/mock-llm/`

**Preconditions**:
- Registry includes the dedicated ImagePullBackOff fixture scenario.
- Scenario override map contains the workflow name/environment key and a non-empty seeded UUID.

**Test Steps**:
1. **Given** an ImagePullBackOff prompt for a critical Pod in staging at priority P2.
2. **When** the Mock LLM registry resolves the scenario with the workflow UUID override.
3. **Then** the selected scenario names the dedicated workflow and returns the overridden UUID, not the generic fallback workflow.

**Acceptance Criteria**:
- Scenario detection is specific to ImagePullBackOff and does not capture unrelated signals.
- Workflow ID equals the seeded UUID for `imagepullbackoff-metrics-v1:staging`.

### IT-AA-004-001: Completed AIAnalysis records confidence histogram

**BR**: BR-AI-OBSERVABILITY-004
**Priority**: P1
**Type**: Integration (Ginkgo/Gomega)
**File**: `test/integration/aianalysis/metrics_integration_test.go`

**Preconditions**:
- AIAnalysis controller, DataStorage, PostgreSQL, Redis, and Mock LLM integration dependencies are live.
- The test workflow schema is seeded; the target Pod exists in `staging`.

**Test Steps**:
1. **Given** an ImagePullBackOff AIAnalysis request with critical severity, staging environment, P2 priority, and an existing Pod target.
2. **When** the Mock LLM discovers/selects the matching workflow and the controller completes the analysis.
3. **Then** the status is `Completed`, the selected workflow is non-nil, and the `ImagePullBackOff` histogram sample count exceeds the per-attempt baseline.

**Acceptance Criteria**:
- The selected workflow ID is present in the real `list_workflows` result and passes the `get_workflow` context gate.
- The metric sample-count delta is positive; no test helper returns a constant as evidence.

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega.
- **Mocks**: None required beyond the Mock LLM scenario registry test fixture.
- **Location**: `test/services/mock-llm/`.

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega.
- **Mocks**: No mocks for the controller, Kubernetes API, catalog, or metric recorder. Mock LLM remains the external AI dependency.
- **Infrastructure**: envtest API server, Podman, PostgreSQL, Redis, DataStorage, Kubernaut Agent, and Mock LLM.
- **Location**: `test/integration/aianalysis/`.

### 10.3 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | `go.mod` requirement (1.26.6 at preflight) | Build and test |
| Ginkgo CLI | Repository-pinned version | Focused/full BDD test execution |
| Podman | 5.x | Integration dependency containers |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| envtest assets | Test infrastructure | Available on CI/local setup-envtest | Integration specs cannot start API server | Install through repository Make target. |
| Podman images | Test infrastructure | Last CI artifacts available | External services cannot start | Load DataStorage/Agent artifacts; build updated Mock LLM from source locally or use same-commit CI image. |

### 11.2 Execution Order

1. **Plan/RED**: Add this plan, scenario-routing UT, and real metric assertion; verify RED.
2. **GREEN**: Add fixture YAML, `TestWorkflow` entry, Mock LLM scenario, and target Pod setup.
3. **REFACTOR**: Extract histogram sample-count helper and improve failure diagnostics.
4. **WIRING VERIFICATION**: Confirm Mock LLM scenario → seeded UUID → `list_workflows` → `get_workflow` → selected workflow → completed metric path.
5. **Validation**: Focused UT/IT, full AIAnalysis integration lane, repository build/lint/unit checks.

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/testing/2443/TEST_PLAN.md` | IEEE 829 strategy and traceability |
| Mock LLM BDD spec | `test/services/mock-llm/` | Scenario and seeded UUID verification |
| Test workflow schema | `test/fixtures/workflows/imagepullbackoff-metrics/workflow-schema.yaml` | Eligible test-only catalog candidate |
| Integration spec update | `test/integration/aianalysis/metrics_integration_test.go` | Real completion and histogram sample-count proof |
| Coverage report | CI artifact | Test lane results |

---

## 13. Execution

```bash
# Mock LLM unit scenario
go test ./test/services/mock-llm -ginkgo.focus='UT-MOCK-004-001'

# Focused integration scenario
GINKGO_FOCUS='IT-AA-004-001' TEST_PROCS=1 make test-integration-aianalysis

# Full integration lane
KIND_EXPERIMENTAL_PROVIDER=podman GOTOOLCHAIN=auto TEST_PROCS=4 \
  KUBERNAUT_CI_ARTIFACT_TAG=pr-2443-obs004-validation IMAGE_REGISTRY= IMAGE_TAG= \
  make test-integration-aianalysis

# Repository checks
go build ./...
golangci-lint run --timeout=5m
make test
```

For local validation, do not reuse a stale Mock LLM artifact after changing its scenario. Build that image from the updated source; CI will use the image built from the same commit.

### 13.1 Execution Results

- `go test ./test/services/mock-llm ./internal/kubernautagent/workflowcatalog`: **PASS**.
- Focused IT-AA-004-001 with the fresh host artifact tag: **PASS**.
- Full `make test-integration-aianalysis`: **PASS**, including all 62 main-suite specs and the capacity-retry, cascade-cancel, and schema-rejection suites.
- `go build ./...`: **PASS**.
- `go test ./... -run=^$ -timeout=30s`: **PASS** (compile-only post-refactor check).
- Targeted `golangci-lint` for `test/integration/aianalysis` and `test/services/mock-llm`: **PASS**, zero issues.
- Repository-wide `make test` was attempted at default and serial Ginkgo process counts. The retries encountered timing failures in unrelated `UT-AA-KA-065-026` and `IT-2364-003` under `pkg/apifrontend/agent`; the isolated KA spec passes, and the changed packages plus full AIAnalysis integration lane pass.
- Repository-wide lint gates still report findings outside this plan's changed paths; targeted lint for the changed Go packages is clean.

---

## 14. Wiring Verification (TDD Phase 4)

| Code Path | Entry Point | Exit Point | Wiring IT | Status |
|-----------|-------------|------------|-----------|--------|
| ImagePullBackOff fixture scenario | Mock LLM `DefaultRegistry` | Workflow ID response uses seed override | IT-AA-004-001 | Pass |
| Test workflow seed | AIAnalysis `SynchronizedBeforeSuite` direct-CRD seeder | Workflow is discoverable by DataStorage-backed catalog | IT-AA-004-001 | Pass |
| Target Pod fixture | Ginkgo spec creates target Pod before AIAnalysis CR | Agent enrichment finds resource; analysis can complete | IT-AA-004-001 | Pass |
| Metric recording | Real AIAnalysis reconcile reaches Completed with SelectedWorkflow | Histogram sample count increases | IT-AA-004-001 | Pass |

No production caller is added; these rows prove test-harness and existing production wiring.

---

## 15. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `metrics_integration_test.go`, confidence score case | Waits only for selected workflow; histogram helper returns constant 1; cites BR-AI-022 | Wait for Completed plus selection; compare actual sample-count delta; cite BR-AI-OBSERVABILITY-004 | The metric is recorded only on the completed selected-workflow path; BR-AI-022 is threshold logic. |

---

## 16. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-09-23 | Initial plan from PR #2443 host reproduction and workflow-catalog preflight. |
| 1.1 | 2026-09-23 | Completed RED/GREEN/REFACTOR test work and recorded unit/IT execution results. |
