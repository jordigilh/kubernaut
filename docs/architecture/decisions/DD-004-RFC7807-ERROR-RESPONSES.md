# DD-004: RFC 7807 Error Response Standard

**Status**: ✅ **APPROVED** (Production Standard)
**Version**: 1.2
**Date**: October 30, 2025
**Last Updated**: December 18, 2025
**Last Reviewed**: December 18, 2025
**Confidence**: 95%

---

## 📝 **Changelog**

### **Version 1.2** (December 18, 2025)
**Changes**:
- **Format Correction**: Removed "Implementation Status by Service" section (operational tracking)
- **Rationale**: DD documents define decisions, not track implementation status
- **Impact**: Documentation-only change (no functional impact)
- **Moved To**: Implementation tracking moved to `docs/handoff/DD_004_V1_1_IMPLEMENTATION_TRACKER.md`
- **Validation Section**: Replaced status tracking with proper validation strategy (how to verify compliance)

### **Version 1.1** (December 18, 2025)
**Changes**:
- **Domain Correction**: Changed error type URI domain from `kubernaut.io` to `kubernaut.ai`
- **Path Standardization**: Changed path from `/errors/` to `/problems/` (aligns with RFC 7807 "Problem Details" terminology)
- **Rationale**: `kubernaut.ai` is the correct production domain; `/problems/` matches RFC 7807 naming convention
- **Impact**: Metadata-only change (status codes and error structure unchanged)

### **Version 1.0** (October 30, 2025)
**Initial Release**:
- Established RFC 7807 as mandatory standard for all HTTP error responses
- Defined error type URI convention (original: `https://kubernaut.io/errors/{error-type}`)
- Documented required fields, content-type headers, and implementation patterns
- Set production readiness approval

---

## 🎯 **Overview**

This design decision establishes **RFC 7807 (Problem Details for HTTP APIs)** as the mandatory standard for all HTTP error responses across Kubernaut services. This ensures consistent, machine-readable error handling for clients and operators.

**Key Principle**: All HTTP error responses (4xx, 5xx) MUST use RFC 7807 Problem Details format. Success responses (2xx) use service-specific formats.

**Scope**: All Kubernaut services that expose HTTP APIs (Gateway, HolmesGPT API, DataStorage, etc.).

---

## 📋 **Table of Contents**

1. [Context & Problem](#context--problem)
2. [Requirements](#requirements)
3. [Alternatives Considered](#alternatives-considered)
4. [Decision](#decision)
5. [Implementation](#implementation)
6. [Examples](#examples)
7. [Migration Guide](#migration-guide)
8. [Validation](#validation)
9. [References](#references)

---

## 🎯 **Context & Problem**

### **Challenge**

Kubernaut consists of multiple microservices (Gateway, HolmesGPT API, DataStorage, etc.) that expose HTTP APIs. Without a standardized error format:

1. ⚠️ **Inconsistent Errors**: Each service uses different error formats
2. ⚠️ **Poor Client Experience**: Clients must parse multiple error formats
3. ⚠️ **Limited Debugging**: Error responses lack structured information
4. ⚠️ **Non-Standard**: Custom formats not aligned with industry standards

### **Business Impact**

- **Operator Efficiency**: Standardized errors improve troubleshooting speed
- **Client Integration**: Single error parser for all Kubernaut services
- **API Quality**: Industry-standard format improves API professionalism
- **Monitoring**: Structured errors enable better alerting and metrics

---

## 📋 **Requirements**

### **Functional Requirements**

| ID | Requirement | Priority | Status |
|----|-------------|----------|--------|
| **FR-1** | All HTTP error responses (4xx, 5xx) use RFC 7807 | P0 | 🔄 In Progress |
| **FR-2** | Error responses include type, title, detail, status, instance | P0 | 🔄 In Progress |
| **FR-3** | Error type URIs follow `https://kubernaut.ai/problems/{error-type}` (v1.1) | P0 | 🔄 In Progress |
| **FR-4** | Content-Type header set to `application/problem+json` | P0 | 🔄 In Progress |
| **FR-5** | Optional request ID for tracing (extension member) | P1 | 🔄 In Progress |
| **FR-6** | Success responses (2xx) use service-specific formats | P0 | 🔄 In Progress |

### **Non-Functional Requirements**

| ID | Requirement | Target | Status |
|----|-------------|--------|--------|
| **NFR-1** | All services implement RFC 7807 before production | 100% | 🔄 In Progress |
| **NFR-2** | Error responses parseable by standard libraries | 100% | 🔄 In Progress |
| **NFR-3** | Consistent error format across all Kubernaut services | 100% | 🔄 In Progress |

**Note**: Backward compatibility is NOT required (pre-release product).

---

## 🏗️ **Alternatives Considered**

### **Alternative 1: Custom JSON Error Format**

**Approach**: Define Kubernaut-specific error format

**Example**:
```json
{
  "error": "validation_failed",
  "message": "Invalid request",
  "code": 400
}
```

**Pros**:
- ✅ Full control over format
- ✅ Simpler implementation (no standard to follow)

**Cons**:
- ❌ Non-standard (clients must learn custom format)
- ❌ No documentation URI (no link to error details)
- ❌ Limited structure (no instance field for tracing)
- ❌ Poor industry alignment (not following best practices)

**Confidence**: 40% (rejected)

---

### **Alternative 2: RFC 7807 Problem Details** ⭐ **APPROVED**

**Approach**: Use IETF standard RFC 7807 for all error responses

**Example** (v1.1):
```json
{
  "type": "https://kubernaut.ai/problems/validation-error",
  "title": "Bad Request",
  "detail": "Invalid Content-Type header format",
  "status": 400,
  "instance": "/api/v1/signals/prometheus"
}
```

**Pros**:
- ✅ **Industry Standard**: IETF RFC (March 2016)
- ✅ **Well-Documented**: Extensive documentation and examples
- ✅ **Machine-Readable**: Type URI enables programmatic error handling
- ✅ **Structured**: Clear fields for type, title, detail, status, instance
- ✅ **Extensible**: Supports custom fields (e.g., request_id)
- ✅ **Client Libraries**: Many languages have RFC 7807 parsers
- ✅ **Consistent**: Single format across all services

**Cons**:
- ⚠️ More verbose than simple JSON (acceptable trade-off)
- ⚠️ Requires Content-Type: `application/problem+json` (standard practice)

**Confidence**: 95% (approved)

---

### **Alternative 3: Google API Error Format**

**Approach**: Use Google Cloud API error format

**Example**:
```json
{
  "error": {
    "code": 400,
    "message": "Invalid request",
    "status": "INVALID_ARGUMENT"
  }
}
```

**Pros**:
- ✅ Used by major cloud provider
- ✅ Well-documented

**Cons**:
- ❌ Google-specific (not IETF standard)
- ❌ No type URI (no link to documentation)
- ❌ No instance field (harder to trace specific requests)
- ❌ Less extensible than RFC 7807

**Confidence**: 60% (rejected)

---

## ✅ **Decision**

**APPROVED: Alternative 2** - RFC 7807 Problem Details

**Rationale**:
1. **Industry Standard**: IETF RFC provides credibility and alignment with best practices
2. **Machine-Readable**: Type URIs enable programmatic error handling and documentation
3. **Structured**: Clear fields improve debugging and monitoring
4. **Extensible**: Supports custom fields (request_id, trace_id, etc.)
5. **Client-Friendly**: Standard format reduces integration complexity

**Key Insight**: Adopting an industry standard reduces cognitive load for developers and operators familiar with RFC 7807 from other systems.

---

## 💻 **Implementation**

### **RFC 7807 Error Structure**

**Reference Implementation**: `pkg/gateway/errors/rfc7807.go` (Gateway service example)

```go
// RFC7807Error represents an RFC 7807 Problem Details error response
// Specification: https://tools.ietf.org/html/rfc7807
type RFC7807Error struct {
    // REQUIRED FIELDS (per RFC 7807)
    Type   string `json:"type"`   // URI reference identifying the problem type
    Title  string `json:"title"`  // Short, human-readable summary
    Detail string `json:"detail"` // Human-readable explanation
    Status int    `json:"status"` // HTTP status code

    // OPTIONAL FIELDS (per RFC 7807)
    Instance string `json:"instance"` // URI reference to specific occurrence

    // EXTENSION MEMBERS (Kubernaut-specific)
    RequestID string `json:"request_id,omitempty"` // Request tracing
}
```

---

### **Error Type URI Convention**

**Format**: `https://kubernaut.ai/problems/{error-type}`

**Version History**:
- **v1.1** (Dec 18, 2025): Changed to `https://kubernaut.ai/problems/{error-type}`
- **v1.0** (Oct 30, 2025): Original `https://kubernaut.io/errors/{error-type}`

**Standard Error Types**:

| HTTP Status | Error Type | Title | Use Case |
|-------------|-----------|-------|----------|
| **400** | `validation-error` | Bad Request | Invalid request format, missing fields |
| **405** | `method-not-allowed` | Method Not Allowed | Wrong HTTP method (GET instead of POST) |
| **415** | `unsupported-media-type` | Unsupported Media Type | Wrong Content-Type header |
| **500** | `internal-error` | Internal Server Error | Unexpected server errors |
| **503** | `service-unavailable` | Service Unavailable | Dependencies down, graceful shutdown |
| **504** | `gateway-timeout` | Gateway Timeout | Upstream service timeout |

**Example URIs** (v1.1):
- `https://kubernaut.ai/problems/validation-error`
- `https://kubernaut.ai/problems/service-unavailable`
- `https://kubernaut.ai/problems/internal-error`

---

### **Content-Type Header**

**REQUIRED**: All RFC 7807 error responses MUST set:

```
Content-Type: application/problem+json
```

**Rationale**: Clients can distinguish between success (application/json) and error (application/problem+json) responses by Content-Type.

---

### **Response Helper Function**

**Pattern**: Each service should implement a helper function for RFC 7807 responses

**Example Implementation Pattern**:

```go
// writeJSONError writes an RFC 7807 compliant error response
func (s *Server) writeJSONError(w http.ResponseWriter, r *http.Request, message string, statusCode int) {
    w.Header().Set("Content-Type", "application/problem+json")
    w.WriteHeader(statusCode)

    // Extract request ID from context for tracing
    requestID := middleware.GetRequestID(r.Context())

    // Sanitize error message to prevent sensitive data exposure
    sanitizedMessage := middleware.SanitizeForLog(message)

    // Determine error type and title based on status code
    errorType, title := getErrorTypeAndTitle(statusCode)

    errorResponse := gwerrors.RFC7807Error{
        Type:      errorType,
        Title:     title,
        Detail:    sanitizedMessage,
        Status:    statusCode,
        Instance:  r.URL.Path,
        RequestID: requestID,
    }

    if err := json.NewEncoder(w).Encode(errorResponse); err != nil {
        // Fallback to plain text if JSON encoding fails
        http.Error(w, message, statusCode)
    }
}
```

---

### **Error Type Mapping**

**Pattern**: Map HTTP status codes to error types

```go
func getErrorTypeAndTitle(statusCode int) (string, string) {
    switch statusCode {
    case http.StatusBadRequest:
        return gwerrors.ErrorTypeValidationError, gwerrors.TitleBadRequest
    case http.StatusMethodNotAllowed:
        return gwerrors.ErrorTypeMethodNotAllowed, gwerrors.TitleMethodNotAllowed
    case http.StatusUnsupportedMediaType:
        return gwerrors.ErrorTypeUnsupportedMediaType, gwerrors.TitleUnsupportedMediaType
    case http.StatusInternalServerError:
        return gwerrors.ErrorTypeInternalError, gwerrors.TitleInternalServerError
    case http.StatusServiceUnavailable:
        return gwerrors.ErrorTypeServiceUnavailable, gwerrors.TitleServiceUnavailable
    default:
        return gwerrors.ErrorTypeUnknown, gwerrors.TitleUnknown
    }
}
```

---

## 📊 **Examples**

### **Example 1: Validation Error (400)**

**Request**:
```bash
curl -X POST http://<service>:8080/api/v1/<endpoint> \
  -H "Content-Type: text/plain" \
  -d '{"invalid": "json"}'
```

**Response** (HTTP 400, v1.1):
```json
{
  "type": "https://kubernaut.ai/problems/validation-error",
  "title": "Bad Request",
  "detail": "Invalid Content-Type header format",
  "status": 400,
  "instance": "/api/v1/<endpoint>",
  "request_id": "req-abc123"
}
```

**Headers**:
```
Content-Type: application/problem+json
```

---

### **Example 2: Service Unavailable (503)**

**Request**:
```bash
curl http://<service>:8080/ready
```

**Response** (HTTP 503, v1.1):
```json
{
  "type": "https://kubernaut.ai/problems/service-unavailable",
  "title": "Service Unavailable",
  "detail": "Service is temporarily unavailable",
  "status": 503,
  "instance": "/ready"
}
```

**Headers**:
```
Content-Type: application/problem+json
```

---

### **Example 3: Method Not Allowed (405)**

**Request**:
```bash
curl -X GET http://<service>:8080/api/v1/<endpoint>
```

**Response** (HTTP 405, v1.1):
```json
{
  "type": "https://kubernaut.ai/problems/method-not-allowed",
  "title": "Method Not Allowed",
  "detail": "Only POST method is allowed for this endpoint",
  "status": 405,
  "instance": "/api/v1/<endpoint>"
}
```

**Headers**:
```
Content-Type: application/problem+json
Allow: POST
```

---

### **Example 4: Internal Server Error (500)**

**Request**:
```bash
curl -X POST http://<service>:8080/api/v1/<endpoint> \
  -H "Content-Type: application/json" \
  -d '{"data": "test"}'
```

**Response** (HTTP 500, v1.1):
```json
{
  "type": "https://kubernaut.ai/problems/internal-error",
  "title": "Internal Server Error",
  "detail": "Failed to process request due to internal error",
  "status": 500,
  "instance": "/api/v1/<endpoint>",
  "request_id": "req-xyz789"
}
```

**Headers**:
```
Content-Type: application/problem+json
```

---

## 🔄 **Migration Guide**

### **For Existing Services**

**Step 1: Create RFC 7807 Error Package**

Create `pkg/{service}/errors/rfc7807.go`:

```go
package errors

// RFC7807Error represents an RFC 7807 Problem Details error response
type RFC7807Error struct {
    Type      string `json:"type"`
    Title     string `json:"title"`
    Detail    string `json:"detail"`
    Status    int    `json:"status"`
    Instance  string `json:"instance"`
    RequestID string `json:"request_id,omitempty"`
}

// Error type URI constants
const (
    // v1.1 (Dec 18, 2025): Updated domain to kubernaut.ai, path to /problems/
    ErrorTypeValidationError      = "https://kubernaut.ai/problems/validation-error"
    ErrorTypeUnsupportedMediaType = "https://kubernaut.ai/problems/unsupported-media-type"
    ErrorTypeMethodNotAllowed     = "https://kubernaut.ai/problems/method-not-allowed"
    ErrorTypeInternalError        = "https://kubernaut.ai/problems/internal-error"
    ErrorTypeServiceUnavailable   = "https://kubernaut.ai/problems/service-unavailable"
    ErrorTypeUnknown              = "https://kubernaut.ai/problems/unknown"
)

// Error title constants
const (
    TitleBadRequest           = "Bad Request"
    TitleUnsupportedMediaType = "Unsupported Media Type"
    TitleMethodNotAllowed     = "Method Not Allowed"
    TitleInternalServerError  = "Internal Server Error"
    TitleServiceUnavailable   = "Service Unavailable"
    TitleUnknown              = "Error"
)
```

---

**Step 2: Update Error Response Functions**

Replace custom error responses with RFC 7807:

**Before**:
```go
func (s *Server) handleError(w http.ResponseWriter, message string, statusCode int) {
    w.WriteHeader(statusCode)
    json.NewEncoder(w).Encode(map[string]string{
        "error": message,
        "code":  strconv.Itoa(statusCode),
    })
}
```

**After**:
```go
func (s *Server) handleError(w http.ResponseWriter, r *http.Request, message string, statusCode int) {
    w.Header().Set("Content-Type", "application/problem+json")
    w.WriteHeader(statusCode)

    errorType, title := getErrorTypeAndTitle(statusCode)

    errorResponse := errors.RFC7807Error{
        Type:     errorType,
        Title:    title,
        Detail:   message,
        Status:   statusCode,
        Instance: r.URL.Path,
    }

    json.NewEncoder(w).Encode(errorResponse)
}
```

---

**Step 3: Update All Error Call Sites**

Find and replace all error responses:

```bash
# Find all custom error responses
grep -r "http.Error\|WriteHeader.*[45][0-9][0-9]" pkg/{service}/ --include="*.go"

# Update each to use RFC 7807 helper function
```

---

**Step 4: Add Integration Tests**

Test RFC 7807 compliance:

```go
It("should return RFC 7807 error for invalid request", func() {
    resp := SendInvalidRequest(testServer.URL)
    Expect(resp.StatusCode).To(Equal(400))

    // Verify Content-Type
    Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))

    // Parse RFC 7807 response
    var errorResp errors.RFC7807Error
    err := json.NewDecoder(resp.Body).Decode(&errorResp)
    Expect(err).ToNot(HaveOccurred())

    // Verify required fields (v1.1)
    Expect(errorResp.Type).To(Equal("https://kubernaut.ai/problems/validation-error"))
    Expect(errorResp.Title).To(Equal("Bad Request"))
    Expect(errorResp.Status).To(Equal(400))
    Expect(errorResp.Instance).To(ContainSubstring("/api/"))
})
```

---

### **Migration Checklist**

**Per Service**:
- [ ] Create `pkg/{service}/errors/rfc7807.go` package
- [ ] Implement `writeJSONError()` helper function
- [ ] Update all error response call sites
- [ ] Add `Content-Type: application/problem+json` header
- [ ] Add integration tests for RFC 7807 compliance
- [ ] Update API documentation with RFC 7807 examples
- [ ] Verify backward compatibility (status codes unchanged)

---

## ✅ **Validation**

### **Validation Strategy**

**How to Verify DD-004 Compliance**:

Services are DD-004 compliant when all HTTP error responses (4xx, 5xx) meet these criteria:

1. ✅ **Content-Type**: `application/problem+json` header present
2. ✅ **Required Fields**: type, title, detail, status, instance all populated
3. ✅ **Error Type URI**: Matches `https://kubernaut.ai/problems/{error-type}` format (v1.1)
4. ✅ **Status Codes**: HTTP status codes unchanged from service's existing behavior
5. ✅ **Extension Members**: Optional request_id field present when available

**Reference Implementation**: `pkg/gateway/errors/rfc7807.go` demonstrates compliant structure

**Implementation Tracking**: See `docs/handoff/DD_004_V1_1_IMPLEMENTATION_TRACKER.md` for service-specific status

**Note**: Success responses (2xx) are not affected by DD-004 and may use service-specific formats.

---

### **Compliance Tests**

**Required Tests** (per service):

1. **Content-Type Header**: Verify `application/problem+json`
2. **Required Fields**: Verify type, title, detail, status, instance
3. **Error Type URIs**: Verify `https://kubernaut.ai/problems/{type}` format (v1.1)
4. **Status Code Mapping**: Verify correct error type for each status
5. **Request ID**: Verify request_id included when available

**Example Test**:
```go
var _ = Describe("RFC 7807 Compliance", func() {
    It("should return RFC 7807 error for 400 Bad Request", func() {
        resp := SendBadRequest(testServer.URL)

        // Verify status code
        Expect(resp.StatusCode).To(Equal(400))

        // Verify Content-Type
        Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))

        // Parse response
        var errorResp errors.RFC7807Error
        err := json.NewDecoder(resp.Body).Decode(&errorResp)
        Expect(err).ToNot(HaveOccurred())

        // Verify required fields (v1.1)
        Expect(errorResp.Type).To(HavePrefix("https://kubernaut.ai/problems/"))
        Expect(errorResp.Title).ToNot(BeEmpty())
        Expect(errorResp.Detail).ToNot(BeEmpty())
        Expect(errorResp.Status).To(Equal(400))
        Expect(errorResp.Instance).ToNot(BeEmpty())
    })
})
```

---

## 📚 **References**

### **RFC 7807 Specification**

- **Title**: Problem Details for HTTP APIs
- **URL**: https://tools.ietf.org/html/rfc7807
- **Status**: IETF Standard (March 2016)
- **Authors**: M. Nottingham, E. Wilde

**Key Sections**:
- Section 3: Problem Details Object
- Section 4.2: Extension Members
- Section 5: Security Considerations

---

### **Industry Examples**

1. **Zalando RESTful API Guidelines**
   - https://opensource.zalando.com/restful-api-guidelines/#176
   - Uses RFC 7807 for all error responses

2. **Microsoft Azure API Guidelines**
   - https://github.com/microsoft/api-guidelines/blob/vNext/Guidelines.md#7102-error-condition-responses
   - Recommends RFC 7807 for error responses

3. **Spring Framework**
   - https://spring.io/blog/2013/11/01/exception-handling-in-spring-mvc#using-http-status-codes
   - Built-in support for RFC 7807 via `ProblemDetail`

---

### **Kubernaut Implementation**

1. **Reference Implementation**: `pkg/gateway/errors/rfc7807.go` (Gateway service)
2. **Example Document**: `docs/architecture/RFC7807_READINESS_UPDATE.md`
3. **Design Decision**: `docs/architecture/DD-004-RFC7807-ERROR-RESPONSES.md` (this document)
4. **Migration Guide**: See "Migration Guide" section above

---

### **Related Documents**

1. **Graceful Shutdown**: `docs/architecture/GRACEFUL_SHUTDOWN_DESIGN.md`
2. **Business Requirements**: `docs/requirements/BR-GATEWAY-*.md`
3. **ADR-027**: Multi-Architecture Build Strategy

---

## ✅ **Summary**

### **Key Design Decisions**

#### **1. RFC 7807 Standard**

**Decision**: Use IETF RFC 7807 for all HTTP error responses

**Rationale**:
- Industry standard (IETF RFC)
- Machine-readable (type URIs)
- Well-documented and widely adopted
- Extensible (supports custom fields)

**Trade-off**: More verbose than simple JSON, but provides better client experience

---

#### **2. Error Type URI Convention**

**Decision**: Use `https://kubernaut.ai/problems/{error-type}` format (v1.1)

**Version History**:
- **v1.1** (Dec 18, 2025): `https://kubernaut.ai/problems/{error-type}`
  - Changed domain to kubernaut.ai (correct production domain)
  - Changed path to /problems/ (aligns with RFC 7807 "Problem Details" terminology)
- **v1.0** (Oct 30, 2025): `https://kubernaut.io/errors/{error-type}` (original)

**Rationale**:
- Consistent namespace for all Kubernaut errors
- Production domain (kubernaut.ai) instead of staging/legacy (kubernaut.io)
- "Problems" terminology matches RFC 7807 specification name
- Enables future documentation at these URIs
- Follows RFC 7807 best practices

**Trade-off**: Requires maintaining error type registry

---

#### **3. Extension Members**

**Decision**: Include `request_id` as optional extension member

**Rationale**:
- Enables request tracing across services
- Improves debugging and monitoring
- Follows RFC 7807 extension pattern

**Trade-off**: Adds complexity to error structure

---

#### **4. Mandatory for All Services**

**Decision**: All HTTP-based services MUST implement RFC 7807 before production

**Rationale**:
- Consistent client experience
- Simplified error handling
- Professional API quality

**Trade-off**: Requires migration effort for existing services

---

### **Confidence Assessment**

**Overall Confidence**: 95% (Production Standard)

**Breakdown**:
- **Standard Selection**: 95% ✅ (RFC 7807 is industry best practice)
- **Implementation Pattern**: 95% ✅ (proven in Gateway service)
- **Client Impact**: 95% ✅ (backward compatible, improves experience)
- **Migration Effort**: 90% ✅ (straightforward, well-documented)

**Why 95%**: Only minor risk is migration effort for existing services, but pattern is proven in Gateway.

---

### **Production Readiness**

**Status**: ✅ **APPROVED FOR PRODUCTION**

**Evidence**:
- ✅ IETF standard (March 2016)
- ✅ Widely adopted (Zalando, Microsoft, Spring)
- ✅ Proven in Gateway service (115 tests passing)
- ✅ Clear migration path for other services
- ✅ Backward compatible (status codes unchanged)

**Recommendation**: ✅ **MANDATORY** for all HTTP-based services before production deployment

---

**Document Version**: 1.1
**Last Updated**: December 18, 2025
**Status**: ✅ **APPROVED FOR PRODUCTION**
**Next Review**: After all services migrate to v1.1 domain/path standards

