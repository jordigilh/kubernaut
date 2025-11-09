# Kubernaut Event Data Format Design

**Date**: November 8, 2025
**Context**: Defining `event_data` JSONB format strategy for unified audit table
**Confidence**: 95%

---

## 🎯 Executive Summary

**Question**: Should `event_data` format be service-specific or common across all services?

**Answer**: ✅ **Hybrid Approach** - Common envelope with service-specific payloads (95% confidence)

**Key Decision**:
```
Common Envelope (Standardized)
    ↓
Service-Specific Payload (Flexible)
    ↓
Signal-Source Payload (Fully Flexible)
```

**Industry Pattern**: AWS CloudTrail, Google Cloud Audit Logs, and Kubernetes all use this hybrid approach.

---

## 📊 Industry Analysis: Event Data Patterns

### Pattern 1: Fully Common Format (Rare)

**Example**: Simplified logging systems
```json
{
  "message": "User logged in",
  "level": "info",
  "tags": ["auth", "user"]
}
```

**Pros**: ✅ Simple, consistent
**Cons**: ❌ Not flexible enough for diverse services
**Industry Usage**: <10% (only for simple logging)

---

### Pattern 2: Fully Service-Specific (Chaotic)

**Example**: No standardization
```json
// Gateway service (random structure)
{
  "alert": "HighCPU",
  "fp": "abc123",
  "ns": "prod"
}

// AI service (completely different structure)
{
  "model_name": "gpt-4",
  "token_count": 1500,
  "analysis_result": {...}
}
```

**Pros**: ✅ Maximum flexibility
**Cons**: ❌ No consistency, hard to query, no reusable tooling
**Industry Usage**: <5% (anti-pattern)

---

### Pattern 3: Hybrid Approach (Industry Standard) ⭐

**Example**: AWS CloudTrail, Google Cloud Audit Logs, Kubernetes

```json
{
  // Common envelope (standardized fields)
  "version": "1.0",
  "service": "gateway",
  "operation": "signal_received",

  // Service-specific payload (flexible)
  "payload": {
    // Gateway-specific fields
    "alert_name": "HighCPU",
    "signal_fingerprint": "abc123",
    "namespace": "production"
  },

  // Optional: Signal-source payload (fully flexible)
  "source_payload": {
    // Original signal from Prometheus/AWS/etc (untouched)
  }
}
```

**Pros**: ✅ Consistency + Flexibility, ✅ Queryable common fields, ✅ Extensible
**Cons**: ⚠️ Requires documentation
**Industry Usage**: >85% (industry standard)

---

## 🏗️ Recommended Format for Kubernaut

### **Hybrid Approach: Common Envelope + Service-Specific Payload**

**Confidence**: 95%

### Schema Structure

```
┌─────────────────────────────────────────────────────────────┐
│                    event_data JSONB                          │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  COMMON ENVELOPE (Standardized)                      │  │
│  │  - version, service, operation, status               │  │
│  │  → Consistent across all services                    │  │
│  │  → Queryable with JSON operators                     │  │
│  └──────────────────────────────────────────────────────┘  │
│                           +                                  │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  SERVICE PAYLOAD (Service-Specific)                  │  │
│  │  - Gateway: alert_name, signal_fingerprint, etc.     │  │
│  │  - AI: model, tokens, confidence, etc.               │  │
│  │  - Workflow: steps, actions, etc.                    │  │
│  │  → Service-specific business data                    │  │
│  └──────────────────────────────────────────────────────┘  │
│                           +                                  │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  SOURCE PAYLOAD (Optional, Fully Flexible)           │  │
│  │  - Original signal from external source              │  │
│  │  - Preserved for debugging/compliance                │  │
│  │  → Untouched external data                           │  │
│  └──────────────────────────────────────────────────────┘  │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

---

## 📋 Detailed Format Specification

### Common Envelope (Standardized Across All Services)

```json
{
  // ═══════════════════════════════════════════════════════════
  // COMMON ENVELOPE (Required, Standardized)
  // ═══════════════════════════════════════════════════════════

  "version": "1.0",                    // Schema version (for evolution)
  "service": "gateway",                // Service name (gateway, ai, workflow, etc.)
  "operation": "signal_received",      // Operation performed
  "status": "success",                 // Operation status (success, failure, partial)

  // Optional common fields
  "attempt": 1,                        // Retry attempt number (if applicable)
  "retry_reason": null,                // Reason for retry (if attempt > 1)

  // ═══════════════════════════════════════════════════════════
  // SERVICE PAYLOAD (Service-Specific)
  // ═══════════════════════════════════════════════════════════

  "payload": {
    // Service-specific fields (see per-service schemas below)
  },

  // ═══════════════════════════════════════════════════════════
  // SOURCE PAYLOAD (Optional, External Signal)
  // ═══════════════════════════════════════════════════════════

  "source_payload": {
    // Original signal from external source (optional, for debugging)
  }
}
```

---

## 🔍 Per-Service Payload Schemas

### 1. Gateway Service

**Operations**: `signal_received`, `signal_deduplicated`, `storm_detected`, `crd_created`

```json
{
  "version": "1.0",
  "service": "gateway",
  "operation": "signal_received",
  "status": "success",

  "payload": {
    // Signal identification
    "signal_fingerprint": "fp-abc123",
    "alert_name": "HighCPU",
    "severity": "critical",

    // Kubernetes context
    "namespace": "production",
    "cluster": "prod-cluster-01",
    "pod": "api-gateway-7d8f9c-xyz",

    // Deduplication
    "is_duplicate": false,
    "duplicate_count": 0,
    "first_seen": "2025-11-08T10:30:00Z",
    "last_seen": "2025-11-08T10:30:00Z",

    // Storm detection
    "storm_detected": false,
    "storm_window_count": 0,

    // Action taken
    "action": "created_crd",
    "crd_name": "remediation-req-001",
    "crd_namespace": "kubernaut-system"
  },

  "source_payload": {
    // Original Prometheus alert (untouched)
    "labels": {
      "alertname": "HighCPU",
      "namespace": "production",
      "pod": "api-gateway-7d8f9c-xyz"
    },
    "annotations": {
      "description": "CPU usage is above 80%",
      "summary": "High CPU detected"
    },
    "startsAt": "2025-11-08T10:30:00Z",
    "endsAt": null
  }
}
```

---

### 2. Context API Service

**Operations**: `query_executed`, `cache_hit`, `cache_miss`, `aggregation_executed`

```json
{
  "version": "1.0",
  "service": "context-api",
  "operation": "query_executed",
  "status": "success",

  "payload": {
    // Query details
    "query_type": "incident_query",
    "query_params": {
      "namespace": "production",
      "severity": "critical",
      "time_range": "24h"
    },

    // Results
    "result_count": 42,
    "query_duration_ms": 125,

    // Cache
    "cache_hit": true,
    "cache_key": "incidents:production:critical:24h",
    "cache_ttl_seconds": 300,

    // Data source
    "data_source": "data_storage_api",
    "data_storage_duration_ms": 0  // 0 because cache hit
  }
}
```

---

### 3. AI Analysis Service

**Operations**: `analysis_started`, `analysis_completed`, `context_optimized`, `llm_called`

```json
{
  "version": "1.0",
  "service": "ai-analysis",
  "operation": "analysis_completed",
  "status": "success",
  "attempt": 2,
  "retry_reason": "llm_timeout",

  "payload": {
    // LLM details
    "llm_provider": "ollama",
    "llm_model": "gpt-4",
    "llm_endpoint": "http://192.168.1.169:8080",

    // Token usage
    "input_tokens": 1500,
    "output_tokens": 500,
    "total_tokens": 2000,
    "estimated_cost_usd": 0.04,

    // Analysis results
    "analysis_type": "root_cause_analysis",
    "confidence_score": 0.92,
    "root_cause": "High CPU due to memory leak in pod api-gateway-7d8f9c-xyz",
    "recommended_actions": [
      "Restart pod",
      "Increase memory limit",
      "Enable memory profiling"
    ],

    // Context optimization
    "context_size_before": 15000,
    "context_size_after": 8000,
    "context_reduction_percent": 46.7,

    // Performance
    "analysis_duration_ms": 2300,
    "llm_call_duration_ms": 1800,
    "context_optimization_duration_ms": 500
  }
}
```

---

### 4. Workflow Service

**Operations**: `workflow_created`, `workflow_step_started`, `workflow_step_completed`, `workflow_completed`

```json
{
  "version": "1.0",
  "service": "workflow",
  "operation": "workflow_step_completed",
  "status": "success",

  "payload": {
    // Workflow identification
    "workflow_id": "wf-001",
    "workflow_type": "remediation",
    "workflow_phase": "execution",

    // Step details
    "step_number": 2,
    "step_total": 5,
    "step_name": "scale_deployment",
    "step_type": "kubernetes_action",

    // Step execution
    "step_duration_ms": 1500,
    "step_result": "success",
    "step_output": {
      "deployment": "api-gateway",
      "replicas_before": 3,
      "replicas_after": 5,
      "scaling_duration_ms": 1200
    },

    // Workflow progress
    "steps_completed": 2,
    "steps_remaining": 3,
    "estimated_completion_time": "2025-11-08T10:35:00Z"
  }
}
```

---

### 5. Data Storage Service

**Operations**: `audit_stored`, `query_executed`, `embedding_stored`, `dual_write_completed`

```json
{
  "version": "1.0",
  "service": "data-storage",
  "operation": "dual_write_completed",
  "status": "success",

  "payload": {
    // Write details
    "write_type": "audit_trail",
    "record_count": 1,

    // PostgreSQL
    "postgres_write_duration_ms": 15,
    "postgres_table": "audit_events",
    "postgres_row_id": "abc-123-def-456",

    // Redis
    "redis_write_duration_ms": 5,
    "redis_key": "audit:rr-2025-001:gateway",
    "redis_ttl_seconds": 86400,

    // Dual-write coordination
    "dual_write_duration_ms": 20,
    "dual_write_success": true,
    "rollback_required": false
  }
}
```

---

### 6. Execution Service

**Operations**: `action_started`, `action_completed`, `action_rolled_back`, `dry_run_executed`

```json
{
  "version": "1.0",
  "service": "execution",
  "operation": "action_completed",
  "status": "success",

  "payload": {
    // Action details
    "action_type": "kubectl_scale",
    "action_command": "kubectl scale deployment api-gateway --replicas=5",
    "dry_run": false,

    // Target resource
    "target_resource_type": "Deployment",
    "target_resource_name": "api-gateway",
    "target_namespace": "production",
    "target_cluster": "prod-cluster-01",

    // Execution results
    "execution_duration_ms": 1200,
    "exit_code": 0,
    "stdout": "deployment.apps/api-gateway scaled",
    "stderr": "",

    // Safety checks
    "safety_checks_passed": true,
    "safety_checks": [
      {
        "check": "rbac_permission",
        "result": "passed"
      },
      {
        "check": "resource_exists",
        "result": "passed"
      },
      {
        "check": "namespace_exists",
        "result": "passed"
      }
    ]
  }
}
```

---

## 🔍 External Signal Sources (Fully Flexible)

### OpenTelemetry Trace

```json
{
  "version": "1.0",
  "service": "gateway",
  "operation": "otel_trace_received",
  "status": "success",

  "payload": {
    // Minimal Kubernaut processing
    "trace_id": "abc123",
    "span_id": "xyz789",
    "parent_span_id": "parent123",
    "ingestion_timestamp": "2025-11-08T10:30:00Z"
  },

  "source_payload": {
    // Full OpenTelemetry span (untouched)
    "traceId": "abc123",
    "spanId": "xyz789",
    "parentSpanId": "parent123",
    "name": "POST /api/v1/users",
    "kind": "SPAN_KIND_SERVER",
    "startTimeUnixNano": "1699437000000000000",
    "endTimeUnixNano": "1699437000125000000",
    "attributes": {
      "http.method": "POST",
      "http.status_code": 200,
      "http.url": "/api/v1/users",
      "service.name": "api-gateway",
      "service.version": "1.2.3"
    }
  }
}
```

### AWS CloudWatch Alarm

```json
{
  "version": "1.0",
  "service": "gateway",
  "operation": "aws_alarm_received",
  "status": "success",

  "payload": {
    // Minimal Kubernaut processing
    "alarm_name": "HighCPU",
    "alarm_state": "ALARM",
    "metric_name": "CPUUtilization",
    "ingestion_timestamp": "2025-11-08T10:30:00Z"
  },

  "source_payload": {
    // Full AWS CloudWatch alarm (untouched)
    "AlarmName": "HighCPU",
    "AlarmArn": "arn:aws:cloudwatch:us-east-1:123456789012:alarm:HighCPU",
    "StateReason": "Threshold Crossed: 1 datapoint [95.0] was greater than the threshold (80.0)",
    "StateValue": "ALARM",
    "MetricName": "CPUUtilization",
    "Namespace": "AWS/EC2",
    "Dimensions": [
      {
        "Name": "InstanceId",
        "Value": "i-1234567890abcdef0"
      }
    ],
    "Threshold": 80.0,
    "ComparisonOperator": "GreaterThanThreshold"
  }
}
```

---

## 📊 Query Patterns with Common Envelope

### Query 1: All Events for a Service

```sql
-- Query by common envelope field
SELECT * FROM audit_events
WHERE event_data->>'service' = 'gateway'
  AND event_timestamp > NOW() - INTERVAL '24 hours'
ORDER BY event_timestamp DESC;
```

### Query 2: All Failed Operations

```sql
-- Query by common envelope field
SELECT * FROM audit_events
WHERE event_data->>'status' = 'failure'
  AND event_timestamp > NOW() - INTERVAL '24 hours'
ORDER BY event_timestamp DESC;
```

### Query 3: All Retry Attempts

```sql
-- Query by common envelope field
SELECT * FROM audit_events
WHERE (event_data->>'attempt')::integer > 1
  AND event_timestamp > NOW() - INTERVAL '24 hours'
ORDER BY event_timestamp DESC;
```

### Query 4: Service-Specific Field

```sql
-- Query by service-specific payload field
SELECT * FROM audit_events
WHERE event_data->>'service' = 'gateway'
  AND event_data->'payload'->>'alert_name' = 'HighCPU'
  AND event_timestamp > NOW() - INTERVAL '24 hours'
ORDER BY event_timestamp DESC;
```

### Query 5: Aggregation by Service

```sql
-- Aggregate by common envelope field
SELECT
    event_data->>'service' as service,
    event_data->>'operation' as operation,
    COUNT(*) as total_events,
    COUNT(*) FILTER (WHERE event_data->>'status' = 'success') as success_count,
    COUNT(*) FILTER (WHERE event_data->>'status' = 'failure') as failure_count,
    ROUND(100.0 * COUNT(*) FILTER (WHERE event_data->>'status' = 'success') / COUNT(*), 2) as success_rate
FROM audit_events
WHERE event_timestamp > NOW() - INTERVAL '7 days'
GROUP BY event_data->>'service', event_data->>'operation'
ORDER BY success_rate ASC;
```

---

## 🎯 Benefits of Hybrid Approach

### 1. Consistency Across Services
- ✅ All events have `version`, `service`, `operation`, `status`
- ✅ Tooling can rely on common fields
- ✅ Dashboards can aggregate across services

### 2. Service Flexibility
- ✅ Each service defines its own `payload` schema
- ✅ No constraints on service-specific fields
- ✅ Services can evolve independently

### 3. External Signal Preservation
- ✅ Original signal preserved in `source_payload`
- ✅ Debugging and compliance requirements met
- ✅ No data loss from transformation

### 4. Query Performance
- ✅ Common fields are queryable (indexed)
- ✅ Service-specific fields are queryable (GIN index)
- ✅ Aggregations are fast (common envelope)

### 5. Schema Evolution
- ✅ `version` field enables schema migration
- ✅ Services can add new fields without breaking queries
- ✅ Old events remain queryable

---

## 📋 Implementation Guidelines

### 1. Schema Documentation (Required)

**Location**: `docs/services/stateless/data-storage/schemas/event_data/`

```
event_data/
├── common_envelope.md          # Common envelope specification
├── gateway_payload.md          # Gateway service payload schema
├── context_api_payload.md      # Context API service payload schema
├── ai_analysis_payload.md      # AI Analysis service payload schema
├── workflow_payload.md         # Workflow service payload schema
├── data_storage_payload.md     # Data Storage service payload schema
└── execution_payload.md        # Execution service payload schema
```

### 2. JSON Schema Validation (Recommended)

```json
// common_envelope.schema.json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["version", "service", "operation", "status", "payload"],
  "properties": {
    "version": {
      "type": "string",
      "pattern": "^[0-9]+\\.[0-9]+$"
    },
    "service": {
      "type": "string",
      "enum": ["gateway", "context-api", "ai-analysis", "workflow", "data-storage", "execution"]
    },
    "operation": {
      "type": "string"
    },
    "status": {
      "type": "string",
      "enum": ["success", "failure", "partial", "pending"]
    },
    "attempt": {
      "type": "integer",
      "minimum": 1
    },
    "retry_reason": {
      "type": "string"
    },
    "payload": {
      "type": "object"
    },
    "source_payload": {
      "type": "object"
    }
  }
}
```

### 3. Helper Functions (Go)

```go
// pkg/datastorage/audit/event_data.go
package audit

import (
    "encoding/json"
    "fmt"
)

// CommonEnvelope represents the standardized wrapper for all event_data
type CommonEnvelope struct {
    Version       string                 `json:"version"`
    Service       string                 `json:"service"`
    Operation     string                 `json:"operation"`
    Status        string                 `json:"status"`
    Attempt       *int                   `json:"attempt,omitempty"`
    RetryReason   *string                `json:"retry_reason,omitempty"`
    Payload       map[string]interface{} `json:"payload"`
    SourcePayload map[string]interface{} `json:"source_payload,omitempty"`
}

// NewEventData creates a new event_data with common envelope
func NewEventData(service, operation, status string, payload map[string]interface{}) *CommonEnvelope {
    return &CommonEnvelope{
        Version:   "1.0",
        Service:   service,
        Operation: operation,
        Status:    status,
        Payload:   payload,
    }
}

// WithSourcePayload adds the original signal payload
func (e *CommonEnvelope) WithSourcePayload(sourcePayload map[string]interface{}) *CommonEnvelope {
    e.SourcePayload = sourcePayload
    return e
}

// WithRetry adds retry information
func (e *CommonEnvelope) WithRetry(attempt int, reason string) *CommonEnvelope {
    e.Attempt = &attempt
    e.RetryReason = &reason
    return e
}

// ToJSON converts to JSON for storage
func (e *CommonEnvelope) ToJSON() ([]byte, error) {
    return json.Marshal(e)
}

// FromJSON parses JSON from storage
func FromJSON(data []byte) (*CommonEnvelope, error) {
    var envelope CommonEnvelope
    if err := json.Unmarshal(data, &envelope); err != nil {
        return nil, fmt.Errorf("failed to unmarshal event data: %w", err)
    }
    return &envelope, nil
}
```

### 4. Usage Example

```go
// Gateway service
func (g *Gateway) auditSignalReceived(ctx context.Context, signal *Signal) error {
    // Create service-specific payload
    payload := map[string]interface{}{
        "signal_fingerprint": signal.Fingerprint,
        "alert_name":         signal.AlertName,
        "severity":           signal.Severity,
        "namespace":          signal.Namespace,
        "cluster":            signal.Cluster,
        "is_duplicate":       signal.IsDuplicate,
        "storm_detected":     signal.StormDetected,
        "action":             "created_crd",
        "crd_name":           signal.RemediationRequestName,
    }

    // Create event_data with common envelope
    eventData := audit.NewEventData("gateway", "signal_received", "success", payload)

    // Add original signal (optional)
    eventData.WithSourcePayload(signal.OriginalPayload)

    // Convert to JSON
    eventDataJSON, err := eventData.ToJSON()
    if err != nil {
        return err
    }

    // Store audit event
    event := &AuditEvent{
        EventType:     "gateway.signal.received",
        EventCategory: "signal",
        EventAction:   "received",
        EventOutcome:  "success",
        ActorType:     "service",
        ActorID:       "gateway",
        CorrelationID: signal.RemediationID,
        EventData:     eventDataJSON,  // JSONB
    }

    return g.auditStore.StoreAudit(ctx, event)
}
```

---

## 🎯 Final Recommendation

### **Hybrid Approach: Common Envelope + Service-Specific Payload**

**Confidence**: 95%

**Rationale**:
1. ✅ **Industry standard**: AWS, Google, Kubernetes all use this pattern
2. ✅ **Consistency**: Common fields across all services
3. ✅ **Flexibility**: Services define their own payload schemas
4. ✅ **Queryable**: Both common and service-specific fields
5. ✅ **Extensible**: New services and fields without breaking changes
6. ✅ **Debuggable**: Original signal preserved in `source_payload`

**Common Envelope Fields** (Required):
- `version`: Schema version (e.g., "1.0")
- `service`: Service name (e.g., "gateway", "ai-analysis")
- `operation`: Operation performed (e.g., "signal_received")
- `status`: Operation status (e.g., "success", "failure")
- `payload`: Service-specific data (flexible object)

**Optional Fields**:
- `attempt`: Retry attempt number
- `retry_reason`: Reason for retry
- `source_payload`: Original external signal (for debugging)

**Why 95% (not 100%)**:
- 5% uncertainty: Services may need additional common fields over time
  - **Mitigation**: `version` field enables schema evolution

---

## 📋 Next Steps

1. ✅ Approve hybrid approach (common envelope + service-specific payload)
2. 📝 Document per-service payload schemas
3. 🔧 Implement helper functions (`NewEventData`, `ToJSON`, etc.)
4. 🧪 Create JSON schema validation (optional but recommended)
5. 🚀 Begin implementation in Day 21

---

**Status**: ⏸️ Awaiting user approval to proceed with hybrid event_data format

