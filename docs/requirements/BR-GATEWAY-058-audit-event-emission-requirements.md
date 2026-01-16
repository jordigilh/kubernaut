# BR-GATEWAY-058: Audit Event Emission Requirements for Gateway Service

**Status**: ✅ APPROVED (Enhanced in BR-GATEWAY-058-A)
**Version**: 1.1.0 (Enhanced Correlation ID Pattern)
**Date**: 2026-01-16
**Owner**: Gateway Service Team
**Stakeholders**: Security Team, Compliance Team, SRE Team

---

## Executive Summary

Gateway service must emit comprehensive audit events for all signal processing operations to meet SOC2 compliance requirements. This includes successful CRD creation and failed CRD creation events with full context for debugging and compliance auditing.

**Business Value**: Provides tamper-evident audit trail for all Gateway operations, supporting SOC2 CC6.8 (Non-Repudiation) and CC7.2 (Monitoring Activities) compliance.

---

## Business Need

### Problem Statement

Gateway is the entry point for all alerts in the Kubernaut platform. Every signal processed, whether successful or failed, must be audited for:
- **Compliance**: SOC2 requires non-repudiation of all operations
- **Debugging**: SRE teams need to trace alert flow from ingestion to remediation
- **Analytics**: Product team needs to measure signal volume, success rates, and failure patterns
- **Security**: Security team needs to detect potential attack patterns or abuse

### Current State

Gateway emits basic audit events but lacks:
- Human-readable correlation IDs for failed CRD creation
- Comprehensive error details for failure classification
- Circuit breaker state tracking in audit events

### Desired State

Gateway emits structured audit events with:
- **Human-readable correlation IDs** for operator efficiency
- **Comprehensive error details** including circuit breaker state
- **Pattern-matching support** for SRE queries
- **Consistent event structure** across all processing outcomes

---

## Business Objectives

### Primary Objectives

1. **Compliance**: Meet SOC2 audit trail requirements for all Gateway operations
2. **Observability**: Enable SRE teams to trace and debug signal processing
3. **Reliability**: Track error patterns to identify systemic issues
4. **Security**: Detect anomalous behavior through audit log analysis

### Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| **Audit Event Coverage** | 100% | All ProcessSignal calls emit audit events |
| **Correlation ID Readability** | >90% SRE satisfaction | SRE team survey |
| **Pattern Matching Success Rate** | >95% | Query success for alert-based filters |
| **Audit Event Latency** | <10ms | P95 latency for audit emission |

---

## Functional Requirements

### FR-1: Successful CRD Creation Audit Event

**Requirement**: Emit `gateway.crd.created` audit event when RemediationRequest CRD is successfully created.

**Event Structure**:
```json
{
  "event_id": "550e8400-e29b-41d4-a716-446655440000",
  "event_type": "gateway.crd.created",
  "event_category": "gateway",
  "event_action": "created",
  "event_outcome": "success",
  "actor_type": "gateway",
  "actor_id": "crd-creator",
  "resource_type": "RemediationRequest",
  "resource_id": "rr-prod-123-1234567890",
  "correlation_id": "rr-prod-123-1234567890",
  "namespace": "prod-payment-service",
  "event_data": {
    "event_type": "gateway.crd.created",
    "signal_type": "prometheus",
    "alert_name": "HighMemoryUsage",
    "namespace": "prod-payment-service",
    "fingerprint": "a1b2c3d4...",
    "severity": "critical",
    "resource_kind": "Pod",
    "resource_name": "payment-api-789",
    "remediation_request_name": "rr-prod-123-1234567890"
  }
}
```

**Implementation**: `pkg/gateway/server.go:emitCRDCreatedAudit()`

---

### FR-2: Failed CRD Creation Audit Event (BR-GATEWAY-058-A: Enhanced)

**Requirement**: Emit `gateway.crd.failed` audit event when RemediationRequest CRD creation fails.

**Enhancement (BR-GATEWAY-058-A)**: Use human-readable correlation ID instead of SHA256 fingerprint for better operator experience.

**Correlation ID Format**:
```
alertname:namespace:kind:name
```

**Examples**:
- `HighMemoryUsage:prod-payment-service:Pod:payment-api-789`
- `NodeNotReady:default:Node:worker-node-1`
- `DeploymentReplicasUnavailable:prod-api:Deployment:api-server`

**Event Structure**:
```json
{
  "event_id": "550e8400-e29b-41d4-a716-446655440001",
  "event_type": "gateway.crd.failed",
  "event_category": "gateway",
  "event_action": "created",
  "event_outcome": "failure",
  "actor_type": "gateway",
  "actor_id": "crd-creator",
  "resource_type": "RemediationRequest",
  "resource_id": "HighMemoryUsage:prod-payment-service:Pod:payment-api-789",
  "correlation_id": "HighMemoryUsage:prod-payment-service:Pod:payment-api-789",
  "namespace": "prod-payment-service",
  "event_data": {
    "event_type": "gateway.crd.failed",
    "signal_type": "prometheus",
    "alert_name": "HighMemoryUsage",
    "namespace": "prod-payment-service",
    "fingerprint": "a1b2c3d4...",
    "severity": "critical",
    "resource_kind": "Pod",
    "resource_name": "payment-api-789",
    "error_details": {
      "component": "gateway",
      "code": "ERR_K8S_API_UNAVAILABLE",
      "message": "Failed to create RemediationRequest: API server unavailable",
      "retry_possible": true
    }
  }
}
```

**Rationale for Enhanced Correlation ID (BR-GATEWAY-058-A)**:

| Aspect | SHA256 Hash (Old) | Readable String (New) |
|--------|-------------------|------------------------|
| **Human Readable** | ❌ No | ✅ Yes |
| **SRE Debugging** | ❌ Requires lookup | ✅ Immediate context |
| **Pattern Matching** | ❌ Cannot filter by alert | ✅ Easy wildcards |
| **Industry Standard** | ✅ Distributed tracing | ✅ OpenTelemetry semantic conventions |
| **Uniqueness** | ✅ Guaranteed | ✅ Alert context provides uniqueness |

**Query Examples**:
```sql
-- Find all failures for specific alert
SELECT * FROM audit_events
WHERE correlation_id LIKE 'HighMemoryUsage:%'
  AND event_type = 'gateway.crd.failed';

-- Find all failures in namespace
SELECT * FROM audit_events
WHERE correlation_id LIKE '%:prod-payment-service:%'
  AND event_type = 'gateway.crd.failed';

-- Find failures for specific resource
SELECT * FROM audit_events
WHERE correlation_id LIKE '%:Pod:payment-api-789'
  AND event_type = 'gateway.crd.failed';
```

**Implementation**: `pkg/gateway/server.go:emitCRDCreationFailedAudit()`, `constructReadableCorrelationID()`

---

### FR-3: Error Details in Failed Events

**Requirement**: Include comprehensive error details for all failed CRD creation events.

**Error Details Structure**:
```json
{
  "component": "gateway",
  "code": "ERR_K8S_API_UNAVAILABLE",
  "message": "Failed to create RemediationRequest: API server unavailable",
  "retry_possible": true
}
```

**Error Codes**:

| Code | Description | Retry Possible |
|------|-------------|----------------|
| `ERR_K8S_API_UNAVAILABLE` | K8s API server unavailable | ✅ Yes |
| `ERR_INVALID_CRD_SPEC` | CRD validation failed | ❌ No |
| `ERR_CIRCUIT_BREAKER_OPEN` | Circuit breaker open (fail-fast) | ✅ Yes |
| `ERR_UNAUTHORIZED` | K8s API authorization failed | ❌ No |
| `ERR_CONFLICT` | CRD already exists | ❌ No |

**Implementation**: `pkg/gateway/audit_helpers.go:toAPIErrorDetails()`

---

### FR-4: Circuit Breaker State Tracking (BR-GATEWAY-093)

**Requirement**: When circuit breaker is open, emit `gateway.crd.failed` audit event with specialized error details.

**Circuit Breaker Error Details**:
```json
{
  "component": "gateway",
  "code": "ERR_CIRCUIT_BREAKER_OPEN",
  "message": "K8s API circuit breaker is open (fail-fast mode) - preventing cascade failure",
  "retry_possible": true
}
```

**Implementation**: `pkg/gateway/server.go:emitCRDCreationFailedAudit()` with `gobreaker.ErrOpenState` detection

---

## Non-Functional Requirements

### NFR-1: Performance

**Requirement**: Audit event emission must not block signal processing.

**Implementation**:
- Fire-and-forget pattern (DD-AUDIT-002)
- Buffered audit store (ADR-038)
- Target latency: <10ms (P95)

---

### NFR-2: Reliability

**Requirement**: Audit event emission failures must be logged but not fail signal processing.

**Implementation**:
```go
if storeErr := s.auditStore.StoreAudit(ctx, event); storeErr != nil {
    s.logger.Info("Failed to emit audit event", "error", storeErr)
    // Continue processing - do NOT return error
}
```

---

### NFR-3: Data Integrity

**Requirement**: Audit events must be immutable and tamper-evident.

**Implementation**:
- PostgreSQL JSONB storage (ADR-034)
- Append-only audit table
- No UPDATE or DELETE operations
- Event ID (UUID) for uniqueness

---

## Testing Requirements

### Integration Tests

**Coverage**: All audit emission scenarios validated in `test/integration/gateway/audit_emission_integration_test.go`

**Test Scenarios**:
- GW-INT-AUD-001: Successful CRD creation
- GW-INT-AUD-002: Duplicate signal (deduplication)
- GW-INT-AUD-016: K8s API failure
- GW-INT-AUD-017: Error type classification
- GW-INT-AUD-019: Circuit breaker state tracking
- GW-INT-AUD-020: Audit ID uniqueness

**Validation**:
- Audit event structure matches OpenAPI schema
- Correlation ID format is human-readable (BR-GATEWAY-058-A)
- Error details include all required fields
- Events queryable by correlation ID patterns

---

## Implementation Details

### Code Structure

**Files**:
- `pkg/gateway/server.go`: Main audit emission logic
- `pkg/gateway/audit_helpers.go`: Type conversion helpers
- `pkg/api/openapi.yaml`: Audit event schema (GatewayAuditPayload)

**Helper Functions**:
- `constructReadableCorrelationID(signal)`: BR-GATEWAY-058-A enhancement
- `toAPIErrorDetails(errorDetails)`: Error details conversion
- `toGatewayAuditPayloadSeverity(severity)`: Severity mapping

---

## Monitoring & Observability

### Metrics

**Required Metrics**:
- `gateway_audit_events_emitted_total{event_type, outcome}`: Total audit events emitted
- `gateway_audit_emission_duration_seconds{event_type}`: Audit emission latency
- `gateway_audit_emission_failures_total{event_type, error_code}`: Audit emission failures

**Alerts**:
- `GatewayAuditEmissionFailureRate > 5%`: Alert SRE team
- `GatewayAuditEmissionLatencyP95 > 50ms`: Performance degradation

---

## Related Documentation

### Design Decisions

- **DD-AUDIT-002**: Audit Shared Library Design
- **DD-AUDIT-003**: Service Audit Trace Requirements
- **DD-AUDIT-CORRELATION-001**: Correlation ID standards

### Business Requirements

- **BR-GATEWAY-093**: Circuit Breaker for Kubernetes API
- **BR-AUDIT-005**: Hybrid Provider Data Capture (SOC2 compliance)

### Architecture

- **ADR-034**: Unified Audit Table Design
- **ADR-038**: Async Buffered Audit Ingestion

---

## Migration & Rollout

### Phase 1: Enhancement Implementation (Current)

**Status**: ✅ Complete (2026-01-16)

**Changes**:
- Implemented `constructReadableCorrelationID()` helper
- Updated `emitCRDCreationFailedAudit()` to use readable correlation ID
- Updated integration tests (GW-INT-AUD-016, 017, 019) to query by readable ID

**Migration Impact**: None - failed CRD creation events did not exist before (new feature)

---

### Phase 2: Monitoring & Validation (Next)

**Tasks**:
- Add metrics for audit event emission
- Validate query performance with readable correlation IDs
- SRE team feedback on correlation ID readability

---

## Version History

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| 1.0.0 | 2026-01-15 | Initial BR document | Gateway Team |
| 1.1.0 | 2026-01-16 | **BR-GATEWAY-058-A**: Enhanced correlation ID pattern (human-readable) | Gateway Team |

---

## Approval

**Approved By**: Gateway Service Owner, Security Team, Compliance Team
**Date**: 2026-01-16
**Priority**: P0 - Foundational (SOC2 Compliance)
**Compliance**: SOC2 CC6.8 (Non-Repudiation), CC7.2 (Monitoring Activities)

---

## Appendix: Correlation ID Evolution

### Original Pattern (Pre-BR-GATEWAY-058-A)

**Correlation ID**: SHA256 fingerprint (64-char hex)
```
a1b2c3d4e5f6789012345678901234567890abcdef1234567890abcdef123456
```

**Problem**: Not human-readable, requires reverse lookup to understand alert context.

---

### Enhanced Pattern (BR-GATEWAY-058-A)

**Correlation ID**: Human-readable alert context
```
HighMemoryUsage:prod-payment-service:Pod:payment-api-789
```

**Benefits**:
- ✅ Immediate context for SRE debugging
- ✅ Pattern matching for alert/namespace queries
- ✅ Aligns with OpenTelemetry semantic conventions
- ✅ Fingerprint (SHA256) still available in `event_data.fingerprint` for deduplication

---

### Industry Comparison

| System | Correlation ID Pattern | Readability |
|--------|------------------------|-------------|
| **OpenTelemetry** | Semantic attributes | ✅ High |
| **AWS X-Ray** | Trace ID (hex) | ❌ Low |
| **Jaeger** | Span ID (hex) | ❌ Low |
| **Kubernaut Gateway (Pre-058-A)** | SHA256 fingerprint | ❌ Low |
| **Kubernaut Gateway (Post-058-A)** | `alertname:namespace:kind:name` | ✅ High |

**Conclusion**: BR-GATEWAY-058-A aligns Gateway with OpenTelemetry best practices for semantic observability.

---

## Key Stakeholders

- **Primary**: Gateway Service Team (implementation & maintenance)
- **Secondary**: Security Team (compliance validation)
- **Tertiary**: SRE Team (operational usage & feedback)
- **Compliance**: SOC2 Audit Team (audit trail verification)

---

## Support & Maintenance

**Owner**: Gateway Service Team
**Escalation**: Platform Architecture Team
**Documentation**: This BR document (authoritative reference)
**Code Location**: `pkg/gateway/server.go`, `pkg/gateway/audit_helpers.go`

---

**END OF BUSINESS REQUIREMENT BR-GATEWAY-058**
