# Test Plan: Fix Must-Gather Collector Drift + Deterministic Live-Cluster Drift Detection

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-2037-v1.0
**Feature**: Fix drifted `cmd/must-gather` collector scripts (stale CRD/service/namespace
enumeration lists) and revive + strengthen the dormant live-cluster bats E2E harness into a
CI-gated, deterministic drift detector.
**Version**: 1.0
**Created**: 2026-08-09
**Author**: AI Agent (Cursor)
**Status**: Implementation Complete (local unit/lint validation green; live-cluster `e2e-tests` CI job validated on PR push)
**Branch**: `fix/2037-must-gather-drift`

---

## 1. Introduction

### 1.1 Purpose

`cmd/must-gather` has never been exercised against a live Kubernetes cluster in CI — only
mocked-`kubectl` unit bats. As the project grew (6 -> 10 CRD types, 8 -> 12 chart-managed
services, 3-namespace -> single-release-namespace deployment model), the tool's static
enumeration lists silently rotted. It is already shipped to customers and today silently collects
incomplete diagnostic data (support engineers get partial CRD/log evidence without any signal that
collection was incomplete).

### 1.2 Objectives

1. `crds.sh`: replace the static 6-entry `CRD_TYPES` array with dynamic discovery of every
   `*.kubernaut.ai` CRD registered in the cluster — self-heals as CRDs are added/removed.
2. `gather.sh`: replace the 3-entry hardcoded `KUBERNAUT_NAMESPACES` array
   (`kubernaut-system`, `kubernaut-notifications`, `kubernaut-workflows`) with two explicit,
   overridable namespaces — `RELEASE_NAMESPACE` (default `kubernaut-system`, flag `--namespace=`)
   and `WORKFLOW_NAMESPACE` (default `kubernaut-workflows`, flag `--workflow-namespace=`) — and
   drop the obsolete `kubernaut-notifications` entry.
3. `logs.sh`: replace the static 8-entry `SERVICE_PATTERNS` pod-name-prefix allowlist with
   "collect logs from every pod in `RELEASE_NAMESPACE`", matching the drift-proof precedent
   already established by `test/infrastructure/datastorage.go`'s `MustGatherPodLogs`.
4. `datastorage.sh`: build `DATASTORAGE_URL` from `RELEASE_NAMESPACE` instead of a hardcoded
   `kubernaut-system` literal.
5. Revive the dormant `cmd/must-gather/test/integration/test_e2e.bats` live-cluster harness,
   strengthen its assertions from `count -gt 0` to exact/dynamic (compared against live-cluster
   truth, not a second hardcoded number that can itself drift), and wire it into CI as a new
   `e2e-tests` job so this class of drift is caught automatically going forward.
6. Zero regression in the existing 45 mocked-`kubectl` unit bats tests.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit bats pass rate | 100% | `make -C cmd/must-gather test` |
| New live-cluster E2E bats pass rate | 100% | new `e2e-tests` CI job |
| Shellcheck | Zero new warnings | `make -C cmd/must-gather lint` |
| Drift coverage | Collected CRD/log counts match live-cluster truth exactly (not `-gt 0`) | `test_e2e.bats` |
| Backward compatibility | 0 regressions | full `cmd/must-gather` unit bats suite stays green |

### 1.4 Preflight Correction: `KUBERNAUT_NAMESPACES` blast radius wider than originally scoped

The approved plan named 4 files (`crds.sh`, `gather.sh`, `logs.sh`, `datastorage.sh`). During RED,
direct code inspection found `KUBERNAUT_NAMESPACES` is *also* consumed unchanged by
`collectors/events.sh`, `collectors/metrics.sh`, and `collectors/cluster-state.sh` (7 loop sites
total). Rather than touch all 6 collector files, `gather.sh` continues to compute and export
`KUBERNAUT_NAMESPACES` exactly as before (so those 3 untouched collectors need zero changes) — only
its *contents* change, from the 3 hardcoded literals to
`("${RELEASE_NAMESPACE}" "${WORKFLOW_NAMESPACE}")`. `logs.sh` is the one collector that stops
consuming the array entirely (it now targets `RELEASE_NAMESPACE` only, since Tekton job-pod logs in
`WORKFLOW_NAMESPACE` are already collected more precisely by `tekton.sh`'s label-selector-based
`kubectl logs`).

**Follow-up finding (out of scope for this issue, flagged for the user)**: `collectors/metrics.sh`
has its own, separate hardcoded-endpoint drift — `http://data-storage.kubernaut-system:8080/metrics`
(wrong service name; chart service is `datastorage`, not `data-storage`) and
`http://notification-controller-metrics.kubernaut-notifications:8080/metrics` (obsolete namespace,
likely-wrong service name too). Not fixed here — not in the approved plan's scope, and metrics
collection failure is lower-severity than the CRD/log/namespace gaps this issue targets. Reported to
the user in this issue's closing summary rather than silently expanded or silently dropped.

---

## 2. References

### 2.1 Authority (governing documents)

- [Issue #2037](https://github.com/jordigilh/kubernaut/issues/2037): Fix must-gather collector
  drift (milestone v1.6)
- [Issue #2036](https://github.com/jordigilh/kubernaut/issues/2036): Integrate must-gather into
  E2E suites for continuous dogfooding (milestone v1.6.1, depends on this issue)
- **BR-PLATFORM-001** ([docs/requirements/BR-PLATFORM-001-must-gather-diagnostic-collection.md](../../requirements/BR-PLATFORM-001-must-gather-diagnostic-collection.md)):
  enterprise diagnostic collection tool, OpenShift must-gather pattern
- DD-WE-002: `kubernaut-workflows` namespace for Tekton PipelineRun execution
  (`pkg/workflowexecution/config/config.go`)

### 2.2 Cross-References

- Drift-proof precedent: `test/infrastructure/datastorage.go` `MustGatherPodLogs()` — collects
  logs from every pod in a namespace with zero per-service allowlist.
- CI recipe reuse: [.github/workflows/helm-smoke-test.yml](../../../.github/workflows/helm-smoke-test.yml)
  (Kind podman-provider cluster + `make image-build` + `helm install charts/kubernaut`).
- Container-test-runner reuse: [cmd/must-gather/Makefile](../../../cmd/must-gather/Makefile)'s
  `test-container` target (bats run inside the built must-gather image).
- Single-namespace deployment model confirmed via [scripts/helm-smoke-test.sh](../../../scripts/helm-smoke-test.sh)
  (`NAMESPACE="kubernaut-system"` default, single-install-guard hook,
  `charts/kubernaut/templates/*/*.yaml` all render into `{{ .Release.Namespace }}`).

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|-----------------|------------|
| R1 | New `e2e-tests` CI job (Kind + Helm + image build + containerized bats) runtime is unverified, may run long | Medium — CI cost/time | Medium | IT-MG-2037-005 | Scope to existing `cmd/must-gather/**` path filters (already narrow); 25-minute job timeout; reuse `helm-smoke-test.yml`'s image-build step rather than duplicating |
| R2 | `kind get kubeconfig --internal` + `--network kind` container-network join is a first use in this repo | Low — well-documented Kind capability, not novel experimentation | Low | IT-MG-2037-005 | Fallback documented: uncontainerized `make test-e2e` (already proven, host `kubectl` against Kind's host-mapped port) if container-network join proves flaky in first CI runs |
| R3 | Dynamic CRD discovery regex could match unrelated future CRDs with a similar suffix | Low | Low | UT-MG-2037-001, IT-MG-2037-001 | Anchor regex to `\.kubernaut\.ai$` |
| R4 | `logs.sh` no longer iterates `WORKFLOW_NAMESPACE` — could be perceived as losing coverage | Low — that namespace's pod logs (Tekton job pods) are already collected more precisely by `tekton.sh`'s `-l tekton.dev/pipelineRun=` label-selector logic | Certain (intentional) | IT-MG-2037-003 | Documented explicitly in Section 1.4; no duplicate/competing collection path introduced |
| R5 | `KUBERNAUT_NAMESPACES` has a wider blast radius (events.sh/metrics.sh/cluster-state.sh) than the original 4-file scope | Low if mishandled: could silently break 3 untouched collectors | N/A (caught during RED, before GREEN) | UT-MG-2037-002 | `gather.sh` keeps computing/exporting the same-shaped `KUBERNAUT_NAMESPACES` array (now 2 correct entries, not 3 stale ones) — zero changes required in events.sh/metrics.sh/cluster-state.sh |
| R6 (out of scope) | `metrics.sh` has its own separate hardcoded-endpoint drift (wrong service name, obsolete namespace) | Low — metrics collection is best-effort/non-blocking already | Certain (pre-existing) | N/A | Flagged to user in closing summary; not fixed in this issue (not in approved scope) |

### 3.1 Risk-to-Test Traceability

R1/R2 are CI-infrastructure risks proven acceptable by the new job's own successful first run
(IT-MG-2037-005) plus the documented fallback. R3 is proven by an explicit non-kubernaut CRD fixture
in UT-MG-2037-001 that must NOT appear in the collected set. R4 is a documented design decision, not
a defect — proven by IT-MG-2037-003 asserting `logs.sh` output covers exactly the release-namespace
pods. R5 is proven by UT-MG-2037-002 asserting `KUBERNAUT_NAMESPACES` still contains exactly 2
entries (both configurable) after `gather.sh` parses flags. R6 is explicitly out of scope, not
silently dropped.

---

## 4. Scope

### 4.1 Features to be Tested

- `crds.sh` dynamic `*.kubernaut.ai` CRD discovery (replacing the static 6-entry array).
- `gather.sh` `--namespace=`/`--workflow-namespace=` flag parsing, `RELEASE_NAMESPACE`/
  `WORKFLOW_NAMESPACE` defaults and export, corrected `KUBERNAUT_NAMESPACES` contents.
- `logs.sh` all-pod (no allowlist) log collection scoped to `RELEASE_NAMESPACE`.
- `datastorage.sh` `DATASTORAGE_URL` built from `RELEASE_NAMESPACE`.
- `test_e2e.bats` exact/dynamic live-cluster assertions (CRD completeness, log completeness).
- New `e2e-tests` CI job wiring (Kind + Helm chart + must-gather image, live-cluster run).

### 4.2 Features Not to be Tested

- `metrics.sh`, `events.sh`, `cluster-state.sh` internal logic (unchanged, out of this issue's
  scope beyond `KUBERNAUT_NAMESPACES` contents correctness, covered indirectly by R5's mitigation).
- Sanitization logic (`sanitizers/`) — unchanged.
- `tekton.sh` PipelineRun/TaskRun collection logic — unchanged (still hardcoded to
  `kubernaut-workflows`; acceptable since DD-WE-002 makes that a fixed architectural constant, not
  drift, and it is out of the approved plan's scope).
- Issue #2036 (Go/Ginkgo E2E suite integration) — explicitly deferred, milestone v1.6.1.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| `crds.sh` uses `kubectl get crd -o name \| grep -E '\.kubernaut\.ai$'` instead of a maintained allowlist | Eliminates this drift class permanently — self-heals as CRDs are added/removed, no code change needed |
| `logs.sh` collects all pods in `RELEASE_NAMESPACE`, no per-service pattern list | Matches the existing `MustGatherPodLogs` precedent; eliminates this drift class permanently |
| `gather.sh` keeps exporting `KUBERNAUT_NAMESPACES` (2 corrected entries) rather than refactoring all 6 collectors | Minimizes blast radius (Section 1.4) — `events.sh`/`metrics.sh`/`cluster-state.sh` need zero changes |
| `kubernaut-notifications` dropped, not replaced | Confirmed obsolete: no chart template or live code path deploys anything there (notification-controller now renders into `{{ .Release.Namespace }}` like every other service) |
| Shared `utils/namespace.sh` helper (REFACTOR phase) for default-resolution | Avoids copy-pasting `: "${RELEASE_NAMESPACE:=kubernaut-system}"` across `logs.sh`/`datastorage.sh`; closes off this exact class of drift recurring |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit (mocked `kubectl`/`curl`)**: 100% of new/changed branches in the 4 collector scripts —
  dynamic CRD discovery, namespace flag parsing/defaults, all-pod log collection, datastorage URL
  construction.
- **Integration (live Kind cluster + real Helm-installed chart)**: proves the actual production
  entry point (`/usr/bin/gather` inside the built must-gather image) collects a complete,
  cluster-truth-verified set of CRDs and service logs — the deterministic drift detector this issue
  exists to create.

### 5.2 Two-Tier Minimum

Every GREEN-phase script change has both a mocked unit bats case (proves the logic in isolation)
and is exercised by the strengthened live-cluster `test_e2e.bats` (proves the wiring through the
actual built image) — satisfying the Pyramid Invariant (UT proves logic, IT proves wiring).

### 5.4 Pass/Fail Criteria

**PASS**: all UT-MG-2037-* and IT-MG-2037-* cases pass; `make -C cmd/must-gather test` (45 existing
+ new unit bats) stays green; new `e2e-tests` CI job passes; `make -C cmd/must-gather lint`
(shellcheck) clean.

**FAIL**: any existing unit bats regresses; the live-cluster job under-collects relative to cluster
truth; any of the 4 scripts silently swallows a `kubectl`/`curl` error without documenting it in the
collection output (existing `|| echo "Warning: ..."` pattern must be preserved).

---

## 6. Test Items

### 6.1 Script-Level Logic Under Test

| File | Functions/Sections | Lines (approx) |
|------|--------------------|-----------------|
| `cmd/must-gather/collectors/crds.sh` | `CRD_TYPES` discovery | ~10 |
| `cmd/must-gather/gather.sh` | argument parsing, `RELEASE_NAMESPACE`/`WORKFLOW_NAMESPACE`/`KUBERNAUT_NAMESPACES` | ~20 |
| `cmd/must-gather/collectors/logs.sh` | pod discovery loop | ~30 |
| `cmd/must-gather/collectors/datastorage.sh` | `DATASTORAGE_URL` construction | ~2 |
| `cmd/must-gather/utils/namespace.sh` (new, REFACTOR) | shared default-resolution | ~10 |

### 6.2 CI/Infra Artifacts (verified via successful job run, not hand-unit-tested)

| File | Change |
|------|--------|
| `.github/workflows/must-gather-tests.yml` | new `e2e-tests` job |
| `cmd/must-gather/Makefile` | new `test-e2e-container` target |
| `cmd/must-gather/test/integration/test_e2e.bats` | strengthened assertions |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID(s) | Status |
|-------|-------------|----------|------|------------|--------|
| BR-PLATFORM-001.2 | Support engineer gets complete CRD state (all registered `*.kubernaut.ai` types, not a stale subset) | P0 | Unit + Integration | UT-MG-2037-001, IT-MG-2037-001 | Done |
| BR-PLATFORM-001.3 | Support engineer gets complete service log coverage (all pods in the release namespace, not a stale allowlist) | P0 | Unit + Integration | UT-MG-2037-003, IT-MG-2037-003 | Done |
| BR-PLATFORM-001 | Collection targets the correct, configurable release/workflow namespaces matching the current single-namespace deployment model | P1 | Unit | UT-MG-2037-002 | Done |
| BR-PLATFORM-001.6a | DataStorage API collection targets the correct release namespace | P1 | Unit | UT-MG-2037-004 | Done |
| BR-PLATFORM-001.3.4 | End-to-end must-gather run against a live cluster is deterministically verified complete, gating every PR touching `cmd/must-gather` | P0 | Integration | IT-MG-2037-001..005 | Done |

---

## 8. Test Scenarios

### Tier 1: Unit Tests (mocked `kubectl`/`curl`)

| ID | Business Outcome Under Test | Phase |
|----|------------------------------|-------|
| UT-MG-2037-001 | `crds.sh` collects every `*.kubernaut.ai` CRD returned by `kubectl get crd -o name` (including types absent from the old static list), and ignores non-kubernaut CRDs | RED/GREEN |
| UT-MG-2037-002 | `gather.sh` resolves `RELEASE_NAMESPACE`/`WORKFLOW_NAMESPACE` from `--namespace=`/`--workflow-namespace=` flags (overridden) and from defaults (unset), and `KUBERNAUT_NAMESPACES` contains exactly those 2 values (no `kubernaut-notifications`) | RED/GREEN |
| UT-MG-2037-003 | `logs.sh` collects logs from every pod in `RELEASE_NAMESPACE` regardless of name, including services absent from the old `SERVICE_PATTERNS` allowlist (e.g. `authwebhook-*`, `apifrontend-*`) | RED/GREEN |
| UT-MG-2037-004 | `datastorage.sh` builds `DATASTORAGE_URL` using the configured `RELEASE_NAMESPACE`, not a hardcoded `kubernaut-system` literal | RED/GREEN |

### Tier 2: Integration Tests (live Kind cluster, real Helm-installed chart, containerized must-gather image)

| ID | Business Outcome Under Test | Phase |
|----|------------------------------|-------|
| IT-MG-2037-001 | Collected CRD directory count exactly equals the live cluster's `*.kubernaut.ai` CRD count (not `-gt 0`) | RED/GREEN |
| IT-MG-2037-002 | Collection targets the actual chart release namespace; `gather.sh --namespace=` override is honored end-to-end | RED/GREEN |
| IT-MG-2037-003 | Every live pod in the release namespace has a corresponding collected `logs/<ns>/<pod>/current.log` | RED/GREEN |
| IT-MG-2037-004 | DataStorage API collection succeeds against the live in-cluster service (URL correctly resolves) | RED/GREEN |
| IT-MG-2037-005 | Full containerized `/usr/bin/gather` run (the actual shipped image, not raw host scripts) completes successfully against the live cluster via the new `e2e-tests` CI job | RED/GREEN |

### Tier Skip Rationale

- **E2E** (full Ginkgo fullpipeline/fleet suites): not required — this issue's own Tier 2 live-Kind
  integration tests already exercise the real production artifact (the built container image)
  against a real cluster; a further Go/Ginkgo E2E layer is exactly what issue #2036 adds
  (milestone v1.6.1), intentionally sequenced after this issue.

---

## 9. Test Cases (P0 detail)

### UT-MG-2037-001: crds.sh dynamic CRD discovery

**BR**: BR-PLATFORM-001.2
**Priority**: P0
**Type**: Unit (bats, mocked `kubectl`)
**File**: `cmd/must-gather/test/test_crds.bats`

**Test Steps**:
1. **Given**: mocked `kubectl get crd -o name` returns a list including `actiontypes.kubernaut.ai`
   and `effectivenessassessments.kubernaut.ai` (both absent from the old static 6-entry list) plus
   one unrelated CRD (`widgets.example.com`).
2. **When**: running `crds.sh`.
3. **Then**: collection directories exist for both new types; no directory exists for
   `widgets.example.com`.

**Acceptance Criteria**: exact set match, no false positives/negatives.

### UT-MG-2037-003: logs.sh all-pod collection

**BR**: BR-PLATFORM-001.3
**Priority**: P0
**Type**: Unit (bats, mocked `kubectl`)
**File**: `cmd/must-gather/test/test_logs.bats`

**Test Steps**:
1. **Given**: mocked `kubectl get pods -n kubernaut-system --no-headers` returns pods including
   `authwebhook-abc123` and `apifrontend-xyz789` (both absent from the old `SERVICE_PATTERNS`
   allowlist), and mocked `kubectl logs` returns canned content.
2. **When**: running `logs.sh` with `RELEASE_NAMESPACE=kubernaut-system`.
3. **Then**: `logs/kubernaut-system/authwebhook-abc123/current.log` and
   `logs/kubernaut-system/apifrontend-xyz789/current.log` both exist and contain the mocked log
   content.

**Acceptance Criteria**: previously-unmatched services are now collected, proving the allowlist
removal actually took effect (not a false-positive pre-existing-file artifact — see Section 15).

### IT-MG-2037-001: Live-cluster CRD completeness

**BR**: BR-PLATFORM-001.2
**Priority**: P0
**Type**: Integration (bats, live Kind cluster)
**File**: `cmd/must-gather/test/integration/test_e2e.bats`

**Test Steps**:
1. **Given**: a Kind cluster with the Kubernaut Helm chart installed (all CRDs registered).
2. **When**: running the built must-gather image's `/usr/bin/gather` against the cluster.
3. **Then**: collected CRD directory count equals `kubectl get crd -o name | grep -c
   '\.kubernaut\.ai$'` evaluated against the same live cluster.

**Acceptance Criteria**: exact match, computed at test time (self-correcting as CRDs are
added/removed in future releases).

---

## 10. Environmental Needs

- **Unit**: bats-core 1.11.0 (existing container-based `make test-container` harness), mocked
  `kubectl`/`curl`, no external infra.
- **Integration**: Kind cluster (podman provider, `KIND_EXPERIMENTAL_PROVIDER=podman`), Helm 3,
  `charts/kubernaut` installed with default values, must-gather image built locally
  (`make -C cmd/must-gather build-local`), `kind get kubeconfig --internal` for in-network access.

---

## 11. Dependencies & Schedule

No blocking dependencies within this issue. Depends on nothing upstream; issue #2036 (milestone
v1.6.1) depends on this issue completing first. Execution order: RED (test plan +
strengthened/new UT+IT bats, CI job skeleton, all failing against current code) -> GREEN (4 script
fixes + CI wiring, all green) -> REFACTOR (shared namespace helper, shellcheck, README) -> final
validation.

---

## 12. Test Deliverables

| Deliverable | Location |
|-------------|----------|
| This test plan | `docs/testing/2037/TEST_PLAN.md` |
| Updated collector scripts | `cmd/must-gather/collectors/crds.sh`, `cmd/must-gather/gather.sh`, `cmd/must-gather/collectors/logs.sh`, `cmd/must-gather/collectors/datastorage.sh` |
| New shared helper | `cmd/must-gather/utils/namespace.sh` |
| Updated unit bats | `cmd/must-gather/test/test_crds.bats`, `test_logs.bats`, `test_datastorage.bats`, `test_gather_main.bats`, `helpers.bash` |
| Strengthened live-cluster bats | `cmd/must-gather/test/integration/test_e2e.bats` |
| New CI job | `.github/workflows/must-gather-tests.yml` (`e2e-tests`) |
| New Makefile target | `cmd/must-gather/Makefile` (`test-e2e-container`) |
| Updated docs | `cmd/must-gather/README.md` (flags table) |

---

## 13. Execution

```bash
# Unit (containerized, host-agnostic)
make -C cmd/must-gather test

# Lint
make -C cmd/must-gather lint

# Live-cluster integration (requires a real cluster + KUBECONFIG)
KUBERNAUT_E2E_TESTS=1 make -C cmd/must-gather test-e2e-container
```

---

## 14. Wiring Verification (TDD Phase 4)

| Component | Production Entry Point | Wiring Code Location | Test ID(s) |
|-----------|-------------------------|------------------------|------------|
| Dynamic CRD discovery | must-gather image `ENTRYPOINT /usr/bin/gather` | `cmd/must-gather/collectors/crds.sh` | UT-MG-2037-001, IT-MG-2037-001 |
| `RELEASE_NAMESPACE`/`WORKFLOW_NAMESPACE` flags | `/usr/bin/gather` | `cmd/must-gather/gather.sh` | UT-MG-2037-002, IT-MG-2037-002 |
| All-pod log collection | `/usr/bin/gather` | `cmd/must-gather/collectors/logs.sh` | UT-MG-2037-003, IT-MG-2037-003 |
| DataStorage URL namespace parameterization | `/usr/bin/gather` | `cmd/must-gather/collectors/datastorage.sh` | UT-MG-2037-004, IT-MG-2037-004 |
| Live-cluster drift-detection CI gate | `.github/workflows/must-gather-tests.yml` (`e2e-tests` job, every PR touching `cmd/must-gather/**`) | `cmd/must-gather/test/integration/test_e2e.bats` + `cmd/must-gather/Makefile` `test-e2e-container` | IT-MG-2037-005 |

---

## 15. Existing Tests Requiring Updates

- `cmd/must-gather/test/helpers.bash` — `mock_kubectl` gains a `get crd -o name` branch (list of
  CRD names, not a single CRD's YAML); `get pods ... --no-headers` gains a plain-text branch
  distinct from the existing `-o yaml` PodList branch; `mock_curl` gains argv logging so tests can
  assert the requested URL. `setup_test_environment` exports `RELEASE_NAMESPACE`/
  `WORKFLOW_NAMESPACE` in place of the 3-entry `KUBERNAUT_NAMESPACES`.
- `cmd/must-gather/test/test_logs.bats` — the "all 8 V1.0 services" test
  (pre-creates destination files before running `logs.sh`, so it never actually proved collection
  — see UT-MG-2037-003's design) is replaced with a genuine collection-proof case using previously
  unmatched service names.
- `cmd/must-gather/test/test_crds.bats`, `test_datastorage.bats`, `test_gather_main.bats` — extended
  with the new cases above; no existing case's assertions are weakened.
- `cmd/must-gather/test/integration/test_e2e.bats` — assertions strengthened from `-gt 0` to
  exact/dynamic per Section 9.

No pre-existing case is expected to regress — all changes are either additive or fix a
demonstrated false-positive in the existing suite (see `test_logs.bats` note above).

---

## 16. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-08-09 | Initial test plan, written before RED phase begins. |
