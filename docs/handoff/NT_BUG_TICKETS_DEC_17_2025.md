# Notification Service: Bug Tickets from Integration/E2E Test Analysis

**Date**: December 17, 2025
**Source**: Complete time.Sleep() remediation and test investigation
**Total Issues**: 8 unique bugs (affects 13 test failures)

---

## 🐛 **Priority 1 (P1) - Critical Bugs**

### **NT-BUG-001: Duplicate Audit Event Emission Across Reconciles**

**Priority**: P1 - Critical
**Affected Tests**: 4 (3 integration + 1 E2E)
- `controller_audit_emission_test.go:107` - Expected 1 event, got 3
- `02_audit_correlation_test.go:206` (E2E) - Expected 9 events, got 27
- `04_failed_delivery_audit_test.go:383` (E2E) - Expected 2 events, got 3

**Description**:
Audit helper emits the same event on every controller reconcile instead of once per lifecycle stage. This causes 3x duplication for successful notifications (initial reconcile, status update reconcile, final reconcile).

**Root Cause**:
Audit event emission in `internal/controller/notification/audit.go` is called on every reconcile without checking if event was already emitted for this lifecycle stage.

**Business Impact**:
- ❌ Violates DD-AUDIT-004 (accurate audit trail)
- ❌ Bloats audit database with duplicate events
- ❌ Makes audit queries unreliable (3x events per action)
- ❌ Affects compliance reporting accuracy

**Recommended Fix**:
1. Add idempotency tracking to controller (e.g., `auditEventsEmitted map[string]bool`)
2. Only emit audit event on lifecycle stage transition (Pending→Sent, Sent→Acknowledged, etc.)
3. Alternative: Track emitted events in CRD status (e.g., `status.auditEventIDs []string`)

**Acceptance Criteria**:
- ✅ Exactly 1 audit event per lifecycle stage
- ✅ No duplicate events across multiple reconciles
- ✅ All 4 tests pass

**Estimated Effort**: 4-6 hours

---

### **NT-BUG-002: Duplicate Delivery Attempt Recording**

**Priority**: P1 - Critical
**Affected Tests**: 1
- `status_update_conflicts_test.go:414` - Expected 1 attempt, got 5

**Description**:
Delivery attempts are recorded on every reconcile instead of once per actual delivery attempt. Similar root cause to NT-BUG-001.

**Root Cause**:
`recordDeliveryAttempt()` in controller is called on every reconcile without checking if attempt was already recorded.

**Business Impact**:
- ❌ Violates BR-NOT-015 (accurate delivery tracking)
- ❌ Incorrect retry count reporting
- ❌ Makes debugging difficult (cannot distinguish real vs duplicate attempts)

**Recommended Fix**:
1. Add delivery attempt tracking to CRD status (e.g., `status.lastRecordedAttempt int`)
2. Only record new attempts when `attemptCount` increases
3. Alternative: Use attempt UUID to deduplicate in recording logic

**Acceptance Criteria**:
- ✅ Exactly 1 delivery attempt record per actual attempt
- ✅ No duplicate records across reconciles
- ✅ Test passes

**Estimated Effort**: 2-3 hours

---

## 🐛 **Priority 2 (P2) - Important Bugs**

### **NT-BUG-003: No PartiallySent State for Partial Channel Failures**

**Priority**: P2 - Important
**Affected Tests**: 1
- `multichannel_retry_test.go:177` - Notification stuck in retry when Slack fails (503), Console succeeds

**Description**:
Controller doesn't implement `PartiallySent` state when some channels succeed and others fail (with retryable errors). Notification enters indefinite retry loop instead.

**Root Cause**:
Controller logic treats any channel failure as notification failure, even when other channels succeeded. No `PartiallySent` phase implemented.

**Business Impact**:
- ❌ Users don't know some channels succeeded
- ❌ Unnecessary retries for already-successful channels
- ❌ Poor user experience (notification shows as "in progress" when partially complete)

**Recommended Fix**:
1. Add `NotificationPhasePartiallySent` to API types
2. Update controller logic:
   - If `successfulDeliveries > 0` AND `failedDeliveries > 0` → `PartiallySent`
   - Only retry failed channels, not successful ones
3. Add idempotency: Skip channels with `status.deliveredChannels` already set

**Acceptance Criteria**:
- ✅ PartiallySent state when partial success
- ✅ Retries only affect failed channels
- ✅ Test passes

**Estimated Effort**: 6-8 hours (requires API change)

---

### **NT-BUG-004: Duplicate Channels Cause Permanent Failure**

**Priority**: P2 - Important
**Affected Tests**: 1
- `data_validation_test.go:521` - Duplicate channels (2x Console) → Failed instead of Sent

**Description**:
When notification has duplicate channels, controller deduplicates correctly ("Channel already delivered successfully, skipping") but then marks notification as permanently failed with 0 failed deliveries.

**Root Cause**:
Deduplication logic removes channel from delivery queue, but controller interprets empty queue as "no deliveries possible" → permanent failure.

**Business Impact**:
- ❌ Valid notifications marked as failed
- ❌ Confusing error message ("permanently failed" with 0 failed deliveries)
- ❌ Poor idempotency (duplicate channel spec is valid)

**Recommended Fix**:
1. After deduplication, check if `successfulDeliveries > 0`
2. If yes → Mark as `Sent` (deliveries already complete)
3. If no → Continue with remaining channels

**Acceptance Criteria**:
- ✅ Duplicate channels result in `Sent` state
- ✅ No "permanently failed" with 0 failures
- ✅ Test passes

**Estimated Effort**: 2-3 hours

---

## 🐛 **Priority 3 (P3) - Minor Issues**

### **NT-TEST-001: Actor ID Naming Mismatch in E2E Tests**

**Priority**: P3 - Minor
**Affected Tests**: 1
- `04_failed_delivery_audit_test.go:219` (E2E) - Expected `"notification"`, got `"notification-controller"`

**Description**:
E2E test expects `actor_id: "notification"` but controller uses `"notification-controller"` as service name.

**Root Cause**:
Test expectation doesn't match actual implementation.

**Business Impact**:
- ⚠️  Minimal - cosmetic inconsistency
- ✅ Audit events are correct, just naming difference

**Recommended Fix** (Choose one):
1. **Option A**: Update test to expect `"notification-controller"` ✅ **RECOMMENDED**
2. **Option B**: Change controller to use `"notification"` as actor_id

**Acceptance Criteria**:
- ✅ Test passes with consistent actor_id
- ✅ No functional changes to audit

**Estimated Effort**: 30 minutes

---

### **NT-TEST-002: Mock Slack Server State Pollution**

**Priority**: P3 - Minor
**Affected Tests**: 1 (intermittent)
- `performance_concurrent_test.go:110` - All 10 notifications fail (mock Slack in failure mode from previous test)

**Description**:
Mock Slack server state persists across tests, causing subsequent tests to fail when previous tests configured failure mode.

**Root Cause**:
Mock server not reset to success mode in `AfterEach` or test lacks isolation.

**Business Impact**:
- ⚠️  Test flakiness depending on execution order
- ❌ CI/CD instability

**Recommended Fix**:
1. Add mock server reset to `AfterEach` in suite:
   ```go
   AfterEach(func() {
       mockSlackServer.Reset() // Reset to success mode
   })
   ```
2. Alternative: Each test explicitly sets mock mode in `BeforeEach`

**Acceptance Criteria**:
- ✅ Tests pass regardless of execution order
- ✅ Mock server isolated between tests
- ✅ No intermittent failures

**Estimated Effort**: 1-2 hours

---

## 📊 **Bug Summary Table**

| Bug ID | Priority | Description | Tests Affected | Effort |
|--------|----------|-------------|----------------|--------|
| NT-BUG-001 | P1 | Duplicate audit event emission | 4 | 4-6h |
| NT-BUG-002 | P1 | Duplicate delivery attempt recording | 1 | 2-3h |
| NT-BUG-003 | P2 | No PartiallySent state | 1 | 6-8h |
| NT-BUG-004 | P2 | Duplicate channels → failure | 1 | 2-3h |
| NT-TEST-001 | P3 | Actor ID naming mismatch | 1 | 0.5h |
| NT-TEST-002 | P3 | Mock server state pollution | 1 | 1-2h |

**Total**: 6 unique bugs affecting 9 test failures (4 additional failures are infrastructure-related)

---

## 🎯 **Recommended Fix Order**

### **Sprint 1: Critical Issues (P1)**
1. **NT-BUG-001** - Duplicate audit emission (4-6 hours)
   - **Impact**: Fixes 4 tests (3 integration + 1 E2E)
   - **Value**: Critical for audit compliance

2. **NT-BUG-002** - Duplicate delivery recording (2-3 hours)
   - **Impact**: Fixes 1 test
   - **Value**: Critical for accurate delivery tracking

**Sprint 1 Total**: 6-9 hours → Fixes 5 tests

---

### **Sprint 2: Important Issues (P2)**
3. **NT-BUG-004** - Duplicate channels handling (2-3 hours)
   - **Impact**: Fixes 1 test
   - **Value**: Improves idempotency
   - **Quick win**: Less effort than NT-BUG-003

4. **NT-BUG-003** - PartiallySent state (6-8 hours)
   - **Impact**: Fixes 1 test
   - **Value**: Better user experience for multi-channel
   - **Higher effort**: Requires API change

**Sprint 2 Total**: 8-11 hours → Fixes 2 more tests

---

### **Sprint 3: Minor Issues (P3)**
5. **NT-TEST-001** - Actor ID fix (30 minutes)
   - **Impact**: Fixes 1 E2E test
   - **Value**: Test consistency

6. **NT-TEST-002** - Mock server isolation (1-2 hours)
   - **Impact**: Fixes intermittent failure
   - **Value**: Test stability

**Sprint 3 Total**: 1.5-2.5 hours → Fixes 2 more tests

---

## 📋 **Infrastructure Issues (Not Bugs)**

### **audit_integration_test.go:76** (6 tests)
**Status**: ✅ **Not a bug** - Tests correctly `Fail()` when Data Storage unavailable
**Resolution**: Start Data Storage infrastructure (already running in our environment)
**Command**: `podman-compose -f podman-compose.notification.test.yml up -d`

---

## 🎓 **Lessons for Future Development**

### **1. Idempotency Patterns**
- **Always check** if action was already performed before repeating
- **Track state** in CRD status or in-memory map
- **Test for** duplicate reconcile scenarios

### **2. Test Isolation**
- **Reset mocks** in `AfterEach` hooks
- **Use unique** test identifiers to avoid collisions
- **Filter queries** by namespace to avoid concurrent test interference

### **3. Audit Event Emission**
- **Emit once** per lifecycle transition, not per reconcile
- **Use correlation IDs** to track event chains
- **Validate counts** in tests (not just "not nil")

---

## ✅ **Next Steps**

1. **Create GitHub Issues** for each bug (NT-BUG-001 through NT-TEST-002)
2. **Assign to Sprint 1** (P1 bugs)
3. **Update test expectations** for known issues (add `Skip()` with ticket reference)
4. **Start with NT-BUG-001** (highest impact: fixes 4 tests)
5. **Document fixes** in ADRs for idempotency patterns

---

**Document Created**: December 17, 2025
**Total Bugs**: 6 unique issues
**Total Tests Affected**: 9 (+ 6 infrastructure)
**Total Estimated Effort**: 16-21.5 hours (over 3 sprints)


