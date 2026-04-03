# Implementation Plan: #265 — RemediationRequest CRD 24h TTL Enforcement

**Branch**: `development/v1.4`
**Test Plan**: `docs/testing/265/TEST_PLAN.md`
**Estimated Effort**: 3-4 days

---

## Design Decisions

### D1: Centralized TTL in terminal housekeeping block

Rather than touching every terminal transition path (8+ locations), set `RetentionExpiryTime` in the existing terminal housekeeping block (`reconciler.go:546-589`). This:
- Handles ALL terminal phases (Completed, Failed, TimedOut, Cancelled, Skipped)
- Is idempotent (skip if already set)
- Adds at most one extra reconcile cycle (negligible for 24h TTL)

### D2: Setter pattern for RetentionPeriod

Use `SetRetentionPeriod(d time.Duration)` on the Reconciler, consistent with existing `SetRESTMapper`, `SetAsyncPropagation`, `SetDSClient`. Default is 24h (set in `NewReconciler`).

### D3: Cleanup at top of terminal housekeeping

Order in the terminal housekeeping block:
1. If `RetentionExpiryTime` is nil → set it → status update → RequeueAfter
2. If expired → emit audit → delete CRD → return (no requeue)
3. If not expired → RequeueAfter time.Until → return
4. (existing) Ready safety net, notification tracking, EA tracking

---

## Phase 1: Config Wiring

### Phase 1a — RED

**Files**: `test/unit/remediationorchestrator/config_test.go`

Add tests:
- UT-RO-265-011: `DefaultConfig().Retention.Period == 24 * time.Hour`
- UT-RO-265-012: `LoadFromFile` parses `retention.period: "48h"` from YAML
- UT-RO-265-013: `Validate` rejects `retention.period: "-1h"`

### Phase 1b — GREEN

**Files**:
- `internal/config/remediationorchestrator/config.go`: Add `RetentionConfig` struct with `Period time.Duration`, add to `Config`, wire in `DefaultConfig()` (24h), add validation in `Validate()`
- `internal/controller/remediationorchestrator/reconciler.go`: Add `retentionPeriod time.Duration` field, default to `24 * time.Hour` in `NewReconciler`, add `SetRetentionPeriod` setter
- `cmd/remediationorchestrator/main.go`: Call `reconciler.SetRetentionPeriod(cfg.Retention.Period)`

### Phase 1c — REFACTOR

- Ensure config test YAML fixture covers retention field
- Verify `pkg/remediationorchestrator/types.go` `RetentionPeriod` stays (used by timeout detector tests); no changes needed

---

## Phase 2: CompletedAt Fix (F3)

### Phase 2a — RED

**Files**: `test/unit/remediationorchestrator/controller/completed_at_fix_test.go` (new)

Add tests:
- UT-RO-265-009: Create RR in Executing phase, call `Reconcile` with global timeout exceeded → verify `CompletedAt` is set
- UT-RO-265-010: Create RR in Analyzing phase, trigger `transitionToFailed` → verify `CompletedAt` is set

### Phase 2b — GREEN

**Files**: `internal/controller/remediationorchestrator/reconciler.go`

- `transitionToFailed` (~line 1940 in status update closure): Add `now := metav1.Now(); rr.Status.CompletedAt = &now`
- `handleGlobalTimeout` (~line 2010 in status update closure): Add `now := metav1.Now(); rr.Status.CompletedAt = &now`

### Phase 2c — REFACTOR

- Check existing tests that might assert `CompletedAt == nil` on Failed/TimedOut paths; update if needed

---

## Phase 3: TTL Enforcement (core)

### Phase 3a — RED

**Files**: `test/unit/remediationorchestrator/controller/retention_ttl_test.go` (new)

Add tests:
- UT-RO-265-001: Completed RR with nil RetentionExpiryTime → set to ~now+24h, RequeueAfter
- UT-RO-265-002: Failed RR with nil RetentionExpiryTime → set
- UT-RO-265-003: TimedOut RR with nil RetentionExpiryTime → set
- UT-RO-265-004: Pending RR → RetentionExpiryTime remains nil
- UT-RO-265-005: RR with existing RetentionExpiryTime → not overwritten
- UT-RO-265-006: RR with expired RetentionExpiryTime → deleted
- UT-RO-265-007: RR with future RetentionExpiryTime → RequeueAfter(time.Until)
- UT-RO-265-008: Audit emitted before deletion (requires non-nil auditStore mock)

### Phase 3b — GREEN

**Files**: `internal/controller/remediationorchestrator/reconciler.go`

In the terminal housekeeping block (`Reconcile` around line 550, after the `IsTerminal` check):

```go
// #265: TTL enforcement — set RetentionExpiryTime, cleanup expired CRDs
if rr.Status.RetentionExpiryTime == nil {
    // First reconcile after terminal: stamp expiry and requeue
    expiry := metav1.NewTime(time.Now().Add(r.retentionPeriod))
    if err := helpers.UpdateRemediationRequestStatus(ctx, r.client, rr, func(rr *remediationv1.RemediationRequest) error {
        rr.Status.RetentionExpiryTime = &expiry
        return nil
    }); err != nil {
        logger.Error(err, "Failed to set RetentionExpiryTime")
        return ctrl.Result{}, err
    }
    logger.Info("RetentionExpiryTime set", "expiry", expiry.Format(time.RFC3339))
    return ctrl.Result{RequeueAfter: r.retentionPeriod}, nil
}

if time.Now().After(rr.Status.RetentionExpiryTime.Time) {
    // TTL expired — emit audit, delete CRD
    r.emitRetentionCleanupAudit(ctx, rr)
    if err := r.client.Delete(ctx, rr); err != nil {
        if !apierrors.IsNotFound(err) {
            logger.Error(err, "Failed to delete expired RemediationRequest")
            return ctrl.Result{}, err
        }
    }
    logger.Info("Deleted expired RemediationRequest", "retentionExpiryTime", rr.Status.RetentionExpiryTime.Format(time.RFC3339))
    return ctrl.Result{}, nil
}

// Not yet expired — requeue for cleanup
return ctrl.Result{RequeueAfter: time.Until(rr.Status.RetentionExpiryTime.Time)}, nil
```

Add `emitRetentionCleanupAudit` method near other audit emitters.

### Phase 3c — REFACTOR (F7 + F8)

- Remove local `IsTerminalPhase` function (reconciler.go ~line 3328)
- Update any callers to use `phase.IsTerminal` from `pkg/remediationorchestrator/phase`
- Add `"k8s.io/apimachinery/pkg/api/errors"` import if not present

---

## Phase 4: Integration Tests

### Phase 4a — RED

**Files**: `test/integration/remediationorchestrator/retention_ttl_integration_test.go` (new)

- IT-RO-265-001: Create RR → force to Failed → verify RetentionExpiryTime set → wait for TTL (use 2s retention) → verify CRD deleted
- IT-RO-265-002: Create RR → manually set expired RetentionExpiryTime on status → trigger reconcile → verify deletion

### Phase 4b — GREEN

Wire `SetRetentionPeriod(2 * time.Second)` in integration test suite for #265-specific tests.

---

## Phase 5: Helm + Manifest Updates

**Files**:
- `charts/kubernaut/values.yaml`: Add `remediationOrchestrator.config.retention.period: "24h"`
- `charts/kubernaut/templates/remediationorchestrator/remediationorchestrator.yaml`: Add `retention.period` in ConfigMap template
- `config/remediationorchestrator.yaml`: Add retention section in sample config

No new tests needed (Helm rendering covered by existing chart tests if any, or manual validation).

---

## File Change Summary

| File | Change Type | Phase |
|------|-------------|-------|
| `internal/config/remediationorchestrator/config.go` | Add RetentionConfig | 1b |
| `internal/controller/remediationorchestrator/reconciler.go` | Add retentionPeriod, TTL logic, CompletedAt fix, remove IsTerminalPhase | 1b, 2b, 3b, 3c |
| `cmd/remediationorchestrator/main.go` | Wire SetRetentionPeriod | 1b |
| `test/unit/remediationorchestrator/config_test.go` | Add retention config tests | 1a |
| `test/unit/remediationorchestrator/controller/completed_at_fix_test.go` | New: CompletedAt tests | 2a |
| `test/unit/remediationorchestrator/controller/retention_ttl_test.go` | New: TTL tests | 3a |
| `test/integration/remediationorchestrator/retention_ttl_integration_test.go` | New: TTL integration | 4a |
| `charts/kubernaut/values.yaml` | Add retention config | 5 |
| `charts/kubernaut/templates/remediationorchestrator/remediationorchestrator.yaml` | Add retention to ConfigMap | 5 |
| `config/remediationorchestrator.yaml` | Add retention section | 5 |
