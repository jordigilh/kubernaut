# Gateway E2E API Group Mismatch - CRITICAL BLOCKER

**Date**: December 13, 2025
**Status**: 🔴 **BLOCKED** - API group mismatch between code and CRD
**Root Cause**: Code expects `kubernaut.ai` but CRD is `remediation.kubernaut.ai`
**Impact**: Gateway pod crashes on startup, blocks all E2E tests

---

## 🚨 Critical Issue

Gateway pod crashing with API group mismatch:

```
Failed to create Gateway server:
failed to create fingerprint field index:
unable to retrieve the complete list of server APIs:
kubernaut.ai/v1alpha1: no matches for kubernaut.ai/v1alpha1, Resource=
```

**Root Cause**: API Group Migration incomplete

**Code Expects**: `kubernaut.ai/v1alpha1` (per `api/remediation/v1alpha1/groupversion_info.go:30`)
**CRD Installed**: `remediation.kubernaut.ai/v1alpha1` (per `config/crd/bases/remediation.kubernaut.ai_remediationrequests.yaml`)

---

## 🔍 Analysis

### API Group Migration Status

**Code Side** (✅ MIGRATED):
```go
// api/remediation/v1alpha1/groupversion_info.go:30
GroupVersion = schema.GroupVersion{Group: "kubernaut.ai", Version: "v1alpha1"}
```

**CRD Manifest** (❌ NOT MIGRATED):
```yaml
# config/crd/bases/remediation.kubernaut.ai_remediationrequests.yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: remediationrequests.remediation.kubernaut.ai  # ← OLD API GROUP
spec:
  group: remediation.kubernaut.ai  # ← OLD API GROUP
```

### Why This Happened

1. **API Group Migration**: System-wide migration from `remediation.kubernaut.ai` → `kubernaut.ai`
2. **Code Updated**: `groupversion_info.go` already migrated
3. **CRD Not Regenerated**: `make manifests` not run after migration
4. **Compilation Error Blocking**: Notification controller has syntax errors, preventing `make manifests`

---

## 🛠️ Solutions

### Option A: Fix Notification Controller, Then Regenerate CRDs (PROPER FIX)
**What**: Fix syntax errors in notification controller, then run `make manifests`
**Why**: Ensures all CRDs are properly regenerated with correct API group
**How**:
```bash
# Fix notification controller syntax errors
# (Lines 193, 203, 877, 926 in internal/controller/notification/notificationrequest_controller.go)

# Regenerate CRDs
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
make manifests

# Verify CRD has correct API group
grep "group:" config/crd/bases/*.yaml
```
**Expected**: CRD manifest updated to `kubernaut.ai`
**Risk**: LOW - Proper fix

---

### Option B: Manually Update CRD Manifest (QUICK WORKAROUND)
**What**: Manually edit CRD YAML to use `kubernaut.ai`
**Why**: Bypasses notification controller compilation errors
**How**:
```bash
# Edit config/crd/bases/remediation.kubernaut.ai_remediationrequests.yaml
# Change:
#   name: remediationrequests.remediation.kubernaut.ai
#   group: remediation.kubernaut.ai
# To:
#   name: remediationrequests.kubernaut.ai
#   group: kubernaut.ai

# Also rename file:
mv config/crd/bases/remediation.kubernaut.ai_remediationrequests.yaml \
   config/crd/bases/kubernaut.ai_remediationrequests.yaml
```
**Expected**: Gateway starts successfully
**Risk**: MEDIUM - Manual edit, may be overwritten by `make manifests`

---

### Option C: Revert API Group Migration in Code (ROLLBACK)
**What**: Revert `groupversion_info.go` to use `remediation.kubernaut.ai`
**Why**: Match existing CRD manifests
**How**:
```go
// api/remediation/v1alpha1/groupversion_info.go
GroupVersion = schema.GroupVersion{Group: "remediation.kubernaut.ai", Version: "v1alpha1"}
```
**Expected**: Gateway starts with old API group
**Risk**: HIGH - Reverses migration work, not aligned with system-wide migration

---

## 📋 Recommendation

**Option A** (Fix Notification Controller + Regenerate CRDs)

**Why**:
- ✅ Proper fix that aligns code and manifests
- ✅ Ensures all CRDs are consistently migrated
- ✅ Prevents future mismatches
- ✅ Follows proper development workflow

**Steps**:
1. Fix notification controller syntax errors (4 locations)
2. Run `make manifests`
3. Verify CRD API group is `kubernaut.ai`
4. Rebuild Gateway image
5. Run E2E tests

**Estimated Time**: 15-20 minutes

---

## 🔗 Related

**API Group Migration**: `docs/handoff/SHARED_APIGROUP_MIGRATION_NOTICE.md`
**Design Decision**: `docs/architecture/decisions/DD-CRD-001-api-group-domain-selection.md`

---

**Status**: 🔴 **BLOCKED** - API group mismatch
**Priority**: P0 - Blocks Gateway E2E testing
**Owner**: Gateway Team (requires notification controller fix from other team)
**Recommended**: Option A (proper fix)


