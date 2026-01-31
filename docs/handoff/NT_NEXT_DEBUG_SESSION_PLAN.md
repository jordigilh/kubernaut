# Notification Audit Debug - Next Session Plan

**Date**: January 31, 2026  
**Status**: Cluster Preservation + Direct API Testing  
**Goal**: Isolate bug location (OpenAPI client vs DataStorage service vs DB schema)

---

## Strategy

Use **preserved cluster** + **direct curl testing** to bypass OpenAPI client and test DataStorage API directly. This will reveal whether the bug is in:
- **OpenAPI spec/client** (old spec, wrong parameters)
- **DataStorage service** (query builder, result parsing)
- **Database schema** (table/column mismatch)

---

## Step-by-Step Debug Plan

### Step 1: Preserve Cluster (5 min)

```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut

# Run NT E2E with cluster preservation
KEEP_CLUSTER=true make test-e2e-notification

# Cluster preserved at:
# - Name: notification-e2e
# - Kubeconfig: ~/.kube/notification-e2e-config
# - Namespace: notification-e2e
```

---

### Step 2: Verify Events in Database (2 min)

```bash
export KUBECONFIG=~/.kube/notification-e2e-config

# Get PostgreSQL pod
PG_POD=$(kubectl -n notification-e2e get pods -l app=postgresql -o jsonpath='{.items[0].metadata.name}')

# Check event count
kubectl -n notification-e2e exec -i "$PG_POD" -- \
  sh -c 'PGPASSWORD="$POSTGRES_PASSWORD" psql -U "$POSTGRES_USER" -d action_history -c "
SELECT COUNT(*) as total,
       event_category,
       event_type
FROM audit_events
WHERE event_category = '\''notification'\''
GROUP BY event_category, event_type;"'

# Get sample correlation IDs for testing
kubectl -n notification-e2e exec -i "$PG_POD" -- \
  sh -c 'PGPASSWORD="$POSTGRES_PASSWORD" psql -U "$POSTGRES_USER" -d action_history -c "
SELECT DISTINCT correlation_id
FROM audit_events
WHERE event_category = '\''notification'\''
LIMIT 5;"'
```

---

### Step 3: Test DataStorage API Directly with curl (10 min)

**Get DataStorage pod for port-forward**:
```bash
DS_POD=$(kubectl -n notification-e2e get pods -l app=datastorage -o jsonpath='{.items[0].metadata.name}')

# Port-forward DataStorage API
kubectl -n notification-e2e port-forward "$DS_POD" 8080:8080 &
PF_PID=$!
```

**Test 1: Query with NO filters (should return all events)**:
```bash
# This tests if API can return ANY events at all
curl -s "http://localhost:8080/api/v1/audit/events?limit=10" | jq '.'

# Expected: Some events (total > 0)
# Actual (current bug): {"data":[], "pagination":{"total":0}}
```

**Test 2: Query by event_category only**:
```bash
curl -s "http://localhost:8080/api/v1/audit/events?event_category=notification&limit=10" | jq '.'

# Expected: notification events
# Actual: Empty?
```

**Test 3: Query by event_type only**:
```bash
curl -s "http://localhost:8080/api/v1/audit/events?event_type=notification.message.sent&limit=10" | jq '.'

# Expected: notification.message.sent events
# Actual: Empty?
```

**Test 4: Query by correlation_id** (use one from Step 2):
```bash
CORR_ID="27b7ab77-6ed0-4fe6-99ff-742a1c8d3ae2"  # Example from previous run

curl -s "http://localhost:8080/api/v1/audit/events?correlation_id=$CORR_ID" | jq '.'

# Expected: Events for this correlation_id
# Actual: Empty?
```

**Test 5: Query with ALL filters** (like tests do):
```bash
curl -s "http://localhost:8080/api/v1/audit/events?event_type=notification.message.sent&event_category=notification&correlation_id=$CORR_ID" | jq '.'

# This mimics exactly what test code does
```

**Test 6: Check OpenAPI spec compliance**:
```bash
# Check if DataStorage expects different parameter names
curl -s "http://localhost:8080/openapi.json" | jq '.paths["/api/v1/audit/events"].get.parameters'

# Verify parameter names match:
# - event_type
# - event_category
# - correlation_id
# - limit
# - offset
```

---

### Step 4: Compare OpenAPI Client vs Direct curl (5 min)

**Check generated OpenAPI client**:
```bash
# See what parameters the client uses
cat pkg/datastorage/ogen-client/oas_parameters_gen.go | grep -A 20 "QueryAuditEventsParams"
```

**Expected Parameters**:
```go
type QueryAuditEventsParams struct {
    EventType      OptString
    EventCategory  OptString
    CorrelationID  OptString
    Since          OptDateTime
    Until          OptDateTime
    Limit          OptInt
    Offset         OptInt
}
```

**Check for mismatches**:
- Are parameter names correct? (snake_case vs camelCase)
- Are parameters optional? (Opt prefix)
- Does OpenAPI spec match what DataStorage expects?

---

### Step 5: Check DataStorage Request Logs (3 min)

While curl is running, watch DataStorage logs in real-time:

```bash
kubectl -n notification-e2e logs -f "$DS_POD" | grep "audit/events"
```

**Look for**:
- Actual query parameters received
- "Audit events queried successfully" with count
- Any filter parsing logs
- Any SQL-related logs (if we add logging)

---

### Step 6: Test Different Parameter Formats (5 min)

```bash
# Try snake_case (if DataStorage expects it)
curl -s "http://localhost:8080/api/v1/audit/events?event_category=notification" | jq '.'

# Try camelCase (if DataStorage expects it)
curl -s "http://localhost:8080/api/v1/audit/events?eventCategory=notification" | jq '.'

# Try with header authentication (like test does)
TEST_SA_TOKEN=$(kubectl -n notification-e2e create token notification-e2e-sa)

curl -s -H "Authorization: Bearer $TEST_SA_TOKEN" \
  "http://localhost:8080/api/v1/audit/events?event_category=notification" | jq '.'
```

---

## Expected Outcomes

### Scenario A: curl Works, OpenAPI Client Fails
**Diagnosis**: Bug in OpenAPI spec or generated client
**Fix**: Update OpenAPI spec, regenerate client
**Evidence**: curl returns events, test client returns empty

### Scenario B: curl Fails, Direct psql Works
**Diagnosis**: Bug in DataStorage query execution
**Fix**: Add SQL logging, fix query builder
**Evidence**: curl returns empty, psql returns events

### Scenario C: Different Parameters Work
**Diagnosis**: Parameter name mismatch (snake_case vs camelCase)
**Fix**: Align parameter names across stack
**Evidence**: curl with different param names returns events

### Scenario D: Everything Fails
**Diagnosis**: Database schema issue or connection pooling bug
**Fix**: Check table schema, connection pool config
**Evidence**: No query works, even without filters

---

## Key Questions to Answer

1. **Does curl with NO filters return events?**
   - YES → API works, filters are the problem
   - NO → API fundamentally broken

2. **Does curl with ONE filter work?**
   - YES → Multiple filter combination bug
   - NO → Single filter processing broken

3. **Do parameter names matter?**
   - event_category vs eventCategory
   - correlation_id vs correlationId

4. **Does auth affect results?**
   - With token vs without token
   - Different service accounts

5. **What do DataStorage logs show?**
   - Are filters received correctly?
   - Is count query returning 0 from DB?

---

## Files to Check

### DataStorage API Handler
```bash
cat pkg/datastorage/server/audit_events_handler.go | grep -A 30 "parseQueryFilters"
```

**Look for**:
- How URL params are parsed
- Parameter name mapping
- Filter construction

### Query Builder
```bash
cat pkg/datastorage/query/audit_events_builder.go | grep -A 50 "WithEventType\|WithEventCategory"
```

**Look for**:
- How filters are added to SQL
- WHERE clause construction
- Parameter binding

### OpenAPI Spec
```bash
cat pkg/datastorage/openapi.json | jq '.paths["/api/v1/audit/events"]'
```

**Check**:
- Parameter names (in: query)
- Required vs optional
- Data types

---

## Cleanup

```bash
# Stop port-forward
kill $PF_PID

# Delete cluster when done
kind delete cluster --name=notification-e2e
rm ~/.kube/notification-e2e-config
```

---

## Documentation to Update

After debug session, update:

1. **Root Cause Document**: Add curl test results
2. **Fix Implementation**: Based on findings
3. **Test Plan**: Add curl-based integration test

---

## Success Criteria

**Debug session successful if we can answer**:
- ✅ Does curl return events? (YES/NO)
- ✅ Which filters work/don't work? (list)
- ✅ What's the bug location? (client/service/db)
- ✅ What's the exact fix needed? (specific code change)

---

## Previous Investigation Results

**What We Know**:
- ✅ 110 events in PostgreSQL (verified)
- ✅ Direct SQL query works (returns 2 events)
- ✅ OpenAPI client queries return 0 (all 78 queries)
- ✅ No errors in logs
- ❌ No SQL logging (can't see actual SQL)

**What We Need to Know**:
- ❓ Does curl return events?
- ❓ Are parameters named correctly?
- ❓ Is OpenAPI spec wrong?
- ❓ Is query builder broken?

---

**Next Session**: Run this plan with preserved cluster + curl testing  
**Estimated Time**: 30-45 minutes  
**Expected Output**: Exact bug location + fix strategy
