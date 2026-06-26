# Test Plan: Fix #1371 extractNamespace Defaults to "default" for Cluster-Scoped Resources

**Issue**: [#1371](https://github.com/jordigilh/kubernaut/issues/1371)
**Service Type**: [x] CRD Controller (Gateway)
**Date**: 2026-06-26
**Status**: Active

---

## Business Requirements

| BR ID | Description |
|---|---|
| BR-GATEWAY-001 | Signal ingestion and resource identification |
| BR-GATEWAY-004 | Signal fingerprinting (owner-chain-based deduplication) |

## FedRAMP Control Objectives

| Control | Objective | How This Fix Maps |
|---------|-----------|-------------------|
| SI-10 (Information Input Validation) | Namespace must reflect actual K8s resource scope | Empty string for cluster-scoped, "default" fallback only for namespaced |
| AU-3 (Content of Audit Records) | Signals carry correct namespace for audit trail | RR CRD created with accurate namespace; downstream consumers already handle `""` |

## Fingerprint Impact

Changing namespace from `"default"` to `""` for cluster-scoped resources alters `CalculateOwnerFingerprint` output (`sha256(namespace:kind:name)`). This is a **one-time transition** — new fingerprints are correct, old ones age out. Accepted per issue #1371.

---

## Test Scenario Naming Convention

**Format**: `{TIER}-{SERVICE}-1371-{SEQUENCE}`

- `GW` = Gateway

---

## Component 1: APIResourceRegistry.IsNamespacedKind (New Method)

### Unit Tests — Scope Detection (SI-10)

| ID | Scenario | Expected Outcome |
|---|---|---|
| UT-GW-1371-001 | `IsNamespacedKind("Deployment")` — namespaced workload | Returns `true` — namespace required for Deployments |
| UT-GW-1371-002 | `IsNamespacedKind("Node")` — cluster-scoped infrastructure | Returns `false` — Nodes have no namespace |
| UT-GW-1371-003 | `IsNamespacedKind("Namespace")` — cluster-scoped meta | Returns `false` — Namespace resource is itself cluster-scoped |
| UT-GW-1371-004 | `IsNamespacedKind("PersistentVolume")` — cluster-scoped storage | Returns `false` — PVs are cluster-scoped (PVCs are namespaced) |
| UT-GW-1371-005 | `IsNamespacedKind("UnknownKind")` — unknown kind | Returns `true` — conservative default avoids dropping namespace |
| UT-GW-1371-006 | `IsNamespacedKind` with nil snapshot (pre-initialization) | Returns `true` — graceful degradation before registry ready |

---

## Component 2: extractNamespace (Signature Change)

### Unit Tests — Label Extraction (SI-10)

| ID | Scenario | Expected Outcome |
|---|---|---|
| UT-GW-1371-007 | No namespace labels present | Returns `("", false)` — caller decides based on kind scope |
| UT-GW-1371-008 | `namespace` label present | Returns `("production", true)` — explicit namespace honored |
| UT-GW-1371-009 | `exported_namespace` label present | Returns `("staging", true)` — federation namespace takes precedence (#1029) |
| UT-GW-1371-010 | Both `namespace` and `exported_namespace` present | Returns `exported_namespace` value — precedence rule: exported > namespace |

---

## Component 3: Alert Parsing Pipeline (Scope-Aware Namespace)

### Unit Tests — End-to-End Namespace Resolution (AU-3)

| ID | Scenario | Expected Outcome |
|---|---|---|
| UT-GW-1371-011 | Cluster-scoped Node alert (no namespace label) | `signal.Resource.Namespace == ""` — RR created with empty namespace |
| UT-GW-1371-012 | Namespaced Deployment alert (no namespace label) | `signal.Resource.Namespace == "default"` — "default" fallback only for namespaced kinds |
| UT-GW-1371-013 | Namespaced Deployment alert (explicit namespace) | `signal.Resource.Namespace == "production"` — explicit namespace always honored |
| UT-GW-1371-014 | Cluster-scoped Node alert WITH explicit namespace label | `signal.Resource.Namespace == "monitoring"` — if user provides namespace, honor it |

### Integration Tests (Wiring)

| ID | Scenario | Expected Outcome |
|---|---|---|
| IT-GW-1371-015 | Full `adapter.Parse()` of KubeNodeNotReady alert | Signal produced with `Namespace == ""` through complete production parse path |

---

## Existing Tests to Update

| ID | File | Required Change |
|---|---|---|
| (unnamed) | resource_extraction_test.go | Cluster-scoped test: change `Equal("default")` to `BeEmpty()` |
| (unnamed) | resource_extraction_business_test.go | Node test (line 125): change `Equal("default")` to `BeEmpty()` |
| (unnamed) | resource_extraction_business_test.go | NodeDiskPressure test: change `Equal("default")` to `BeEmpty()` |

---

## Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|---|---|---|---|
| IsNamespacedKind | extractNamespace call sites in ConvertToSignal/Parse | pkg/gateway/adapters/prometheus_adapter.go | IT-GW-1371-015 |
| kindNamespaced in snapshot | buildSnapshot | pkg/gateway/adapters/resource_registry.go | UT-GW-1371-001 |

---

## Test Execution Summary

| Test Category | Tests | Status |
|---|---|---|
| GW Unit Tests — Scope Detection (UT-GW-1371-001..006) | 6 | Pending |
| GW Unit Tests — Label Extraction (UT-GW-1371-007..010) | 4 | Pending |
| GW Unit Tests — Pipeline (UT-GW-1371-011..014) | 4 | Pending |
| GW Integration Tests (IT-GW-1371-015) | 1 | Pending |
| Existing Test Updates | 3 | Pending |
| **Total** | **18** | **Pending** |
