# E2E Kind Cluster Conversion - Notification Service

**Date**: November 30, 2025
**Status**: 🚧 **IN PROGRESS**
**Estimated Time**: 6-8 hours total

---

## 🎯 **Objective**

Convert notification E2E tests from **envtest** to **Kind cluster** infrastructure to match Gateway/DataStorage E2E patterns.

---

## 📋 **Implementation Checklist**

### **Phase 1: Infrastructure Setup** (~2-3 hours)

- [ ] **1.1** Create `test/infrastructure/notification.go` (~30 min)
  - `CreateNotificationCluster()` - Kind cluster creation
  - `DeleteNotificationCluster()` - Cleanup
  - `DeployNotificationController()` - Controller deployment
  - `WaitForNotificationController()` - Readiness check

- [ ] **1.2** Create `test/infrastructure/kind-notification-config.yaml` (~15 min)
  - 2 nodes (control-plane + worker)
  - NodePort mappings for controller
  - CRD installation

- [ ] **1.3** Create `test/e2e/notification/manifests/` (~45 min)
  - `notification-deployment.yaml` - Controller deployment
  - `notification-service.yaml` - Service (NodePort)
  - `notification-rbac.yaml` - ServiceAccount, Role, RoleBinding
  - `notification-configmap.yaml` - Controller configuration

- [ ] **1.4** Create notification controller Docker image build (~30 min)
  - Dockerfile for notification controller
  - Build + load into Kind cluster

### **Phase 2: E2E Suite Conversion** (~2-3 hours)

- [ ] **2.1** Rewrite `notification_e2e_suite_test.go` (~60 min)
  - Replace envtest with Kind cluster setup
  - Use `SynchronizedBeforeSuite` (like Gateway)
  - Create cluster ONCE on process 1
  - Deploy notification controller to Kind
  - Keep FileService for E2E validation
  - Setup per-process unique namespaces

- [ ] **2.2** Update test files to use Kind (~60 min)
  - `01_notification_lifecycle_audit_test.go`
  - `02_audit_correlation_test.go`
  - `03_file_delivery_validation_test.go`
  - `04_metrics_validation_test.go`
  - Ensure CRDs are created in Kind cluster
  - Ensure controller processes CRDs from Kind

- [ ] **2.3** Update FileService integration (~30 min)
  - Mount shared volume for FileService output
  - Or use controller logs for validation
  - Ensure E2E validation still works

### **Phase 3: Integration & Testing** (~1-2 hours)

- [ ] **3.1** Fix unit test failures (~30 min)
  - Fix flaky concurrent file delivery test
  - Ensure 140/140 passing

- [ ] **3.2** Fix integration test timeouts (~30 min)
  - Investigate why tests timeout at 30s
  - Fix or increase timeout appropriately

- [ ] **3.3** Run all E2E tests on Kind (~30 min)
  - Verify all 12 E2E tests pass on Kind
  - Fix any Kind-specific issues

### **Phase 4: CI/CD & Documentation** (~1 hour)

- [ ] **4.1** Restore Makefile E2E targets (~15 min)
  - `test-e2e-notification` target
  - Proper timeout (15-20 min for Kind startup)

- [ ] **4.2** Update CI/CD workflow (~15 min)
  - Change `infrastructure: none` → `infrastructure: kind`
  - Add E2E job back
  - Update timeout to 20-30 min

- [ ] **4.3** Update documentation (~30 min)
  - Correct test counts: 140 unit + 97 integration + 12 E2E
  - Document Kind cluster requirement
  - Update all session summaries

---

## 📊 **Current Status**

| Phase | Status | Time Spent |
|-------|--------|------------|
| **1. Infrastructure** | ⏸️ Not Started | 0h |
| **2. Suite Conversion** | ⏸️ Not Started | 0h |
| **3. Integration & Testing** | 🔄 Partial | ~1h |
| **4. CI/CD & Docs** | ⏸️ Not Started | 0h |

---

## 🔍 **Key Implementation Details**

### **1. Kind Cluster Setup Pattern**

```go
// Based on Gateway E2E pattern
var _ = SynchronizedBeforeSuite(
    // Process 1: Create cluster ONCE
    func() []byte {
        clusterName = "notification-e2e"
        kubeconfigPath = "~/.kube/notification-kubeconfig"

        // Create Kind cluster
        err := infrastructure.CreateNotificationCluster(clusterName, kubeconfigPath, GinkgoWriter)

        // Deploy notification controller
        err = infrastructure.DeployNotificationController(ctx, namespace, kubeconfigPath, GinkgoWriter)

        // Wait for controller ready
        err = infrastructure.WaitForNotificationController(ctx, namespace, kubeconfigPath)

        return []byte(kubeconfigPath)
    },
    // All processes: Setup per-process resources
    func(data []byte) {
        kubeconfigPath = string(data)
        // Create unique namespace per process
        testNamespace = fmt.Sprintf("notif-e2e-proc-%d", GinkgoParallelProcess())
    },
)
```

### **2. FileService Strategy**

**Option A**: Mount shared volume for file output
```yaml
# In notification-deployment.yaml
volumes:
  - name: e2e-output
    hostPath:
      path: /tmp/kubernaut-e2e-notifications
volumeMounts:
  - name: e2e-output
    mountPath: /tmp/kubernaut-e2e-notifications
```

**Option B**: Read from controller logs (simpler)
```go
// In tests
logs := getControllerLogs(ctx, namespace, "notification-controller")
Expect(logs).To(ContainSubstring("Delivered notification"))
```

**Recommended**: Option A (matches current FileService design)

### **3. Test Validation Flow**

```
1. Create NotificationRequest CRD in Kind cluster
2. Controller (running in Kind pod) processes CRD
3. Controller delivers via ConsoleService + FileService
4. FileService writes to mounted volume
5. Test reads file from shared volume
6. Test validates file content (current E2E behavior)
```

---

## 🚨 **Blocking Issues**

### **Issue 1: Unit Test Flakiness**
**Status**: 🔴 **BLOCKING**
**Test**: `should create unique files for concurrent deliveries`
**Cause**: Microsecond timestamp collisions on fast machines
**Fix**: Added 1ms delay between goroutines (needs verification)

### **Issue 2: Integration Test Timeouts**
**Status**: 🟡 **INVESTIGATING**
**Tests**: Integration suite times out at 30s
**Cause**: Unknown - needs RCA
**Next**: Run with increased timeout and investigate slowness

---

## 📈 **Expected Outcomes**

### **Before** (Current - Incorrect)
- Unit: 140 tests (envtest ❌)
- Integration: 97 tests (envtest)
- E2E: 0 tests (deleted by mistake)

### **After** (Correct)
- Unit: 140 tests (no infrastructure)
- Integration: 97 tests (envtest)
- E2E: 12 tests (**Kind cluster** ✅)

---

## 🎯 **Success Criteria**

- [ ] Kind cluster creates successfully
- [ ] Notification controller deploys to Kind
- [ ] All 12 E2E tests pass on Kind (100%)
- [ ] All 97 integration tests pass (100%)
- [ ] All 140 unit tests pass (100%)
- [ ] CI/CD runs E2E tests with Kind
- [ ] Documentation updated
- [ ] Total: 249/249 tests passing (100%)

---

## 🔗 **Reference Files**

### **Gateway E2E** (Reference Pattern)
- `test/e2e/gateway/gateway_e2e_suite_test.go` - Kind setup
- `test/infrastructure/gateway.go` - Infrastructure code
- `test/infrastructure/kind-gateway-config.yaml` - Kind config

### **Notification E2E** (To Convert)
- `test/e2e/notification/notification_e2e_suite_test.go` - Currently envtest
- `test/e2e/notification/01_notification_lifecycle_audit_test.go`
- `test/e2e/notification/02_audit_correlation_test.go`
- `test/e2e/notification/03_file_delivery_validation_test.go`
- `test/e2e/notification/04_metrics_validation_test.go`

---

## ⏱️ **Time Estimates**

| Task | Estimated | Actual |
|------|-----------|--------|
| Infrastructure files | 2h | - |
| Suite conversion | 2h | - |
| Test fixes | 1h | 1h (partial) |
| CI/CD & docs | 1h | - |
| **TOTAL** | **6h** | **1h** |

---

## 📝 **Next Session Tasks**

When resuming:

1. **Start with Phase 1.1**: Create `test/infrastructure/notification.go`
2. **Reference**: Copy patterns from `test/infrastructure/gateway.go`
3. **Test incrementally**: Build Kind cluster first, then add controller
4. **Validate**: Ensure controller starts and processes CRDs

---

**Status**: Ready to implement. User going to sleep. Will continue in next session.


