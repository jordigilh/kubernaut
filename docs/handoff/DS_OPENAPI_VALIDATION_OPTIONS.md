# DataStorage - OpenAPI Validation Options Analysis

**Date**: 2025-12-13
**Context**: Addressing empty string validation for required fields
**Current Solution**: Manual validation (implemented and working)

---

## 🎯 **Problem Statement**

Go's JSON unmarshaling doesn't enforce OpenAPI's "required" constraint for empty strings:

```go
// Missing "event_type" in JSON
{"version": "1.0", "event_category": "gateway"}

// After json.Unmarshal():
req.EventType = ""  // Empty string (zero value), NOT an error
```

**Question**: How should we handle this across the codebase?

---

## ✅ **Option 1: Manual Validation** ⭐ **CURRENT (Recommended)**

### **Implementation**

**File**: `pkg/datastorage/server/helpers/openapi_conversion.go`

```go
func ValidateAuditEventRequest(req *dsclient.AuditEventRequest) error {
    // Validate required fields are not empty
    requiredFields := map[string]string{
        "version":        req.Version,
        "event_type":     req.EventType,
        "event_category": req.EventCategory,
        "event_action":   req.EventAction,
        "correlation_id": req.CorrelationId,
    }

    for field, value := range requiredFields {
        if value == "" {
            return fmt.Errorf("%s is required and cannot be empty", field)
        }
    }

    return nil
}
```

### **Pros**
- ✅ **Already implemented and tested** (100% pass rate)
- ✅ Clear and maintainable
- ✅ Custom error messages per field
- ✅ No additional dependencies
- ✅ Minimal code (~15 lines per validator)
- ✅ Fine-grained control over validation logic
- ✅ Can add custom business rules easily

### **Cons**
- ⚠️ Manual updates if OpenAPI spec changes required fields
- ⚠️ Needs to be applied to each handler/endpoint
- ⚠️ Potential for inconsistency across handlers

### **When to Use**
- ✅ Small to medium number of endpoints
- ✅ Need custom validation logic beyond OpenAPI spec
- ✅ Want explicit control over error messages
- ✅ **Current DataStorage use case** (few endpoints, custom validation)

### **Maintenance**
**Pattern to follow for new endpoints**:

1. Create validation function in `pkg/datastorage/server/helpers/validation.go`
2. List all required string fields
3. Check for empty strings
4. Return descriptive errors

---

## 🔧 **Option 2: OpenAPI Middleware Validation**

### **Implementation**

**Add middleware using `kin-openapi`**:

```go
// pkg/datastorage/server/middleware/openapi.go
package middleware

import (
    "net/http"

    "github.com/getkin/kin-openapi/openapi3"
    "github.com/getkin/kin-openapi/openapi3filter"
    "github.com/getkin/kin-openapi/routers"
    "github.com/getkin/kin-openapi/routers/gorillamux"
)

func OpenAPIValidation(specPath string) func(http.Handler) http.Handler {
    // Load OpenAPI spec
    loader := openapi3.NewLoader()
    doc, err := loader.LoadFromFile(specPath)
    if err != nil {
        panic(fmt.Sprintf("Failed to load OpenAPI spec: %v", err))
    }

    // Create router for matching requests to operations
    router, err := gorillamux.NewRouter(doc)
    if err != nil {
        panic(fmt.Sprintf("Failed to create router: %v", err))
    }

    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Find the operation for this request
            route, pathParams, err := router.FindRoute(r)
            if err != nil {
                // Not in OpenAPI spec, pass through
                next.ServeHTTP(w, r)
                return
            }

            // Validate request against OpenAPI spec
            requestValidationInput := &openapi3filter.RequestValidationInput{
                Request:    r,
                PathParams: pathParams,
                Route:      route,
            }

            if err := openapi3filter.ValidateRequest(r.Context(), requestValidationInput); err != nil {
                // Validation failed - return 400
                http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
                return
            }

            // Validation passed, proceed to handler
            next.ServeHTTP(w, r)
        })
    }
}

// In server.go
func (s *Server) setupRoutes() {
    // Apply OpenAPI validation middleware to all routes
    r.Use(middleware.OpenAPIValidation("api/openapi/data-storage-v1.yaml"))

    // Define routes
    r.Post("/audit/events", s.handleCreateAuditEvent)
    // ...
}
```

### **Pros**
- ✅ **Automatic validation** for all endpoints
- ✅ Validates against OpenAPI spec (single source of truth)
- ✅ Catches empty required fields automatically
- ✅ Validates types, formats, patterns, enums
- ✅ No manual validation code per endpoint
- ✅ Spec changes automatically reflected

### **Cons**
- ⚠️ **Additional dependency** (`kin-openapi` ~500KB)
- ⚠️ Generic error messages (less user-friendly)
- ⚠️ Performance overhead (parsing spec on every request)
- ⚠️ May be overkill for simple services
- ⚠️ Harder to customize validation logic
- ⚠️ Requires router integration (gorilla/mux, chi, etc.)

### **When to Use**
- ✅ Large number of endpoints
- ✅ Want to enforce OpenAPI spec strictly
- ✅ Minimal custom validation needed
- ✅ Prefer automated validation over manual code

### **Estimated Effort**
- Initial setup: 2-3 hours
- Per-endpoint validation removal: 5-10 minutes each
- Testing: 1-2 hours

---

## 🛠️ **Option 3: Generate Validation Code**

### **Implementation**

**Use `oapi-codegen` with validation generation**:

```bash
# Generate types + validation functions
oapi-codegen \
  --generate types,validation \
  --package client \
  api/openapi/data-storage-v1.yaml > pkg/datastorage/client/generated.go
```

**Generated code example**:
```go
// Auto-generated validation method
func (r *AuditEventRequest) Validate() error {
    if r.Version == "" {
        return fmt.Errorf("field 'version' is required")
    }
    if r.EventType == "" {
        return fmt.Errorf("field 'event_type' is required")
    }
    // ... for all required fields

    // Validate enum values
    if r.EventOutcome != "success" && r.EventOutcome != "failure" && r.EventOutcome != "pending" {
        return fmt.Errorf("invalid value for event_outcome")
    }

    return nil
}

// In handler
func (s *Server) handleCreateAuditEvent(w http.ResponseWriter, r *http.Request) {
    var req dsclient.AuditEventRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        // handle error
    }

    // Auto-generated validation
    if err := req.Validate(); err != nil {
        writeRFC7807Error(w, http.StatusBadRequest, "validation_error", err.Error())
        return
    }

    // Proceed with validated request
}
```

### **Pros**
- ✅ **Generated from OpenAPI spec** (single source of truth)
- ✅ Automatic updates when spec changes
- ✅ No runtime overhead (compile-time generation)
- ✅ Type-safe validation methods
- ✅ Can customize generated code if needed

### **Cons**
- ⚠️ **Not currently supported** by oapi-codegen v2.5.1
- ⚠️ Would need to use alternative tool (e.g., `go-swagger`)
- ⚠️ Generated code may be verbose
- ⚠️ Less control over error messages
- ⚠️ Regeneration step in build process

### **When to Use**
- ✅ Want automated validation tied to OpenAPI spec
- ✅ Prefer compile-time generation over runtime validation
- ✅ Willing to use alternative code generator

### **Estimated Effort**
- Tool evaluation: 2-4 hours
- Migration: 4-6 hours
- Testing: 2-3 hours

---

## 📋 **Option 4: Struct Tags + Validator Library**

### **Implementation**

**Use a validator library like `go-playground/validator`**:

```go
import "github.com/go-playground/validator/v10"

// Modify OpenAPI spec to generate validation tags
// OR manually add tags to generated structs (not recommended)

type AuditEventRequest struct {
    Version       string `json:"version" validate:"required,min=1"`
    EventType     string `json:"event_type" validate:"required,min=1"`
    EventCategory string `json:"event_category" validate:"required,min=1"`
    EventAction   string `json:"event_action" validate:"required,min=1"`
    CorrelationId string `json:"correlation_id" validate:"required,uuid"`
}

// In handler
var validate = validator.New()

func (s *Server) handleCreateAuditEvent(w http.ResponseWriter, r *http.Request) {
    var req AuditEventRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        // handle error
    }

    if err := validate.Struct(req); err != nil {
        // Convert validator errors to RFC 7807
        writeRFC7807Error(w, http.StatusBadRequest, "validation_error", err.Error())
        return
    }
}
```

### **Pros**
- ✅ Declarative validation (struct tags)
- ✅ Rich validation rules (min, max, email, uuid, etc.)
- ✅ Widely used library (proven)
- ✅ Good performance

### **Cons**
- ⚠️ **Cannot modify generated OpenAPI structs** (regeneration would lose tags)
- ⚠️ Would need wrapper structs (duplication)
- ⚠️ Additional dependency
- ⚠️ Generic error messages

### **When to Use**
- ❌ **NOT recommended for OpenAPI-generated types**
- ✅ Good for internal/custom types only

---

## 🎯 **Recommendation Matrix**

| Scenario | Recommended Option | Rationale |
|----------|-------------------|-----------|
| **DataStorage (current)** | **Option 1: Manual** ⭐ | Small service, already implemented, custom validation needed |
| **New microservice (<10 endpoints)** | **Option 1: Manual** | Simplicity, control, no dependencies |
| **Large API (20+ endpoints)** | **Option 2: Middleware** | Automation worth the overhead |
| **Strict OpenAPI compliance** | **Option 2: Middleware** | Enforces spec automatically |
| **Need custom business rules** | **Option 1: Manual** | Fine-grained control |

---

## ✅ **DataStorage Decision: Keep Option 1**

### **Rationale**

1. ✅ **Already implemented and tested** (100% pass rate)
2. ✅ **Small service** (6 endpoints)
3. ✅ **Custom validation needed** (timestamp bounds, field lengths)
4. ✅ **No additional dependencies**
5. ✅ **Clear and maintainable**

### **Going Forward**

**Pattern to Follow**:
1. For each new endpoint that accepts requests:
   - Create a `Validate[TypeName]Request()` function
   - Check required string fields for empty values
   - Add custom business validation
   - Return descriptive errors

2. Document in handler:
```go
// handleNewEndpoint handles POST /api/v1/new-endpoint
// BR-XXX: Business requirement reference
func (s *Server) handleNewEndpoint(w http.ResponseWriter, r *http.Request) {
    var req dsclient.NewRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        // handle decode error
    }

    // Validate request (including empty required fields)
    if err := helpers.ValidateNewRequest(&req); err != nil {
        writeRFC7807Error(w, http.StatusBadRequest, "validation_error", err.Error())
        return
    }

    // Proceed with validated request
}
```

3. Add validation helper to `pkg/datastorage/server/helpers/validation.go`

---

## 📚 **For Future Services**

If we build a **larger API service** (20+ endpoints), **revisit Option 2** (middleware):

### **When to Switch**
- More than 15-20 endpoints
- Minimal custom validation needed
- Want strict OpenAPI compliance

### **Migration Path**
1. Add `kin-openapi` dependency
2. Create middleware package
3. Apply to router
4. Remove manual validation functions
5. Test all endpoints

**Estimated effort**: 1-2 days

---

## 🔗 **References**

- **OpenAPI Validation**: https://github.com/getkin/kin-openapi
- **oapi-codegen**: https://github.com/oapi-codegen/oapi-codegen
- **Current Implementation**: `pkg/datastorage/server/helpers/openapi_conversion.go`
- **ADR-034**: Unified Audit Table Design (canonical field names)

---

**Document Status**: ✅ Complete
**Decision**: **Keep Option 1 (Manual Validation)**
**Confidence**: 95%

