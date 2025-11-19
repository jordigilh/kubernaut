# DD-STORAGE-010: Query API Pagination Strategy

**Status**: Approved
**Date**: 2025-01-18
**Deciders**: Engineering Team
**Related**: ADR-034 (Unified Audit Table Design), BR-STORAGE-021 (REST API Read Endpoints)

---

## Context

The Data Storage Service needs a REST API for querying audit events from the `audit_events` table. The API must support:

1. **Query Filters**: `correlation_id`, `event_type`, `service`, `outcome`, `severity`, time ranges
2. **Pagination**: Handle large result sets efficiently
3. **Performance**: Query latency <100ms for typical queries
4. **Consistency**: Reliable results even with concurrent inserts

Two pagination strategies were evaluated:
- **Offset-Based Pagination**: `limit` and `offset` parameters
- **Cursor-Based Pagination**: Opaque cursor token marking position

---

## Decision

### **V1.0: Implement Offset-Based Pagination**

**Rationale**:
1. **Business Requirements Alignment** (100%)
   - BR-STORAGE-021: REST API Read Endpoints
   - BR-STORAGE-023: Pagination Validation (limit: 1-1000, offset: ≥0)
   - ADR-034 Phase 1.3: Query API (4 hours)

2. **Use Case Fit** (95%)
   - Remediation timelines are **bounded** (typically <1000 events)
   - Most queries are for **completed remediations** (static historical data)
   - Users expect **page navigation** ("page 1, 2, 3")

3. **Implementation Simplicity** (90%)
   - 5-6 hours implementation (vs 12-16 for cursor-based)
   - Reuses existing query builder patterns
   - Standard HTTP query parameters

4. **Industry Alignment** (100%)
   - AWS CloudWatch, Datadog, Azure Monitor use offset-based for historical queries
   - Proven pattern for bounded time-range queries

### **V1.1: Add Cursor-Based Pagination (Future)**

**Deferred Until**:
- Live remediation monitoring feature (real-time event stream)
- Query performance profiling shows OFFSET scans >100ms
- User feedback requests infinite scroll or real-time feeds

**Estimated Effort**: 3-4 hours (enhancement)

---

## API Specification

### **V1.0: Offset-Based Pagination**

#### **Endpoint**
```http
GET /api/v1/audit/events
```

#### **Query Parameters**

| Parameter | Type | Required | Example | Description |
|-----------|------|----------|---------|-------------|
| `correlation_id` | string | No | `rr-2025-001` | Filter by remediation request |
| `event_type` | string | No | `gateway.signal.received` | Filter by event type |
| `service` | string | No | `gateway`, `aianalysis`, `workflow` | Filter by service |
| `outcome` | string | No | `success`, `failure` | Filter by outcome |
| `severity` | string | No | `critical`, `warning`, `info` | Filter by severity |
| `since` | string | No | `24h`, `2025-01-15T10:00:00Z` | Start time (relative or absolute) |
| `until` | string | No | `2025-01-15T23:59:59Z` | End time (absolute only) |
| `limit` | int | No | `50` (default: 100) | Page size (1-1000) |
| `offset` | int | No | `100` (default: 0) | Page offset (≥0) |

#### **Time Parameter Formats**

**Relative Time** (parsed with `time.ParseDuration`):
- `24h` → Last 24 hours
- `7d` → Last 7 days
- `30m` → Last 30 minutes

**Absolute Time** (parsed with RFC3339):
- `2025-01-15T10:00:00Z`
- `2025-01-15T10:00:00-05:00`

#### **Response Format**

**Success (200 OK)**:
```json
{
  "data": [
    {
      "event_id": "550e8400-e29b-41d4-a716-446655440000",
      "event_type": "gateway.signal.received",
      "service": "gateway",
      "correlation_id": "rr-2025-001",
      "event_timestamp": "2025-01-15T10:30:00Z",
      "outcome": "success",
      "severity": "critical",
      "resource_type": "pod",
      "resource_id": "api-server-123",
      "actor_type": "service",
      "actor_id": "gateway",
      "event_data": {
        "service": "gateway",
        "version": "1.0",
        "data": {
          "gateway": {
            "signal_type": "prometheus",
            "alert_name": "HighMemoryUsage"
          }
        }
      }
    }
  ],
  "pagination": {
    "limit": 50,
    "offset": 0,
    "total": 245,
    "has_more": true
  }
}
```

**Error (400 Bad Request - RFC 7807)**:
```json
{
  "type": "https://kubernaut.io/errors/validation-error",
  "title": "Validation Error",
  "status": 400,
  "detail": "Validation failed for query parameters: limit - must be between 1 and 1000",
  "instance": "/api/v1/audit/events",
  "field_errors": {
    "limit": "must be between 1 and 1000"
  }
}
```

#### **SQL Query Pattern**

```sql
-- Query with filters and pagination
SELECT * FROM audit_events
WHERE correlation_id = $1
  AND event_type = $2
  AND event_timestamp >= $3
  AND event_timestamp <= $4
ORDER BY event_timestamp DESC, event_id DESC
LIMIT $5 OFFSET $6;

-- Count query for pagination metadata
SELECT COUNT(*) FROM audit_events
WHERE correlation_id = $1
  AND event_type = $2
  AND event_timestamp >= $3
  AND event_timestamp <= $4;
```

#### **Example Queries**

```bash
# Query by correlation_id (remediation timeline)
GET /api/v1/audit/events?correlation_id=rr-2025-001&limit=50

# Query by event_type (filter by signal source)
GET /api/v1/audit/events?event_type=gateway.signal.received&limit=50

# Query by time range (last 24 hours)
GET /api/v1/audit/events?since=24h&limit=50

# Query with multiple filters
GET /api/v1/audit/events?service=gateway&outcome=failure&since=24h&limit=50

# Pagination (page 2)
GET /api/v1/audit/events?correlation_id=rr-2025-001&limit=50&offset=50
```

---

### **V1.1: Cursor-Based Pagination (Future)**

#### **Additional Query Parameters**

| Parameter | Type | Required | Example | Description |
|-----------|------|----------|---------|-------------|
| `cursor` | string | No | `eyJldmVudF9pZCI6IjEyMzQ1In0=` | Opaque cursor token |

**Note**: `cursor` and `offset` are mutually exclusive. If `cursor` is provided, `offset` is ignored.

#### **Response Format**

```json
{
  "data": [...],
  "pagination": {
    "limit": 50,
    "total": 245,
    "has_more": true,
    "next_cursor": "eyJldmVudF9pZCI6IjEyMzQ1IiwidGltZXN0YW1wIjoiMjAyNS0wMS0xNVQxMDowMDowMFoifQ=="
  }
}
```

#### **Cursor Format**

**Encoded** (Base64 JSON):
```
eyJldmVudF9pZCI6IjEyMzQ1IiwidGltZXN0YW1wIjoiMjAyNS0wMS0xNVQxMDowMDowMFoifQ==
```

**Decoded**:
```json
{
  "event_id": "550e8400-e29b-41d4-a716-446655440000",
  "timestamp": "2025-01-15T10:00:00Z"
}
```

#### **SQL Query Pattern**

```sql
-- First page (no cursor)
SELECT * FROM audit_events
WHERE correlation_id = $1
ORDER BY event_timestamp DESC, event_id DESC
LIMIT 50;

-- Next page (with cursor)
SELECT * FROM audit_events
WHERE correlation_id = $1
  AND (event_timestamp, event_id) < ($2, $3)  -- Cursor values
ORDER BY event_timestamp DESC, event_id DESC
LIMIT 50;
```

#### **Example Queries**

```bash
# First page (no cursor)
GET /api/v1/audit/events?correlation_id=rr-2025-001&limit=50

# Next page (with cursor from previous response)
GET /api/v1/audit/events?correlation_id=rr-2025-001&limit=50&cursor=eyJldmVudF9pZCI6IjEyMzQ1In0=
```

---

## Comparison: Offset vs Cursor-Based Pagination

### **Offset-Based Pagination**

**Pros**:
- ✅ Simple to implement (5-6 hours)
- ✅ Easy to jump to any page (`offset=500` for page 11)
- ✅ Works well for static/historical datasets
- ✅ Familiar to users ("page 1, 2, 3" navigation)
- ✅ Industry standard for bounded time-range queries

**Cons**:
- ❌ **Duplicate Records**: If new events inserted while paginating
  ```
  User requests page 1 (offset=0, limit=50)  → Gets events 1-50
  [10 NEW EVENTS INSERTED]
  User requests page 2 (offset=50, limit=50) → Gets events 41-90 (10 duplicates!)
  ```
- ❌ **Skipped Records**: If events deleted while paginating
- ❌ **Performance**: `OFFSET 10000` scans and discards 10,000 rows (slow for large offsets)

**Use Cases**:
- ✅ Historical queries (completed remediations)
- ✅ Bounded datasets (<10,000 records)
- ✅ Static data (few inserts/deletes during pagination)
- ✅ Admin dashboards with page navigation

---

### **Cursor-Based Pagination**

**Pros**:
- ✅ **No Duplicates**: Cursor marks exact position, unaffected by inserts
- ✅ **No Skipped Records**: Consistent results even with deletions
- ✅ **Performance**: Uses indexed WHERE clause (no OFFSET scan)
- ✅ **Real-Time Data**: Works perfectly with constantly-changing datasets

**Cons**:
- ❌ Cannot jump to arbitrary page (no "page 10" concept)
- ❌ More complex to implement (12-16 hours)
- ❌ Cursor must include unique identifier (event_id) + sort key (timestamp)
- ❌ Cursor tokens are opaque (harder to debug)

**Use Cases**:
- ✅ Real-time data with constant inserts (live remediation monitoring)
- ✅ Large datasets (>10,000 records)
- ✅ Infinite scroll UIs (mobile apps, dashboards)
- ✅ Event streams (Twitter feed, Slack messages)

---

## Industry Examples

### **Offset-Based Pagination**

| Service | API Endpoint | Use Case |
|---------|--------------|----------|
| **AWS CloudWatch** | `GET /logs?limit=50&nextToken=...` | Historical log queries |
| **Datadog** | `GET /api/v2/logs?limit=50&page[offset]=100` | Audit log queries |
| **Azure Monitor** | `GET /logs/query?$top=50&$skip=100` | Time-range queries |

### **Cursor-Based Pagination**

| Service | API Endpoint | Use Case |
|---------|--------------|----------|
| **GitHub API** | `GET /repos/:owner/:repo/commits?per_page=50&after=cursor` | Repository commits |
| **Stripe API** | `GET /v1/charges?limit=50&starting_after=cursor` | Payment transactions |
| **Twitter API** | `GET /2/tweets/search?max_results=50&next_token=cursor` | Tweet feeds |
| **Kubernetes API** | `GET /api/v1/pods?limit=50&continue=cursor` | Pod lists |

---

## Performance Considerations

### **V1.0: Offset-Based Performance**

**Query Latency** (with indexes):
- **Small Offset** (`offset=0-100`): <10ms
- **Medium Offset** (`offset=100-1000`): 10-50ms
- **Large Offset** (`offset=1000-10000`): 50-200ms

**Indexes Required** (from ADR-034):
```sql
CREATE INDEX idx_audit_events_correlation_id ON audit_events(correlation_id);
CREATE INDEX idx_audit_events_event_type ON audit_events(event_type);
CREATE INDEX idx_audit_events_timestamp ON audit_events(event_timestamp DESC);
CREATE INDEX idx_audit_events_service ON audit_events(service);
```

**Target Performance**:
- Query latency: <100ms (99th percentile)
- Throughput: 100 queries/second
- Typical result set: 50-500 events per remediation

---

### **V1.1: Cursor-Based Performance**

**Query Latency** (with indexes):
- **Any Position**: <10ms (constant time, no OFFSET scan)

**Indexes Required** (composite index):
```sql
CREATE INDEX idx_audit_events_cursor ON audit_events(event_timestamp DESC, event_id DESC);
```

**Target Performance**:
- Query latency: <50ms (99th percentile)
- Throughput: 200 queries/second
- Infinite scroll support (no performance degradation)

---

## Implementation Plan

### **V1.0: Offset-Based Pagination** (5-6 hours)

#### **Phase 1: DO-RED** (1 hour)
**File**: `test/integration/datastorage/audit_events_query_api_test.go`

**Test Cases**:
1. Query by `correlation_id` returns all related events
2. Query by `event_type` filters correctly
3. Query by `service` filters correctly
4. Query by time range (`since=24h`) returns events within range
5. Query with multiple filters (`service=gateway&outcome=failure`)
6. Pagination returns correct subset with metadata
7. Invalid `limit` (0, 1001) returns RFC 7807 error
8. Invalid `offset` (-1) returns RFC 7807 error
9. Invalid `since` format returns RFC 7807 error
10. Empty result set returns `{"data": [], "pagination": {...}}`

---

#### **Phase 2: DO-GREEN** (2.5 hours)

**2.1 Time Parsing Helper** (30 minutes)
- **File**: `pkg/datastorage/query/time_parser.go`
- **Function**: `ParseTimeParam(param string) (time.Time, error)`
- **Support**: Relative (`24h`, `7d`) and absolute (RFC3339) formats

**2.2 Audit Events Query Builder** (30 minutes)
- **File**: `pkg/datastorage/query/audit_events_builder.go`
- **Type**: `AuditEventsQueryBuilder`
- **Methods**: `Build()`, `BuildCount()`

**2.3 Audit Events Repository** (30 minutes)
- **File**: `pkg/datastorage/repository/audit_events_repository.go`
- **Interface**: `AuditEventsRepository`
- **Method**: `Query(ctx, filters) ([]AuditEvent, *PaginationMetadata, error)`

**2.4 HTTP Handler** (1 hour)
- **File**: `pkg/datastorage/server/audit_events_handler.go` (enhance existing)
- **Method**: `GetEvents(w http.ResponseWriter, r *http.Request)`
- **Router**: `GET /api/v1/audit/events`

---

#### **Phase 3: DO-REFACTOR** (1 hour)

**3.1 Query Performance Optimization**
- Add integration test with 1000+ events
- Measure query latency (target: <100ms)
- Verify indexes are used (EXPLAIN ANALYZE)
- Add structured logging for slow queries (>100ms)

**3.2 Code Quality**
- Extract time parsing to helper function
- Add comprehensive error messages
- Add OpenAPI spec for GET endpoint
- Update changelog (V5.8 → V5.9)

---

#### **Phase 4: CHECK** (30 minutes)

**Validation Checklist**:
- ✅ All integration tests pass
- ✅ Query latency <100ms (with 1000+ events)
- ✅ RFC 7807 errors for invalid parameters
- ✅ OpenAPI spec updated
- ✅ No lint errors

---

### **V1.1: Cursor-Based Pagination** (3-4 hours)

**Deferred Until**:
- Live remediation monitoring feature added
- Query performance profiling shows OFFSET scans >100ms
- User feedback requests infinite scroll

**Implementation Tasks**:
1. Add `cursor` query parameter parsing
2. Implement cursor encoding/decoding (Base64 JSON)
3. Update SQL queries to use `WHERE (timestamp, event_id) < (cursor_timestamp, cursor_id)`
4. Add `next_cursor` to response
5. Maintain backward compatibility with offset-based pagination
6. Add integration tests for cursor-based queries

---

## Success Criteria

### **V1.0 Success Criteria**

✅ **Functional Requirements**:
- Query by `correlation_id` returns all related events in chronological order
- Query by `event_type` filters correctly
- Time range queries support relative (`24h`) and absolute (RFC3339) formats
- Pagination returns correct subset with metadata (`total`, `has_more`)
- Invalid parameters return RFC 7807 errors with specific field names

✅ **Non-Functional Requirements**:
- Query latency <100ms (with 1000+ events)
- Integration tests pass with real PostgreSQL
- No lint errors
- OpenAPI spec updated

---

### **V1.1 Success Criteria** (Future)

✅ **Functional Requirements**:
- Cursor-based pagination returns consistent results (no duplicates/skips)
- Backward compatibility with offset-based pagination maintained
- `next_cursor` returned in response when `has_more=true`

✅ **Non-Functional Requirements**:
- Query latency <50ms (constant time, no OFFSET scan)
- Integration tests pass for both offset and cursor-based pagination

---

## Consequences

### **Positive**

1. **V1.0 Simplicity** (95% confidence)
   - 5-6 hours implementation (vs 12-16 for cursor-based)
   - Reuses existing query builder patterns
   - Familiar pagination model for users

2. **V1.0 Business Alignment** (100% confidence)
   - Meets BR-STORAGE-021, BR-STORAGE-023 requirements
   - Follows ADR-034 Phase 1.3 specification
   - Industry standard for historical queries

3. **V1.1 Future-Proof** (90% confidence)
   - Cursor-based pagination can be added without breaking changes
   - Backward compatibility maintained
   - Performance optimization path defined

### **Negative**

1. **V1.0 Duplicate Risk** (10% concern)
   - Offset-based pagination can return duplicates if events inserted during pagination
   - **Mitigation**: Most queries are for completed remediations (static data)
   - **Acceptable**: V1.1 will add cursor-based for live monitoring

2. **V1.0 Performance Limitation** (15% concern)
   - Large offsets (>1000) may have slower queries (50-200ms)
   - **Mitigation**: Remediation timelines are typically <1000 events
   - **Acceptable**: V1.1 will optimize for large datasets

### **Neutral**

1. **Migration Path**
   - V1.1 cursor-based pagination can coexist with offset-based
   - Clients can choose pagination strategy based on use case
   - No breaking changes required

---

## References

- **ADR-034**: Unified Audit Table Design
- **BR-STORAGE-021**: REST API Read Endpoints
- **BR-STORAGE-023**: Pagination Validation
- **AWS CloudWatch Logs API**: https://docs.aws.amazon.com/AmazonCloudWatchLogs/latest/APIReference/
- **Datadog Logs API**: https://docs.datadoghq.com/api/latest/logs/
- **GitHub API Pagination**: https://docs.github.com/en/rest/guides/using-pagination-in-the-rest-api
- **Stripe API Pagination**: https://stripe.com/docs/api/pagination

---

## Approval

**Approved By**: Engineering Team
**Date**: 2025-01-18
**Decision**: Implement V1.0 offset-based pagination, defer V1.1 cursor-based to future enhancement

