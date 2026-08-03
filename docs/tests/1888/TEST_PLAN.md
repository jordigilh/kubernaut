# Test Plan: RESTMapper cache self-heals on lookup failure (#1888) — `main`/v1.6 port

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1888-main-v1.0
**Version**: 1.0
**Created**: 2026-08-03
**Author**: AI Assistant
**Status**: Implemented
**Branch**: `fix/1888-1890-restmapper-cache-invalidation-main` (targets `main`)

---

## 1. Introduction

### 1.1 Purpose

This is the `main`/v1.6 port of the fix designed, TDD'd, and CI-validated on `release/v1.5` in
[PR #1896](https://github.com/jordigilh/kubernaut/pull/1896) (branch
`fix/1888-restmapper-cache-invalidation`), tracked on `main` by sibling issue
[#1890](https://github.com/jordigilh/kubernaut/issues/1890). Full root-cause analysis and
alternatives are documented in
[DD-K8S-001](../../architecture/decisions/DD-K8S-001-restmapper-cache-invalidation-on-lookup-failure.md),
ported alongside this test plan — not repeated here.

### 1.2 Codebase-structure comparison (confirmed by direct inspection)

Unlike the #1889/#1892 port (PR #1897), which needed to account for meaningful structural drift
in `internal/kubernautagent/investigator`, `pkg/shared/k8s/gvk.go` and `gvk_test.go` were found to
be **byte-identical** between `origin/release/v1.5` (pre-fix) and `origin/main` at the time of this
port (verified via direct diff before editing, CHECKPOINT A). The `release/v1.5` fix and its full
test suite were therefore applied to `main` as-is, with no adaptation required beyond copying the
change.

### 1.3 Success Metrics

| Metric | Target | Measurement | Result |
|--------|--------|-------------|--------|
| Unit test pass rate | 100% | `make test-unit-shared-packages` | 37/37 suites passed |
| `ResolveGVKForKind` line coverage | 100% (new retry branch fully exercised) | `go tool cover -func` on the make target's coverage output | 100.0% |
| Backward compatibility | 0 regressions | Full `make test-unit-shared-packages` suite | 0 failed |
| Build/lint | Clean | `go build ./...`, `go vet`, `golangci-lint run` on `pkg/shared/k8s/...` | Clean, 0 issues |

---

## 2. References

- [PR #1896](https://github.com/jordigilh/kubernaut/pull/1896) — original `release/v1.5` implementation (design authority)
- [DD-K8S-001](../../architecture/decisions/DD-K8S-001-restmapper-cache-invalidation-on-lookup-failure.md) — root cause, alternatives, decision
- Issue #1888, Issue #1890 (`main`/v1.6 tracking clone)

---

## 3. Test Items

| File | Change |
|------|--------|
| `pkg/shared/k8s/gvk.go` | `ResolveGVKForKind` fallback retries once via `Reset()` on `KindsFor` failure/empty result, when the mapper implements `meta.ResettableRESTMapper` |
| `pkg/shared/k8s/gvk_test.go` | **Extended**. Added `resettableMockMapper`, `alwaysFailingResettableMapper`, and 4 new test cases |

---

## 4. BR Coverage Matrix

| Reference | Description | Priority | Tier | Test ID | Status |
|-----------|--------------|----------|------|---------|--------|
| #1888 | Self-heal REST-mapper cache on lookup failure | P1 | Unit | UT-K8S-1888-001 | Pass |
| #1888 | Retry is bounded to exactly one extra attempt | P2 | Unit | UT-K8S-1888-002 | Pass |
| #1888 | Error still propagates when retry also fails | P2 | Unit | UT-K8S-1888-003 | Pass |
| #1888 | Non-resettable mappers are unaffected (no panic/no retry attempted) | P2 | Unit | UT-K8S-1888-004 | Pass |

---

## 5. Execution Evidence

```bash
go build ./pkg/shared/k8s/...                          # clean
go vet ./pkg/shared/k8s/...                             # clean
golangci-lint run --timeout=5m ./pkg/shared/k8s/...     # 0 issues
make test-unit-shared-packages                          # 37 suites passed, 0 failed
go tool cover -func=coverage_unit_shared-packages.out | grep gvk.go
#   pkg/shared/k8s/gvk.go:69   ResolveGVKForKind          100.0%
#   pkg/shared/k8s/gvk.go:104  ResolveGVKWithAPIVersion   100.0%
```

---

## 6. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-08-03 | Initial port of PR #1896 to `main`. `gvk.go`/`gvk_test.go` were byte-identical to `release/v1.5`'s pre-fix state, so the fix and tests were applied unchanged. All tests pass; build/vet/lint clean. |
