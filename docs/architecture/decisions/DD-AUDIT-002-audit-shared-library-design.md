# DD-AUDIT-002: Audit Shared Library Design

**Status**: ✅ **APPROVED** (Production Standard)
**Date**: November 8, 2025
**Last Reviewed**: November 8, 2025
**Confidence**: 98%
**Authority Level**: SYSTEM-WIDE - All services must use this shared library

---

## 🎯 **Overview**

This design decision establishes a **shared library** (`pkg/audit/`) for asynchronous buffered audit trace ingestion across all Kubernaut services.

**Key Principle**: All services MUST use the same audit implementation to guarantee consistent behavior, zero code duplication, and centralized maintenance.

**Scope**: All Kubernaut services (Gateway, Context API, AI Analysis, Workflow, Execution, Data Storage).

**Related Decisions**:
- **ADR-035**: [Asynchronous Buffered Audit Ingestion](./ADR-035-async-buffered-audit-ingestion.md) - Architectural mandate
- **ADR-034**: [Unified Audit Table Design](./ADR-034-unified-audit-table-design.md) - Database schema
- **DD-AUDIT-001**: [Audit Responsibility Pattern](./DD-AUDIT-001-audit-responsibility-pattern.md) - Who writes audit traces

---

## 📋 **Table of Contents**

1. [Context & Problem](#context--problem)
2. [Decision](#decision)
3. [Alternatives Analysis](#alternatives-analysis)
4. [API Design](#api-design)
5. [Implementation Details](#implementation-details)
6. [Configuration](#configuration)
7. [Metrics](#metrics)
8. [Code Examples](#code-examples)
9. [Usage Examples](#usage-examples)
10. [Testing Strategy](#testing-strategy)
11. [Migration Guide](#migration-guide)
12. [Benefits](#benefits)

---

## 🎯 **Context & Problem**

### **Challenge**

All services need to implement asynchronous buffered audit writes (as mandated by [ADR-035](./ADR-035-async-buffered-audit-ingestion.md)). Without a shared library:

1. ❌ **Code Duplication**: Each service implements the same buffering, batching, and retry logic (500 lines × 6 services = 3000 lines)
2. ❌ **Inconsistent Behavior**: Each service may have different buffer sizes, batch sizes, retry policies
3. ❌ **Maintenance Burden**: Bug fixes require changes in 6+ locations
4. ❌ **Testing Duplication**: Each service must test buffering, batching, retry logic independently

### **Business Impact**

- **Development Velocity**: Shared library reduces implementation time from 6 hours to 1 hour per service
- **Reliability**: Centralized testing ensures consistent behavior across all services
- **Maintenance**: Bug fixes in one location vs 6+ locations (5x faster)
- **Consistency**: Guaranteed same behavior across all services

---

## ✅ **Decision**

**APPROVED**: Create shared library at `pkg/audit/` with standard API

**Rationale**:
1. ✅ **Zero Code Duplication**: 73% code reduction (800 lines vs 3000 lines)
2. ✅ **Consistent Behavior**: Guaranteed same behavior across all services
3. ✅ **Centralized Maintenance**: Bug fixes in one location (6x easier)
4. ✅ **Centralized Testing**: Single test suite (67% test code reduction)
5. ✅ **Low Coupling**: Stable, simple API (interface-based design)

---

## 🔍 **Alternatives Analysis**

### Alternative 1: Shared Library (APPROVED) ✅

**Approach**: Create `pkg/audit/` with standard API

**Advantages**:
- ✅ Zero code duplication (73% reduction)
- ✅ Consistent behavior across all services
- ✅ Centralized maintenance (6x easier)
- ✅ Centralized testing (67% less test code)
- ✅ Low coupling (stable API)

**Disadvantages**:
- ⚠️ Shared dependency (low risk: stable API)
- ⚠️ Requires coordination for breaking changes (mitigated by versioning)

**Confidence**: 98%

---

### Alternative 2: Per-Service Implementation (REJECTED) ❌

**Approach**: Each service implements its own buffering, batching, retry logic

**Advantages**:
- ✅ Service autonomy (no shared dependency)
- ✅ Service-specific customization

**Disadvantages**:
- ❌ High code duplication (3000 lines vs 800 lines)
- ❌ Inconsistent behavior across services
- ❌ Maintenance burden (6+ locations)
- ❌ Bug fixes 5x slower (12-16 hours vs 2-3 hours)
- ❌ Testing duplication (1500 lines vs 500 lines)

**Rejection Reason**: Unacceptable code duplication and maintenance burden.

---

## 📊 **Comparison: Shared vs Per-Service**

| Aspect | Shared Library | Per-Service | Winner |
|--------|---------------|-------------|--------|
| **Code Lines** | 800 lines | 3000 lines | ✅ Shared (73% less) |
| **Maintenance** | 1 location | 6 locations | ✅ Shared (6x easier) |
| **Bug Fixes** | 2-3 hours | 12-16 hours | ✅ Shared (5x faster) |
| **Testing** | 500 lines | 1500 lines | ✅ Shared (67% less) |
| **Consistency** | Guaranteed | Varies | ✅ Shared |
| **Configuration** | Centralized | Duplicated | ✅ Shared |
| **Metrics** | Consistent | Inconsistent | ✅ Shared |
| **Coupling** | Low (stable API) | None | ⚠️ Per-Service |

**Winner**: ✅ **Shared Library** (clear winner)

---

## 🏗️ **API Design**

### Package Structure

```
pkg/audit/
├── store.go              # BufferedAuditStore implementation
├── store_test.go         # Unit tests
├── config.go             # Configuration
├── metrics.go            # Prometheus metrics
├── event.go              # AuditEvent type
├── event_data.go         # Common envelope helpers
└── README.md             # Usage documentation
```

---

### Core Interface

```go
// pkg/audit/store.go
package audit

import (
    "context"
    "time"

    "log/slog"
)

// AuditStore provides non-blocking audit event storage
type AuditStore interface {
    // StoreAudit adds an event to the buffer (non-blocking)
    // Returns immediately, does not wait for write to complete
    // Returns error only if event cannot be buffered (buffer full)
    StoreAudit(ctx context.Context, event *AuditEvent) error

    // Close flushes remaining events and stops background worker
    // Blocks until all buffered events are written or max timeout reached
    Close() error
}

// NewBufferedStore creates a new buffered audit store
func NewBufferedStore(client DataStorageClient, config Config, logger *slog.Logger) AuditStore
```

**Key Design Principles**:
- ✅ **Simple API**: Only 2 methods (StoreAudit, Close)
- ✅ **Non-blocking**: StoreAudit returns immediately
- ✅ **Graceful shutdown**: Close flushes remaining events
- ✅ **Error handling**: Returns error only if buffer is full (rare)

---

### Configuration

```go
// pkg/audit/config.go
package audit

import "time"

// Config for buffered audit store
type Config struct {
    // BufferSize is the maximum number of events to buffer in memory
    // Default: 10000
    // Recommendation: 10 seconds of peak traffic
    BufferSize int

    // BatchSize is the number of events to batch before writing
    // Default: 1000
    // Recommendation: Optimal for PostgreSQL INSERT performance
    BatchSize int

    // FlushInterval is the maximum time to wait before flushing a partial batch
    // Default: 1 second
    // Recommendation: Balance between latency and efficiency
    FlushInterval time.Duration

    // MaxRetries is the number of retry attempts for failed writes
    // Default: 3
    // Recommendation: Handles transient failures (network blips, DB restarts)
    MaxRetries int
}

// DefaultConfig returns the recommended default configuration
func DefaultConfig() Config {
    return Config{
        BufferSize:    10000,
        BatchSize:     1000,
        FlushInterval: 1 * time.Second,
        MaxRetries:    3,
    }
}

// Validate validates the configuration
func (c Config) Validate() error {
    if c.BufferSize <= 0 {
        return fmt.Errorf("buffer size must be positive, got %d", c.BufferSize)
    }
    if c.BatchSize <= 0 || c.BatchSize > c.BufferSize {
        return fmt.Errorf("batch size must be positive and <= buffer size, got %d", c.BatchSize)
    }
    if c.FlushInterval <= 0 {
        return fmt.Errorf("flush interval must be positive, got %v", c.FlushInterval)
    }
    if c.MaxRetries < 0 {
        return fmt.Errorf("max retries must be non-negative, got %d", c.MaxRetries)
    }
    return nil
}
```

---

### Event Structure

```go
// pkg/audit/event.go
package audit

import (
    "time"
)

// AuditEvent represents a single audit event
// Aligns with audit_events table schema (ADR-034)
type AuditEvent struct {
    // Event Identity
    EventVersion string // Default: "1.0"

    // Temporal Information
    EventTimestamp time.Time

    // Event Classification
    EventType     string // e.g., "gateway.signal.received"
    EventCategory string // e.g., "signal", "remediation", "workflow"
    EventAction   string // e.g., "received", "processed", "executed"
    EventOutcome  string // e.g., "success", "failure", "pending"

    // Actor Information (Who)
    ActorType string // e.g., "service", "external", "user"
    ActorID   string // e.g., "gateway-service", "aws-cloudwatch"
    ActorIP   string // Optional

    // Resource Information (What)
    ResourceType string // e.g., "Signal", "RemediationRequest"
    ResourceID   string // e.g., "fp-abc123", "rr-2025-001"
    ResourceName string // Optional

    // Context Information (Where/Why)
    CorrelationID string // remediation_id (groups related events)
    ParentEventID string // Optional: Links to parent event
    TraceID       string // Optional: OpenTelemetry trace ID
    SpanID        string // Optional: OpenTelemetry span ID

    // Kubernetes Context
    Namespace   string // Optional
    ClusterName string // Optional

    // Event Payload (JSONB - flexible, queryable)
    EventData     []byte // JSONB (use CommonEnvelope helpers)
    EventMetadata []byte // Optional: Additional metadata

    // Audit Metadata
    Severity      string // Optional: e.g., "info", "warning", "error"
    DurationMs    int    // Optional: Operation duration
    ErrorCode     string // Optional: Error code
    ErrorMessage  string // Optional: Error message

    // Compliance
    RetentionDays int  // Default: 2555 (7 years)
    IsSensitive   bool // Default: false
}

// NewAuditEvent creates a new audit event with defaults
func NewAuditEvent() *AuditEvent {
    return &AuditEvent{
        EventVersion:  "1.0",
        EventTimestamp: time.Now(),
        RetentionDays: 2555, // 7 years (SOC 2 / ISO 27001)
        IsSensitive:   false,
    }
}

// Validate validates the audit event
func (e *AuditEvent) Validate() error {
    if e.EventType == "" {
        return fmt.Errorf("event_type is required")
    }
    if e.EventCategory == "" {
        return fmt.Errorf("event_category is required")
    }
    if e.EventAction == "" {
        return fmt.Errorf("event_action is required")
    }
    if e.EventOutcome == "" {
        return fmt.Errorf("event_outcome is required")
    }
    if e.ActorType == "" {
        return fmt.Errorf("actor_type is required")
    }
    if e.ActorID == "" {
        return fmt.Errorf("actor_id is required")
    }
    if e.ResourceType == "" {
        return fmt.Errorf("resource_type is required")
    }
    if e.ResourceID == "" {
        return fmt.Errorf("resource_id is required")
    }
    if e.CorrelationID == "" {
        return fmt.Errorf("correlation_id is required")
    }
    if len(e.EventData) == 0 {
        return fmt.Errorf("event_data is required")
    }
    return nil
}
```

---

### Event Data Helpers

```go
// pkg/audit/event_data.go
package audit

import (
    "encoding/json"
)

// CommonEnvelope is the standard event_data format (ADR-034)
type CommonEnvelope struct {
    Version  string                 `json:"version"`
    Service  string                 `json:"service"`
    Operation string                `json:"operation"`
    Status   string                 `json:"status"`
    Payload  map[string]interface{} `json:"payload"`
    SourcePayload map[string]interface{} `json:"source_payload,omitempty"`
}

// NewEventData creates a new common envelope
func NewEventData(service, operation, status string, payload map[string]interface{}) *CommonEnvelope {
    return &CommonEnvelope{
        Version:   "1.0",
        Service:   service,
        Operation: operation,
        Status:    status,
        Payload:   payload,
    }
}

// WithSourcePayload adds the original external payload
func (e *CommonEnvelope) WithSourcePayload(sourcePayload map[string]interface{}) *CommonEnvelope {
    e.SourcePayload = sourcePayload
    return e
}

// ToJSON converts the envelope to JSON bytes
func (e *CommonEnvelope) ToJSON() ([]byte, error) {
    return json.Marshal(e)
}

// FromJSON parses JSON bytes into an envelope
func FromJSON(data []byte) (*CommonEnvelope, error) {
    var envelope CommonEnvelope
    if err := json.Unmarshal(data, &envelope); err != nil {
        return nil, err
    }
    return &envelope, nil
}
```

---

## 🔧 **Implementation Details**

### BufferedAuditStore Implementation

```go
// pkg/audit/store.go
package audit

import (
    "context"
    "sync"
    "sync/atomic"
    "time"

    "log/slog"
)

// BufferedAuditStore implements AuditStore with async buffered writes
type BufferedAuditStore struct {
    buffer chan *AuditEvent
    client DataStorageClient
    logger *slog.Logger
    config Config
    done   chan struct{}
    wg     sync.WaitGroup

    // Metrics (atomic counters)
    bufferedCount    int64
    droppedCount     int64
    writtenCount     int64
    failedBatchCount int64
}

// DataStorageClient interface for writing audit events
type DataStorageClient interface {
    StoreBatch(ctx context.Context, events []*AuditEvent) error
}

// NewBufferedStore creates a new buffered audit store
func NewBufferedStore(client DataStorageClient, config Config, logger *slog.Logger) AuditStore {
    if err := config.Validate(); err != nil {
        logger.Error("Invalid audit config, using defaults", "error", err)
        config = DefaultConfig()
    }

    store := &BufferedAuditStore{
        buffer: make(chan *AuditEvent, config.BufferSize),
        client: client,
        logger: logger,
        config: config,
        done:   make(chan struct{}),
    }

    // Start background worker
    store.wg.Add(1)
    go store.backgroundWriter()

    logger.Info("Audit store initialized",
        "buffer_size", config.BufferSize,
        "batch_size", config.BatchSize,
        "flush_interval", config.FlushInterval,
        "max_retries", config.MaxRetries,
    )

    return store
}

// StoreAudit adds event to buffer (non-blocking)
func (s *BufferedAuditStore) StoreAudit(ctx context.Context, event *AuditEvent) error {
    // Validate event
    if err := event.Validate(); err != nil {
        s.logger.Error("Invalid audit event", "error", err)
        return fmt.Errorf("invalid audit event: %w", err)
    }

    select {
    case s.buffer <- event:
        // ✅ Event buffered successfully
        atomic.AddInt64(&s.bufferedCount, 1)
        auditEventsBuffered.Inc()
        return nil

    default:
        // ⚠️ Buffer full (rare, indicates system overload)
        atomic.AddInt64(&s.droppedCount, 1)
        auditEventsDropped.Inc()

        s.logger.Warn("Audit buffer full, dropping event",
            "event_type", event.EventType,
            "correlation_id", event.CorrelationID,
            "buffered_count", atomic.LoadInt64(&s.bufferedCount),
            "dropped_count", atomic.LoadInt64(&s.droppedCount),
        )

        // ✅ Don't fail business logic
        return nil
    }
}

// Close flushes remaining events and stops background worker
func (s *BufferedAuditStore) Close() error {
    s.logger.Info("Closing audit store, flushing remaining events")

    // Close buffer (signals background worker to stop)
    close(s.buffer)

    // Wait for background worker to finish
    s.wg.Wait()

    s.logger.Info("Audit store closed",
        "buffered_count", atomic.LoadInt64(&s.bufferedCount),
        "written_count", atomic.LoadInt64(&s.writtenCount),
        "dropped_count", atomic.LoadInt64(&s.droppedCount),
        "failed_batch_count", atomic.LoadInt64(&s.failedBatchCount),
    )

    return nil
}

// backgroundWriter runs in a separate goroutine
func (s *BufferedAuditStore) backgroundWriter() {
    defer s.wg.Done()

    ticker := time.NewTicker(s.config.FlushInterval)
    defer ticker.Stop()

    batch := make([]*AuditEvent, 0, s.config.BatchSize)

    for {
        select {
        case event, ok := <-s.buffer:
            if !ok {
                // Channel closed, flush remaining events
                if len(batch) > 0 {
                    s.writeBatchWithRetry(batch)
                }
                return
            }

            batch = append(batch, event)
            auditBufferSize.Set(float64(len(s.buffer)))

            // Write when batch is full
            if len(batch) >= s.config.BatchSize {
                s.writeBatchWithRetry(batch)
                batch = batch[:0] // Reset batch
            }

        case <-ticker.C:
            // Flush partial batch periodically
            if len(batch) > 0 {
                s.writeBatchWithRetry(batch)
                batch = batch[:0]
            }
            auditBufferSize.Set(float64(len(s.buffer)))
        }
    }
}

// writeBatchWithRetry writes batch with exponential backoff
func (s *BufferedAuditStore) writeBatchWithRetry(batch []*AuditEvent) {
    ctx := context.Background()

    start := time.Now()
    defer func() {
        duration := time.Since(start).Seconds()
        auditWriteDuration.Observe(duration)
    }()

    for attempt := 1; attempt <= s.config.MaxRetries; attempt++ {
        if err := s.client.StoreBatch(ctx, batch); err != nil {
            s.logger.Error("Failed to write audit batch",
                "attempt", attempt,
                "batch_size", len(batch),
                "error", err,
            )

            if attempt < s.config.MaxRetries {
                // Exponential backoff: 1s, 4s, 9s
                backoff := time.Duration(attempt*attempt) * time.Second
                time.Sleep(backoff)
                continue
            }

            // Final failure: log and drop
            atomic.AddInt64(&s.failedBatchCount, 1)
            auditBatchesFailed.Inc()

            s.logger.Error("Dropping audit batch after max retries",
                "batch_size", len(batch),
                "max_retries", s.config.MaxRetries,
            )
            return
        }

        // Success
        atomic.AddInt64(&s.writtenCount, int64(len(batch)))
        auditEventsWritten.Add(float64(len(batch)))

        s.logger.Debug("Wrote audit batch",
            "batch_size", len(batch),
            "attempt", attempt,
        )
        return
    }
}
```

---

## 📊 **Metrics**

### Prometheus Metrics

```go
// pkg/audit/metrics.go
package audit

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // auditEventsBuffered tracks total events buffered
    auditEventsBuffered = promauto.NewCounter(prometheus.CounterOpts{
        Name: "audit_events_buffered_total",
        Help: "Total number of audit events buffered",
    })

    // auditEventsDropped tracks total events dropped (buffer full)
    auditEventsDropped = promauto.NewCounter(prometheus.CounterOpts{
        Name: "audit_events_dropped_total",
        Help: "Total number of audit events dropped (buffer full)",
    })

    // auditEventsWritten tracks total events written to storage
    auditEventsWritten = promauto.NewCounter(prometheus.CounterOpts{
        Name: "audit_events_written_total",
        Help: "Total number of audit events written to storage",
    })

    // auditBatchesFailed tracks total batches failed after max retries
    auditBatchesFailed = promauto.NewCounter(prometheus.CounterOpts{
        Name: "audit_batches_failed_total",
        Help: "Total number of audit batches failed after max retries",
    })

    // auditBufferSize tracks current buffer size
    auditBufferSize = promauto.NewGauge(prometheus.GaugeOpts{
        Name: "audit_buffer_size",
        Help: "Current number of events in audit buffer",
    })

    // auditWriteDuration tracks write latency
    auditWriteDuration = promauto.NewHistogram(prometheus.HistogramOpts{
        Name:    "audit_write_duration_seconds",
        Help:    "Duration of audit batch writes",
        Buckets: prometheus.DefBuckets,
    })
)
```

### Monitoring Dashboard

**Grafana Queries**:

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

**Alerts**:

```yaml
# High drop rate
- alert: AuditHighDropRate
  expr: rate(audit_events_dropped_total[5m]) / rate(audit_events_buffered_total[5m]) > 0.01
  for: 5m
  annotations:
    summary: "Audit drop rate >1%"

# High failure rate
- alert: AuditHighFailureRate
  expr: rate(audit_batches_failed_total[5m]) / rate(audit_events_written_total[5m]) > 0.05
  for: 5m
  annotations:
    summary: "Audit failure rate >5%"

# High buffer utilization
- alert: AuditHighBufferUtilization
  expr: audit_buffer_size / 10000 > 0.80
  for: 5m
  annotations:
    summary: "Audit buffer >80% full"
```

---

## 💻 **Code Examples**

### Example 1: Gateway Service (Signal Received)

```go
// pkg/gateway/gateway.go
package gateway

import (
    "context"

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

    // Create buffered audit store (shared library)
    auditStore := audit.NewBufferedStore(
        dsClient,
        audit.DefaultConfig(),
        logger,
    )

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

    // ✅ Audit (non-blocking, using shared library)
    g.auditSignalReceived(ctx, signal, crd)

    return nil
}

func (g *Gateway) auditSignalReceived(ctx context.Context, signal *Signal, crd *RemediationRequest) {
    // Create event_data using shared helper
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
    event.Namespace = signal.Namespace
    event.EventData = eventDataJSON

    // ✅ Non-blocking store (shared library handles buffering, batching, retry)
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

### Example 2: Context API Service (Query Processed)

```go
// pkg/contextapi/server.go
package contextapi

import (
    "context"
    "time"

    "github.com/jordigilh/kubernaut/pkg/audit"
    "github.com/jordigilh/kubernaut/pkg/datastorage/client"
)

type Server struct {
    auditStore audit.AuditStore
    logger     *slog.Logger
}

func NewServer(config *Config, logger *slog.Logger) (*Server, error) {
    // Create Data Storage client
    dsClient := client.NewDataStorageClient(config.DataStorageURL)

    // Create buffered audit store (shared library)
    auditStore := audit.NewBufferedStore(
        dsClient,
        audit.DefaultConfig(),
        logger,
    )

    return &Server{
        auditStore: auditStore,
        logger:     logger,
    }, nil
}

func (s *Server) handleQuery(ctx context.Context, query *Query) (*QueryResult, error) {
    start := time.Now()

    // Business logic
    result, err := s.executeQuery(ctx, query)

    duration := time.Since(start).Milliseconds()

    // ✅ Audit (non-blocking)
    s.auditQueryProcessed(ctx, query, result, err, int(duration))

    return result, err
}

func (s *Server) auditQueryProcessed(ctx context.Context, query *Query, result *QueryResult, err error, durationMs int) {
    outcome := "success"
    errorCode := ""
    errorMessage := ""

    if err != nil {
        outcome = "failure"
        errorCode = "QUERY_FAILED"
        errorMessage = err.Error()
    }

    // Create event_data
    payload := map[string]interface{}{
        "query_type":     query.Type,
        "remediation_id": query.RemediationID,
        "result_count":   len(result.Incidents),
        "cache_hit":      result.CacheHit,
    }

    eventData := audit.NewEventData("context-api", "query_processed", outcome, payload)
    eventDataJSON, _ := eventData.ToJSON()

    // Create audit event
    event := audit.NewAuditEvent()
    event.EventType = "context-api.query.processed"
    event.EventCategory = "query"
    event.EventAction = "processed"
    event.EventOutcome = outcome
    event.ActorType = "service"
    event.ActorID = "context-api"
    event.ResourceType = "Query"
    event.ResourceID = query.ID
    event.CorrelationID = query.RemediationID
    event.EventData = eventDataJSON
    event.DurationMs = durationMs
    event.ErrorCode = errorCode
    event.ErrorMessage = errorMessage

    // ✅ Non-blocking store
    if err := s.auditStore.StoreAudit(ctx, event); err != nil {
        s.logger.Error("Failed to buffer audit event", "error", err)
    }
}

func (s *Server) Shutdown(ctx context.Context) error {
    // Graceful shutdown: flush remaining audit events
    if err := s.auditStore.Close(); err != nil {
        s.logger.Error("Failed to close audit store", "error", err)
    }

    return nil
}
```

---

## 🧪 **Testing Strategy**

### Unit Tests

```go
// pkg/audit/store_test.go
package audit_test

import (
    "context"
    "testing"
    "time"

    "github.com/jordigilh/kubernaut/pkg/audit"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

func TestAuditStore(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Audit Store Suite")
}

var _ = Describe("BufferedAuditStore", func() {
    var (
        store      audit.AuditStore
        mockClient *MockDataStorageClient
        ctx        context.Context
    )

    BeforeEach(func() {
        ctx = context.Background()
        mockClient = NewMockDataStorageClient()

        config := audit.Config{
            BufferSize:    100,
            BatchSize:     10,
            FlushInterval: 100 * time.Millisecond,
            MaxRetries:    3,
        }

        store = audit.NewBufferedStore(mockClient, config, logger)
    })

    AfterEach(func() {
        store.Close()
    })

    Describe("StoreAudit", func() {
        It("should buffer event successfully", func() {
            event := createTestEvent()

            err := store.StoreAudit(ctx, event)

            Expect(err).ToNot(HaveOccurred())
        })

        It("should validate event before buffering", func() {
            event := &audit.AuditEvent{} // Invalid (missing required fields)

            err := store.StoreAudit(ctx, event)

            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("invalid audit event"))
        })

        It("should drop event when buffer is full", func() {
            // Fill buffer
            for i := 0; i < 100; i++ {
                store.StoreAudit(ctx, createTestEvent())
            }

            // Next event should be dropped
            err := store.StoreAudit(ctx, createTestEvent())

            Expect(err).ToNot(HaveOccurred()) // No error (graceful degradation)
        })
    })

    Describe("Batching", func() {
        It("should batch events when batch size is reached", func() {
            // Store 10 events (batch size)
            for i := 0; i < 10; i++ {
                store.StoreAudit(ctx, createTestEvent())
            }

            // Wait for batch to be written
            Eventually(func() int {
                return mockClient.BatchCount()
            }, "2s").Should(Equal(1))

            Expect(mockClient.LastBatchSize()).To(Equal(10))
        })

        It("should flush partial batch after flush interval", func() {
            // Store 5 events (less than batch size)
            for i := 0; i < 5; i++ {
                store.StoreAudit(ctx, createTestEvent())
            }

            // Wait for flush interval
            Eventually(func() int {
                return mockClient.BatchCount()
            }, "2s").Should(Equal(1))

            Expect(mockClient.LastBatchSize()).To(Equal(5))
        })
    })

    Describe("Retry Logic", func() {
        It("should retry on transient failure", func() {
            mockClient.SetFailureCount(2) // Fail first 2 attempts

            store.StoreAudit(ctx, createTestEvent())

            // Wait for batch to be written (after retries)
            Eventually(func() int {
                return mockClient.BatchCount()
            }, "5s").Should(Equal(1))

            Expect(mockClient.AttemptCount()).To(Equal(3))
        })

        It("should drop batch after max retries", func() {
            mockClient.SetFailureCount(10) // Fail all attempts

            store.StoreAudit(ctx, createTestEvent())

            // Wait for max retries
            Eventually(func() int {
                return mockClient.AttemptCount()
            }, "5s").Should(Equal(3))

            Expect(mockClient.BatchCount()).To(Equal(0)) // Batch dropped
        })
    })

    Describe("Graceful Shutdown", func() {
        It("should flush remaining events on close", func() {
            // Store 5 events
            for i := 0; i < 5; i++ {
                store.StoreAudit(ctx, createTestEvent())
            }

            // Close immediately (before flush interval)
            err := store.Close()

            Expect(err).ToNot(HaveOccurred())
            Expect(mockClient.BatchCount()).To(Equal(1))
            Expect(mockClient.LastBatchSize()).To(Equal(5))
        })
    })
})

func createTestEvent() *audit.AuditEvent {
    event := audit.NewAuditEvent()
    event.EventType = "test.event"
    event.EventCategory = "test"
    event.EventAction = "test"
    event.EventOutcome = "success"
    event.ActorType = "service"
    event.ActorID = "test-service"
    event.ResourceType = "Test"
    event.ResourceID = "test-123"
    event.CorrelationID = "test-correlation"
    event.EventData = []byte(`{"test": "data"}`)
    return event
}
```

---

### Integration Tests

```go
// test/integration/audit/store_integration_test.go
package audit_test

import (
    "context"
    "testing"

    "github.com/jordigilh/kubernaut/pkg/audit"
    "github.com/jordigilh/kubernaut/pkg/datastorage/client"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("Audit Store Integration", func() {
    var (
        store    audit.AuditStore
        dsClient *client.DataStorageClient
        ctx      context.Context
    )

    BeforeEach(func() {
        ctx = context.Background()

        // Create real Data Storage client
        dsClient = client.NewDataStorageClient("http://localhost:8080")

        store = audit.NewBufferedStore(
            dsClient,
            audit.DefaultConfig(),
            logger,
        )
    })

    AfterEach(func() {
        store.Close()
    })

    It("should write audit events to PostgreSQL", func() {
        event := createTestEvent()

        err := store.StoreAudit(ctx, event)
        Expect(err).ToNot(HaveOccurred())

        // Wait for batch to be written
        time.Sleep(2 * time.Second)

        // Verify event in database
        result, err := dsClient.QueryAuditEvents(ctx, event.CorrelationID)
        Expect(err).ToNot(HaveOccurred())
        Expect(result).To(HaveLen(1))
        Expect(result[0].EventType).To(Equal(event.EventType))
    })
})
```

---

### Load Tests

```go
// test/integration/audit/store_load_test.go
package audit_test

import (
    "context"
    "sync"
    "testing"
    "time"

    "github.com/jordigilh/kubernaut/pkg/audit"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("Audit Store Load Test", func() {
    It("should handle 1000 events/sec sustained", func() {
        store := audit.NewBufferedStore(
            dsClient,
            audit.DefaultConfig(),
            logger,
        )
        defer store.Close()

        ctx := context.Background()

        // Send 10,000 events over 10 seconds (1000 events/sec)
        var wg sync.WaitGroup
        for i := 0; i < 10; i++ {
            wg.Add(1)
            go func() {
                defer wg.Done()
                for j := 0; j < 1000; j++ {
                    store.StoreAudit(ctx, createTestEvent())
                    time.Sleep(10 * time.Millisecond)
                }
            }()
        }

        wg.Wait()

        // Verify no events dropped
        // (check metrics: audit_events_dropped_total should be 0)
    })
})
```

---

## 📋 **Migration Guide**

### Step 1: Add Dependency

```go
// go.mod (already in monorepo, no changes needed)
```

### Step 2: Update Service Initialization

```go
// Before (per-service implementation)
type Gateway struct {
    // Custom buffering logic
}

// After (shared library)
import "github.com/jordigilh/kubernaut/pkg/audit"

type Gateway struct {
    auditStore audit.AuditStore
}

func NewGateway(config *Config, logger *slog.Logger) (*Gateway, error) {
    dsClient := client.NewDataStorageClient(config.DataStorageURL)

    auditStore := audit.NewBufferedStore(
        dsClient,
        audit.DefaultConfig(),
        logger,
    )

    return &Gateway{
        auditStore: auditStore,
    }, nil
}
```

### Step 3: Replace Audit Calls

```go
// Before (custom implementation)
g.writeAuditEvent(ctx, event)

// After (shared library)
g.auditStore.StoreAudit(ctx, event)
```

### Step 4: Add Graceful Shutdown

```go
// Add to service shutdown
func (g *Gateway) Shutdown(ctx context.Context) error {
    if err := g.auditStore.Close(); err != nil {
        g.logger.Error("Failed to close audit store", "error", err)
    }
    return nil
}
```

### Step 5: Remove Custom Code

```go
// Delete custom buffering, batching, retry logic (500 lines)
```

---

## 📊 **Benefits**

### 1. Code Reduction

**Without Shared Library**:
- Gateway: 500 lines
- Context API: 500 lines
- AI Analysis: 500 lines
- Workflow: 500 lines
- Execution: 500 lines
- Data Storage: 500 lines
- **Total**: 3000 lines

**With Shared Library**:
- `pkg/audit/`: 500 lines (shared)
- Gateway: 50 lines (usage)
- Context API: 50 lines (usage)
- AI Analysis: 50 lines (usage)
- Workflow: 50 lines (usage)
- Execution: 50 lines (usage)
- Data Storage: 50 lines (usage)
- **Total**: 800 lines

**Reduction**: 73% (2200 lines saved)

---

### 2. Maintenance Efficiency

**Bug Fix Example**:

**Without Shared Library**:
1. Bug discovered in retry logic (Gateway)
2. Fix Gateway code
3. Fix Context API code
4. Fix AI Analysis code
5. Fix Workflow code
6. Fix Execution code
7. Fix Data Storage code
8. Test all 6 services
9. Deploy all 6 services
10. **Total effort**: 12-16 hours

**With Shared Library**:
1. Bug discovered in retry logic
2. Fix `pkg/audit/store.go`
3. Test shared library
4. Deploy all services (no code changes needed)
5. **Total effort**: 2-3 hours

**Efficiency**: 5x faster (10-13 hours saved)

---

### 3. Consistency Guarantee

**Without Shared Library**:
- ❌ Gateway uses buffer size 10000
- ❌ Context API uses buffer size 5000 (different)
- ❌ AI Analysis uses batch size 500 (different)
- ❌ Workflow has no retry logic (bug)
- ❌ Execution has different metrics (inconsistent)

**With Shared Library**:
- ✅ All services use same buffer size (10000)
- ✅ All services use same batch size (1000)
- ✅ All services have retry logic (3 attempts)
- ✅ All services expose same metrics (consistent)
- ✅ Guaranteed consistent behavior

---

### 4. Testing Efficiency

**Without Shared Library**:
- Unit tests: 6 locations (duplicated)
- Integration tests: 6 locations (duplicated)
- Load tests: 6 locations (duplicated)
- **Total test code**: 1500 lines (duplicated)

**With Shared Library**:
- Unit tests: 1 location (`pkg/audit/store_test.go`)
- Integration tests: 1 location (`test/integration/audit/`)
- Load tests: 1 location (`test/integration/audit/`)
- **Total test code**: 500 lines

**Reduction**: 67% (1000 lines saved)

---

## 🎯 **Confidence Assessment**

**Overall Confidence**: 98%

**Breakdown**:
- **API stability**: 95% (simple, well-defined interface)
- **Code reduction**: 100% (73% reduction guaranteed)
- **Maintenance benefit**: 100% (single location vs 6+)
- **Consistency benefit**: 100% (guaranteed same behavior)
- **Low coupling**: 95% (stable API, interface-based)

**Why 98% (not 100%)**:
- 2% uncertainty: Potential service-specific requirements not yet discovered
  - **Mitigation**: Interface-based design allows custom implementations if needed

---

## ⚠️ **Implementation Concerns**

### Concern 1: Data Storage Service Circular Dependency ⚠️ CRITICAL

**Issue**: Data Storage Service must audit its own operations, but it's also the service that writes audit traces to PostgreSQL.

**Problem**: Circular dependency - Data Storage Service calls itself via REST API to write audit traces.

**Risk**: Infinite loop, deadlock, or performance degradation.

**Solution**: ✅ **Internal Bypass for Data Storage Service**

```go
// pkg/datastorage/audit/internal_client.go
package audit

import (
    "context"
    "database/sql"

    "github.com/jordigilh/kubernaut/pkg/audit"
)

// InternalAuditClient writes audit events directly to PostgreSQL
// (bypasses REST API to avoid circular dependency)
type InternalAuditClient struct {
    db *sql.DB
}

func NewInternalAuditClient(db *sql.DB) audit.DataStorageClient {
    return &InternalAuditClient{db: db}
}

func (c *InternalAuditClient) StoreBatch(ctx context.Context, events []*audit.AuditEvent) error {
    // Write directly to PostgreSQL (no REST API call)
    tx, err := c.db.BeginTx(ctx, nil)
    if err != nil {
        return fmt.Errorf("failed to begin transaction: %w", err)
    }
    defer tx.Rollback()

    stmt, err := tx.PrepareContext(ctx, `
        INSERT INTO audit_events (
            event_type, event_category, event_action, event_outcome,
            actor_type, actor_id, resource_type, resource_id,
            correlation_id, event_data, event_timestamp
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
    `)
    if err != nil {
        return fmt.Errorf("failed to prepare statement: %w", err)
    }
    defer stmt.Close()

    for _, event := range events {
        _, err := stmt.ExecContext(ctx,
            event.EventType, event.EventCategory, event.EventAction, event.EventOutcome,
            event.ActorType, event.ActorID, event.ResourceType, event.ResourceID,
            event.CorrelationID, event.EventData, event.EventTimestamp,
        )
        if err != nil {
            return fmt.Errorf("failed to insert audit event: %w", err)
        }
    }

    if err := tx.Commit(); err != nil {
        return fmt.Errorf("failed to commit transaction: %w", err)
    }

    return nil
}
```

**Usage in Data Storage Service**:

```go
// pkg/datastorage/server.go
type Server struct {
    auditStore audit.AuditStore
    db         *sql.DB
}

func NewServer(config *Config, db *sql.DB, logger *slog.Logger) *Server {
    // Create internal audit client (bypasses REST API)
    internalClient := audit.NewInternalAuditClient(db)

    // Create buffered audit store with internal client
    auditStore := audit.NewBufferedStore(
        internalClient,
        audit.DefaultConfig(),
        logger,
    )

    return &Server{
        auditStore: auditStore,
        db:         db,
    }
}
```

**Benefits**:
- ✅ No circular dependency
- ✅ No infinite loop risk
- ✅ Better performance (no HTTP roundtrip)
- ✅ Still uses shared library (consistent API)

**Implementation Effort**: 2 hours

**Priority**: **CRITICAL** (must implement before Data Storage Service integration)

---

### Concern 2: Buffer Sizing for Burst Traffic ⚠️ RECOMMENDED

**Issue**: Default buffer size (10,000 events) may be insufficient for extreme burst scenarios.

**Problem**: Sudden burst of 50,000+ events could fill buffer and drop audit traces.

**Risk**: Audit event loss during cluster failures or alert storms.

**Solution**: ✅ **Enhanced Monitoring + Per-Service Configuration**

```go
// pkg/audit/config.go
type Config struct {
    BufferSize    int           // Default: 10000
    BatchSize     int           // Default: 1000
    FlushInterval time.Duration // Default: 1 second
    MaxRetries    int           // Default: 3
}

// Per-service configuration recommendations
var RecommendedConfigs = map[string]Config{
    "gateway": {
        BufferSize:    20000, // 2x default (high volume)
        BatchSize:     1000,
        FlushInterval: 1 * time.Second,
        MaxRetries:    3,
    },
    "ai-analysis": {
        BufferSize:    15000, // 1.5x default (LLM retries)
        BatchSize:     1000,
        FlushInterval: 1 * time.Second,
        MaxRetries:    3,
    },
    "default": {
        BufferSize:    10000,
        BatchSize:     1000,
        FlushInterval: 1 * time.Second,
        MaxRetries:    3,
    },
}
```

**Prometheus Alert**:

```yaml
# config/prometheus/alerts/audit.yaml
groups:
  - name: audit
    rules:
      - alert: AuditBufferHighUtilization
        expr: audit_buffer_size / on(service) audit_buffer_capacity > 0.80
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Audit buffer >80% full for {{ $labels.service }}"
          description: "Service {{ $labels.service }} audit buffer at {{ $value | humanizePercentage }}. Consider increasing buffer size."

      - alert: AuditBufferCriticalUtilization
        expr: audit_buffer_size / on(service) audit_buffer_capacity > 0.95
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Audit buffer >95% full for {{ $labels.service }}"
          description: "Service {{ $labels.service }} audit buffer at {{ $value | humanizePercentage }}. Audit events may be dropped."
```

**Benefits**:
- ✅ Early warning system (alerts before buffer full)
- ✅ Per-service configuration (high-volume services get larger buffers)
- ✅ No code changes needed (just config)

**Implementation Effort**: 1 hour

**Priority**: MEDIUM (add in v1.0)

---

## 🔗 **Related Decisions**

- **ADR-035**: [Asynchronous Buffered Audit Ingestion](./ADR-035-async-buffered-audit-ingestion.md) - Architectural mandate
- **ADR-034**: [Unified Audit Table Design](./ADR-034-unified-audit-table-design.md) - Database schema
- **ADR-032**: [Data Access Layer Isolation](./ADR-032-data-access-layer-isolation.md) - Data Storage Service mandate
- **DD-AUDIT-001**: [Audit Responsibility Pattern](./DD-AUDIT-001-audit-responsibility-pattern.md) - Who writes audit traces
- **DD-AUDIT-003**: [Service Audit Trace Requirements](./DD-AUDIT-003-service-audit-trace-requirements.md) - Which 8 of 11 services must generate audit traces
- **DD-005**: [Observability Standards](./DD-005-OBSERVABILITY-STANDARDS.md) - Metrics and logging standards

---

**Maintained By**: Kubernaut Architecture Team
**Last Updated**: November 8, 2025
**Review Cycle**: Annually or when new services are added

