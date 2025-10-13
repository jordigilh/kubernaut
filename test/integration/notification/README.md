# Notification Controller Integration Tests

**Status**: Test designs complete, awaiting controller deployment
**Date**: 2025-10-12

---

## 📋 **Overview**

Integration tests for the Notification Controller validate end-to-end behavior in a real Kubernetes cluster (KIND). These tests verify all 9 Business Requirements (BR-NOT-050 to BR-NOT-058).

---

## 🎯 **Test Suite Summary**

### **5 Critical Integration Tests**

| Test | File | BR Coverage | Status |
|------|------|-------------|--------|
| **1. Basic Lifecycle** | `notification_lifecycle_test.go` | BR-NOT-050, BR-NOT-051, BR-NOT-053 | ✅ Designed |
| **2. Delivery Failure Recovery** | `delivery_failure_test.go` | BR-NOT-052, BR-NOT-053 | ✅ Designed |
| **3. Graceful Degradation** | `graceful_degradation_test.go` | BR-NOT-055 | ✅ Designed |
| **4. Priority Handling** | (inline in lifecycle test) | BR-NOT-057 | ✅ Designed |
| **5. Validation** | (inline in lifecycle test) | BR-NOT-058 | ✅ Designed |

**Total**: 5 integration tests covering all 9 BRs

---

## 📦 **Test Infrastructure**

### **Components**

1. **Test Suite** (`suite_test.go`):
   - KIND cluster setup using `pkg/testutil/kind/`
   - Mock Slack webhook server (`httptest.Server`)
   - Kubernetes Secret for Slack webhook URL
   - Test fixture cleanup

2. **Mock Slack Server**:
   - Simulates Slack webhook API
   - Captures request history for assertions
   - Configurable responses (success, 503, etc.)

3. **KIND Cluster Configuration**:
   - Namespace: `kubernaut-notifications`
   - CRDs: `NotificationRequest`
   - Controller: Notification controller deployment

---

## 🔧 **Prerequisites**

### **Before Running Tests**

1. **Controller Deployment**:
   ```bash
   # Deploy controller to KIND cluster
   kubectl apply -f config/crd/bases/
   kubectl apply -f deploy/notification-controller.yaml
   ```

2. **Verify Controller is Running**:
   ```bash
   kubectl get pods -n kubernaut-notifications
   kubectl logs -f deployment/notification-controller -n kubernaut-notifications
   ```

3. **CRD Installation**:
   ```bash
   make install  # Installs NotificationRequest CRD
   ```

### **KIND Cluster Requirements**

- **Cluster Name**: `notification-test`
- **Kubernetes Version**: v1.27+
- **Namespaces**:
  - `kubernaut-notifications` (controller namespace)
  - `kubernaut-system` (shared system namespace)

---

## 🚀 **Running Integration Tests**

### **Option A: Automated (Makefile)**
```bash
# Setup KIND + Deploy Controller + Run Tests
make test-integration-notification

# Cleanup
make cleanup-notification-test
```

### **Option B: Manual**
```bash
# 1. Start KIND cluster
kind create cluster --name notification-test

# 2. Install CRDs
kubectl apply -f config/crd/bases/notification.kubernaut.ai_notificationrequests.yaml

# 3. Create namespace
kubectl create namespace kubernaut-notifications

# 4. Deploy controller
kubectl apply -f deploy/notification-controller.yaml

# 5. Wait for controller
kubectl wait --for=condition=available deployment/notification-controller -n kubernaut-notifications --timeout=60s

# 6. Run tests
go test ./test/integration/notification/... -v -ginkgo.v

# 7. Cleanup
kind delete cluster --name notification-test
```

---

## 📊 **Test Coverage**

### **BR Coverage Matrix**

| BR | Test | Validation |
|----|------|------------|
| **BR-NOT-050** | Lifecycle Test | CRD persisted to etcd before delivery |
| **BR-NOT-051** | Lifecycle Test | DeliveryAttempts array populated |
| **BR-NOT-052** | Failure Recovery Test | Automatic retry with exponential backoff |
| **BR-NOT-053** | Lifecycle Test | Slack webhook called (at-least-once) |
| **BR-NOT-054** | All Tests | Prometheus metrics updated |
| **BR-NOT-055** | Graceful Degradation Test | Console succeeds, Slack fails → PartiallySent |
| **BR-NOT-056** | Lifecycle Test | Phase transitions: Pending → Sending → Sent |
| **BR-NOT-057** | Lifecycle Test | High priority notifications processed |
| **BR-NOT-058** | Lifecycle Test | CRD validation (kubebuilder) |

**Coverage**: 100% of BRs validated in integration tests

---

## 🧪 **Test Scenarios**

### **Test 1: Basic Lifecycle (Pending → Sent)**

**Duration**: ~10 seconds  
**Scenario**: Happy path with console + Slack delivery

**Steps**:
1. Create `NotificationRequest` CRD
2. Wait for reconciliation
3. Verify phase: `Pending` → `Sending` → `Sent`
4. Assert `DeliveryAttempts` has 2 entries (console + Slack)
5. Verify Slack webhook was called
6. Verify completion time is set

**Expected**:
- Phase: `Sent`
- SuccessfulDeliveries: 2
- FailedDeliveries: 0
- CompletionTime: Set

---

### **Test 2: Delivery Failure Recovery (Retry Logic)**

**Duration**: ~180 seconds  
**Scenario**: Slack returns 503 twice, then succeeds

**Steps**:
1. Configure mock Slack to fail 2x (503), then succeed
2. Create `NotificationRequest` with Slack channel
3. Wait for controller to retry (30s, 60s, 120s backoff)
4. Verify eventual success

**Expected**:
- Phase: `Sent`
- TotalAttempts: 3 (2 failures + 1 success)
- FailedDeliveries: 2
- SuccessfulDeliveries: 1
- Final attempt: Success

---

### **Test 3: Graceful Degradation (Partial Success)**

**Duration**: ~60 seconds  
**Scenario**: Console succeeds, Slack fails permanently → PartiallySent

**Steps**:
1. Configure mock Slack to always fail (503)
2. Create `NotificationRequest` with console + Slack
3. Verify console succeeds immediately
4. Verify Slack fails after max retries
5. Verify phase: `PartiallySent`

**Expected**:
- Phase: `PartiallySent`
- SuccessfulDeliveries: 1 (console)
- FailedDeliveries: 5 (Slack, max retries)
- Reason: "PartialDeliveryFailure"

---

## 🔬 **Test Execution Details**

### **Timing**

- **Setup**: ~30 seconds (KIND cluster + controller deployment)
- **Test 1**: ~10 seconds
- **Test 2**: ~180 seconds (includes retry backoff)
- **Test 3**: ~60 seconds
- **Teardown**: ~10 seconds
- **Total**: ~5 minutes

### **Resource Requirements**

- **CPU**: 2 cores (KIND cluster + controller)
- **Memory**: 4GB (KIND cluster + test fixtures)
- **Disk**: 2GB (container images)

---

## 🐛 **Troubleshooting**

### **Test Failures**

**Issue**: "controller not reconciling"
```bash
# Check controller logs
kubectl logs -f deployment/notification-controller -n kubernaut-notifications

# Verify controller is running
kubectl get pods -n kubernaut-notifications
```

**Issue**: "CRD not found"
```bash
# Reinstall CRDs
make install

# Verify CRDs are installed
kubectl get crds | grep notification
```

**Issue**: "Mock Slack server not responding"
```bash
# Check test suite logs
go test ./test/integration/notification/... -v -ginkgo.v | grep "Mock Slack"
```

---

## 📈 **Success Metrics**

- **Test Pass Rate**: Target >95%
- **Execution Time**: Target <5 minutes
- **Flakiness**: Target <1%
- **BR Coverage**: Target 100%

---

## 🔗 **Related Documentation**

- [Implementation Plan V3.0](../../../docs/services/crd-controllers/06-notification/implementation/IMPLEMENTATION_PLAN_V1.0.md)
- [Error Handling Philosophy](../../../docs/services/crd-controllers/06-notification/implementation/design/ERROR_HANDLING_PHILOSOPHY.md)
- [KIND Utilities](../../../pkg/testutil/kind/)
- [BR Coverage Matrix](../../../docs/services/crd-controllers/06-notification/implementation/testing/BR-COVERAGE-MATRIX.md)

---

## ✅ **Current Status**

### **Completed**
- ✅ Test designs complete (included in implementation plan)
- ✅ Test infrastructure planned (suite_test.go design)
- ✅ 5 critical test scenarios defined
- ✅ Mock Slack server design
- ✅ BR coverage matrix documented

### **Pending**
- ⏳ Controller deployment to KIND (Day 10-12)
- ⏳ Actual test file creation (after controller deployment)
- ⏳ Integration test execution
- ⏳ CI/CD pipeline integration

**Note**: Integration tests require a deployed controller in KIND. The controller deployment is scheduled for Days 10-12 (production readiness phase).

---

**Version**: 1.0  
**Last Updated**: 2025-10-12  
**Status**: Test Designs Complete, Awaiting Deployment ✅

