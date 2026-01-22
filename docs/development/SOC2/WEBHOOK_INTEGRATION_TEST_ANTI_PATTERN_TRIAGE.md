# Webhook Integration Test Anti-Pattern Triage

**Date**: January 6, 2026
**Status**: 🔴 **BLOCKING ISSUE**
**Severity**: HIGH
**Reference**: TESTING_GUIDELINES.md §1688-1949 (Audit Infrastructure Testing Anti-Pattern)

---

## 🚨 **Problem: Integration Tests Test Infrastructure, Not Business Logic**

### **Current Approach (WRONG)**

The current `WEBHOOK_TEST_PLAN.md` integration tests (lines 703-909) follow the **infrastructure testing anti-pattern**:

```go
// ❌ FORBIDDEN: Testing webhook HTTP server infrastructure
var _ = Describe("WorkflowExecution Webhook Integration", func() {
    var (
        webhookURL string
        httpClient *http.Client
    )

    BeforeEach(func() {
        webhookURL = "https://127.0.0.1:9443/mutate-workflowexecution"
        httpClient = &http.Client{
            Transport: &http.Transport{
                TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
            },
        }
    })

    It("should populate clearedBy on block clearance request", func() {
        wfe := &workflowexecutionv1.WorkflowExecution{...}
        admissionReview := createAdmissionReview(wfe, "operator@example.com")

        // ❌ WRONG: Directly calling webhook HTTP endpoint
        resp, err := httpClient.Post(webhookURL, "application/json", admissionReview)
        Expect(err).ToNot(HaveOccurred())
        Expect(resp.StatusCode).To(Equal(http.StatusOK))

        // ❌ WRONG: Parsing webhook HTTP response
        var reviewResp admissionv1.AdmissionReview
        _ = json.NewDecoder(resp.Body).Decode(&reviewResp)

        // ❌ WRONG: Testing webhook infrastructure behavior
        Expect(reviewResp.Response.Allowed).To(BeTrue())
        Expect(reviewResp.Response.Patch).ToNot(BeNil())
    })
})
```

### **Why This is Wrong**

Per **TESTING_GUIDELINES.md §1688-1949**:

| Issue | Impact |
|-------|--------|
| **Wrong Responsibility** | Tests webhook HTTP server infrastructure, not service business logic |
| **Wrong Ownership** | These tests belong in `controller-runtime` webhook library tests |
| **Missing Coverage** | Kubernetes API Server → Webhook integration is NOT tested |
| **False Confidence** | Tests pass but don't validate webhook works when K8s API calls it |

**Key Insight**: If your test manually creates HTTP requests and calls webhook endpoints, you're testing infrastructure, not business logic.

---

## ✅ **Correct Approach: Business Logic with Webhook Side Effects**

Per **TESTING_GUIDELINES.md §1773-1862** and webhook E2E test patterns (WEBHOOK_TEST_PLAN.md lines 910+), integration tests should:

1. **Create CRDs** via `k8sClient` (business operation)
2. **Wait for operation completion** (business logic)
3. **Verify webhook populated fields** (side effect validation)

### **Correct Pattern (Using envtest)**

```go
// ✅ CORRECT: Test business logic, verify webhook as side effect
var _ = Describe("BR-AUTH-001: WorkflowExecution Authentication Integration", func() {
    var (
        ctx       context.Context
        namespace string
    )

    BeforeEach(func() {
        ctx = context.Background()
        namespace = "default"
    })

    Context("when operator clears workflow execution block", func() {
        It("should capture operator identity via webhook", func() {
            // ✅ CORRECT: Trigger business operation (create CRD)
            wfe := &workflowexecutionv1.WorkflowExecution{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-wfe-auth",
                    Namespace: namespace,
                },
                Spec: workflowexecutionv1.WorkflowExecutionSpec{
                    WorkflowName: "test-workflow",
                },
            }
            Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

            // ✅ CORRECT: Simulate operator updating block clearance (business logic)
            wfe.Status.BlockClearanceRequest = &workflowexecutionv1.BlockClearanceRequest{
                Reason:    "Integration test clearance",
                ClearedBy: "", // Will be populated by webhook
            }
            Expect(k8sClient.Status().Update(ctx, wfe)).To(Succeed())

            // ✅ CORRECT: Verify webhook populated fields (SIDE EFFECT)
            Eventually(func() string {
                var updated workflowexecutionv1.WorkflowExecution
                _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(wfe), &updated)
                if updated.Status.BlockClearanceRequest == nil {
                    return ""
                }
                return updated.Status.BlockClearanceRequest.ClearedBy
            }, 10*time.Second, 500*time.Millisecond).ShouldNot(BeEmpty(),
                "Webhook should populate clearedBy field when K8s API calls it")

            // ✅ CORRECT: Validate webhook populated correct values
            var updated workflowexecutionv1.WorkflowExecution
            Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(wfe), &updated)).To(Succeed())

            Expect(updated.Status.BlockClearanceRequest.ClearedBy).To(ContainSubstring("@"),
                "clearedBy should contain operator email")
            Expect(updated.Status.BlockClearanceRequest.ClearedAt).ToNot(BeNil(),
                "clearedAt timestamp should be set by webhook")
        })

        It("should reject unauthenticated block clearance", func() {
            // ✅ CORRECT: Test error scenario with business logic
            wfe := &workflowexecutionv1.WorkflowExecution{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-wfe-unauth",
                    Namespace: namespace,
                },
                Spec: workflowexecutionv1.WorkflowExecutionSpec{
                    WorkflowName: "test-workflow",
                },
            }
            Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

            // Configure webhook to simulate unauthenticated request (test-specific setup)
            // This would require test-specific webhook configuration or mock

            wfe.Status.BlockClearanceRequest = &workflowexecutionv1.BlockClearanceRequest{
                Reason:    "Unauthorized test",
                ClearedBy: "",
            }

            // ✅ CORRECT: Expect K8s API to reject due to webhook validation
            err := k8sClient.Status().Update(ctx, wfe)
            Expect(err).To(HaveOccurred(),
                "Webhook should reject unauthenticated clearance request")
            Expect(err.Error()).To(ContainSubstring("authentication required"),
                "Error should indicate missing authentication")
        })
    })

    Context("when webhook is unavailable", func() {
        It("should fail CRD updates (fail-safe behavior)", func() {
            // ✅ CORRECT: Test webhook failure scenario via K8s API
            // Stop webhook server or configure webhook to fail

            wfe := &workflowexecutionv1.WorkflowExecution{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-wfe-failsafe",
                    Namespace: namespace,
                },
                Spec: workflowexecutionv1.WorkflowExecutionSpec{
                    WorkflowName: "test-workflow",
                },
            }
            Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

            wfe.Status.BlockClearanceRequest = &workflowexecutionv1.BlockClearanceRequest{
                Reason:    "Test",
                ClearedBy: "",
            }

            // ✅ CORRECT: K8s API should fail when webhook is unavailable
            err := k8sClient.Status().Update(ctx, wfe)
            Expect(err).To(HaveOccurred(),
                "K8s API should fail when webhook is unavailable (fail-safe)")
        })
    })
})
```

---

## 📊 **Pattern Comparison**

| Aspect | ❌ Wrong Pattern (Current) | ✅ Correct Pattern (Required) |
|--------|---------------------------|-------------------------------|
| **Test Focus** | Webhook HTTP server | Business operations |
| **Primary Action** | `httpClient.Post(webhookURL)` | `k8sClient.Update(CRD)` |
| **What's Validated** | HTTP response parsing works | Webhook populates CRD fields |
| **Test Ownership** | Should be in controller-runtime | Correctly in service tests |
| **Business Value** | Tests infrastructure | Tests service behavior |
| **Failure Detection** | Won't catch K8s API → webhook integration issues | Catches webhook integration failures |
| **Infrastructure** | Standalone HTTP server | envtest with webhook enabled |

---

## 🔧 **Required Changes**

### **1. Update Integration Test Infrastructure**

**Current (WRONG)**:
```go
// Standalone webhook HTTP server
BeforeEach(func() {
    webhookURL = "https://127.0.0.1:9443/mutate-workflowexecution"
    httpClient = &http.Client{...}
})
```

**Required (CORRECT)**:
```go
// envtest with webhook configuration
var _ = BeforeSuite(func() {
    ctx, cancel = context.WithCancel(context.TODO())

    By("Setting up test environment with webhook")
    testEnv = &envtest.Environment{
        CRDDirectoryPaths: []string{
            filepath.Join("..", "..", "..", "config", "crd", "bases"),
        },
        WebhookInstallOptions: envtest.WebhookInstallOptions{
            Paths: []string{
                filepath.Join("..", "..", "..", "config", "webhook"),
            },
        },
    }

    cfg, err := testEnv.Start()
    Expect(err).NotTo(HaveOccurred())

    // Setup webhook server with envtest
    webhookInstallOptions := &testEnv.WebhookInstallOptions
    webhookServer := webhook.NewServer(webhook.Options{
        Host:    webhookInstallOptions.LocalServingHost,
        Port:    webhookInstallOptions.LocalServingPort,
        CertDir: webhookInstallOptions.LocalServingCertDir,
    })

    // Register webhook handlers
    webhookServer.Register("/mutate-workflowexecution", &webhook.Admission{
        Handler: &webhooks.WorkflowExecutionAuthHandler{},
    })

    // Start webhook server
    go func() {
        defer GinkgoRecover()
        Expect(webhookServer.Start(ctx)).To(Succeed())
    }()

    // Setup k8sClient (NOT httpClient)
    k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
    Expect(err).NotTo(HaveOccurred())
})
```

### **2. Rewrite All Integration Tests**

**Delete**:
- All tests that create `admissionReview` manually
- All tests that call `httpClient.Post(webhookURL)`
- All tests that parse HTTP responses

**Create**:
- Tests that create CRDs via `k8sClient.Create()`
- Tests that update CRDs via `k8sClient.Update()` or `k8sClient.Status().Update()`
- Tests that verify webhook populated fields via `Eventually()` + `k8sClient.Get()`

### **3. Update Test Count and Tiers**

**Before (WRONG)**:
- Unit: 70 tests (pkg/authwebhook)
- Integration: 11 tests (HTTP webhook calls)
- E2E: 14 tests (Kind cluster)

**After (CORRECT)**:
- Unit: 70 tests (pkg/authwebhook) ✅ Keep as-is
- Integration: 9 tests (envtest with webhook, CRD operations)
- E2E: 14 tests (Kind cluster) ✅ Keep as-is

**Rationale**: Integration tier reduces to 9 tests because:
- TLS certificate validation → Moves to E2E (requires real K8s API)
- Concurrent requests → Moves to E2E (requires production-like environment)
- Multi-CRD flows → Simplifies to 3 CRD types × 2 scenarios = 6 tests
- Plus 3 webhook failure scenarios

---

## 📋 **Revised Integration Test Scenarios**

### **WorkflowExecution (3 tests)**

1. **INT-WE-01**: Block clearance captures operator identity
   - Create WFE → Update with block clearance → Verify `clearedBy` populated
2. **INT-WE-02**: Reject clearance without reason
   - Create WFE → Update with empty reason → Expect error
3. **INT-WE-03**: Reject clearance with weak reason
   - Create WFE → Update with "Test" (< 10 words) → Expect error

### **RemediationApprovalRequest (3 tests)**

1. **INT-RAR-01**: Approval captures operator identity
   - Create RAR → Update with `Decision: "Approved"` → Verify `approvedBy` populated
2. **INT-RAR-02**: Rejection captures operator identity
   - Create RAR → Update with `Decision: "Rejected"` → Verify `rejectedBy` populated
3. **INT-RAR-03**: Reject invalid decision
   - Create RAR → Update with `Decision: "Maybe"` → Expect error

### **NotificationRequest (3 tests)**

1. **INT-NR-01**: DELETE captures operator identity
   - Create NR → `k8sClient.Delete()` → Verify annotations added
2. **INT-NR-02**: Annotations include timestamp
   - Create NR → `k8sClient.Delete()` → Verify `cancelled-at` annotation
3. **INT-NR-03**: Reject unauthenticated DELETE
   - Configure webhook for unauth → Create NR → `k8sClient.Delete()` → Expect error

---

## 🎯 **Success Criteria After Fix**

| Criterion | Target | Validation |
|-----------|--------|------------|
| **Integration tests use envtest** | 100% | No `httpClient.Post()` calls |
| **Integration tests create CRDs** | 100% | All tests use `k8sClient.Create()` |
| **Integration tests verify side effects** | 100% | All tests use `Eventually()` + `k8sClient.Get()` |
| **No manual HTTP requests** | 0 | `grep -r "httpClient.Post" test/integration/webhooks/` returns nothing |
| **Test count** | 9 tests | 3 per CRD type |

---

## 🚀 **Action Items**

1. **BLOCKING**: Rewrite all integration tests to follow correct pattern
2. **REQUIRED**: Update `WEBHOOK_TEST_PLAN.md` with correct integration test examples
3. **REQUIRED**: Update `WEBHOOK_IMPLEMENTATION_PLAN.md` Day 2-4 to reflect envtest approach
4. **RECOMMENDED**: Add CI check to detect HTTP webhook calls in integration tests

```bash
# CI check for wrong pattern
if grep -r "httpClient.Post.*webhook\|admission.Request" test/integration/webhooks --include="*_test.go" | grep -v "pkg/authwebhook"; then
    echo "⚠️  ERROR: Integration tests should NOT make HTTP webhook calls"
    echo "   Integration tests should create CRDs and verify webhook side effects"
    echo "   See: TESTING_GUIDELINES.md §1688-1949"
    echo "   See: WEBHOOK_INTEGRATION_TEST_ANTI_PATTERN_TRIAGE.md"
    exit 1
fi
```

---

## 📚 **References**

- **TESTING_GUIDELINES.md §1688-1949**: Audit Infrastructure Testing Anti-Pattern (same principle applies)
- **WEBHOOK_TEST_PLAN.md lines 910+**: E2E tests follow correct pattern (create CRDs, verify side effects)
- **controller-runtime envtest webhook docs**: https://book.kubebuilder.io/reference/envtest.html#testing-webhooks

---

**Status**: 🔴 **BLOCKING - Must fix before Day 2 implementation**
**Severity**: HIGH - Wrong test pattern invalidates integration test coverage
**Owner**: Webhook Team
**Next Step**: Rewrite integration tests following correct pattern

