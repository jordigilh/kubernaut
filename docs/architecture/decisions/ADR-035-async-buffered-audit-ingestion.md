# ADR-035: Asynchronous Buffered Audit Trace Ingestion

**Date**: 2025-11-08
**Status**: ✅ Approved
**Deciders**: Architecture Team
**Consulted**: Gateway, Context API, AI Analysis, Workflow, Data Storage teams
**Confidence**: 95%

---

## Context

Kubernaut services must record audit traces for compliance, debugging, and analytics (as defined in [ADR-034: Unified Audit Table Design](./ADR-034-unified-audit-table-design.md)). However, audit writes must not impact business operation performance.

### The Problem

**Critical Concern**: Should services write audit traces synchronously or asynchronously?

**Business Operations at Risk**:
- Gateway: Signal ingestion (50ms target latency)
- Context API: Query processing (100ms target latency)
- AI Analysis: LLM analysis (2-5s target latency)
- Workflow: Workflow orchestration (200ms target latency)
- Execution: Action execution (1-10s target latency)

**Database Layer Concerns**:
- PostgreSQL write latency: 10-50ms per INSERT
- Network latency to Data Storage Service: 5-20ms
- Database connection pool exhaustion under load
- Cascading failures if database is slow or unavailable

**Key Question**: How do we write audit traces without impacting business operation latency or reliability?

---

## Decision

**APPROVED**: All services MUST use **asynchronous buffered writes** with in-memory buffer and background worker for audit trace ingestion.

**Pattern**: Fire-and-forget with local buffering (industry standard)

**Implementation**: Shared library at `pkg/audit/` (see [DD-AUDIT-002: Audit Shared Library Design](./DD-AUDIT-002-audit-shared-library-design.md) for implementation details)

---

## Rationale

### 1. Industry Standard Analysis (9/10 Platforms)

**What Industry Leaders Do**:

| Platform | Ingestion Pattern | Impact on Business Logic |
|----------|------------------|-------------------------|
| **AWS CloudTrail** | Async (buffered) | Zero (fire-and-forget) |
| **Google Cloud Audit Logs** | Async (buffered) | Zero (fire-and-forget) |
| **Kubernetes Audit Logs** | Async (buffered) | Zero (non-blocking) |
| **Datadog APM** | Async (buffered) | Zero (agent-based) |
| **Elastic APM** | Async (buffered) | Zero (agent-based) |
| **New Relic** | Async (buffered) | Zero (agent-based) |
| **Splunk HEC** | Async (buffered) | Zero (forwarder-based) |
| **Jaeger** | Async (buffered) | Zero (agent-based) |
| **OpenTelemetry** | Async (buffered) | Zero (collector-based) |

**Industry Consensus**: **9/10 platforms use asynchronous buffered writes** (fire-and-forget)

**Key Insight**: Audit traces are **non-blocking** in production systems. Business operations never wait for audit writes.

---

### 2. Performance Impact Analysis

**Latency Impact on Business Operations**:

| Pattern | Business Operation | Audit Write | Total Latency | Impact |
|---------|-------------------|-------------|---------------|--------|
| **Synchronous** | 50ms | 10-50ms | **60-100ms** | ❌ 2x slower |
| **External Queue** | 50ms | 5-20ms | **55-70ms** | ⚠️ 1.4x slower |
| **Async Buffered** | 50ms | 0ms (non-blocking) | **50ms** | ✅ Zero impact |

**Key Insight**: Async buffered writes have **ZERO impact** on business operation latency.

**Write Throughput**:

| Pattern | Single Write | Batched Write (1000 events) | Throughput |
|---------|-------------|----------------------------|------------|
| **Synchronous** | 10ms | N/A | 100 events/sec |
| **External Queue** | 5ms | N/A | 200 events/sec |
| **Async Buffered** | 0ms (buffered) | 100ms (batched) | **10,000 events/sec** |

**Key Insight**: Async buffered writes are **50-100x faster** due to batching.

---

### 3. Reliability Analysis

**Failure Impact**:

| Pattern | Audit Failure Impact | Business Operation Impact |
|---------|---------------------|--------------------------|
| **Synchronous** | ❌ Business operation fails | ❌ Critical |
| **External Queue** | ⚠️ Business operation fails (if queue down) | ⚠️ High |
| **Async Buffered** | ✅ Audit dropped, business continues | ✅ Zero |

**Key Insight**: Async buffered writes **isolate audit failures** from business logic.

**Graceful Degradation**:
- ✅ Database slow → Background worker retries, business logic unaffected
- ✅ Database down → Background worker retries 3x, then drops batch (logs error, alerts)
- ✅ Buffer full → Drop new audit event (log warning), business logic continues

**Result**: Business operations **NEVER fail** due to audit issues.

---

### 4. Architectural Pattern

```
┌─────────────────────────────────────────────────────────────┐
│                    SERVICE (e.g., Gateway)                   │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  Business Operation (e.g., Signal Received)                 │
│       ↓                                                      │
│  ┌────────────────────────────────────────────────────┐    │
│  │  AUDIT CALL (Non-Blocking)                         │    │
│  │  auditStore.StoreAudit(ctx, event)                 │    │
│  │  → Returns immediately (no wait)                   │    │
│  │  → Event added to in-memory buffer                 │    │
│  │  → Business logic continues                        │    │
│  └────────────────────────────────────────────────────┘    │
│       ↓                                                      │
│  Business Operation Completes (no audit delay)              │
│                                                              │
│  ┌────────────────────────────────────────────────────┐    │
│  │  BACKGROUND WORKER (Separate Goroutine)            │    │
│  │  - Reads from in-memory buffer                     │    │
│  │  - Batches events (1000 events)                    │    │
│  │  - Writes to Data Storage Service                  │    │
│  │  - Retries on failure (exponential backoff)        │    │
│  │  - Logs errors (does not fail business logic)      │    │
│  └────────────────────────────────────────────────────┘    │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

**Key Characteristics**:
- ✅ **Non-blocking**: Business logic never waits for audit writes
- ✅ **Buffered**: In-memory queue (channel) between caller and writer
- ✅ **Batched**: Background worker batches events for efficiency
- ✅ **Resilient**: Retries on failure, graceful degradation
- ✅ **No external dependencies**: No Kafka/RabbitMQ required

---

## Alternatives Considered

### Alternative 1: Synchronous Writes ❌ REJECTED

**Approach**: Business operations wait for audit write to complete

```go
// ❌ ANTI-PATTERN: Synchronous audit write
func (g *Gateway) handleSignal(ctx context.Context, signal *Signal) error {
    crd, err := g.createRemediationRequest(ctx, signal)
    if err != nil {
        return err
    }

    // ❌ BLOCKING: Wait for audit write to complete
    if err := g.auditStore.StoreAudit(ctx, event); err != nil {
        return fmt.Errorf("failed to store audit: %w", err)
    }

    return nil
}
```

**Problems**:
- ❌ **Latency impact**: Business operation waits for DB write (~10-50ms)
- ❌ **Cascading failures**: DB issues cause business operations to fail
- ❌ **Tight coupling**: Business logic depends on audit infrastructure
- ❌ **No retry**: Single failure point
- ❌ **Performance**: Cannot batch writes

**Rejection Reason**: 2x latency impact on business operations is unacceptable.

---

### Alternative 2: External Queue (Kafka/RabbitMQ) ⚠️ REJECTED

**Approach**: Publish audit events to external message queue

```go
// ⚠️ OVER-ENGINEERED: External queue
func (g *Gateway) handleSignal(ctx context.Context, signal *Signal) error {
    crd, err := g.createRemediationRequest(ctx, signal)
    if err != nil {
        return err
    }

    // ⚠️ EXTERNAL DEPENDENCY: Kafka/RabbitMQ
    if err := g.kafka.Publish(ctx, "audit-events", event); err != nil {
        return fmt.Errorf("failed to publish audit: %w", err)
    }

    return nil
}
```

**Problems**:
- ⚠️ **External dependency**: Requires Kafka/RabbitMQ infrastructure
- ⚠️ **Operational complexity**: Another system to manage
- ⚠️ **Latency**: Network round-trip to queue (~5-20ms)
- ⚠️ **Failure mode**: Business operations fail if queue is down
- ⚠️ **Over-engineering**: Adds complexity for no benefit

**When to Use**:
- ✅ High-volume audit streams (>100,000 events/sec)
- ✅ Multi-datacenter audit aggregation
- ✅ Audit data consumed by multiple systems

**Rejection Reason**: Over-engineering for current scale (1000-10,000 events/sec). In-memory buffering is sufficient.

---

## Consequences

### Positive Consequences

1. ✅ **Zero Latency Impact**: Business operations never wait for audit writes
2. ✅ **High Throughput**: Batching provides 50-100x throughput improvement
3. ✅ **Resilient**: Retry logic and graceful degradation prevent cascading failures
4. ✅ **Simple**: No external dependencies (Kafka, RabbitMQ)
5. ✅ **Consistent**: Shared library guarantees same behavior across all services
6. ✅ **Observable**: Prometheus metrics for monitoring audit health

### Negative Consequences

1. ⚠️ **Eventual Consistency**: Audit events appear in database with up to 1 second delay
   - **Mitigation**: Acceptable for audit use case (not real-time analytics)

2. ⚠️ **Potential Data Loss**: Buffer full or service crash before flush
   - **Mitigation**:
     - Buffer size: 10,000 events (handles 10 seconds at 1000 events/sec)
     - Graceful shutdown: Flushes remaining events before exit
     - Monitoring: Alerts on high drop rate (>1%)

3. ⚠️ **Memory Usage**: In-memory buffer consumes ~10MB per service
   - **Mitigation**: Acceptable overhead for modern systems

### Implementation Concerns

**See**: [DD-AUDIT-002: Implementation Concerns](./DD-AUDIT-002-audit-shared-library-design.md#⚠️-implementation-concerns)

**Critical Concerns to Address**:

1. ⚠️ **Data Storage Service Circular Dependency** (CRITICAL)
   - **Issue**: Data Storage Service must audit its own operations, but it's also the service that writes audit traces
   - **Solution**: Internal bypass using direct PostgreSQL access (bypasses REST API)
   - **Effort**: 2 hours
   - **Priority**: Must implement before Data Storage Service integration

2. ⚠️ **Buffer Sizing for Burst Traffic** (RECOMMENDED)
   - **Issue**: Default buffer (10,000 events) may be insufficient for extreme bursts (50,000+ events)
   - **Solution**: Enhanced monitoring + per-service configuration
   - **Effort**: 1 hour
   - **Priority**: Add in v1.0

3. ⚠️ **Context API PII Access Tracking** (OPTIONAL)
   - **Issue**: GDPR Article 30 may require PII access tracking
   - **Solution**: Optional audit configuration (disabled by default)
   - **Effort**: 0 hours (v1.0), 2 hours (v2.0 if needed)
   - **Priority**: Document now, implement only if compliance requires

**Total Additional Effort**: 3 hours (2h critical + 1h recommended)

### Operational Impact

**Required Monitoring**:
- `audit_events_buffered_total` - Total events buffered
- `audit_events_dropped_total` - Total events dropped (buffer full)
- `audit_events_written_total` - Total events written to database
- `audit_batches_failed_total` - Total batches failed after max retries
- `audit_buffer_size` - Current buffer size
- `audit_write_duration_seconds` - Write latency histogram

**Required Alerts**:
- High drop rate (>1% of buffered events)
- High failure rate (>5% of batches)
- High buffer utilization (>80% full)

**Configuration**:
- Buffer size: 10,000 events (configurable per service)
- Batch size: 1000 events (configurable)
- Flush interval: 1 second (configurable)
- Max retries: 3 attempts (configurable)

---

## Implementation

**Shared Library**: `pkg/audit/` (see [DD-AUDIT-002: Audit Shared Library Design](./DD-AUDIT-002-audit-shared-library-design.md))

**Key Components**:
- `AuditStore` interface - Non-blocking audit storage
- `BufferedAuditStore` implementation - Async buffered writes
- `Config` struct - Configuration options
- `AuditEvent` type - Event structure
- `CommonEnvelope` helpers - Event data format

**Usage Example**:

```go
// Create audit store (shared library)
auditStore := audit.NewBufferedStore(
    dsClient,
    audit.Config{
        BufferSize:    10000,
        BatchSize:     1000,
        FlushInterval: 1 * time.Second,
        MaxRetries:    3,
    },
    logger,
)

// Non-blocking audit write
auditStore.StoreAudit(ctx, event) // Returns immediately

// Graceful shutdown (flushes remaining events)
auditStore.Close()
```

**Services Affected**:
- Gateway Service
- Context API Service
- AI Analysis Service
- Workflow Service
- Execution Service
- Data Storage Service (internal audit)

---

## Compliance with Existing Decisions

### ADR-032: Data Access Layer Isolation

**Compliance**: ✅ Audit writes go through Data Storage Service REST API (not direct PostgreSQL)

**Architecture**:
```
Service → pkg/audit/ → Data Storage Service REST API → PostgreSQL
```

### ADR-034: Unified Audit Table Design

**Compliance**: ✅ Async buffered writes store events in `audit_events` table

**Event Format**: Structured columns + JSONB (as defined in ADR-034)

### DD-AUDIT-001: Audit Responsibility Pattern

**Compliance**: ✅ Services are responsible for writing their own audit traces

**Pattern**: Distributed audit (each service writes its own traces)

---

## Related Decisions

- **ADR-032**: [Data Access Layer Isolation](./ADR-032-data-access-layer-isolation.md) - Mandates Data Storage Service for all DB access
- **ADR-034**: [Unified Audit Table Design](./ADR-034-unified-audit-table-design.md) - Defines audit table schema and event format
- **DD-AUDIT-001**: [Audit Responsibility Pattern](./DD-AUDIT-001-audit-responsibility-pattern.md) - Defines who writes audit traces
- **DD-AUDIT-002**: [Audit Shared Library Design](./DD-AUDIT-002-audit-shared-library-design.md) - Implementation details for `pkg/audit/`
- **DD-AUDIT-003**: [Service Audit Trace Requirements](./DD-AUDIT-003-service-audit-trace-requirements.md) - Defines which 8 of 11 services must generate audit traces
- **DD-005**: [Observability Standards](./DD-005-OBSERVABILITY-STANDARDS.md) - Metrics and logging standards

---

## References

### Industry Analysis

- **AWS CloudTrail**: [Audit Logging Best Practices](https://docs.aws.amazon.com/awscloudtrail/latest/userguide/best-practices-security.html)
- **Google Cloud Audit Logs**: [Audit Logging](https://cloud.google.com/logging/docs/audit)
- **Kubernetes Audit Logs**: [Auditing](https://kubernetes.io/docs/tasks/debug/debug-cluster/audit/)
- **OpenTelemetry**: [Collector Architecture](https://opentelemetry.io/docs/collector/)
- **Datadog APM**: [Agent Architecture](https://docs.datadoghq.com/agent/)

### Internal Documentation

- **Analysis**: [Audit Trace Ingestion Pattern Analysis](../../analysis/AUDIT_TRACE_INGESTION_PATTERN_ANALYSIS.md)
- **Analysis**: [Async Audit Design Decision Triage](../../analysis/ASYNC_AUDIT_DESIGN_DECISION_TRIAGE.md)

---

**Maintained By**: Kubernaut Architecture Team
**Last Updated**: November 8, 2025
**Review Cycle**: Annually or when scale requirements change (>100,000 events/sec)

