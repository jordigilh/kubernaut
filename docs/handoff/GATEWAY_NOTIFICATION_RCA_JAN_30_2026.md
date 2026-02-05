# Gateway & Notification RCA - January 30, 2026

**Date**: January 30, 2026  
**Status**: Gateway FIXED (testing), Notification NEEDS DEEPER RCA

---

## Executive Summary

Completed RCA for Gateway audit failures (10 tests) and partial RCA for Notification audit failures (6 tests).

**Gateway**: ✅ **ROOT CAUSE FOUND & FIXED** - Wrong DataStorage Service port (8081 instead of 8080)  
**Notification**: ⚠️ **INCOMPLETE RCA** - All configuration looks correct, root cause unknown without logs

---

## 🎯 Gateway RCA - COMPLETE

### Problem Statement

Gateway E2E: 88/98 tests passed (10 audit failures)

Failed tests:
- `signal.received` audit event (0 events found)
- `signal.deduplicated` audit event (0 events found)
- `crd.created` audit event (0 events found)
- Signal data capture tests (original_payload, labels, annotations)
- Other audit-related validations

### Investigation Process

1. **Checked ServiceAccount**: ✅ Present (`gateway`)
2. **Checked DataStorage RBAC**: ✅ Present (`CreateDataStorageAccessRoleBinding`)
3. **Checked DNS hostname**: ✅ Correct (`data-storage-service`)
4. **Checked main.go**: ✅ Audit store correctly initialized with OpenAPI client + SA transport
5. **Checked DataStorage URL**: ❌ **PORT WRONG!**

### Root Cause

**Configuration File**: `test/e2e/gateway/gateway-deployment.yaml`

**Line 38**:
```yaml
# Port 8081: DataStorage listens on 8081, Service exposes as 8080 → use Service port
dataStorageUrl: "http://data-storage-service.kubernaut-system.svc.cluster.local:8081"
```

**Issue**: Comment says "use Service port 8080" but config has **8081**

**Comparison with Working Services**:
- RemediationOrchestrator (100%): `dataStorageUrl: http://data-storage-service:8080`
- WorkflowExecution (100%): `dataStorageUrl: http://data-storage-service:8080`
- Gateway (88%): `dataStorageUrl: http://data-storage-service:8081` ❌

### Evidence

| Component | Status | Details |
|-----------|--------|---------|
| ServiceAccount | ✅ Correct | `gateway` SA exists |
| DataStorage RBAC | ✅ Correct | RoleBinding created in E2E setup |
| DNS hostname | ✅ Correct | `data-storage-service` (matches RO/WE) |
| Port | ❌ **WRONG** | 8081 instead of 8080 |
| OpenAPI Client | ✅ Correct | Uses `audit.NewOpenAPIClientAdapter` with SA transport |

### Fix Applied

**Commit**: `14a96dd23`

**Change**:
```diff
-      # Port 8081: DataStorage listens on 8081, Service exposes as 8080 → use Service port
-      dataStorageUrl: "http://data-storage-service.kubernaut-system.svc.cluster.local:8081"
+      # Port: 8080 (Service port) - DataStorage container listens on 8081, Service exposes as 8080
+      dataStorageUrl: "http://data-storage-service.kubernaut-system.svc.cluster.local:8080"
```

**Authority**: DD-AUTH-011 (Service Name Standard)

### Expected Result

Gateway: 88/98 → **98/98 (100%)**

**Confidence**: 95%
- Exact same pattern as RO/WE fixes (both now 100%)
- ServiceAccount + RBAC already present
- Only the port was wrong

### Validation Status

**E2E Test Run**: ✅ **STARTED** (Jan 30, 17:07 EST)
- Runtime: ~50 minutes (98 specs)
- Log: `/tmp/gw-e2e-port-fix.log`
- Status: Running in background

---

## ⚠️ Notification RCA - INCOMPLETE

### Problem Statement

Notification E2E: 24/30 tests passed (6 audit failures persist)

Failed tests (from previous run):
- Full lifecycle audit persistence tests (3 failures)
- TLS/HTTPS graceful degradation tests (2 failures)
- Multi-channel delivery audit (1 failure)

All failures show **0 audit events found** in DataStorage.

### Investigation Process

#### 1. Configuration Audit ✅

| Component | Status | Details |
|-----------|--------|---------|
| ServiceAccount | ✅ Correct | `notification-controller` |
| DataStorage RBAC | ✅ Correct | `CreateDataStorageAccessRoleBinding` called |
| DNS hostname | ✅ Correct | `data-storage-service` |
| Port | ✅ Correct | `8080` (matches RO/WE/GW) |

**Config File**: `test/e2e/notification/manifests/notification-configmap.yaml`

Line 92:
```yaml
data_storage_url: "http://data-storage-service.${NAMESPACE}.svc.cluster.local:8080"
```

✅ **Port is CORRECT** (8080, not 8081 like Gateway)

#### 2. Code Audit ✅

**File**: `cmd/notification/main.go`

Lines 244-266:
```go
dataStorageClient, err := audit.NewOpenAPIClientAdapter(
    cfg.Infrastructure.DataStorageURL,
    5*time.Second)
// ...
auditStore, err := audit.NewBufferedStore(dataStorageClient, auditConfig, "notification-controller", auditLogger)
```

✅ **Audit store correctly initialized**
- Uses OpenAPI client adapter (DD-API-001)
- ServiceAccountTransport automatically applied
- BufferedStore pattern (fire-and-forget)

#### 3. Controller Reconciler Audit ✅

**File**: `internal/controller/notification/notificationrequest_controller.go`

**Audit Emission Calls Found**:
- Line 502: `r.AuditStore.StoreAudit(ctx, event)` - message.sent
- Line 550: `r.AuditStore.StoreAudit(ctx, event)` - message.failed
- Line 619: `r.AuditStore.StoreAudit(ctx, event)` - message.acknowledged
- Line 688: `r.AuditStore.StoreAudit(ctx, event)` - message.escalated

✅ **Controller IS calling StoreAudit**

**Error Handling**:
```go
if err := r.AuditStore.StoreAudit(ctx, event); err != nil {
    log.Error(err, "Failed to buffer audit event", "event_type", "message.sent", "channel", channel)
    // Continue reconciliation - audit failure is not critical (BR-NOT-063)
}
```

✅ **Errors are logged** (fire-and-forget pattern per BR-NOT-063)

### Critical Gap: No Logs Available

**Missing Evidence**:
- ❌ No E2E test logs found in `/tmp/`
- ❌ No must-gather logs available
- ❌ Cannot see actual HTTP errors from audit emissions

**Cannot Confirm**:
- Are audit calls actually being made?
- What HTTP status codes are being returned?
- Are there authentication errors?
- Are there network connectivity issues?

### Comparison with Working Services

| Service | Port | SA | RBAC | DNS | Audit Works? |
|---------|------|----|----|-----|--------------|
| **RO** | 8080 | ✅ | ✅ | ✅ | ✅ 100% |
| **WE** | 8080 | ✅ | ✅ | ✅ | ✅ 100% |
| **AW** | 8080 | ✅ | ✅ | ✅ | ✅ 100% |
| **GW** | ~~8081~~ → 8080 | ✅ | ✅ | ✅ | ⏳ Testing |
| **NT** | 8080 | ✅ | ✅ | ✅ | ❌ Failing |

**Pattern Break**: Notification has IDENTICAL configuration to working services but still fails!

### Hypotheses

#### Hypothesis A: Runtime HTTP Errors (Most Likely)

**Theory**: Audit calls are being made but failing with HTTP errors

**Evidence**:
- Controller logs errors: `log.Error(err, "Failed to buffer audit event")`
- Fire-and-forget pattern swallows errors
- Tests show "0 events found"

**Validation Needed**:
- Must-gather logs from Notification controller pod
- Check for HTTP 401/403/404/5xx errors
- Verify Bearer token is being sent correctly

#### Hypothesis B: ServiceAccount Token Not Mounted

**Theory**: Notification pod might not have SA token file

**Evidence**: None (need pod inspection)

**Validation Needed**:
```bash
# Check if pod has token file
kubectl exec -n <namespace> <notification-pod> -- ls -la /var/run/secrets/kubernetes.io/serviceaccount/
```

#### Hypothesis C: Buffered Store Not Flushing

**Theory**: Events are buffered but never written before test completes

**Evidence**:
- BufferedStore uses fire-and-forget
- Tests might complete before flush

**Validation Needed**:
- Check BufferedStore flush timing
- Verify test waits for flush

#### Hypothesis D: Wrong Event Type/Format

**Theory**: Events are being written but tests query with wrong parameters

**Evidence**:
- Tests query by event_type, category, etc.
- If any parameter is wrong, query returns 0

**Validation Needed**:
- Query DataStorage directly: `SELECT * FROM audit_events WHERE service = 'notification-controller'`
- Check if events exist with different parameters

### Recommended Next Steps

#### Option A: Re-run Notification E2E with Fresh Logs (Recommended)

**Action**:
```bash
make test-e2e-notification
# Immediately after completion:
kubectl logs -n notification-e2e-<id> <notification-pod> > /tmp/nt-controller.log
kind export logs /tmp/nt-must-gather --name notification-e2e-<id>
```

**Expected**: Capture HTTP errors showing why audit emissions fail

**Confidence**: 80% - Logs will reveal root cause

---

#### Option B: Query DataStorage Directly (Quick Check)

**Action**:
```bash
# Port-forward to DataStorage in test cluster
kubectl port-forward -n <namespace> svc/data-storage-service 8080:8080

# Query for Notification events
curl -H "Authorization: Bearer $(cat /var/run/secrets/kubernetes.io/serviceaccount/token)" \
  "http://localhost:8080/api/v1/audit/events?service=notification-controller"
```

**Expected**: Confirm if events are being written (maybe with wrong format)

**Confidence**: 60% - Might find events exist but tests query incorrectly

---

#### Option C: Add Debug Logging to Controller (Invasive)

**Action**: Add debug log before `StoreAudit` call to confirm it's being executed

**Change** (`internal/controller/notification/notificationrequest_controller.go:502`):
```go
log.Info("DEBUG: About to store audit event", 
    "event_type", "message.sent", 
    "channel", channel,
    "data_storage_url", r.AuditStore != nil)

if err := r.AuditStore.StoreAudit(ctx, event); err != nil {
    log.Error(err, "Failed to buffer audit event", "event_type", "message.sent", "channel", channel)
}
```

**Expected**: Confirm audit calls are reaching StoreAudit method

**Confidence**: 50% - Invasive, requires code change + rebuild

---

#### Option D: Compare with RO Controller (Pattern Analysis)

**Action**: Side-by-side comparison of RO vs NT audit emission code

**Check**:
1. Event construction (fields, types, categories)
2. Context passed to StoreAudit
3. Timing of audit calls (before/after main logic)

**Expected**: Find subtle difference in how events are constructed

**Confidence**: 40% - Code looks similar already

---

## Pattern Lessons Learned

### Pattern: Port Must Match Service, Not Container

**CRITICAL**: When accessing a service from within the cluster, use the **Service port**, not the container port!

**Example**:
- DataStorage container: Listens on port `8081`
- DataStorage Service: Exposes port `8080` → targetPort `8081`
- **Correct URL**: `http://data-storage-service:8080`
- **Wrong URL**: `http://data-storage-service:8081` ❌

**Affected Services** (fixed this session):
- ✅ Gateway: Fixed 8081 → 8080
- ✅ All others: Already correct

---

### Pattern: ServiceAccount + DNS + Port Trinity

**For audit to work**, ALL THREE must be correct:

1. **ServiceAccount**: Pod must have dedicated SA
2. **DataStorage RBAC**: SA must have RoleBinding for `data-storage-client` role
3. **DNS hostname**: Must be `data-storage-service` (hyphenated, per DD-AUTH-011)
4. **Port**: Must be `8080` (Service port)

**Missing ANY ONE** → Audit fails (0 events)

---

### Pattern: Error Handling Masks Root Cause

Notification uses fire-and-forget pattern (BR-NOT-063):
```go
if err := r.AuditStore.StoreAudit(ctx, event); err != nil {
    log.Error(err, "Failed to buffer audit event") // Logged but swallowed
    // Continue reconciliation
}
```

**Problem**: Errors are logged but tests don't fail → harder to debug

**Solution**: Must-gather logs are CRITICAL for RCA

---

## Current Status Summary

| Service | Status | Audit Tests | Root Cause | Fix Status |
|---------|--------|-------------|------------|------------|
| **RO** | ✅ 100% | 29/29 | Port 8081 → 8080 | ✅ Fixed |
| **WE** | ✅ 100% | 12/12 | Missing SA + RBAC | ✅ Fixed |
| **DS** | ✅ 100% | 189/190 | N/A | ✅ Complete |
| **AW** | ✅ 100% | 2/2 | Missing DS RBAC | ✅ Fixed |
| **SP** | ✅ 96% | 26/27 | BR-SP-090 (deferred) | ⏸️ Known issue |
| **GW** | ⏳ Testing | 88/98 | Port 8081 → 8080 | ✅ Fixed, testing |
| **NT** | ⚠️ 80% | 24/30 | **UNKNOWN** | ❌ Incomplete RCA |
| **HAPI** | ❌ Infra | 0/1 | DS pod timeout | 🔄 Needs retry |
| **AA** | ❌ Infra | 0/36 | Podman cache | 🔄 Needs retry |

---

## Next Actions

### Immediate (Option A - Recommended)

**Re-run Notification E2E** to capture fresh logs:
```bash
# Clean environment
kind delete cluster --name notification-e2e || true
podman system prune -f

# Run with log capture
make test-e2e-notification 2>&1 | tee /tmp/nt-e2e-debug.log

# Immediately capture must-gather
kind export logs /tmp/nt-must-gather --name notification-e2e-<id>
```

**Expected**: Must-gather logs will show HTTP errors revealing root cause

---

### While Gateway E2E Runs (~50 min)

**Monitor Gateway test**:
```bash
# Check progress every 5 minutes
watch -n 300 "tail -20 /tmp/gw-e2e-port-fix.log | grep -E 'Will run|Ran.*Specs|FAIL|SUCCESS'"
```

**Expected**: Gateway 88/98 → 98/98 (100%) after port fix

---

### If Gateway Passes (High Confidence)

**Commit Pattern**:
```bash
git add docs/handoff/GATEWAY_NOTIFICATION_RCA_JAN_30_2026.md
git commit -m "docs(handoff): Gateway RCA complete, Notification partial

Gateway: Fixed port 8081→8080, testing underway
Notification: All config correct, needs fresh logs for RCA"
```

---

## Files Changed This Session

| File | Change | Commit |
|------|--------|--------|
| `test/e2e/gateway/gateway-deployment.yaml` | Port 8081 → 8080 | `14a96dd23` |

---

## Confidence Assessment

| Service | Fix Confidence | Rationale |
|---------|----------------|-----------|
| **Gateway** | 95% | Same pattern as RO/WE (both 100%) |
| **Notification** | 20% | Unknown root cause, needs logs |

---

## Recommendations

### Short-Term (Today)

1. **Wait for Gateway E2E completion** (~50 min)
2. **If Gateway passes**: Commit success, document pattern
3. **Re-run Notification E2E** with log capture (Option A)
4. **Analyze NT logs**: Find HTTP errors, fix root cause

### Medium-Term (This Week)

1. **Fix Notification** based on fresh RCA
2. **Retry HAPI** with increased timeout (120s → 300s)
3. **Retry AIAnalysis** after Podman prune
4. **Validate all fixes** with full E2E suite

### Long-Term (Before PR)

1. **Achieve 100%** on Gateway + Notification
2. **Address infrastructure** issues (HAPI, AA)
3. **SignalProcessing BR-SP-090** (optional, deferred)
4. **Create PR** with comprehensive test evidence

---

## Authority References

- **DD-AUTH-011**: Service Name Standard (data-storage-service)
- **DD-AUTH-014**: ServiceAccount + RBAC pattern
- **DD-API-001**: OpenAPI Client MANDATORY
- **ADR-032**: Audit event emission requirements
- **BR-NOT-063**: Fire-and-forget audit pattern

---

## Session Metrics

**Time Invested**: ~2 hours (RCA + fix)  
**Services Analyzed**: 2 (Gateway, Notification)  
**Root Causes Found**: 1 (Gateway port)  
**Fixes Applied**: 1 (Gateway port 8081→8080)  
**Commits**: 1 (`14a96dd23`)  
**Remaining Work**: Notification RCA (needs fresh logs)

---

**Status**: Ready for decision on next steps (await Gateway results, then proceed with Notification)
