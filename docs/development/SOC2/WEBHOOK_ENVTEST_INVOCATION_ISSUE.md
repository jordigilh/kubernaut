# Webhook envtest Invocation Issue - Diagnostic Report

**Date**: January 6, 2026
**Status**: 🔍 **Under Investigation**
**Test Results**: **7/9 Passing** (78%)
**Issue**: Webhooks not being invoked by envtest for DELETE operations
**Authority**: DD-WEBHOOK-003, DD-TESTING-001

---

## 🎯 **Executive Summary**

**Problem**: NotificationRequest DELETE webhook is correctly configured but NOT being invoked by envtest, causing 2 integration tests to fail.

**Evidence**:
- ✅ Webhook handler registered on correct path (`/validate-notificationrequest-delete`)
- ✅ ValidatingWebhookConfiguration manifest exists (`config/webhook/manifests.yaml`)
- ✅ envtest WebhookInstallOptions enabled
- ✅ Audit store functional (works for other webhooks)
- ❌ **NO webhook invocation logs** (webhook never receives requests)
- ❌ **Tests timeout after 60s** waiting for audit events that never arrive

**Current Status**:
- WorkflowExecution webhook: 3/3 passing ✅ (UPDATE operations)
- RemediationApprovalRequest webhook: 2/2 passing ✅ (UPDATE operations)
- NotificationRequest webhook: 0/2 passing ❌ (DELETE operations)

---

## 🔍 **Detailed Investigation**

### **Configuration Verification**

#### ✅ **Webhook Manifest** (`config/webhook/manifests.yaml`)
```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: kubernaut-authwebhook-validating
webhooks:
  - name: notificationrequest.validate.kubernaut.ai
    admissionReviewVersions: ["v1"]
    clientConfig:
      service:
        name: authwebhook
        namespace: default
        path: /validate-notificationrequest-delete
    failurePolicy: Fail
    rules:
      - apiGroups: ["kubernaut.ai"]
        apiVersions: ["v1alpha1"]
        operations: ["DELETE"]  # ← Configured for DELETE
        resources: ["notificationrequests"]
```

**Status**: ✅ Correctly configured for DELETE operations

#### ✅ **envtest Configuration** (`suite_test.go`)
```go
testEnv = &envtest.Environment{
    CRDDirectoryPaths: []string{
        filepath.Join("..", "..", "..", "config", "crd", "bases"),
    },
    WebhookInstallOptions: envtest.WebhookInstallOptions{
        Paths: []string{filepath.Join("..", "..", "..", "config", "webhook")},
    },
}
```

**Status**: ✅ WebhookInstallOptions enabled and pointing to correct directory

#### ✅ **Webhook Handler Registration** (`suite_test.go`)
```go
nrHandler := webhooks.NewNotificationRequestDeleteHandler(auditStore)
_ = nrHandler.InjectDecoder(decoder)
webhookServer.Register("/validate-notificationrequest-delete", &webhook.Admission{Handler: nrHandler})
```

**Status**: ✅ Handler registered on correct path matching manifest

---

### **Test Execution Evidence**

#### ❌ **Missing Webhook Invocation Logs**
```bash
# Expected debug logs (from webhook handler):
🔍 DELETE webhook invoked: Operation=DELETE, Name=test-nr-cancel-xxx
✅ Unmarshaled NotificationRequest
✅ Authenticated user: admin
📝 Creating audit event for DELETE operation
💾 Storing audit event to Data Storage

# Actual logs: NONE (webhook never invoked)
```

**Conclusion**: envtest is NOT routing DELETE requests through the webhook

#### ✅ **Working Webhooks** (UPDATE operations)
- WorkflowExecution webhook: 3/3 passing (mutating webhook for UPDATE)
- RemediationApprovalRequest webhook: 2/2 passing (mutating webhook for UPDATE)

**Pattern**: Only DELETE operations fail, UPDATE operations work

---

## 🤔 **Possible Root Causes**

### **Hypothesis 1: envtest Limitation with DELETE Webhooks**
**Likelihood**: ⭐⭐⭐⭐ (High)

envtest may not fully support ValidatingWebhooks for DELETE operations:
- envtest is a lightweight testing framework, not a full K8s API server
- DELETE webhooks are more complex (no object mutation possible)
- envtest may only support webhooks for CREATE/UPDATE operations

**Evidence**:
- No webhook invocation logs
- UPDATE webhooks work, DELETE webhooks don't
- envtest documentation doesn't explicitly mention DELETE webhook support

**Test**:
```bash
# Run same tests in Kind cluster (real K8s)
make test-integration-authwebhook-kind
```

### **Hypothesis 2: envtest Webhook Registration Timing**
**Likelihood**: ⭐⭐ (Low)

Webhooks might not be fully registered when tests start:
- We added `time.Sleep(2 * time.Second)` after webhook server start
- But envtest might need more time or explicit confirmation

**Evidence**:
- Weak (all tests would fail if timing issue, but 7/9 pass)

**Test**:
```bash
# Increase sleep duration in suite_test.go
time.Sleep(10 * time.Second)  // Increase from 2s
```

### **Hypothesis 3: envtest Webhook Path Mismatch**
**Likelihood**: ⭐ (Very Low)

The webhook path in the manifest doesn't match the registered handler:
- Manifest: `/validate-notificationrequest-delete`
- Handler registration: `/validate-notificationrequest-delete`

**Evidence**:
- Paths match exactly
- No errors in envtest startup logs

**Status**: ❌ Ruled out (paths match)

### **Hypothesis 4: CRD Not Actually Created in envtest**
**Likelihood**: ⭐⭐ (Low)

NotificationRequest CRD might not be created properly, so DELETE has nothing to delete:
- Test calls `k8sClient.Delete(ctx, nr)` which succeeds
- But if CRD not actually in envtest, webhook wouldn't be invoked

**Evidence**:
- DELETE succeeds (no error)
- Other tests query CRDs successfully

**Test**:
```go
// In test, before DELETE:
var retrieved notificationv1.NotificationRequest
err := k8sClient.Get(ctx, client.ObjectKey{Name: nrName, Namespace: namespace}, &retrieved)
Expect(err).ToNot(HaveOccurred(), "CRD should exist before DELETE")
GinkgoWriter.Printf("✅ CRD exists: %s/%s (UID: %s)\n", retrieved.Namespace, retrieved.Name, retrieved.UID)
```

---

## 🎯 **Recommended Next Steps**

### **Priority 1: Verify envtest DELETE Webhook Support** ⭐⭐⭐⭐⭐
**Action**: Research envtest documentation and test limitations

**Commands**:
```bash
# Check envtest version
go list -m sigs.k8s.io/controller-runtime

# Search envtest source code for DELETE webhook handling
grep -r "Delete.*webhook\|DELETE.*admission" vendor/sigs.k8s.io/controller-runtime/pkg/envtest/
```

**Expected Outcome**: Determine if envtest supports DELETE webhooks

### **Priority 2: Run Tests in Kind Cluster** ⭐⭐⭐⭐
**Action**: Execute same tests in real Kubernetes environment

**Commands**:
```bash
# Start Kind cluster
kind create cluster --name kubernaut-webhook-test

# Deploy webhook service
kubectl apply -f config/webhook/manifests.yaml
kubectl apply -f config/crd/bases/

# Run integration tests against Kind
make test-integration-authwebhook-kind
```

**Expected Outcome**: If tests pass in Kind but fail in envtest, confirms envtest limitation

### **Priority 3: Add Diagnostic Logging** ⭐⭐⭐
**Action**: Add detailed logging to debug webhook invocation

**Changes**:
```go
// In suite_test.go, after webhook server start:
By("Verifying webhook configurations are loaded")
mutatingWebhook := &admissionregistrationv1.MutatingWebhookConfiguration{}
err = k8sClient.Get(ctx, client.ObjectKey{Name: "kubernaut-authwebhook-mutating"}, mutatingWebhook)
GinkgoWriter.Printf("✅ MutatingWebhookConfiguration: %v\n", err)

validatingWebhook := &admissionregistrationv1.ValidatingWebhookConfiguration{}
err = k8sClient.Get(ctx, client.ObjectKey{Name: "kubernaut-authwebhook-validating"}, validatingWebhook)
GinkgoWriter.Printf("✅ ValidatingWebhookConfiguration: %v\n", err)
if err == nil {
    GinkgoWriter.Printf("   Webhooks: %+v\n", validatingWebhook.Webhooks)
}
```

### **Priority 4: Test CRD Existence Before DELETE** ⭐⭐
**Action**: Verify CRD actually exists in envtest before deleting

**Changes**:
```go
// In notificationrequest_test.go, before DELETE:
By("Verifying NotificationRequest exists in envtest before DELETE")
var retrieved notificationv1.NotificationRequest
err := k8sClient.Get(ctx, client.ObjectKey{Name: nrName, Namespace: namespace}, &retrieved)
Expect(err).ToNot(HaveOccurred(), "CRD must exist for DELETE to trigger webhook")
GinkgoWriter.Printf("✅ CRD exists: %s/%s (UID: %s, ResourceVersion: %s)\n",
    retrieved.Namespace, retrieved.Name, retrieved.UID, retrieved.ResourceVersion)
```

---

##  🎯 **Alternative Approaches if envtest Limitation Confirmed**

### **Option A: Skip DELETE Tests in envtest, Run in E2E/Kind** ⭐⭐⭐⭐⭐
**Pros**:
- Fast unit/integration tests (envtest)
- Full webhook coverage in E2E tests (Kind)
- No envtest workarounds needed

**Cons**:
- Longer test feedback loop for DELETE attribution
- E2E tests more complex to run

**Implementation**:
```go
// In notificationrequest_test.go:
Context("INT-NR-01: when operator cancels notification via DELETE", func() {
    It("should capture operator identity in audit trail via webhook", func() {
        if os.Getenv("SKIP_DELETE_WEBHOOK_TESTS") == "true" {
            Skip("DELETE webhooks not supported in envtest - run E2E tests in Kind")
        }
        // ... test code ...
    })
})
```

### **Option B: Mock DELETE Attribution in Integration Tests** ⭐⭐⭐
**Pros**:
- All tests pass in envtest
- Fast feedback loop

**Cons**:
- Not testing real webhook invocation
- Reduced confidence in DELETE attribution

**Implementation**:
```go
// Create manual audit event for DELETE in test setup
func simulateDELETEWebhookAudit(ctx context.Context, nr *notificationv1.NotificationRequest) {
    auditEvent := audit.NewAuditEventRequest()
    audit.SetEventType(auditEvent, "notification.request.deleted")
    audit.SetEventCategory(auditEvent, "webhook")
    audit.SetCorrelationID(auditEvent, nr.Name)
    // ... set other fields ...
    err := auditStore.StoreAudit(ctx, auditEvent)
    Expect(err).ToNot(HaveOccurred())
}
```

### **Option C: Use controller-runtime Manager Instead of Standalone Webhook Server** ⭐⭐
**Pros**:
- controller-runtime Manager might have better envtest integration

**Cons**:
- Significant refactoring required
- May not solve envtest DELETE webhook issue

---

## 📊 **Current Test Status**

| CRD | Operation | Tests | Status | Webhook Type |
|-----|-----------|-------|--------|--------------|
| **WorkflowExecution** | UPDATE (block clearance) | 3/3 | ✅ PASSING | Mutating |
| **RemediationApprovalRequest** | UPDATE (decision) | 2/2 | ✅ PASSING | Mutating |
| **NotificationRequest** | DELETE (cancellation) | 0/2 | ❌ FAILING | Validating |
| **NotificationRequest** | Normal completion (no webhook) | 2/2 | ✅ PASSING | N/A |

**Overall**: 7/9 passing (78%)

---

## 🚀 **Success Criteria**

| Metric | Target | Actual | Gap |
|--------|--------|--------|-----|
| **Test Pass Rate** | 100% (9/9) | 78% (7/9) | -22% |
| **Webhook Invocation** | 100% | 67% (2/3 webhooks) | -33% |
| **Audit Event Capture** | 100% | 78% (7/9 scenarios) | -22% |

---

## 📚 **References**

- **envtest Documentation**: https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/envtest
- **Kubernetes Admission Webhooks**: https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/
- **DD-WEBHOOK-003**: Webhook-Complete Audit Pattern
- **DD-TESTING-001**: Audit Event Validation Standards
- **BR-AUTH-001**: SOC2 CC8.1 Operator Attribution Requirements

---

**Document Status**: 🔍 Active Investigation
**Review Schedule**: After envtest limitations confirmed
**Decision Required**: Choose Alternative Approach if envtest doesn't support DELETE webhooks

