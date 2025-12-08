# REQUEST: Dynamic Toolset Team - Kubeconfig Location Standardization

**Date**: December 8, 2025
**From**: RO Team (Architecture Coordination)
**To**: Dynamic Toolset Team
**Priority**: Medium
**Type**: Standardization Request

---

## 📋 Summary

The Dynamic Toolset E2E tests use a partially compliant kubeconfig location that doesn't follow the exact authoritative naming convention defined in `TESTING_GUIDELINES.md`.

---

## ⚠️ Current Implementation (Partial Compliance)

**File**: `test/e2e/toolset/toolset_e2e_suite_test.go`

```go
// Line 88
kubeconfigPath = fmt.Sprintf("%s/.kube/kind-toolset-config", homeDir)
```

**Problem**: Uses `kind-toolset-config` instead of the standard `toolset-e2e-config` pattern.

---

## ✅ Required Change (Authoritative Standard)

**Per** `docs/development/business-requirements/TESTING_GUIDELINES.md`:

```go
// CORRECT: Standard E2E config naming
kubeconfigPath = fmt.Sprintf("%s/.kube/toolset-e2e-config", homeDir)
```

**Also update the log message** (line 79):
```go
// FROM:
logger.Info("  • Kubeconfig: ~/.kube/kind-toolset-config")

// TO:
logger.Info("  • Kubeconfig: ~/.kube/toolset-e2e-config")
```

---

## 📐 Authoritative Standard

| Element | Standard |
|---------|----------|
| **Pattern** | `~/.kube/{service}-e2e-config` |
| **Toolset Location** | `~/.kube/toolset-e2e-config` |
| **Cluster Name** | `toolset-e2e` |

**Reference**: `docs/development/business-requirements/TESTING_GUIDELINES.md` (Kubeconfig Isolation Policy section)

---

## 🎯 Why This Matters

1. **Consistency**: All services use same naming pattern (`{service}-e2e-config`)
2. **Discoverability**: Easy to identify E2E kubeconfigs with `ls ~/.kube/*-e2e-config`
3. **Documentation**: Single pattern to document and maintain

---

## ✅ Compliance Checklist

After making changes:
- [ ] Kubeconfig path is `~/.kube/toolset-e2e-config`
- [ ] Log message reflects correct path
- [ ] Cluster name is `toolset-e2e`
- [ ] Tests pass with new kubeconfig location
- [ ] Old `~/.kube/kind-toolset-config` files cleaned up (if any)

---

## 📊 Current Compliance Status

| Service | Status |
|---------|--------|
| Gateway | ❌ Non-compliant (separate request) |
| AIAnalysis | ✅ Compliant |
| RO | ✅ Compliant |
| Data Storage | ✅ Compliant |
| Notification | ⚠️ Partial (separate request) |
| WorkflowExecution | ⚠️ Partial (separate request) |
| Toolset | ⚠️ Partial (`kind-toolset-config`) |

---

## 📝 Dynamic Toolset Team Response

**Status**: ⏳ Pending
**Acknowledged By**:
**Date**:
**Notes**:

---

## 🔗 Related Documents

- `docs/development/business-requirements/TESTING_GUIDELINES.md` (authoritative)
- `test/infrastructure/toolset.go` (infrastructure setup - also needs update)

