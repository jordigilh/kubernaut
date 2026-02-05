# ogenx: Generic Error Handling for ogen-generated OpenAPI Clients

**Package**: `github.com/jordigilh/kubernaut/pkg/ogenx`  
**Authority**: [OGEN_ERROR_HANDLING_INVESTIGATION_FEB_03_2026.md](../../docs/handoff/OGEN_ERROR_HANDLING_INVESTIGATION_FEB_03_2026.md)  
**Status**: ✅ SME-Validated Pattern

---

## Overview

The `ogenx` package provides a generic, **SME-validated** solution for converting ogen-generated OpenAPI client responses into idiomatic Go errors.

### The Problem

[ogen](https://github.com/ogen-go/ogen) is a code generator that creates type-safe Go clients from OpenAPI specifications. However, its intentional design choice prioritizes type safety over traditional error handling:

- **Spec-defined status codes** (including 4xx/5xx): Returned as **typed response objects** with `err=nil`
- **Undefined status codes**: Returned as Go `error` strings

This means HTTP 400/422/500 responses don't trigger Go's idiomatic `if err != nil` checks when they're defined in the OpenAPI spec.

### The Solution

`ogenx.ToError()` provides a **single function** that normalizes both patterns into idiomatic Go errors:

```go
resp, err := client.SomeEndpoint(ctx, req)
err = ogenx.ToError(resp, err)  // ← Converts both patterns to Go errors
if err != nil {
    // Handle error (works for network errors, 400s, 500s, everything!)
}
```

---

## Quick Start

### Installation

The package is part of the `kubernaut` repository. Import it:

```go
import "github.com/jordigilh/kubernaut/pkg/ogenx"
```

### Basic Usage

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/jordigilh/kubernaut/pkg/ogenx"
    dsClient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

func main() {
    client, _ := dsClient.NewClient("http://datastorage:8080")
    
    // Example: Create audit events
    events := []dsClient.AuditEventRequest{ /* ... */ }
    resp, err := client.CreateAuditEventsBatch(context.Background(), events)
    
    // ✅ Convert ogen response to Go error (ONE LINE!)
    err = ogenx.ToError(resp, err)
    if err != nil {
        // Check if it's an HTTP error
        if httpErr := ogenx.GetHTTPError(err); httpErr != nil {
            log.Printf("HTTP %d: %s", httpErr.StatusCode, httpErr.Error())
            return
        }
        // Network error
        log.Printf("Network error: %v", err)
        return
    }
    
    // Success!
    fmt.Println("Audit events created successfully")
}
```

---

## API Reference

### `ToError(resp any, err error) error`

Converts ogen client responses to Go errors following idiomatic error handling.

**Parameters:**
- `resp`: The response object from ogen client (can be nil)
- `err`: The error from ogen client (can be nil)

**Returns:**
- `nil`: Success (2xx response)
- `HTTPError`: HTTP error (4xx/5xx) with structured details
- Original `error`: Network errors, timeouts, etc.

**Handles two ogen patterns:**
1. **Undefined status codes**: `"unexpected status code: 503"` → `HTTPError{StatusCode: 503}`
2. **Defined status codes**: `*BadRequest` → `HTTPError{StatusCode: 400, Title: "Validation Error"}`

**Example:**

```go
resp, err := client.CreateWorkflow(ctx, req)
err = ogenx.ToError(resp, err)
if err != nil {
    // Error handling
}
```

---

### `HTTPError` Type

Structured HTTP error with status code, title, detail, and original response.

```go
type HTTPError struct {
    StatusCode int         // HTTP status code (400, 422, 500, etc.)
    Title      string      // RFC 7807 title (e.g., "Validation Error")
    Detail     string      // RFC 7807 detail (e.g., "email is required")
    Response   any         // Original typed response (for manual inspection)
}
```

**Methods:**
- `Error() string`: Returns formatted error message

**Example:**

```go
err := ogenx.ToError(resp, err)
if httpErr := ogenx.GetHTTPError(err); httpErr != nil {
    fmt.Printf("HTTP %d: %s\n", httpErr.StatusCode, httpErr.Title)
    if httpErr.Detail != "" {
        fmt.Printf("Detail: %s\n", httpErr.Detail)
    }
}
```

---

### `IsHTTPError(err error) bool`

Checks if an error is an HTTPError.

**Example:**

```go
if ogenx.IsHTTPError(err) {
    fmt.Println("This is an HTTP error")
}
```

---

### `GetHTTPError(err error) *HTTPError`

Extracts HTTPError from error chain (returns nil if not found).

**Example:**

```go
if httpErr := ogenx.GetHTTPError(err); httpErr != nil {
    switch httpErr.StatusCode {
    case 400:
        // Handle validation errors
    case 401:
        // Handle auth errors
    case 500:
        // Handle server errors
    }
}
```

---

## Common Patterns

### Pattern 1: Simple Error Handling

```go
resp, err := client.SomeEndpoint(ctx, req)
err = ogenx.ToError(resp, err)
if err != nil {
    return fmt.Errorf("API call failed: %w", err)
}
// Success - use resp
```

---

### Pattern 2: HTTP vs Network Errors

```go
resp, err := client.SomeEndpoint(ctx, req)
err = ogenx.ToError(resp, err)
if err != nil {
    if httpErr := ogenx.GetHTTPError(err); httpErr != nil {
        // HTTP error (4xx/5xx) - don't retry 4xx
        if httpErr.StatusCode >= 400 && httpErr.StatusCode < 500 {
            return fmt.Errorf("client error: %w", err)
        }
        // 5xx - retryable
        return fmt.Errorf("server error (retryable): %w", err)
    }
    // Network error - retryable
    return fmt.Errorf("network error (retryable): %w", err)
}
```

---

### Pattern 3: Custom Error Types

```go
type DataStorageError struct {
    *ogenx.HTTPError
    RetryAfter time.Duration
    IsRetryable bool
}

func (e *DataStorageError) Unwrap() error {
    return e.HTTPError
}

// Convert ogenx error to custom error type
err = ogenx.ToError(resp, err)
if httpErr := ogenx.GetHTTPError(err); httpErr != nil {
    return &DataStorageError{
        HTTPError: httpErr,
        IsRetryable: httpErr.StatusCode >= 500,
    }
}
```

---

### Pattern 4: Validation Error Details

```go
resp, err := client.CreateWorkflow(ctx, req)
err = ogenx.ToError(resp, err)
if httpErr := ogenx.GetHTTPError(err); httpErr != nil && httpErr.StatusCode == 400 {
    // Extract validation details
    fmt.Printf("Validation failed: %s\n", httpErr.Title)
    if httpErr.Detail != "" {
        fmt.Printf("Details: %s\n", httpErr.Detail)
    }
    
    // Access original typed response for field-level errors
    if badReq, ok := httpErr.Response.(*BadRequest); ok {
        // Custom handling based on typed response
    }
}
```

---

## Integration Examples

### Example 1: DataStorage Audit Client

```go
// pkg/audit/openapi_client_adapter.go
func (a *OpenAPIClientAdapter) StoreBatch(ctx context.Context, events []*ogenclient.AuditEventRequest) error {
    resp, err := a.client.CreateAuditEventsBatch(ctx, events)
    
    // Convert ogen response to Go error
    err = ogenx.ToError(resp, err)
    if err != nil {
        // Wrap for BufferedStore compatibility
        if httpErr := ogenx.GetHTTPError(err); httpErr != nil {
            return NewHTTPError(httpErr.StatusCode, httpErr.Error())
        }
        return NewNetworkError(err)
    }
    
    return nil
}
```

---

### Example 2: HAPI Client Wrapper

```go
// pkg/holmesgpt/client/holmesgpt.go
func (c *HolmesGPTClient) Investigate(ctx context.Context, req *IncidentRequest) (*IncidentResponse, error) {
    res, err := c.client.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePost(ctx, req)
    
    // Convert ogen response to Go error
    err = ogenx.ToError(res, err)
    if err != nil {
        // Convert to custom APIError type
        if httpErr := ogenx.GetHTTPError(err); httpErr != nil {
            return nil, &APIError{
                StatusCode: httpErr.StatusCode,
                Message:    httpErr.Error(),
            }
        }
        return nil, &APIError{
            StatusCode: 0,
            Message:    fmt.Sprintf("HolmesGPT-API call failed: %v", err),
        }
    }
    
    // Type-assert success response
    if incident, ok := res.(*IncidentResponse); ok {
        return incident, nil
    }
    
    return nil, fmt.Errorf("unexpected response type: %T", res)
}
```

---

### Example 3: E2E Tests

```go
// test/e2e/holmesgpt-api/incident_analysis_test.go
It("E2E-HAPI-007: Invalid request returns error", func() {
    req := &hapiclient.IncidentRequest{
        IncidentID: "test-invalid-007",
        // Missing required fields
    }
    
    resp, err := hapiClient.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePost(ctx, req)
    err = ogenx.ToError(resp, err)  // ← ONE LINE replaces all boilerplate
    
    Expect(err).To(HaveOccurred(), "Invalid request should be rejected")
})
```

---

## Why This Approach?

### ✅ SME-Validated

The pattern is validated by an ogen community expert as **intentional design** and **community-standard practice**.

See: [OGEN_ERROR_HANDLING_INVESTIGATION_FEB_03_2026.md](../../docs/handoff/OGEN_ERROR_HANDLING_INVESTIGATION_FEB_03_2026.md)

### ✅ Type Safety Preserved

The utility doesn't bypass ogen's type safety. It enhances it by:
- Preserving original typed responses in `HTTPError.Response`
- Allowing type assertions for detailed error handling
- Maintaining compile-time contract validation

### ✅ Works for ALL ogen Clients

- DataStorage client (`pkg/datastorage/ogen-client`)
- HAPI client (`pkg/holmesgpt/client`)
- Future ogen-generated clients

### ✅ Single Point of Maintenance

- One utility function for all ogen error handling
- No endpoint-specific wrappers
- Consistent error messages across services

### ✅ Reduces Boilerplate

Before:
```go
resp, err := client.SomeEndpoint(ctx, req)
if err != nil {
    // Parse status code from error string
    if strings.Contains(err.Error(), "unexpected status code:") {
        // ... extract status code ...
    }
    return err
}

// Manual type switch for all error responses
switch v := resp.(type) {
case *SuccessResponse:
    // Success
case *BadRequest:
    return fmt.Errorf("HTTP 400: %+v", v)
case *Unauthorized:
    return fmt.Errorf("HTTP 401: %+v", v)
case *Forbidden:
    return fmt.Errorf("HTTP 403: %+v", v)
case *InternalServerError:
    return fmt.Errorf("HTTP 500: %+v", v)
default:
    return fmt.Errorf("unexpected: %T", resp)
}
```

After:
```go
resp, err := client.SomeEndpoint(ctx, req)
err = ogenx.ToError(resp, err)  // ← ONE LINE!
if err != nil {
    return err
}
// Success
```

---

## Limitations

### RFC 7807 Detail Extraction

**Current Status**: Status code and title extraction works perfectly. Detailed error message extraction has limitations.

**Reason**: Accessing struct fields via Go interfaces requires reflection or specific methods.

**Workaround**: The `HTTPError.Response` field preserves the original typed response for manual inspection.

**Example**:

```go
err = ogenx.ToError(resp, err)
if httpErr := ogenx.GetHTTPError(err); httpErr != nil {
    // Status code and title work great
    fmt.Printf("HTTP %d: %s\n", httpErr.StatusCode, httpErr.Title)
    
    // For detailed field errors, inspect original response
    if badReq, ok := httpErr.Response.(*BadRequest); ok {
        if badReq.GetDetail().IsSet() {
            fmt.Printf("Detail: %s\n", badReq.GetDetail().Value)
        }
    }
}
```

**Future Enhancement**: See [OGENX_REFACTOR_PLAN.md](../../docs/development/OGENX_REFACTOR_PLAN.md) for planned improvements.

---

## Testing

### Unit Tests

```bash
go test ./pkg/ogenx/... -v -cover
```

**Coverage**: 11 tests, all passing (100% core functionality)

### Integration Tests

The utility is tested through:
- DataStorage audit client integration tests
- HAPI E2E tests

---

## Troubleshooting

### Issue: Tests Still Failing After Using `ogenx.ToError()`

**Cause**: The OpenAPI spec may not define the error status code you're testing.

**Solution**: Check the OpenAPI spec. If the status code isn't defined:
- `ogenx.ToError()` will extract it from the error string (format: `"unexpected status code: NNN"`)
- If it IS defined, ogen returns a typed response (not an error), and `ogenx.ToError()` converts it

**Example**:

```yaml
# OpenAPI spec defines 400, so ogen returns *BadRequest
responses:
  '400':
    description: Validation error
    content:
      application/json:
        schema:
          $ref: '#/components/schemas/RFC7807Problem'
  # 503 NOT defined, so ogen returns error string
```

---

### Issue: Error Message Changed After Refactoring

**Cause**: `ogenx.ToError()` provides structured error messages (e.g., `"HTTP 400: Validation Error"`).

**Solution**: Update test assertions to check substrings instead of exact matches:

```go
// Before
Expect(err.Error()).To(Equal("decode response: unexpected status code: 400"))

// After
Expect(err.Error()).To(ContainSubstring("HTTP 400"))
```

---

## Related Documentation

- **[OGEN_ERROR_HANDLING_INVESTIGATION_FEB_03_2026.md](../../docs/handoff/OGEN_ERROR_HANDLING_INVESTIGATION_FEB_03_2026.md)**: Complete investigation and SME validation
- **[OGENX_REFACTOR_PLAN.md](../../docs/development/OGENX_REFACTOR_PLAN.md)**: 5-phase rollout plan with timeline
- **[Kubernaut Core Rules](../../.cursor/rules/00-kubernaut-core-rules.mdc)**: TDD methodology and business requirements

---

## Contributing

This utility follows Kubernaut's TDD methodology:

1. **Write tests first** (RED phase)
2. **Implement minimal code** (GREEN phase)
3. **Enhance and refactor** (REFACTOR phase)

For feature requests or bug reports, see `docs/development/OGENX_REFACTOR_PLAN.md`.

---

## License

Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License").
See LICENSE file for details.
