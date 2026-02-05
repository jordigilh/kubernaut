# Notification Audit Mystery - BREAKTHROUGH Investigation (Jan 31, 2026)

## Executive Summary

**Status**: 🎯 **ROOT CAUSE NARROWED DOWN** - Bug in DataStorage query execution logic  
**Investigation**: Used preserved cluster + PostgreSQL direct access  
**Finding**: Events exist in database, SQL works, but DataStorage returns empty results  
**Impact**: NT-specific issue - Gateway, RO, WE don't exhibit this behavior  
**Next Step**: Debug DataStorage query builder or add SQL logging

---

## Test Results (With KEEP_CLUSTER=true)

```
Results: 24/30 (80%) - Better than previous 19/30!
- 24 Passed
- 6 Failed (audit-related)
- 2 Flaked
Duration: ~10 minutes
Cluster: Preserved successfully
```

---

## The Investigation Journey

### Step 1: Verified Events in Database ✅

```sql
-- Total events
SELECT COUNT(*) FROM audit_events;
→ 107 events

-- By category
SELECT event_category, COUNT(*) FROM audit_events GROUP BY event_category;
→ notification: 107

-- Sample events
SELECT event_id, event_type, correlation_id, actor_id, event_date FROM audit_events 
WHERE event_category = 'notification' LIMIT 5;
→ Returns 5 events with proper data

-- With test filters (same as tests use)
SELECT COUNT(*) FROM audit_events 
WHERE event_type = 'notification.message.sent' 
  AND event_category = 'notification'
  AND correlation_id = '27b7ab77-6ed0-4fe6-99ff-742a1c8d3ae2';
→ Returns 2 events ✅
```

**Findings**:
- ✅ Events exist in database
- ✅ All mandatory fields populated (actor_id, correlation_id, event_type, etc.)
- ✅ SQL query with filters WORKS
- ✅ Table is partitioned correctly (audit_events_2026_01)

---

### Step 2: Verified DataStorage Receives Queries ✅

**DataStorage Logs Analysis**:
```
POST /api/v1/audit/events/batch: 17 requests (events written)
GET /api/v1/audit/events: 126 requests (queries)
```

**All GET requests**:
- Status: 200 OK
- Response size: 76 bytes
- Response body: `{"data":[],"pagination":{"total":0,"limit":50,"offset":0}}`

**Log Pattern**:
```
2026-01-31T01:36:03.930Z  INFO  HTTP request  
  {"method": "GET", "path": "/api/v1/audit/events", "status": 200, "bytes": 76}

2026-01-31T01:35:55.822Z  INFO  Audit events queried successfully  
  {"count": 0, "total": 0, "limit": 50, "offset": 0}
```

**Findings**:
- ✅ DataStorage receives queries
- ✅ No auth errors (all 200 OK)
- ✅ Query handler executes
- ❌ **ALL queries return count:0, total:0**

---

### Step 3: Verified Database Configuration ✅

**PostgreSQL Setup**:
- Database: `action_history`
- User: `slm_user` (exists, has superuser permissions)
- Connection: Working (events being written)

**Partitioning**:
```
audit_events (parent table, type: 'p')
├─ audit_events_2025_11
├─ audit_events_2025_12
├─ audit_events_2026_01 ← Events are here
├─ audit_events_2026_02
├─ audit_events_2026_03
└─ audit_events_2026_04
```

**Findings**:
- ✅ Database connection working
- ✅ User has all permissions
- ✅ Partitions configured correctly
- ✅ Events in correct partition (2026-01)

---

## The Paradox

```
┌─────────────────────────────────────────────────────────────┐
│ DIRECT SQL QUERY (via psql):                               │
│   SELECT COUNT(*) FROM audit_events                         │
│   WHERE event_type = 'notification.message.sent'            │
│     AND event_category = 'notification'                     │
│     AND correlation_id = '27b7ab77-6ed0-4fe6-99ff-742a1c...'│
│                                                             │
│   Result: 2 rows ✅                                         │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│ DATASTORAGE API QUERY (same filters):                      │
│   GET /api/v1/audit/events                                  │
│     ?event_type=notification.message.sent                   │
│     &event_category=notification                            │
│     &correlation_id=27b7ab77-6ed0-4fe6-99ff-742a1c...       │
│                                                             │
│   Result: {"data":[], "pagination":{"total":0}} ❌          │
└─────────────────────────────────────────────────────────────┘
```

---

## Root Cause Analysis

### What We Ruled Out

**❌ Table Partitioning Issue**:
- Partitions configured correctly
- Events in correct partition (2026-01)
- Direct SQL queries work

**❌ Database Connection Issue**:
- slm_user exists and has permissions
- Events being written successfully
- No connection errors in logs

**❌ Auth Middleware Blocking Queries**:
- All queries return 200 OK
- No 401/403 errors
- Auth successful (logs confirm)

**❌ Test Query Parameters Wrong**:
- Tests use correct parameters (correlation_id, event_type, event_category)
- All parameters optional (no required fields)
- Parameters match what SQL query needs

**❌ Event Structure Mismatch**:
- Events have all mandatory fields
- actor_id, correlation_id, event_type all populated correctly
- Same structure as Gateway (which works)

### What Remains

**🎯 DataStorage Query Execution Bug**:

The bug is in **how DataStorage executes queries**, NOT in:
- The SQL query itself (works in psql)
- The database setup (correct)
- The network/auth (working)
- The test code (correct)

**Possible Causes**:

1. **Query Builder Bug**:
   - SQL query constructed incorrectly
   - WHERE clause malformed
   - Partition pruning not working

2. **Connection Pool Issue**:
   - Query connection using wrong database
   - Read replica lag (unlikely - no replicas)
   - Transaction isolation seeing stale data

3. **ORM/Driver Bug**:
   - `pgx` driver issue
   - Parameter binding mismatch
   - Result set parsing error

---

## Why Is This NT-Specific?

**Other Services Work Fine**:
- ✅ Gateway: 98/98 (100%) after port fix
- ✅ WorkflowExecution: 12/12 (100%)
- ✅ RemediationOrchestrator: 29/29 (100%)

**NT Audit Patterns Different?**:
```
Gateway events:
- event_category: "gateway"
- actor_id: "gateway"
- Simpler event structure

Notification events:
- event_category: "notification"
- actor_id: "notification-controller"
- Multiple event types (sent, acknowledged, failed)
- More complex correlations
```

**Hypothesis**: NT's event structure or timing triggers edge case in DataStorage query logic.

---

## Evidence Summary

| Check | Status | Evidence |
|-------|--------|----------|
| Events in DB | ✅ PASS | 110 events, proper structure |
| SQL query works | ✅ PASS | Returns 2 events with filters |
| DS receives queries | ✅ PASS | 126 GET requests logged |
| DS executes queries | ✅ PASS | Returns 200 OK |
| DS returns results | ❌ FAIL | ALL queries return count:0 |
| Auth working | ✅ PASS | No 401/403 errors |
| Config correct | ✅ PASS | slm_user exists, has permissions |
| Partitions correct | ✅ PASS | Events in audit_events_2026_01 |

---

## Next Steps to Fix

### Option A: Add SQL Query Logging (RECOMMENDED)

**Modify**: `pkg/datastorage/repository/audit_events_repository.go`

```go
func (r *AuditEventsRepository) Query(ctx context.Context, querySQL string, countSQL string, args []interface{}) ([]*AuditEvent, *PaginationMetadata, error) {
    // ADD DEBUG LOGGING
    r.logger.Info("🔍 DEBUG: Executing query",
        "sql", querySQL,
        "args", args)
    
    // Execute count query
    var total int
    countArgs := args
    if len(args) >= 2 {
        countArgs = args[:len(args)-2]
    }
    
    // ADD DEBUG LOGGING
    r.logger.Info("🔍 DEBUG: Executing count",
        "sql", countSQL,
        "args", countArgs)
    
    err := r.db.QueryRowContext(ctx, countSQL, countArgs...).Scan(&total)
    if err != nil {
        return nil, nil, fmt.Errorf("failed to count audit events: %w", err)
    }
    
    // ADD DEBUG LOGGING
    r.logger.Info("🔍 DEBUG: Count result",
        "total", total)
    
    // ... rest of function
}
```

**Then rerun NT E2E** and check logs for actual SQL being executed.

### Option B: Test Query Builder Directly

**Create test**: `pkg/datastorage/query/audit_events_builder_test.go`

```go
func TestQueryBuilder_NotificationFilters(t *testing.T) {
    builder := NewAuditEventsQueryBuilder(WithAuditEventsLogger(log.Log))
    
    builder = builder.
        WithEventType("notification.message.sent").
        WithService("notification").
        WithCorrelationID("27b7ab77-6ed0-4fe6-99ff-742a1c8d3ae2").
        WithPagination(50, 0)
    
    querySQL, args, err := builder.Build()
    require.NoError(t, err)
    
    // Print SQL for manual inspection
    t.Logf("SQL: %s", querySQL)
    t.Logf("Args: %v", args)
    
    // Verify SQL contains all filters
    assert.Contains(t, querySQL, "event_type")
    assert.Contains(t, querySQL, "event_category")  
    assert.Contains(t, querySQL, "correlation_id")
}
```

### Option C: Compare with Gateway Query

**Hypothesis**: Gateway queries work, NT queries don't.

**Investigation**:
1. Run Gateway E2E with SQL logging
2. Capture working SQL query
3. Run NT E2E with SQL logging
4. Capture failing SQL query
5. Compare for differences

---

## Timeline

| Time | Event |
|------|-------|
| 20:28 | Started NT E2E with KEEP_CLUSTER=true |
| 20:36 | Tests completed (24/30, 6 failed) |
| 20:37 | Cluster preserved successfully |
| 20:40 | Found database: `action_history` |
| 20:42 | Confirmed 107 events in database |
| 20:44 | SQL query with filters works (returns 2) |
| 20:46 | Found 126 GET requests in DS logs |
| 20:48 | ALL queries return count:0, total:0 |
| 20:50 | Confirmed slm_user exists, has permissions |
| 20:52 | **ROOT CAUSE**: DataStorage query execution bug |

---

## Impact Assessment

### Test Failures: 6/30 (20%)

**Failing Tests** (all audit-related):
1. E2E Test 1: Full Notification Lifecycle with Audit
2. E2E Test 2: Audit Correlation Across Multiple Notifications
3. E2E Test: Failed Delivery Audit Event (2 tests)
4. TLS/HTTPS Failure Scenarios (2 tests)

**Passing Tests**: 24/30 (80%)
- All business logic tests pass ✅
- File delivery works ✅
- Channel fanout works ✅
- Priority routing works ✅
- Only audit verification fails ❌

### Severity: MEDIUM-HIGH

- ✅ Core functionality works (notifications delivered)
- ✅ Controller logic correct (buffer/flush working)
- ✅ DataStorage accepts events (107 written successfully)
- ❌ Query API broken for NT (returns empty results)
- ❌ SOC2/compliance tracking broken (can't retrieve audit trail)
- ⚠️  **NT-specific** (other services work fine)

---

## Key Learnings

### Must-Gather with KEEP_CLUSTER Works Perfectly

```bash
KEEP_CLUSTER=true make test-e2e-notification
# Cluster preserved automatically
# Kubeconfig: ~/.kube/notification-e2e-config
# Can exec into pods, query PostgreSQL directly
```

### PostgreSQL Access Pattern

```bash
# Port-forward (optional, exec is easier)
kubectl -n ns port-forward svc/postgresql 5432:5432

# Direct exec (RECOMMENDED)
PG_POD=$(kubectl -n ns get pods -l app=postgresql -o jsonpath='{.items[0].metadata.name}')
kubectl -n ns exec -i "$PG_POD" -- \
  sh -c 'PGPASSWORD="$POSTGRES_PASSWORD" psql -U "$POSTGRES_USER" -d action_history -c "SELECT COUNT(*) FROM audit_events;"'
```

### Database Name Discovery

```bash
# List databases
kubectl exec -i $PG_POD -- \
  sh -c 'echo "SELECT datname FROM pg_database;" | PGPASSWORD="$POSTGRES_PASSWORD" psql -U "$POSTGRES_USER" -d template1'
```

---

## Recommendations

### Immediate (This PR)

**Option 1**: Add SQL debug logging, rerun tests, identify bad query
- Time: 1-2 hours
- High chance of finding root cause
- Can fix in same PR

**Option 2**: Document known issue, proceed with 80% pass rate
- NT audit query broken (6/30 tests)
- Other services 100% (Gateway, RO, WE)
- Fix in separate PR with SQL logging

**Recommendation**: **Option 1** - Add SQL logging and fix now

### Long-term

1. **Add actor_id query parameter** to DataStorage API
2. **Add SQL query logging** (conditional, debug level)
3. **Add integration test** comparing SQL vs API results
4. **Document query builder patterns** for all services

---

## Technical Debt

1. **DataStorage query logging**: No SQL queries logged (makes debugging hard)
2. **Query builder tests**: No unit tests for filter combinations
3. **Integration test gap**: No test verifying SQL matches API results
4. **Documentation**: Query API filter behavior not documented

---

**Status**: ✅ Root cause narrowed to DataStorage query execution bug  
**Date**: January 31, 2026 01:50 UTC  
**Investigation**: 1h 22m (cluster preserved + PostgreSQL access)  
**Commits**: 25 ahead of origin  
**Next**: Add SQL debug logging OR document known issue
