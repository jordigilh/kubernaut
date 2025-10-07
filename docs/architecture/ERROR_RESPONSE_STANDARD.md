# Error Response Standard - Kubernaut Services

**Version**: v1.0
**Last Updated**: October 6, 2025
**Status**: ✅ **STANDARDIZED**
**Scope**: All 6 Stateless HTTP Services

---

## 📋 **Standard Error Response Format**

### **JSON Structure** (All Services)

```json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "Human-readable error message",
    "details": {
      "field": "fieldName",
      "reason": "Validation failed"
    }
  },
  "timestamp": "2025-10-06T10:15:30Z",
  "path": "/api/v1/resource",
  "correlationId": "req-20251006101530-abc123",
  "requestId": "req-uuid-12345",
  "retryAfter": 60,
  "documentation": "https://docs.kubernaut.io/errors/ERROR_CODE"
}
```

---

## 🎯 **Required Fields** (All Errors)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `error.code` | string | ✅ **YES** | Machine-readable error code |
| `error.message` | string | ✅ **YES** | Human-readable message |
| `timestamp` | string (ISO 8601) | ✅ **YES** | Error timestamp |
| `path` | string | ✅ **YES** | Request path |
| `correlationId` | string | ✅ **YES** | Log correlation ID |

---

## 📊 **Optional Fields** (Context-Dependent)

| Field | Type | When to Include | Description |
|-------|------|-----------------|-------------|
| `error.details` | object | Validation errors | Structured validation details |
| `requestId` | string | Idempotent operations | Client-provided request ID |
| `retryAfter` | integer | 429, 503 errors | Seconds until retry |
| `documentation` | string | All errors | Error documentation URL |

---

## 🔧 **Go Implementation**

### **Error Response Structure**

```go
// pkg/errors/response.go
package errors

import (
    "encoding/json"
    "net/http"
    "time"

    "github.com/jordigilh/kubernaut/pkg/correlation"
)

// ErrorResponse represents a standardized API error response
type ErrorResponse struct {
    Error         *ErrorDetail `json:"error"`
    Timestamp     string       `json:"timestamp"`
    Path          string       `json:"path"`
    CorrelationID string       `json:"correlationId"`
    RequestID     string       `json:"requestId,omitempty"`
    RetryAfter    int          `json:"retryAfter,omitempty"`
    Documentation string       `json:"documentation,omitempty"`
}

// ErrorDetail contains error-specific information
type ErrorDetail struct {
    Code    string      `json:"code"`
    Message string      `json:"message"`
    Details interface{} `json:"details,omitempty"`
}

// WriteError writes a standardized error response
func WriteError(w http.ResponseWriter, r *http.Request, statusCode int, code string, message string, details interface{}) {
    correlationID := correlation.FromContext(r.Context())

    response := &ErrorResponse{
        Error: &ErrorDetail{
            Code:    code,
            Message: message,
            Details: details,
        },
        Timestamp:     time.Now().UTC().Format(time.RFC3339),
        Path:          r.URL.Path,
        CorrelationID: correlationID,
        Documentation: "https://docs.kubernaut.io/errors/" + code,
    }

    // Add requestId if provided by client
    if requestID := r.Header.Get("X-Request-ID"); requestID != "" {
        response.RequestID = requestID
    }

    // Add retryAfter for rate limit and service unavailable errors
    if statusCode == http.StatusTooManyRequests || statusCode == http.StatusServiceUnavailable {
        response.RetryAfter = calculateRetryAfter(statusCode)
    }

    w.Header().Set("Content-Type", "application/json")
    w.Header().Set("X-Correlation-ID", correlationID)

    if response.RetryAfter > 0 {
        w.Header().Set("Retry-After", fmt.Sprintf("%d", response.RetryAfter))
    }

    w.WriteHeader(statusCode)
    json.NewEncoder(w).Encode(response)
}

func calculateRetryAfter(statusCode int) int {
    switch statusCode {
    case http.StatusTooManyRequests:
        return 60 // 1 minute
    case http.StatusServiceUnavailable:
        return 30 // 30 seconds
    default:
        return 0
    }
}
```

---

### **Validation Error Helper**

```go
// ValidationError represents field-level validation errors
type ValidationError struct {
    Field    string `json:"field"`
    Expected string `json:"expected,omitempty"`
    Received string `json:"received,omitempty"`
    Reason   string `json:"reason"`
}

// WriteValidationError writes a 400 Bad Request with validation details
func WriteValidationError(w http.ResponseWriter, r *http.Request, field string, reason string) {
    details := &ValidationError{
        Field:  field,
        Reason: reason,
    }

    WriteError(w, r, http.StatusBadRequest, "VALIDATION_ERROR",
        fmt.Sprintf("Validation failed for field: %s", field), details)
}
```

---

## 📊 **Standard Error Codes**

### **Authentication & Authorization** (401, 403)

| Code | HTTP Status | Message | Details |
|------|-------------|---------|---------|
| `AUTH_TOKEN_MISSING` | 401 | Missing Authorization header | None |
| `AUTH_TOKEN_INVALID` | 401 | Invalid or expired token | `{"reason": "token expired"}` |
| `AUTH_TOKEN_UNAUTHORIZED` | 403 | Token not authorized for this resource | `{"requiredPermission": "read:context"}` |

**Example**:
```json
{
  "error": {
    "code": "AUTH_TOKEN_INVALID",
    "message": "Invalid or expired token",
    "details": {
      "reason": "token expired at 2025-10-06T10:00:00Z"
    }
  },
  "timestamp": "2025-10-06T10:15:30Z",
  "path": "/api/v1/context",
  "correlationId": "req-20251006101530-abc123"
}
```

---

### **Validation Errors** (400)

| Code | HTTP Status | Message | Details |
|------|-------------|---------|---------|
| `VALIDATION_ERROR` | 400 | Validation failed | Field-specific validation error |
| `INVALID_JSON` | 400 | Invalid JSON in request body | Parsing error details |
| `MISSING_REQUIRED_FIELD` | 400 | Required field missing | `{"field": "namespace"}` |
| `INVALID_FIELD_VALUE` | 400 | Invalid value for field | `{"field": "priority", "expected": "P0/P1/P2", "received": "invalid"}` |

**Example**:
```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Validation failed for field: namespace",
    "details": {
      "field": "namespace",
      "expected": "valid Kubernetes namespace name",
      "received": "invalid namespace!",
      "reason": "namespace contains invalid characters"
    }
  },
  "timestamp": "2025-10-06T10:15:30Z",
  "path": "/api/v1/context",
  "correlationId": "req-20251006101530-abc123",
  "documentation": "https://docs.kubernaut.io/errors/VALIDATION_ERROR"
}
```

---

### **Resource Errors** (404, 409)

| Code | HTTP Status | Message | Details |
|------|-------------|---------|---------|
| `RESOURCE_NOT_FOUND` | 404 | Requested resource not found | `{"resourceType": "deployment", "name": "api"}` |
| `RESOURCE_CONFLICT` | 409 | Resource already exists | `{"name": "existing-resource"}` |

---

### **Rate Limiting** (429)

| Code | HTTP Status | Message | Details |
|------|-------------|---------|---------|
| `RATE_LIMIT_EXCEEDED` | 429 | Rate limit exceeded | `{"limit": 100, "window": "60s", "client": "ai-analysis-sa"}` |
| `INVESTIGATION_LIMIT_EXCEEDED` | 429 | Investigation rate limit exceeded (HolmesGPT) | `{"limit": "5/min", "client": "ai-analysis-sa"}` |

**Example**:
```json
{
  "error": {
    "code": "RATE_LIMIT_EXCEEDED",
    "message": "Rate limit exceeded",
    "details": {
      "limit": 100,
      "window": "60s",
      "client": "ai-analysis-sa",
      "remaining": 0
    }
  },
  "timestamp": "2025-10-06T10:15:30Z",
  "path": "/api/v1/context",
  "correlationId": "req-20251006101530-abc123",
  "retryAfter": 60,
  "documentation": "https://docs.kubernaut.io/errors/RATE_LIMIT_EXCEEDED"
}
```

---

### **Service Errors** (500, 503)

| Code | HTTP Status | Message | Details |
|------|-------------|---------|---------|
| `INTERNAL_SERVER_ERROR` | 500 | Internal server error | None (details logged) |
| `DATABASE_ERROR` | 500 | Database operation failed | `{"operation": "query", "table": "incident_embeddings"}` |
| `EXTERNAL_SERVICE_ERROR` | 502 | External service error | `{"service": "prometheus", "endpoint": "http://prometheus:9090"}` |
| `SERVICE_UNAVAILABLE` | 503 | Service temporarily unavailable | `{"reason": "database connection pool exhausted"}` |

**Example**:
```json
{
  "error": {
    "code": "SERVICE_UNAVAILABLE",
    "message": "Service temporarily unavailable",
    "details": {
      "reason": "database connection pool exhausted",
      "maxConnections": 100,
      "currentConnections": 100
    }
  },
  "timestamp": "2025-10-06T10:15:30Z",
  "path": "/api/v1/context",
  "correlationId": "req-20251006101530-abc123",
  "retryAfter": 30,
  "documentation": "https://docs.kubernaut.io/errors/SERVICE_UNAVAILABLE"
}
```

---

## 🎯 **Service-Specific Error Codes**

### **Gateway Service**

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `SIGNAL_VALIDATION_FAILED` | 400 | Invalid signal payload |
| `ADAPTER_NOT_FOUND` | 400 | No adapter for signal type |
| `DEDUPLICATION_ERROR` | 500 | Redis deduplication failed |
| `CRD_CREATION_FAILED` | 500 | RemediationRequest CRD creation failed |

---

### **Context API Service**

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `QUERY_INVALID` | 400 | Invalid query parameters |
| `EMBEDDING_GENERATION_FAILED` | 500 | Failed to generate embeddings |
| `VECTOR_SEARCH_FAILED` | 500 | Vector DB query failed |

---

### **Data Storage Service**

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `AUDIT_WRITE_FAILED` | 500 | Failed to write audit record |
| `EMBEDDING_STORAGE_FAILED` | 500 | Failed to store embedding |
| `INVALID_AUDIT_RECORD` | 400 | Invalid audit record format |

---

### **HolmesGPT API Service**

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `INVESTIGATION_FAILED` | 500 | LLM investigation failed |
| `TOOLSET_NOT_FOUND` | 404 | Requested toolset not available |
| `LLM_TIMEOUT` | 504 | LLM request timed out |

---

### **Notification Service**

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `NOTIFICATION_SEND_FAILED` | 500 | Failed to send notification |
| `CHANNEL_UNAVAILABLE` | 503 | Notification channel unavailable |
| `TEMPLATE_RENDER_FAILED` | 500 | Template rendering failed |

---

### **Dynamic Toolset Service**

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `DISCOVERY_FAILED` | 500 | Service discovery failed |
| `CONFIGMAP_UPDATE_FAILED` | 500 | ConfigMap update failed |
| `TOOLSET_VALIDATION_FAILED` | 400 | Invalid toolset configuration |

---

## 📚 **Error Documentation**

### **Error Documentation Site**

**Base URL**: `https://docs.kubernaut.io/errors/`

**Structure**:
```
docs/errors/
├── AUTH_TOKEN_MISSING.md
├── VALIDATION_ERROR.md
├── RATE_LIMIT_EXCEEDED.md
├── SERVICE_UNAVAILABLE.md
└── ...
```

**Example Documentation** (`RATE_LIMIT_EXCEEDED.md`):
```markdown
# RATE_LIMIT_EXCEEDED

**HTTP Status**: 429 Too Many Requests
**Retry**: Yes (see Retry-After header)

## Description
Rate limit exceeded for the current client.

## Resolution
1. Wait for the time specified in Retry-After header
2. Reduce request rate
3. Contact support if limit is insufficient

## Details
- Per-client limits vary by service
- See rate limiting documentation for details
```

---

## ✅ **Implementation Checklist**

### **For Each Service**:

1. ✅ **Use standard error format**: `ErrorResponse` struct
2. ✅ **Include all required fields**: code, message, timestamp, path, correlationId
3. ✅ **Add optional fields** when appropriate: details, retryAfter, documentation
4. ✅ **Structured logging**: Log error with correlationId and details
5. ✅ **Error metrics**: Prometheus counters for error types
6. ✅ **Documentation links**: Include error code documentation URLs
7. ✅ **Consistent error codes**: Use standard codes across services

---

## 📊 **Error Metrics**

```
# Error count by code
{service}_errors_total{code="VALIDATION_ERROR"} 10

# Error rate
rate({service}_errors_total[5m])

# Error ratio
rate({service}_errors_total[5m]) / rate({service}_requests_total[5m])
```

---

## 🎯 **Testing Error Responses**

### **Test Script**

```bash
#!/bin/bash
# test-error-responses.sh

SERVICE_URL="http://context-api:8080"
TOKEN="<serviceaccount-token>"

# Test 1: Missing required field (400)
curl -X POST "$SERVICE_URL/api/v1/context" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"namespace": ""}' \
  | jq

# Expected: {"error": {"code": "VALIDATION_ERROR", ...}}

# Test 2: Invalid token (401)
curl "$SERVICE_URL/api/v1/context" \
  -H "Authorization: Bearer invalid-token" \
  | jq

# Expected: {"error": {"code": "AUTH_TOKEN_INVALID", ...}}

# Test 3: Rate limit (429)
for i in {1..150}; do
  curl -s "$SERVICE_URL/api/v1/context?namespace=prod" \
    -H "Authorization: Bearer $TOKEN" \
    | jq -r '.error.code' || echo "200"
done | sort | uniq -c

# Expected: ~50 null (200 OK), ~100 RATE_LIMIT_EXCEEDED (429)
```

---

## 📚 **Related Documentation**

- [LOG_CORRELATION_ID_STANDARD.md](./LOG_CORRELATION_ID_STANDARD.md) - Correlation ID format
- [RATE_LIMITING_STANDARD.md](./RATE_LIMITING_STANDARD.md) - Rate limiting details
- [KUBERNETES_TOKENREVIEWER_AUTH.md](./KUBERNETES_TOKENREVIEWER_AUTH.md) - Authentication errors

---

**Document Status**: ✅ Complete
**Compliance**: 6/6 services covered
**Last Updated**: October 6, 2025
**Version**: 1.0
