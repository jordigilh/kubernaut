# Webhook Integration Test Fix Plan - Option A (TDD Compliant)

**Date**: January 6, 2026
**Status**: 🚀 **ACTIVE** - User approved Option A
**Purpose**: Rewrite integration tests to follow TESTING_GUIDELINES.md patterns
**Timeline**: 4-6 hours
**Owner**: Webhook Team

---

## 🎯 **Goal: TDD-Compliant Integration Tests**

**Replace**: HTTP webhook infrastructure tests
**With**: Business logic tests that verify webhook side effects via envtest

**Principle**: Test CRD operations (business logic), verify webhook populated fields (side effects)

---

## 📋 **Phase 1: Update Test Infrastructure (1.5 hours)**

### **Task 1.1: Create envtest Suite Setup**

**File**: `test/integration/authwebhook/suite_test.go`

**Implementation**:
```go
package authwebhook_test

import (
    "context"
    "path/filepath"
    "testing"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
    notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
    "github.com/jordigilh/kubernaut/pkg/authwebhook"

    "k8s.io/client-go/kubernetes/scheme"
    "k8s.io/client-go/rest"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/envtest"
    "sigs.k8s.io/controller-runtime/pkg/webhook"
    logf "sigs.k8s.io/controller-runtime/pkg/log"
    "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
    cfg       *rest.Config
    k8sClient client.Client
    testEnv   *envtest.Environment
    ctx       context.Context
    cancel    context.CancelFunc
)

func TestAuthWebhookIntegration(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "AuthWebhook Integration Suite - BR-AUTH-001 SOC2 Attribution")
}

var _ = BeforeSuite(func() {
    logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

    ctx, cancel = context.WithCancel(context.TODO())

    By("Bootstrapping test environment")
    testEnv = &envtest.Environment{
        CRDDirectoryPaths: []string{
            filepath.Join("..", "..", "..", "config", "crd", "bases"),
        },
        ErrorIfCRDPathMissing: true,
        WebhookInstallOptions: envtest.WebhookInstallOptions{
            Paths: []string{
                filepath.Join("..", "..", "..", "config", "webhook"),
            },
        },
    }

    var err error
    cfg, err = testEnv.Start()
    Expect(err).NotTo(HaveOccurred())
    Expect(cfg).NotTo(BeNil())

    // Register schemes
    err = workflowexecutionv1.AddToScheme(scheme.Scheme)
    Expect(err).NotTo(HaveOccurred())
    err = remediationv1.AddToScheme(scheme.Scheme)
    Expect(err).NotTo(HaveOccurred())
    err = notificationv1.AddToScheme(scheme.Scheme)
    Expect(err).NotTo(HaveOccurred())

    // Create k8s client
    k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
    Expect(err).NotTo(HaveOccurred())
    Expect(k8sClient).NotTo(BeNil())

    // Setup webhook server
    webhookInstallOptions := &testEnv.WebhookInstallOptions
    webhookServer := webhook.NewServer(webhook.Options{
        Host:    webhookInstallOptions.LocalServingHost,
        Port:    webhookInstallOptions.LocalServingPort,
        CertDir: webhookInstallOptions.LocalServingCertDir,
    })

    // Register webhook handlers (pkg/authwebhook not yet implemented, will be in GREEN phase)
    // For now, we'll register placeholder handlers that will be replaced
    authenticator := authwebhook.NewAuthenticator()
    
    webhookServer.Register("/mutate-workflowexecution",
        authwebhook.NewWorkflowExecutionHandler(authenticator))
    webhookServer.Register("/mutate-remediationapprovalrequest",
        authwebhook.NewRemediationApprovalRequestHandler(authenticator))
    webhookServer.Register("/validate-notificationrequest-delete",
        authwebhook.NewNotificationRequestDeleteHandler(authenticator))

    // Start webhook server
    go func() {
        defer GinkgoRecover()
        err := webhookServer.Start(ctx)
        Expect(err).NotTo(HaveOccurred())
    }()

    // Wait for webhook server to be ready
    Eventually(func() error {
        return webhookServer.StartedChecker()(nil)
    }, 10*time.Second, 100*time.Millisecond).Should(Succeed())
})

var _ = AfterSuite(func() {
    By("Tearing down the test environment")
    cancel()
    err := testEnv.Stop()
    Expect(err).NotTo(HaveOccurred())
})
```

**Deliverables**:
- ✅ envtest environment configured
- ✅ Webhook server running with envtest
- ✅ K8s client available for CRD operations
- ✅ All 3 CRD schemes registered

**Time**: 1 hour

---

### **Task 1.2: Create Test Helpers**

**File**: `test/integration/authwebhook/helpers.go`

**Implementation**:
```go
package authwebhook_test

import (
    "context"
    "time"

    . "github.com/onsi/gomega"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

// waitForStatusField polls for a status field to be populated by webhook
func waitForStatusField(
    ctx context.Context,
    obj client.Object,
    fieldGetter func() string,
    timeout time.Duration,
) {
    Eventually(func() string {
        err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)
        if err != nil {
            return ""
        }
        return fieldGetter()
    }, timeout, 500*time.Millisecond).ShouldNot(BeEmpty(),
        "Webhook should populate status field within %s", timeout)
}

// createAndWaitForCRD creates a CRD and waits for it to be ready
func createAndWaitForCRD(ctx context.Context, obj client.Object) {
    Expect(k8sClient.Create(ctx, obj)).To(Succeed(),
        "CRD creation should succeed")

    // Wait for CRD to be created (eventually consistent)
    Eventually(func() error {
        return k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)
    }, 10*time.Second, 500*time.Millisecond).Should(Succeed(),
        "CRD should be retrievable after creation")
}

// updateStatusAndWaitForWebhook updates CRD status and waits for webhook mutation
func updateStatusAndWaitForWebhook(
    ctx context.Context,
    obj client.Object,
    updateFunc func(),
    verifyFunc func() bool,
) {
    // Apply status update
    updateFunc()
    Expect(k8sClient.Status().Update(ctx, obj)).To(Succeed(),
        "Status update should trigger webhook")

    // Wait for webhook to populate fields
    Eventually(func() bool {
        err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)
        if err != nil {
            return false
        }
        return verifyFunc()
    }, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
        "Webhook should mutate CRD within 10 seconds")
}
```

**Deliverables**:
- ✅ Reusable helper functions for CRD operations
- ✅ `Eventually()` patterns for async validation
- ✅ No `time.Sleep()` anti-patterns

**Time**: 30 minutes

---

## 📋 **Phase 2: WorkflowExecution Integration Tests (1 hour)**

### **Task 2.1: Block Clearance Attribution Test**

**File**: `test/integration/authwebhook/workflowexecution_test.go`

**Test ID**: INT-WE-01

**Implementation**:
```go
package authwebhook_test

import (
    "context"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("BR-WE-013: WorkflowExecution Block Clearance Attribution", func() {
    var (
        ctx       context.Context
        namespace string
    )

    BeforeEach(func() {
        ctx = context.Background()
        namespace = "default"
    })

    Context("INT-WE-01: when operator clears workflow execution block", func() {
        It("should capture operator identity via webhook", func() {
            // ✅ CORRECT: Trigger business operation (create CRD)
            wfe := &workflowexecutionv1.WorkflowExecution{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-wfe-clearance",
                    Namespace: namespace,
                },
                Spec: workflowexecutionv1.WorkflowExecutionSpec{
                    WorkflowName: "test-workflow",
                },
            }

            createAndWaitForCRD(ctx, wfe)

            By("Operator requests block clearance (business operation)")
            // ✅ CORRECT: Business logic - operator updates status
            updateStatusAndWaitForWebhook(ctx, wfe,
                func() {
                    wfe.Status.BlockClearanceRequest = &workflowexecutionv1.BlockClearanceRequest{
                        Reason:    "Integration test clearance - validated operator decision after analysis",
                        ClearedBy: "", // Will be populated by webhook
                    }
                },
                func() bool {
                    return wfe.Status.BlockClearanceRequest != nil &&
                        wfe.Status.BlockClearanceRequest.ClearedBy != ""
                },
            )

            By("Verifying webhook populated authentication fields (side effect)")
            // ✅ CORRECT: Validate webhook side effects
            Expect(wfe.Status.BlockClearanceRequest.ClearedBy).To(ContainSubstring("@"),
                "clearedBy should contain operator email from K8s UserInfo")
            Expect(wfe.Status.BlockClearanceRequest.ClearedAt).ToNot(BeNil(),
                "clearedAt timestamp should be set by webhook")

            GinkgoWriter.Printf("✅ Block cleared by: %s at %s\n",
                wfe.Status.BlockClearanceRequest.ClearedBy,
                wfe.Status.BlockClearanceRequest.ClearedAt)
        })
    })

    Context("INT-WE-02: when clearance reason is missing", func() {
        It("should reject clearance request via webhook validation", func() {
            wfe := &workflowexecutionv1.WorkflowExecution{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-wfe-no-reason",
                    Namespace: namespace,
                },
                Spec: workflowexecutionv1.WorkflowExecutionSpec{
                    WorkflowName: "test-workflow",
                },
            }

            createAndWaitForCRD(ctx, wfe)

            By("Operator attempts clearance without reason (invalid business operation)")
            wfe.Status.BlockClearanceRequest = &workflowexecutionv1.BlockClearanceRequest{
                Reason:    "", // Invalid - empty reason
                ClearedBy: "",
            }

            // ✅ CORRECT: Expect K8s API to reject due to webhook validation
            err := k8sClient.Status().Update(ctx, wfe)
            Expect(err).To(HaveOccurred(),
                "Webhook should reject clearance without reason")
            Expect(err.Error()).To(ContainSubstring("reason"),
                "Error should mention missing reason")
        })
    })

    Context("INT-WE-03: when clearance reason is too short", func() {
        It("should reject clearance with weak justification", func() {
            wfe := &workflowexecutionv1.WorkflowExecution{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-wfe-weak-reason",
                    Namespace: namespace,
                },
                Spec: workflowexecutionv1.WorkflowExecutionSpec{
                    WorkflowName: "test-workflow",
                },
            }

            createAndWaitForCRD(ctx, wfe)

            By("Operator provides insufficient justification (SOC2 CC7.4 violation)")
            wfe.Status.BlockClearanceRequest = &workflowexecutionv1.BlockClearanceRequest{
                Reason:    "Test", // < 10 words minimum
                ClearedBy: "",
            }

            // ✅ CORRECT: Webhook should enforce minimum reason length
            err := k8sClient.Status().Update(ctx, wfe)
            Expect(err).To(HaveOccurred(),
                "Webhook should reject weak justification (< 10 words)")
            Expect(err.Error()).To(Or(
                ContainSubstring("words"),
                ContainSubstring("reason"),
            ), "Error should mention reason validation")
        })
    })
})
```

**BR**: BR-WE-013, BR-AUTH-001, SOC2 CC7.4, SOC2 CC8.1

**Deliverables**:
- ✅ 3 WorkflowExecution integration tests
- ✅ Business logic focus (CRD operations)
- ✅ Webhook side effects validated
- ✅ No HTTP client calls

**Time**: 45 minutes

---

## 📋 **Phase 3: RemediationApprovalRequest Integration Tests (45 min)**

### **Task 3.1: Approval/Rejection Attribution Tests**

**File**: `test/integration/authwebhook/remediationapprovalrequest_test.go`

**Test IDs**: INT-RAR-01, INT-RAR-02, INT-RAR-03

**Implementation**: Similar to WorkflowExecution tests, but for RAR CRD

**Key Test Scenarios**:
1. **INT-RAR-01**: Approval captures `approvedBy` and `decidedAt`
2. **INT-RAR-02**: Rejection captures `rejectedBy` and `decidedAt`
3. **INT-RAR-03**: Invalid decision (not "Approved"/"Rejected") is rejected

**Time**: 45 minutes

---

## 📋 **Phase 4: NotificationRequest Integration Tests (45 min)**

### **Task 4.1: DELETE Attribution Tests**

**File**: `test/integration/authwebhook/notificationrequest_test.go`

**Test IDs**: INT-NR-01, INT-NR-02, INT-NR-03

**Implementation**: Tests for DELETE operations with annotation capture

**Key Test Scenarios**:
1. **INT-NR-01**: DELETE captures `cancelled-by` annotation
2. **INT-NR-02**: DELETE captures `cancelled-at` timestamp annotation
3. **INT-NR-03**: Unauthenticated DELETE is rejected

**Time**: 45 minutes

---

## 📋 **Phase 5: Update Documentation (1 hour)**

### **Task 5.1: Update WEBHOOK_TEST_PLAN.md**

**File**: `docs/development/SOC2/WEBHOOK_TEST_PLAN.md`

**Changes**:
- Replace lines 703-909 (integration tests section)
- Add envtest infrastructure setup
- Update test count from 11 to 9 tests (removed TLS, concurrent tests → moved to E2E)
- Add correct integration test examples
- Update test infrastructure diagram

**Time**: 30 minutes

---

### **Task 5.2: Update WEBHOOK_IMPLEMENTATION_PLAN.md**

**File**: `docs/development/SOC2/WEBHOOK_IMPLEMENTATION_PLAN.md`

**Changes**:
- Update Day 2-4 "integration test" tasks to reflect envtest approach
- Remove references to HTTP client testing
- Add envtest setup to Day 1 deliverables
- Update success metrics for integration tests

**Time**: 30 minutes

---

## 📊 **Timeline Summary**

| Phase | Tasks | Duration | Cumulative |
|-------|-------|----------|------------|
| **Phase 1** | Test infrastructure + helpers | 1.5 hours | 1.5 hours |
| **Phase 2** | WorkflowExecution tests | 1 hour | 2.5 hours |
| **Phase 3** | RemediationApprovalRequest tests | 45 min | 3.25 hours |
| **Phase 4** | NotificationRequest tests | 45 min | 4 hours |
| **Phase 5** | Update documentation | 1 hour | 5 hours |
| **Buffer** | Testing, review, adjustments | 1 hour | 6 hours |

**Total**: 6 hours (including buffer)

---

## ✅ **Success Criteria**

| Criterion | Target | Validation |
|-----------|--------|------------|
| **envtest configured** | 100% | Suite runs, webhook server starts |
| **CRD operations used** | 100% | All tests use `k8sClient.Create()`, `k8sClient.Update()` |
| **Webhook side effects validated** | 100% | All tests use `Eventually()` + `k8sClient.Get()` |
| **No HTTP client calls** | 0 | `grep -r "httpClient" test/integration/authwebhook/` returns nothing |
| **Test count** | 9 tests | 3 per CRD type |
| **All tests passing** | 9/9 | `make test-integration-authwebhook` succeeds |

---

## 🎯 **Next Steps**

### **Immediate (Now)**

1. Create `test/integration/authwebhook/` directory
2. Implement Phase 1 (envtest infrastructure)
3. Run suite to verify envtest works

### **After Phase 1 Complete**

4. Implement Phases 2-4 (CRD-specific tests)
5. Run tests incrementally
6. Debug any envtest issues

### **After Tests Passing**

7. Update documentation (Phase 5)
8. Commit with proper BR references
9. Proceed to Day 2 implementation (handler code in GREEN phase)

---

## 📚 **References**

- **TESTING_GUIDELINES.md §1688-1949**: Audit Infrastructure Anti-Pattern (same principle)
- **controller-runtime envtest**: https://book.kubebuilder.io/reference/envtest.html
- **Webhook testing example**: https://book.kubebuilder.io/cronjob-tutorial/webhook-implementation.html#testing

---

**Status**: 🚀 **READY TO START**
**Confidence**: 95% (envtest is well-documented pattern)
**Owner**: Webhook Team
**Next Action**: Create `test/integration/authwebhook/suite_test.go`



