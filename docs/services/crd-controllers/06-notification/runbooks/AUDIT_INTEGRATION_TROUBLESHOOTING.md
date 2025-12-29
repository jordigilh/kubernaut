# Audit Integration Troubleshooting Runbook

**Service**: Notification Controller
**Feature**: ADR-034 Unified Audit Table Integration
**Version**: 1.0
**Last Updated**: 2025-11-21

---

## 📋 Overview

This runbook provides troubleshooting procedures for the Notification Controller's ADR-034 unified audit table integration, including fire-and-forget writes, DLQ fallback, and audit correlation.

**Key Components**:
- Buffered Audit Store (`pkg/audit/`)
- Data Storage HTTP Client
- Redis DLQ (Dead Letter Queue)
- Audit Helpers (`internal/controller/notification/audit.go`)

---

## 🚨 Common Issues

### **Issue 1: Audit Events Not Appearing in audit_events Table**

**Symptoms**:
- Notifications delivered successfully
- No corresponding audit events in `audit_events` table
- No errors in controller logs

**Possible Causes**:
1. Buffered audit store not flushing
2. Data Storage Service unavailable
3. HTTP client configuration incorrect
4. Audit helpers not being called

**Diagnosis**:

```bash
# Check Prometheus metrics for audit store
kubectl exec -n kubernaut-system notification-controller-xxx -- \
  curl -s localhost:9090/metrics | grep audit

# Expected metrics:
# audit_events_buffered{service="notification"} 0
# audit_events_written_total{service="notification"} > 0
# audit_batch_write_duration_seconds_count > 0
```

**Resolution Steps**:

1. **Verify Data Storage Service is reachable**:
```bash
# From notification controller pod
kubectl exec -n kubernaut-system notification-controller-xxx -- \
  curl -v http://datastorage-service.kubernaut-system.svc.cluster.local:8080/health

# Should return HTTP 200
```

2. **Check audit store initialization**:
```bash
# Check controller logs for audit store init
kubectl logs -n kubernaut-system notification-controller-xxx | \
  grep "Audit store initialized"

# Expected output:
# {"level":"info","msg":"Audit store initialized","service":"notification","buffer_size":1000,"batch_size":10}
```

3. **Verify audit helper calls**:
```bash
# Check for audit helper invocations
kubectl logs -n kubernaut-system notification-controller-xxx | \
  grep -E "notification.message.(sent|failed|acknowledged|escalated)"
```

4. **Force flush pending events** (if needed):
```bash
# Restart controller to trigger graceful shutdown flush
kubectl rollout restart -n kubernaut-system deployment/notification-controller

# Check logs for flush
kubectl logs -n kubernaut-system notification-controller-xxx | \
  grep "Closing audit store"

# Expected: "buffered_count":N, "written_count":N, "dropped_count":0
```

---

### **Issue 2: High audit_events_dropped_total Count**

**Symptoms**:
- Prometheus metric `audit_events_dropped_total{service="notification"}` increasing
- Audit events missing from `audit_events` table
- Controller logs show "Dropping audit batch after max retries"

**Possible Causes**:
1. Data Storage Service repeatedly unavailable
2. Network issues between Notification and Data Storage
3. DLQ (Redis) also unavailable
4. Rate limiting or resource exhaustion

**Diagnosis**:

```bash
# Check drop metrics
kubectl exec -n kubernaut-system notification-controller-xxx -- \
  curl -s localhost:9090/metrics | grep audit_events_dropped

# Check failed batch count
kubectl exec -n kubernaut-system notification-controller-xxx -- \
  curl -s localhost:9090/metrics | grep audit_failed_batch_write_total

# Check controller logs for errors
kubectl logs -n kubernaut-system notification-controller-xxx --tail=100 | \
  grep -i "failed to write audit batch"
```

**Resolution Steps**:

1. **Check Data Storage Service health**:
```bash
# Check Data Storage pods
kubectl get pods -n kubernaut-system -l app=datastorage-service

# Check Data Storage logs
kubectl logs -n kubernaut-system datastorage-service-xxx --tail=50 | \
  grep -i error
```

2. **Verify network connectivity**:
```bash
# Test from Notification pod to Data Storage
kubectl exec -n kubernaut-system notification-controller-xxx -- \
  curl -X POST \
  -H "Content-Type: application/json" \
  -d '[{"event_version":"1.0","event_type":"test"}]' \
  http://datastorage-service.kubernaut-system.svc.cluster.local:8080/api/v1/audit/events
```

3. **Check Redis DLQ status**:
```bash
# Connect to Redis and check DLQ stream
kubectl exec -n kubernaut-system redis-xxx -- redis-cli

# In Redis CLI:
XLEN audit:dlq:notification
XINFO STREAM audit:dlq:notification

# Check for pending messages
XPENDING audit:dlq:notification notification-consumer-group
```

4. **Increase retry configuration** (if transient failures):
```yaml
# In deployment manifest
env:
  - name: AUDIT_MAX_RETRIES
    value: "5"  # Increase from default 3
  - name: AUDIT_FLUSH_INTERVAL
    value: "200ms"  # Increase from default 100ms
```

---

### **Issue 3: Audit Events with Incorrect correlation_id**

**Symptoms**:
- Audit events created successfully
- `correlation_id` is notification name instead of remediation request name
- Cannot trace full workflow across services

**Possible Causes**:
1. NotificationRequest CRD missing `metadata.remediationRequestName`
2. Audit helpers falling back to notification name

**Diagnosis**:

```bash
# Query audit events by notification name
psql -h datastorage-postgres -U audit -d kubernaut -c \
  "SELECT event_id, correlation_id, resource_id FROM audit_events WHERE resource_type='NotificationRequest' ORDER BY event_timestamp DESC LIMIT 10;"

# Check if correlation_id matches resource_id (indicating fallback)
```

**Resolution Steps**:

1. **Verify NotificationRequest CRD has remediationRequestName**:
```bash
# Get NotificationRequest CRD
kubectl get notificationrequest <notification-name> -n <namespace> -o yaml

# Expected:
# spec:
#   metadata:
#     remediationRequestName: "<remediation-id>"
```

2. **Update upstream services to include remediationRequestName**:
```yaml
# Example: Remediation Orchestrator creating NotificationRequest
apiVersion: kubernaut.ai/v1alpha1
kind: NotificationRequest
metadata:
  name: escalation-timeout-123
  namespace: kubernaut-system
spec:
  type: escalation
  priority: critical
  subject: "Remediation Timeout: Pod OOMKilled"
  body: "..."
  channels: [slack]
  recipients:
    - slack: "#sre-alerts"
  metadata:
    remediationRequestName: "remediation-prod-001"  # REQUIRED for correlation
    cluster: "prod-us-west"
```

3. **Verify audit helpers use correct correlation ID**:
```bash
# Check audit helper implementation
cat internal/controller/notification/audit.go | \
  grep -A 5 "correlationID :="

# Expected logic:
# correlationID := notification.Spec.Metadata["remediationRequestName"]
# if correlationID == "" {
#     correlationID = notification.Name  // fallback
# }
```

---

### **Issue 4: High audit_batch_write_duration_seconds**

**Symptoms**:
- Prometheus metric `audit_batch_write_duration_seconds` > 500ms
- Slow notification delivery (though should be non-blocking)
- Controller logs show slow HTTP POST to Data Storage

**Possible Causes**:
1. Data Storage Service overloaded
2. PostgreSQL slow queries
3. Network latency
4. Large batch sizes

**Diagnosis**:

```bash
# Check write duration percentiles
kubectl exec -n kubernaut-system notification-controller-xxx -- \
  curl -s localhost:9090/metrics | grep audit_batch_write_duration_seconds

# Check Data Storage Service performance
kubectl logs -n kubernaut-system datastorage-service-xxx | \
  grep -i "slow query"
```

**Resolution Steps**:

1. **Reduce batch size** (if large batches causing slowness):
```yaml
# In deployment manifest
env:
  - name: AUDIT_BATCH_SIZE
    value: "5"  # Reduce from default 10
```

2. **Increase flush interval** (to allow larger batches):
```yaml
env:
  - name: AUDIT_FLUSH_INTERVAL
    value: "200ms"  # Increase from default 100ms
```

3. **Scale Data Storage Service**:
```bash
# Increase Data Storage replicas
kubectl scale deployment datastorage-service \
  -n kubernaut-system --replicas=3
```

4. **Check PostgreSQL performance**:
```sql
-- Connect to PostgreSQL
psql -h datastorage-postgres -U audit -d kubernaut

-- Check slow queries
SELECT query, mean_exec_time, calls
FROM pg_stat_statements
WHERE query LIKE '%INSERT INTO audit_events%'
ORDER BY mean_exec_time DESC LIMIT 5;

-- Check index usage
\d audit_events
-- Ensure indexes on event_timestamp, correlation_id, resource_id
```

---

## 📊 Monitoring & Alerts

### **Key Prometheus Metrics**

```promql
# Audit events buffered (should be low, <100)
audit_events_buffered{service="notification"}

# Total audit events written (should increase steadily)
audit_events_written_total{service="notification"}

# Dropped audit events (should be 0)
audit_events_dropped_total{service="notification"}

# Failed batch writes (should be 0 or very low)
audit_failed_batch_write_total{service="notification"}

# Batch write duration (should be <100ms p95)
histogram_quantile(0.95, rate(audit_batch_write_duration_seconds_bucket{service="notification"}[5m]))
```

### **Recommended Alerts**

**Alert 1: High Audit Drop Rate**
```yaml
alert: NotificationAuditHighDropRate
expr: rate(audit_events_dropped_total{service="notification"}[5m]) > 0.1
for: 5m
labels:
  severity: critical
annotations:
  summary: "Notification audit events being dropped"
  description: "{{ $value }} audit events/sec dropped for notification service"
  runbook: "https://docs.company.com/runbooks/audit-integration"
```

**Alert 2: Audit Write Latency High**
```yaml
alert: NotificationAuditWriteSlow
expr: histogram_quantile(0.95, rate(audit_batch_write_duration_seconds_bucket{service="notification"}[5m])) > 0.5
for: 10m
labels:
  severity: warning
annotations:
  summary: "Notification audit writes are slow"
  description: "P95 audit write latency is {{ $value }}s (threshold: 0.5s)"
```

**Alert 3: Audit Buffer Full**
```yaml
alert: NotificationAuditBufferFull
expr: audit_events_buffered{service="notification"} > 900
for: 2m
labels:
  severity: warning
annotations:
  summary: "Notification audit buffer near capacity"
  description: "Audit buffer at {{ $value }}/1000 events"
```

---

## 🔍 Advanced Debugging

### **Enable Debug Logging**

```yaml
# In deployment manifest
env:
  - name: LOG_LEVEL
    value: "debug"  # Enables detailed audit store logging
```

**Debug Log Patterns**:
```bash
# Watch audit store operations in real-time
kubectl logs -n kubernaut-system notification-controller-xxx -f | \
  grep -E "Wrote audit batch|Failed to write|Dropping audit batch"
```

### **Query Audit Events Directly**

```sql
-- Connect to PostgreSQL
psql -h datastorage-postgres -U audit -d kubernaut

-- Find recent notification audit events
SELECT
    event_id,
    event_timestamp,
    event_type,
    event_outcome,
    resource_id,
    correlation_id,
    event_data->>'channel' as channel
FROM audit_events
WHERE
    event_category = 'notification'
    AND event_timestamp > NOW() - INTERVAL '1 hour'
ORDER BY event_timestamp DESC
LIMIT 20;

-- Check for missing correlation
SELECT
    COUNT(*) as total_events,
    COUNT(DISTINCT correlation_id) as unique_correlations,
    SUM(CASE WHEN correlation_id = resource_id THEN 1 ELSE 0 END) as fallback_count
FROM audit_events
WHERE event_category = 'notification';
```

### **Test Audit Store Directly**

```go
// Test audit store from Go
package main

import (
    "context"
    "time"
    "github.com/jordigilh/kubernaut/pkg/audit"
)

func testAuditStore() {
    client := audit.NewHTTPDataStorageClient(
        "http://datastorage-service.kubernaut-system.svc.cluster.local:8080",
        httpClient,
    )

    store, _ := audit.NewBufferedStore(
        client,
        audit.Config{BufferSize: 100, BatchSize: 5, FlushInterval: 100 * time.Millisecond},
        "test",
        logger,
    )

    event := &audit.AuditEvent{
        EventVersion:   "1.0",
        EventTimestamp: time.Now(),
        EventType:      "test.event",
        EventCategory:  "test",
        EventAction:    "test",
        EventOutcome:   "success",
        // ... other fields
    }

    err := store.StoreAudit(context.Background(), event)
    if err != nil {
        log.Fatal(err)
    }

    time.Sleep(200 * time.Millisecond)  // Wait for flush
    store.Close()  // Force final flush
}
```

---

## 🛠️ Configuration Reference

### **Environment Variables**

| Variable | Default | Description |
|----------|---------|-------------|
| `AUDIT_BUFFER_SIZE` | 1000 | Max events buffered before forcing flush |
| `AUDIT_BATCH_SIZE` | 10 | Events per batch write to Data Storage |
| `AUDIT_FLUSH_INTERVAL` | 100ms | Max time between flushes |
| `AUDIT_MAX_RETRIES` | 3 | Max retry attempts before DLQ |
| `DATA_STORAGE_URL` | `http://datastorage-service...` | Data Storage Service endpoint |

### **Tuning Guidelines**

| Scenario | Buffer Size | Batch Size | Flush Interval |
|----------|-------------|------------|----------------|
| **Low Volume** (<100 events/min) | 500 | 5 | 200ms |
| **Medium Volume** (100-1000 events/min) | 1000 | 10 | 100ms |
| **High Volume** (>1000 events/min) | 2000 | 20 | 50ms |

---

## ✅ Health Check

Run this checklist to verify audit integration health:

```bash
#!/bin/bash
# Audit Integration Health Check

echo "=== Audit Integration Health Check ==="

# 1. Check audit store metrics
echo "1. Checking audit metrics..."
kubectl exec -n kubernaut-system notification-controller-0 -- \
  curl -s localhost:9090/metrics | grep -E "audit_events_(buffered|written|dropped)"

# 2. Check Data Storage connectivity
echo "2. Testing Data Storage connectivity..."
kubectl exec -n kubernaut-system notification-controller-0 -- \
  curl -s -o /dev/null -w "%{http_code}" \
  http://datastorage-service.kubernaut-system.svc.cluster.local:8080/health

# 3. Check recent audit events
echo "3. Checking recent audit events in database..."
kubectl exec -n kubernaut-system datastorage-postgres-0 -- \
  psql -U audit -d kubernaut -c \
  "SELECT COUNT(*) FROM audit_events WHERE event_category='notification' AND event_timestamp > NOW() - INTERVAL '5 minutes';"

# 4. Check for dropped events
echo "4. Checking for dropped audit events..."
DROPPED=$(kubectl exec -n kubernaut-system notification-controller-0 -- \
  curl -s localhost:9090/metrics | grep "audit_events_dropped_total" | awk '{print $2}')

if [ "$DROPPED" == "0" ]; then
    echo "✅ No dropped audit events"
else
    echo "❌ WARNING: $DROPPED audit events dropped"
fi

echo "=== Health Check Complete ==="
```

---

## 📞 Support

**Escalation Path**:
1. Check this runbook
2. Review [DD-NOT-001 Implementation Plan](../DD-NOT-001-ADR034-AUDIT-INTEGRATION-v2.0-FULL.md)
3. Check [ADR-034](../../../../architecture/decisions/ADR-034-unified-audit-table-design.md)
4. Contact SRE team (#sre-alerts on Slack)

**Related Documentation**:
- [ADR-034: Unified Audit Table Design](../../../../architecture/decisions/ADR-034-unified-audit-table-design.md)
- [ADR-038: Async Buffered Audit Ingestion](../../../../architecture/decisions/ADR-038-async-buffered-audit-ingestion.md)
- [DD-009: Audit Write Error Recovery](../../../../architecture/decisions/DD-009-audit-write-error-recovery.md)
- [Data Storage Service Runbook](../../../stateless/data-storage/runbooks/AUDIT_SERVICE_TROUBLESHOOTING.md)

---

**Version**: 1.0
**Last Updated**: 2025-11-21
**Maintainer**: Notification Service Team


