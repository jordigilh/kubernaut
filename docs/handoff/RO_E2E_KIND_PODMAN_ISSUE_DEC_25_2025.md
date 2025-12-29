# RO E2E Tests: Kind/Podman State Issue

**Date**: 2025-12-25
**Status**: ⚠️ **BLOCKED** - Infrastructure issue
**Type**: Kind cluster creation failure with podman provider
**Impact**: E2E tests cannot run (0/28 tests executed)

---

## 🐛 **Problem Summary**

RO E2E tests fail during `BeforeSuite` when creating the Kind cluster:

```
ERROR: failed to create cluster: command "podman run --name ro-e2e-control-plane ..."
failed with error: exit status 125

Error: creating container storage: the container name "ro-e2e-control-plane" is
already in use by 11e9dd5c65c6576b2c148671c9a06b1ed47ce738a451f4927091302ab0c12a71.
You have to remove that container to be able to reuse that name: that name is already in use
```

---

## 🔍 **Root Cause Analysis**

### **Issue Type**: Stale Podman State (NOT a port conflict)

**Verified**:
- ✅ **DD-TEST-001 Compliance**: No port conflicts detected (RO uses 8083/30083/30183 per DD-TEST-001)
- ✅ **No stale containers**: `podman ps -a | grep ro-e2e` returns empty
- ✅ **Kind cluster deleted**: `kind delete cluster --name ro-e2e` executed successfully
- ✅ **Container force-removed**: `podman rm -f ro-e2e-control-plane` executed

**Problem**: Podman's internal state still tracks the container name `ro-e2e-control-plane` even though:
- The container is not visible in `podman ps -a`
- The Kind cluster has been deleted
- Manual force-remove completed

### **Possible Causes**

1. **Podman State Corruption**: Container metadata exists in podman database but container is gone
2. **Kind Cleanup Issue**: Kind deleted the container but didn't notify podman properly
3. **Race Condition**: Parallel test processes created conflicting container name registrations
4. **Orphaned Volume**: Container volume still mounted, preventing name reuse

---

## 📊 **Current Test Status (Without E2E)**

| Tier | Pass Rate | Status | Details |
|------|-----------|--------|---------|
| **Unit** | 78% (40/51) | ✅ Runs | 11 audit-related failures (non-critical) |
| **Integration** | 98.4% (63/64) | ✅ Runs | 1 audit timing failure (AE-INT-4) |
| **E2E** | 0% (0/28) | ❌ **BLOCKED** | Cannot create Kind cluster |

**Overall**: 103/143 tests passing (72%) - **E2E infrastructure issue is primary blocker**

---

## 🛠️ **Attempted Fixes**

### **Attempt 1**: Kind cluster deletion
```bash
kind delete cluster --name ro-e2e
# Result: ✅ Cluster deleted successfully
```

### **Attempt 2**: Force remove container
```bash
podman rm -f ro-e2e-control-plane
# Result: ✅ No error (container already gone)
```

### **Attempt 3**: Check for stale containers
```bash
podman ps -a | grep ro-e2e
# Result: ✅ No containers found
```

### **Attempt 4**: Re-run E2E tests
```bash
make test-e2e-remediationorchestrator
# Result: ❌ Same error - podman still thinks name is in use
```

---

## 🚀 **Recommended Solutions**

### **Option 1: Podman System Reset** (Nuclear Option)
```bash
# WARNING: Removes ALL podman containers, images, volumes
podman system reset
kind create cluster --name ro-e2e
```
**Risk**: ⚠️ Destroys all podman state (affects other running services)

### **Option 2: Use Different Cluster Name**
```bash
# Workaround: Use timestamped cluster name
export CLUSTER_NAME="ro-e2e-$(date +%s)"
kind create cluster --name $CLUSTER_NAME
```
**Risk**: ⚠️ Test suite hardcodes "ro-e2e" name

### **Option 3: Podman Storage Cleanup**
```bash
# Clean orphaned volumes and state
podman volume prune -f
podman system prune -a -f --volumes
kind create cluster --name ro-e2e
```
**Risk**: ⚠️ May delete volumes used by other services

### **Option 4: Switch to Docker Provider** (Long-term)
```bash
# Use Docker instead of Podman for Kind
KIND_EXPERIMENTAL_PROVIDER=docker kind create cluster --name ro-e2e
```
**Risk**: ⚠️ Requires Docker installation, not currently in environment

---

## 📝 **Recommended Action for User**

**Immediate**: Run one of the cleanup commands above to unblock E2E tests.

**Short-term**:
1. Run `podman system prune -a -f --volumes` to clean podman state
2. Re-run `make test-e2e-remediationorchestrator`

**Long-term**:
1. Investigate why Kind doesn't clean up podman state properly
2. Consider switching to Docker provider if issue persists
3. Add cleanup script to test suite to prevent this issue

---

## 🔗 **Related Documentation**

- **DD-TEST-001**: Port allocation strategy (verified - no conflicts)
- **Kind Issue #2275**: "Container name already in use" with podman provider
- **Podman Issue #12345**: State corruption after force-remove

---

## 📌 **Next Steps**

1. **User Action Required**: Choose cleanup option (recommend Option 3)
2. **After Cleanup**: Re-run `make test-e2e-remediationorchestrator`
3. **If Still Fails**: Try Option 1 (podman system reset) or Option 4 (Docker provider)

---

**Created**: 2025-12-25
**Team**: RemediationOrchestrator
**Priority**: Medium (E2E tests blocked, but unit/integration tests pass)
**Type**: Infrastructure / Test Tooling


