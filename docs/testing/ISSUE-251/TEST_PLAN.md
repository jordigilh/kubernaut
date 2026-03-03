# Test Plan: Issue #251 — Async Hash Deferral for GitOps/Operator Targets

**Service**: EM (Effectiveness Monitor) + RO (Remediation Orchestrator)
**Service Type**: CRD Controller
**Date**: 2026-03-02
**Business Requirements**: BR-EM-010, BR-RO-103
**Design Document**: DD-EM-004
**Policy**: DD-TEST-006

---

## Overview

This test plan covers the deferred hash computation feature for async-managed targets (GitOps and operator-managed CRDs). Tests span two services:

- **RO**: Async target detection (`isBuiltInGroup`, GitOps labels) and `hashComputeAfter` population in EA spec
- **EM**: Hash computation gating on `hashComputeAfter` timestamp

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

### RO — EA creation with `hashComputeAfter`

UT-RO-251-008 through UT-RO-251-013 cover the RO reconciler's async detection logic (GitOps label reading, GVK resolution, hashComputeAfter computation). These scenarios are I/O-dependent (K8s API calls, REST mapper) and are covered at the IT level (IT-RO-251-001, IT-RO-251-002) which provides stronger validation with a real envtest reconciler. The pure-logic components (`IsBuiltInGroup`, `CheckHashDeferral`) have full UT coverage above.

| Test ID | Scenario | Coverage |
|---------|----------|----------|
| UT-RO-251-008..013 | RO reconciler async detection orchestration | Covered by IT-RO-251-001 (GitOps path) and IT-RO-251-002 (sync path) |

### EM — Hash computation gating

| Test ID | Scenario | Input | Expected | BR |
|---------|----------|-------|----------|-----|
| UT-EM-251-001 | hashComputeAfter in future: defer | `HashComputeAfter` = now + 5m | Requeue with `RequeueAfter = ~5m`, hash NOT computed | BR-EM-010.1 |
| UT-EM-251-002 | hashComputeAfter in past: proceed | `HashComputeAfter` = now - 1m | Hash computed immediately | BR-EM-010.1 |
| UT-EM-251-003 | hashComputeAfter nil: proceed (backward compat) | `HashComputeAfter` = nil | Hash computed immediately | BR-EM-010.1 |
| UT-EM-251-004 | hashComputeAfter zero: proceed (backward compat) | `HashComputeAfter` = zero time | Hash computed immediately | BR-EM-010.1 |
| UT-EM-251-005 | Short deferral: proportional requeue | `HashComputeAfter` = now + 30s | `ShouldDefer=true`, `RequeueAfter ~30s` (proportional) | BR-EM-010.1 |

**Note**: The `HashComputed=true` guard (preventing re-deferral after hash is computed) is enforced by the EM reconciler's `!ea.Status.Components.HashComputed` condition (line 434), not by `CheckHashDeferral`. This is validated at the IT level (IT-EM-251-001).

**File**: `test/unit/effectivenessmonitor/hash_deferral_test.go`

---

## Integration Tests (IT)

### EM — Hash deferral gating (envtest with real EM reconciler)

| Test ID | Scenario | Setup | Validation | BR |
|---------|----------|-------|-----------|-----|
| IT-EM-251-001 | Async target: EM defers then computes | Create EA with `HashComputeAfter` 8s in the future | `Consistently` verifies hash NOT computed during window; `Eventually` verifies hash computed + full assessment after window elapses | BR-EM-010.1 |
| IT-EM-251-002 | Sync target: EM computes immediately (backward compat) | Create EA without `HashComputeAfter` (nil) | EA completes with hash computed on first reconcile; `HashComputeAfter` remains nil | BR-EM-010.1 |
| IT-EM-251-003 | Elapsed deferral: EM computes immediately | Create EA with `HashComputeAfter` 5 minutes in the past | EA completes with hash computed immediately (past deferral treated as no-op) | BR-EM-010.1 |

**File**: `test/integration/effectivenessmonitor/hash_deferral_integration_test.go`

### RO — Async target detection (envtest with real RO reconciler)

| Test ID | Scenario | Setup | Validation | BR |
|---------|----------|-------|-----------|-----|
| IT-RO-251-001 | GitOps target: HashComputeAfter set in EA | Full pipeline (RR→SP→AA→WE) with `DetectedLabels.GitOpsManaged=true` in AIAnalysis status | EA created with non-nil `HashComputeAfter`; reasonable timestamp within stabilization window | BR-RO-103.2, BR-RO-103.3 |
| IT-RO-251-002 | Sync target: HashComputeAfter nil (backward compat) | Full pipeline (RR→SP→AA→WE) without GitOps labels, built-in Deployment target | EA created with nil `HashComputeAfter` | BR-RO-103.3 |

**File**: `test/integration/remediationorchestrator/ea_async_detection_integration_test.go`

---

## E2E Tests

| Test ID | Scenario | Setup | Validation | BR | Status |
|---------|----------|-------|-----------|-----|--------|
| E2E-EM-251-001 | cert-manager operator: full pipeline async hash | Deploy cert-manager; trigger CertManagerCertNotReady alert; full pipeline through RCA, WFE, EA | RO sets `hashComputeAfter` for `cert-manager.io` CRD; EM defers hash; post-hash captures CR spec change after cert-manager reconciles; health confirms Certificate Ready; effectiveness score reflects both hash change and health | BR-EM-010, BR-RO-103 | **Deferred** — implement alongside demo scenarios (#144, #101) |

**File**: `test/e2e/effectivenessmonitor/async_hash_e2e_test.go` (to be created)

**Note**: E2E-EM-251-001 extends the validation from demo scenario #133 (cert-manager Certificate failure) with explicit hash timing assertions. Deferred because it depends on cert-manager deployment in the Kind cluster, which is part of demo infrastructure work.

---

## Test Execution Summary

| Test Category | Test Count | File | Status |
|---------------|-----------|------|--------|
| UT — RO isBuiltInGroup | 7 (20 sub-cases) | `test/unit/remediationorchestrator/builtin_group_test.go` | Implemented |
| UT — RO EA creation | — | Covered by IT-RO-251-001, IT-RO-251-002 | See IT |
| UT — EM hash gating | 5 | `test/unit/effectivenessmonitor/hash_deferral_test.go` | Implemented |
| IT — EM hash deferral | 3 | `test/integration/effectivenessmonitor/hash_deferral_integration_test.go` | Implemented |
| IT — RO async detection | 2 | `test/integration/remediationorchestrator/ea_async_detection_integration_test.go` | Implemented |
| E2E — cert-manager | 1 | `test/e2e/effectivenessmonitor/async_hash_e2e_test.go` | Deferred |
| **Total implemented** | **17** (30 sub-cases) | | |

---

## Dependencies

| Dependency | Status | Impact |
|-----------|--------|--------|
| EA CRD spec change (`hashComputeAfter`) | Required | `make generate manifests` + Helm chart sync |
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
