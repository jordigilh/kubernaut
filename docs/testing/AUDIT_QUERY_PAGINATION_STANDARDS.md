# Audit Query Pagination Standards - AUTHORITATIVE

**Status**: ✅ MANDATORY
**Created**: 2026-01-24
**Authority**: Platform Team
**Applies To**: ALL integration tests querying Data Storage audit events

---

## 🚨 **CRITICAL: Use Proper Query Filters**

**RULE**: When querying audit events from Data Storage REST API, **ALWAYS** filter by `correlation_id` + `event_type` + `event_category`.

**Why**: Under concurrent test execution (12 parallel processes), hundreds of audit events can be created. Without proper filters, queries return ALL events from ALL tests, causing timeouts and flaky failures.

**Result**: Proper filtering → 1-2 events per query → No pagination needed ✅

---

## 📊 **The Problem: Pagination Bug**

### **Discovered**: 2026-01-24 (PR#20)
### **Impact**: 13 test files across 5 services (11 integration + 2 E2E)
### **Symptom**: Tests pass under low load, fail under high load (flaky tests)

### **Affected Files**
- **Notification** (3): `controller_audit_emission_test.go`, `controller_partial_failure_test.go`, `suite_test.go`
- **Remediation Orchestrator** (4): `audit_emission_integration_test.go`, `audit_phase_lifecycle_integration_test.go`, `remediationrequest_lifecycle_integration_test.go`, `suite_test.go`
- **AI Analysis** (2): `audit_emission_integration_test.go`, `suite_test.go`
- **Gateway** (2): `audit_integration_test.go`, `suite_test.go`
- **Data Storage E2E** (2): `01_happy_path_test.go`, `22_audit_validation_helper_test.go`

**Note**: Data Storage integration tests are NOT affected - they query the database directly via SQL, bypassing the REST API pagination issue.

### **Root Cause**

```go
// ❌ BUGGY PATTERN (DO NOT USE)
resp, err := dsClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
    EventType:     ogenclient.NewOptString(eventType),
    EventCategory: ogenclient.NewOptString("notification"),
    // ❌ MISSING: CorrelationID filter!
    // Result: Returns ALL events of this type from ALL tests!
})
return resp.Data  // ❌ Under load: 200+ events, test's event not found!
```

**Under High Load**:
- 12 parallel test processes create 200+ events in seconds
- Query returns only first 50-100 events (depending on limit)
- Test's events are at positions 101-212 → **NOT FOUND**
- Test times out → **flaky failure**

**Real Example** (PR#20 Run 3/3):
```
Data Storage logs:
04:13:02.670  POST /audit/events/batch: 121 events created
04:13:02.774  GET /audit/events: "total": 212, "count": 50 returned
              ^ Test looking for event at position 180 → TIMEOUT ❌
```

---

## ✅ **The Solution: Proper Query Filters**

### **Standard Pattern** (ALWAYS USE)

```go
// ✅ CORRECT: Filter by correlation_id + event_type + event_category
params := ogenclient.QueryAuditEventsParams{
    CorrelationID: ogenclient.NewOptString(correlationID),  // ← Isolate test data
    EventType:     ogenclient.NewOptString(eventType),      // ← Specific event
    EventCategory: ogenclient.NewOptString(eventCategory),  // ← Specific service
}

resp, err := dsClient.QueryAuditEvents(ctx, params)
Expect(err).ToNot(HaveOccurred())

events := resp.Data  // ✅ Returns 1-2 events, no pagination needed!
```

**Why This Works**:
- `event_type` only: Returns 200+ events (all tests, all services) ❌
- `event_type + event_category`: Returns 50+ events (all tests, one service) ❌
- `correlation_id + event_type + event_category`: Returns 1-2 events ✅

**Result**: Proper filtering → No pagination needed → Faster, simpler tests

### **When Pagination IS Needed**

Only use pagination loops for these specific cases:

1. **Testing pagination itself** (e.g., DS E2E pagination API tests)
2. **Queries without correlation_id** (rare - reconstruction, forensics)
3. **Intentionally broad queries** (e.g., "all events for analysis")

For 99% of integration/E2E tests: **Just add proper filters, skip pagination**

```go
// queryAllAuditEvents fetches ALL audit events across ALL pages.
// MANDATORY for concurrent test environments where 100+ events may exist.
//
// Usage:
//   params := ogenclient.QueryAuditEventsParams{
//       EventType:     ogenclient.NewOptString("notification.message.sent"),
//       CorrelationID: ogenclient.NewOptString("my-correlation-id"),
//   }
//   events, err := queryAllAuditEvents(params)
//
func queryAllAuditEvents(params ogenclient.QueryAuditEventsParams) ([]ogenclient.AuditEvent, error) {
	var allEvents []ogenclient.AuditEvent
	offset := 0
	limit := 100 // Reasonable page size (balance between API calls and memory)

	for {
		// Set pagination params for this page
		params.Limit = ogenclient.NewOptInt(limit)
		params.Offset = ogenclient.NewOptInt(offset)

		resp, err := dsClient.QueryAuditEvents(context.Background(), params)
		if err != nil {
			return nil, fmt.Errorf("failed to query audit events (offset=%d): %w", offset, err)
		}

		if resp.Data == nil || len(resp.Data) == 0 {
			break // No more results
		}

		// Accumulate events from this page
		allEvents = append(allEvents, resp.Data...)

		// If we got fewer results than the limit, we've reached the end
		if len(resp.Data) < limit {
			break
		}

		// Move to next page
		offset += limit
	}

	return allEvents, nil
}
```

---

## 📋 **Implementation Checklist**

When writing integration tests that query audit events:

### **1. Use Pagination Helper**
- ✅ Copy `queryAllAuditEvents` helper to your test suite
- ✅ Call it instead of direct `dsClient.QueryAuditEvents`
- ✅ Use `Eventually()` with the helper for timing tolerance

### **2. Set Appropriate Timeouts**
- ✅ Use at least 10-20 seconds for `Eventually()` timeout
- ✅ Account for audit buffer flush time (up to 1 second per ADR-032)
- ✅ Add explicit flush before querying if immediate results needed

### **3. Test Under Load**
- ✅ Run tests with `-p 12` (12 parallel processes) locally
- ✅ Verify tests pass 10/10 times under concurrent load
- ✅ Check Data Storage logs for total event count

---

## 🔍 **Detection: How to Spot This Bug**

### **Code Review Red Flags**

**❌ Missing Pagination**:
```go
// RED FLAG: No loop, only gets first page
resp, err := dsClient.QueryAuditEvents(ctx, params)
return resp.Data
```

**❌ Fixed Limit Without Loop**:
```go
// RED FLAG: Limit=100 but no pagination loop
params.Limit = ogenclient.NewOptInt(100)
resp, err := dsClient.QueryAuditEvents(ctx, params)
return resp.Data
```

**❌ No Limit Specified**:
```go
// RED FLAG: Defaults to 50, no pagination
resp, err := dsClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
    EventType: ogenclient.NewOptString(eventType),
})
return resp.Data
```

**✅ Correct Pattern**:
```go
// GREEN: Pagination loop with offset advancement
for {
    params.Limit = ogenclient.NewOptInt(limit)
    params.Offset = ogenclient.NewOptInt(offset)
    resp, err := dsClient.QueryAuditEvents(ctx, params)
    // ... accumulate results ...
    if len(resp.Data) < limit { break }
    offset += limit
}
```

### **Test Behavior Red Flags**

- ⚠️ **Flaky under CI (12 procs), passes locally (1 proc)**
- ⚠️ **Fails with "Eventually() timeout" after 10-20 seconds**
- ⚠️ **Passes in Run 1/2, fails in Run 3 (depends on execution order)**
- ⚠️ **Data Storage logs show `"total": 200+` but test finds 0 events**

---

## 🛠️ **Migration Guide**

### **Step 1: Add Pagination Helper to Suite**

In your `suite_test.go` or test file:

```go
// queryAllAuditEvents fetches ALL audit events across ALL pages.
// Required for concurrent test execution (prevents missing events beyond first page).
func queryAllAuditEvents(params ogenclient.QueryAuditEventsParams) ([]ogenclient.AuditEvent, error) {
	var allEvents []ogenclient.AuditEvent
	offset := 0
	limit := 100

	for {
		params.Limit = ogenclient.NewOptInt(limit)
		params.Offset = ogenclient.NewOptInt(offset)

		resp, err := dsClient.QueryAuditEvents(context.Background(), params)
		if err != nil {
			return nil, err
		}

		if resp.Data == nil || len(resp.Data) == 0 {
			break
		}

		allEvents = append(allEvents, resp.Data...)

		if len(resp.Data) < limit {
			break
		}

		offset += limit
	}

	return allEvents, nil
}
```

### **Step 2: Replace Direct Query Calls**

**Before**:
```go
resp, err := dsClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
    EventType:     ogenclient.NewOptString("notification.message.sent"),
    CorrelationID: ogenclient.NewOptString(correlationID),
})
Expect(err).ToNot(HaveOccurred())
events := resp.Data
```

**After**:
```go
events, err := queryAllAuditEvents(ogenclient.QueryAuditEventsParams{
    EventType:     ogenclient.NewOptString("notification.message.sent"),
    CorrelationID: ogenclient.NewOptString(correlationID),
})
Expect(err).ToNot(HaveOccurred())
```

### **Step 3: Update Counting/Filtering Helpers**

**Before**:
```go
func countAuditEvents(eventType, correlationID string) int {
    resp, err := dsClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
        EventType:     ogenclient.NewOptString(eventType),
        CorrelationID: ogenclient.NewOptString(correlationID),
    })
    if err != nil { return 0 }
    return len(resp.Data)  // ❌ Wrong count if >50 events
}
```

**After**:
```go
func countAuditEvents(eventType, correlationID string) int {
    events, err := queryAllAuditEvents(ogenclient.QueryAuditEventsParams{
        EventType:     ogenclient.NewOptString(eventType),
        CorrelationID: ogenclient.NewOptString(correlationID),
    })
    if err != nil { return 0 }
    return len(events)  // ✅ Correct count across all pages
}
```

---

## 📚 **Performance Considerations**

### **Page Size (Limit)**

**Recommended**: `limit = 100`

**Tradeoff**:
- **Smaller (50)**: More API calls, less memory per call
- **Larger (500)**: Fewer API calls, more memory per call
- **100**: Balanced for typical test scenarios

### **Memory Usage**

**Typical Test**:
- 1 correlation ID queries = 2-10 events = ~50 KB
- **Safe** for pagination

**High-Load Test**:
- All events query = 200+ events = ~1-2 MB
- Still **safe** for integration tests
- Consider filtering by `correlation_id` to reduce result set

### **API Call Count**

**Example** (200 events total):
- Without pagination: 1 call (returns 50, misses 150) ❌
- With pagination (limit=100): 2 calls (returns all 200) ✅

**Impact**: Negligible (2-3 calls vs 1 call, each <100ms)

---

## 🧪 **Validation Commands**

### **Run Tests Under High Load**

```bash
# Run with 12 parallel processes (matches CI)
make test-integration-notification GOMAXPROCS=12 GINKGO_PROCS=12

# Run multiple times to check for flakiness
for i in {1..10}; do
  echo "=== Run $i/10 ==="
  make test-integration-notification || break
done
```

### **Check Data Storage Event Count**

```bash
# In test logs, look for:
grep "total.*events" /tmp/kubernaut-must-gather/*/notification_notification_datastorage_test.log

# Should see lines like:
# "total": 212, "count": 50  <- If you see this, pagination is needed!
```

### **Verify Pagination Logic**

```bash
# Grep for pagination patterns
grep -n "for.*QueryAuditEvents\|offset.*limit.*QueryAuditEvents" \
  test/integration/*/audit*test.go

# Should find pagination loops in all query helpers
```

---

## 🎯 **Success Criteria**

Tests are correctly handling pagination when:

- ✅ Tests pass 10/10 times under high load (`-p 12`)
- ✅ Code review shows pagination loop in ALL query helpers
- ✅ No direct `dsClient.QueryAuditEvents()` calls without pagination
- ✅ Data Storage logs show `total > limit` but tests still find events

---

## 📖 **Related Documentation**

- **Root Cause Analysis**: `docs/triage/PR20_NT_AUDIT_EMISSION_FLAKY_TESTS_JAN_24_2026.md`
- **All Services Impact**: `docs/triage/PR20_AUDIT_QUERY_PAGINATION_BUG_ALL_SERVICES_JAN_24_2026.md`
- **Data Storage API**: `api/openapi/data-storage-v1.yaml`
- **Testing Strategy**: `.cursor/rules/03-testing-strategy.mdc`
- **ADR-032**: Audit buffering and flush timing

---

## 🚫 **AI Assistant Enforcement**

### **MANDATORY Pre-Code Generation Check**

Before generating audit query code, AI MUST:

```xml
<function_calls>
<invoke name="grep">
<parameter name="pattern">dsClient.QueryAuditEvents</parameter>
<parameter name="path">[target_file]</parameter>
<parameter name="output_mode">content</parameter>
<parameter name="-A">10</parameter>
</invoke>