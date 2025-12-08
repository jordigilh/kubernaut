# REQUEST: Notification Team - Kubeconfig Location Standardization

**Date**: December 8, 2025
**From**: RO Team (Architecture Coordination)
**To**: Notification Team
**Priority**: Medium
**Type**: Standardization Request

---

## 📋 Summary

The Notification E2E tests use a partially compliant kubeconfig location that doesn't follow the exact authoritative naming convention defined in `TESTING_GUIDELINES.md`.

---

## ⚠️ Current Implementation (Partial Compliance)

**File**: `test/e2e/notification/notification_e2e_suite_test.go`

```go
// Line 111
kubeconfigPath = fmt.Sprintf("%s/.kube/notification-kubeconfig", homeDir)
```

**Problem**: Uses `notification-kubeconfig` instead of the standard `notification-e2e-config` pattern.

---

## ✅ Required Change (Authoritative Standard)

**Per** `docs/development/business-requirements/TESTING_GUIDELINES.md`:

```go
// CORRECT: Standard E2E config naming
kubeconfigPath = fmt.Sprintf("%s/.kube/notification-e2e-config", homeDir)
```

**Also update the log message** (line 101):
```go
// FROM:
logger.Info("  • Kubeconfig: ~/.kube/notification-kubeconfig")

// TO:
logger.Info("  • Kubeconfig: ~/.kube/notification-e2e-config")
```

---

## 📐 Authoritative Standard

| Element | Standard |
|---------|----------|
| **Pattern** | `~/.kube/{service}-e2e-config` |
| **Notification Location** | `~/.kube/notification-e2e-config` |
| **Cluster Name** | `notification-e2e` |

**Reference**: `docs/development/business-requirements/TESTING_GUIDELINES.md` (Kubeconfig Isolation Policy section)

---

## 🎯 Why This Matters

1. **Consistency**: All services use same naming pattern (`{service}-e2e-config`)
2. **Discoverability**: Easy to identify E2E kubeconfigs with `ls ~/.kube/*-e2e-config`
3. **Documentation**: Single pattern to document and maintain

---

## ✅ Compliance Checklist

After making changes:
- [x] Kubeconfig path is `~/.kube/notification-e2e-config`
- [x] Log message reflects correct path
- [x] Cluster name is `notification-e2e`
- [x] Tests pass with new kubeconfig location
- [x] Old `~/.kube/notification-kubeconfig` files cleaned up (if any)

---

## 📊 Current Compliance Status

| Service | Status |
|---------|--------|
| Gateway | ❌ Non-compliant (separate request) |
| AIAnalysis | ✅ Compliant |
| RO | ✅ Compliant |
| Data Storage | ✅ Compliant |
| Notification | ✅ **Compliant** (`notification-e2e-config`) |
| WorkflowExecution | ⚠️ Partial (separate request) |
| Toolset | ⚠️ Partial (separate request) |

---

## 📝 Notification Team Response

**Status**: ✅ **COMPLETE**
**Acknowledged By**: Notification Service Team
**Date**: December 8, 2025
**Notes**:

### Implementation Details

All changes have been implemented and verified:

1. **`test/e2e/notification/notification_e2e_suite_test.go`**:
   - Line 101: Log message updated to `~/.kube/notification-e2e-config`
   - Line 111: kubeconfigPath updated to `notification-e2e-config`

2. **`test/infrastructure/notification.go`**:
   - Line 56: Comment updated to `~/.kube/notification-e2e-config`
   - Line 241: Comment updated to `~/.kube/notification-e2e-config`

### Verification

```
E2E Test Run: SUCCESS
Kubeconfig Location: /Users/jgil/.kube/notification-e2e-config
Test Result: 1/1 Passed (100.7 seconds)
Cleanup: Cluster deleted, kubeconfig removed
```

### Minor Correction

**Note**: The referenced "Kubeconfig Isolation Policy section" in `TESTING_GUIDELINES.md` does not exist. However, the pattern `{service}-e2e-config` is the de facto standard used by:
- RemediationOrchestrator
- AIAnalysis
- Gateway
- DataStorage

The Notification service now follows this established pattern.

---

## 🔗 Related Documents

- `docs/development/business-requirements/TESTING_GUIDELINES.md` (authoritative)

