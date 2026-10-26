# Test Plan: Local Gateway Cluster Attribution Boundary

**Test Plan Identifier**: TP-2555-v1
**Feature**: Local-mode Gateway ignores alert cluster attribution while Fleet mode preserves registered remote routing
**Version**: 1.0
**Created**: 2026-10-26
**Author**: Kubernaut development agent
**Status**: Active
**Branch**: `fix/local-gateway-cluster-id`

## 1. Introduction

### 1.1 Purpose

Issue #2555 fixes a trust-boundary defect where a local Gateway treated an
Alertmanager `cluster` label as a Fleet identity and attempted remote owner
resolution. This plan proves the local and Fleet contracts across unit,
integration, and deployed Gateway E2E paths.

### 1.2 Objectives

1. Local `Parse` and `ParseBatch` ignore common and per-alert cluster labels,
   use the local owner resolver, and produce an empty effective `ClusterID`.
2. Fleet mode continues to require registered IDs, preserve them, and fail
   closed for missing or unregistered identities.
3. The deployed local Gateway accepts cluster-labeled alerts and persists no
   cluster attribution in `RemediationRequest.Spec.ClusterID`.

### 1.3 Success Metrics

| Metric | Target | Evidence |
|---|---:|---|
| Affected unit scenarios | 100% | `go test ./pkg/gateway/adapters -count=1` |
| Affected integration scenarios | 100% when envtest prerequisites exist | `IT-GW-2555-001/002` |
| Affected E2E scenarios | 100% | `E2E-GW-2555-001/002` |
| Fleet regression scenarios | 100% | Existing Fleet dispatch and fail-closed tests |
| Unrelated source changes | 0 | Scoped Git diff review |

## 2. References

### 2.1 Authority

- Issue #2555: Local-mode Gateway incorrectly resolves alert cluster IDs remotely
- `BR-INTEGRATION-065`: Fleet boundary and remote owner resolution
- ADR-068: Fleet federation architecture
- `pkg/gateway/adapters/prometheus_adapter.go`

### 2.2 Control objectives

- FedRAMP AC-4, AC-6, SI-10, AU-3
- SOC 2 CC7.2
- OWASP ASVS V4.1.1 and V4.1.5

## 3. Risks and Mitigations

| ID | Risk | Impact | Mitigation |
|---|---|---|---|
| R1 | Local labels still trigger remote resolution | Local alerts are rejected | UT, IT, and E2E common/per-alert cases |
| R2 | Local raw ID leaks into fingerprint or CRD | Cross-cluster deduplication and routing are corrupted | Assert equal local fingerprints and empty effective/CRD IDs |
| R3 | Fleet fallback to local resolver is restored | Remote signals can target the wrong cluster | Existing registered, missing, unregistered, and reader-error Fleet tests |
| R4 | Test infrastructure hides product behavior | False confidence | Record envtest and Kind setup failures separately from assertions |

## 4. Scope

### 4.1 Features tested

- `PrometheusAdapter.Parse` and `ParseBatch` mode-aware resolver and cluster
  identity handling.
- Gateway production signal path through the deployed local webhook.
- Fleet fail-closed behavior for missing and unregistered IDs.

### 4.2 Features not tested

- Fleet E2E and FullPipeline lanes: explicitly out of scope for this issue.
- Remote infrastructure changes: no MCP Gateway or registration changes are
  required.
- Alert sources other than Prometheus/Alertmanager.

### 4.3 Compatibility impact

- Existing local alerts without cluster attribution are unchanged.
- Local alerts that include `cluster` or `cluster_id` are now accepted when
  their local resources are otherwise valid; those labels no longer select an
  owner resolver, change the fingerprint, or populate
  `RemediationRequest.Spec.ClusterID`.
- Fleet behavior is unchanged: remote attribution remains required, must be
  registered, and does not fall back to local resolution.

## 5. Approach

### 5.1 TDD sequence

1. **RED**: Add UT and direct E2E business scenarios; UTs failed with
   `remote owner resolver unavailable for cluster "local"` before the fix.
2. **GREEN**: Local mode returns the local resolver for every alert label and
   normalizes the effective cluster ID to empty; Fleet mode remains strict.
3. **REFACTOR**: Clarify cluster provenance comments and keep the mode boundary
   explicit in the adapter.
4. **Wiring verification**: Existing Gateway production registration remains the
   caller; integration and deployed E2E paths prove dispatch.

### 5.2 Business outcome bar

Tests assert accepted signals, owner-resolution outcome, fingerprint identity,
and persisted cluster provenance. They do not assert private helper calls only.

## 6. Test Items

| Tier | Item |
|---|---|
| Unit | `pkg/gateway/adapters/prometheus_adapter.go` and `owner_resolver_fleet_test.go` |
| Integration | `test/integration/gateway/adapters_integration_test.go` with envtest |
| E2E | `test/e2e/gateway/31_prometheus_adapter_test.go` with a real local Gateway and Kind |

## 7. BR Coverage Matrix

| Requirement/control | Scenario | Tier | Status |
|---|---|---|---|
| #2555, AC-4, AC-6, ASVS V4.1.1 | Common label ignored locally | UT/IT/E2E-2555-001 | Pass / IT environment-blocked / Pass |
| #2555, AC-4, AC-6, ASVS V4.1.1 | Per-alert label ignored locally | UT/IT/E2E-2555-002 | Pass / IT environment-blocked / Pass |
| BR-INTEGRATION-065, AC-4, AC-6, SI-10, ASVS V4.1.5 | Registered Fleet ID remains remote | Existing UT | Pass |
| #2555, SI-10, ASVS V4.1.5 | Missing/unregistered Fleet ID fails closed | Existing UT | Pass |
| AU-3, CC7.2 | Local CRD has no invented cluster provenance | E2E-2555-001/002 | Pass |

## 8. Test Scenarios

| ID | Business outcome | Phase/status |
|---|---|---|
| `UT-GW-2555-001` | Local common `cluster` label succeeds, uses local ownership, keeps empty ID and stable local fingerprint | Pass |
| `UT-GW-2555-002` | Local per-alert `cluster` label has the same behavior | Pass |
| `IT-GW-2555-001` | Integration adapter path proves common-label behavior | Blocked: missing `/usr/local/kubebuilder/bin/etcd` |
| `IT-GW-2555-002` | Integration adapter path proves per-alert behavior | Blocked: missing `/usr/local/kubebuilder/bin/etcd` |
| `E2E-GW-2555-001` | Deployed local Gateway persists empty `Spec.ClusterID` for common attribution | Pass |
| `E2E-GW-2555-002` | Deployed local Gateway persists empty `Spec.ClusterID` for per-alert attribution | Pass |

## 9. Test Cases

### UT-GW-2555-001 and UT-GW-2555-002

- **Given**: A Prometheus adapter without a Fleet reader factory.
- **When**: A common or per-alert cluster label is present.
- **Then**: Parsing succeeds through the local owner resolver, the effective
  `ClusterID` is empty, and the fingerprint matches an unattributed local alert.

### E2E-GW-2555-001 and E2E-GW-2555-002

- **Given**: A real local Gateway deployed by `test/e2e/gateway`.
- **When**: An authorized webhook contains common or per-alert `cluster=local`.
- **Then**: The Gateway creates a RemediationRequest and its
  `Spec.ClusterID` remains empty.

## 10. Environmental Needs

- Unit: Go, Ginkgo/Gomega; no external mocks beyond existing Fleet reader test doubles.
- Integration: envtest binaries, including `etcd`, plus the existing Gateway
  DataStorage harness.
- E2E: Podman, Kind, local image builds, and an isolated `gateway-e2e` cluster.
- E2E command: `E2E_COVERAGE=false go test ./test/e2e/gateway -ginkgo.focus='E2E-GW-2555'`.

## 11. Execution Order

1. RED unit tests.
2. GREEN production fix and focused unit/package tests.
3. Integration tests when envtest prerequisites are available.
4. Direct local Gateway E2E only.
5. Build, compile-only, lint, and scoped diff checks.

Fleet and FullPipeline commands are not part of this plan.

## 12. Existing Tests Requiring Updates

`UT-GW-2430-001` previously interpreted a nil reader factory plus a non-empty
label as a remote signal. Under #2555, nil reader factory means local mode, so
the test now represents Fleet mode with an empty reader registry and verifies
unregistered IDs fail closed.

## 13. Wiring Verification

| Code path | Production entry | Observable outcome | Evidence |
|---|---|---|---|
| Prometheus adapter local dispatch | Gateway Prometheus webhook registration | Local RemediationRequest with empty `Spec.ClusterID` | E2E-GW-2555-001/002 |
| Fleet dispatch boundary | `cmd/gateway` Fleet reader wiring | Remote ID preserved or invalid ID rejected | Existing UT-GW-P1/2430 tests |

## 14. Changelog

| Version | Date | Change |
|---|---|---|
| 1.0 | 2026-10-26 | Initial issue #2555 test plan and execution evidence |
