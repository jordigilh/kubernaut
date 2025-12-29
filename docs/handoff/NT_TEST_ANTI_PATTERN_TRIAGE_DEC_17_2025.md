# Notification Test Anti-Pattern Triage

**Date**: December 17, 2025
**Status**: ✅ **PHASE 1 COMPLETE** | ⏸️ **PHASE 2 DEFERRED** | ⏸️ **PHASE 3 PENDING**
**Priority**: P1 - These violations reduce test reliability and mask real issues
**Authority**: [TESTING_GUIDELINES.md](../development/business-requirements/TESTING_GUIDELINES.md)

---

## 🎉 **PHASE 1 COMPLETE (December 17, 2025)**

### What Was Fixed

| Category | Status | Count Fixed | Automated Enforcement |
|----------|--------|-------------|----------------------|
| **Integration Tests** | ✅ COMPLETE | 19 NULL-TESTING violations | ✅ forbidigo + pre-commit hook |
| **Approved Exceptions** | ✅ DOCUMENTED | 4 time.Sleep() instances | ✅ Excluded in linter config |
| **Skip() Anti-Pattern** | ✅ FIXED | 1 → Changed to Fail() | ✅ Blocked by pre-commit hook |

### New Enforcement Tools

- **`.golangci.yml`**: Forbidigo linter rules detect anti-patterns at build time
- **`.githooks/pre-commit`**: Blocks commits with anti-pattern violations
- **`scripts/setup-githooks.sh`**: One-command developer setup

**📄 Full Phase 1 Report**: [NT_PHASE1_ANTI_PATTERN_FIXES_COMPLETE_DEC_17_2025.md](./NT_PHASE1_ANTI_PATTERN_FIXES_COMPLETE_DEC_17_2025.md)

---

## 🎯 **Executive Summary (Original Triage)**

Triage identified **3 categories of anti-pattern violations** in Notification tests:

| Anti-Pattern | Severity | Original Count | Phase 1 Fixed | Remaining |
|--------------|----------|----------------|---------------|-----------|
| **NULL-TESTING** | 🔴 HIGH | 52 instances | 19 (integration) | 33 (unit - Phase 2) |
| **time.Sleep()** | 🔴 HIGH | 18 instances | 4 documented | 14 (Phase 2 review) |
| **Skip()** | 🟡 MEDIUM | 1 instance | 1 fixed | 0 |

**Test Coverage**: Per [docs/services/crd-controllers/03-workflowexecution/testing-strategy.md](../services/crd-controllers/03-workflowexecution/testing-strategy.md):
- ✅ **Unit Tests**: 239/239 passing (100%) - **but with weak assertions**
- ✅ **Integration Tests**: 105/107 passing (96%) - **but with timing anti-patterns**
- ⚠️  **E2E Tests**: 12/14 passing (85.7%) - **blocked by infrastructure**

---

## 🚫 **Anti-Pattern #1: NULL-TESTING (52 Instances)**

### Definition (from TESTING_GUIDELINES.md)

> **NULL-TESTING**: Weak assertions (not nil, > 0, empty checks) that don't validate business outcomes
>
> ❌ **BAD**: `Expect(event.EventData).ToNot(BeNil())`
> ✅ **GOOD**: `Expect(event.EventData.NotificationID).To(Equal("test-notification"))`

### Impact

- ✅ Test passes even if EventData contains **wrong values**
- ✅ Test passes even if EventData has **incorrect structure**
- ❌ Test does NOT validate **business requirements**

---

### Violations by File

#### **Integration Tests**: 19 Instances

**File**: `test/integration/notification/audit_integration_test.go`

| Line | Violation | Should Assert |
|------|-----------|---------------|
| 392 | `Expect(resp.JSON200).ToNot(BeNil())` | Validate actual response structure |
| 393 | `Expect(resp.JSON200.Data).ToNot(BeNil())` | Validate data contains expected events |
| 464 | `Expect(fetchedEvent.ActorType).ToNot(BeNil())` | `Expect(*fetchedEvent.ActorType).To(Equal("service"))` |
| 466 | `Expect(fetchedEvent.ActorId).ToNot(BeNil())` | `Expect(*fetchedEvent.ActorId).To(Equal("notification-controller"))` |
| 468 | `Expect(fetchedEvent.ResourceType).ToNot(BeNil())` | `Expect(*fetchedEvent.ResourceType).To(Equal("NotificationRequest"))` |
| **512** | `Expect(event.EventData).ToNot(BeNil())` | **⬅️ USER-IDENTIFIED VIOLATION** |

**Recommended Fix for Line 512**:
```go
// ❌ BEFORE (NULL-TESTING anti-pattern)
Expect(event.EventData).ToNot(BeNil(), "Event data must be populated")

// ✅ AFTER (Business outcome validation)
// Unmarshal EventData to structured type (V2.2 pattern)
eventDataMap, ok := event.EventData.(map[string]interface{})
Expect(ok).To(BeTrue(), "EventData should be a map")

var sentData notificationaudit.MessageSentEventData
eventDataBytes, _ := json.Marshal(eventDataMap)
err = json.Unmarshal(eventDataBytes, &sentData)
Expect(err).ToNot(HaveOccurred())

// Validate business requirements (ADR-034 compliance)
Expect(sentData.NotificationID).To(Equal(notificationName))
Expect(sentData.Channel).To(Equal("console"))
Expect(sentData.Subject).To(Equal(notification.Spec.Subject))
Expect(sentData.Priority).To(Equal(string(notification.Spec.Priority)))
```

**File**: `test/integration/notification/controller_audit_emission_test.go`

| Line | Violation | Should Assert |
|------|-----------|---------------|
| 105 | `Expect(events).ToNot(BeEmpty())` | Validate specific event count and types |
| 361 | `Expect(events).ToNot(BeEmpty())` | Validate event correlation IDs match |
| 412 | `Expect(ackEvent.ActorType).ToNot(BeNil())` | `Expect(*ackEvent.ActorType).To(Equal("service"))` |
| 415 | `Expect(ackEvent.ActorId).ToNot(BeNil())` | `Expect(*ackEvent.ActorId).To(Equal("notification-controller"))` |
| 418 | `Expect(ackEvent.ResourceType).ToNot(BeNil())` | `Expect(*ackEvent.ResourceType).To(Equal("NotificationRequest"))` |
| 421 | `Expect(ackEvent.ResourceId).ToNot(BeNil())` | `Expect(*ackEvent.ResourceId).To(Equal(notificationName))` |

**File**: `test/integration/notification/status_update_conflicts_test.go`

| Line | Violation | Should Assert |
|------|-----------|---------------|
| 112 | `ShouldNot(BeEmpty())` | Validate specific delivery attempt fields |
| 414 | `Expect(notif.Status.DeliveryAttempts).ToNot(BeEmpty())` | Validate attempt count and status values |
| 420 | `Expect(errorMsg).ToNot(BeEmpty())` | Validate error message contains expected text |

**Files with Multiple Violations**:
- `error_propagation_test.go`: 5 instances (lines 166, 272, 333, 458, 569)

---

#### **Unit Tests**: 33 Instances

**File**: `test/unit/notification/audit_test.go` - **20 INSTANCES**

| Line Range | Pattern | Recommended Fix |
|------------|---------|-----------------|
| 83-94 | 4x `ActorType/ActorId/ResourceType/ResourceId ToNot(BeNil())` | Validate actual pointer values: `Expect(*event.ActorType).To(Equal("service"))` |
| 117 | `Expect(eventData).ToNot(BeEmpty())` | Validate EventData structure and field values |
| 192-214 | 5x NULL checks for message.failed event | Validate error field values and failure details |
| 232-254 | 5x NULL checks for message.acknowledged event | Validate acknowledgment metadata |
| 279 | Generic `ToNot(BeNil())` check | Validate specific event type and fields |
| 650-685 | 5x NULL checks in parametrized tests | Each parameter should have value assertions |

**Example Fix**:
```go
// ❌ BEFORE
Expect(event.ActorType).ToNot(BeNil())
Expect(event.ActorId).ToNot(BeNil())

// ✅ AFTER (Validate actual values per ADR-034)
Expect(event.ActorType).ToNot(BeNil())
Expect(*event.ActorType).To(Equal("service"), "Actor type should be 'service' per ADR-034")

Expect(event.ActorId).ToNot(BeNil())
Expect(*event.ActorId).To(Equal("notification-controller"), "Actor ID should identify the service")
```

**File**: `test/unit/notification/routing_config_test.go` - **6 INSTANCES**

| Line | Violation | Should Assert |
|------|-----------|---------------|
| 45-46 | `Expect(config).ToNot(BeNil())` + `Expect(config.Route).ToNot(BeNil())` | Validate routing rules and matchers |
| 117 | `Expect(receiver).ToNot(BeNil())` | Validate receiver configuration fields |
| 321 | `Expect(receiver).ToNot(BeNil())` | Validate receiver name and type |
| 420-421 | 2x NULL checks | Validate config reloads correctly with expected values |

**Other Unit Test Files**: 7 instances across:
- `routing_hotreload_test.go`: 2 instances
- `routing_integration_test.go`: 1 instance
- `sanitization/sanitizer_fallback_test.go`: 3 instances
- `status_test.go`: 1 instance

---

### Remediation Strategy

#### **Phase 1: Critical Audit Tests (P0 - Immediate)**
**Files**: `audit_integration_test.go`, `controller_audit_emission_test.go`, `audit_test.go`
**Instances**: 31 (60% of total)
**Timeline**: 1-2 days

**Pattern**:
```go
// ❌ Anti-pattern
Expect(event.EventData).ToNot(BeNil())
Expect(event.ActorType).ToNot(BeNil())

// ✅ Fix
eventData := unmarshalEventData(event.EventData)  // Helper function
Expect(eventData.NotificationID).To(Equal(expectedID))
Expect(eventData.Channel).To(Equal(expectedChannel))
Expect(*event.ActorType).To(Equal("service"))
Expect(*event.ActorId).To(Equal("notification-controller"))
```

#### **Phase 2: Status and Error Tests (P1 - Next Sprint)**
**Files**: `status_update_conflicts_test.go`, `error_propagation_test.go`
**Instances**: 10
**Timeline**: 2-3 days

#### **Phase 3: Routing and Config Tests (P2 - Refactoring)**
**Files**: `routing_*.go`, `sanitization/*.go`
**Instances**: 11
**Timeline**: 3-4 days

---

## 🚫 **Anti-Pattern #2: time.Sleep() (18 Instances)**

### Definition (from TESTING_GUIDELINES.md)

> **time.Sleep() is ABSOLUTELY FORBIDDEN** in tests for waiting on asynchronous operations
>
> ❌ **BAD**: `time.Sleep(2 * time.Second)`
> ✅ **GOOD**: `Eventually(func() bool { return condition }, 30*time.Second, 1*time.Second).Should(BeTrue())`

### Impact

- ❌ **Flaky Tests**: Fixed sleep durations cause intermittent failures
- ❌ **Slow Tests**: Always wait full duration even if condition met earlier
- ❌ **Race Conditions**: Sleep doesn't guarantee condition is met
- ❌ **CI Instability**: Different machine speeds cause test failures

---

### Violations by File

#### **Integration Tests**: 18 Instances

**File**: `test/integration/notification/suite_test.go`

| Line | Violation | Should Use | Rationale |
|------|-----------|------------|-----------|
| 195 | `time.Sleep(2 * time.Second)` // Wait for manager | `Eventually(func() bool { return managerIsReady() }, 30*time.Second).Should(BeTrue())` | Manager startup is async |
| 626 | `time.Sleep(100 * time.Millisecond)` // Wait for deletion | `Eventually(func() error { return secretDoesNotExist() }, 5*time.Second).Should(Succeed())` | Deletion is async |

**File**: `test/integration/notification/crd_lifecycle_test.go`

| Line | Violation | Impact |
|------|-----------|--------|
| 535 | `time.Sleep(100 * time.Millisecond)` in goroutine loop | Creates race condition with status updates |

**File**: `test/integration/notification/status_update_conflicts_test.go` - **3 INSTANCES**

| Line | Violation | Should Use |
|------|-----------|------------|
| 64 | `time.Sleep(100 * time.Millisecond)` // Allow environment to settle | ❌ REMOVE - unnecessary |
| 118 | `time.Sleep(2 * time.Second)` // Let controller update | `Eventually(func() string { return getResourceVersion() }).ShouldNot(Equal(originalVersion))` |
| 344 | `time.Sleep(500 * time.Millisecond)` // Wait for reconciliation | `Eventually(func() bool { return isReconciling() }).Should(BeTrue())` |

**File**: `test/integration/notification/resource_management_test.go` - **4 INSTANCES**

| Line | Violation | Purpose | Correct Pattern |
|------|-----------|---------|-----------------|
| 207 | `time.Sleep(2 * time.Second)` | Wait for goroutine cleanup | `Eventually(func() int { return runtime.NumGoroutine() }, 10*time.Second).Should(BeNumerically("<=", baselineGoroutines+5))` |
| 488 | `time.Sleep(3 * time.Second)` | Wait for resource cleanup | `Eventually(func() bool { return resourcesAreCleanedUp() }, 15*time.Second).Should(BeTrue())` |
| 514 | `time.Sleep(2 * time.Second)` | Ensure no notifications pending | `Consistently(func() int { return getPendingNotifications() }, 5*time.Second).Should(Equal(0))` |
| 615 | `time.Sleep(3 * time.Second)` | Wait for burst recovery | `Eventually(func() bool { return systemIsIdle() }, 15*time.Second).Should(BeTrue())` |

**File**: `test/integration/notification/performance_extreme_load_test.go` - **3 INSTANCES**

| Line | Violation | Pattern |
|------|-----------|---------|
| 163 | `time.Sleep(5 * time.Second)` | Goroutine cleanup check |
| 282 | `time.Sleep(5 * time.Second)` | Goroutine cleanup check |
| 391 | `time.Sleep(5 * time.Second)` | Goroutine cleanup check |

**Recommended Fix**:
```go
// ❌ BEFORE
time.Sleep(5 * time.Second)
runtime.GC()
currentGoroutines := runtime.NumGoroutine()
Expect(currentGoroutines).To(BeNumerically("<=", baselineGoroutines+toleranceBuffer))

// ✅ AFTER
Eventually(func() int {
    runtime.GC()
    return runtime.NumGoroutine()
}, 15*time.Second, 1*time.Second).Should(
    BeNumerically("<=", baselineGoroutines+toleranceBuffer),
    "Goroutines should be cleaned up after test completion")
```

**File**: `test/integration/notification/performance_edge_cases_test.go` - **1 INSTANCE**

| Line | Violation | Fix |
|------|-----------|-----|
| 487 | `time.Sleep(2 * time.Second)` | `Eventually(func() bool { return queueIsEmpty() }, 10*time.Second).Should(BeTrue())` |

**File**: `test/integration/notification/crd_rapid_lifecycle_test.go` - **4 INSTANCES**

| Line | Violation | Purpose |
|------|-----------|---------|
| 102 | `time.Sleep(50 * time.Millisecond)` | Allow reconciliation window |
| 175 | `time.Sleep(100 * time.Millisecond)` | Allow delivery attempt |
| 192 | `time.Sleep(50 * time.Millisecond)` | Ensure deletion processed |
| 324 | `time.Sleep(50 * time.Millisecond)` | Brief processing window |

**Note**: These might be **intentional staggering** (acceptable per TESTING_GUIDELINES.md if followed by `Eventually()`), but need review.

---

#### **Unit Tests**: Acceptable Usage

**File**: `test/unit/notification/slack_delivery_test.go`

| Line | Usage | Verdict |
|------|-------|---------|
| 202 | `time.Sleep(100 * time.Millisecond)` in mock server | ✅ **ACCEPTABLE** - Simulating server delay |
| 238 | `time.Sleep(2 * time.Second)` to trigger timeout | ✅ **ACCEPTABLE** - Testing timing behavior |
| 285 | `time.Sleep(1 * time.Second)` to trigger timeout | ✅ **ACCEPTABLE** - Testing timing behavior |

**File**: `test/unit/notification/retry_test.go`

| Line | Usage | Verdict |
|------|-------|---------|
| 234 | `time.Sleep(backoff)` in retry loop | ✅ **ACCEPTABLE** - Intentional backoff testing |

**File**: `test/unit/notification/file_delivery_test.go`

| Line | Usage | Verdict |
|------|-------|---------|
| 141 | `time.Sleep(50 * time.Millisecond)` for unique filenames | ✅ **ACCEPTABLE** - Intentional staggering |

---

### Remediation Strategy

#### **Immediate Fix Template**
```go
// ❌ ANTI-PATTERN
time.Sleep(2 * time.Second)
Expect(condition).To(BeTrue())

// ✅ FIX
Eventually(func() bool {
    return condition
}, 30*time.Second, 1*time.Second).Should(BeTrue(),
    "Condition should be met within timeout")
```

#### **Goroutine Cleanup Pattern**
```go
// ❌ ANTI-PATTERN
time.Sleep(5 * time.Second)
runtime.GC()
currentGoroutines := runtime.NumGoroutine()
Expect(currentGoroutines).To(BeNumerically("<=", baseline+5))

// ✅ FIX
Eventually(func() int {
    runtime.GC()
    return runtime.NumGoroutine()
}, 15*time.Second, 1*time.Second).Should(
    BeNumerically("<=", baseline+5),
    "Goroutines should be cleaned up")
```

---

## 🚫 **Anti-Pattern #3: Skip() (1 Instance)**

### Definition (from TESTING_GUIDELINES.md)

> **Skip() is ABSOLUTELY FORBIDDEN** in ALL test tiers, with NO EXCEPTIONS
>
> ❌ **BAD**: `Skip("Data Storage not available")`
> ✅ **GOOD**: `Fail("REQUIRED: Data Storage not available - start infrastructure first")`

### Impact

- ❌ **False Confidence**: Skipped tests show "green" but don't validate anything
- ❌ **Hidden Dependencies**: Missing infrastructure goes undetected in CI
- ❌ **Compliance Gaps**: Audit tests skipped = audit not validated

---

### Violation

**File**: `test/integration/notification/audit_integration_test.go`

```go:70:79:test/integration/notification/audit_integration_test.go
BeforeEach(func() {
    // ... setup code ...

    // Check Data Storage availability
    resp, err := httpClient.Get(dataStorageURL + "/health")
    if err != nil {
        Skip(fmt.Sprintf(  // ❌ FORBIDDEN
            "⏭️  Skipping: Data Storage not available at %s\n"+
                "  Per TESTING_GUIDELINES.md: Integration tests use real services when available\n"+
                "  Start with: podman-compose -f podman-compose.test.yml up -d",
            dataStorageURL))
    }
})
```

**Recommended Fix**:
```go
BeforeEach(func() {
    // ... setup code ...

    // Check Data Storage availability (REQUIRED per DD-AUDIT-003)
    resp, err := httpClient.Get(dataStorageURL + "/health")
    if err != nil || resp.StatusCode != http.StatusOK {
        Fail(fmt.Sprintf(
            "REQUIRED: Data Storage not available at %s\n"+
                "  Per DD-AUDIT-003: Audit infrastructure is MANDATORY\n"+
                "  Per TESTING_GUIDELINES.md: Integration tests MUST use real services\n\n"+
                "  Start with: podman-compose -f podman-compose.test.yml up -d\n"+
                "  Verify with: curl %s/health",
            dataStorageURL, dataStorageURL))
    }
})
```

**Rationale**:
- If Notification can run without Data Storage, audit is **effectively optional**
- This violates DD-AUDIT-003 (audit infrastructure MANDATORY)
- Skip() in CI means audit features are **not validated**
- **CI should fail** if infrastructure is missing

---

## 📊 **Compliance Status by Test Tier**

| Test Tier | Tests | NULL-TESTING | time.Sleep() | Skip() | Status |
|-----------|-------|--------------|--------------|--------|--------|
| **Unit** | 239 passing | 33 instances | 5 acceptable | 0 | 🟡 **PARTIAL** - Weak assertions |
| **Integration** | 105 passing | 19 instances | 18 violations | 1 violation | 🔴 **NON-COMPLIANT** |
| **E2E** | 12/14 passing | TBD | TBD | TBD | ⏳ **PENDING ANALYSIS** |

---

## 🎯 **Recommended Action Plan**

### Phase 1: Critical Fixes (P0 - This Sprint)
**Timeline**: 2-3 days
**Files**: Audit-related tests (31 NULL-TESTING + 1 Skip())

1. ✅ Fix `audit_integration_test.go:512` (user-identified violation)
2. ✅ Fix `audit_integration_test.go:75` (Skip() → Fail())
3. ✅ Fix all audit test NULL-TESTING violations (20 in unit, 11 in integration)

**Success Criteria**:
- Zero Skip() calls in integration tests
- All audit assertions validate actual field values
- All audit tests validate ADR-034 compliance

---

### Phase 2: Integration Test Stability (P1 - Next Sprint)
**Timeline**: 3-4 days
**Files**: All integration tests with time.Sleep() (18 instances)

1. ✅ Replace all time.Sleep() with Eventually()
2. ✅ Add proper timeout configurations (per TESTING_GUIDELINES.md)
3. ✅ Verify tests pass consistently in CI

**Success Criteria**:
- Zero time.Sleep() followed by assertions
- All async operations use Eventually()
- Integration test flakiness reduced to <1%

---

### Phase 3: Comprehensive Assertion Strengthening (P2 - Backlog)
**Timeline**: 1-2 weeks
**Files**: All remaining tests with NULL-TESTING (21 instances)

1. ✅ Fix routing/config tests (11 instances)
2. ✅ Fix status/error tests (10 instances)
3. ✅ Add business outcome validations

**Success Criteria**:
- All assertions validate business requirements
- Test descriptions match actual assertions
- Coverage metrics reflect real validation

---

## 📚 **References**

1. **[TESTING_GUIDELINES.md](../development/business-requirements/TESTING_GUIDELINES.md)** - Anti-pattern definitions and remediation
2. **[03-testing-strategy.mdc](../../.cursor/rules/03-testing-strategy.mdc)** - Defense-in-depth testing strategy
3. **[docs/services/crd-controllers/03-workflowexecution/testing-strategy.md](../services/crd-controllers/03-workflowexecution/testing-strategy.md)** - Reference implementation (WorkflowExecution)
4. **DD-AUDIT-003** - Audit infrastructure MANDATORY requirement
5. **ADR-034** - Unified Audit Table Design (field validation requirements)

---

## ❓ **Questions for User**

Before proceeding with remediation, clarify:

1. **Priority Confirmation**: Should we fix audit tests (Phase 1) immediately, or address all anti-patterns in parallel?

2. **time.Sleep() in Rapid Lifecycle Tests**: Lines 102, 175, 192, 324 in `crd_rapid_lifecycle_test.go` - are these intentional staggering (acceptable) or should they be replaced?

3. **E2E Tests**: Should we triage E2E tests for similar anti-patterns after Podman is stable?

4. **Enforcement**: Should we add linter rules (forbidigo) to prevent future anti-patterns?

5. **CI Integration**: Should we add pre-commit hooks to detect these patterns?

---

**Status**: ⏳ **AWAITING USER FEEDBACK**
**Next Steps**: User confirms priority and answers questions → Begin Phase 1 remediation

