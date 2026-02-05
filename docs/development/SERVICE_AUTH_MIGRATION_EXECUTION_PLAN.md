# Service Authentication Migration Execution Plan

**Date**: 2026-01-27  
**Status**: Ready for Execution  
**Based on**: RemediationOrchestrator success (59/59 tests passing)

---

## Executive Summary

All 4 remaining services are **Kubernetes controllers** (not HTTP servers), which makes migration **SIMPLER** than expected:

**Key Insight**: Controllers don't need HTTP authentication middleware - they ONLY need authenticated clients for DataStorage API calls.

### Migration Scope Per Service

| Component | Changes Required | Complexity |
|-----------|------------------|------------|
| **Controller Code** | ✅ None (already uses audit store) | N/A |
| **HTTP Middleware** | ✅ Not applicable (controllers, not HTTP servers) | N/A |
| **Integration Tests** | 🔧 Update to use authenticated clients | Low |
| **Deployment Manifests** | 📝 Add client RBAC (NOT service RBAC) | Low |

**Estimated Time Per Service**: 1-2 hours (vs. 3-4 hours for HTTP services)

---

## Current Status Analysis

### Service Architecture

All 4 services are Kubernetes controllers that:
1. Watch CRDs (NotificationRequest, AIAnalysisRequest, etc.)
2. Reconcile resources
3. Write audit events to DataStorage via HTTP client
4. **Do NOT** accept incoming HTTP requests (no authentication middleware needed)

### Existing Audit Implementation

All services already have audit stores:
```go
// Current (unauthenticated)
dsClient, err := audit.NewOpenAPIClientAdapterWithTransport(...)
realAuditStore, err = audit.NewBufferedStore(dsClient, ...)
```

**Migration**: Replace with authenticated client (1 line change!)

---

## Migration Pattern (Simplified for Controllers)

### Before (Unauthenticated)

```go
// test/integration/<service>/suite_test.go
var _ = SynchronizedBeforeSuite(func() []byte {
    // ... envtest setup ...

    // ❌ Create UNAUTHENTICATED DataStorage client
    dsClient, err := audit.NewOpenAPIClientAdapterWithTransport(
        dataStorageBaseURL,
        http.DefaultTransport,
        5*time.Second,
    )
    
    realAuditStore, err = audit.NewBufferedStore(
        dsClient,
        audit.DefaultConfig(),
        ctrl.Log.WithName("audit"),
    )
    // ...
})
```

### After (Authenticated)

```go
// test/integration/<service>/suite_test.go
var _ = SynchronizedBeforeSuite(func() []byte {
    // ... envtest setup (force IPv4, create ServiceAccounts) ...

    // Serialize token for Phase 2
    return []byte(fmt.Sprintf("%s:%s", cfg.Host, clientToken))
}, func(data []byte) {
    // Phase 2: Parse token and create authenticated clients
    parts := strings.Split(string(data), ":")
    clientToken := parts[1]

    // ✅ Create AUTHENTICATED DataStorage clients
    dsClients = integration.NewAuthenticatedDataStorageClients(
        dataStorageBaseURL,
        clientToken,
        5*time.Second,
    )

    // Create audit store with authenticated client
    realAuditStore, err = audit.NewBufferedStore(
        dsClients.AuditClient, // ← Automatically authenticated!
        audit.DefaultConfig(),
        ctrl.Log.WithName("audit"),
    )
    // ...
})
```

---

## Service-Specific Details

### 1. NotificationController

**Priority**: 1 (Highest - critical for SOC2 notification delivery attribution)

**Complexity**: Low  
**Estimated Time**: 1.5 hours  
**Test Count**: 21 integration tests

**Migration Steps**:
1. Update `test/integration/notification/suite_test.go`:
   - Add Phase 1: Force IPv4, create ServiceAccounts, bootstrap DataStorage
   - Add Phase 2: Create authenticated clients, update audit store creation
2. Create `test/shared/integration/notification_auth.go` (if needed for service-specific clients)
3. Add `deploy/notification/client-rbac.yaml` (client ServiceAccount + RBAC)
4. Run tests: `make test-integration-notification`

**Expected Result**: 21/21 tests passing

**Service-Specific Notes**:
- Uses `notificationaudit.Manager` for audit events
- Audit events: `notification.message.sent`, `notification.message.failed`, etc.

---

### 2. WorkflowExecution

**Priority**: 2 (High - workflow execution attribution)

**Complexity**: Low  
**Estimated Time**: 1.5 hours  
**Test Count**: 13 integration tests

**Migration Steps**:
1. Update `test/integration/workflowexecution/suite_test.go`:
   - Add Phase 1: Force IPv4, create ServiceAccounts, bootstrap DataStorage
   - Add Phase 2: Create authenticated clients, update audit store creation
2. Create `test/shared/integration/workflowexecution_auth.go` (if needed)
3. Add `deploy/workflowexecution/client-rbac.yaml`
4. Run tests: `make test-integration-workflowexecution`

**Expected Result**: 13/13 tests passing

**Service-Specific Notes**:
- Uses `workflowexecution.Manager` for audit events
- Audit events: `workflowexecution.workflow.started`, `workflowexecution.workflow.completed`, etc.

---

### 3. AIAnalysis

**Priority**: 3 (Medium - AI decision attribution)

**Complexity**: Low  
**Estimated Time**: 1.5 hours  
**Test Count**: 12 integration tests

**Migration Steps**:
1. Update `test/integration/aianalysis/suite_test.go`:
   - Add Phase 1: Force IPv4, create ServiceAccounts, bootstrap DataStorage
   - Add Phase 2: Create authenticated clients, update audit store creation
2. Create `test/shared/integration/aianalysis_auth.go` (if needed)
3. Add `deploy/aianalysis/client-rbac.yaml`
4. Run tests: `make test-integration-aianalysis`

**Expected Result**: 12/12 tests passing

**Service-Specific Notes**:
- Uses `aianalysis.Manager` for audit events
- Audit events: `aianalysis.analysis.completed`, `aianalysis.analysis.failed`, etc.

---

### 4. SignalProcessing

**Priority**: 4 (Medium - signal classification attribution)

**Complexity**: Low  
**Estimated Time**: 1.5 hours  
**Test Count**: 9 integration tests

**Migration Steps**:
1. Update `test/integration/signalprocessing/suite_test.go`:
   - Add Phase 1: Force IPv4, create ServiceAccounts, bootstrap DataStorage
   - Add Phase 2: Create authenticated clients, update audit store creation
2. Create `test/shared/integration/signalprocessing_auth.go` (if needed)
3. Add `deploy/signalprocessing/client-rbac.yaml`
4. Run tests: `make test-integration-signalprocessing`

**Expected Result**: 9/9 tests passing

**Service-Specific Notes**:
- Uses `signalprocessing.Manager` for audit events
- Audit events: `signalprocessing.signal.processed`, `signalprocessing.classification.decision`, etc.

---

## Detailed Step-by-Step Guide (Per Service)

### Step 1: Update suite_test.go (1 hour)

#### A. Add Phase 1 (SynchronizedBeforeSuite First Function)

```go
var _ = SynchronizedBeforeSuite(func() []byte {
    // CRITICAL: Force IPv4 binding (DD-TEST-012)
    sharedTestEnv := &envtest.Environment{
        CRDDirectoryPaths: []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
        ControlPlane: envtest.ControlPlane{
            APIServer: &envtest.APIServer{
                SecureServing: envtest.SecureServing{
                    ListenAddr: envtest.ListenAddr{
                        Address: "127.0.0.1", // Force IPv4, NOT "localhost"
                    },
                },
            },
        },
    }

    cfg, err := sharedTestEnv.Start()
    Expect(err).NotTo(HaveOccurred())

    k8sClient, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
    Expect(err).NotTo(HaveOccurred())

    // Create DataStorage service ServiceAccount + RBAC
    // (Reuse existing infrastructure.CreateServiceAccountWithToken from RO)
    datastorageToken, err := infrastructure.CreateServiceAccountWithToken(
        ctx, k8sClient, cfg, "default", "datastorage-service",
        "datastorage-tokenreview",
        []rbacv1.PolicyRule{
            {
                APIGroups: []string{"authentication.k8s.io"},
                Resources: []string{"tokenreviews"},
                Verbs:     []string{"create"},
            },
            {
                APIGroups: []string{"authorization.k8s.io"},
                Resources: []string{"subjectaccessreviews"},
                Verbs:     []string{"create"},
            },
        },
    )
    Expect(err).NotTo(HaveOccurred())

    // Create client ServiceAccount + RBAC
    clientToken, err := infrastructure.CreateServiceAccountWithToken(
        ctx, k8sClient, cfg, "default", "<service>-client",
        "<service>-client",
        []rbacv1.PolicyRule{
            {
                APIGroups: []string{""},
                Resources: []string{"events"},
                Verbs:     []string{"create", "get", "list"},
            },
            // WORKAROUND: envtest TokenReview quirk
            {
                APIGroups: []string{"authentication.k8s.io"},
                Resources: []string{"tokenreviews"},
                Verbs:     []string{"create"},
            },
        },
    )
    Expect(err).NotTo(HaveOccurred())

    // Generate kubeconfig for DataStorage
    kubeconfigPath, err := infrastructure.GenerateKubeconfigForPodman(
        cfg, datastorageToken, "datastorage-service",
    )
    Expect(err).NotTo(HaveOccurred())

    // Bootstrap DataStorage container
    err = infrastructure.BootstrapDataStorage(ctx, kubeconfigPath)
    Expect(err).NotTo(HaveOccurred())

    // Serialize for Phase 2
    return []byte(fmt.Sprintf("%s:%s", cfg.Host, clientToken))
}, func(data []byte) {
    // Phase 2: Create authenticated clients
    parts := strings.Split(string(data), ":")
    clientToken := parts[1]

    // Create authenticated DataStorage clients
    dsClients = integration.NewAuthenticatedDataStorageClients(
        dataStorageBaseURL,
        clientToken,
        5*time.Second,
    )

    // Create audit store with authenticated client
    realAuditStore, err = audit.NewBufferedStore(
        dsClients.AuditClient, // ← Authenticated!
        audit.DefaultConfig(),
        ctrl.Log.WithName("audit"),
    )
    Expect(err).NotTo(HaveOccurred())

    // ... rest of setup ...
})
```

#### B. Update Global Variables

```go
var (
    // ... existing vars ...

    // DD-AUTH-014: Authenticated DataStorage clients (audit + OpenAPI with ServiceAccount tokens)
    dsClients *integration.AuthenticatedDataStorageClients
)
```

#### C. Remove Unauthenticated Client Creation

```diff
- // OLD: Unauthenticated client
- dsClient, err := audit.NewOpenAPIClientAdapterWithTransport(
-     dataStorageBaseURL,
-     http.DefaultTransport,
-     5*time.Second,
- )
- realAuditStore, err = audit.NewBufferedStore(
-     dsClient,
-     audit.DefaultConfig(),
-     ctrl.Log.WithName("audit"),
- )

+ // NEW: Authenticated client (from Phase 2 above)
+ realAuditStore, err = audit.NewBufferedStore(
+     dsClients.AuditClient, // Already created in Phase 2
+     audit.DefaultConfig(),
+     ctrl.Log.WithName("audit"),
+ )
```

### Step 2: Add Deployment RBAC (15 minutes)

Create `deploy/<service>/client-rbac.yaml`:

```yaml
# Client ServiceAccount for <service> integration tests
apiVersion: v1
kind: ServiceAccount
metadata:
  name: <service>-client
  namespace: kubernaut
---
# ClusterRole for client permissions
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: <service>-client
rules:
# Service-specific API permissions
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "get", "list", "watch"]
# WORKAROUND: envtest TokenReview quirk
- apiGroups: ["authentication.k8s.io"]
  resources: ["tokenreviews"]
  verbs: ["create"]
---
# ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: <service>-client
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: <service>-client
subjects:
- kind: ServiceAccount
  name: <service>-client
  namespace: kubernaut
```

### Step 3: Test & Validate (15 minutes)

```bash
# Build
go build ./test/integration/<service>/...

# Run tests
make test-integration-<service>

# Expected output:
# ✅ Ran N of N Specs in X seconds
# ✅ SUCCESS! -- N Passed | 0 Failed | 0 Pending | 0 Skipped

# Verify authentication logs
grep "Token validated successfully" /tmp/<service>-integration.log
```

---

## Timeline & Resource Allocation

### Parallel Execution (Recommended)

**Total Duration**: 2-3 hours (if executed in parallel)

| Service | Developer | Start Time | End Time | Status |
|---------|-----------|------------|----------|--------|
| NotificationController | Dev 1 | T+0h | T+1.5h | ⏳ |
| WorkflowExecution | Dev 2 | T+0h | T+1.5h | ⏳ |
| AIAnalysis | Dev 1 | T+1.5h | T+3h | ⏳ |
| SignalProcessing | Dev 2 | T+1.5h | T+3h | ⏳ |

### Sequential Execution (Alternative)

**Total Duration**: 6 hours (if executed sequentially)

**Recommended Order**:
1. NotificationController (1.5h) - Highest priority
2. WorkflowExecution (1.5h) - High priority
3. AIAnalysis (1.5h) - Medium priority
4. SignalProcessing (1.5h) - Medium priority

---

## Success Criteria

### Per Service

- ✅ Integration tests pass with real K8s auth (X/X passing)
- ✅ Zero authentication failures (except explicit auth tests)
- ✅ Tests complete in <3 minutes
- ✅ Logs show "Token validated successfully" messages
- ✅ RBAC correctly configured (caught by integration tests)

### Overall

- ✅ All 4 services migrated
- ✅ E2E test pass rate improves (fewer auth failures)
- ✅ CI/CD pipeline faster (less time debugging auth issues)
- ✅ SOC2 compliance: All audit events have user attribution

---

## Risk Assessment

### Low Risk Items (Controllers)

**Why Low Risk?**
1. Controllers don't need HTTP auth middleware (no code changes)
2. Only test infrastructure changes (suite_test.go)
3. Pattern proven with RemediationOrchestrator (59/59 passing)
4. Rollback is simple (revert suite_test.go changes)

**Mitigation**:
- Test each service independently
- Use RemediationOrchestrator as reference
- Validate locally before pushing to CI/CD

---

## Troubleshooting Quick Reference

### Issue: `connection refused` to `[::1]:PORT`

**Fix**: Force IPv4 binding in envtest config (see Step 1A)

### Issue: `tokenreviews.authentication.k8s.io is forbidden`

**Fix**: Add `tokenreviews` permission to client ServiceAccount (see Step 2)

### Issue: `401 Unauthorized` on all test queries

**Fix**: Ensure `dsClients.AuditClient` is used (not new unauthenticated client)

**Full Troubleshooting**: See DD-TEST-012 section "Troubleshooting Guide"

---

## Checklist Template (Copy for Each Service)

### Pre-Migration

- [ ] Review service architecture (controller structure)
- [ ] Identify all DataStorage API calls (audit writes, queries)
- [ ] Review existing integration test suite
- [ ] Backup current test files

### Implementation

#### Suite Test Update (1 hour)

- [ ] Add IPv4 binding to envtest config
- [ ] Create DataStorage service ServiceAccount + RBAC (Phase 1)
- [ ] Create client ServiceAccount + RBAC (Phase 1)
- [ ] Generate kubeconfig for Podman container (Phase 1)
- [ ] Bootstrap DataStorage with kubeconfig (Phase 1)
- [ ] Serialize client token to Phase 2 (Phase 1)
- [ ] Create authenticated clients using helpers (Phase 2)
- [ ] Update audit store creation with authenticated client (Phase 2)
- [ ] Remove old unauthenticated client creation

#### Deployment RBAC (15 minutes)

- [ ] Create `deploy/<service>/client-rbac.yaml`
- [ ] Define ServiceAccount
- [ ] Define ClusterRole with API permissions
- [ ] Define ClusterRoleBinding
- [ ] Add envtest TokenReview workaround

#### Validation (15 minutes)

- [ ] Build: `go build ./test/integration/<service>/...`
- [ ] Run tests: `make test-integration-<service>`
- [ ] Verify X/X tests passing
- [ ] Check logs for "Token validated successfully"
- [ ] Verify tests complete in <3 minutes
- [ ] Check for auth errors (should be 0)

### Post-Migration

- [ ] Update service README with auth architecture
- [ ] Document any service-specific auth requirements
- [ ] Commit changes with clear message
- [ ] Update this execution plan with lessons learned

---

## Next Actions

1. **Review this plan** with team
2. **Allocate resources** (1-2 developers for parallel execution)
3. **Schedule migration window** (2-3 hours for all services)
4. **Start with NotificationController** (highest priority, 1.5 hours)
5. **Document lessons learned** after each service

---

## References

- **DD-AUTH-014**: Middleware-based SAR authentication (design decision)
- **DD-TEST-012**: envtest real authentication pattern (testing methodology)
- **Reference Implementation**: RemediationOrchestrator
  - `test/integration/remediationorchestrator/suite_test.go` (lines 120-380)
  - `test/shared/integration/datastorage_auth.go`
  - `test/infrastructure/serviceaccount.go`
- **Troubleshooting**: `docs/handoff/DD_AUTH_014_ENVTEST_IPV6_BLOCKER.md`

---

**Status**: ✅ Ready for Execution  
**Estimated Completion**: 2-3 hours (parallel) | 6 hours (sequential)  
**Confidence**: High (proven pattern with RemediationOrchestrator)
