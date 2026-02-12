# DataStorage & HolmesGPT API Audit Tests DD-TESTING-001 Compliance Triage

**Date**: January 3, 2026
**Status**: ⚠️ **NON-COMPLIANT** - 12 Violations Found
**Services**: DataStorage (DS), HolmesGPT API (HAPI)
**Authority**: DD-TESTING-001: Audit Event Validation Standards

---

## 🎯 **Executive Summary**

Completed triage of the 2 remaining services (DataStorage and HolmesGPT API) that were initially overlooked.

**Compliance Score**:
- **DataStorage**: **0% (3 violations)** ❌
- **HolmesGPT API**: **0% (9 violations)** ❌

**Total Violations**: 12 (3 in Go, 9 in Python)

---

## 📊 **Service 7: DataStorage (DS) - Workflow Catalog Audit**

### **Status**: ⚠️ **0% DD-TESTING-001 Compliant** (3 violations)

**Test File**: `test/e2e/datastorage/06_workflow_search_audit_test.go`

**Audit Events Emitted**:
- `workflow.catalog.search_completed` - Workflow catalog search operations

### **Violations Found** (3)

| Line | Type | Violation | Severity |
|------|------|-----------|----------|
| 351 | Non-Deterministic Count | `BeNumerically(">=", 1)` | 🔴 HIGH |
| 366 | Non-Deterministic Count | `BeNumerically(">=", 0.5)` | 🔴 HIGH |
| 374 | Non-Deterministic Count | `BeNumerically(">=", 0)` | 🔴 HIGH |

---

### **Violation 1** (Line 351)

**Current Code**:
```go
Expect(resultsData["total_found"]).To(BeNumerically(">=", 1),
    "Should find at least one workflow")
```

**Problem**: Would pass with 1, 2, 10, or 100 workflows found. Hides issues with search quality.

**Required Fix**:
```go
Expect(resultsData["total_found"]).To(Equal(expectedCount),
    "Workflow catalog search should return exactly N results for this test case")
```

**Rationale**: Deterministic validation ensures search returns expected number of workflows.

---

### **Violation 2** (Line 366)

**Current Code**:
```go
Expect(scoring["confidence"]).To(BeNumerically(">=", 0.5),
    "Confidence score should be reasonable")
```

**Problem**: "Reasonable" is too vague. Test would pass with 0.5, 0.6, 0.9, or 1.0.

**Required Fix**:
```go
Expect(scoring["confidence"]).To(BeNumerically(">=", 0.7),
    "Workflow catalog should score relevant workflows with high confidence (≥0.7)")

// OR for more precision:
Expect(scoring["confidence"]).To(And(
    BeNumerically(">=", 0.7),
    BeNumerically("<=", 1.0),
), "Confidence should be between 0.7 and 1.0 for this test case")
```

**Rationale**: More precise confidence thresholds validate search quality.

---

### **Violation 3** (Line 374)

**Current Code**:
```go
Expect(searchMetadata["duration_ms"]).To(BeNumerically(">=", 0),
    "Duration should be non-negative")
```

**Problem**: Only validates non-negativity. Doesn't catch performance regressions.

**Required Fix**:
```go
Expect(searchMetadata["duration_ms"]).To(And(
    BeNumerically(">", 0),
    BeNumerically("<", 1000),
), "Workflow catalog search should complete within 1 second")
```

**Rationale**: Performance bounds catch regressions.

---

### **Assessment**

**Priority**: 🟡 **MEDIUM**
- E2E test, not core integration test
- 3 violations are all non-deterministic counts
- No time.Sleep() violations found
- Uses `Eventually()` correctly (line 245 comment)

**Estimated Fix Time**: ~10 minutes

---

## 📊 **Service 8: HolmesGPT API (HAPI) - LLM Interaction Audit**

### **Status**: ⚠️ **0% DD-TESTING-001 Compliant** (9 violations)

**Test Files**:
- `holmesgpt-api/tests/integration/test_hapi_audit_flow_integration.py`
- `holmesgpt-api/tests/e2e/test_audit_pipeline_e2e.py`

**Audit Events Emitted**:
- `aiagent.llm.request` - LLM API call initiated
- `aiagent.llm.response` - LLM API response received
- `aiagent.workflow.validation_attempt` - Workflow validation performed

### **Violations Found** (9)

#### **Integration Test Violations** (6)

| Line | Type | Violation | Severity |
|------|------|-----------|----------|
| 154 | time.sleep() | `time.sleep(poll_interval)` | 🟡 MEDIUM |
| 274 | Non-Deterministic Count | `assert len(events) >= 2` | 🔴 HIGH |
| 433 | Non-Deterministic Count | `assert len(events) >= 2` | 🔴 HIGH |
| 589 | Non-Deterministic Count | `assert len(events) >= 2` | 🔴 HIGH |
| 261 | Weak Assertion | `assert response is not None` | 🟢 LOW |
| 513 | Weak Assertion | `assert field_value is not None` | 🟢 LOW |

#### **E2E Test Violations** (3)

| Line | Type | Violation | Severity |
|------|------|-----------|----------|
| 244 | time.sleep() | `time.sleep(poll_interval)` | 🟡 MEDIUM |
| 265 | time.sleep() | `time.sleep(seconds)` | 🟡 MEDIUM |
| 377 | Non-Deterministic Count | `assert len(llm_requests) >= 1` | 🔴 HIGH |
| 431 | Non-Deterministic Count | `assert len(llm_responses) >= 1` | 🔴 HIGH |
| 487 | Non-Deterministic Count | `assert len(validation_events) >= 1` | 🔴 HIGH |

---

### **Python-Specific Patterns**

#### **Violation Pattern 1: Non-Deterministic Count (Python)**

**Current Code** (Line 274):
```python
assert len(events) >= 2, f"Expected at least 2 audit events (llm_request, llm_response), got {len(events)}"
```

**Problem**: Would pass with 2, 3, 4, ... N events. Duplicate events would go undetected.

**Required Fix**:
```python
assert len(events) == 2, f"Expected exactly 2 audit events (llm_request, llm_response), got {len(events)}"
```

**Rationale**: One LLM call = exactly 2 events (request + response).

---

#### **Violation Pattern 2: time.sleep() for Polling (Python)**

**Current Code** (Line 154):
```python
while retries < max_retries:
    events = query_audit_events(...)
    if len(events) >= expected_count:
        return events
    time.sleep(poll_interval)  # ❌ VIOLATION
    retries += 1
```

**Problem**: Fixed polling interval, not condition-based.

**Required Fix (Python equivalent of Eventually)**:
```python
def poll_for_audit_events(query_func, expected_count, timeout=30, poll_interval=0.5):
    """
    Poll for audit events with timeout (Python equivalent of Eventually).

    DD-TESTING-001: Use polling with timeout instead of fixed time.sleep()
    """
    start_time = time.time()
    while time.time() - start_time < timeout:
        events = query_func()
        if len(events) == expected_count:  # ✅ Deterministic
            return events
        time.sleep(poll_interval)

    raise AssertionError(f"Expected {expected_count} events, got {len(events)} after {timeout}s")

# Usage:
events = poll_for_audit_events(
    lambda: query_audit_events(...),
    expected_count=2,
    timeout=30
)
```

**Rationale**: Condition-based polling, deterministic count validation.

---

#### **Violation Pattern 3: Non-Deterministic Event Type Counts**

**Current Code** (Line 377):
```python
llm_requests = [e for e in events if e.event_type == "llm_request"]
assert len(llm_requests) >= 1, f"llm_request event not found. Found events: {[e.event_type for e in events]}"
```

**Problem**: Would pass with 1, 2, 3, ... N llm_request events.

**Required Fix**:
```python
llm_requests = [e for e in events if e.event_type == "llm_request"]
assert len(llm_requests) == 1, \
    f"Expected exactly 1 llm_request event, got {len(llm_requests)}. Found events: {[e.event_type for e in events]}"
```

**Rationale**: One LLM call = one request event.

---

### **Assessment**

**Priority**: 🔴 **HIGH**
- 9 violations across integration and E2E tests
- Mix of time.sleep() and non-deterministic counts
- Python service (different patterns than Go)
- Critical for LLM interaction auditing

**Estimated Fix Time**: ~30-40 minutes
- ~15 min: Fix non-deterministic counts (6 locations)
- ~15 min: Replace time.sleep() with polling helper (3 locations)
- ~10 min: Test and verify

---

## 📋 **Combined Violations Summary**

### **By Service**

| Service | P1 (Counts) | P2 (Sleep) | P3 (Weak) | Total |
|---------|-------------|------------|-----------|-------|
| **DataStorage** | 3 | 0 | 0 | 3 |
| **HolmesGPT API** | 6 | 3 | 0 | 9 |
| **Total** | **9** | **3** | **0** | **12** |

### **By Test Type**

| Test Type | DataStorage | HolmesGPT API | Total |
|-----------|-------------|---------------|-------|
| **Integration** | 0 | 6 | 6 |
| **E2E** | 3 | 3 | 6 |
| **Total** | **3** | **9** | **12** |

---

## 🔧 **Implementation Plan**

### **DataStorage Fixes** (~10 min)

**File**: `test/e2e/datastorage/06_workflow_search_audit_test.go`

1. Line 351: `BeNumerically(">=", 1)` → `Equal(expectedCount)`
2. Line 366: `BeNumerically(">=", 0.5)` → `BeNumerically(">=", 0.7)` with upper bound
3. Line 374: `BeNumerically(">=", 0)` → Add upper bound for performance validation

### **HolmesGPT API Fixes** (~40 min)

**Integration Test** (`test_hapi_audit_flow_integration.py`):
1. Line 154: Replace `time.sleep()` with polling helper
2. Lines 274, 433, 589: `len(events) >= 2` → `len(events) == 2`

**E2E Test** (`test_audit_pipeline_e2e.py`):
1. Lines 244, 265: Replace `time.sleep()` with polling helper
2. Lines 377, 431, 487: `len(...) >= 1` → `len(...) == 1`

**Create Helper**:
```python
# holmesgpt-api/tests/helpers/audit_polling.py
def poll_for_audit_events(query_func, expected_count, timeout=30, poll_interval=0.5):
    """DD-TESTING-001 compliant audit event polling."""
    # Implementation above
```

---

## 🎯 **Success Criteria**

### **DataStorage**

- [ ] 3 non-deterministic counts fixed
- [ ] Performance bounds added
- [ ] E2E test passes

### **HolmesGPT API**

- [ ] 6 non-deterministic counts fixed to exact values
- [ ] 3 time.sleep() replaced with polling helper
- [ ] Polling helper created and tested
- [ ] Integration and E2E tests pass

---

## 📊 **Updated Overall Compliance**

### **With DS & HAPI Included**

**Total Services**: 8
**Total Violations**: 46 (34 previous + 12 new)
**Fixed**: 22 (48%)
**Remaining**: 24 (52%)

| Service | Compliance | Violations | Status |
|---------|------------|------------|--------|
| **AIAnalysis** | ✅ 100% | 0 (12 fixed) | ✅ COMPLIANT |
| **SignalProcessing** | ✅ 91% | 1 (10 fixed) | ✅ MOSTLY COMPLIANT |
| **RO (Integration)** | ✅ 100% | 0 | ✅ COMPLIANT |
| **RO (E2E)** | ⚠️ 0% | 3 | ⏭️ E2E |
| **Workflow Execution** | ⚠️ 0% | 3 | ⏭️ PENDING |
| **Gateway** | ⚠️ 0% | 4 | ⏭️ PENDING |
| **Notification** | ⚠️ 0% | 1 | ⏭️ PENDING |
| **DataStorage** | ⚠️ 0% | 3 | ⏭️ E2E |
| **HolmesGPT API** | ⚠️ 0% | 9 | ⏭️ PENDING |
| **Total** | - | **34** | **48% Fixed** |

---

## 🚀 **Recommended Fix Priority**

### **Phase 3A: Quick Wins** (~50 min)

1. **Notification** (1 violation, ~5 min)
2. **DataStorage** (3 violations, ~10 min)
3. **Workflow Execution** (3 violations, ~15 min)
4. **Gateway** (4 violations, ~20 min)

**Total**: 11 violations, ~50 minutes

### **Phase 3B: Complex** (~40 min)

5. **HolmesGPT API** (9 violations, ~40 min)
   - Requires Python polling helper
   - Different patterns than Go

### **Phase 4: E2E** (~15 min)

6. **RO E2E** (3 violations, ~15 min)

**Expected Final State**: 8/8 services at ≥90% compliance

---

## 📚 **References**

- **Authority**: `docs/architecture/decisions/DD-TESTING-001-audit-event-validation-standards.md`
- **DS Test**: `test/e2e/datastorage/06_workflow_search_audit_test.go`
- **HAPI Integration**: `holmesgpt-api/tests/integration/test_hapi_audit_flow_integration.py`
- **HAPI E2E**: `holmesgpt-api/tests/e2e/test_audit_pipeline_e2e.py`

---

## 🎯 **Conclusion**

Identified 12 additional violations in the 2 initially-overlooked services:
- **DataStorage**: 3 violations (workflow catalog search audit)
- **HolmesGPT API**: 9 violations (LLM interaction audit)

**Total Scope**: Now 8 services, 46 total violations (22 fixed, 24 remaining)

**Revised Estimate**: ~90 minutes to achieve 8/8 services at ≥90% compliance

---

**Document Status**: ✅ Complete - Missing Services Found
**Created**: 2026-01-03
**Priority**: ⚠️ HIGH (Complete scope coverage)
**Business Impact**: Ensures all audit-emitting services are DD-TESTING-001 compliant



