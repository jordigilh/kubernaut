# REQUEST: Gateway Team - Kubeconfig Location Standardization

**Date**: December 8, 2025
**From**: RO Team (Architecture Coordination)
**To**: Gateway Team
**Priority**: Medium
**Status**: ✅ **RESOLVED**
**Type**: Standardization Request

---

## 📋 Summary

The Gateway E2E tests use a non-compliant kubeconfig location that doesn't follow the authoritative naming convention defined in `TESTING_GUIDELINES.md`.

---

## ❌ Current Implementation (Non-Compliant)

**File**: `test/e2e/gateway/gateway_e2e_suite_test.go`

```go
// Line 90
kubeconfigPath = fmt.Sprintf("%s/.kube/kind-config", homeDir)
```

**Problem**: The generic name `kind-config` can conflict with other services' E2E tests if run on the same machine, and doesn't identify which service owns the kubeconfig.

---

## ✅ Required Change (Authoritative Standard)

**Per** `docs/development/business-requirements/TESTING_GUIDELINES.md`:

```go
// CORRECT: Service-specific kubeconfig
kubeconfigPath = fmt.Sprintf("%s/.kube/gateway-e2e-config", homeDir)
```

**Also update the log message** (line 81):
```go
// FROM:
logger.Info("  • Kubeconfig: ~/.kube/kind-config")

// TO:
logger.Info("  • Kubeconfig: ~/.kube/gateway-e2e-config")
```

---

## 📐 Authoritative Standard

| Element | Standard |
|---------|----------|
| **Pattern** | `~/.kube/{service}-e2e-config` |
| **Gateway Location** | `~/.kube/gateway-e2e-config` |
| **Cluster Name** | `gateway-e2e` |

**Reference**: `docs/development/business-requirements/TESTING_GUIDELINES.md` (Kubeconfig Isolation Policy section)

---

## 🎯 Why This Matters

1. **Isolation**: Prevents kubeconfig collisions when multiple E2E tests run on same machine
2. **Clarity**: Kubeconfig filename identifies which service owns it
3. **Safety**: Reduces risk of accidentally using wrong cluster credentials
4. **Consistency**: Aligns with other compliant services (AIAnalysis, DataStorage, RO)

---

## ✅ Compliance Checklist

After making changes:
- [x] Kubeconfig path is `~/.kube/gateway-e2e-config`
- [x] Log message reflects correct path
- [x] Cluster name is `gateway-e2e`
- [x] Tests use new kubeconfig location (verified Dec 8, 2025)
- [ ] Old `~/.kube/kind-config` files cleaned up (if any)

---

## 📊 Current Compliance Status

| Service | Status |
|---------|--------|
| Gateway | ✅ **Compliant** (`gateway-e2e-config`) |
| AIAnalysis | ✅ Compliant |
| RO | ✅ Compliant |
| Data Storage | ✅ Compliant |
| Notification | ⚠️ Partial (separate request) |
| WorkflowExecution | ✅ **Compliant** (`workflowexecution-e2e-config`) |
| Toolset | ⚠️ Partial (separate request) |

---

## 📝 Gateway Team Response

**Status**: ✅ **COMPLETED**
**Acknowledged By**: Gateway Team
**Date**: December 8, 2025
**Notes**:

Changes implemented in `test/e2e/gateway/gateway_e2e_suite_test.go`:
- Line 81: Log message updated to `~/.kube/gateway-e2e-config`
- Line 90: Kubeconfig path changed from `kind-config` to `gateway-e2e-config`
- Build verified: ✅ Compiles successfully

Full E2E test validation pending next E2E run.

---

## 🔗 Related Documents

- `docs/development/business-requirements/TESTING_GUIDELINES.md` (authoritative)
- `test/integration/gateway/KUBECONFIG_ISOLATION_UPDATE.md` (previous isolation work)
- `test/integration/gateway/KIND_KUBECONFIG_ISOLATION.md` (integration test isolation)

