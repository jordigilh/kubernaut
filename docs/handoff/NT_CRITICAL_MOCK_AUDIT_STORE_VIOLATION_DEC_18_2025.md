# 🚨 CRITICAL: Mock Audit Store in Integration Tests - MANDATE VIOLATION

**Date**: December 18, 2025, 17:10 UTC
**Severity**: **CRITICAL** - Direct violation of testing mandate
**Discovered By**: User code review
**Impact**: 107 of 113 tests using MOCK instead of REAL Data Storage
**Mandate**: DD-AUDIT-003 + 03-testing-strategy.mdc

---

## **The Violation**

**FACT**: Only **6 of 113** integration tests use the REAL Data Storage service
**MANDATE**: **ALL tests that emit audit events MUST use REAL Data Storage** (no mocks in integration)

**Current Reality**:
- ✅ 6 tests in `audit_integration_test.go`: Use REAL Data Storage ✅
- ❌ **107 other tests**: Use MOCK `testAuditStore` ❌ **VIOLATION**

---

## **Evidence**

### **File**: `test/integration/notification/suite_test.go`

**Line 229** - Controller instantiated with MOCK:
```go
// Create mock audit store for testing audit emission
// This captures audit events emitted by the controller during reconciliation
testAuditStore = NewTestAuditStore()  // ❌ IN-MEMORY MOCK

// Create controller with all dependencies including audit (Defense-in-Depth Layer 4)
err = (&notification.NotificationRequestReconciler{
    Client:         k8sManager.GetClient(),
    Scheme:         k8sManager.GetScheme(),
    ConsoleService: consoleService,
    SlackService:   slackService,
    Sanitizer:      sanitizer,
    AuditStore:     testAuditStore,  // ❌ MOCK INJECTED HERE (line 238)
    AuditHelpers:   auditHelpers,
}).SetupWithManager(k8sManager)
```

### **File**: `test/integration/notification/controller_audit_emission_test.go`

**Lines 99-102** - Tests verify MOCK, not real service:
```go
// DEFENSE-IN-DEPTH VERIFICATION: Check audit store for sent event
Eventually(func() int {
    return len(testAuditStore.GetEventsByType("notification.message.sent"))  // ❌ CHECKING MOCK
}, 5*time.Second, 200*time.Millisecond).Should(BeNumerically(">=", 1),
    "Controller should emit notification.message.sent audit event")
```

**Impact**: ~30+ tests in `controller_audit_emission_test.go` all verify mock, not real audit trail.

---

## **Why This Is Critical**

### **Testing Mandate Violation**

**Per `03-testing-strategy.mdc`** (Defense-in-Depth Strategy):
```markdown
Integration Tests (>50% coverage):
- **MOCK**: NONE - Use real services in Kind cluster
- **REAL**: Cross-service interactions, CRD coordination, K8s API
```

**Per `DD-AUDIT-003`** (Audit Infrastructure Mandate):
```markdown
Audit infrastructure is MANDATORY
Integration tests MUST use real services (no Skip() allowed)
```

### **What We're NOT Testing**

By using mocks, we're NOT validating:
1. ❌ **Audit events actually written to PostgreSQL**
2. ❌ **Data Storage REST API integration**
3. ❌ **Network failures between controller and DS**
4. ❌ **Async buffered writes and batch flushing**
5. ❌ **Database schema compatibility**
6. ❌ **OpenAPI client correctness** (DD-API-001)
7. ❌ **ADR-034 field compliance in actual storage**
8. ❌ **Query performance with real data**
9. ❌ **Graceful degradation when DS unavailable**
10. ❌ **Audit trail completeness end-to-end**

---

## **Test Breakdown**

### **Tests Using REAL Data Storage** (6 tests - ✅ CORRECT):

**File**: `audit_integration_test.go`
1. ✅ BR-NOT-062: Unified Audit Table Integration
2. ✅ BR-NOT-062: Async Buffered Audit Writes
3. ✅ BR-NOT-063: Graceful Audit Degradation
4. ✅ Graceful Shutdown
5. ✅ BR-NOT-064: Audit Event Correlation
6. ✅ ADR-034: Unified Audit Table Format

**These tests**:
- Check `BeforeEach` for Data Storage availability
- Use real `dsClient.QueryAuditEventsWithResponse()`
- Validate events in **PostgreSQL**
- **This is the CORRECT pattern**

### **Tests Using MOCK** (~107 tests - ❌ VIOLATION):

**File**: `controller_audit_emission_test.go` (~30 tests)
- ❌ Audit on Successful Delivery (Console)
- ❌ Audit on Slack Delivery
- ❌ Audit on Failed Delivery
- ❌ Audit on Acknowledged Notification
- ❌ Multi-Channel Audit
- ❌ Correlation ID Test
- ... and ~24 more

**File**: `multichannel_retry_test.go`, `status_update_conflicts_test.go`, etc. (~77 tests)
- All other integration tests that trigger audit emission
- None verify audit events in real Data Storage
- **This is INCORRECT**

---

## **Required Fix**

### **Option A: Use Real Audit Store for ALL Tests** (RECOMMENDED)

**Change**: `suite_test.go` lines 224-238

**BEFORE** (Current - WRONG):
```go
// Create mock audit store for testing audit emission
testAuditStore = NewTestAuditStore()  // ❌ MOCK

err = (&notification.NotificationRequestReconciler{
    // ...
    AuditStore:     testAuditStore,  // ❌ MOCK
    AuditHelpers:   auditHelpers,
}).SetupWithManager(k8sManager)
```

**AFTER** (Correct):
```go
// Create REAL audit store using Data Storage service (DD-AUDIT-003 mandate)
// Requires infrastructure: test/integration/notification/podman-compose.notification.test.yml
dataStorageURL := os.Getenv("DATA_STORAGE_URL")
if dataStorageURL == "" {
    dataStorageURL = "http://localhost:18110"
}

realAuditStore, err := audit.NewAuditStore(audit.AuditStoreConfig{
    DataStorageURL: dataStorageURL,
    ServiceName:    "notification-controller",
    BufferSize:     1000,
    BatchSize:      10,
    FlushInterval:  100 * time.Millisecond,
    MaxRetries:     3,
})
Expect(err).ToNot(HaveOccurred(), "Failed to create real audit store")

err = (&notification.NotificationRequestReconciler{
    // ...
    AuditStore:     realAuditStore,  // ✅ REAL audit store
    AuditHelpers:   auditHelpers,
}).SetupWithManager(k8sManager)
```

**Impact**:
- ✅ ALL 113 tests now use real Data Storage
- ✅ Complete end-to-end validation
- ✅ Compliance with DD-AUDIT-003 mandate
- ⚠️ Tests require infrastructure (already exists)

### **Option B: Skip Tests Without Infrastructure** (NOT RECOMMENDED)

Add infrastructure check to `controller_audit_emission_test.go` `BeforeEach`:
```go
BeforeEach(func() {
    // Check if Data Storage is available
    _, err := http.Get("http://localhost:18110/health")
    if err != nil {
        Skip("Data Storage not available (required for audit tests)")
    }

    if realAuditStore != nil {
        realAuditStore.Clear()  // Clear real audit store, not mock
    }
})
```

**Why NOT RECOMMENDED**:
- ❌ Allows tests to pass without validating critical functionality
- ❌ Violates DD-AUDIT-003 ("no Skip() allowed")
- ❌ False confidence in test coverage

---

## **Migration Path**

### **Phase 1: Immediate** (Required for 100% mandate compliance)

1. ✅ **Fix podman-compose container naming** (Issue 1 - prerequisite)
2. ⏳ **Replace testAuditStore with real audit store** (suite_test.go)
3. ⏳ **Remove testAuditStore mock implementation** (cleanup)
4. ⏳ **Update controller_audit_emission_test.go** (remove Clear() calls or update to clear real store)
5. ⏳ **Run full test suite** (verify all 113 tests pass with real DS)

### **Phase 2: Validation** (Verify mandate compliance)

1. ⏳ **Verify ALL tests write to PostgreSQL** (spot check via psql)
2. ⏳ **Confirm no mocks used in integration** (grep for testAuditStore usage)
3. ⏳ **Document compliance** (update test documentation)

### **Phase 3: Enforcement** (Prevent regression)

1. ⏳ **Add linter check**: Fail build if `testAuditStore` or `NewTestAuditStore` found in integration tests
2. ⏳ **Update testing guidelines**: Explicitly forbid mocks in integration layer
3. ⏳ **Code review checklist**: "Does this integration test use real services?"

---

## **Expected Test Results After Fix**

**Current** (with mock):
```
Without Infrastructure: 107/113 passing (6 audit tests fail)
With Infrastructure: 113/113 passing
```

**After Fix** (with real audit store):
```
Without Infrastructure: 0/113 passing (ALL audit tests fail - ✅ CORRECT)
With Infrastructure: 113/113 passing (ALL tests use real DS - ✅ CORRECT)
```

**Why This Is Better**:
- Forces infrastructure to be available (as it should be)
- No false confidence from mock-only validation
- True integration testing

---

## **Urgency Assessment**

**Severity**: **CRITICAL**

**Why**:
1. **Direct mandate violation**: DD-AUDIT-003, 03-testing-strategy.mdc
2. **False confidence**: 107 tests "passing" without validating real audit trail
3. **Production risk**: Bugs in DS integration not caught by tests
4. **OpenAPI validation gap**: DD-API-001 enum fixes only caught by 6 tests, not 113

**Precedent**:
- **Notification Team** found 2 OpenAPI bugs by using real DS client (6 tests)
- **Remediation Team** found 3rd OpenAPI bug (enum value) by using real DS
- **What bugs are hiding** in the 107 tests that use mocks?

---

## **Recommendation**

**IMMEDIATE ACTION REQUIRED**:
1. Fix podman-compose container naming (5 minutes)
2. Replace testAuditStore with real audit store (15 minutes)
3. Verify all 113 tests require infrastructure (5 minutes)
4. Document compliance (10 minutes)

**Total Time**: 35 minutes to mandate compliance

**Risk of NOT Fixing**:
- Production audit trail gaps not caught by tests
- False test coverage metrics
- Continued mandate violations
- Bugs only discovered in production

---

**Status**: 🚨 **CRITICAL VIOLATION** - Immediate fix required
**Owner**: Notification Team
**Mandate Authority**: DD-AUDIT-003, 03-testing-strategy.mdc
**Estimated Fix Time**: 35 minutes


