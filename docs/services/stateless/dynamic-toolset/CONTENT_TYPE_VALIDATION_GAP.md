# Content-Type Validation Gap - Dynamic Toolset Service

**Status**: 🔴 CRITICAL GAP IDENTIFIED
**Date**: November 9, 2025
**Priority**: P0 (Security & API Contract)
**Identified During**: RFC 7807 Implementation (Step 3 - DO-GREEN)

---

## Problem Statement

The Dynamic Toolset service currently **does not validate Content-Type headers** on POST endpoints. This is a serious security and API contract gap that allows clients to send requests with incorrect Content-Type headers (e.g., `text/plain`, `text/html`) and have them processed as if they were `application/json`.

### Affected Endpoints
1. `POST /api/v1/toolsets/generate` - Expects `application/json`
2. `POST /api/v1/toolsets/validate` - Expects `application/json`
3. `POST /api/v1/discover` - Expects `application/json` (if applicable)

### Current Behavior
```bash
# This should return 415 Unsupported Media Type, but currently returns 400 Bad Request
curl -X POST http://localhost:8080/api/v1/toolsets/validate \
  -H "Content-Type: text/plain" \
  -d "not json"

# Response: 400 Bad Request (should be 415)
```

---

## Security Implications

1. **Content-Type Confusion Attacks**: Attackers could exploit missing validation to bypass security controls
2. **API Contract Violation**: Clients receive misleading error messages (400 instead of 415)
3. **Inconsistent Behavior**: Different services may handle this differently, leading to confusion
4. **RFC 7807 Compliance**: Cannot properly test 415 Unsupported Media Type errors

---

## Required Implementation

### 1. Content-Type Validation Middleware

Create `pkg/toolset/server/middleware/content_type.go`:

```go
package middleware

import (
	"net/http"
	"strings"

	toolseterrors "github.com/jordigilh/kubernaut/pkg/toolset/errors"
)

// ValidateContentType middleware ensures POST/PUT/PATCH requests have application/json Content-Type
func ValidateContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only validate for methods that typically send a body
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
			contentType := r.Header.Get("Content-Type")

			// Remove charset and other parameters
			if idx := strings.Index(contentType, ";"); idx != -1 {
				contentType = contentType[:idx]
			}
			contentType = strings.TrimSpace(contentType)

			// Validate Content-Type is application/json
			if contentType != "application/json" && contentType != "" {
				// Return RFC 7807 error
				rfc7807Err := toolseterrors.NewRFC7807Error(
					http.StatusUnsupportedMediaType,
					"Content-Type must be application/json",
					r.URL.Path,
				)

				// Extract request ID from context if available
				if requestID, ok := r.Context().Value("request_id").(string); ok {
					rfc7807Err.RequestID = requestID
				}

				w.Header().Set("Content-Type", "application/problem+json")
				w.WriteHeader(http.StatusUnsupportedMediaType)
				json.NewEncoder(w).Encode(rfc7807Err)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}
```

### 2. Apply Middleware to POST Endpoints

Update `pkg/toolset/server/server.go`:

```go
// setupRoutes configures HTTP routes
func (s *Server) setupRoutes() {
	// Health endpoints
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/ready", s.handleReady)

	// API endpoints with Content-Type validation
	s.mux.Handle("/api/v1/toolsets/validate",
		middleware.ValidateContentType(http.HandlerFunc(s.handleValidateToolset)))
	s.mux.Handle("/api/v1/toolsets/generate",
		middleware.ValidateContentType(http.HandlerFunc(s.handleGenerateToolset)))
	s.mux.HandleFunc("/api/v1/toolsets/", s.handleToolsetsRouter)
	s.mux.HandleFunc("/api/v1/toolset", s.handleGetLegacyToolset)
	s.mux.HandleFunc("/api/v1/services", s.handleListServices)
	s.mux.Handle("/api/v1/discover",
		middleware.ValidateContentType(http.HandlerFunc(s.handleDiscover)))

	// Metrics endpoint
	s.mux.HandleFunc("/metrics", s.handleMetrics)
}
```

### 3. Unit Tests

Create `pkg/toolset/server/middleware/content_type_test.go`:

```go
package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/toolset/server/middleware"
)

var _ = Describe("Content-Type Validation Middleware", func() {
	var handler http.Handler

	BeforeEach(func() {
		// Create a simple handler that returns 200 OK
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		})

		// Wrap with Content-Type validation middleware
		handler = middleware.ValidateContentType(nextHandler)
	})

	Describe("POST requests", func() {
		It("should accept application/json", func() {
			req := httptest.NewRequest("POST", "/test", strings.NewReader(`{"key":"value"}`))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
		})

		It("should accept application/json with charset", func() {
			req := httptest.NewRequest("POST", "/test", strings.NewReader(`{"key":"value"}`))
			req.Header.Set("Content-Type", "application/json; charset=utf-8")

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
		})

		It("should reject text/plain with 415", func() {
			req := httptest.NewRequest("POST", "/test", strings.NewReader("plain text"))
			req.Header.Set("Content-Type", "text/plain")

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusUnsupportedMediaType))
			Expect(w.Header().Get("Content-Type")).To(Equal("application/problem+json"))
		})

		It("should reject text/html with 415", func() {
			req := httptest.NewRequest("POST", "/test", strings.NewReader("<html></html>"))
			req.Header.Set("Content-Type", "text/html")

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusUnsupportedMediaType))
		})

		It("should accept empty Content-Type (for backward compatibility)", func() {
			req := httptest.NewRequest("POST", "/test", strings.NewReader(`{"key":"value"}`))
			// No Content-Type header set

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
		})
	})

	Describe("GET requests", func() {
		It("should not validate Content-Type for GET", func() {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("Content-Type", "text/html")

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
		})
	})
})

func TestContentTypeMiddleware(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Content-Type Middleware Test Suite")
}
```

### 4. Integration Tests

Update `test/integration/toolset/rfc7807_compliance_test.go` to add Test 1 back as 415:

```go
Describe("Test 1: Unsupported Media Type (415)", func() {
	It("should return RFC 7807 error when Content-Type is not application/json", func() {
		req, err := http.NewRequest("POST", testServer.URL+"/api/v1/toolsets/validate",
			strings.NewReader("invalid"))
		Expect(err).ToNot(HaveOccurred())
		req.Header.Set("Content-Type", "text/plain")

		resp, err := http.DefaultClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusUnsupportedMediaType))
		Expect(resp.Header.Get("Content-Type")).To(Equal("application/problem+json"))

		var errorResp errors.RFC7807Error
		err = json.NewDecoder(resp.Body).Decode(&errorResp)
		Expect(err).ToNot(HaveOccurred())

		Expect(errorResp.Type).To(Equal("https://kubernaut.io/errors/unsupported-media-type"))
		Expect(errorResp.Title).To(Equal("Unsupported Media Type"))
		Expect(errorResp.Status).To(Equal(415))
	})
})
```

---

## Business Requirement

**BR-TOOLSET-043: Content-Type Validation**
- **Priority**: P0
- **Status**: 🔴 Missing
- **Description**: Validate Content-Type header on POST/PUT/PATCH requests to ensure API contract compliance
- **Acceptance Criteria**:
  - POST requests with `Content-Type: text/plain` return 415 Unsupported Media Type
  - POST requests with `Content-Type: application/json` are accepted
  - POST requests with `Content-Type: application/json; charset=utf-8` are accepted
  - GET requests are not affected by Content-Type validation
  - RFC 7807 error format used for 415 responses
- **Test Coverage**:
  - Unit: `pkg/toolset/server/middleware/content_type_test.go` (5 tests)
  - Integration: `test/integration/toolset/rfc7807_compliance_test.go` (Test 1)

---

## Implementation Timeline

**Estimated Effort**: 2-3 hours

1. **Phase 1**: Create Content-Type validation middleware (30 min)
2. **Phase 2**: Add unit tests for middleware (45 min)
3. **Phase 3**: Apply middleware to POST endpoints (15 min)
4. **Phase 4**: Update integration tests (30 min)
5. **Phase 5**: Update BR documentation (30 min)

---

## Related Services

This gap likely exists in other services as well. After implementing for Dynamic Toolset, audit:

- ✅ Gateway Service (check if implemented)
- ✅ Context API (check if implemented)
- ✅ Data Storage Service (check if implemented)
- ✅ HolmesGPT API (Python/FastAPI - check if implemented)
- ⏸️ Notification Service (CRD controller - not applicable)

---

## References

- **RFC 7231 Section 6.5.13**: 415 Unsupported Media Type
- **RFC 7807**: Problem Details for HTTP APIs
- **ADR-036**: Authentication and Authorization Strategy (security context)
- **Gateway Service**: Check `pkg/gateway/middleware/` for reference implementation

---

**Action Items**:
1. [ ] Implement Content-Type validation middleware
2. [ ] Add unit tests (5 tests minimum)
3. [ ] Apply middleware to POST endpoints
4. [ ] Update integration tests
5. [ ] Document BR-TOOLSET-043
6. [ ] Audit other services for same gap

