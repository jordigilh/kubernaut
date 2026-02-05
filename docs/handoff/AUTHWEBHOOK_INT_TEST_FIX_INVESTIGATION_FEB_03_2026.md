# AuthWebhook Integration Test Fix Investigation

**Date**: February 3, 2026  
**Branch**: `feature/k8s-sar-user-id-stateless-services`  
**Status**: 🔧 **IN PROGRESS** - Issue #1 Fixed, Issues #2 & #3 Require Further Action

---

## 📋 **Issue Summary**

After refactoring AuthWebhook INT tests to use Manager pattern (matching production), we have:
- **Current**: 8/12 passing (67%)
- **Target**: 12/12 passing (100%)
- **4 failing tests** identified with 3 distinct root causes

---

## ✅ **Issue #1: Invalid Kubernetes Resource Names** - FIXED

### **Root Cause**
DescribeTable entries pass uppercase testID ("INT-RAR-01") as the `testSuffix` parameter to `createRAR()`, which creates resource names like `"test-rar-INT-RAR-01-a76c773d"`. Kubernetes requires lowercase RFC 1123 subdomain names.

### **Error Evidence**
```
RemediationApprovalRequest.kubernaut.ai "test-rar-INT-RAR-01-a76c773d" is invalid: 
metadata.name: Invalid value: "test-rar-INT-RAR-01-a76c773d": 
a lowercase RFC 1123 subdomain must consist of lower case alphanumeric characters
```

### **Affected Tests**
- INT-RAR-01: Operator approves production remediation
- INT-RAR-02: Operator rejects risky remediation

### **Why INT-RAR-05/06 Pass**
These tests explicitly pass lowercase suffixes:
```go
createRAR(scenario, "audit")        // ← lowercase "audit"
createRAR(scenario, "preservation") // ← lowercase "preservation"
```

### **Fix Applied**
```go
// remediationapprovalrequest_test.go:58
// Before:
Name: "test-rar-" + testSuffix + "-" + randomSuffix(),

// After:
Name: "test-rar-" + strings.ToLower(testSuffix) + "-" + randomSuffix(),
```

**Files Modified**:
- `test/integration/authwebhook/remediationapprovalrequest_test.go`
  - Added `"strings"` import
  - Updated line 58 to use `strings.ToLower(testSuffix)`

**Expected Impact**: +2 tests passing (INT-RAR-01, INT-RAR-02)

---

## ❓ **Issue #2: Webhook Not Intercepting RAR Status Updates** - NEEDS INVESTIGATION

### **Symptoms**
- **Test**: INT-RAR-04 (Identity Forgery Prevention)
- **Failure**: Timeout at `helpers.go:72` after 10 seconds
- **Error**: Webhook mutation never occurs (`DecidedBy` field remains empty)

### **Current Log Analysis**

#### **What We Know from Logs**:

1. ✅ **Webhook Registration Successful**:
```
INFO controller-runtime.webhook Registering webhook {"path": "/mutate-remediationapprovalrequest"}
✅ Registered RemediationApprovalRequest webhook handler
```

2. ✅ **Manager Started Successfully**:
```
[Process 1] ✅ Manager started (webhook server ready for requests)
• Webhook server: 127.0.0.1:64521 (via Manager)
• CertDir: /var/folders/.../envtest-serving-certs-2146737956
```

3. ❌ **NO Webhook Invocation Logs**:
   - No `Handle()` method entry logs
   - No admission request processing logs
   - No user extraction logs

#### **Comparison: Working Test (INT-RAR-05) vs Failing Test (INT-RAR-04)**

**INT-RAR-05** (PASSES):
```go
updateStatusAndWaitForWebhook(ctx, k8sClient, rar,
    func() {
        rar.Status.Decision = remediationv1.ApprovalDecisionApproved
        rar.Status.DecisionMessage = "Approved after security review"
    },
    func() bool {
        return rar.Status.DecidedBy != ""
    },
)
// Timeline: 14:25:05.76 → 14:25:06.346 (0.586 seconds - SUCCESS)
```

**INT-RAR-04** (FAILS):
```go
updateStatusAndWaitForWebhook(ctx, k8sClient, rar,
    func() {
        rar.Status.Decision = remediationv1.ApprovalDecisionApproved
        rar.Status.DecisionMessage = "Approved"
        rar.Status.DecidedBy = forgedIdentity // USER TRIES TO SET THIS
    },
    func() bool {
        return rar.Status.DecidedBy == authenticatedUser
    },
)
// Timeline: TIMEOUT after 10 seconds
```

**Key Difference**: INT-RAR-04 sets `DecidedBy` in the update function, INT-RAR-05 does not.

**Webhook Handler Logic** (`remediationapprovalrequest_handler.go:88-92`):
```go
// Check if decidedBy is already set (preserve existing attribution)
if rar.Status.DecidedBy != "" {
    // Already decided - don't overwrite
    return admission.Allowed("decision already attributed")
}
```

**Hypothesis**: The webhook IS being called, but returns early (line 91) because `DecidedBy` is already set by the test. The webhook logs this as `"decision already attributed"` but **we have no log proving this**.

---

### **Can We Investigate Issue #2 with Current Logs?**

**Answer**: ❌ **NO - Insufficient Logging**

**What's Missing**:
1. **No webhook entry logs**: We can't prove webhook was invoked
2. **No early-exit logs**: Can't confirm if line 91 is being hit
3. **No timing logs**: Can't measure webhook response time
4. **No error logs**: Can't see if webhook encounters errors before returning

**Current Handler Logging Gaps**:
```go
func (h *RemediationApprovalRequestAuthHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
    rar := &remediationv1.RemediationApprovalRequest{}
    
    // ❌ NO LOG: Webhook invoked for RAR {name}
    
    err := json.Unmarshal(req.Object.Raw, rar)
    if err != nil {
        return admission.Errored(http.StatusBadRequest, ...)
    }
    
    if rar.Status.Decision == "" {
        // ❌ NO LOG: Skipping (no decision made)
        return admission.Allowed("no decision made")
    }
    
    // ... validation ...
    
    if rar.Status.DecidedBy != "" {
        // ❌ NO LOG: Skipping (already decided) - CRITICAL GAP!
        return admission.Allowed("decision already attributed")
    }
    
    // ❌ NO LOG: Populating DecidedBy for {user}
    rar.Status.DecidedBy = authCtx.Username
    
    // ❌ NO LOG: Audit event emitted
}
```

---

### **Recommended Logging Enhancement**

**File**: `pkg/authwebhook/remediationapprovalrequest_handler.go`

**Add Structured Logging**:
```go
import (
    "sigs.k8s.io/controller-runtime/pkg/log"
)

func (h *RemediationApprovalRequestAuthHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
    logger := log.FromContext(ctx).WithName("rar-webhook")
    
    rar := &remediationv1.RemediationApprovalRequest{}
    
    // LOG: Entry point
    logger.Info("Webhook invoked",
        "operation", req.Operation,
        "namespace", req.Namespace,
        "name", req.Name,
    )
    
    err := json.Unmarshal(req.Object.Raw, rar)
    if err != nil {
        logger.Error(err, "Failed to decode RAR")
        return admission.Errored(http.StatusBadRequest, ...)
    }
    
    // LOG: Decision check
    logger.V(1).Info("Checking decision status",
        "decision", rar.Status.Decision,
        "decidedBy", rar.Status.DecidedBy,
    )
    
    if rar.Status.Decision == "" {
        logger.Info("Skipping RAR (no decision made)")
        return admission.Allowed("no decision made")
    }
    
    // LOG: Validation
    if !validDecisions[rar.Status.Decision] {
        logger.Info("Rejecting RAR (invalid decision)",
            "decision", rar.Status.Decision,
        )
        return admission.Denied(...)
    }
    
    // LOG: CRITICAL - Idempotency check
    if rar.Status.DecidedBy != "" {
        logger.Info("Skipping RAR (already decided) - IDEMPOTENCY",
            "existingDecidedBy", rar.Status.DecidedBy,
            "decision", rar.Status.Decision,
        )
        return admission.Allowed("decision already attributed")
    }
    
    // LOG: Authentication
    authCtx, err := h.authenticator.ExtractUser(ctx, &req.AdmissionRequest)
    if err != nil {
        logger.Error(err, "Authentication failed")
        return admission.Denied(...)
    }
    logger.Info("User authenticated",
        "username", authCtx.Username,
        "uid", authCtx.UID,
    )
    
    // LOG: Mutation
    logger.Info("Populating DecidedBy field",
        "authenticatedUser", authCtx.Username,
        "decision", rar.Status.Decision,
    )
    rar.Status.DecidedBy = authCtx.Username
    now := metav1.Now()
    rar.Status.DecidedAt = &now
    
    // LOG: Audit
    logger.Info("Emitting webhook audit event",
        "correlationID", rar.Name,
        "eventAction", "approval_decided",
    )
    // ... audit event code ...
    
    // LOG: Success
    logger.Info("RAR mutation complete",
        "decidedBy", rar.Status.DecidedBy,
        "decidedAt", rar.Status.DecidedAt.Time,
    )
    
    // Create patch and return
    marshaledRAR, _ := json.Marshal(rar)
    return admission.PatchResponseFromRaw(req.Object.Raw, marshaledRAR)
}
```

**Benefits**:
1. ✅ Proves webhook is invoked (or not)
2. ✅ Identifies exact code path taken
3. ✅ Reveals idempotency logic triggering
4. ✅ Shows timing of each webhook operation
5. ✅ Enables debugging without re-running tests

**Expected Log Output** (INT-RAR-04 with logging):
```
INFO rar-webhook Webhook invoked operation=UPDATE namespace=default name=test-rar-forgery-...
INFO rar-webhook Checking decision status decision=Approved decidedBy=malicious-user@example.com
INFO rar-webhook Skipping RAR (already decided) - IDEMPOTENCY existingDecidedBy=malicious-user@example.com decision=Approved
```

**This log would PROVE**: Webhook is being called, but idempotency check prevents mutation because test pre-sets `DecidedBy`.

---

### **Alternative Hypothesis: Webhook Not Being Called At All**

If logging reveals webhook is **never invoked**, possible causes:

1. **WebhookConfiguration Not Applied**:
   - Check: `kubectl get mutatingwebhookconfigurations -n default`
   - Verify: `remediationapprovalrequests/status` rule exists

2. **Certificate Mismatch**:
   - Check: Webhook server cert matches WebhookConfiguration `caBundle`
   - Verify: envtest cert patching occurred

3. **Port Mismatch**:
   - Check: WebhookConfiguration URL points to correct port
   - Verify: Manager webhook server is listening on expected port

4. **Timing Race**:
   - Even with Manager, if test runs IMMEDIATELY after `k8sManager.Start()`, 
     webhook server might not have completed TLS handshake

**Recommended Additional Checks** (if logging shows no invocation):
```bash
# In test suite BeforeEach (after Manager starts):
Eventually(func() error {
    // Try to hit webhook health endpoint
    resp, err := httpClient.Get(fmt.Sprintf("https://%s:%d/healthz", 
        webhookHost, webhookPort))
    return err
}, 5*time.Second, 500*time.Millisecond).Should(Succeed(),
    "Webhook server must respond to health checks before tests start")
```

---

## 🔧 **Issue #3: Certificate Watcher File Access Errors** - FIX AVAILABLE

### **Root Cause**
Manager's `certwatcher` component monitors TLS certificate files for hot-reload in production. In parallel test execution:
1. Process A starts Manager (watches cert files in `/tmp/envtest-serving-certs-XXX/`)
2. Process B completes tests, tears down envtest (deletes cert directory)
3. Process A's certwatcher detects `REMOVE` event, tries to re-read cert
4. **Error**: File no longer exists

### **Error Evidence**
```
ERROR controller-runtime.certwatcher error re-reading certificate
"error": "open /var/folders/.../tls.crt: no such file or directory"

DEBUG controller-runtime.certwatcher certificate event
"event": "REMOVE \"/var/folders/.../tls.key\""
```

### **Timeline**
```
14:25:01 - Processes 2,11,12,6,7... start Managers (watch certs)
14:25:04 - Processes 11,12,8,7... complete, tear down envtest (delete certs)
14:25:05 - Process 1 starts Manager (tries to watch deleted certs) → ERROR
```

### **Why This Affects INT-NR-02 Specifically**
INT-NR-02 test was running when the certwatcher error occurred, causing test instability.

---

### **Fix Options**

#### **Option A: Disable certwatcher in Tests (RECOMMENDED)**

**Rationale**: 
- Tests don't need hot-reload (certs are static per test process)
- Production deployments use Kubernetes-managed cert rotation (cert-manager)
- certwatcher is a production optimization, not test requirement

**Implementation**:
```go
// test/integration/authwebhook/suite_test.go:260

By("Creating controller-runtime Manager with test-optimized configuration")
webhookInstallOptions := &testEnv.WebhookInstallOptions

// Create webhook server WITHOUT certwatcher
// Pattern: Override Server type to disable file watching in tests
webhookServer := &testWebhookServer{
    Server: webhook.NewServer(webhook.Options{
        Host:    webhookInstallOptions.LocalServingHost,
        Port:    webhookInstallOptions.LocalServingPort,
        CertDir: webhookInstallOptions.LocalServingCertDir,
    }),
}

k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{
    Scheme: scheme.Scheme,
    Metrics: metricsserver.Options{
        BindAddress: "0",
    },
    WebhookServer: webhookServer,
})
```

**Custom Webhook Server** (disables certwatcher):
```go
// testWebhookServer wraps webhook.Server and overrides cert loading
type testWebhookServer struct {
    *webhook.Server
}

// StartedChecker returns a checker that always reports ready
// (bypasses certwatcher startup wait)
func (s *testWebhookServer) StartedChecker() healthz.Checker {
    return func(_ *http.Request) error {
        return nil // Always ready
    }
}
```

**Alternative**: Use webhook server options if available in controller-runtime to disable file watching.

---

#### **Option B: Coordinate Cert Cleanup Across Processes**

**Rationale**: Ensure all Managers stop before any cert cleanup happens

**Implementation**:
```go
// SynchronizedAfterSuite Phase 1 (ALL processes)
By("Stopping Manager before cert cleanup")
if cancel != nil {
    cancel() // Stop Manager's webhook server
}

// Wait for Manager to fully stop
time.Sleep(2 * time.Second)

// THEN tear down envtest (deletes certs)
if testEnv != nil {
    err := testEnv.Stop()
    // ...
}
```

**Limitation**: Adds 2-second delay per process (24 seconds total for 12 processes)

---

#### **Option C: Copy Certs to Process-Local Directory**

**Rationale**: Each process watches its own cert copy, isolated from other processes

**Implementation**:
```go
By("Copying envtest certs to process-local directory")
processLocalCertDir := filepath.Join(os.TempDir(), 
    fmt.Sprintf("aw-int-certs-process-%d", GinkgoParallelProcess()))
os.MkdirAll(processLocalCertDir, 0755)

// Copy certs from envtest to process-local dir
copyFile(
    filepath.Join(webhookInstallOptions.LocalServingCertDir, "tls.crt"),
    filepath.Join(processLocalCertDir, "tls.crt"),
)
copyFile(
    filepath.Join(webhookInstallOptions.LocalServingCertDir, "tls.key"),
    filepath.Join(processLocalCertDir, "tls.key"),
)

// Use process-local cert dir for Manager
k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{
    // ...
    WebhookServer: webhook.NewServer(webhook.Options{
        Host:    webhookInstallOptions.LocalServingHost,
        Port:    webhookInstallOptions.LocalServingPort,
        CertDir: processLocalCertDir, // ← Isolated per process
    }),
})
```

**Limitation**: Requires cert copying logic, more complex

---

### **Recommended Fix: Option A (Disable certwatcher)**

**Why**:
- ✅ Simplest implementation
- ✅ Matches test requirements (no hot-reload needed)
- ✅ Aligns with production pattern (Kubernetes cert-manager handles rotation)
- ✅ No performance penalty
- ✅ No coordination complexity

**How to Verify Fix**:
```bash
# After applying fix, search logs for certwatcher errors
grep "certwatcher" /tmp/aw-int-test.log
# Should return: NO RESULTS
```

---

## 📊 **Expected Results After All Fixes**

| Fix | Tests Affected | Expected Impact |
|-----|---------------|----------------|
| **#1: Lowercase resource names** | INT-RAR-01, INT-RAR-02 | +2 passing |
| **#2: Add webhook logging** | INT-RAR-04 | Diagnostic data (may reveal test bug) |
| **#3: Disable certwatcher** | INT-NR-02 | +1 passing |

**Target**: 11/12 or 12/12 passing (depending on INT-RAR-04 root cause)

---

## 🎯 **Recommended Action Plan**

### **Phase 1: Quick Wins (This Session)**
1. ✅ **Fix #1**: Applied (lowercase resource names)
2. 🔧 **Fix #3**: Apply (disable certwatcher) - **DO THIS NOW**
3. 🔧 **Add webhook logging**: Implement structured logging - **DO THIS NOW**
4. 🧪 **Re-run tests**: Validate fixes

### **Phase 2: Issue #2 Deep Dive (Next Session)**
1. Analyze webhook logs from Phase 1 test run
2. If webhook not invoked: Check WebhookConfiguration, certs, timing
3. If webhook invoked but idempotency triggered: Fix test logic (don't pre-set `DecidedBy`)
4. Document findings and final fix

---

## 📚 **Related Documentation**

- [RAR Audit RCA](./RCA_AUTHWEBHOOK_INT_E2E_FAILURES_FEB_03_2026.md) - Original webhook timing investigation
- [REFACTOR Metrics](./RAR_AUDIT_REFACTOR_METRICS_FEB_03_2026.md) - Recent REFACTOR phase work
- [DD-TEST-002](../architecture/decisions/DD-TEST-002-parallel-integration-test-execution.md) - Parallel test execution patterns

---

**Status**: Ready to apply Fix #3 and add webhook logging, then re-run tests.
