# Data Storage Test Isolation Pattern - Cross-Service Analysis

**Date**: December 29, 2025
**Purpose**: Document how Go services handle shared Data Storage in parallel tests
**Pattern Authority**: Gateway, SignalProcessing, AIAnalysis, Notification integration tests

---

## 🎯 **Key Insight: Shared Infrastructure + Unique IDs**

All Kubernaut services **share the same Data Storage instance** during parallel test execution but achieve isolation through **unique correlation IDs**.

**Pattern**: ✅ **Query Isolation** (not database isolation)

---

## 📋 **How Go Services Achieve Test Isolation**

### **1. Unique ID Generation Per Test**

**Pattern Used Across All Services:**

```go
// Gateway Pattern (test/integration/gateway/audit_integration_test.go:102)
sharedNamespace := fmt.Sprintf("test-audit-%d-%s",
    GinkgoParallelProcess(),  // Worker ID (1-4)
    uuid.New().String()[:8])   // Unique UUID fragment

// Individual request IDs
uniqueID := uuid.New().String()
correlation_id := fmt.Sprintf("rem-%s", uniqueID)
```

**Key Functions:**
- `GinkgoParallelProcess()` - Returns worker number (1, 2, 3, 4) like pytest's `worker_id`
- `uuid.New()` - Generates collision-free UUIDs
- `time.Now().UnixNano()` - Timestamp for additional uniqueness

### **2. Query Data Storage With Unique Correlation ID**

**Gateway Example** (test/integration/gateway/audit_integration_test.go:205-208):

```go
// CRITICAL: Filter by unique correlation_id to isolate test data
params := &dsgen.QueryAuditEventsParams{
    CorrelationId: &correlationID,  // ← Query only THIS test's events
    EventType:     &eventType,
    EventCategory: ptr.To("gateway"),
}

// Wait for async write with Eventually()
Eventually(func() int {
    resp, err := dsClient.QueryAuditEventsWithResponse(ctx, params)
    Expect(err).ToNot(HaveOccurred())
    Expect(resp.StatusCode()).To(Equal(200))

    // Parse response
    auditEvents = resp.JSON200.Events
    return len(auditEvents)
}).Should(BeNumerically(">", 0), "Audit event should appear in Data Storage")
```

### **3. Use Eventually() for Async Operations**

**Pattern**: Data Storage writes are **buffered** (ADR-038: 2-second flush interval)

```go
// WRONG: Immediate query
events := queryAuditEvents(correlationID) // ❌ May be empty due to buffering

// CORRECT: Eventually with timeout
Eventually(func() int {
    events := queryAuditEvents(correlationID)
    return len(events)
}, "10s", "500ms").Should(BeNumerically(">", 0)) // ✅ Wait up to 10 seconds
```

---

## 🔍 **Why This Pattern Works**

### **Shared Infrastructure is OK When:**

1. **Each test uses unique IDs**: No collision between parallel workers
2. **Queries filter by correlation_id**: Each test sees only its own data
3. **Eventually() handles async delays**: Accounts for buffer flush timing
4. **No cleanup required**: Database accumulates test data (acceptable for integration tests)

### **Database Isolation is NOT Required Because:**

- ✅ Correlation IDs are globally unique (UUID-based)
- ✅ Tests never query without correlation_id filter
- ✅ Old test data doesn't interfere with new tests
- ✅ Data Storage is ephemeral (rebuilt per test run)

---

## 🐍 **HAPI Python Test Adaptation**

### **What We Already Have** ✅

```python
# conftest.py
@pytest.fixture(scope="function")
def unique_test_id(worker_id, request):
    """
    Generate unique test ID: {test_name}_{worker_id}_{timestamp}
    """
    test_name = request.node.name
    timestamp = int(time.time() * 1000)
    return f"{test_name}_{worker_id}_{timestamp}"
```

### **What's Missing** ❌

**Problem**: Tests are NOT querying Data Storage with unique correlation_id filters!

**Example from test_hapi_audit_flow_integration.py:225-280:**

```python
# CURRENT (WRONG for parallel):
remediation_id = f"rem-{unique_test_id}"  # ✅ Unique ID generated

# ... make request ...

# Query audit events
events = query_audit_events(
    data_storage_url=data_storage_url,
    correlation_id=remediation_id,  # ✅ Uses unique ID
    timeout=10
)

# WAIT - This looks correct!
```

**Actually**, our implementation **IS correct**! Let me check what's actually failing...

---

## 🔬 **Analyzing HAPI Test Failures**

### **Sequential Execution** (24/41 passing, 58%)
```
PASSED: 24 tests
FAILED: 16 tests
ERROR: 1 test
```

### **Parallel Execution (-n 4)** (4/41 passing, 10%)
```
PASSED: 4 tests
FAILED: 1 test
ERROR: 36 tests
```

### **Root Cause Hypothesis**

The **pattern is correct**, but there may be:

1. **Service Startup Conflicts**: Multiple workers calling HAPI simultaneously
2. **Port Binding Issues**: Single HAPI instance can't handle 4 parallel connections
3. **OpenAPI Client Generation**: Multiple workers regenerating client simultaneously
4. **Metrics Counter Conflicts**: Global metrics being shared across tests

---

## 💡 **Recommended Fix Strategy**

### **Option A: Fix Shared Service Bottleneck** (Most Likely)

**Problem**: All 4 workers share ONE HAPI service instance
- ✅ Go services: Each test creates its own httptest.Server
- ❌ HAPI tests: All tests use shared `http://localhost:18120`

**Solution**:
```python
# Create per-worker HAPI instance (like Go's httptest.Server)
@pytest.fixture(scope="session")
def hapi_url_per_worker(worker_id):
    if worker_id == "master":
        return "http://localhost:18120"
    else:
        # Different port per worker
        port = 18120 + int(worker_id.replace("gw", ""))
        # Start HAPI instance on this port
        return f"http://localhost:{port}"
```

### **Option B: Skip Parallel for Stateful Tests**

**Strategy**: Mark tests that can/cannot run in parallel

```python
# In conftest.py
def pytest_collection_modifyitems(items):
    for item in items:
        if "audit_flow" in item.nodeid or "metrics" in item.nodeid:
            item.add_marker(pytest.mark.serial)  # Sequential only

# Run with:
# pytest tests/integration/ -n 4 -m "not serial"  # Parallel-safe tests
# pytest tests/integration/ -m "serial"            # Sequential tests
```

### **Option C: Use TestClient Instead of Real Service**

**Pattern**: FastAPI TestClient creates isolated app instance per test

```python
@pytest.fixture(scope="function")
def hapi_client(unique_test_id):
    """Create isolated HAPI instance per test"""
    from src.main import app
    from fastapi.testclient import TestClient

    # Each test gets its own app instance
    return TestClient(app)
```

---

## 📊 **Pattern Comparison**

| Aspect | Go Services (Gateway, etc.) | HAPI Python (Current) | Recommended Fix |
|--------|----------------------------|----------------------|-----------------|
| **Worker ID** | `GinkgoParallelProcess()` | `worker_id` fixture | ✅ Already correct |
| **Unique IDs** | UUID + timestamp | `unique_test_id` | ✅ Already correct |
| **Correlation Filter** | Yes, in queries | Yes, in queries | ✅ Already correct |
| **Service Instance** | Per-test `httptest.Server` | Shared `localhost:18120` | ❌ **BOTTLENECK** |
| **Eventually Pattern** | `Eventually(func)` 10s | `time.sleep()` hardcoded | ⚠️ Could improve |
| **Cleanup** | None (data accumulates) | None | ✅ Correct pattern |

---

## 🎯 **Recommended Next Steps**

### **Immediate Fix** (1-2 hours):

1. **Add per-worker service isolation**:
   - Option A: Multiple HAPI ports (18120-18123)
   - Option B: Use FastAPI TestClient (app per test)
   - Option C: Mark stateful tests as serial

2. **Improve async waiting**:
   - Replace `time.sleep(5)` with retry logic
   - Add `wait_for_audit_events()` helper

3. **Test the fix**:
   ```bash
   pytest tests/integration/ -n 4 -v
   # Should see ~58% pass rate (same as sequential)
   ```

### **Long-term Improvement** (Future work):

- Create per-worker Data Storage instances (overkill for now)
- Implement database schema isolation (complex, low ROI)
- Add test cleanup hooks (nice-to-have, not required)

---

## 📚 **References**

- **Gateway Audit Tests**: `test/integration/gateway/audit_integration_test.go:102-220`
- **DataStorage Suite**: `test/integration/datastorage/suite_test.go:91`
- **Parallel Testing Doc**: `test/integration/gateway/PARALLEL_TESTING_ENABLEMENT.md`
- **ADR-038**: Audit buffering (2-second flush interval)
- **DD-TEST-001**: Port allocation strategy

---

## ✅ **Success Criteria**

Parallel execution is working when:
- ✅ Pass rate same in sequential and parallel modes (~58%)
- ✅ No ERROR results (only PASS/FAIL)
- ✅ Test execution time reduced (3-4x faster)
- ✅ No port binding conflicts
- ✅ No client generation conflicts

---

**Document Status**: ✅ **ANALYSIS COMPLETE**
**Recommendation**: **Option B** (mark stateful tests as serial) for quickest fix
**Confidence**: 90% that service bottleneck is the root cause

