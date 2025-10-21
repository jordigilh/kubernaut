# Context API to Data Storage Schema Mapping

**Version**: 1.0
**Date**: 2025-10-21
**Status**: Active
**Authority**: Data Storage Service owns canonical schema

---

## Overview

This document maps Context API's `IncidentEvent` model fields to Data Storage Service's database schema. Context API is a **consumer service** that adapts to Data Storage's authoritative schema via SQL JOINs.

**Design Decision**: [DD-SCHEMA-001](../../../architecture/decisions/DD-SCHEMA-001-data-storage-schema-authority.md)

---

## Schema Architecture

Context API queries combine data from three Data Storage tables:

```
resource_action_traces (RAT)
    ↓ JOIN (action_history_id)
action_histories (AH)
    ↓ JOIN (resource_id)
resource_references (RR)
```

---

## Field Mapping

### Primary Identification

| Context API Field | Data Storage Mapping | Table | Notes |
|-------------------|---------------------|-------|-------|
| `id` | `rat.id` | resource_action_traces | Primary key |
| `name` | `rat.alert_name` | resource_action_traces | Alert name (e.g., "HighMemoryUsage") |
| `alert_fingerprint` | `rat.alert_fingerprint` | resource_action_traces | Added in migration 008 |
| `remediation_request_id` | `rat.action_id` | resource_action_traces | UUID for this action |

**SQL**:
```sql
rat.id,
rat.alert_name AS name,
rat.alert_fingerprint,
rat.action_id AS remediation_request_id
```

---

### Context Fields

| Context API Field | Data Storage Mapping | Table | Notes |
|-------------------|---------------------|-------|-------|
| `namespace` | `rr.namespace` | resource_references | Kubernetes namespace |
| `cluster_name` | `rat.cluster_name` | resource_action_traces | Added in migration 008 |
| `environment` | `rat.environment` | resource_action_traces | Added in migration 008 (prod/staging/dev) |
| `target_resource` | `rr.kind` | resource_references | Resource type (Deployment, Pod, etc.) |

**SQL**:
```sql
rr.namespace,
rat.cluster_name,
rat.environment,
rr.kind AS target_resource
```

**Filtering**:
- Namespace filter: `WHERE rr.namespace = ?`
- Cluster filter: `WHERE rat.cluster_name = ?`
- Environment filter: `WHERE rat.environment = ?`

---

### Status Fields

| Context API Field | Data Storage Mapping | Table | Notes |
|-------------------|---------------------|-------|-------|
| `phase` | `CASE rat.execution_status` | resource_action_traces | Derived from execution_status |
| `status` | `rat.execution_status` | resource_action_traces | pending, completed, failed, etc. |
| `severity` | `rat.alert_severity` | resource_action_traces | critical, warning, info |
| `action_type` | `rat.action_type` | resource_action_traces | scale, restart, delete, etc. |

**SQL**:
```sql
CASE rat.execution_status
    WHEN 'completed' THEN 'completed'
    WHEN 'failed' THEN 'failed'
    WHEN 'rolled-back' THEN 'failed'
    WHEN 'pending' THEN 'pending'
    WHEN 'executing' THEN 'processing'
    ELSE 'pending'
END AS phase,
rat.execution_status AS status,
rat.alert_severity AS severity,
rat.action_type
```

**Filtering**:
- Phase filter: Use CASE expression in WHERE clause
- Status filter: `WHERE rat.execution_status = ?`
- Severity filter: `WHERE rat.alert_severity = ?`
- Action type filter: `WHERE rat.action_type = ?`

---

### Timing Fields

| Context API Field | Data Storage Mapping | Table | Notes |
|-------------------|---------------------|-------|-------|
| `start_time` | `rat.action_timestamp` | resource_action_traces | When action started |
| `end_time` | `rat.execution_end_time` | resource_action_traces | When action completed (nullable) |
| `duration` | `rat.execution_duration_ms` | resource_action_traces | Duration in milliseconds (nullable) |

**SQL**:
```sql
rat.action_timestamp AS start_time,
rat.execution_end_time AS end_time,
rat.execution_duration_ms AS duration
```

**Time Range Filtering**:
```sql
WHERE rat.action_timestamp >= ? AND rat.action_timestamp < ?
```

---

### Error Tracking

| Context API Field | Data Storage Mapping | Table | Notes |
|-------------------|---------------------|-------|-------|
| `error_message` | `rat.execution_error` | resource_action_traces | Error details (nullable) |

**SQL**:
```sql
rat.execution_error AS error_message
```

---

### Metadata

| Context API Field | Data Storage Mapping | Table | Notes |
|-------------------|---------------------|-------|-------|
| `metadata` | `rat.action_parameters` | resource_action_traces | JSONB, serialized to string |

**SQL**:
```sql
rat.action_parameters::TEXT AS metadata
```

**Note**: Context API expects string, Data Storage stores JSONB. Cast required.

---

### Vector Embeddings

| Context API Field | Data Storage Mapping | Table | Notes |
|-------------------|---------------------|-------|-------|
| `embedding` | `rat.embedding` | resource_action_traces | From migration 006 (384-dim) |

**SQL**:
```sql
rat.embedding
```

**Semantic Search**:
```sql
WHERE rat.embedding <=> ? < ?  -- pgvector cosine distance
ORDER BY rat.embedding <=> ?
LIMIT ?
```

---

### Audit Timestamps

| Context API Field | Data Storage Mapping | Table | Notes |
|-------------------|---------------------|-------|-------|
| `created_at` | `rat.created_at` | resource_action_traces | When record created |
| `updated_at` | `rat.updated_at` | resource_action_traces | When record updated |

**SQL**:
```sql
rat.created_at,
rat.updated_at
```

---

## Complete JOIN Query

### Base Query for ListIncidents

```sql
SELECT
    -- Primary identification
    rat.id,
    rat.alert_name AS name,
    rat.alert_fingerprint,
    rat.action_id AS remediation_request_id,

    -- Context
    rr.namespace,
    rat.cluster_name,
    rat.environment,
    rr.kind AS target_resource,

    -- Status
    CASE rat.execution_status
        WHEN 'completed' THEN 'completed'
        WHEN 'failed' THEN 'failed'
        WHEN 'rolled-back' THEN 'failed'
        WHEN 'pending' THEN 'pending'
        WHEN 'executing' THEN 'processing'
        ELSE 'pending'
    END AS phase,
    rat.execution_status AS status,
    rat.alert_severity AS severity,
    rat.action_type,

    -- Timing
    rat.action_timestamp AS start_time,
    rat.execution_end_time AS end_time,
    rat.execution_duration_ms AS duration,

    -- Error tracking
    rat.execution_error AS error_message,

    -- Metadata
    rat.action_parameters::TEXT AS metadata,

    -- Vector embeddings (optional, only for semantic search)
    -- rat.embedding,

    -- Audit timestamps
    rat.created_at,
    rat.updated_at

FROM resource_action_traces rat
JOIN action_histories ah ON rat.action_history_id = ah.id
JOIN resource_references rr ON ah.resource_id = rr.id
```

### WHERE Clause Patterns

```sql
-- Namespace filter
WHERE rr.namespace = $1

-- Severity filter
WHERE rat.alert_severity = $1

-- Cluster filter
WHERE rat.cluster_name = $1

-- Environment filter
WHERE rat.environment = $1

-- Action type filter
WHERE rat.action_type = $1

-- Time range filter
WHERE rat.action_timestamp >= $1 AND rat.action_timestamp < $2

-- Combined filters (all use AND)
WHERE rr.namespace = $1
  AND rat.alert_severity = $2
  AND rat.cluster_name = $3
```

### ORDER BY and LIMIT

```sql
ORDER BY rat.action_timestamp DESC
LIMIT $1 OFFSET $2
```

### Count Query

```sql
SELECT COUNT(*)
FROM resource_action_traces rat
JOIN action_histories ah ON rat.action_history_id = ah.id
JOIN resource_references rr ON ah.resource_id = rr.id
WHERE [same filters as main query]
```

---

## Migration Dependencies

Context API schema compatibility requires:

1. **Migration 001-007**: Base Data Storage schema (already applied)
2. **Migration 008**: Context API compatibility fields (new)
   - `alert_fingerprint VARCHAR(64)`
   - `cluster_name VARCHAR(100)`
   - `environment VARCHAR(20)`

---

## Performance Considerations

### Indexes Used

From Data Storage migrations:

```sql
-- Primary lookup
idx_rat_action_history (action_history_id)
idx_ah_resource_id (resource_id)

-- Filtering indexes
idx_resource_namespace (namespace)
idx_rat_alert_name (alert_name)
idx_rat_execution_status (execution_status)
idx_rat_action_type (action_type)

-- New indexes from migration 008
idx_rat_alert_fingerprint (alert_fingerprint)
idx_rat_cluster_name (cluster_name)
idx_rat_environment (environment)
idx_rat_context_filters (cluster_name, environment, alert_severity)

-- Time-based partitioning
resource_action_traces partitioned by action_timestamp (monthly)
```

### Query Optimization

1. **Partition Pruning**: Queries with time range benefit from monthly partitions
2. **Index-Only Scans**: Most filters use indexed columns
3. **JOIN Optimization**: Foreign key indexes on join columns
4. **JSONB Avoidance**: Direct columns instead of JSONB extraction

### Expected Performance

| Query Type | Expected Performance | Notes |
|-----------|---------------------|-------|
| Single namespace | < 10ms | Uses namespace index |
| Severity filter | < 20ms | Uses severity index |
| Time range (1 month) | < 50ms | Single partition scan |
| Time range (3 months) | < 150ms | 3 partition scans |
| Semantic search (k=10) | < 100ms | pgvector index scan |
| Combined filters | < 30ms | Composite index |

---

## Testing Strategy

### Unit Tests

Test SQL query generation in `test/unit/contextapi/sqlbuilder/`:
- Verify correct table aliases (rat., ah., rr.)
- Verify JOIN structure
- Verify field mappings
- Verify WHERE clause generation

### Integration Tests

Test actual database queries in `test/integration/contextapi/`:
- Insert test data into all 3 tables
- Verify query results match expected format
- Test filtering (namespace, severity, cluster, etc.)
- Test pagination
- Test semantic search

### Data Fixtures

```go
// Insert test data in correct order:
1. resource_references (namespace, kind, name)
2. action_histories (resource_id)
3. resource_action_traces (action_history_id, all fields)
```

---

## Future Schema Changes

All schema changes go through Data Storage Service migrations:

1. Data Storage team creates migration (e.g., `009_new_field.sql`)
2. Data Storage team runs migration on database
3. Context API team updates SQL queries to use new field
4. Context API team updates `IncidentEvent` model if needed
5. Both services rebuild and redeploy

**No direct database changes by Context API team.**

---

## Related Documentation

- [Data Storage Implementation Plan](../../../../crd-controllers/07-datastorage/implementation/IMPLEMENTATION_PLAN_V1.0.md)
- [Context API Implementation Plan](./IMPLEMENTATION_PLAN_V2.0.md)
- [DD-SCHEMA-001: Data Storage Schema Authority](../../../architecture/decisions/DD-SCHEMA-001-data-storage-schema-authority.md)
- [Migration 008: Context API Compatibility](../../../../../migrations/008_context_api_compatibility.sql)

---

## Revision History

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| 1.0 | 2025-10-21 | Initial schema mapping documentation | AI Assistant |


