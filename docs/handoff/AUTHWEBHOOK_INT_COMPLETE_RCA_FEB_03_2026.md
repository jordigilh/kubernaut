# AuthWebhook Integration Test - Complete Root Cause Analysis

**Date**: February 3, 2026  
**Branch**: `feature/k8s-sar-user-id-stateless-services`  
**Test Results**: **11/12 PASSING (92%)** - 1 remaining failure with SECURITY FIX REQUIRED  
**Status**: 🔴 **SECURITY ISSUE IDENTIFIED** - Webhook allows identity forgery

---

## 📊 **Final Test Results**

### **Test Run Summary**
- **Runtime**: 131.6 seconds (~2.2 minutes)
- **Results**: **11/12 PASSED (92%)** ✅
- **Progress**: From 9/12 → 11/12 (+2 tests fixed)
- **Remaining**: 1 test failure (INT-RAR-04) with **SECURITY IMPLICATIONS**

---

## ✅ **Fixes Applied & Validated**

### **Fix #1: Invalid Resource Names** ✅ **VALIDATED**

**Root Cause**: Uppercase letters in Kubernetes resource names (`test-rar-INT-RAR-01-xxx`)

**Fix Applied**:
```go
// test/integration/authwebhook/remediationapprovalrequest_test.go:58
Name: "test-rar-" + strings.ToLower(testSuffix) + "-" + randomSuffix(),
```

**Tests Fixed**:
- ✅ INT-RAR-01: Operator approves production remediation
- ✅ INT-RAR-02: Operator rejects risky remediation

**Validation**:
```
✅ Webhook logs show successful invocation:
  INFO rar-webhook Webhook invoked name="test-rar-int-rar-01-60f62d31"
  INFO rar-webhook Populating DecidedBy field authenticatedUser="admin"
  INFO rar-webhook RAR mutation complete decidedBy="admin"
```

---

### **Fix #2: Webhook Diagnostic Logging** ✅ **IMPLEMENTED**

**Enhancement Applied**: Added structured logging to RAR webhook handler

**Pattern**: Kubernaut Logging Standard (LOGGING_STANDARD.md)
- Uses `ctrl.Log.WithName("rar-webhook")` for CRD controllers
- Structured fields (`"operation"`, `"namespace"`, `"name"`, etc.)
- Key decision points logged (entry, decision check, idempotency, mutation, exit)

**Log Coverage**:
```go
// Entry
logger.Info("Webhook invoked", "operation", req.Operation, "name", req.Name)

// Decision validation
logger.Info("Checking decision status", "decision", rar.Status.Decision, "decidedBy", rar.Status.DecidedBy)

// Early exits
logger.Info("Skipping RAR (no decision made)")
logger.Info("Rejecting RAR (invalid decision)", "decision", rar.Status.Decision)
logger.Info("Skipping RAR (already decided) - IDEMPOTENCY", "existingDecidedBy", rar.Status.DecidedBy)

// Mutation
logger.Info("Populating DecidedBy field", "authenticatedUser", authCtx.Username)
logger.Info("Webhook audit event emitted", "correlationID", rar.Name)

// Success
logger.Info("RAR mutation complete", "decidedBy", rar.Status.DecidedBy, "decision", rar.Status.Decision)

// Errors
logger.Error(err, "Failed to decode RemediationApprovalRequest")
logger.Error(err, "Authentication failed")
```

**Validation**: Logs captured 21 webhook invocations across 12 tests ✅

---

### **Fix #3: Certwatcher File Access Errors** ✅ **VALIDATED**

**Root Cause**: Manager's `certwatcher` monitors TLS cert files for hot-reload. In parallel execution (12 processes), when Process A completes and deletes its cert directory, Process B's certwatcher tries to re-read the deleted files.

**Error Before Fix**:
```
ERROR controller-runtime.certwatcher error re-reading certificate
"error": "open /var/folders/.../tls.crt: no such file or directory"
```

**Fix Applied**:
```go
// test/integration/authwebhook/suite_test.go:248-275

// Load TLS certificate once at startup (bypass certwatcher)
certPath := filepath.Join(webhookInstallOptions.LocalServingCertDir, "tls.crt")
keyPath := filepath.Join(webhookInstallOptions.LocalServingCertDir, "tls.key")
cert, err := tls.LoadX509KeyPair(certPath, keyPath)
Expect(err).ToNot(HaveOccurred())

k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{
    // ...
    WebhookServer: webhook.NewServer(webhook.Options{
        Host:    webhookInstallOptions.LocalServingHost,
        Port:    webhookInstallOptions.LocalServingPort,
        CertDir: webhookInstallOptions.LocalServingCertDir,
        TLSOpts: []func(*tls.Config){
            // Provide certificate directly (bypasses certwatcher file monitoring)
            func(config *tls.Config) {
                config.GetCertificate = func(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
                    return &cert, nil
                }
            },
        },
    }),
})
```

**Rationale**:
- Tests use **static certs** (no rotation needed)
- Production uses **Kubernetes cert-manager** (handles rotation externally)
- certwatcher is unnecessary for test environment
- Eliminates file system race conditions in parallel execution

**Test Fixed**:
- ✅ INT-NR-02: Normal lifecycle completion (no webhook)

**Validation**: ❌ **NO certwatcher errors** in test logs ✅

---

## 🔴 **Issue #4: SECURITY BUG IDENTIFIED - Identity Forgery Allowed**

### **Test**: INT-RAR-04 (Identity Forgery Prevention)

### **Status**: ❌ **FAILING** - Test timeout at `helpers.go:72`

---

### **Webhook Diagnostic Logs** (Issue #2 logging revealed the RCA)

```
INFO rar-webhook Webhook invoked operation="UPDATE" namespace="default" name="test-rar-forgery-026d7795"
INFO rar-webhook Checking decision status decision="Approved" decidedBy="malicious-user@example.com"
INFO rar-webhook Skipping RAR (already decided) - IDEMPOTENCY existingDecidedBy="malicious-user@example.com" decision="Approved"
```

**Key Observation**: Webhook sees `decidedBy="malicious-user@example.com"` and returns early (idempotency check).

---

### **Root Cause: Webhook Idempotency Logic Allows Identity Forgery**

**Current Webhook Logic** (`remediationapprovalrequest_handler.go:115-123`):
```go
// Check if decidedBy is already set (preserve existing attribution)
if rar.Status.DecidedBy != "" {
    // Already decided - don't overwrite
    logger.Info("Skipping RAR (already decided) - IDEMPOTENCY",
        "existingDecidedBy", rar.Status.DecidedBy,
        "decision", rar.Status.Decision,
    )
    return admission.Allowed("decision already attributed")
}
```

**Problem**: Webhook checks NEW object's `DecidedBy` field, but **does not compare with OLD object**. This means:
1. User updates RAR status with forged `DecidedBy` value
2. Webhook receives admission request with `DecidedBy` already set
3. Webhook assumes "already decided" and preserves the forged value
4. **Result**: User successfully forges identity ❌

---

### **Security Impact Analysis**

#### **Attack Scenario**:
```yaml
# Step 1: User creates status update with forged identity
apiVersion: kubernaut.ai/v1alpha1
kind: RemediationApprovalRequest
metadata:
  name: test-rar
status:
  decision: Approved               # User makes decision
  decidedBy: victim@example.com    # User forges victim's identity
  decidedAt: "2026-02-03T14:00:00Z"
```

**Expected Behavior** (per BR-AUTH-001, SOC 2 CC8.1):
- Webhook extracts authenticated user from K8s admission request
- Webhook OVERWRITES `decidedBy` with authenticated user
- Audit trail records TRUE operator identity

**Actual Behavior** (SECURITY BUG):
- Webhook sees `decidedBy != ""`
- Webhook preserves forged identity
- Audit trail records FORGED identity
- **Result**: Non-repudiation violated (SOC 2 CC6.8) ❌

#### **SOC 2 Compliance Impact**:
- ❌ **CC8.1 (User Attribution)**: Cannot trust `decidedBy` field
- ❌ **CC6.8 (Non-Repudiation)**: Operators can deny actions by forging identity
- ❌ **CC7.4 (Audit Completeness)**: Audit trail contains false attribution

---

### **Correct Idempotency Logic**

**Goal**: Prevent identity forgery while maintaining true idempotency

**Implementation**: Check OLD object to determine if decision is NEW

```go
// pkg/authwebhook/remediationapprovalrequest_handler.go

func (h *RemediationApprovalRequestAuthHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
    logger := ctrl.Log.WithName("rar-webhook")
    
    logger.Info("Webhook invoked",
        "operation", req.Operation,
        "namespace", req.Namespace,
        "name", req.Name,
    )
    
    rar := &remediationv1.RemediationApprovalRequest{}
    err := json.Unmarshal(req.Object.Raw, rar)
    if err != nil {
        logger.Error(err, "Failed to decode RemediationApprovalRequest")
        return admission.Errored(http.StatusBadRequest, ...)
    }
    
    // NEW: Decode OLD object to check if decision is truly new
    var oldRAR *remediationv1.RemediationApprovalRequest
    if len(req.OldObject.Raw) > 0 {
        oldRAR = &remediationv1.RemediationApprovalRequest{}
        if err := json.Unmarshal(req.OldObject.Raw, oldRAR); err != nil {
            logger.Error(err, "Failed to decode old RemediationApprovalRequest")
            return admission.Errored(http.StatusBadRequest, ...)
        }
    }
    
    logger.Info("Checking decision status",
        "newDecision", rar.Status.Decision,
        "newDecidedBy", rar.Status.DecidedBy,
        "oldDecision", func() string {
            if oldRAR != nil {
                return string(oldRAR.Status.Decision)
            }
            return "(no old object)"
        }(),
    )
    
    // Check if a decision has been made
    if rar.Status.Decision == "" {
        logger.Info("Skipping RAR (no decision made)")
        return admission.Allowed("no decision made")
    }
    
    // Validate decision enum
    validDecisions := map[remediationv1.ApprovalDecision]bool{
        remediationv1.ApprovalDecisionApproved: true,
        remediationv1.ApprovalDecisionRejected: true,
        remediationv1.ApprovalDecisionExpired:  true,
    }
    if !validDecisions[rar.Status.Decision] {
        logger.Info("Rejecting RAR (invalid decision)", "decision", rar.Status.Decision)
        return admission.Denied(...)
    }
    
    // SECURITY: Check if this is a NEW decision (true idempotency)
    // Compare OLD object's decision with NEW object's decision
    // Only populate DecidedBy if decision is TRULY new
    isNewDecision := oldRAR == nil || oldRAR.Status.Decision == ""
    
    if !isNewDecision {
        // Decision already exists in OLD object - preserve existing attribution
        logger.Info("Skipping RAR (decision already exists in old object) - TRUE IDEMPOTENCY",
            "oldDecision", oldRAR.Status.Decision,
            "oldDecidedBy", oldRAR.Status.DecidedBy,
            "newDecision", rar.Status.Decision,
        )
        return admission.Allowed("decision already attributed")
    }
    
    // SECURITY: This is a NEW decision - OVERWRITE any user-provided DecidedBy
    // Even if user sets DecidedBy in their request, webhook must use authenticated identity
    if rar.Status.DecidedBy != "" {
        logger.Info("SECURITY: Overwriting user-provided DecidedBy (forgery prevention)",
            "userProvidedValue", rar.Status.DecidedBy,
        )
    }
    
    // Extract authenticated user
    authCtx, err := h.authenticator.ExtractUser(ctx, &req.AdmissionRequest)
    if err != nil {
        logger.Error(err, "Authentication failed")
        return admission.Denied(...)
    }
    
    logger.Info("User authenticated", "username", authCtx.Username, "uid", authCtx.UID)
    
    // Populate authentication fields (OVERWRITE any user-provided values)
    logger.Info("Populating DecidedBy field",
        "authenticatedUser", authCtx.Username,
        "decision", rar.Status.Decision,
    )
    rar.Status.DecidedBy = authCtx.Username
    now := metav1.Now()
    rar.Status.DecidedAt = &now
    
    // ... audit event code (unchanged) ...
    
    logger.Info("RAR mutation complete",
        "decidedBy", rar.Status.DecidedBy,
        "decidedAt", rar.Status.DecidedAt.Time,
        "decision", rar.Status.Decision,
    )
    
    return admission.PatchResponseFromRaw(req.Object.Raw, marshaledRAR)
}
```

---

### **Why This Fix is Correct**

#### **Idempotency** (Preserves Existing Decisions):
```
OLD object: decision="" (no decision yet)
NEW object: decision="Approved", decidedBy="" (user makes decision)
Webhook: POPULATE DecidedBy from authenticated user ✅

OLD object: decision="Approved", decidedBy="alice@example.com" (already decided)
NEW object: decision="Approved", decidedBy="alice@example.com" (no change)
Webhook: SKIP (true idempotency - decision already exists) ✅
```

#### **Security** (Prevents Identity Forgery):
```
OLD object: decision="" (no decision yet)
NEW object: decision="Approved", decidedBy="victim@example.com" (user forges identity)
Webhook: OVERWRITE DecidedBy with authenticated user ✅

Result: decidedBy="attacker@example.com" (real identity captured)
```

---

### **Test Expectation Validation**

**INT-RAR-04 Test Code** (line 237, 241):
```go
updateStatusAndWaitForWebhook(ctx, k8sClient, rar,
    func() {
        rar.Status.Decision = remediationv1.ApprovalDecisionApproved
        rar.Status.DecisionMessage = "Approved"
        rar.Status.DecidedBy = forgedIdentity // USER TRIES TO SET THIS
    },
    func() bool {
        // Wait for webhook to overwrite DecidedBy
        return rar.Status.DecidedBy != "" && rar.Status.DecidedBy != forgedIdentity
    },
)
```

**Test Comment** (line 221):
> "User-provided DecidedBy MUST be overwritten by webhook"

**Test Expectation**: ✅ **CORRECT** - Webhook SHOULD overwrite forged identity

**Current Webhook Behavior**: ❌ **INCORRECT** - Webhook preserves forged identity

**Conclusion**: Test is CORRECT, production code has SECURITY BUG

---

## 🔍 **Diagnostic Evidence from Webhook Logs**

### **Working Tests** (INT-RAR-01, INT-RAR-02, INT-RAR-05, INT-RAR-06)

```
# INT-RAR-01: Approval (NO DecidedBy pre-set)
INFO rar-webhook Webhook invoked name="test-rar-int-rar-01-60f62d31"
INFO rar-webhook Checking decision status decision="Approved" decidedBy=""
INFO rar-webhook Populating DecidedBy field authenticatedUser="admin"
INFO rar-webhook RAR mutation complete decidedBy="admin"
✅ TEST PASSES

# INT-RAR-02: Rejection (NO DecidedBy pre-set)
INFO rar-webhook Webhook invoked name="test-rar-int-rar-02-e4059520"
INFO rar-webhook Checking decision status decision="Rejected" decidedBy=""
INFO rar-webhook Populating DecidedBy field authenticatedUser="admin"
INFO rar-webhook RAR mutation complete decidedBy="admin"
✅ TEST PASSES

# INT-RAR-05: Audit Event Emission (NO DecidedBy pre-set)
INFO rar-webhook Webhook invoked name="test-rar-audit-37162cf4"
INFO rar-webhook Checking decision status decision="Approved" decidedBy=""
INFO rar-webhook Populating DecidedBy field authenticatedUser="admin"
INFO rar-webhook RAR mutation complete decidedBy="admin"
✅ TEST PASSES
```

---

### **Failing Test** (INT-RAR-04)

```
# INT-RAR-04: Identity Forgery (DecidedBy PRE-SET by user)
INFO rar-webhook Webhook invoked name="test-rar-forgery-026d7795"
INFO rar-webhook Checking decision status decision="Approved" decidedBy="malicious-user@example.com"
INFO rar-webhook Skipping RAR (already decided) - IDEMPOTENCY existingDecidedBy="malicious-user@example.com" decision="Approved"
❌ TEST FAILS (timeout after 10s - webhook doesn't mutate)

Webhook Behavior:
  ✅ Webhook WAS invoked (not a timing issue)
  ❌ Webhook returned early (idempotency check triggered incorrectly)
  ❌ DecidedBy field NOT overwritten (forged identity preserved)
  
Test Expectation:
  ✅ User sets DecidedBy to forged value
  ✅ Webhook should OVERWRITE with authenticated user
  ❌ Webhook preserves forged value (SECURITY BUG)
```

---

## 🎯 **Root Cause Summary**

### **Issue #1: Invalid Resource Names** ✅ **FIXED**
- **Cause**: Uppercase in K8s resource names
- **Fix**: `strings.ToLower(testSuffix)`
- **Tests Fixed**: INT-RAR-01, INT-RAR-02 (+2 tests)

### **Issue #2: Insufficient Logging** ✅ **FIXED**
- **Cause**: No diagnostic logs in webhook handler
- **Fix**: Added structured logging with `ctrl.Log`
- **Outcome**: Revealed Issue #4 (security bug)

### **Issue #3: Certwatcher File Errors** ✅ **FIXED**
- **Cause**: Parallel process cert deletion
- **Fix**: Bypass certwatcher with static cert via `TLSOpts`
- **Tests Fixed**: INT-NR-02 (+1 test)

### **Issue #4: Identity Forgery SECURITY BUG** 🔴 **NEEDS FIX**
- **Cause**: Webhook uses NEW object only (doesn't check OLD object for true idempotency)
- **Impact**: User can forge `DecidedBy` field, violating SOC 2 CC8.1/CC6.8
- **Fix Required**: Compare OLD vs NEW object to determine if decision is truly new
- **Tests Blocked**: INT-RAR-04 (1 test)

---

## 🔧 **Recommended Security Fix**

### **Implementation**

**File**: `pkg/authwebhook/remediationapprovalrequest_handler.go`

**Changes Required**:
1. Decode `req.OldObject` to get previous RAR state
2. Check if `oldRAR.Status.Decision == ""` (decision is NEW)
3. If NEW decision: OVERWRITE any user-provided `DecidedBy` (security)
4. If EXISTING decision: SKIP (true idempotency)

**Pseudo-code**:
```go
// Decode OLD object
var oldRAR *remediationv1.RemediationApprovalRequest
if len(req.OldObject.Raw) > 0 {
    oldRAR = &remediationv1.RemediationApprovalRequest{}
    json.Unmarshal(req.OldObject.Raw, oldRAR)
}

// Check if this is a NEW decision
isNewDecision := oldRAR == nil || oldRAR.Status.Decision == ""

if !isNewDecision {
    // Decision already exists in OLD object - true idempotency
    return admission.Allowed("decision already attributed")
}

// NEW decision - OVERWRITE any user-provided DecidedBy (security)
if rar.Status.DecidedBy != "" {
    logger.Info("SECURITY: Overwriting user-provided DecidedBy",
        "userProvidedValue", rar.Status.DecidedBy,
    )
}

// Extract authenticated user and populate (security-critical)
authCtx, err := h.authenticator.ExtractUser(ctx, &req.AdmissionRequest)
rar.Status.DecidedBy = authCtx.Username // OVERWRITE
rar.Status.DecidedAt = &now
```

---

### **Test Validation After Fix**

**Expected Results**:
```
INT-RAR-01: Approval (no forgery attempt) → ✅ PASS (already passing)
INT-RAR-02: Rejection (no forgery attempt) → ✅ PASS (already passing)
INT-RAR-04: Identity Forgery Prevention → ✅ PASS (will pass after fix)
INT-RAR-05: Webhook Audit Event → ✅ PASS (already passing)
INT-RAR-06: DecidedBy Preservation → ✅ PASS (already passing)

Target: 12/12 tests passing (100%)
```

---

## 📊 **Test Results Comparison**

| Iteration | Passing | Failing | Issues Identified | Issues Fixed |
|-----------|---------|---------|-------------------|--------------|
| **Before Fixes** | 9/12 (75%) | 3 | Webhook timing | None |
| **Manager Pattern** | 8/12 (67%) | 4 | certwatcher + naming | None |
| **All Fixes Applied** | 11/12 (92%) | 1 | Security bug | Naming (2), certwatcher (1) |
| **After Security Fix** | 12/12 (100%) ✅ | 0 | None | All (4) |

---

## 🔒 **Security Fix Priority**

**Priority**: **P0 - CRITICAL SECURITY BUG**

**Justification**:
- Violates SOC 2 compliance requirements (CC8.1, CC6.8)
- Allows identity forgery (operators can frame each other)
- Affects audit trail integrity (non-repudiation violated)
- Production vulnerability (not just test issue)

**Affected Deployments**:
- ✅ Tests caught the bug (INT-RAR-04 failing)
- ⚠️ Production may be vulnerable (if users craft malicious status updates)
- ⚠️ E2E test may miss this (if E2E doesn't test forgery scenario)

---

## ✅ **Files Modified**

### **Test Files**:
1. `test/integration/authwebhook/suite_test.go`
   - Added `crypto/tls` import
   - Implemented certwatcher bypass (TLSOpts with static cert)
   - Lines: +23/-10

2. `test/integration/authwebhook/remediationapprovalrequest_test.go`
   - Added `strings` import
   - Fixed resource naming (lowercase conversion)
   - Lines: +2/-1

### **Production Code**:
3. `pkg/authwebhook/remediationapprovalrequest_handler.go`
   - Added `ctrl` import
   - Implemented structured logging (entry, decision check, idempotency, mutation, exit)
   - Lines: +40/-5
   - **TODO**: Implement security fix (OLD object comparison)

---

## 📚 **Related Documentation**

### **Standards**:
- [LOGGING_STANDARD.md](../architecture/LOGGING_STANDARD.md) - Kubernaut logging patterns (ctrl.Log for CRD controllers)
- [BR-AUTH-001](../requirements/BR-AUTH-001-user-attribution.md) - User Attribution (SOC 2 CC8.1)

### **Architecture**:
- [ADR-034](../architecture/decisions/ADR-034-unified-audit-table-design.md) - Audit table design (v1.7)
- [DD-WEBHOOK-003](../architecture/decisions/DD-WEBHOOK-003-webhook-complete-audit-pattern.md) - Webhook audit pattern

### **Previous RCAs**:
- [RCA_AUTHWEBHOOK_INT_E2E_FAILURES_FEB_03_2026.md](./RCA_AUTHWEBHOOK_INT_E2E_FAILURES_FEB_03_2026.md) - Initial webhook timing investigation
- [AUTHWEBHOOK_INT_TEST_FIX_INVESTIGATION_FEB_03_2026.md](./AUTHWEBHOOK_INT_TEST_FIX_INVESTIGATION_FEB_03_2026.md) - Issue #1, #2, #3 investigation

---

## 🎯 **Next Steps**

### **Immediate (P0 - Security)**:
1. 🔴 **Implement OLD object comparison** (security fix)
2. 🧪 **Run AuthWebhook INT tests** (validate 12/12 passing)
3. 🧪 **Run AuthWebhook unit tests** (ensure no regressions)
4. 📝 **Update BR-AUTH-001** (document OLD object check requirement)
5. 📝 **Create DD-SECURITY-001** (identity forgery prevention pattern)

### **Short-Term (P1 - E2E Validation)**:
1. 🧪 **Run E2E tests** (test-e2e-remediationorchestrator)
2. 📊 **Validate E2E-RO-AUD006-001** (should pass with security fix)
3. 📝 **Update RCA document** with final resolution

### **Long-Term (P2 - Documentation)**:
1. 📝 **Update TEST_PLAN_BR_AUDIT_006** (add security test scenario)
2. 📝 **Update DD-WEBHOOK-003** (document OLD object pattern)
3. 📝 **Create handoff document** (security fix summary)

---

**Status**: ✅ **11/12 tests passing**, 🔴 **1 SECURITY BUG identified and fix designed**

**Confidence**: **95%** that OLD object comparison will fix INT-RAR-04 and prevent identity forgery

**Evidence**:
1. ✅ Webhook logs prove idempotency check is triggering incorrectly
2. ✅ Test expectation is correct (user-provided DecidedBy should be overwritten)
3. ✅ K8s admission.Request provides OldObject for comparison
4. ✅ Pattern validated in other webhooks (check old vs new state)
