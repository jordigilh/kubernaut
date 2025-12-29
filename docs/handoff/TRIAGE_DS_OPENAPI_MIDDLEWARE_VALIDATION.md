# DataStorage - OpenAPI Middleware Validation Triage

**Date**: 2025-12-13
**Status**: 📋 **TRIAGE COMPLETE** - Ready for implementation
**Priority**: Medium (Quality improvement, not blocking)
**Estimated Effort**: 4-6 hours

---

## 🎯 **Objective**

Replace manual validation functions with automated OpenAPI middleware validation using `kin-openapi` library.

**Benefits**:
- ✅ Automatic validation for all endpoints
- ✅ Single source of truth (OpenAPI spec)
- ✅ Less manual code to maintain
- ✅ Catches spec violations automatically

---

## 🔍 **Current State Analysis**

### **Manual Validation Locations**

**Files with manual validation**:
1. ✅ `pkg/datastorage/server/helpers/openapi_conversion.go` - `ValidateAuditEventRequest()`
2. ✅ `pkg/datastorage/server/helpers/validation.go` - Various validation helpers
3. ✅ `pkg/datastorage/server/audit_events_handler.go` - Handler-specific validation

**Current validation logic** (~80 lines total):
- Required field empty checks
- Enum validation (event_outcome)
- Timestamp bounds validation
- Field length constraints

### **Router Architecture**

**Current router**: Chi router (`github.com/go-chi/chi/v5`)

```go
// pkg/datastorage/server/server.go
func (s *Server) setupRoutes() {
    r := chi.NewRouter()

    // Existing middleware
    r.Use(middleware.RequestID)
    r.Use(middleware.RealIP)
    r.Use(middleware.Logger)
    r.Use(middleware.Recoverer)

    // Routes
    r.Post("/audit/events", s.handleCreateAuditEvent)
    r.Post("/audit/events/batch", s.handleBatchAuditEvents)
    r.Get("/audit/events", s.handleQueryAuditEvents)
    r.Post("/audit/events/query", s.handleQueryAuditEventsPost)

    r.Get("/workflows/search", s.handleWorkflowSearch)
    r.Post("/workflows", s.handleCreateWorkflow)
    // ... more routes
}
```

**Good news**: Chi router is compatible with `kin-openapi`! ✅

---

## 📋 **Implementation Plan**

### **Phase 1: Setup Dependencies** (30 minutes)

#### **Step 1.1: Add Dependencies**
```bash
go get github.com/getkin/kin-openapi/openapi3@latest
go get github.com/getkin/kin-openapi/openapi3filter@latest
go get github.com/getkin/kin-openapi/routers/legacy@latest
```

**Expected versions**:
- `kin-openapi`: v0.124.0+ (latest as of Dec 2025)

#### **Step 1.2: Verify OpenAPI Spec**
```bash
# Validate spec is correct
go run github.com/getkin/kin-openapi/cmd/validate@latest \
  api/openapi/data-storage-v1.yaml
```

**Action**: Fix any spec validation errors before proceeding

---

### **Phase 2: Create Middleware Package** (1-1.5 hours)

#### **Step 2.1: Create Middleware Structure**

**New file**: `pkg/datastorage/server/middleware/openapi.go`

```go
/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package middleware

import (
	"context"
	"fmt"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/go-logr/logr"
)

// OpenAPIValidator is a middleware that validates requests against OpenAPI spec
type OpenAPIValidator struct {
	router routers.Router
	logger logr.Logger
}

// NewOpenAPIValidator creates a new OpenAPI validation middleware
// BR-STORAGE-034: Automatic API request validation
func NewOpenAPIValidator(specPath string, logger logr.Logger) (*OpenAPIValidator, error) {
	// Load OpenAPI spec
	ctx := context.Background()
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true

	doc, err := loader.LoadFromFile(specPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load OpenAPI spec from %s: %w", specPath, err)
	}

	// Validate spec
	if err := doc.Validate(ctx); err != nil {
		return nil, fmt.Errorf("OpenAPI spec validation failed: %w", err)
	}

	// Create router for matching requests to operations
	router, err := legacy.NewRouter(doc)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAPI router: %w", err)
	}

	logger.Info("OpenAPI validator initialized",
		"spec_path", specPath,
		"api_version", doc.Info.Version,
		"paths_count", len(doc.Paths.Map()))

	return &OpenAPIValidator{
		router: router,
		logger: logger,
	}, nil
}

// Middleware returns a Chi-compatible middleware handler
func (v *OpenAPIValidator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Find the operation for this request
		route, pathParams, err := v.router.FindRoute(r)
		if err != nil {
			// Route not in OpenAPI spec (e.g., /health, /metrics)
			// Pass through without validation
			v.logger.V(2).Info("Route not in OpenAPI spec, skipping validation",
				"method", r.Method,
				"path", r.URL.Path,
				"error", err)
			next.ServeHTTP(w, r)
			return
		}

		// Log validation attempt
		v.logger.V(1).Info("Validating request against OpenAPI spec",
			"method", r.Method,
			"path", r.URL.Path,
			"operation_id", route.Operation.OperationID)

		// Configure validation options
		options := &openapi3filter.Options{
			// Enable strict validation
			ExcludeRequestBody:    false,
			ExcludeResponseBody:   true, // Don't validate responses (performance)
			IncludeResponseStatus: false,

			// Custom validation for required fields
			MultiError: true, // Collect all errors, not just first one
		}

		// Create validation input
		requestValidationInput := &openapi3filter.RequestValidationInput{
			Request:    r,
			PathParams: pathParams,
			Route:      route,
			Options:    options,
		}

		// Validate request
		if err := openapi3filter.ValidateRequest(r.Context(), requestValidationInput); err != nil {
			// Validation failed - return RFC 7807 error
			v.logger.Info("Request validation failed",
				"method", r.Method,
				"path", r.URL.Path,
				"operation_id", route.Operation.OperationID,
				"error", err)

			v.writeValidationError(w, r, err)
			return
		}

		// Validation passed
		v.logger.V(1).Info("Request validation passed",
			"method", r.Method,
			"path", r.URL.Path,
			"operation_id", route.Operation.OperationID)

		// Proceed to handler
		next.ServeHTTP(w, r)
	})
}

// writeValidationError writes an RFC 7807 error response for validation failures
func (v *OpenAPIValidator) writeValidationError(w http.ResponseWriter, r *http.Request, validationErr error) {
	// Parse validation errors
	var details string
	var validationErrors []string

	// kin-openapi returns MultiError for multiple validation failures
	if multiErr, ok := validationErr.(interface{ Unwrap() []error }); ok {
		for _, err := range multiErr.Unwrap() {
			validationErrors = append(validationErrors, err.Error())
		}
		details = fmt.Sprintf("Multiple validation errors: %v", validationErrors)
	} else {
		details = validationErr.Error()
	}

	// RFC 7807 Problem Details
	problem := map[string]interface{}{
		"type":   "https://api.kubernaut.io/problems/validation_error",
		"title":  "Request Validation Error",
		"status": http.StatusBadRequest,
		"detail": details,
	}

	// Add request context
	if requestID := r.Header.Get("X-Request-ID"); requestID != "" {
		w.Header().Set("X-Request-ID", requestID)
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusBadRequest)

	// Write JSON response
	import "encoding/json"
	if err := json.NewEncoder(w).Encode(problem); err != nil {
		v.logger.Error(err, "Failed to encode validation error response")
	}
}
```

#### **Step 2.2: Add Tests**

**New file**: `pkg/datastorage/server/middleware/openapi_test.go`

```go
package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/datastorage/server/middleware"
)

func TestOpenAPIMiddleware(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "OpenAPI Middleware Suite")
}

var _ = Describe("OpenAPI Validator Middleware", func() {
	var (
		validator *middleware.OpenAPIValidator
		logger    logr.Logger
	)

	BeforeEach(func() {
		logger = logr.Discard()
		var err error
		validator, err = middleware.NewOpenAPIValidator(
			"../../../api/openapi/data-storage-v1.yaml",
			logger,
		)
		Expect(err).ToNot(HaveOccurred())
	})

	Context("Valid requests", func() {
		It("should pass validation for valid audit event", func() {
			// Create valid request
			body := `{
				"version": "1.0",
				"event_type": "gateway.signal.received",
				"event_category": "gateway",
				"event_action": "received",
				"event_outcome": "success",
				"correlation_id": "test-123",
				"event_timestamp": "2025-12-13T12:00:00Z",
				"event_data": {}
			}`

			req := httptest.NewRequest("POST", "/api/v1/audit/events", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()

			// Mock handler
			handler := validator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusCreated)
			}))

			handler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusCreated))
		})
	})

	Context("Invalid requests", func() {
		It("should reject request with missing required field", func() {
			// Missing event_type
			body := `{
				"version": "1.0",
				"event_category": "gateway",
				"event_action": "received",
				"event_outcome": "success",
				"correlation_id": "test-123",
				"event_timestamp": "2025-12-13T12:00:00Z",
				"event_data": {}
			}`

			req := httptest.NewRequest("POST", "/api/v1/audit/events", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()

			handler := validator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusCreated)
			}))

			handler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			Expect(rr.Header().Get("Content-Type")).To(Equal("application/problem+json"))
		})

		It("should reject request with invalid enum value", func() {
			body := `{
				"version": "1.0",
				"event_type": "gateway.signal.received",
				"event_category": "gateway",
				"event_action": "received",
				"event_outcome": "invalid_value",
				"correlation_id": "test-123",
				"event_timestamp": "2025-12-13T12:00:00Z",
				"event_data": {}
			}`

			req := httptest.NewRequest("POST", "/api/v1/audit/events", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()

			handler := validator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusCreated)
			}))

			handler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
		})
	})

	Context("Routes not in spec", func() {
		It("should pass through non-OpenAPI routes", func() {
			req := httptest.NewRequest("GET", "/health", nil)
			rr := httptest.NewRecorder()

			handler := validator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			handler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK))
		})
	})
})
```

---

### **Phase 3: Integrate Middleware** (30 minutes)

#### **Step 3.1: Update Server Setup**

**File**: `pkg/datastorage/server/server.go`

```go
import (
	"github.com/jordigilh/kubernaut/pkg/datastorage/server/middleware"
)

func (s *Server) setupRoutes() {
	r := chi.NewRouter()

	// Existing middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// NEW: Add OpenAPI validation middleware
	openapiValidator, err := middleware.NewOpenAPIValidator(
		"api/openapi/data-storage-v1.yaml", // Spec path
		s.logger,
	)
	if err != nil {
		s.logger.Error(err, "Failed to initialize OpenAPI validator")
		// Fallback: Continue without validation (or panic in production)
	} else {
		r.Use(openapiValidator.Middleware)
		s.logger.Info("OpenAPI validation middleware enabled")
	}

	// Routes (validation now automatic)
	r.Post("/audit/events", s.handleCreateAuditEvent)
	r.Post("/audit/events/batch", s.handleBatchAuditEvents)
	r.Get("/audit/events", s.handleQueryAuditEvents)
	// ... rest of routes
}
```

#### **Step 3.2: Add Configuration**

**File**: `pkg/datastorage/config/config.go`

```go
type Config struct {
	// ... existing fields

	// OpenAPI validation settings
	OpenAPIValidation struct {
		Enabled  bool   `yaml:"enabled" default:"true"`
		SpecPath string `yaml:"spec_path" default:"api/openapi/data-storage-v1.yaml"`
	} `yaml:"openapi_validation"`
}
```

**File**: `config/datastorage.yaml`

```yaml
openapi_validation:
  enabled: true
  spec_path: "api/openapi/data-storage-v1.yaml"
```

---

### **Phase 4: Remove Manual Validation** (1-1.5 hours)

#### **Step 4.1: Identify Validation to Remove**

**Validation that OpenAPI middleware will handle**:
- ✅ Required field presence (including empty strings!)
- ✅ Enum validation (event_outcome)
- ✅ Type validation (string, number, etc.)
- ✅ Format validation (date-time, uuid, etc.)

**Validation to KEEP** (custom business rules):
- ⚠️ Timestamp bounds (5 min future, 7 days past)
- ⚠️ Field length constraints (if not in OpenAPI spec)
- ⚠️ Cross-field validation

#### **Step 4.2: Update Handlers**

**File**: `pkg/datastorage/server/audit_events_handler.go`

```go
func (s *Server) handleCreateAuditEvent(w http.ResponseWriter, r *http.Request) {
	// ... existing code

	var req dsclient.AuditEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeRFC7807Error(w, ...)
		return
	}

	// REMOVE: helpers.ValidateAuditEventRequest(&req)
	// OpenAPI middleware already validated:
	// - Required fields (including empty strings)
	// - Enum values
	// - Field types

	// KEEP: Custom business validation
	if err := helpers.ValidateTimestampBounds(req.EventTimestamp); err != nil {
		writeRFC7807Error(w, ...)
		return
	}

	if err := helpers.ValidateFieldLengths(req); err != nil {
		writeRFC7807Error(w, ...)
		return
	}

	// Proceed with conversion and storage
	event, err := helpers.ConvertAuditEventRequest(req)
	// ...
}
```

#### **Step 4.3: Clean Up Helper Files**

**Actions**:
1. ✅ Remove `ValidateAuditEventRequest()` from `openapi_conversion.go`
2. ✅ Keep `ValidateTimestampBounds()` (custom business rule)
3. ✅ Keep `ValidateFieldLengths()` (if not in OpenAPI spec)
4. ✅ Update tests to reflect middleware validation

---

### **Phase 5: Update OpenAPI Spec** (30 minutes)

#### **Step 5.1: Add Missing Constraints**

**File**: `api/openapi/data-storage-v1.yaml`

Ensure spec has all validation rules:

```yaml
components:
  schemas:
    AuditEventRequest:
      type: object
      required:
        - version
        - event_type
        - event_timestamp
        - event_category
        - event_action
        - event_outcome
        - correlation_id
        - event_data
      properties:
        version:
          type: string
          minLength: 1  # ADD: Reject empty strings
          maxLength: 20
          example: "1.0"
        event_type:
          type: string
          minLength: 1  # ADD: Reject empty strings
          maxLength: 100
          example: gateway.signal.received
        event_category:
          type: string
          minLength: 1  # ADD: Reject empty strings
          maxLength: 50
          example: gateway
        event_action:
          type: string
          minLength: 1  # ADD: Reject empty strings
          maxLength: 50
          example: received
        event_outcome:
          type: string
          enum: [success, failure, pending]
          example: success
        correlation_id:
          type: string
          minLength: 1  # ADD: Reject empty strings
          maxLength: 255
          format: uuid   # OPTIONAL: If always UUID
          example: "550e8400-e29b-41d4-a716-446655440000"
        event_timestamp:
          type: string
          format: date-time
          example: "2025-12-13T12:00:00Z"
        # ... rest of fields
```

**Key additions**:
- `minLength: 1` on all required string fields (rejects empty strings!)
- `maxLength` constraints
- `format` specifications where applicable

#### **Step 5.2: Validate Spec**

```bash
go run github.com/getkin/kin-openapi/cmd/validate@latest \
  api/openapi/data-storage-v1.yaml
```

---

### **Phase 6: Testing** (1.5-2 hours)

#### **Step 6.1: Unit Tests**

```bash
# Test middleware package
go test ./pkg/datastorage/server/middleware/... -v
```

**Expected**: 100% pass rate

#### **Step 6.2: Integration Tests**

```bash
# Run full integration test suite
go test ./test/integration/datastorage/... -v
```

**Expected**: 149/149 passing (same as current)

**If failures occur**:
- Check if OpenAPI spec matches handler expectations
- Verify middleware isn't too strict
- Update spec or tests as needed

#### **Step 6.3: E2E Tests**

```bash
# Run E2E tests
go test ./test/e2e/datastorage/... -v -timeout=30m
```

**Expected**: All passing (after Podman machine restart)

---

## 🎯 **Validation Strategy**

### **What OpenAPI Middleware Will Validate**

✅ **Automatic** (no code needed):
- Required fields present
- Required fields not empty (with `minLength: 1`)
- Enum values valid
- Field types correct
- String formats (date-time, uuid, email, etc.)
- Number ranges (min, max)
- Array constraints (minItems, maxItems)

### **What to Keep as Custom Validation**

⚠️ **Manual** (business rules):
- Timestamp bounds (5 min future, 7 days past)
- Cross-field validation
- Database lookups (e.g., parent_event_id exists)
- Complex business rules not expressible in OpenAPI

---

## 📊 **Impact Analysis**

### **Code Changes**

| Action | Files | Lines |
|--------|-------|-------|
| **Add middleware package** | +2 files | +250 lines |
| **Update server.go** | 1 file | +15 lines |
| **Update config** | 2 files | +10 lines |
| **Update OpenAPI spec** | 1 file | +20 lines |
| **Remove validation helpers** | 2 files | -60 lines |
| **Update handlers** | 3 files | -30 lines |
| **NET CHANGE** | **11 files** | **+205 lines** |

### **Dependencies**

**New dependencies**:
```
github.com/getkin/kin-openapi/openapi3 v0.124.0
github.com/getkin/kin-openapi/openapi3filter v0.124.0
github.com/getkin/kin-openapi/routers/legacy v0.124.0
```

**Size**: ~2MB total (reasonable for validation library)

### **Performance Impact**

**Middleware overhead per request**:
- OpenAPI route matching: ~0.1ms
- Request validation: ~0.5-1ms
- **Total**: ~1-2ms per request

**For DataStorage** (low traffic):
- Current load: <100 req/s
- Impact: Negligible (<0.2% latency increase)
- **Verdict**: ✅ Acceptable

---

## 🚨 **Risks & Mitigations**

### **Risk 1: Spec Drift**

**Problem**: OpenAPI spec doesn't match actual handler behavior

**Mitigation**:
- ✅ Validate spec before deployment
- ✅ Add spec validation to CI/CD
- ✅ Keep spec as single source of truth
- ✅ Document spec update process

### **Risk 2: Too Strict Validation**

**Problem**: Middleware rejects valid requests

**Mitigation**:
- ✅ Make middleware optional (config flag)
- ✅ Log validation failures for monitoring
- ✅ Add spec relaxation where needed
- ✅ Gradual rollout (test → staging → prod)

### **Risk 3: Missing Business Rules**

**Problem**: Custom validation removed by mistake

**Mitigation**:
- ✅ Keep list of custom validations
- ✅ Test all edge cases
- ✅ Compare before/after validation behavior

### **Risk 4: Breaking Changes**

**Problem**: Existing clients break due to stricter validation

**Mitigation**:
- ✅ Test with existing integration/E2E tests
- ✅ Add `minLength: 1` carefully
- ✅ Monitor error rates after deployment
- ✅ Have rollback plan (disable middleware)

---

## 📋 **Rollout Plan**

### **Phase A: Development** (Day 1)
1. Add dependencies
2. Create middleware package
3. Write unit tests
4. Update OpenAPI spec

### **Phase B: Integration** (Day 1-2)
1. Add middleware to server
2. Update configuration
3. Run integration tests
4. Fix any failures

### **Phase C: Cleanup** (Day 2)
1. Remove redundant validation
2. Update handler code
3. Run full test suite
4. Document changes

### **Phase D: Validation** (Day 2)
1. Run all 3 test tiers
2. Manual testing
3. Performance testing
4. Documentation update

### **Phase E: Deployment** (Day 3+)
1. Deploy to test environment
2. Monitor for errors
3. Deploy to staging
4. Deploy to production
5. Monitor metrics

---

## ✅ **Success Criteria**

| Criterion | Target |
|-----------|--------|
| **Unit tests** | 100% pass |
| **Integration tests** | 149/149 pass |
| **E2E tests** | 100% pass |
| **Code reduction** | -60 lines (validation helpers) |
| **Performance impact** | <2ms latency increase |
| **Spec compliance** | 100% |

---

## 📚 **Documentation Updates Needed**

1. ✅ **Architecture docs**: Add middleware validation to data flow
2. ✅ **API docs**: Document validation behavior
3. ✅ **Development guide**: How to update OpenAPI spec
4. ✅ **Troubleshooting**: Common validation errors
5. ✅ **Migration guide**: For future services

---

## 🎯 **Recommendation**

### **✅ PROCEED with Implementation**

**Confidence**: 90%

**Rationale**:
1. ✅ Chi router is compatible
2. ✅ OpenAPI spec is well-defined
3. ✅ Clear implementation path
4. ✅ Acceptable performance impact
5. ✅ Reduces maintenance burden
6. ✅ Improves spec compliance

**Estimated Timeline**: 2-3 days (including testing)

**Next Steps**:
1. Get approval for dependency addition
2. Start with Phase 1 (setup dependencies)
3. Create middleware package
4. Test thoroughly
5. Roll out gradually

---

## 📊 **Effort Breakdown**

| Phase | Tasks | Time |
|-------|-------|------|
| **1. Setup** | Dependencies, spec validation | 30 min |
| **2. Middleware** | Package creation, tests | 1.5 hrs |
| **3. Integration** | Server setup, config | 30 min |
| **4. Cleanup** | Remove manual validation | 1.5 hrs |
| **5. Spec Update** | Add constraints | 30 min |
| **6. Testing** | All 3 tiers | 2 hrs |
| **7. Documentation** | Update docs | 1 hr |
| **TOTAL** | | **6-8 hours** |

**Buffer**: Add 2 hours for unexpected issues

**Total Estimate**: **1-2 days**

---

**Document Status**: ✅ Triage Complete
**Ready for Implementation**: ✅ Yes
**Blockers**: None
**Dependencies**: `kin-openapi` library approval

