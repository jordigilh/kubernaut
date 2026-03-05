# Test Plan: Issue #251 — Async Hash Deferral for GitOps/Operator Targets

**Service**: EM (Effectiveness Monitor) + RO (Remediation Orchestrator)
**Service Type**: CRD Controller
**Date**: 2026-03-02
**Business Requirements**: BR-EM-010, BR-RO-103
**Design Document**: DD-EM-004
**Policy**: DD-TEST-006

---

> **#277 Update**: Issue #277 migrated the EA CRD from an absolute `Spec.HashComputeAfter`
> (`*metav1.Time`) to a relative `Spec.Config.HashComputeDelay` (`*metav1.Duration`).
> The EM computes the deferral deadline as `creation + HashComputeDelay`.
> Where this test plan says "deferral deadline" it refers to the deadline the EM derives
> from `Config.HashComputeDelay`. See [DD-EM-004 v3.0](../../architecture/decisions/DD-EM-004-async-hash-deferral.md).

## Overview

This test plan covers the deferred hash computation feature for async-managed targets (GitOps and operator-managed CRDs). Tests span two services:

- **RO**: Async target detection (`isBuiltInGroup`, GitOps labels) and `Config.HashComputeDelay` population in EA spec
- **EM**: Hash computation gating on `Config.HashComputeDelay` duration

---

## Unit Tests (UT)

### RO — `isBuiltInGroup` utility

| Test ID | Scenario | Input | Expected | BR |
|---------|----------|-------|----------|-----|
| UT-RO-251-001 | Built-in core group | `""` | `true` | BR-RO-103.1 |
| UT-RO-251-002 | Built-in apps group | `"apps"` | `true` | BR-RO-103.1 |
| UT-RO-251-003 | Built-in networking group | `"networking.k8s.io"` | `true` | BR-RO-103.1 |
| UT-RO-251-004 | CRD group: cert-manager | `"cert-manager.io"` | `false` | BR-RO-103.1 |
| UT-RO-251-005 | CRD group: postgres operator | `"acid.zalan.do"` | `false` | BR-RO-103.1 |
| UT-RO-251-006 | CRD group: ArgoCD | `"argoproj.io"` | `false` | BR-RO-103.1 |
| UT-RO-251-007 | All built-in groups covered | Each built-in group | All `true` | BR-RO-103.1 |

**File**: `test/unit/remediationorchestrator/builtin_group_test.go`

### RO — EA creation with `hashComputeDelay`

UT-RO-251-008 through UT-RO-251-013 cover the RO reconciler's async detection logic (GitOps label reading, GVK resolution, HashComputeDelay computation). These scenarios are I/O-dependent (K8s API calls, REST mapper) and are covered at the IT level (IT-RO-251-001, IT-RO-251-002) which provides stronger validation with a real envtest reconciler. The pure-logic components (`IsBuiltInGroup`, `CheckHashDeferral`) have full UT coverage above.

| Test ID | Scenario | Coverage |
|---------|----------|----------|
| UT-RO-251-008..013 | RO reconciler async detection orchestration | Covered by IT-RO-251-001 (GitOps path) and IT-RO-251-002 (sync path) |

### EM — Hash computation gating

| Test ID | Scenario | Input | Expected | BR |
|---------|----------|-------|----------|-----|
| UT-EM-251-001 | HashComputeDelay in future: defer | `HashComputeDelay` = 5m (deadline in future) | Requeue with `RequeueAfter = ~5m`, hash NOT computed | BR-EM-010.1 |
| UT-EM-251-002 | HashComputeDelay elapsed: proceed | `HashComputeDelay` = 1m (deadline in past) | Hash computed immediately | BR-EM-010.1 |
| UT-EM-251-003 | HashComputeDelay nil: proceed (backward compat) | `HashComputeDelay` = nil | Hash computed immediately | BR-EM-010.1 |
| UT-EM-251-004 | HashComputeDelay zero: proceed (backward compat) | `HashComputeDelay` = 0 | Hash computed immediately | BR-EM-010.1 |
| UT-EM-251-005 | Short deferral: proportional requeue | `HashComputeDelay` = 30s (deadline in future) | `ShouldDefer=true`, `RequeueAfter ~30s` (proportional) | BR-EM-010.1 |

**Note**: The `HashComputed=true` guard (preventing re-deferral after hash is computed) is enforced by the EM reconciler's `!ea.Status.Components.HashComputed` condition (line 434), not by `CheckHashDeferral`. This is validated at the IT level (IT-EM-251-001).

**File**: `test/unit/effectivenessmonitor/hash_deferral_test.go`

---

## Integration Tests (IT)

### EM — Hash deferral gating (envtest with real EM reconciler)

| Test ID | Scenario | Setup | Validation | BR |
|---------|----------|-------|-----------|-----|
| IT-EM-251-001 | Async target: EM defers then computes | Create EA with `Config.HashComputeDelay = 8s` (deferral deadline in future) | `Consistently` verifies hash NOT computed during window; `Eventually` verifies hash computed + full assessment after window elapses | BR-EM-010.1 |
| IT-EM-251-002 | Sync target: EM computes immediately (backward compat) | Create EA without `HashComputeDelay` (nil) | EA completes with hash computed on first reconcile; `HashComputeDelay` remains nil | BR-EM-010.1 |
| IT-EM-251-003 | Elapsed deferral: EM computes immediately | Create EA with `Config.HashComputeDelay` such that deadline is 5 minutes in the past | EA completes with hash computed immediately (past deferral treated as no-op) | BR-EM-010.1 |

**File**: `test/integration/effectivenessmonitor/hash_deferral_integration_test.go`

### RO — Async target detection (envtest with real RO reconciler)

| Test ID | Scenario | Setup | Validation | BR |
|---------|----------|-------|-----------|-----|
| IT-RO-251-001 | GitOps target: HashComputeDelay set in EA | Full pipeline (RR→SP→AA→WE) with `DetectedLabels.GitOpsManaged=true` in AIAnalysis status | EA created with non-nil `Config.HashComputeDelay`; reasonable duration from RO config | BR-RO-103.2, BR-RO-103.3 |
| IT-RO-251-002 | Sync target: HashComputeDelay nil (backward compat) | Full pipeline (RR→SP→AA→WE) without GitOps labels, built-in Deployment target | EA created with nil `Config.HashComputeDelay` | BR-RO-103.3 |

**File**: `test/integration/remediationorchestrator/ea_async_detection_integration_test.go`

---

## E2E Tests

| Test ID | Scenario | Setup | Validation | BR | Status |
|---------|----------|-------|-----------|-----|--------|
| E2E-FP-251-001 | cert-manager CRD: full pipeline async hash deferral | Install cert-manager in BeforeAll (self-contained); inject CertManagerCertNotReady alert; Mock LLM returns `rca_resource_kind: Certificate` | RO resolves `Certificate` via REST mapper → `cert-manager.io/v1` (non-built-in) → sets `Config.HashComputeDelay`; EM defers hash computation; audit `assessment.scheduled` includes `hash_compute_after` (derived timestamp); EA reaches terminal phase | BR-EM-010, BR-RO-103 | **Implemented** |

**File**: `test/e2e/fullpipeline/02_async_hash_deferral_test.go`

**Design decisions**:
- Runs in the Full Pipeline (FP) E2E suite — same Kind cluster as `01_full_remediation_lifecycle_test.go`
- cert-manager installed in `BeforeAll` (self-contained, ~2 min impact only on this test)
- Reuses `oomkill-increase-memory-v1` workflow for pipeline flow (the async detection depends only on `AffectedResource.Kind`, not the actual workflow)
- Mock LLM `cert_not_ready` scenario returns `rca_resource_kind: "Certificate"` with `rca_resource_api_version: "cert-manager.io/v1"`
- Test fixtures are isolated: own namespace, own cleanup, no impact on other FP tests

---

## Test Execution Summary

| Test Category | Test Count | File | Status |
|---------------|-----------|------|--------|
| UT — RO isBuiltInGroup | 7 (20 sub-cases) | `test/unit/remediationorchestrator/builtin_group_test.go` | Implemented |
| UT — RO EA creation | — | Covered by IT-RO-251-001, IT-RO-251-002 | See IT |
| UT — EM hash gating | 5 | `test/unit/effectivenessmonitor/hash_deferral_test.go` | Implemented |
| IT — EM hash deferral | 3 | `test/integration/effectivenessmonitor/hash_deferral_integration_test.go` | Implemented |
| IT — RO async detection | 2 | `test/integration/remediationorchestrator/ea_async_detection_integration_test.go` | Implemented |
| E2E — cert-manager async | 1 | `test/e2e/fullpipeline/02_async_hash_deferral_test.go` | Implemented |
| **Total implemented** | **18** (31 sub-cases) | | |

---

## Dependencies

| Dependency | Status | Impact |
|-----------|--------|--------|
| EA CRD config field (`Config.HashComputeDelay`) | Required | `make generate manifests` + Helm chart sync |
| `resolveGVKForKind` accessible from EA creator | Required | Extract to shared utility or pass resolved group |
| AA.Status.PostRCAContext.DetectedLabels available | Existing | RO reads from already-fetched AA object |
| cert-manager deployed in E2E cluster | Required for E2E | Kind cluster with cert-manager installed |

---

## References

- [BR-EM-010](../../requirements/BR-EM-010-async-hash-deferral.md) — EM hash deferral requirement
- [BR-RO-103](../../requirements/BR-RO-103-async-target-detection.md) — RO async target detection
- [DD-EM-004](../../architecture/decisions/DD-EM-004-async-hash-deferral.md) — Design document
- [DD-EM-002](../../architecture/decisions/DD-EM-002-canonical-spec-hash.md) — Canonical spec hash
- [#251](https://github.com/jordigilh/kubernaut/issues/251) — Implementation issue
- [#133](https://github.com/jordigilh/kubernaut/issues/133) — Demo: cert-manager Certificate failure
