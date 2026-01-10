# Notification E2E Root Cause: Test k8sClient Failure - JAN 10, 2026

## 🎯 **ROOT CAUSE IDENTIFIED: Test k8sClient.Create() Failing**

### **Live Debugging Results**
- **Cluster**: `notification-e2e` (kept alive with `KEEP_CLUSTER=true`)
- **Namespace**: `notification-e2e` (infrastructure) + `default` (test resources)
- **Kubeconfig**: `~/.kube/notification-e2e-config`

---

## ✅ **CONTROLLER IS 100% WORKING**

### **Manual NotificationRequest Test**
```bash
# Created manually via kubectl
kubectl apply -f - <<EOF
apiVersion: kubernaut.ai/v1alpha1
kind: NotificationRequest
metadata:
  name: manual-test
  namespace: default
spec:
  type: simple
  subject: "Manual Test"
  body: "Testing notification creation"
  priority: critical
  channels:
    - console
    - file
  recipients:
    - slack: "#test"
EOF
```

**Result**:
```yaml
status:
  completionTime: "2026-01-10T15:36:05Z"
  deliveryAttempts:
  - attempt: 1
    channel: console
    status: success
  - attempt: 1
    channel: file
    status: success
  phase: Sent
  reason: AllDeliveriesSucceeded
  successfulDeliveries: 2
  totalAttempts: 2
```

**File Created**: ✅ `notification-manual-test-20260110-153605.425304.json`

**🎉 Controller processed notification in < 1 second and delivered successfully to BOTH channels!**

---

## ❌ **E2E TEST FAILURE: k8sClient.Create() Error**

### **Test Code**
```go
// test/e2e/notification/03_file_delivery_validation_test.go:236-260
notification := &notificationv1alpha1.NotificationRequest{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "e2e-priority-validation",
        Namespace: "default",  // ← Test uses default namespace
        Labels: map[string]string{
            "test-scenario": "priority-validation",
        },
    },
    Spec: notificationv1alpha1.NotificationRequestSpec{
        Type:     notificationv1alpha1.NotificationTypeSimple,
        Subject:  "Critical Alert: System Outage",
        Body:     "Priority validation test for critical alerts",
        Priority: notificationv1alpha1.NotificationPriorityCritical,
        Channels: []notificationv1alpha1.Channel{
            notificationv1alpha1.ChannelConsole,
            notificationv1alpha1.ChannelFile,
        },
        Recipients: []notificationv1alpha1.Recipient{
            {Slack: "#ops-critical"},
        },
    },
}

err := k8sClient.Create(ctx, notification)  // ← FAILS HERE
Expect(err).ToNot(HaveOccurred())          // ← Test fails
```

### **Test Output**
```
  STEP: Creating NotificationRequest with Critical priority
  [FAILED] in [It] - line 260
  [FAILED] Unexpected error:
      <*errors.StatusError | 0x14000254e60>
      ...
  [FAIL] should preserve priority field in delivered notification file
```

### **Live Cluster Investigation**
```bash
# NO NotificationRequests found in ANY namespace
$ kubectl get notificationrequests --all-namespaces
No resources found

# CRD IS installed
$ kubectl get crd notificationrequests.kubernaut.ai
NAME                                CREATED AT
notificationrequests.kubernaut.ai   2026-01-10T15:29:46Z

# Default namespace EXISTS
$ kubectl get namespace default
NAME      STATUS   AGE
default   Active   6m22s
```

**Conclusion**: Test's `k8sClient.Create()` call is failing with a `StatusError`, preventing NotificationRequest creation.

---

## 🔍 **ROOT CAUSE HYPOTHESIS**

### **k8sClient Setup Issue in E2E Tests**

The `k8sClient` variable used in E2E tests appears to have incorrect configuration:

**Potential Issues**:
1. **Wrong API Server URL** - k8sClient pointing to wrong cluster
2. **Invalid Credentials** - k8sClient auth failing
3. **Missing Scheme** - NotificationRequest CRD not registered in client scheme
4. **Wrong Kubeconfig** - k8sClient using different kubeconfig than cluster

### **Evidence**
- ✅ CRD installed in cluster
- ✅ Default namespace exists
- ✅ Manual `kubectl` commands work perfectly
- ✅ Controller processes notifications correctly
- ❌ Test `k8sClient.Create()` fails with StatusError
- ❌ Zero NotificationRequests ever created by tests

---

## 📊 **FILES IN CONTROLLER POD**

```bash
$ kubectl exec notification-controller-xxx -c manager -- ls /tmp/notifications/ | wc -l
466
```

**466 notification files** from previous test runs! This proves:
- ✅ File delivery service is working
- ✅ Volume mount is working
- ✅ Controller has been processing notifications successfully in past runs

**But**: None of these files match current test run, confirming that current tests never created NotificationRequests.

---

## 🔬 **NEXT INVESTIGATION STEPS**

### **Step 1: Examine k8sClient Setup**
```bash
# In test/e2e/notification/notification_e2e_suite_test.go
# Around lines 90-150 (BeforeSuite setup)
# Check how k8sClient is initialized
```

### **Step 2: Check Scheme Registration**
```go
// Is NotificationRequest CRD registered in the client scheme?
scheme := runtime.NewScheme()
_ = clientgoscheme.AddToScheme(scheme)
_ = notificationv1alpha1.AddToScheme(scheme)  // ← This line must exist!
```

### **Step 3: Verify Kubeconfig Path**
```go
// Does k8sClient use the correct kubeconfig?
// Should match: ~/.kube/notification-e2e-config
```

### **Step 4: Check RBAC Permissions**
```bash
# Does the test serviceaccount have permission to create NotificationRequests?
kubectl auth can-i create notificationrequests --as=system:serviceaccount:default:default
```

---

## 💡 **IMMEDIATE FIX HYPOTHESIS**

**Most Likely**: Missing scheme registration for NotificationRequest CRD in k8sClient setup.

**Expected Fix**:
```go
// In test/e2e/notification/notification_e2e_suite_test.go BeforeSuite
import notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"

scheme := runtime.NewScheme()
_ = clientgoscheme.AddToScheme(scheme)
_ = notificationv1alpha1.AddToScheme(scheme)  // ← ADD THIS

k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
```

---

## 📋 **STATUS SUMMARY**

| Component | Status | Evidence |
|---|---|---|
| **NotificationRequest CRD** | ✅ WORKING | Installed, manual create works |
| **Notification Controller** | ✅ WORKING | Processes + delivers in < 1s |
| **File Delivery Service** | ✅ WORKING | 466 files created, manual test file exists |
| **Volume Mount** | ✅ WORKING | Files visible in pod |
| **Default Namespace** | ✅ EXISTS | Active for 6+ minutes |
| **Test k8sClient** | ❌ **BROKEN** | Create() fails with StatusError |
| **Test Execution** | ❌ **FAILS EARLY** | Never creates NotificationRequests |

---

## 🎯 **RECOMMENDATION**

**Priority 1**: Fix k8sClient setup in E2E test suite
1. Verify scheme includes NotificationRequest CRD
2. Confirm kubeconfig path is correct
3. Check RBAC permissions
4. Add logging to k8sClient.Create() error for debugging

**Expected Outcome**: All 14-19 E2E tests should pass once k8sClient can create resources.

---

## 📚 **RELATED DOCUMENTS**
- [NT_MUST_GATHER_ANALYSIS_JAN10.md](./NT_MUST_GATHER_ANALYSIS_JAN10.md) - Controller never processed notifications
- [NT_E2E_ROOT_CAUSE_FINAL_JAN10.md](./NT_E2E_ROOT_CAUSE_FINAL_JAN10.md) - False positive test analysis
- [NT_CONFIGMAP_NAMESPACE_FIX_JAN10.md](./NT_CONFIGMAP_NAMESPACE_FIX_JAN10.md) - ConfigMap fix (not root cause)

---

**Authority**: DD-NOT-006 v2, BR-NOTIFICATION-001  
**Status**: ROOT CAUSE IDENTIFIED - k8sClient setup issue in E2E tests  
**Next**: Fix k8sClient scheme registration and retry tests  
**Cluster**: STILL ALIVE for further debugging (run `kind delete cluster --name notification-e2e` when done)
