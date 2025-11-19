# Event Data Schemas

**Version**: 1.0
**Last Updated**: November 18, 2025
**Purpose**: Standard event_data JSONB schemas for unified audit table (ADR-034)

---

## Overview

This directory contains the standardized event_data JSONB schemas for the unified `audit_events` table. Each schema defines the structure of service-specific audit data stored in the `event_data` JSONB column.

### Business Requirements

- **BR-STORAGE-033-001**: Standardized event_data JSONB structure
- **BR-STORAGE-033-002**: Type-safe event building API
- **BR-STORAGE-033-003**: Consistent field naming across services

---

## Available Schemas

| Service | Schema Document | Purpose |
|---------|----------------|---------|
| **Gateway** | [gateway_schema.md](./gateway_schema.md) | Signal ingestion, deduplication, storm detection |
| **AI Analysis** | [aianalysis_schema.md](./aianalysis_schema.md) | LLM analysis, RCA, workflow selection |
| **Workflow** | [workflow_schema.md](./workflow_schema.md) | Workflow execution, steps, approvals |

---

## Common Envelope Structure

All event_data follows this base structure:

```json
{
  "version": "1.0",
  "service": "gateway|aianalysis|workflow",
  "event_type": "service.specific.event",
  "timestamp": "2025-11-18T10:00:00Z",
  "data": {
    "service_name": {
      // Service-specific fields
    }
  }
}
```

### Base Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `version` | string | Yes | Schema version (semver) |
| `service` | string | Yes | Originating service name |
| `event_type` | string | Yes | Specific event identifier |
| `timestamp` | string | Yes | Event creation time (RFC3339) |
| `data` | object | Yes | Service-specific data |

---

## Usage with Go Builders

### Example: Gateway Event

```go
import "github.com/jordigilh/kubernaut/pkg/datastorage/audit"

eventData, err := audit.NewGatewayEvent("signal.received").
    WithSignalType("prometheus").
    WithAlertName("HighMemoryUsage").
    WithFingerprint("sha256:abc123").
    WithNamespace("production").
    WithResource("pod", "api-server-123").
    WithSeverity("critical").
    WithPriority("P0").
    WithEnvironment("production").
    Build()
```

### Example: AI Analysis Event

```go
eventData, err := audit.NewAIAnalysisEvent("analysis.completed").
    WithAnalysisID("analysis-2025-001").
    WithLLM("anthropic", "claude-haiku-4-5").
    WithTokenUsage(2500, 750).
    WithRCA("OOMKilled", "critical", 0.95).
    WithWorkflow("workflow-increase-memory").
    Build()
```

### Example: Workflow Event

```go
eventData, err := audit.NewWorkflowEvent("workflow.completed").
    WithWorkflowID("workflow-increase-memory").
    WithExecutionID("exec-2025-001").
    WithPhase("completed").
    WithCurrentStep(5, 5).
    WithDuration(45000).
    WithOutcome("success").
    Build()
```

---

## Querying Event Data

### PostgreSQL JSONB Queries

```sql
-- Find all Prometheus signals
SELECT * FROM audit_events
WHERE event_data->>'service' = 'gateway'
AND event_data->'data'->'gateway'->>'signal_type' = 'prometheus';

-- Find AI analyses with high confidence
SELECT * FROM audit_events
WHERE event_data->>'service' = 'aianalysis'
AND (event_data->'data'->'ai_analysis'->>'confidence')::float > 0.9;

-- Find workflows that required approval
SELECT * FROM audit_events
WHERE event_data->>'service' = 'workflow'
AND event_data->'data'->'workflow'->>'approval_required' = 'true';
```

### Using GIN Index for Performance

```sql
-- Efficient JSONB containment query (uses idx_audit_events_event_data_gin)
SELECT * FROM audit_events
WHERE event_data @> '{"service": "gateway", "data": {"gateway": {"signal_type": "prometheus"}}}';
```

---

## Schema Versioning

### Current Version: 1.0

- Initial release
- Support for Gateway, AI Analysis, and Workflow services

### Future Versions

Schema changes will follow semantic versioning:
- **Major**: Breaking changes (e.g., removing fields)
- **Minor**: Backward-compatible additions (e.g., new fields)
- **Patch**: Documentation fixes, clarifications

---

## Validation

### Go Builder Validation

All builders validate at build time:
```go
eventData, err := builder.Build()
if err != nil {
    // Handle validation error
}
```

### PostgreSQL JSONB Validation

```sql
-- Validate event_data has required base fields
SELECT event_id, event_data
FROM audit_events
WHERE NOT (
    event_data ? 'version' AND
    event_data ? 'service' AND
    event_data ? 'event_type' AND
    event_data ? 'timestamp' AND
    event_data ? 'data'
);
```

---

## Best Practices

### DO ✅
- Use Go builders for type safety
- Include all contextually relevant fields
- Follow naming conventions (snake_case for JSON)
- Preserve original payloads in base64 when debugging

### DON'T ❌
- Manually construct JSONB maps
- Use inconsistent field names
- Store sensitive data unencrypted
- Omit correlation IDs

---

## Related Documentation

- **[ADR-034: Unified Audit Table Design](../../../../architecture/decisions/ADR-034-unified-audit-table-design.md)**
- **[Data Storage Service API](../../api-specification.md)**
- **[Integration Points](../../integration-points.md)**

---

**For Questions**: Refer to individual schema documents for service-specific field definitions and examples.

