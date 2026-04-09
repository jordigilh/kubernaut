# Test Plan: E2E FullPipeline — Helm chart deployment

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-239-v1
**Feature**: Deploy the product stack for FullPipeline E2E via `helm install`/`helm upgrade` (`charts/kubernaut/`) instead of programmatic inline Go manifests, while retaining E2E-only infrastructure for Mock LLM, Prometheus, AlertManager, event-exporter, and mock-slack.
**Version**: 1.0
**Created**: 2026-04-09
**Author**: AI Assistant
**Status**: Draft
**Branch**: `development/v1.3_part2` (or branch carrying Issue #239)

---

## 1. Introduction

> **IEEE 829 §3** — Purpose, objectives, and measurable success criteria for the test effort.

### 1.1 Purpose

This test plan defines how Issue #239 will be validated: replacing duplicated inline Kubernetes manifests in `SetupFullPipelineInfrastructure` with the authoritative Helm chart so CI exercises the same deployment path as operators, while test-only components stay on the existing E2E infrastructure path. It also covers optional E2E coverage instrumentation, NodePort accessibility for Gateway and DataStorage, and guarding against CRD and RBAC drift between chart templates and `config/crd/bases`.

### 1.2 Objectives

1. **Chart fidelity**: `helm template` (or equivalent) with `values-e2e.yaml` renders without error and produces the FullPipeline product workload set (PostgreSQL, Valkey, DataStorage, AuthWebhook, Gateway, all controllers, Kubernaut Agent, RBAC, hooks) consistent with today’s E2E expectations.
2. **Regression safety**: All five existing FullPipeline Ginkgo E2E specs continue to pass **without source changes** to those test files (`01_full_remediation_lifecycle`, `02_approval_lifecycle`, `02_async_hash_deferral`, `03_ka_target_resource` — plus suite/bootstrap as applicable).
3. **Operational parity**: Install ordering remains correct (PostgreSQL → migrations → DataStorage → seeding → controllers → test infra), with Helm hooks and/or post-install scripting aligned to that sequence.
4. **Traceability**: Every scenario maps to **BR-E2E-001** and the **E2E-FP-*** scenario identifiers exercised by the FullPipeline suite.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Helm render pass rate | 100% | `helm template kubernaut ./charts/kubernaut -f charts/kubernaut/values-e2e.yaml` (or project-standard wrapper) exits 0 |
| FullPipeline E2E pass rate | 100% | Ginkgo E2E under `test/e2e/fullpipeline/` with FullPipeline infrastructure enabled |
| Backward compatibility | 0 regressions | Five existing FullPipeline test files unchanged and green |
| NodePort accessibility (P1) | Gateway :30080, DataStorage :30081 reachable from test host when overrides applied | HTTP/TCP checks per TP-239-004 |
| E2E coverage artifacts (P1) | When `E2E_COVERAGE=true`, coverage output present for all six controllers | Files under `GOCOVERDIR` / documented artifact layout |
| Drift guard (P2) | CI job green | Template diff or policy check for critical RBAC/env consistency (TP-239-006) |

---

## 2. References

> **IEEE 829 §2** — Documents that govern or inform this test plan.

### 2.1 Authority (governing documents)

- **BR-E2E-001**: End-to-end validation of the Kubernaut remediation pipeline (umbrella for FullPipeline E2E).
- **E2E-FP-***: FullPipeline E2E scenario identifiers referenced by existing `test/e2e/fullpipeline/` specifications.
- Issue **#239**: E2E FullPipeline — deploy via Helm chart instead of programmatic manifests.

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- Helm chart: `charts/kubernaut/`
- E2E infrastructure: `test/infrastructure/fullpipeline_e2e.go`
- CRD bases: `config/crd/bases/`
- Chart CRDs: `charts/kubernaut/files/crds/`

---

## 3. Risks & Mitigations

> **IEEE 829 §5** — Software risk issues that drive test design.

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Ordering dependencies: Postgres before DataStorage before seeding before controllers | Pipeline hangs or flaky E2E; data not ready when tests run | High | TP-239-003 | Use Helm lifecycle hooks and/or documented post-install script; mirror current E2E ordering; add explicit readiness in suite if needed |
| R2 | Secret/bootstrap differs between chart values and former inline manifests | Auth failures, pods CrashLoop | Medium | TP-239-002, TP-239-003 | `values-e2e.yaml` documents parity with prior secrets; TP-239-002 compares critical env/secret references |
| R3 | EffectivenessMonitor monitoring URLs: chart defaults may disable Prom/AM; E2E requires in-cluster URLs | EM metrics/alerts tests fail or skip incorrectly | Medium | TP-239-003 | Override in `values-e2e.yaml` to enable URLs required by FullPipeline; assert in TP-239-002 where static |
| R4 | Coverage: `GOCOVERDIR` + hostPath not present in chart today | No coverage files when `E2E_COVERAGE=true` | High (pre-change) | TP-239-005 | Add optional `e2e.coverage.enabled` (or equivalent) to chart templates for E2E-only volumes/env |
| R5 | Dual CRD source (`charts/kubernaut/files/crds` vs `config/crd/bases`) | Schema drift, apply failures, false greens | Medium | TP-239-006, TP-239-003 | REFACTOR: single source of truth; P2 drift guard in CI |

### 3.1 Risk-to-Test Traceability

- **R1** → **TP-239-003** (full suite), **TP-239-001** (render sanity).
- **R2** → **TP-239-002**, **TP-239-003**.
- **R3** → **TP-239-002** (values/manifest expectations), **TP-239-003** (runtime).
- **R4** → **TP-239-005**.
- **R5** → **TP-239-006** (primary), **TP-239-001** (render includes expected CRD install hooks/resources per chosen alignment).

**Coverage gap flag**: If TP-239-006 is deferred, R5 mitigation is incomplete until CRD consolidation lands.

---

## 4. Scope

> **IEEE 829 §6/§7** — Features to be tested and features not to be tested.

### 4.1 Features to be Tested

- **FullPipeline E2E infrastructure** (`test/infrastructure/fullpipeline_e2e.go`): `SetupFullPipelineInfrastructure` (and helpers) refactored to install/upgrade the **product stack** via Helm using `charts/kubernaut/` and `values-e2e.yaml`.
- **Helm chart** (`charts/kubernaut/`): Renders and deploys PostgreSQL, Valkey, DataStorage, AuthWebhook, Gateway, all controllers, Kubernaut Agent, RBAC, and hooks as modeled by the chart.
- **E2E values** (`charts/kubernaut/values-e2e.yaml`, new): NodePort overrides (Gateway **30080**, DataStorage **30081**), EM/monitoring overrides as required, and optional `e2e.coverage.enabled` for `GOCOVERDIR` + hostPath volumes.
- **Test-only infrastructure** (retain programmatic or `kubectl` apply path): Mock LLM, Prometheus, AlertManager, event-exporter, mock-slack — **not** moved into the product chart for this issue.
- **CI / automation**: Optional drift guard validating rendered manifests for critical RBAC/env consistency (TP-239-006).

### 4.2 Features Not to be Tested

- **cert-manager**, **memory-eater**, and other components explicitly out of scope for the product chart in this issue (unless already part of FullPipeline infra today — document in implementation if touched).
- **Production** `values.yaml` / airgap profiles: not the primary subject; only **E2E** values file is in scope for TP-239-001/002 unless a shared regression is explicitly added.
- **Changing assertions** inside the five existing FullPipeline E2E test files: out of scope — they must pass unchanged.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Incremental strategy: Helm for product stack, E2E infra for test-only | Minimizes chart bloat; keeps Mock LLM and observability mocks out of the shipping chart |
| Optional `e2e.coverage.enabled` in chart | Coverage is CI/E2E-only; must not affect default operator installs |
| NodePort overrides in `values-e2e.yaml` | Stable localhost ports **30080** / **30081** for gateway and datastorage per FullPipeline client expectations |
| Single CRD source (REFACTOR target) | Eliminates R5 drift between `files/crds` and `config/crd/bases` |

---

## 5. Approach

> **IEEE 829 §8/§9/§10** — Test strategy, pass/fail criteria, and suspension/resumption.

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: Where pure logic is extracted (e.g., helpers that diff “expected component set” vs rendered manifests), target **>=80%** of that **unit-testable** subset. Much of this issue is orchestration; **primary** quality signal is regression E2E.
- **Integration**: Optional tests that run `helm template` in CI or invoke a thin Go wrapper — classify as integration if subprocess/filesystem I/O is used; target **>=80%** of any new **integration-testable** helpers introduced.
- **E2E**: FullPipeline suite validates the **deployed** stack. Coverage is measured primarily by **TP-239-003** (existing scenarios). Optional **E2E_COVERAGE** path (TP-239-005) validates instrumentation, not necessarily line coverage thresholds for controllers in this plan.

### 5.2 Two-Tier Minimum

- **BR-E2E-001** / **E2E-FP-*** outcomes are covered by **E2E** (Tier 3) plus **at least one** faster tier: **helm render / manifest parity** tests (TP-239-001, TP-239-002) implemented as unit or CI integration checks.

### 5.3 Business Outcome Quality Bar

Tests prove the **operator-visible** outcome: the same cluster capabilities FullPipeline relied on (workloads, services, ports, secrets, monitoring hooks) are present when installed via Helm, and remediation lifecycle behavior remains correct end-to-end.

### 5.4 Pass/Fail Criteria

> **IEEE 829 §9** — When is this test plan considered passed or failed?

**PASS** — all of the following must be true:

1. All **P0** tests pass (TP-239-001, TP-239-002, TP-239-003): 0 failures.
2. All **P1** tests pass (**TP-239-004**, **TP-239-005**) or have documented exceptions approved by reviewer.
3. No regressions: five existing FullPipeline E2E files pass **unchanged**.
4. Helm-based install completes with ordering that satisfies R1 (verified by TP-239-003 and implementation review).

**FAIL** — any of the following:

1. Any **P0** test fails.
2. Any existing FullPipeline E2E test fails.
3. **P1** fails without approved exception (team policy).
4. **P2** (**TP-239-006**) fails **after** it has been promoted from optional to mandatory in the issue/PR acceptance criteria (until then, P2 failure is a quality signal, not automatic FAIL per this plan v1).

### 5.5 Suspension & Resumption Criteria

> **IEEE 829 §10** — When should testing stop? When can it resume?

**Suspend testing when**:

- Kind cluster or CI runner cannot provision Kubernetes or run Helm.
- `values-e2e.yaml` or chart changes break `helm template` (blocking TP-239-001).
- Cascading failures: more than one P0 failure shares the same root cause — stop and fix infrastructure first.

**Resume testing when**:

- Cluster/Helm availability restored; `helm template` succeeds.
- Blocking defect for ordering or secrets fixed and merged.
- CRD source alignment or drift guard merged if R5 was the blocker.

---

## 6. Test Items

> **IEEE 829 §4** — Software items to be tested, with version identification.

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|----------------|
| New or existing helpers under `test/infrastructure/` or `test/e2e/fullpipeline/` (if added) | Manifest/component set comparison, value validation | TBD (post-implementation) |

*Note: If no pure helpers are extracted, Section 6.1 may remain TBD with coverage rationale in Section 8 (Tier Skip).*

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|----------------|
| `test/infrastructure/fullpipeline_e2e.go` | `SetupFullPipelineInfrastructure`, Helm install/upgrade wiring, ordering | TBD |
| `charts/kubernaut/templates/**` | Conditional E2E coverage volumes/env | TBD |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | Branch carrying Issue #239 | Helm 3.x compatible chart |
| Helm chart | `charts/kubernaut/` Chart.yaml version | Bumped per release process |
| CRDs | Single source after REFACTOR | Until then: document which path E2E applies |

---

## 7. BR Coverage Matrix

> Kubernaut-specific. Maps every business requirement to test scenarios across tiers.

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-E2E-001 | E2E validation of remediation / FullPipeline behavior | P0 | E2E / Infra | TP-239-003 | Pending |
| BR-E2E-001 | Chart renders for E2E values | P0 | Unit/CI | TP-239-001 | Pending |
| BR-E2E-001 | Product component parity vs prior E2E stack | P0 | Unit/CI | TP-239-002 | Pending |
| BR-E2E-001 | Stable NodePorts for clients | P1 | E2E | TP-239-004 | Pending |
| BR-E2E-001 | Optional controller coverage in E2E | P1 | E2E | TP-239-005 | Pending |
| BR-E2E-001 | CI drift guard for chart vs policy | P2 | CI | TP-239-006 | Pending |

**E2E-FP-*** scenarios: Satisfied indirectly by **TP-239-003** (no change to existing scenario IDs in test sources).

### Status Legend

- **Pending**: Specification complete, implementation not started
- **RED**: Failing test written (TDD RED phase)
- **GREEN**: Minimal implementation passes (TDD GREEN phase)
- **REFACTORED**: Code cleaned up (TDD REFACTOR phase)
- **Pass**: Implemented and passing

---

## 8. Test Scenarios

> **IEEE 829 Test Design Specification** — How test cases are organized and grouped.

### Test ID Naming Convention

This plan uses **TP-239-NNN** for plan-level scenario IDs (aligned with Issue #239). Where tests are implemented in Go with Ginkgo, developers may additionally use **E2E-FP-*** / existing suite labels; **no changes** to existing FullPipeline test file IDs are required by this issue.

### Tier 1: Unit Tests (fast / deterministic)

**Testable code scope**: Pure comparison logic for expected Deployments/Services/labels; optional wrapper around `helm template` output parsing (if kept in-process without cluster).

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| TP-239-001 | Operator can render the chart for E2E without template errors | Pending |
| TP-239-002 | Rendered product resources match the current FullPipeline **product** component set (Deployments/Services expectations) | Pending |

### Tier 2: Integration Tests

**Testable code scope**: Subprocess or CI job invoking `helm template` / `helm install` against a test cluster fixture (if split from E2E).

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| *(optional)* | Same as TP-239-001/002 when implemented as shell-only CI without Go | Pending |

### Tier 3: E2E Tests

**Testable code scope**: Full Kind (or CI) cluster with FullPipeline infrastructure; five unchanged FullPipeline specs.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| TP-239-003 | Full remediation lifecycle, approval, async hash, and KA target flows remain green | Pending |
| TP-239-004 | Gateway reachable on **localhost:30080**, DataStorage on **localhost:30081** with chart overrides | Pending |
| TP-239-005 | With `E2E_COVERAGE=true`, coverage artifacts produced for **all six** controllers | Pending |

### Tier Skip Rationale (if any tier is omitted)

- **Dedicated Tier 2**: Optional if TP-239-001/002 run only as Ginkgo-in-cluster or Makefile targets; rationale: single CI job may combine Tier 1 (render) and Tier 3 (E2E) for speed, documented in Section 13.

---

## 9. Test Cases

> **IEEE 829 Test Case Specification** — Detailed specification for each test case.

### TP-239-001: Helm template renders with values-e2e.yaml

**BR**: BR-E2E-001  
**Priority**: P0  
**Type**: Unit / CI (deterministic)  
**File**: TBD — e.g. `test/unit/infrastructure/helm_fullpipeline_test.go` or CI script invoked by `Makefile`

**Preconditions**:

- `helm` CLI available; chart path `charts/kubernaut/` present.
- `values-e2e.yaml` exists (GREEN phase deliverable).

**Test Steps**:

1. **Given**: Clean worktree with chart and `values-e2e.yaml`.
2. **When**: `helm template` (or project wrapper) runs with E2E values.
3. **Then**: Exit code 0; no Helm render errors.

**Expected Results**:

1. Command completes successfully.
2. Output contains expected top-level resources for the product stack (smoke-level assertion optional in same test or TP-239-002).

**Acceptance Criteria**:

- **Behavior**: Chart is valid for E2E consumption.
- **Correctness**: No template/values coercion errors.

**Dependencies**: None (first P0 gate). **TDD RED**: Add test before `values-e2e.yaml` exists → fails; **GREEN**: add values file + minimal chart hooks as needed.

---

### TP-239-002: Chart product set matches current E2E components

**BR**: BR-E2E-001  
**Priority**: P0  
**Type**: Unit / CI  
**File**: TBD — same package as TP-239-001 or shared helper

**Preconditions**:

- TP-239-001 passes.
- Canonical list of **product** Deployments/Services (and key differences vs test-only) documented in code or fixture.

**Test Steps**:

1. **Given**: Rendered manifests from `helm template` with `values-e2e.yaml`.
2. **When**: Parser extracts workload identity (kind, name, key labels).
3. **Then**: Set equals expected product component set (allowing chart naming conventions if documented).

**Expected Results**:

1. No missing product Deployment/Service.
2. No unexpected removal vs baseline snapshot (unless issue explicitly documents rename).

**Acceptance Criteria**:

- **Behavior**: Helm delivers the same **product** topology FullPipeline relied on.
- **Accuracy**: Explicit exclusion of Mock LLM, Prometheus, AlertManager, event-exporter, mock-slack from the **chart** expectation set.

**Dependencies**: TP-239-001. Mitigates **R2**, **R3** (via values assertions where static).

---

### TP-239-003: FullPipeline Ginkgo E2E regression

**BR**: BR-E2E-001 / **E2E-FP-***  
**Priority**: P0  
**Type**: E2E  
**File**: `test/e2e/fullpipeline/01_full_remediation_lifecycle_test.go`, `02_approval_lifecycle_test.go`, `02_async_hash_deferral_test.go`, `03_ka_target_resource_test.go` (**unchanged**), plus suite wiring

**Preconditions**:

- Cluster with FullPipeline infra applied via refactored `SetupFullPipelineInfrastructure`.
- Test-only components (Mock LLM, Prom, AM, event-exporter, mock-slack) available per existing E2E design.

**Test Steps**:

1. **Given**: E2E environment variables and images as per CI.
2. **When**: Run FullPipeline Ginkgo suite.
3. **Then**: All specs pass.

**Expected Results**:

1. Zero failures; same business outcomes as pre-Helm refactor.

**Acceptance Criteria**:

- **Behavior**: End-to-end remediation scenarios unchanged for users/tests.
- **Correctness**: No edits required to the five existing FullPipeline test files.

**Dependencies**: TP-239-001/002 recommended before full CI dependency; **R1** ordering validated here.

---

### TP-239-004 through TP-239-006 (P1/P2 summary)

| ID | Priority | Summary |
|----|----------|---------|
| TP-239-004 | P1 | With NodePort overrides, assert **localhost:30080** (Gateway) and **localhost:30081** (DataStorage) accept connections (or HTTP health) from the test runner. |
| TP-239-005 | P1 | With `E2E_COVERAGE=true` and chart coverage toggle, assert coverage files for **six** controllers exist post-suite (exact path per implementation). |
| TP-239-006 | P2 | CI job: drift guard on `helm template` output (RBAC rules, critical env vars, or diff vs golden) — mitigates **R5** and secret/env regressions. |

---

## 10. Environmental Needs

> **IEEE 829 §13** — Hardware, software, tools, and infrastructure required.

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory) if implemented under `test/unit/...`
- **Mocks**: N/A for pure manifest parsing; external **helm** binary may be invoked (document as test helper)
- **Location**: TBD under `test/unit/` or `test/infrastructure/`
- **Resources**: Minimal CPU/memory

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega if integrated with envtest/cluster; otherwise Makefile/CI
- **Mocks**: No mocks for Kubernetes — real cluster for full infra tests
- **Infrastructure**: As today for FullPipeline
- **Location**: `test/infrastructure/`, CI workflows

### 10.3 E2E Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Infrastructure**: Kind (or project-standard cluster), Docker/Podman, Helm 3.x
- **Location**: `test/e2e/fullpipeline/`
- **Resources**: Sufficient disk for images; hostPath for coverage when enabled (**R4**)

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | Per `go.mod` | Build and test |
| Ginkgo CLI | v2.x | E2E runner |
| Helm | 3.x | Template and install |
| Docker/Podman | Project standard | Cluster nodes |

---

## 11. Dependencies & Schedule

> **IEEE 829 §12/§16** — Remaining tasks, blocking issues, and execution order.

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| `values-e2e.yaml` | Config | Open until GREEN | TP-239-001 fails (RED expected) | None — deliver in GREEN |
| Chart coverage extension | Code | Open until GREEN | TP-239-005 fails | Feature-flag off default installs |
| CRD single-source (REFACTOR) | Process | May lag | R5 persists | TP-239-006 interim diff |

### 11.2 Execution Order (TDD)

1. **RED**: Add TP-239-001 test asserting `helm template` with `values-e2e.yaml` succeeds (**fails** until file exists).
2. **GREEN**: Add `charts/kubernaut/values-e2e.yaml`; refactor `SetupFullPipelineInfrastructure` to **helm install/upgrade** product stack; `kubectl` (or existing helpers) for test-only; add `e2e.coverage.enabled` chart support; NodePort overrides **30080**/**30081**.
3. **REFACTOR**: Remove redundant inline manifest builders; consolidate CRD source; add TP-239-006 drift guard.
4. **Verification**: TP-239-002 → TP-239-003 → TP-239-004 → TP-239-005 → TP-239-006 (as prioritized).

---

## 12. Test Deliverables

> **IEEE 829 §11** — What artifacts this test effort produces.

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/239/TEST_PLAN.md` | Strategy and traceability |
| `values-e2e.yaml` | `charts/kubernaut/values-e2e.yaml` | E2E overrides |
| Helm template / parity tests | TBD (`test/unit/...` or CI) | TP-239-001, TP-239-002 |
| Refactored infra | `test/infrastructure/fullpipeline_e2e.go` | Helm + test-only split |
| Chart template updates | `charts/kubernaut/templates/**` | Optional E2E coverage |
| Drift guard | CI workflow or script | TP-239-006 |
| Coverage artifacts | CI artifacts | When TP-239-005 enabled |

---

## 13. Execution

```bash
# Helm render (P0 smoke)
helm template kubernaut ./charts/kubernaut -f ./charts/kubernaut/values-e2e.yaml > /tmp/kubernaut-e2e.yaml

# Unit / infra tests (paths TBD after implementation)
go test ./test/unit/infrastructure/... -ginkgo.v

# FullPipeline E2E (exact target may match Makefile)
go test ./test/e2e/fullpipeline/... -ginkgo.v

# Example: focus by description (adjust to project conventions)
go test ./test/e2e/fullpipeline/... -ginkgo.focus="FullPipeline"
```

---

## 14. Existing Tests Requiring Updates (if applicable)

> When implementation changes behavior that existing tests assert on, document the
> required updates here to prevent surprises during TDD GREEN phase.

| Test ID / Location | Current Assertion | Required Change | Reason |
|--------------------|-------------------|-----------------|--------|
| **None (target)** | — | — | Issue #239 acceptance: **five** existing FullPipeline E2E files must pass **unchanged**. All changes live in `test/infrastructure/fullpipeline_e2e.go`, chart, values, and **new** TP-239 tests. |

If any existing file must change during implementation, update this table in the same PR with reviewer approval (exception to #239 constraint).

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-09 | Initial test plan for Issue #239 (Helm-based FullPipeline E2E) |
