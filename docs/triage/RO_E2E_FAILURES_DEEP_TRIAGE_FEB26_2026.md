# RemediationOrchestrator E2E Failures — Deep Triage

**Date**: 2026-02-26  
**Scope**: 19 of 39 RO E2E tests failing  
**Authoritative**: ADR-057 (CRD Namespace Consolidation), DD-PERF-001 (Atomic Status Updates)

---

## Executive Summary

| Category | Count | Root Cause | Fix Complexity |
|----------|-------|------------|----------------|
| **A** | 2 | Conflict retry uses cached Get → stale resourceVersion | Medium |
| **B** | ~10 | RR created in test namespace; RO only watches kubernaut-system | Low |
| **C** | 3 | Same as B (scope_blocking creates RR in test namespaces) | Low |
| **D** | 2 | Same as B (downstream CRDs never created) | Low |
| **E** | 1 | Likely contention/timing or same as A | Medium |

**Primary root cause**: ADR-057 namespace mismatch — tests create RRs in test namespaces but RO only watches `kubernaut-system`.

**Secondary root cause**: Test helpers use `k8sClient.Get()` inside `RetryOnConflict`; cached reads can return stale resourceVersion under parallel load.

---

## 1. Root Cause Analysis

### Root Cause 1: ADR-057 Namespace Mismatch (Categories B, C, D)

**Evidence**:
- `cmd/remediationorchestrator/main.go` lines 109–148: RO cache restricts all CRD watches to `controllerNS` (kubernaut-system)
- ADR-057: "A CRD created in any other namespace is invisible to the controllers"
- `pkg/remediationorchestrator/creator/signalprocessing.go` line 86: SP created in `rr.Namespace`

**Tests that FAIL** (create RR in test namespace):

| File | RR Namespace | Lookup Namespace | RO Sees RR? |
|------|--------------|------------------|-------------|
| `ea_creation_e2e_test.go` | testNS | testNS | No |
| `completion_notification_e2e_test.go` | testNS | testNS | No |
| `needs_human_review_e2e_test.go` | testNS | testNS | No |
| `proactive_signal_mode_e2e_test.go` | testNS | testNS | No |
| `scope_blocking_e2e_test.go` | unmanagedNS / managedNS | managedNS / ns | No |

**Tests that PASS** (create RR in controllerNamespace):

| File | RR Namespace | Lookup Namespace | RO Sees RR? |
|------|--------------|------------------|-------------|
| `lifecycle_e2e_test.go` | controllerNamespace | controllerNamespace | Yes |
| `audit_wiring_e2e_test.go` | controllerNamespace | controllerNamespace | Yes |
| `approval_e2e_test.go` | controllerNamespace | controllerNamespace | Yes |
| `gap8_webhook_test.go` | controllerNamespace | controllerNamespace | Yes |

**Conclusion**: RRs must be created in `controllerNamespace` (kubernaut-system). `Spec.TargetResource.Namespace` can remain the workload/test namespace.

---

### Root Cause 2: Conflict Retry Uses Cached Get (Category A)

**Evidence**:
- Error: `Operation cannot be fulfilled on aianalyses.kubernaut.ai "ai-rr-approval-e2e": the object has been modified; please apply your changes to the latest version and try again`
- `test/shared/helpers/crd_lifecycle.go` lines 142, 178: `SimulateAICompletedWithWorkflow` and `SimulateAIWorkflowNotNeeded` use `RetryOnConflict` with `k8sClient.Get()`
- `suite_test.go` line 210: "Create direct API reader for Eventually() blocks to bypass client cache"
- `k8sretry.DefaultRetry` has 5 steps (per `test/unit/signalprocessing/controller_error_handling_test.go` line 119)

**Mechanism**:
1. Test calls `SimulateAICompletedWithWorkflow` with AI object from `WaitForAICreation`
2. RO controller reconciles and updates AIAnalysis status
3. Test's `RetryOnConflict` runs: `k8sClient.Get()` → may read from cache (stale resourceVersion)
4. Test applies status changes and `Status().Update()` → conflict (server has newer version)
5. Retry: `k8sClient.Get()` again → cache may still be stale → repeated conflicts
6. After 5 attempts, retry exhausts and test fails

**Conclusion**: Use `apiReader` (uncached) for the `Get` inside `RetryOnConflict` to always fetch the latest resourceVersion from the API server.

---

### Root Cause 3: Category E (Phase Stuck at Blocked)

**Context**: E2E-RO-AUD006-001 rejection test — RR stuck at "Blocked", never reaches "AwaitingApproval".

**Possible causes**:
1. **Contention**: Same BeforeEach as approval test; under 4 parallel processes, RO may be slow to reach AwaitingApproval
2. **Scope blocking**: Unlikely — approval test uses `CreateTestNamespaceAndWait` with `WithLabels` (default includes `kubernaut.ai/managed=true`)
3. **Conflict cascade**: If `SimulateAICompletedWithWorkflow` fails with conflict, RAR is never created, so RR never reaches AwaitingApproval

**Conclusion**: Fixing Root Cause 2 (apiReader for conflict retry) may resolve this. If not, increase `WaitForRRPhase` timeout or investigate RO phase transition logic.

---

## 2. Fix Specification

### Fix 1: Align All Tests with ADR-057 (Categories B, C, D)

**Rule**: RR `ObjectMeta.Namespace` MUST be `controllerNamespace`. `Spec.TargetResource.Namespace` stays as workload namespace.

#### 2.1 `ea_creation_e2e_test.go`

```diff
- Namespace: testNS,
+ Namespace: controllerNamespace,
...
  TargetResource: remediationv1.ResourceIdentifier{
      ...
-     Namespace: testNS,
+     Namespace: testNS,  // workload namespace unchanged
  },
```

And all `client.InNamespace(testNS)` for SP/AA/WE/EA/NR lookups → `client.InNamespace(controllerNamespace)`.

**Lines to change**: 73, 87, 104, 133, 161, 170, 209, 252, 286, 300, 317, 346, 374, 383.

#### 2.2 `completion_notification_e2e_test.go`

```diff
- Namespace: testNS,
+ Namespace: controllerNamespace,
```

And all List/Get with `client.InNamespace(testNS)` → `client.InNamespace(controllerNamespace)`.

**Lines**: 70, 84, 101, 130, 163, 172, 196.

#### 2.3 `needs_human_review_e2e_test.go`

```diff
- Namespace: testNS,
+ Namespace: controllerNamespace,
```

And all List/Get namespaces → `controllerNamespace`.

**Lines**: 76, 87, 109, 138, 164, 191, 213, 224, 246, 275, 304, 318, 333.

#### 2.4 `proactive_signal_mode_e2e_test.go`

```diff
- Namespace: testNS,
+ Namespace: controllerNamespace,
```

And all List namespaces → `controllerNamespace`.

**Lines**: 68, 82, 99, 130, 156, 170, 187, 216.

#### 2.5 `scope_blocking_e2e_test.go`

**Critical**: Scope tests intentionally use unmanaged/managed **target** namespaces. Per ADR-057, the **RR CRD** must still live in `controllerNamespace`. Scope validation uses `rr.Spec.TargetResource.Namespace` (see `pkg/remediationorchestrator/routing/blocking.go` ADR-057 fix).

```diff
  rr := &remediationv1.RemediationRequest{
      ObjectMeta: metav1.ObjectMeta{
          Name:      "rr-scope-unmanaged-e2e",
-         Namespace: unmanagedNS,
+         Namespace: controllerNamespace,  // ADR-057: RR in controller NS
      },
      Spec: remediationv1.RemediationRequestSpec{
          ...
          TargetResource: remediationv1.ResourceIdentifier{
              ...
              Namespace: unmanagedNS,  // target namespace for scope check
          },
      },
  }
```

Same pattern for E2E-RO-010-002 (managedNS) and E2E-RO-010-003 (ns).

For `WaitForSPCreation` / `WaitForAICreation` in scope tests: use `controllerNamespace` (SP/AA are created in RR's namespace = controllerNamespace).

**Lines**: 74–75, 85, 137–138, 148, 186–189, 207–208, 221–222, 280–283.

---

### Fix 2: Use apiReader for Conflict-Prone Status Updates (Category A)

**File**: `test/shared/helpers/crd_lifecycle.go`

Add optional `client.Reader` parameter for cache-bypassing Get. If nil, fall back to `k8sClient`.

#### 2.6 Helper signature changes

```go
// SimulateAICompletedWithWorkflow - add optional reader for cache bypass
func SimulateAICompletedWithWorkflow(ctx context.Context, k8sClient client.Client, ai *aianalysisv1.AIAnalysis, opts AICompletionOpts) {
    SimulateAICompletedWithWorkflowWithReader(ctx, k8sClient, nil, ai, opts)
}

func SimulateAICompletedWithWorkflowWithReader(ctx context.Context, k8sClient client.Client, reader client.Reader, ai *aianalysisv1.AIAnalysis, opts AICompletionOpts) {
    getter := reader
    if getter == nil {
        getter = k8sClient
    }
    // ... use getter.Get() inside RetryOnConflict instead of k8sClient.Get()
}
```

**Simpler approach**: Add `reader client.Reader` as first optional param; callers pass `apiReader` when available.

**Recommended**: Add `reader client.Reader` parameter to all Simulate* functions. Use `reader.Get()` when `reader != nil`, else `k8sClient.Get()`.

#### 2.7 Implementation

```go
// SimulateAICompletedWithWorkflow updates AI status as completed with a workflow selection.
// Uses RetryOnConflict to handle races with the RO controller.
// When reader is non-nil (e.g. apiReader), uses it for Get to bypass cache and avoid stale resourceVersion.
func SimulateAICompletedWithWorkflow(ctx context.Context, k8sClient client.Client, ai *aianalysisv1.AIAnalysis, opts AICompletionOpts) {
    SimulateAICompletedWithWorkflowWithReader(ctx, k8sClient, nil, ai, opts)
}

func SimulateAICompletedWithWorkflowWithReader(ctx context.Context, k8sClient client.Client, reader client.Reader, ai *aianalysisv1.AIAnalysis, opts AICompletionOpts) {
    getter := reader
    if getter == nil {
        getter = k8sClient
    }
    // ... existing opts setup ...

    Expect(k8sretry.RetryOnConflict(k8sretry.DefaultRetry, func() error {
        if err := getter.Get(ctx, client.ObjectKeyFromObject(ai), ai); err != nil {
            return err
        }
        // ... rest unchanged ...
    })).To(Succeed())
}
```

Apply same pattern to:
- `SimulateAIWorkflowNotNeeded`
- `SimulateAINeedsHumanReview`
- `SimulateSPCompletion` (if conflicts observed)
- `SimulateWECompletion` (if conflicts observed)

**Call site** (e.g. `approval_e2e_test.go`, `lifecycle_e2e_test.go`):

```go
helpers.SimulateAICompletedWithWorkflowWithReader(ctx, k8sClient, apiReader, ai, helpers.AICompletionOpts{...})
```

**Note**: `apiReader` is package-level in `suite_test.go`; RO E2E tests in same package can use it directly.

---

### Fix 3: Optional — Increase Retry Steps for E2E

If conflicts persist after Fix 2, use a custom retry with more steps:

```go
e2eRetry := k8sretry.DefaultRetry
e2eRetry.Steps = 10
Expect(k8sretry.RetryOnConflict(e2eRetry, func() error { ... })).To(Succeed())
```

---

## 3. Authoritative Documentation Alignment

| Document | Alignment |
|----------|-----------|
| **ADR-057** | All CRDs in controller namespace; RO watch restricted to controller namespace. Tests must create RRs in `controllerNamespace`. |
| **ADR-053** | Scope validation uses target resource namespace (`Spec.TargetResource.Namespace`), not CRD namespace. |
| **DD-PERF-001** | Atomic status updates with `RetryOnConflict`; refetch before update. Using apiReader ensures refetch gets latest version. |
| **suite_test.go** L210 | "Create direct API reader for Eventually() blocks to bypass client cache" — same rationale for Simulate* helpers. |

---

## 4. Category Breakdown

| Category | Tests | Root Cause | Shared? |
|----------|-------|------------|---------|
| **A** | 2 (SimulateAICompletion*) | Cached Get in RetryOnConflict | Yes — same helper |
| **B** | ~10 (SP creation timeout) | RR in test NS, RO doesn't see it | Yes — ADR-057 |
| **C** | 3 (scope_blocking) | RR in test NS, RO doesn't see it | Yes — ADR-057 |
| **D** | 2 (AA/WE creation timeout) | Same as B — no SP → no AA → no WE | Yes — ADR-057 |
| **E** | 1 (rejection phase stuck) | Likely A (conflict) or timing | Possibly A |

**Summary**: 2 root causes. Fix 1 (ADR-057) addresses ~15 failures. Fix 2 (apiReader) addresses 2–3 failures.

---

## 5. Verification Checklist

After applying fixes:

1. [ ] All RRs created in `controllerNamespace`
2. [ ] All SP/AA/WE/EA/NR lookups use `controllerNamespace`
3. [ ] Scope tests: RR in `controllerNamespace`, `TargetResource.Namespace` = target for scope check
4. [ ] Simulate* helpers use apiReader for Get when available
5. [ ] Run: `ginkgo -p --procs=4 ./test/e2e/remediationorchestrator/...`
6. [ ] No conflict errors on AIAnalysis status updates
7. [ ] Scope blocking tests pass (BlockReason = UnmanagedResource when target NS unmanaged)

---

## 6. Files to Modify

| File | Changes |
|------|---------|
| `test/e2e/remediationorchestrator/ea_creation_e2e_test.go` | RR namespace → controllerNamespace; all List/Get namespaces |
| `test/e2e/remediationorchestrator/completion_notification_e2e_test.go` | Same |
| `test/e2e/remediationorchestrator/needs_human_review_e2e_test.go` | Same |
| `test/e2e/remediationorchestrator/proactive_signal_mode_e2e_test.go` | Same |
| `test/e2e/remediationorchestrator/scope_blocking_e2e_test.go` | RR namespace → controllerNamespace; lookup namespace → controllerNamespace |
| `test/shared/helpers/crd_lifecycle.go` | Add reader param to Simulate*; use for Get in RetryOnConflict |
| `test/e2e/remediationorchestrator/approval_e2e_test.go` | Pass apiReader to SimulateAICompletedWithWorkflowWithReader |
| `test/e2e/remediationorchestrator/lifecycle_e2e_test.go` | Pass apiReader to Simulate* (if exported) |

---

## 7. Risk Assessment

| Risk | Mitigation |
|------|------------|
| Breaking other test packages using crd_lifecycle | Keep backward-compatible overloads (reader=nil → use k8sClient) |
| Scope test semantics | ADR-057: scope uses TargetResource.Namespace; RR namespace is irrelevant for scope logic |
| Parallel flakiness | apiReader bypasses cache; reduces conflict rate |
