# REQUEST: WorkflowExecution Team - Kubeconfig Location Standardization

**Date**: December 8, 2025
**From**: RO Team (Architecture Coordination)
**To**: WorkflowExecution Team
**Priority**: Medium
**Type**: Standardization Request

---

## 📋 Summary

The WorkflowExecution E2E tests use a partially compliant kubeconfig location that doesn't follow the exact authoritative naming convention defined in `TESTING_GUIDELINES.md`.

---

## ⚠️ Current Implementation (Partial Compliance)

**File**: `test/e2e/workflowexecution/workflowexecution_e2e_suite_test.go`

```go
// Line 116
kubeconfigPath = fmt.Sprintf("%s/.kube/kind-%s", homeDir, clusterName)
// Results in: ~/.kube/kind-workflowexecution-e2e
```

**Problem**: Uses `kind-{clustername}` pattern instead of the standard `{service}-e2e-config` pattern.

---

## ✅ Required Change (Authoritative Standard)

**Per** `docs/development/business-requirements/TESTING_GUIDELINES.md`:

```go
// CORRECT: Standard E2E config naming
kubeconfigPath = fmt.Sprintf("%s/.kube/workflowexecution-e2e-config", homeDir)
```

**Also update the comment** (line 114-115):
```go
// FROM:
// Standard kubeconfig location: ~/.kube/kind-{clustername}

// TO:
// Standard kubeconfig location: ~/.kube/{service}-e2e-config
// Per docs/development/business-requirements/TESTING_GUIDELINES.md
```

---

## 📐 Authoritative Standard

| Element | Standard |
|---------|----------|
| **Pattern** | `~/.kube/{service}-e2e-config` |
| **WorkflowExecution Location** | `~/.kube/workflowexecution-e2e-config` |
| **Cluster Name** | `workflowexecution-e2e` |

**Reference**: `docs/development/business-requirements/TESTING_GUIDELINES.md` (Kubeconfig Isolation Policy section)

---

## 🎯 Why This Matters

1. **Consistency**: All services use same naming pattern (`{service}-e2e-config`)
2. **Discoverability**: Easy to identify E2E kubeconfigs with `ls ~/.kube/*-e2e-config`
3. **Documentation**: Single pattern to document and maintain

---

## ✅ Compliance Checklist

After making changes:
- [x] Kubeconfig path is `~/.kube/workflowexecution-e2e-config`
- [x] Comment reflects correct pattern
- [x] Cluster name is `workflowexecution-e2e`
- [ ] Tests pass with new kubeconfig location (requires E2E run)
- [ ] Old `~/.kube/kind-workflowexecution-e2e` files cleaned up (if any)

---

## 📊 Current Compliance Status

| Service | Status |
|---------|--------|
| Gateway | ❌ Non-compliant (separate request) |
| AIAnalysis | ✅ Compliant |
| RO | ✅ Compliant |
| Data Storage | ✅ Compliant |
| Notification | ⚠️ Partial (separate request) |
| WorkflowExecution | ✅ **Compliant** (`workflowexecution-e2e-config`) |
| Toolset | ⚠️ Partial (separate request) |

---

## 📝 WorkflowExecution Team Response

**Status**: ✅ **IMPLEMENTED**
**Acknowledged By**: WE Team
**Date**: December 8, 2025
**Notes**:

### Changes Made

1. **Updated `test/e2e/workflowexecution/workflowexecution_e2e_suite_test.go`**:
   - Changed kubeconfig path from `~/.kube/kind-workflowexecution-e2e` to `~/.kube/workflowexecution-e2e-config`
   - Updated comment to reference the standardization request

2. **Verification**:
   - ✅ E2E test suite compiles
   - ✅ Unit tests pass (173/173)
   - ✅ Integration tests pass (41/41)
   - ⏳ E2E test run pending (requires Kind cluster)

### Implementation Details

```go
// test/e2e/workflowexecution/workflowexecution_e2e_suite_test.go
// Standard kubeconfig location: ~/.kube/{service}-e2e-config
// Per RO team kubeconfig standardization (REQUEST_WORKFLOWEXECUTION_KUBECONFIG_STANDARDIZATION.md)
kubeconfigPath = fmt.Sprintf("%s/.kube/workflowexecution-e2e-config", homeDir)
```

### Cleanup Note

Users who previously ran WE E2E tests should clean up old kubeconfig files:
```bash
rm -f ~/.kube/kind-workflowexecution-e2e
rm -f ~/.kube/workflowexecution-kubeconfig
```

---

## 🔗 Related Documents

- `docs/development/business-requirements/TESTING_GUIDELINES.md` (authoritative)

