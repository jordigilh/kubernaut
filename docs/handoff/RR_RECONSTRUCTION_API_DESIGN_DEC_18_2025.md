# RR Reconstruction API Endpoint Design

**Date**: December 18, 2025
**Status**: ✅ **APPROVED FOR V1.0**
**Business Requirement**: BR-AUDIT-005 v2.0 (Enterprise-Grade Audit Integrity and Compliance)
**Priority**: **P0** - V1.0 Release Blocker

---

## 🎯 **Overview**

**Endpoint Purpose**: Reconstruct a RemediationRequest CRD from audit traces after TTL expiration

**Delivery**: **API Endpoint** (V1.0) - CLI wrapper (Post-V1.0)

**Rationale**:
- ✅ **Automation-ready**: Integrate with CI/CD, monitoring, dashboards
- ✅ **Programmatic access**: Build custom tools on top
- ✅ **Scale**: Handle hundreds of concurrent reconstructions
- ✅ **RBAC enforcement**: Role-based access control
- ✅ **Audit logging**: Track who reconstructed what

---

## 📋 **API Specification**

### **Endpoint**: `POST /v1/audit/remediation-requests/:id/reconstruct`

**Method**: `POST` (idempotent)
**Path Parameter**: `:id` - RemediationRequest name (e.g., `rr-2025-001`)
**Content-Type**: `application/json`
**Authentication**: OAuth2 via Kubernetes RBAC

---

### **Request**

```http
POST /v1/audit/remediation-requests/rr-2025-001/reconstruct HTTP/1.1
Host: data-storage.kubernaut.svc.cluster.local
Authorization: Bearer <kubernetes-token>
Content-Type: application/json

{
  "format": "yaml",
  "include_status": true,
  "include_metadata": true
}
```

**Request Body** (all fields optional):

```json
{
  "format": "yaml",              // "yaml" or "json" (default: "yaml")
  "include_status": true,         // Include .status fields (default: true)
  "include_metadata": true,       // Include metadata (timestamps, etc.) (default: true)
  "validation_mode": "strict"     // "strict" or "best_effort" (default: "strict")
}
```

---

### **Response (Success)**

**Status Code**: `200 OK`

```http
HTTP/1.1 200 OK
Content-Type: application/x-yaml

apiVersion: remediation.kubernaut.ai/v1alpha1
kind: RemediationRequest
metadata:
  name: rr-2025-001
  namespace: kubernaut-system
  creationTimestamp: "2025-01-15T10:30:00Z"
  annotations:
    kubernaut.ai/reconstructed: "true"
    kubernaut.ai/reconstruction-timestamp: "2025-01-17T14:22:00Z"
    kubernaut.ai/reconstruction-accuracy: "100%"
    kubernaut.ai/reconstruction-source: "audit-traces"
spec:
  signalFingerprint: "oomkilled-ns-web-pod-api-server"
  signalName: "KubernetesPodOOMKilled"
  severity: "critical"
  targetResource:
    kind: "Pod"
    name: "api-server"
    namespace: "web"
  # ... all other spec fields (100% coverage)
status:
  phase: "Completed"
  outcome: "Success"
  # ... all system-managed status fields (90% coverage)
```

---

### **Response (JSON Format)**

If `format: "json"` is requested:

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "apiVersion": "remediation.kubernaut.ai/v1alpha1",
  "kind": "RemediationRequest",
  "metadata": {
    "name": "rr-2025-001",
    "namespace": "kubernaut-system",
    "creationTimestamp": "2025-01-15T10:30:00Z",
    "annotations": {
      "kubernaut.ai/reconstructed": "true",
      "kubernaut.ai/reconstruction-timestamp": "2025-01-17T14:22:00Z",
      "kubernaut.ai/reconstruction-accuracy": "100%"
    }
  },
  "spec": { ... },
  "status": { ... }
}
```

---

### **Response (Error - RR Not Found)**

**Status Code**: `404 Not Found`

```http
HTTP/1.1 404 Not Found
Content-Type: application/problem+json

{
  "type": "https://kubernaut.ai/api/errors/remediation-request-not-found",
  "title": "RemediationRequest Not Found in Audit Traces",
  "status": 404,
  "detail": "No audit traces found for RemediationRequest 'rr-2025-001'. It may have been deleted before audit traces were written, or the retention period has expired.",
  "instance": "/v1/audit/remediation-requests/rr-2025-001/reconstruct",
  "correlation_id": "req-abc123",
  "suggestions": [
    "Verify the RemediationRequest name is correct",
    "Check if the RR was created after audit logging was enabled",
    "Verify audit retention policy allows reconstruction for this time period"
  ]
}
```

---

### **Response (Error - Insufficient Audit Data)**

**Status Code**: `422 Unprocessable Entity`

```http
HTTP/1.1 422 Unprocessable Entity
Content-Type: application/problem+json

{
  "type": "https://kubernaut.ai/api/errors/insufficient-audit-data",
  "title": "Insufficient Audit Data for Reconstruction",
  "status": 422,
  "detail": "RemediationRequest 'rr-2025-001' cannot be fully reconstructed. Missing audit events: 'gateway.signal.received', 'aianalysis.analysis.completed'. Reconstruction accuracy: 70% (below threshold).",
  "instance": "/v1/audit/remediation-requests/rr-2025-001/reconstruct",
  "correlation_id": "req-abc123",
  "reconstruction_accuracy": 70,
  "missing_events": [
    "gateway.signal.received",
    "aianalysis.analysis.completed"
  ],
  "suggestions": [
    "Enable validation_mode='best_effort' to return partial reconstruction",
    "Verify all services are emitting audit events correctly",
    "Check if audit events were deleted prematurely"
  ]
}
```

---

### **Response (Error - Unauthorized)**

**Status Code**: `403 Forbidden`

```http
HTTP/1.1 403 Forbidden
Content-Type: application/problem+json

{
  "type": "https://kubernaut.ai/api/errors/insufficient-permissions",
  "title": "Insufficient Permissions",
  "status": 403,
  "detail": "User 'developer@example.com' does not have permission to reconstruct RemediationRequests. Required role: 'audit-viewer' or higher.",
  "instance": "/v1/audit/remediation-requests/rr-2025-001/reconstruct",
  "correlation_id": "req-abc123",
  "required_permissions": [
    "audit:remediationrequests:reconstruct"
  ]
}
```

---

### **Response (Error - Rate Limit)**

**Status Code**: `429 Too Many Requests`

```http
HTTP/1.1 429 Too Many Requests
Content-Type: application/problem+json
Retry-After: 60

{
  "type": "https://kubernaut.ai/api/errors/rate-limit-exceeded",
  "title": "Rate Limit Exceeded",
  "status": 429,
  "detail": "Reconstruction rate limit exceeded. Maximum 100 requests per hour for user 'developer@example.com'. Please retry after 60 seconds.",
  "instance": "/v1/audit/remediation-requests/rr-2025-001/reconstruct",
  "correlation_id": "req-abc123",
  "rate_limit": {
    "limit": 100,
    "remaining": 0,
    "reset": "2025-01-17T15:00:00Z"
  }
}
```

---

## 🔐 **Authentication & Authorization**

### **Authentication**: OAuth2 via Kubernetes RBAC

**Token Validation**:
- Kubernetes ServiceAccount token (bearer token)
- Validates against Kubernetes API server
- Uses existing RBAC roles

### **Authorization**: Role-Based Access Control (RBAC)

**Required Permission**: `audit:remediationrequests:reconstruct`

**RBAC Roles**:

| Role | Permission | Use Case |
|------|------------|----------|
| **audit-viewer** | ✅ Reconstruct (read-only) | SREs, compliance auditors |
| **audit-operator** | ✅ Reconstruct + export | Operations team |
| **audit-admin** | ✅ All audit operations | Security team, CISO |

**Example RBAC ClusterRole**:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kubernaut-audit-viewer
rules:
- apiGroups: ["kubernaut.ai"]
  resources: ["audit/remediationrequests/reconstruct"]
  verbs: ["create"]  # POST = create operation
```

---

## 📊 **Rate Limiting**

**Purpose**: Prevent abuse and ensure fair usage

**Limits** (per user):
- **Default**: 100 reconstructions per hour
- **Burst**: 10 concurrent requests
- **Timeout**: 30 seconds per reconstruction

**Headers** (returned in all responses):

```http
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 2025-01-17T15:00:00Z
```

**Rate Limit Algorithm**: Token bucket (refills every hour)

---

## 📝 **Audit Logging**

**Requirement**: All reconstruction requests MUST be audited

**Audit Event**: `audit.reconstruction.requested`

**Event Data**:

```json
{
  "event_type": "audit.reconstruction.requested",
  "event_category": "audit",
  "timestamp": "2025-01-17T14:22:00Z",
  "user": "developer@example.com",
  "source_ip": "10.0.1.42",
  "remediation_request_id": "rr-2025-001",
  "reconstruction_format": "yaml",
  "reconstruction_accuracy": "100%",
  "reconstruction_duration_ms": 245,
  "outcome": "success"
}
```

**Purpose**:
- ✅ **Compliance**: Track who accessed audit data
- ✅ **Security**: Detect suspicious reconstruction patterns
- ✅ **Operations**: Monitor API usage and performance

---

## 🧪 **Testing Strategy**

### **Integration Tests**

```go
var _ = Describe("RR Reconstruction API", func() {
    It("should reconstruct RR from audit traces (200 OK)", func() {
        // 1. Create and execute remediation
        rr := createRemediationRequest(ctx, "test-signal")
        Eventually(getRRPhase(ctx, rr.Name)).Should(Equal("Completed"))

        // 2. Delete RR (simulate TTL expiration)
        deleteRemediationRequest(ctx, rr.Name)

        // 3. Call reconstruction API
        resp := POST("/v1/audit/remediation-requests/" + rr.Name + "/reconstruct", map[string]interface{}{
            "format": "yaml",
        })

        // 4. Validate response
        Expect(resp.StatusCode).To(Equal(200))
        Expect(resp.Body).To(ContainSubstring("apiVersion: remediation.kubernaut.ai/v1alpha1"))
        Expect(resp.Body).To(ContainSubstring("name: " + rr.Name))

        // 5. Validate reconstruction accuracy
        reconstructed := parseYAML(resp.Body)
        Expect(reconstructed.Spec.SignalFingerprint).To(Equal(rr.Spec.SignalFingerprint))
        Expect(reconstructed.Status.Phase).To(Equal("Completed"))
    })

    It("should return 404 when RR not found", func() {
        resp := POST("/v1/audit/remediation-requests/nonexistent-rr/reconstruct", nil)

        Expect(resp.StatusCode).To(Equal(404))
        Expect(resp.Body).To(ContainSubstring("RemediationRequest Not Found"))
    })

    It("should enforce rate limiting (429)", func() {
        // Exhaust rate limit
        for i := 0; i < 101; i++ {
            POST("/v1/audit/remediation-requests/test-rr/reconstruct", nil)
        }

        // Next request should be rate limited
        resp := POST("/v1/audit/remediation-requests/test-rr/reconstruct", nil)
        Expect(resp.StatusCode).To(Equal(429))
        Expect(resp.Headers["Retry-After"]).ToNot(BeEmpty())
    })

    It("should enforce RBAC (403)", func() {
        // Use unauthorized user
        resp := POSTAsUser("/v1/audit/remediation-requests/test-rr/reconstruct", nil, "unauthorized-user")

        Expect(resp.StatusCode).To(Equal(403))
        Expect(resp.Body).To(ContainSubstring("Insufficient Permissions"))
    })
})
```

---

## 📈 **Performance Targets**

| Metric | Target | Rationale |
|--------|--------|-----------|
| **Response Time** | < 500ms (p95) | Fast enough for interactive use |
| **Concurrent Requests** | 100+ | Support multiple users simultaneously |
| **Reconstruction Accuracy** | 100% (spec), 90% (status) | As per BR-AUDIT-005 v2.0 |
| **Availability** | 99.9% | Same as Data Storage service |
| **Error Rate** | < 1% | Robust error handling |

---

## 🚀 **Implementation Plan**

### **Phase 1: Core API (Day 6 of RR Reconstruction Plan)**

**Tasks**:
1. **OpenAPI Spec** (1 hour)
   - Add endpoint to `api/openapi/data-storage-v1.yaml`
   - Define request/response schemas

2. **Handler Implementation** (2 hours)
   - File: `pkg/datastorage/server/reconstruction_handler.go`
   - Implement reconstruction logic
   - Validate RBAC permissions

3. **Error Handling** (1 hour)
   - RFC 7807 problem details
   - Graceful degradation

4. **Audit Logging** (30 min)
   - Log all reconstruction requests

5. **Integration Tests** (2 hours)
   - Test happy path
   - Test error cases
   - Test RBAC enforcement

**Total**: 6.5 hours (included in Day 6 of 6.5-day plan)

---

### **Phase 2: Rate Limiting & Monitoring (Day 6.5)**

**Tasks**:
1. **Rate Limiting** (1 hour)
   - Implement token bucket algorithm
   - Return rate limit headers

2. **Prometheus Metrics** (1 hour)
   - `reconstruction_requests_total{status}`
   - `reconstruction_duration_seconds`
   - `reconstruction_accuracy_percent`

3. **Documentation** (1 hour)
   - API reference
   - Usage examples
   - Troubleshooting guide

**Total**: 3 hours (included in Day 6.5)

---

## 📚 **Usage Examples**

### **Example 1: Reconstruct RR for Incident Investigation**

```bash
# Using curl
curl -X POST \
  "https://data-storage.kubernaut.svc.cluster.local/v1/audit/remediation-requests/rr-2025-001/reconstruct" \
  -H "Authorization: Bearer $KUBERNETES_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"format": "yaml"}' \
  > rr-2025-001-reconstructed.yaml
```

### **Example 2: Programmatic Access (Python)**

```python
import requests
import yaml

def reconstruct_rr(rr_name, token):
    url = f"https://data-storage.kubernaut.svc.cluster.local/v1/audit/remediation-requests/{rr_name}/reconstruct"
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json"
    }
    response = requests.post(url, json={"format": "yaml"}, headers=headers)

    if response.status_code == 200:
        return yaml.safe_load(response.text)
    else:
        raise Exception(f"Reconstruction failed: {response.json()}")

# Usage
rr = reconstruct_rr("rr-2025-001", kubernetes_token)
print(f"Signal: {rr['spec']['signalFingerprint']}")
print(f"Outcome: {rr['status']['outcome']}")
```

### **Example 3: Integration with Dashboard**

```javascript
// React component for RR reconstruction
async function reconstructRR(rrName) {
  const response = await fetch(
    `/v1/audit/remediation-requests/${rrName}/reconstruct`,
    {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${getToken()}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({ format: 'json' })
    }
  );

  if (response.ok) {
    const rr = await response.json();
    return rr;
  } else {
    const error = await response.json();
    throw new Error(error.detail);
  }
}
```

---

## ✅ **Success Criteria**

1. ✅ **API Endpoint**: `POST /v1/audit/remediation-requests/:id/reconstruct` implemented
2. ✅ **RBAC Enforcement**: Only authorized users can reconstruct
3. ✅ **Rate Limiting**: Prevent abuse (100 req/hour per user)
4. ✅ **Audit Logging**: All reconstruction requests logged
5. ✅ **Error Handling**: RFC 7807 problem details
6. ✅ **Performance**: < 500ms response time (p95)
7. ✅ **Testing**: Integration tests for happy path + error cases
8. ✅ **Documentation**: OpenAPI spec + usage examples

---

## 📊 **Post-V1.0 Enhancements**

### **CLI Wrapper** (1-2 days)

```bash
# Simple CLI wrapper around API
kubernaut rr reconstruct rr-2025-001 > rr-reconstructed.yaml
kubernaut rr reconstruct rr-2025-001 --format json | jq '.spec.signalFingerprint'
```

**Implementation**: Thin Go CLI that calls the API endpoint

---

### **Bulk Reconstruction** (2-3 days)

```http
POST /v1/audit/remediation-requests/reconstruct/bulk
{
  "rr_names": ["rr-2025-001", "rr-2025-002", ...],
  "format": "yaml"
}
```

**Use Case**: Reconstruct all RRs for a specific incident or time period

---

### **Web UI** (1 week)

**Features**:
- Search for RR by name, date range, correlation ID
- Click "Reconstruct" button
- View reconstructed RR in YAML/JSON
- Compare reconstructed vs live RR (diff view)

---

## 🎯 **Confidence Assessment**

**API Implementation Feasibility**: **100%** ✅
**Business Value**: **95%** ✅ (see [RR_RECONSTRUCTION_ENTERPRISE_VALUE_CONFIDENCE_ASSESSMENT_DEC_18_2025.md](./RR_RECONSTRUCTION_ENTERPRISE_VALUE_CONFIDENCE_ASSESSMENT_DEC_18_2025.md))
**Overall Recommendation**: **APPROVE FOR V1.0** ✅

---

**Status**: ✅ **APPROVED** - API endpoint for V1.0, CLI wrapper post-V1.0

