# V1.0 Service Maturity Triage - All Services

**Date**: December 19, 2025
**Scope**: All Kubernaut services for V1.0 readiness
**Status**: 🔶 **GAPS IDENTIFIED**

---

## Service Inventory

| Service | Type | Language | Controller? |
|---------|------|----------|-------------|
| **SignalProcessing (SP)** | CRD Controller | Go | ✅ |
| **WorkflowExecution (WE)** | CRD Controller | Go | ✅ |
| **AIAnalysis (AA)** | CRD Controller | Go | ✅ |
| **Notification (NOT)** | CRD Controller | Go | ✅ |
| **RemediationOrchestrator (RO)** | CRD Controller | Go | ✅ |
| **Gateway (GW)** | HTTP API | Go | ❌ |
| **DataStorage (DS)** | HTTP API | Go | ❌ |
| **HolmesGPT API (HAPI)** | HTTP API | Python | ❌ |

---

## 🎯 Maturity Comparison Matrix - CRD Controllers

| Feature | SP | WE | AA | NOT | RO |
|---------|----|----|----|----|-----|
| **Metrics in controller** | ❌ | ✅ | 🟡 | ✅ | 🟡 |
| **Metrics registered with CR** | ❌ | ✅ | 🟡 | ✅ | 🟡 |
| **EventRecorder** | ❌ | ✅ | ✅ | ❌ | ❌ |
| **Predicates (event filter)** | ❌ | ✅ | ✅ | ✅ | 🟡 |
| **Logger field in struct** | ❌ | 🟡 | ✅ | 🟡 | 🟡 |
| **Graceful shutdown (DD-007)** | ✅ | ✅ | ✅ | ✅ | ✅ |
| **Audit integration** | ✅ | ✅ | ✅ | ✅ | ✅ |
| **Healthz probes** | ✅ | ✅ | ✅ | ✅ | ✅ |
| **Config validation (ADR-030)** | ✅ | ✅ | ✅ | ✅ | ✅ |

**Legend**: ✅ Complete | 🟡 Partial | ❌ Missing

---

## 🎯 Maturity Comparison Matrix - HTTP Services

| Feature | GW | DS | HAPI |
|---------|----|----|------|
| **Prometheus metrics** | ✅ | ✅ | ✅ |
| **Health endpoints** | ✅ | ✅ | ✅ |
| **Graceful shutdown (DD-007)** | ✅ | ✅ | ✅ |
| **Audit integration** | ✅ | ✅ | ✅ |
| **Config validation (ADR-030)** | ✅ | ✅ | ✅ |
| **RFC 7807 errors** | ✅ | ✅ | ✅ |
| **OpenAPI spec** | ✅ | ✅ | ✅ |
| **Request logging** | ✅ | ✅ | ✅ |

---

## 🔴 Critical Gaps by Service

### SignalProcessing (SP) - 🔴 MOST GAPS

| Gap | Severity | Status |
|-----|----------|--------|
| Controller doesn't use Metrics | 🔴 CRITICAL | ❌ Not wired |
| No EventRecorder | 🔴 CRITICAL | ❌ Missing |
| Metrics not registered with controller-runtime | 🔴 CRITICAL | ❌ Not registered |
| No Predicates | 🟡 HIGH | ❌ Missing |
| No Logger field | 🟡 HIGH | ❌ Uses inline |

**Fix Effort**: ~1.5 hours

---

### WorkflowExecution (WE) - ✅ MATURE (Reference)

| Feature | Status | Notes |
|---------|--------|-------|
| Metrics in controller | ✅ | `internal/controller/workflowexecution/metrics.go` |
| Metrics registered with CR | ✅ | `metrics.Registry.MustRegister()` in `init()` |
| EventRecorder | ✅ | `mgr.GetEventRecorderFor()` |
| Predicates | ✅ | Uses event filtering |
| Audit | ✅ | Uses `audit.AuditStore` |

**Reference for other controllers.**

---

### AIAnalysis (AA) - 🟡 PARTIAL

| Gap | Severity | Status |
|-----|----------|--------|
| Metrics in controller | 🟡 HIGH | Uses timing but no package |
| EventRecorder | ✅ | Present |
| Predicates | ✅ | `predicate.GenerationChangedPredicate{}` |
| Logger field | ✅ | Has `Log logr.Logger` |

**Fix Effort**: ~30 min (add metrics package)

---

### Notification (NOT) - 🟡 PARTIAL

| Gap | Severity | Status |
|-----|----------|--------|
| Metrics in controller | ✅ | `internal/controller/notification/metrics.go` |
| EventRecorder | 🟡 HIGH | ❌ Missing |
| Predicates | ✅ | Present |
| Audit | ✅ | Uses `audit.AuditStore` |

**Fix Effort**: ~20 min (add EventRecorder)

---

### RemediationOrchestrator (RO) - 🟡 PARTIAL

| Gap | Severity | Status |
|-----|----------|--------|
| Metrics in controller | 🟡 HIGH | Uses CR server metrics only |
| EventRecorder | 🟡 HIGH | ❌ Missing |
| Predicates | 🟡 MEDIUM | Partial usage |
| Audit | ✅ | Uses `audit.AuditStore` |

**Fix Effort**: ~45 min

---

### Gateway (GW) - ✅ MATURE

| Feature | Status | Notes |
|---------|--------|-------|
| Prometheus metrics | ✅ | `pkg/gateway/metrics/metrics.go` |
| HTTP metrics middleware | ✅ | `pkg/gateway/middleware/http_metrics.go` |
| Health endpoints | ✅ | Present |
| Graceful shutdown | ✅ | DD-007 compliant |
| Audit | ✅ | Uses AuditStore |

**No critical gaps.**

---

### DataStorage (DS) - ✅ MATURE

| Feature | Status | Notes |
|---------|--------|-------|
| Prometheus metrics | ✅ | `pkg/datastorage/metrics/metrics.go` |
| DLQ metrics | ✅ | `pkg/datastorage/dlq/metrics.go` |
| Health endpoints | ✅ | Present |
| Graceful shutdown | ✅ | DD-007 compliant |
| Validation | ✅ | `pkg/datastorage/validation/` |

**No critical gaps.**

---

### HolmesGPT API (HAPI) - ✅ MATURE

| Feature | Status | Notes |
|---------|--------|-------|
| Prometheus metrics | ✅ | `src/middleware/metrics.py` |
| Health endpoints | ✅ | `src/extensions/health.py` |
| Graceful shutdown | ✅ | DD-007/BR-HAPI-201 compliant |
| Audit integration | ✅ | `src/audit/` package |
| RFC 7807 errors | ✅ | `src/middleware/rfc7807.py` |
| Hot-reload config | ✅ | `src/config/hot_reload.py` |

**No critical gaps.**

---

## 📊 Gap Summary by Priority

### P0 - Blockers for V1.0 (Must Fix)

| Service | Gap | Effort |
|---------|-----|--------|
| **SP** | Wire metrics to controller | 30 min |
| **SP** | Register metrics with controller-runtime | 20 min |
| **SP** | Add EventRecorder | 20 min |

**Total P0 Effort**: ~70 min

---

### P1 - High Priority (Should Fix)

| Service | Gap | Effort |
|---------|-----|--------|
| **SP** | Add Predicates | 10 min |
| **NOT** | Add EventRecorder | 20 min |
| **RO** | Add controller-level metrics | 30 min |
| **RO** | Add EventRecorder | 20 min |
| **AA** | Add metrics package | 30 min |

**Total P1 Effort**: ~110 min (~2 hours)

---

### P2 - Medium Priority (Nice to Have)

| Service | Gap | Effort |
|---------|-----|--------|
| **SP** | Add Logger field | 15 min |
| **RO** | Improve predicate usage | 15 min |

**Total P2 Effort**: ~30 min

---

## 📋 Fix Priority Order

1. **SP P0 Gaps** - Most critical, affecting observability
2. **NOT EventRecorder** - Debugging capability
3. **RO Metrics + EventRecorder** - Operational visibility
4. **AA Metrics** - SLO monitoring
5. **P2 Items** - Post-V1.0

---

## 🎯 Recommended Actions

### Immediate (Before V1.0 Release)

1. **Fix SP P0 gaps** (~70 min)
   - Wire metrics to controller struct
   - Register with controller-runtime registry
   - Add EventRecorder

2. **Add EventRecorder to NOT, RO** (~40 min)
   - Enables `kubectl describe` debugging

### Pre-V1.0 (Optional but Recommended)

3. **Add metrics to AA, RO controllers** (~60 min)
   - Consistent SLO monitoring across all controllers

4. **Add predicates to SP** (~10 min)
   - Reduces unnecessary reconciliation

---

## Reference Architecture (WE Controller)

The WorkflowExecution controller is the most mature and should be used as reference:

```go
// internal/controller/workflowexecution/workflowexecution_controller.go
type WorkflowExecutionReconciler struct {
    client.Client
    Scheme   *runtime.Scheme
    Recorder record.EventRecorder  // ✅ EventRecorder
    AuditStore audit.AuditStore    // ✅ Audit
    // ... other fields
}

// internal/controller/workflowexecution/metrics.go
func init() {
    metrics.Registry.MustRegister(  // ✅ Registered with CR
        WorkflowExecutionTotal,
        WorkflowExecutionDuration,
        PipelineRunCreationTotal,
    )
}
```

---

## Conclusion

| Category | Services Affected | Total Effort |
|----------|-------------------|--------------|
| P0 Blockers | SP | 70 min |
| P1 High | SP, NOT, RO, AA | 110 min |
| P2 Medium | SP, RO | 30 min |
| **Total** | **4 services** | **~3.5 hours** |

**Recommendation**: Fix P0 + P1 gaps before V1.0 release (~3 hours total).

---

## References

- WE Controller (reference): `internal/controller/workflowexecution/`
- SP Metrics Package: `pkg/signalprocessing/metrics/metrics.go`
- Metrics SLOs: `docs/services/crd-controllers/01-signalprocessing/metrics-slos.md`
- DD-005: Observability Standards
- DD-007: Graceful Shutdown Pattern

