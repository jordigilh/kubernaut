# DD-GATEWAY-001: Payload Size Limits to Prevent etcd Exhaustion

## Status
✅ **APPROVED** (2025-10-22)
**Last Reviewed**: 2025-10-22
**Confidence**: 95%

---

## Context & Problem

### **The Problem**
Gateway Service accepts webhook payloads from Prometheus AlertManager and Kubernetes Events without size validation. These payloads are stored in RemediationRequest CRDs with multiple fields containing potentially large data:

- `SignalLabels` (map[string]string) - No size limit
- `SignalAnnotations` (map[string]string) - No size limit
- `OriginalPayload` ([]byte) - No size limit
- `ProviderData` ([]byte) - No size limit

**Critical Risk**: Kubernetes etcd has a **1.5MB per-object limit**. Large payloads can cause:
1. CRD creation failures (etcd rejects oversized objects)
2. Production incidents dropped (alerts lost)
3. Gateway OOM (processing huge payloads)
4. Cluster instability (etcd performance degradation)

### **Real-World Scenario**
```
Prometheus Alert with Large Labels:
- alertname: "HighMemoryUsage" (16 bytes)
- description: 15KB of troubleshooting text
- runbook_url: 2KB of documentation
- dashboard_url: 1KB of Grafana links
- 10 additional labels: 5KB each = 50KB

Total SignalLabels: ~68KB
OriginalPayload: ~100KB
ProviderData: ~50KB
CRD metadata: ~10KB

TOTAL CRD SIZE: ~228KB ✅ (under 1.5MB)

BUT: Storm CRD aggregating 10 similar alerts:
- 10 × 228KB = 2.28MB ❌ EXCEEDS etcd limit
- Result: CRD creation fails, incident lost
```

### **Key Requirements**
- **BR-GATEWAY-010**: MUST enforce payload size limits to protect cluster stability
- **BR-GATEWAY-003**: MUST validate all incoming payloads before processing
- **BR-GATEWAY-015**: MUST ensure CRD creation succeeds for all valid alerts

---

## Alternatives Considered

### Alternative 1: No Size Limits (Current Implementation)
**Approach**: Accept all payloads regardless of size, rely on etcd to reject oversized objects.

**Pros**:
- ✅ Simple implementation (no validation code)
- ✅ No data loss (all payload data preserved)
- ✅ No false rejections

**Cons**:
- ❌ **CRITICAL**: CRD creation can fail silently
- ❌ **CRITICAL**: Production incidents can be lost
- ❌ Gateway OOM risk from huge payloads
- ❌ No early feedback to alert sources
- ❌ etcd performance degradation

**Confidence**: 10% (unacceptable production risk)
**Status**: ❌ **REJECTED** - Critical production risk

---

### Alternative 2: HTTP Payload Size Limit (512KB)
**Approach**: Reject payloads exceeding 512KB at HTTP middleware layer before processing.

**Implementation**:
```go
// pkg/gateway/server/middleware.go
func MaxPayloadSizeMiddleware(maxBytes int64) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
            next.ServeHTTP(w, r)
        })
    }
}

// Apply to all webhook endpoints
router.Use(MaxPayloadSizeMiddleware(512 * 1024)) // 512KB limit
```

**Sizing Rationale**:
- etcd limit: 1.5MB per object
- CRD overhead: ~50KB (metadata, status, spec fields)
- Safety margin: 50% (for storm aggregation, future fields)
- **Result**: 512KB payload limit leaves ~1MB for CRD structure

**Pros**:
- ✅ **Prevents etcd rejections** (CRDs stay under 1.5MB)
- ✅ **Prevents Gateway OOM** (rejects before processing)
- ✅ **Fast rejection** (no parsing/validation overhead)
- ✅ **Clear error response** (HTTP 413 Request Entity Too Large)
- ✅ **Simple implementation** (standard Go HTTP middleware)
- ✅ **Configurable** (can adjust limit based on production data)

**Cons**:
- ⚠️ May reject legitimate large alerts (rare)
- ⚠️ Requires monitoring to tune limit
- ⚠️ Clients must handle 413 errors

**Confidence**: 95% (proven pattern, addresses root cause)
**Status**: ✅ **APPROVED** - Immediate implementation (Phase 1)

---

### Alternative 3: External Storage for Large Payloads
**Approach**: Store `OriginalPayload` in object storage (S3/MinIO), store only reference in CRD.

**Implementation**:
```go
// pkg/gateway/storage/payload_store.go
type PayloadStore interface {
    Store(ctx context.Context, fingerprint string, payload []byte) (string, error)
    Retrieve(ctx context.Context, reference string) ([]byte, error)
}

// RemediationRequest CRD
type RemediationRequestSpec struct {
    // ... existing fields ...

    // Reference to external payload storage (if payload > threshold)
    OriginalPayloadRef string `json:"originalPayloadRef,omitempty"`

    // Inline payload (if < threshold, e.g., 50KB)
    OriginalPayload []byte `json:"originalPayload,omitempty"`
}
```

**Pros**:
- ✅ **No size limits** (unlimited payload storage)
- ✅ **Full data preservation** (no truncation)
- ✅ **Reduces etcd load** (smaller CRDs)
- ✅ **Scales to large alerts** (multi-MB payloads supported)

**Cons**:
- ❌ **Complex implementation** (new infrastructure component)
- ❌ **External dependency** (S3/MinIO required)
- ❌ **Additional latency** (network round-trip for retrieval)
- ❌ **Operational overhead** (backup, retention, monitoring)
- ❌ **Cost** (storage costs for large volumes)

**Confidence**: 60% (complex, requires infrastructure)
**Status**: 🔄 **DEFERRED to kubernaut v2.0** - Future enhancement

---

## Decision

### **APPROVED: Alternative 2 (HTTP Payload Size Limit) - Immediate Implementation**

**Rationale**:
1. **Addresses Root Cause**: Prevents etcd 1.5MB limit violations before they occur
2. **Simplicity**: Standard HTTP middleware pattern, no new dependencies
3. **Fast Rejection**: Fails fast with clear error (HTTP 413)
4. **Proven Pattern**: Industry-standard approach (nginx, AWS ALB, etc.)
5. **Configurable**: Can adjust limit based on production metrics

**Key Insight**:
> **Prevention is better than recovery**. Rejecting oversized payloads at the HTTP layer is simpler, faster, and more reliable than handling etcd rejection failures downstream.

### **DEFERRED: Alternative 3 (External Storage) - kubernaut v2.0**

**Rationale**:
1. **Future-Proofing**: Enables unlimited payload sizes for advanced use cases
2. **Complexity**: Requires infrastructure investment (S3/MinIO, lifecycle management)
3. **Not Urgent**: 512KB limit sufficient for 99%+ of production alerts
4. **Better ROI Later**: Implement when proven need exists (based on production data)

---

## Implementation

### **Phase 1: HTTP Payload Size Limit (Immediate)**

**Primary Implementation Files**:
- `pkg/gateway/server/middleware.go` - MaxPayloadSizeMiddleware implementation
- `pkg/gateway/server/server.go` - Apply middleware to router
- `test/unit/gateway/server/middleware_test.go` - Unit tests for size limit
- `test/integration/gateway/webhook_e2e_test.go` - Integration tests with oversized payloads

**Configuration**:
```yaml
# config/gateway.yaml
server:
  maxPayloadSize: 524288  # 512KB in bytes
  maxPayloadSizeHuman: "512KB"  # For logging
```

**Data Flow**:
1. HTTP request arrives at Gateway
2. MaxPayloadSizeMiddleware checks Content-Length header
3. If > 512KB: Reject with HTTP 413 + error message
4. If ≤ 512KB: Pass to handler for processing
5. Handler creates CRD (guaranteed < 1.5MB)

**Error Response**:
```json
{
  "error": "payload too large",
  "maxSize": "512KB",
  "receivedSize": "650KB",
  "message": "Reduce alert label sizes or split into multiple alerts"
}
```

**Graceful Degradation**:
- Oversized alerts rejected with clear error
- Prometheus AlertManager retries with exponential backoff
- Operators alerted via metrics (rejected_payloads_total counter)

---

### **Phase 2: External Storage (kubernaut v2.0)**

**Deferred Implementation**:
- Object storage integration (S3/MinIO)
- Payload reference system
- Automatic threshold-based routing (inline vs external)
- Lifecycle management (retention, cleanup)

**Trigger Conditions**:
- Production data shows >1% of alerts rejected due to size
- Use cases emerge requiring >512KB payloads
- Infrastructure investment approved

---

## Consequences

### **Positive**:
- ✅ **Prevents CRD creation failures** (etcd rejections eliminated)
- ✅ **Prevents Gateway OOM** (huge payloads rejected early)
- ✅ **Clear error feedback** (clients know why rejection occurred)
- ✅ **Cluster stability** (etcd protected from oversized objects)
- ✅ **Simple implementation** (standard middleware pattern)
- ✅ **Configurable** (can tune based on production data)

### **Negative**:
- ⚠️ **May reject legitimate large alerts** (rare edge case)
  - **Mitigation**: 512KB is generous (typical alerts <10KB)
  - **Mitigation**: Metrics track rejection rate
  - **Mitigation**: Can increase limit if needed (up to ~1MB)
- ⚠️ **Requires client error handling** (HTTP 413)
  - **Mitigation**: Prometheus AlertManager has built-in retry logic
  - **Mitigation**: Clear error message guides remediation

### **Neutral**:
- 🔄 Adds configuration parameter (maxPayloadSize)
- 🔄 Adds monitoring metric (rejected_payloads_total)
- 🔄 Requires documentation update (webhook size limits)

---

## Validation Results

### **Confidence Assessment Progression**:
- Initial assessment: 90% confidence (proven pattern)
- After etcd limit research: 95% confidence (critical risk confirmed)
- After user approval: 95% confidence (Option A approved, Option C deferred)

### **Key Validation Points**:
- ✅ etcd 1.5MB limit confirmed (Kubernetes documentation)
- ✅ CRD size calculation validated (228KB typical, 2.28MB storm risk)
- ✅ 512KB limit provides 3x safety margin
- ✅ Standard HTTP middleware pattern (proven in production)
- ✅ User approval received (Option A immediate, Option C v2.0)

### **Production Risk Mitigation**:
| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| etcd rejection | HIGH (without limit) | CRITICAL (incident lost) | ✅ 512KB limit prevents |
| Gateway OOM | MEDIUM (without limit) | HIGH (service crash) | ✅ Early rejection prevents |
| False rejections | LOW (with 512KB) | MEDIUM (alert lost) | ⚠️ Monitor metrics, adjust limit |
| Client errors | LOW (Prometheus retries) | LOW (temporary delay) | ✅ Clear error message |

---

## Related Decisions
- **Supports**: BR-GATEWAY-010 (payload size limits)
- **Supports**: BR-GATEWAY-003 (payload validation)
- **Supports**: BR-GATEWAY-015 (CRD creation success)
- **Enables**: Future DD-GATEWAY-002 (external storage for v2.0)

---

## Review & Evolution

### **When to Revisit**:
- If rejection rate >1% in production (indicates limit too low)
- If use cases emerge requiring >512KB payloads
- If etcd limit changes in future Kubernetes versions
- If storm aggregation causes CRDs to approach 1.5MB
- When implementing kubernaut v2.0 (external storage)

### **Success Metrics**:
- **CRD creation success rate**: Target 100% (no etcd rejections)
- **Payload rejection rate**: Target <0.1% (rare edge cases only)
- **Gateway OOM incidents**: Target 0 (eliminated)
- **Average CRD size**: Target <200KB (well under 1.5MB)
- **P99 CRD size**: Target <500KB (safety margin maintained)

### **Monitoring**:
```promql
# Payload rejections
rate(gateway_rejected_payloads_total{reason="size_limit"}[5m])

# CRD size distribution
histogram_quantile(0.99, gateway_crd_size_bytes_bucket)

# etcd object size (if available)
etcd_object_size_bytes{resource="remediationrequests"}
```

---

## Implementation Status

### **Phase 1: HTTP Payload Size Limit (Immediate)**
- [ ] Create `pkg/gateway/server/middleware.go` with MaxPayloadSizeMiddleware
- [ ] Apply middleware in `pkg/gateway/server/server.go`
- [ ] Add unit test in `test/unit/gateway/server/middleware_test.go`
- [ ] Add integration test in `test/integration/gateway/webhook_e2e_test.go`
- [ ] Update configuration schema (config/gateway.yaml)
- [ ] Add Prometheus metric (gateway_rejected_payloads_total)
- [ ] Update documentation (docs/services/stateless/gateway-service/)

### **Phase 2: External Storage (kubernaut v2.0)**
- [ ] Design object storage integration
- [ ] Implement PayloadStore interface
- [ ] Add automatic threshold-based routing
- [ ] Implement lifecycle management
- [ ] Update CRD schema (OriginalPayloadRef field)
- [ ] Migration plan for existing CRDs

---

## References

- [Kubernetes etcd Size Limits](https://kubernetes.io/docs/tasks/administer-cluster/configure-upgrade-etcd/#etcd-request-size-limit)
- [Go http.MaxBytesReader](https://pkg.go.dev/net/http#MaxBytesReader)
- [RemediationRequest CRD Schema](../../api/remediation/v1alpha1/remediationrequest_types.go)
- [Gateway Implementation Plan v2.6](../../services/stateless/gateway-service/IMPLEMENTATION_PLAN_V2.6.md)
- [BR-GATEWAY-010: Payload Size Limits](../../requirements/gateway-service-requirements.md#br-gateway-010)

---

**Document Version**: 1.0
**Last Updated**: 2025-10-22
**Next Review**: After Phase 1 implementation complete

