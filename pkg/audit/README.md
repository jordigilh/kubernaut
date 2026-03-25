# Audit Shared Library

**Package**: `github.com/jordigilh/kubernaut/pkg/audit`

**Authority**: DD-AUDIT-002 (Audit Shared Library Design)

**Related**: ADR-034 (Unified Audit Table Design), ADR-038 (Asynchronous Buffered Audit Ingestion)

---

## ⚠️ **IMPORTANT: DD-API-001 Compliance (2025-12-18)**

**All services MUST use the OpenAPI-based DataStorage client adapter.**

**HTTPDataStorageClient has been DELETED** - use `OpenAPIClientAdapter` instead.

**Required Usage**:
```go
// ✅ CORRECT: DD-API-001 compliant (type-safe, contract-validated)
dsClient, err := audit.NewOpenAPIClientAdapter(url, 5*time.Second)
if err != nil {
    return fmt.Errorf("failed to create Data Storage client: %w", err)
}
```

**See**: [DD-API-001 Documentation](../../docs/architecture/decisions/DD-API-001-openapi-client-mandatory-v1.md)

---

## 📋 **Overview**

This package provides a **shared library** for asynchronous buffered audit trace ingestion across all Kubernaut services.

**All services MUST use this library** to ensure:
- ✅ **Consistent behavior** across all services
- ✅ **Non-blocking business logic** (audit failures don't block operations)
- ✅ **Efficient batching and retry logic**
- ✅ **Graceful degradation** (drop events if buffer full, don't crash)
- ✅ **Zero code duplication** (73% code reduction vs per-service implementation)

---

## 🎯 **Key Features**

### **1. Non-Blocking Audit Writes**
- Audit events are buffered in memory and written asynchronously
- Business logic never blocks waiting for audit writes
- If audit write fails, business logic continues (graceful degradation)

### **2. Automatic Batching**
- Events are batched for efficient writes (default: 1000 events per batch)
- Partial batches are flushed periodically (default: 1 second)
- Reduces database load and improves throughput

### **3. Retry with Exponential Backoff**
- Failed writes are retried automatically (default: 3 attempts)
- Exponential backoff: 1s, 4s, 9s delays
- Handles transient failures (network blips, database restarts)

### **4. Graceful Degradation**
- If buffer is full, events are dropped (not queued indefinitely)
- Dropped events are logged and counted in metrics
- Service continues operating normally

### **5. Observability**
- Prometheus metrics for buffer utilization, drop rate, write latency
- Structured logging for all audit operations
- Alerts for high drop rates and buffer utilization

---

## 🚀 **Quick Start**

### **Step 1: Import the Package**

```go
import (
    "github.com/jordigilh/kubernaut/pkg/audit"
)
```

### **Step 2: Create an Audit Store**

```go
// ✅ DD-API-001: Create OpenAPI client adapter for Data Storage
// This provides type-safe API calls validated against OpenAPI spec
//
// Benefits:
// - Type safety from OpenAPI spec
// - Compile-time contract validation
// - Breaking changes caught during development
// - Same interface as deprecated HTTPDataStorageClient (drop-in replacement)
dsClient, err := audit.NewOpenAPIClientAdapter("http://datastorage-service:8080", 5*time.Second)
if err != nil {
    return fmt.Errorf("failed to create DataStorage client: %w", err)
}

// Create buffered audit store (same as before)
auditStore, err := audit.NewBufferedStore(
    dsClient,
    audit.DefaultConfig(),
    "gateway", // service name for metrics
    logger,
)
if err != nil {
    return fmt.Errorf("failed to create audit store: %w", err)
}
```

### **Step 3: Store Audit Events**

```go
// Create event_data using common envelope
payload := map[string]interface{}{
    "signal_fingerprint": "fp-abc123",
    "alert_name":         "PodOOMKilled",
    "namespace":          "production",
}

eventData := audit.NewEventData("gateway", "signal_received", "success", payload)
eventDataJSON, _ := eventData.ToJSON()

// Create audit event
event := audit.NewAuditEvent()
event.EventType = "gateway.signal.received"
event.EventCategory = "signal"
event.EventAction = "received"
event.EventOutcome = "success"
event.ActorType = "service"
event.ActorID = "gateway"
event.ResourceType = "Signal"
event.ResourceID = "fp-abc123"
event.CorrelationID = "remediation-123"
event.EventData = eventDataJSON

// Store event (non-blocking)
if err := auditStore.StoreAudit(ctx, event); err != nil {
    logger.Error("Failed to buffer audit event", "error", err)
}
```

### **Step 4: Graceful Shutdown**

```go
// Flush remaining events during shutdown
if err := auditStore.Close(); err != nil {
    logger.Error("Failed to close audit store", "error", err)
}
```

---

## 📊 **Configuration**

### **Default Configuration**

```go
config := audit.DefaultConfig()
// BufferSize: 10000
// BatchSize: 1000
// FlushInterval: 1 second
// MaxRetries: 3
```

### **Service-Specific Configuration**

```go
// Gateway (high volume)
config := audit.RecommendedConfig("gateway")
// BufferSize: 20000 (2x default)

// AI Analysis (LLM retries)
config := audit.RecommendedConfig("ai-analysis")
// BufferSize: 15000 (1.5x default)

// Other services
config := audit.RecommendedConfig("default")
// BufferSize: 10000
```

### **Custom Configuration**

```go
config := audit.Config{
    BufferSize:    20000,
    BatchSize:     1000,
    FlushInterval: 1 * time.Second,
    MaxRetries:    3,
}
```

---

## 📈 **Metrics**

### **Prometheus Metrics Exposed**

| Metric | Type | Description |
|--------|------|-------------|
| `audit_events_buffered_total{service}` | Counter | Total events buffered |
| `audit_events_dropped_total{service}` | Counter | Total events dropped (buffer full) |
| `audit_events_written_total{service}` | Counter | Total events written to storage |
| `audit_batches_failed_total{service}` | Counter | Total batches failed after max retries |
| `audit_buffer_size{service}` | Gauge | Current buffer size |
| `audit_write_duration_seconds{service}` | Histogram | Write latency |

### **Key Queries**

```promql
# Drop rate (should be <1%)
rate(audit_events_dropped_total[5m]) / rate(audit_events_buffered_total[5m]) * 100

# Failure rate (should be <5%)
rate(audit_batches_failed_total[5m]) / rate(audit_events_written_total[5m]) * 100

# Buffer utilization (should be <80%)
audit_buffer_size / 10000 * 100

# Write throughput
rate(audit_events_written_total[5m])

# Write latency (p50, p95, p99)
histogram_quantile(0.50, rate(audit_write_duration_seconds_bucket[5m]))
histogram_quantile(0.95, rate(audit_write_duration_seconds_bucket[5m]))
histogram_quantile(0.99, rate(audit_write_duration_seconds_bucket[5m]))
```

### **Recommended Alerts**

```yaml
# High drop rate
- alert: AuditHighDropRate
  expr: rate(audit_events_dropped_total[5m]) / rate(audit_events_buffered_total[5m]) > 0.01
  for: 5m
  annotations:
    summary: "Audit drop rate >1% for {{ $labels.service }}"

# High failure rate
- alert: AuditHighFailureRate
  expr: rate(audit_batches_failed_total[5m]) / rate(audit_events_written_total[5m]) > 0.05
  for: 5m
  annotations:
    summary: "Audit failure rate >5% for {{ $labels.service }}"

# High buffer utilization
- alert: AuditHighBufferUtilization
  expr: audit_buffer_size / 10000 > 0.80
  for: 5m
  annotations:
    summary: "Audit buffer >80% full for {{ $labels.service }}"
```

---

## 📖 **API Reference**

### **AuditStore Interface**

```go
type AuditStore interface {
    // StoreAudit adds an event to the buffer (non-blocking)
    StoreAudit(ctx context.Context, event *AuditEvent) error

    // Close flushes remaining events and stops background worker
    Close() error
}
```

### **AuditEvent Struct**

See [event.go](./event.go) for full field documentation.

**Required Fields**:
- `EventType`: e.g., "gateway.signal.received"
- `EventCategory`: e.g., "signal"
- `EventAction`: e.g., "received"
- `EventOutcome`: e.g., "success"
- `ActorType`: e.g., "service"
- `ActorID`: e.g., "gateway"
- `ResourceType`: e.g., "Signal"
- `ResourceID`: e.g., "fp-abc123"
- `CorrelationID`: e.g., "remediation-123"
- `EventData`: JSON bytes (use `CommonEnvelope`)

**Auto-Generated Fields**:
- `EventID`: UUID (auto-generated)
- `EventVersion`: "1.0" (default)
- `EventTimestamp`: time.Now().UTC() (default, MUST be UTC for DataStorage validation)
- `RetentionDays`: 2555 (7 years, default)
- `IsSensitive`: false (default)

### **CommonEnvelope Struct**

```go
type CommonEnvelope struct {
    Version       string                 // "1.0"
    Service       string                 // "gateway"
    Operation     string                 // "signal_received"
    Status        string                 // "success"
    Payload       map[string]interface{} // Service-specific data
    SourcePayload map[string]interface{} // Optional: original external payload
}
```

**Helper Functions**:
- `NewEventData(service, operation, status, payload)`: Create envelope
- `WithSourcePayload(sourcePayload)`: Add original payload
- `ToJSON()`: Convert to JSON bytes for `EventData` field
- `FromJSON(data)`: Parse JSON bytes back to envelope

---

## 💻 **Complete Example**

### **Gateway Service Integration**

```go
package gateway

import (
    "context"
    "log/slog"

    "github.com/jordigilh/kubernaut/pkg/audit"
    "github.com/jordigilh/kubernaut/pkg/datastorage/client"
)

type Gateway struct {
    auditStore audit.AuditStore
    logger     *slog.Logger
}

func NewGateway(config *Config, logger *slog.Logger) (*Gateway, error) {
    // Create Data Storage client
    dsClient := client.NewDataStorageClient(config.DataStorageURL)

    // Create buffered audit store
    auditStore, err := audit.NewBufferedStore(
        dsClient,
        audit.RecommendedConfig("gateway"),
        "gateway",
        logger,
    )
    if err != nil {
        return nil, fmt.Errorf("failed to create audit store: %w", err)
    }

    return &Gateway{
        auditStore: auditStore,
        logger:     logger,
    }, nil
}

func (g *Gateway) handleSignal(ctx context.Context, signal *Signal) error {
    // Business logic
    crd, err := g.createRemediationRequest(ctx, signal)
    if err != nil {
        return err
    }

    // Audit (non-blocking)
    g.auditSignalReceived(ctx, signal, crd)

    return nil
}

func (g *Gateway) auditSignalReceived(ctx context.Context, signal *Signal, crd *RemediationRequest) {
    // Create event_data
    payload := map[string]interface{}{
        "signal_fingerprint": signal.Fingerprint,
        "alert_name":         signal.AlertName,
        "namespace":          signal.Namespace,
        "is_duplicate":       signal.IsDuplicate,
        "action":             "created_crd",
        "crd_name":           crd.Name,
    }

    eventData := audit.NewEventData("gateway", "signal_received", "success", payload)
    eventData.WithSourcePayload(signal.OriginalPayload)

    eventDataJSON, _ := eventData.ToJSON()

    // Create audit event
    event := audit.NewAuditEvent()
    event.EventType = "gateway.signal.received"
    event.EventCategory = "signal"
    event.EventAction = "received"
    event.EventOutcome = "success"
    event.ActorType = "service"
    event.ActorID = "gateway"
    event.ResourceType = "Signal"
    event.ResourceID = signal.Fingerprint
    event.CorrelationID = signal.RemediationID
    event.Namespace = &signal.Namespace
    event.EventData = eventDataJSON

    // Store event (non-blocking)
    if err := g.auditStore.StoreAudit(ctx, event); err != nil {
        g.logger.Error("Failed to buffer audit event", "error", err)
    }
}

func (g *Gateway) Shutdown(ctx context.Context) error {
    // Graceful shutdown: flush remaining audit events
    if err := g.auditStore.Close(); err != nil {
        g.logger.Error("Failed to close audit store", "error", err)
    }

    return nil
}
```

---

## 🧪 **Testing**

### **Unit Tests**

See [store_test.go](./store_test.go) for comprehensive unit tests.

**Test Coverage**: 70%+ (target)

**Key Test Scenarios**:
- Event buffering (success and validation failure)
- Buffer full (graceful degradation)
- Batching (full batch and partial batch flush)
- Retry logic (transient failures and max retries)
- Graceful shutdown (flush remaining events)

### **Integration Tests**

Integration tests should verify:
- Real Data Storage Service integration
- Event persistence in PostgreSQL
- Query API retrieval
- DLQ fallback (if database unavailable)

---

## 🔗 **Related Documentation**

- **DD-AUDIT-002**: [Audit Shared Library Design](../../docs/architecture/decisions/DD-AUDIT-002-audit-shared-library-design.md)
- **ADR-034**: [Unified Audit Table Design](../../docs/architecture/decisions/ADR-034-unified-audit-table-design.md)
- **ADR-038**: [Asynchronous Buffered Audit Ingestion](../../docs/architecture/decisions/ADR-038-async-buffered-audit-ingestion.md)
- **DD-AUDIT-001**: [Audit Responsibility Pattern](../../docs/architecture/decisions/DD-AUDIT-001-audit-responsibility-pattern.md)
- **DD-AUDIT-003**: [Service Audit Trace Requirements](../../docs/architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md)

---

## ❓ **FAQ**

### **Q: What happens if the buffer is full?**
**A**: Events are dropped (graceful degradation). The service continues operating normally. Monitor `audit_events_dropped_total` metric to detect buffer overruns.

### **Q: What happens if audit writes fail?**
**A**: The library retries with exponential backoff (default: 3 attempts). After max retries, the batch is dropped and logged. Business logic is never blocked.

### **Q: How do I tune buffer size for my service?**
**A**: Use `audit.RecommendedConfig(serviceName)` for service-specific defaults. Monitor `audit_buffer_size` metric and increase if utilization >80%.

### **Q: Can I use this library for non-audit events?**
**A**: No. This library is specifically designed for audit events that must be written to the unified `audit_events` table. Use other mechanisms for application logs or metrics.

### **Q: How do I test my audit integration?**
**A**: Use a mock `DataStorageClient` in unit tests. For integration tests, use a real Data Storage Service instance.

---

**Maintained By**: Kubernaut Platform Team
**Last Updated**: 2025-12-18 (HTTPDataStorageClient deleted, DD-API-001 compliance complete)
**Version**: 2.0


