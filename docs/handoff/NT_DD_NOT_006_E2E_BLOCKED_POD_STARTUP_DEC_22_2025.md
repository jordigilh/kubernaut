# NT DD-NOT-006 E2E Validation - BLOCKED BY POD STARTUP

**Status**: 🔴 BLOCKED
**Date**: December 22, 2025
**Team**: Notification Team
**Blocking Issue**: Controller pod fails readiness probe (timeout after 120s)

---

## ✅ Implementation Complete (95%)

**ALL CODE IMPLEMENTED** - TDD phases 0-4 complete:
- ✅ CRD extended with `ChannelFile` and `ChannelLog`
- ✅ `LogDeliveryService` implemented (95 LOC)
- ✅ `FileDeliveryService` enhanced with CRD config
- ✅ `Orchestrator` wired for new channels
- ✅ `main.go` updated with env vars (`FILE_OUTPUT_DIR`, `LOG_DELIVERY_ENABLED`)
- ✅ 3 E2E tests written (06, 07, 05 updated)
- ✅ Code compiles, binary runs locally
- ✅ ~1600 LOC added/modified

**Files Changed**: 11 files (api, pkg, cmd, test, docs)

---

## ❌ What's Blocked

**E2E Tests Cannot Run**: Controller pod deploys but never becomes ready

**Error**: `error: timed out waiting for the condition on pods/notification-controller-XXXXX`

**Timeline**:
- Runs 1-3: Pod timeout (120s)
- Run 4: Port conflict 9186 (fixed)
- Run 5: Pod timeout again

**Root Cause Unknown** - Need pod logs (cluster auto-deletes before inspection)

---

## 🔍 Top 3 Hypotheses

1. **Volume Mount Issue** (HIGH) - `/tmp/e2e-notifications` may not be accessible in Kind
2. **LogService Init** (MEDIUM) - `LOG_DELIVERY_ENABLED=true` may cause startup error
3. **File Validation** (MEDIUM) - `validateFileOutputDirectory()` may fail in Kind

---

## 🛠️ Immediate Fix (15 min)

**Get pod logs from persistent cluster**:

```bash
# 1. Reuse existing notification-e2e cluster if it exists, or create one
kubectl --kubeconfig ~/.kube/notification-e2e-config get clusters

# 2. Get logs
kubectl --kubeconfig ~/.kube/notification-e2e-config \
  -n notification-e2e logs -l app=notification-controller --tail=100

# 3. Describe pod
kubectl --kubeconfig ~/.kube/notification-e2e-config \
  -n notification-e2e describe pod -l app=notification-controller
```

**Expected**: Specific error showing why `/readyz` probe fails

---

## 📊 Deployment Changes Made

**`test/e2e/notification/manifests/notification-deployment.yaml`**:
```yaml
env:
  - name: FILE_OUTPUT_DIR              # Changed from E2E_FILE_OUTPUT
    value: "/tmp/notifications"
  - name: LOG_DELIVERY_ENABLED          # NEW
    value: "true"

readinessProbe:
  initialDelaySeconds: 30               # Changed from 5
  timeoutSeconds: 5                     # NEW
  failureThreshold: 3                   # NEW

imagePullPolicy: Never                  # Changed from IfNotPresent
volumes:
  - hostPath:
      type: Directory                   # Changed from DirectoryOrCreate
```

---

## 🎯 Next Action

**IMMEDIATE**: Get pod logs to identify startup failure
**ETA**: 15-30 min to unblock
**Confidence**: 90% - likely simple config issue

**Documents**: See `DD-NOT-006-CHANNEL-FILE-LOG-PRODUCTION-FEATURES.md` for full design

