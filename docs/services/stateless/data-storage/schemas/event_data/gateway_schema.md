# Gateway Service - Event Data Schema

**Version**: 1.0
**Service**: gateway
**Purpose**: Signal ingestion, deduplication, storm detection, environment classification

---

## Schema Structure

```json
{
  "version": "1.0",
  "service": "gateway",
  "event_type": "signal.received|signal.deduplicated|signal.storm_detected",
  "timestamp": "2025-11-18T10:00:00Z",
  "data": {
    "gateway": {
      "signal_type": "prometheus|kubernetes",
      "alert_name": "HighMemoryUsage",
      "event_reason": "OOMKilled",
      "fingerprint": "sha256:abc123...",
      "namespace": "production",
      "resource_type": "pod",
      "resource_name": "api-server-xyz-123",
      "severity": "critical|warning|info",
      "priority": "P0|P1|P2|P3",
      "environment": "production|staging|development",
      "deduplication_status": "new|duplicate|storm",
      "storm_detected": false,
      "storm_id": "storm-2025-11-18-001",
      "labels": {
        "app": "api-server",
        "tier": "backend"
      },
      "source_payload": "base64encodeddata=="
    }
  }
}
```

---

## Field Definitions

### Gateway-Specific Fields (`data.gateway`)

| Field | Type | Required | Description | Example |
|-------|------|----------|-------------|---------|
| `signal_type` | string | Yes | Signal source | `"prometheus"`, `"kubernetes"` |
| `alert_name` | string | No | Prometheus alert name | `"HighMemoryUsage"` |
| `event_reason` | string | No | K8s event reason | `"OOMKilled"`, `"FailedScheduling"` |
| `fingerprint` | string | No | Deduplication fingerprint | `"sha256:abc123..."` |
| `namespace` | string | No | K8s namespace | `"production"` |
| `resource_type` | string | No | Resource kind | `"pod"`, `"node"`, `"deployment"` |
| `resource_name` | string | No | Resource identifier | `"api-server-xyz-123"` |
| `severity` | string | No | Signal severity | `"critical"`, `"warning"`, `"info"` |
| `priority` | string | No | Assigned priority | `"P0"`, `"P1"`, `"P2"`, `"P3"` |
| `environment` | string | No | Environment classification | `"production"`, `"staging"`, `"development"` |
| `deduplication_status` | string | No | Deduplication result | `"new"`, `"duplicate"`, `"storm"` |
| `storm_detected` | boolean | Yes | Storm flag | `true`, `false` |
| `storm_id` | string | No | Storm identifier | `"storm-2025-11-18-001"` |
| `labels` | object | No | Additional metadata | `{"app": "api-server"}` |
| `source_payload` | string | No | Base64 original payload | `"eyJhbGVy..."` |

---

## Event Types

### `signal.received`
Signal ingested from external source (Prometheus/K8s)

**Example**:
```json
{
  "version": "1.0",
  "service": "gateway",
  "event_type": "signal.received",
  "timestamp": "2025-11-18T10:00:00Z",
  "data": {
    "gateway": {
      "signal_type": "prometheus",
      "alert_name": "PodOOMKilled",
      "fingerprint": "sha256:abc123",
      "namespace": "production",
      "resource_type": "pod",
      "resource_name": "api-server-xyz-123",
      "severity": "critical",
      "priority": "P0",
      "environment": "production",
      "deduplication_status": "new",
      "storm_detected": false
    }
  }
}
```

### `signal.deduplicated`
Signal identified as duplicate

**Example**:
```json
{
  "data": {
    "gateway": {
      "signal_type": "prometheus",
      "fingerprint": "sha256:same-as-before",
      "deduplication_status": "duplicate"
    }
  }
}
```

### `signal.storm_detected`
Storm detected for related signals

**Example**:
```json
{
  "data": {
    "gateway": {
      "signal_type": "prometheus",
      "storm_detected": true,
      "storm_id": "storm-2025-11-18-001",
      "deduplication_status": "storm"
    }
  }
}
```

---

## Go Builder Usage

```go
import "github.com/jordigilh/kubernaut/pkg/datastorage/audit"

// Prometheus signal
eventData, err := audit.NewGatewayEvent("signal.received").
    WithSignalType("prometheus").
    WithAlertName("HighMemoryUsage").
    WithFingerprint("sha256:abc123").
    WithNamespace("production").
    WithResource("pod", "api-server-123").
    WithSeverity("critical").
    WithPriority("P0").
    WithEnvironment("production").
    WithDeduplicationStatus("new").
    WithLabels(map[string]string{
        "app": "api-server",
        "tier": "backend",
    }).
    Build()

// Kubernetes Event
eventData, err := audit.NewGatewayEvent("signal.received").
    WithSignalType("kubernetes").
    WithEventReason("OOMKilled").
    WithFingerprint("sha256:def456").
    WithNamespace("production").
    WithResource("pod", "database-pod-123").
    WithSeverity("critical").
    Build()

// Storm detection
eventData, err := audit.NewGatewayEvent("signal.storm_detected").
    WithSignalType("prometheus").
    WithAlertName("PodCrashLoop").
    WithStorm("storm-2025-11-18-001").
    Build()
```

---

## Query Examples

### Find all Prometheus signals

```sql
SELECT
    event_id,
    event_timestamp,
    event_data->'data'->'gateway'->>'alert_name' AS alert_name,
    event_data->'data'->'gateway'->>'namespace' AS namespace,
    event_data->'data'->'gateway'->>'severity' AS severity
FROM audit_events
WHERE event_data->>'service' = 'gateway'
AND event_data->'data'->'gateway'->>'signal_type' = 'prometheus'
ORDER BY event_timestamp DESC
LIMIT 100;
```

### Find storms in production

```sql
SELECT
    event_data->'data'->'gateway'->>'storm_id' AS storm_id,
    COUNT(*) AS signal_count,
    MIN(event_timestamp) AS storm_start,
    MAX(event_timestamp) AS storm_end
FROM audit_events
WHERE event_data->>'service' = 'gateway'
AND event_data->'data'->'gateway'->>'storm_detected' = 'true'
AND event_data->'data'->'gateway'->>'environment' = 'production'
GROUP BY storm_id
ORDER BY storm_start DESC;
```

### Find signals by correlation_id

```sql
SELECT
    event_id,
    event_timestamp,
    event_type,
    event_data->'data'->'gateway'->>'deduplication_status' AS status
FROM audit_events
WHERE correlation_id = 'rr-2025-001'
AND event_data->>'service' = 'gateway'
ORDER BY event_timestamp;
```

---

## Business Requirements Mapping

| BR ID | Requirement | Fields Used |
|-------|-------------|-------------|
| BR-STORAGE-033-004 | Gateway event structure | All gateway fields |
| BR-STORAGE-033-005 | Prometheus/K8s support | `signal_type`, `alert_name`, `event_reason` |
| BR-STORAGE-033-006 | Deduplication/storm tracking | `deduplication_status`, `storm_detected`, `storm_id` |

---

## Related Documentation

- **[Gateway Service Overview](../../../gateway-service/overview.md)**
- **[Gateway Implementation](../../../gateway-service/implementation.md)**
- **[ADR-034: Unified Audit Table](../../../../architecture/decisions/ADR-034-unified-audit-table-design.md)**

