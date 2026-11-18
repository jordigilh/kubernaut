# Test Plan: Issue #2650 — Fingerprint Group Dimension

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-2650-v1.0
**Feature**: Include the API group in the deduplication fingerprint hash (BR-GATEWAY-004 cross-adapter consistency)
**Version**: 1.0
**Created**: 2026-11-16
**Author**: Issue #2650 implementation (approved plan v4)
**Status**: Active
**Branch**: `2650-fingerprint-group-dimension`

---

## 1. Introduction

> **IEEE 829 §3** — Purpose, objectives, and measurable success criteria for the test effort.

### 1.1 Purpose

Two resources with the same namespace/kind/name but in different API groups (e.g.
`route.openshift.io` Route vs `serving.knative.dev` Route) currently collide onto one
deduplication fingerprint. This plan proves the fingerprint hash gains an API-group
dimension across the Gateway (GW) and API-Frontend (AF) paths, that the new
`api_version` input on `kubernaut_check_existing_remediation` is validated (SI-10), and
that GW and AF keep producing identical fingerprints for identical cluster+GVK inputs
(AC-4, scoped to top-level-owner resources).

Maintainer decision: NO migration, NO backward compatibility — clean installs only.

### 1.2 Objectives

1. **Group-distinct fingerprints**: same ns/kind/name in two API groups produce distinct fingerprints at UT and IT level.
2. **Determinism preserved**: identical inputs (same group) produce identical hashes; same group different version (`apps/v1` vs `apps/v2`) produces the SAME fingerprint (group-only dimension).
3. **Owner-path group seeding (M1)**: when owner resolution returns an empty APIVersion, the input resource's apiVersion seeds the group.
4. **SI-10**: missing/invalid `api_version` on `kubernaut_check_existing_remediation` is rejected with `ErrInvalidInput` through the real tool closure.
5. **AC-4 parity**: GW and AF produce identical fingerprints for identical cluster+GVK standalone resources.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./pkg/gateway/types/... ./pkg/apifrontend/...` |
| Integration test pass rate | 100% | `go test ./test/integration/gateway/... ./pkg/apifrontend/agent/...` |
| Zero regressions | 0 failing pre-existing suites | `make test` |
| UT scenario coverage of group contract | 100% (expected) | UT-GW-2650-001..007 cover the full group-segment decision table |

---

## 2. References

### 2.1 Authority (governing documents)

- BR-GATEWAY-004: Cross-adapter deduplication consistency (primary)
- SI-10 (FedRAMP): Information input validation — new `api_version` tool input
- AC-4 (FedRAMP, scoped): GW/AF fingerprint parity for top-level-owner resources (DD-GATEWAY-019 Alternative 2)
- Issue #2650: Fingerprint group dimension
- Issue #2592: apiVersion-aware owner resolution (strict GVK contract)
- Issue #2066 / DD-KA-006: prometheus ambiguous-group empty-apiVersion case — known residual #2650 gap, out of scope
- Approved implementation plan: `docs/tests/2650/PLAN.md`

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Integration/E2E No-Mocks Policy](../../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Fixture churn: existing suites construct fingerprints without APIVersion | High (failing suites) | High | Fixture sweep | Two deterministic fixture rules (plan §Fixture rules); RED-first makes each miss a mechanical fix pointed at by a failing assertion |
| R2 | Legacy persisted RRs never match new fingerprints | Accepted | Certain | — | Clean installs only, no migration (maintainer decision) |
| R3 | Empty-group signals colliding with a real group | None | None | UT-GW-2650-004 | Impossible: real groups are non-empty; empty apiVersion is the prometheus ambiguity case (residual gap, out of scope) |
| R4 | Malformed apiVersion collapsing to empty-group segment | High | Low | UT-GW-2650-005 | Raw-string fallback via `scope.ParseAPIVersion` rejection; verified unreachable in production (defense-in-depth) |
| R5 | GW hashes owner's group, AF hashes target's group | Pre-existing seam | — | Parity test | AC-4 claim scoped to top-level-owner parity; proven by UT-AF-2650-004 |

### 3.1 Risk-to-Test Traceability

- R1 → fixture sweep rules (existing suites; no new test IDs)
- R4 → UT-GW-2650-005
- R5 → UT-AF-2650-004 (parity)

---

## 4. Scope

### 4.1 Features to be Tested

- **Group-aware hash** (`pkg/gateway/types/fingerprint.go`): `CalculateClusterAwareFingerprint` hash input becomes `clusterID:group:namespace:kind:name`.
- **Owner-path group seeding** (`pkg/gateway/types/fingerprint.go`): `ResolveFingerprintWithCluster` seeds the owner identity's APIVersion from the input resource when resolution returns empty.
- **AF create path** (`pkg/apifrontend/tools/af_create_rr.go`): `rrFingerprintWithCluster` gains an `apiVersion` parameter; all KA/AF tools funnel through `HandleCreateRR`.
- **AF check path** (`pkg/apifrontend/tools/af_check_existing_rr.go`): `CheckExistingRRArgs` gains required `APIVersion`, validated non-empty.

### 4.2 Features Not to be Tested

- Prometheus ambiguous-group empty-apiVersion dedup collapse (pre-existing #2066, DD-KA-006 contract, audit-logged)
- Any migration/back-compat scheme (none exists by decision)

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Group extraction via `scope.ParseAPIVersion`, raw-string fallback on error | Bare `schema.ParseGroupVersion` silently accepts slash-less group names, collapsing a group into the empty-group segment |
| Group-only dimension (version excluded) | Issue decision: `apps/v1` vs `apps/v2` is the same dedup target |
| No migration | Maintainer decision: clean installs only |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: 100% of in-scope business scenarios (the full group-segment decision table: distinct groups, determinism, cluster+group combination, empty group, malformed fallback, same-group versions, owner-path seeding)
- **Integration**: 100% of Wiring Manifest rows (plan §Wiring Manifest)
- **E2E**: existing a2a/fullpipeline suites via fixture sweep (mock-llm.yaml `api_version` arg), plus the dedicated cross-group journey `E2E-GW-2650-001` on the deployed Gateway (maintainer request; see Tier 3 below)

### 5.2 Two-Tier Minimum

BR-GATEWAY-004 and SI-10 each get UT + IT coverage. AC-4 parity is proven at IT level.

### 5.3 Business Outcome Quality Bar

Tests validate deduplication business outcomes: two cross-group signals → two RRs; missing `api_version` → typed input rejection.

### 5.4 Pass/Fail Criteria

**PASS** — all of the following:

1. All RED tests (UT-GW-2650-001..007, UT-AF-2650-001..003, IT-GW-2650-001, IT-AF-2650-001, parity) pass
2. `go build ./...`, `golangci-lint run --timeout=5m`, `make test` green; zero regressions
3. CHECKPOINT W passes: no fingerprint call site left without the group

**FAIL** — any P0 test fails, or any pre-existing suite regresses without a fixture-rule justification.

### 5.5 Suspension & Resumption Criteria

**Suspend**: envtest unavailable; >5 fixture failures for the same non-rule-covered root cause (investigate before sweeping further).
**Resume**: infrastructure restored; root cause mapped to a fixture rule or escalated.

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/gateway/types/fingerprint.go` | `CalculateClusterAwareFingerprint`, `ResolveFingerprintWithCluster` | 69-179 |
| `pkg/apifrontend/tools/af_create_rr.go` | `rrFingerprintWithCluster`, `HandleCreateRR` (fingerprint plumbing) | 165-174, 270 |
| `pkg/apifrontend/tools/af_check_existing_rr.go` | `HandleCheckExistingRR` (api_version validation + fingerprint) | 44-91 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/gateway/adapters/kubernetes_event_adapter.go` | `Parse` → `ResolveFingerprint` | ~227 |
| `pkg/gateway/adapters/prometheus_adapter.go` | `resolveAlertForParse`/`resolveAlertFingerprint` → `ResolveFingerprintWithCluster` | ~392, ~418 |
| `pkg/apifrontend/tools/af_check_existing_rr.go` | `NewCheckExistingRemediationTool` closure | 96-103 |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | branch `2650-fingerprint-group-dimension` HEAD | Clean installs only (no back-compat) |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-GATEWAY-004 | Cross-group fingerprints distinct (GW hash) | P0 | Unit | UT-GW-2650-001..006 | Pending |
| BR-GATEWAY-004 | Owner-path group seeding | P0 | Unit | UT-GW-2650-007 | Pending |
| BR-GATEWAY-004 | Cross-group signals → two RRs through ProcessSignal | P0 | Integration | IT-GW-2650-001 | Pending |
| BR-GATEWAY-004 | Cross-group events → two RRs on deployed Gateway | P0 | E2E | E2E-GW-2650-001 | Pending |
| BR-GATEWAY-004 | AF create path group dimension | P0 | Unit | UT-AF-2650-001 | Pending |
| BR-GATEWAY-004 | AF check path group dimension | P0 | Unit | UT-AF-2650-002 | Pending |
| SI-10 | `api_version` validated on CheckExistingRRArgs | P0 | Unit | UT-AF-2650-003 | Pending |
| SI-10 | `api_version` validated through real tool closure | P0 | Integration | IT-AF-2650-001 | Pending |
| AC-4 (scoped) | GW/AF fingerprint parity for identical cluster+GVK | P0 | Integration | UT-AF-2650-004 | Pending |

### Status Legend

- **Pending** / **RED** / **GREEN** / **REFACTORED** / **Pass**

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{BR_NUMBER}-{SEQUENCE}` (issue-numbered sequences per #2650).

### Tier 1: Unit Tests

**Scope**: `pkg/gateway/types`, `pkg/apifrontend/tools`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-GW-2650-001` | Same ns/kind/name, two groups (`route.openshift.io/v1` vs `serving.knative.dev/v1`, both `Route`) → distinct fingerprints | Pending |
| `UT-GW-2650-002` | Same resource (same group) → deterministic identical hash | Pending |
| `UT-GW-2650-003` | Cluster+group combined: different cluster same group → distinct; different group same cluster → distinct | Pending |
| `UT-GW-2650-004` | Empty apiVersion → stable, distinct from any non-empty group | Pending |
| `UT-GW-2650-005` | Malformed non-empty apiVersion (slash-less group name `"apps"`, multi-slash `"a/b/c"`) does not collapse to the empty-group hash (raw-string fallback) | Pending |
| `UT-GW-2650-006` | `apps/v1` vs `apps/v2` (same group) → SAME fingerprint (group-only dimension) | Pending |
| `UT-GW-2650-007` | Resource apiVersion known, owner resolution returns empty APIVersion → fingerprint still carries the resource's group (M1 seeding) | Pending |
| `UT-AF-2650-001` | `HandleCreateRR` with two targets differing only in group → two RRs with distinct `SignalFingerprint` | Pending |
| `UT-AF-2650-002` | `HandleCheckExistingRR` matching ns/kind/name but different group → `Exists: false` | Pending |
| `UT-AF-2650-003` | Missing/invalid `api_version` on CheckExistingRRArgs → `ErrInvalidInput` (SI-10) | Pending |

### Tier 2: Integration Tests

**Scope**: `test/integration/gateway`, `pkg/apifrontend/agent`, `pkg/apifrontend/tools`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-GW-2650-001` | Two cross-group signals through `ProcessSignal` production dispatch (adapter → `ResolveFingerprintWithCluster` → CRDCreator) → two RRs; includes standalone-resource cross-group case proving M1 seeding with the registry-miss gap deliberately constructed | Pending |
| `IT-AF-2650-001` | Through the production tool closure (`NewCheckExistingRemediationTool` → `functiontool.New`), missing `api_version` → `ErrInvalidInput` | Pending |
| `UT-AF-2650-004` | GW/AF parity (AC-4): identical cluster+GVK standalone resource → identical fingerprint from the GW path (`ResolveFingerprintWithCluster`) and the AF path (`HandleCreateRR`'s RR `SignalFingerprint`) | Pending |

### Tier 3: E2E Tests

Existing suites cover the changed paths via fixture sweep (mock-llm.yaml `api_version` arg, a2a TC-E2E-A2A-T18, fullpipeline `rrFingerprint` helper). Additionally, per the maintainer's #2650 request for cross-group E2E coverage, `E2E-GW-2650-001` (`test/e2e/gateway/40_fingerprint_group_dimension_test.go`) proves the journey on a deployed Gateway: two Route CRDs (route.openshift.io, serving.knative.dev) + same-named standalone Route CRs in a managed namespace, two Warning events with the respective apiVersions → two RRs with distinct fingerprints and correct `TargetResource.APIVersion`.

### Tier Skip Rationale

- ~~**E2E (new)**: the group dimension is fully proven at UT+IT; existing E2E journeys exercise the changed paths after the fixture sweep.~~ Superseded: E2E-GW-2650-001 added (maintainer request, CodeRabbit follow-up).

---

## 9. Test Cases

### UT-GW-2650-001: cross-group distinctness

**BR**: BR-GATEWAY-004
**Priority**: P0
**Type**: Unit
**File**: `pkg/gateway/types/fingerprint_group_test.go`

**Test Steps**:
1. **Given**: `ResourceIdentifier{Namespace: "prod", Kind: "Route", Name: "shared-route"}` with `APIVersion: "route.openshift.io/v1"` and again with `"serving.knative.dev/v1"`
2. **When**: `CalculateClusterAwareFingerprint` is called for both
3. **Then**: the two fingerprints differ

### UT-GW-2650-005: malformed raw-string fallback

**BR**: BR-GATEWAY-004
**Priority**: P0
**Type**: Unit
**File**: `pkg/gateway/types/fingerprint_group_test.go`

**Test Steps**:
1. **Given**: apiVersion `"apps"` (slash-less group name — silently accepted by `schema.ParseGroupVersion`, rejected by `scope.ParseAPIVersion`) and `"a/b/c"`
2. **When**: fingerprints are computed
3. **Then**: neither equals the empty-apiVersion (empty-group) fingerprint, and the two differ from each other

### UT-GW-2650-007: owner-path group seeding (M1)

**BR**: BR-GATEWAY-004
**Priority**: P0
**Type**: Unit
**File**: `pkg/gateway/types/fingerprint_group_test.go`

**Test Steps**:
1. **Given**: an `OwnerResolverWithAPIVersion` resolver returning `OwnerResolution{Kind, Name, APIVersion: ""}` (registry-miss gap) and an input resource with `APIVersion: "route.openshift.io/v1"`
2. **When**: `ResolveFingerprintWithCluster` runs
3. **Then**: the fingerprint equals `CalculateClusterAwareFingerprint` of the same identity carrying `APIVersion: "route.openshift.io/v1"`

### UT-AF-2650-001/002/003, IT-GW-2650-001, IT-AF-2650-001/002

Detailed in plan §TDD phase mapping (`docs/tests/2650/PLAN.md`); scenarios as tabulated in Section 8.

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: external deps only (triager prom client, scope checker) — same doubles as existing `af_create_rr_test.go`
- **Location**: `pkg/gateway/types/`, `pkg/apifrontend/tools/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Infrastructure**: envtest (gateway IT), fake controller-runtime client + real `functiontool.New` closure (AF ITs)
- **Location**: `test/integration/gateway/`, `pkg/apifrontend/agent/`, `pkg/apifrontend/tools/`

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | repo toolchain | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| envtest | repo pinned | Gateway integration |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

None — plan approved; all production surfaces verified in preflight.

### 11.2 Execution Order

1. **Phase 1 (RED)**: UT-GW-2650-001..007, UT-AF-2650-001..003, IT-GW-2650-001, IT-AF-2650-001, UT-AF-2650-004 — all fail
2. **Phase 2 (GREEN)**: Changes 1-4 (group-aware hash; owner-path seeding; AF create apiVersion param; CheckExistingRRArgs.APIVersion + fallout) + fixture sweep
3. **Phase 3 (REFACTOR)**: dedupe hash input builder / `groupSegment` helper; doc comments; invariant comment on plain-resolver fallback
4. **Phase 4 (WIRING VERIFICATION)**: CHECKPOINT W per Wiring Manifest

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/2650/TEST_PLAN.md` | Strategy and test design |
| UT suite (GW) | `pkg/gateway/types/fingerprint_group_test.go` | Group-dimension BDD specs |
| UT suite (AF) | `pkg/apifrontend/tools/fingerprint_group_2650_test.go` | AF group-dimension BDD specs |
| IT suite (GW) | `test/integration/gateway/fingerprint_group_2650_integration_test.go` | ProcessSignal cross-group journey |
| IT suite (AF) | `pkg/apifrontend/agent/check_existing_api_version_2650_it_test.go` | Tool-closure SI-10 proof |
| Fixture sweep | per plan §Fixture rules | Existing suites aligned to the two rules |

---

## 13. Execution

```bash
# Unit tests
go test ./pkg/gateway/types/... -ginkgo.focus="UT-GW-2650"
go test ./pkg/apifrontend/tools/... -ginkgo.focus="UT-AF-2650"

# Integration tests
go test ./test/integration/gateway/... -ginkgo.focus="IT-GW-2650"
go test ./pkg/apifrontend/agent/... -ginkgo.focus="IT-AF-2650"

# Full gates
go build ./...
golangci-lint run --timeout=5m
make test
```

---

## 14. Wiring Verification (TDD Phase 4)

| Code Path | Entry Point | Exit Point | Wiring IT | Status |
|-----------|-------------|------------|-----------|--------|
| group-aware `CalculateClusterAwareFingerprint` | GW signal ingestion (both adapters) | RR `spec.signalFingerprint` | IT-GW-2650-001 | Pending |
| `ResolveFingerprintWithCluster` group seeding (M1) | same | same | IT-GW-2650-001 (standalone case) | Pending |
| `rrFingerprintWithCluster` + apiVersion | `HandleCreateRR` (all KA/AF tools) | RR `spec.signalFingerprint` | UT-AF-2650-001 + `integration_wiring_test.go` | Pending |
| `CheckExistingRRArgs.APIVersion` | `kubernaut_check_existing_remediation` | `ErrInvalidInput` / dedup lookup | UT-AF-2650-002/003 + IT-AF-2650-001 | Pending |

---

## 15. Existing Tests Requiring Updates

Governed by the plan's two deterministic fixture rules (plan §Fixture rules):

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `pkg/gateway/owner_resolver_k8s_test.go` (~:640) | expected FP hashes identity without APIVersion | expected FP carries the chain's apiVersion | Rule 1: real ownerchain with apiVersion-bearing OwnerReferences |
| `af_create_rr_test.go` seeded RRs (~:464,685,797,948,1014,1062) | seeded fingerprints hash without APIVersion | seeded identities carry the args' apiVersion | Rule 2: seeded RR fingerprints and lookup args must agree on the group |
| `af_check_existing_rr_test.go` (~19,141,157,193 + call sites) | `testFingerprint` and args without APIVersion | helper + args carry apiVersion | Rule 2 |
| `integration_wiring_test.go:49` | `CheckExistingRRArgs` without APIVersion | add APIVersion | Rule 2 |
| `mock-llm.yaml` a2a-t18 args | no `api_version` | add `api_version` | Rule 2 (AF fixtures via args) |
| `af_helpers_test.go` `rrFingerprint` | no apiVersion param | gains apiVersion parameter | Rule 2 |

---

## 16. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-11-16 | Initial test plan |
