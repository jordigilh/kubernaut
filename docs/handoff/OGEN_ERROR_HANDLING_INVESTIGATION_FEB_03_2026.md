# ogen OpenAPI Client: HTTP Error Handling Behavior

**Date**: February 3, 2026  
**Updated**: February 3, 2026 (SME Validation Added)  
**Context**: Investigation into ogen-generated Go client error handling  
**Tool**: [ogen](https://github.com/ogen-go/ogen) v1.18.0 - OpenAPI v3 code generator for Go  
**Question**: How does ogen handle HTTP 4xx/5xx error responses?  
**Status**: ✅ **VALIDATED BY EXTERNAL SME** - Behavior confirmed as intentional design

---

## TL;DR

**Issue**: ogen-generated Go clients return HTTP 400/422 error responses as typed objects with `err=nil`, not as Go errors.

**Why**: ogen treats **all spec-defined status codes** (including errors) as valid response types. Only undefined status codes return errors.

**Impact**: Test code like `if err != nil` doesn't catch validation errors, even though server returns HTTP 400 correctly.

**Solution**: Use error-checking wrapper to convert typed error responses to Go errors:
```go
resp, err := client.Call(ctx, req)
err = CheckError(resp, err)  // ← Wrapper converts typed errors to Go errors
if err != nil {              // ✅ Now works!
    // Handle error
}
```

**Trade-off**: Wrapper adds boilerplate but preserves OpenAPI type safety. Alternative is removing error definitions from spec (loses type safety).

**SME Validation**: ✅ Confirmed by ogen expert as **intentional design** and **community-standard practice**. Wrapper pattern is field-proven and recommended.

---

## SME Validation (February 3, 2026)

### Expert Confirmation

An external SME with deep ogen expertise has validated this investigation:

✅ **Behavior Analysis**: Fully correct - ogen treats all spec-defined status codes as "expected responses"

✅ **Design Intent**: Intentional philosophical choice by ogen maintainers ([@kamilsk](https://github.com/kamilsk), [@scop](https://github.com/scop))
- Emphasizes **type-safety-first** model over Go idioms
- "Defined === Expected" regardless of HTTP semantics
- Strict adherence to OpenAPI contract

✅ **No Configuration Available**: As of v1.18.0, no CLI flag or config option to treat 4xx/5xx as errors

✅ **Community Practice**: Error-wrapper pattern is **standard among ogen users** and field-proven

✅ **Comparison to Other Generators**: 
- `oapi-codegen` treats 4xx/5xx as errors by default
- ogen's approach is intentionally different

### Key SME Insights

**Why ogen doesn't treat 4xx/5xx as errors**:
> "ogen aligns with the OpenAPI spec semantics, not idiomatic Go expectations... It considers [defined status codes] 'expected,' so their decoder branches return (typedResponse, nil)... This design reflects ogen's strict adherence to the API contract as described in the OpenAPI document."

**On the wrapper pattern**:
> "You're already following the standard, field-proven approach: wrapping ogen's typed responses in small, domain-focused helpers... This preserves type safety and remains fully compatible with ogen updates."

**Future improvements**:
> "There are discussions around improved error ergonomics (tracked near issues like #1224, #1266, #1478). Filing a feature request like 'Option: treat non-2xx defined responses as Go errors' would be reasonable."

### Recommended Enhancements

The SME suggested a **generic utility pattern** to reduce boilerplate:

```go
package ogenx

import "fmt"

// NormalizeError converts typed ogen responses to Go errors if applicable.
func NormalizeError(resp any, err error) error {
    if err != nil {
        return err
    }
    
    // Use interface to detect responses with status codes
    type statusGetter interface {
        GetStatus() int
    }
    
    switch v := resp.(type) {
    case statusGetter:
        if code := v.GetStatus(); code >= 400 {
            return fmt.Errorf("HTTP %d: %+v", code, v)
        }
    }
    return nil
}
```

**Usage**:
```go
resp, err := client.SomeEndpoint(ctx, req)
err = ogenx.NormalizeError(resp, err)  // ← Generic wrapper
if err != nil {
    // Handle error
}
```

**Benefits**:
- ✅ Works for all endpoints automatically
- ✅ Single point of maintenance
- ✅ Consistent error handling across codebase
- ✅ Still allows access to typed response details when needed

---

## Executive Summary

**Problem**: When testing a REST API with ogen-generated Go clients, HTTP 400/422 validation errors are not returned as Go `error` values. Instead, they're returned as typed response objects with `err=nil`.

**Root Cause**: ogen's design treats **all OpenAPI spec-defined status codes** (including error codes like 400, 422, 500) as valid response types, not errors. Only status codes **not defined in the spec** return Go errors.

**Impact**: Test code expecting `err != nil` for validation errors fails, even though the server is correctly returning HTTP 400 responses with proper error payloads.

**Options**: 
1. ✅ Use error-checking wrapper to convert typed error responses to Go errors
2. ❌ Remove error status codes from OpenAPI spec (loses type safety)
3. 🤔 Seek ogen configuration or alternative approach (SME input needed)

---

## The Problem in Detail

### Context

**Scenario**: Testing a REST API that validates input and returns HTTP 400 for invalid requests.

**Server Side** (Python/FastAPI):
- API validates requests using Pydantic models
- Returns HTTP 400 Bad Request for validation failures
- Includes RFC 7807 problem details in response body
- Server logs show correct 400 responses being sent

**Client Side** (Go with ogen-generated client):
```go
// Test code expecting validation error
invalidRequest := &client.CreateUserRequest{
    Email: "",  // Invalid - empty email
}

resp, err := apiClient.CreateUser(ctx, invalidRequest)

// Expected: err != nil (validation error)
// Actual:   err == nil, resp = &CreateUserBadRequest{Status: 400, ...}
```

The test fails because `err` is `nil`, even though the server correctly returned HTTP 400.

### Investigation Approach

**Key Questions**:
1. Is this an ogen issue or a problem with our usage?
2. Do other ogen users experience this?
3. Is there a configuration option we're missing?

### Findings from Our Codebase

We have two services using ogen-generated clients:

#### Service A: Audit Storage API

This service has error-parsing logic in production code:

```go
resp, err := client.CreateAuditEventsBatch(ctx, events)
if err != nil {
    // Parse HTTP status code from ogen error if present
    // Ogen errors for non-2xx responses contain "unexpected status code: XXX"
    return parseOgenError(err)
}

**The error parser**:

```go
// parseOgenError extracts HTTP status code from ogen error strings
// Ogen error format: "decode response: unexpected status code: 503"
func parseOgenError(err error) error {
    errMsg := err.Error()
    
    if strings.Contains(errMsg, "unexpected status code:") {
        parts := strings.Split(errMsg, "unexpected status code:")
        if len(parts) == 2 {
            statusStr := strings.TrimSpace(parts[1])
            if statusCode, parseErr := strconv.Atoi(statusStr); parseErr == nil {
                return fmt.Errorf("HTTP %d error: %s", statusCode, errMsg)
            }
        }
    }
    
    return err  // Network error or other issue
}
```

**Why does this parser exist?** It suggests that ogen **sometimes** returns errors as strings.

---

## The Root Cause: ogen's Design Pattern

### How ogen Handles Status Codes

**Key Discovery**: ogen handles HTTP responses **differently** based on whether the status code is **defined in your OpenAPI specification**.

Examining the generated decoder code reveals the pattern:

```go
// Generated by ogen from OpenAPI spec
func decodeCreateUserResponse(resp *http.Response) (res CreateUserRes, _ error) {
    switch resp.StatusCode {
    case 201:  // ✅ Defined in spec
        // Parse success response
        return &User{}, nil
        
    case 400:  // ✅ Defined in spec
        // Parse error response body
        var errorResp CreateUserBadRequest  // RFC 7807 problem details
        // ... decode JSON ...
        return &errorResp, nil  // ← err is nil!
        
    case 422:  // ✅ Defined in spec
        var errorResp CreateUserUnprocessableEntity
        // ... decode JSON ...
        return &errorResp, nil  // ← err is nil!
        
    case 500:  // ✅ Defined in spec
        var errorResp CreateUserInternalServerError
        // ... decode JSON ...
        return &errorResp, nil  // ← err is nil!
    }
    
    // ❌ Status code NOT in spec (e.g., 503, 429)
    return nil, validate.UnexpectedStatusCodeWithResponse(resp)  // ← Returns error!
}
```

### Behavior Matrix

| HTTP Status | Defined in OpenAPI Spec? | ogen Response | Go `error` Value | Can Test with `err != nil`? |
|-------------|--------------------------|---------------|------------------|------------------------------|
| 200/201 | ✅ Yes | Success object | `nil` | ✅ N/A (success case) |
| 400 | ✅ Yes | `*BadRequest` | `nil` ❌ | ❌ No |
| 422 | ✅ Yes | `*UnprocessableEntity` | `nil` ❌ | ❌ No |
| 500 | ✅ Yes | `*InternalServerError` | `nil` ❌ | ❌ No |
| 503 | ❌ No | `nil` | `"unexpected status code: 503"` ✅ | ✅ Yes |
| 429 | ❌ No | `nil` | `"unexpected status code: 429"` ✅ | ✅ Yes |

**Key Insight**: Spec-defined error codes are treated as **valid, expected responses**, not errors.

---

## Why the Error Parser Works for Some Endpoints

### Example: Audit Batch Endpoint

**OpenAPI Spec** (simplified):
```yaml
/api/v1/audit/batch:
  post:
    responses:
      '201':
        description: Batch created successfully
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/BatchResponse'
      # ❌ NO 400/500 DEFINED!
```

**Result**: Any 4xx/5xx response is **undefined** → ogen returns error string → `parseOgenError()` works.

```go
resp, err := client.CreateAuditBatch(ctx, batch)
if err != nil {  // ✅ err = "unexpected status code: 500"
    return parseOgenError(err)  // Can parse status code from string
}
```

### Example: User Creation Endpoint (with RFC 7807)

**OpenAPI Spec** (following best practices):
```yaml
/api/v1/users:
  post:
    responses:
      '201':
        description: User created
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/User'
      '400':  # ✅ DEFINED (RFC 7807)
        description: Validation error
        content:
          application/problem+json:
            schema:
              $ref: '#/components/schemas/ProblemDetails'
      '500':  # ✅ DEFINED (RFC 7807)
        description: Server error
        content:
          application/problem+json:
            schema:
              $ref: '#/components/schemas/ProblemDetails'
```

**Result**: 400/500 are **defined** → ogen returns typed response objects → `err=nil`.

```go
resp, err := client.CreateUser(ctx, user)
if err != nil {  // ❌ err is nil, even for validation errors!
    return err   // This never executes for 400/500!
}

// Must check response type instead:
switch v := resp.(type) {
case *CreateUserBadRequest:
    // Handle validation error
case *CreateUserInternalServerError:
    // Handle server error
case *User:
    // Success
}
```

---

## The Dilemma: RFC 7807 vs. Test Expectations

### Following OpenAPI Best Practices Creates the Issue

**RFC 7807 (Problem Details for HTTP APIs)** recommends:
- Define error responses in OpenAPI spec
- Use structured error format
- Include machine-readable error details

**Example OpenAPI spec following RFC 7807**:
```yaml
/api/v1/users:
  post:
    responses:
      '400':
        description: Validation error
        content:
          application/problem+json:
            schema:
              type: object
              properties:
                type: {type: string}
                title: {type: string}
                status: {type: integer}
                detail: {type: string}
                field_errors: {type: object}
```

**This causes ogen to generate**:
```go
type CreateUserBadRequest struct {
    Type        string            `json:"type"`
    Title       string            `json:"title"`
    Status      int32             `json:"status"`
    Detail      string            `json:"detail"`
    FieldErrors map[string]string `json:"field_errors"`
}

func (CreateUserBadRequest) createUserRes() {}  // Implements response interface
```

**The contradiction**:
- ✅ **OpenAPI best practice**: Define error responses for type safety
- ❌ **Go test expectation**: `err != nil` for HTTP 4xx/5xx
- ❌ **ogen behavior**: Spec-defined = response type, not error

---

## Client Generation Configuration

**ogen version**: v1.18.0  
**Generation command**: `ogen --target . --package client --clean openapi.json`

**No special flags found for error handling** in ogen documentation or generated code.

---

## Verification: Server Behavior is Correct

### Server Logs Confirm Proper HTTP 400 Responses

**Request** (with validation error):
```http
POST /api/v1/users HTTP/1.1
Content-Type: application/json

{"email": ""}  ← Invalid: empty email
```

**Response**:
```http
HTTP/1.1 400 Bad Request
Content-Type: application/problem+json

{
  "type": "https://example.com/problems/validation-error",
  "title": "Validation Error",
  "status": 400,
  "detail": "email is required and cannot be empty",
  "field_errors": {
    "email": "required"
  }
}
```

**Server-side validation is working correctly**. The issue is purely in how the Go client interprets this response.

---

## Solution Options

### Option 1: Error-Checking Wrapper ✅ (Recommended)

**Approach**: Create a helper function that converts ogen typed error responses to Go errors.

**Implementation**:

```go
// Helper function to convert ogen response types to Go errors
func CheckAPIError(resp CreateUserRes, err error) error {
    // Network/transport errors (already Go errors)
    if err != nil {
        return err
    }
    
    // Check response type for HTTP error responses
    switch v := resp.(type) {
    case *CreateUserBadRequest:
        // Extract RFC 7807 problem details
        return fmt.Errorf("HTTP 400 Bad Request: %s (status=%d)", 
            v.Detail, v.Status)
            
    case *CreateUserUnprocessableEntity:
        return fmt.Errorf("HTTP 422 Unprocessable Entity: %s (status=%d)", 
            v.Detail, v.Status)
            
    case *CreateUserInternalServerError:
        return fmt.Errorf("HTTP 500 Internal Server Error: %s", 
            v.Detail)
            
    case *User:
        return nil  // Success response
        
    default:
        return fmt.Errorf("unexpected response type: %T", resp)
    }
}
```

**Usage in tests**:
```go
// Before (fails):
resp, err := apiClient.CreateUser(ctx, &invalidUser)
Expect(err).To(HaveOccurred())  // ❌ Fails - err is nil

// After (works):
resp, err := apiClient.CreateUser(ctx, &invalidUser)
err = CheckAPIError(resp, err)  // Convert typed error to Go error
Expect(err).To(HaveOccurred())  // ✅ Works!
```

**Pros**:
- ✅ Simple, maintainable solution
- ✅ No ogen reconfiguration required
- ✅ Preserves ogen's type safety benefits
- ✅ Can extract structured error details from RFC 7807 responses
- ✅ Works with existing OpenAPI specs

**Cons**:
- ❌ Requires updating test code
- ❌ Boilerplate for each endpoint (can template/generate)
- ❌ Not automatic - developers must remember to use it

### Option 2: Remove Error Responses from OpenAPI Spec ❌ (Not Recommended)

**Approach**: Remove 400/422/500 definitions from the OpenAPI spec so they become "unexpected" status codes.

**Modified spec**:
```yaml
/api/v1/users:
  post:
    responses:
      '201':
        description: User created
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/User'
      # ❌ Remove 400, 422, 500 definitions
```

**Result**: ogen would return `err = "unexpected status code: 400"` for validation errors.

**Pros**:
- ✅ `err != nil` works in tests automatically
- ✅ No wrapper code needed

**Cons**:
- ❌ **Violates RFC 7807 and OpenAPI best practices**
- ❌ **Loses type safety** - can't access structured error details
- ❌ **Breaking change** to API contract
- ❌ Client developers don't know what errors to expect
- ❌ Can't distinguish between 400, 422, 500 without string parsing
- ❌ Sacrifices the main benefit of using OpenAPI

**Verdict**: Not recommended. Defeats the purpose of using OpenAPI.

### Option 3: Alternative Approaches 🤔 (SME Input Needed)

**Questions for ogen experts or community**:

1. **Is there an ogen configuration flag** to treat spec-defined 4xx/5xx as errors?
   - Searched documentation: No evidence found
   - Examined generated code: No configuration hints
   - Default behavior appears to be by design

2. **Is this ogen's intended behavior?**
   - Generated code suggests **yes** - discriminated unions for responses
   - All spec-defined status codes are "valid" responses
   - Only undefined codes return errors

3. **Are there ogen templates/plugins** that modify this behavior?
   - Unknown - requires ogen community input

4. **Should we consider alternative OpenAPI generators?**
   - `oapi-codegen` - Popular Go OpenAPI generator
   - `go-swagger` - Mature, feature-rich
   - `openapi-generator` - Multi-language support
   - Each may handle errors differently

5. **Is the wrapper pattern standard practice** for ogen users?
   - Need community feedback
   - Our codebase uses it implicitly (for undefined status codes)
   - Seems to be the idiomatic solution

---

## Recommendations

### For Immediate Use

**Use Option 1: Error-checking wrapper**

1. ✅ Create helper functions for each endpoint response type
2. ✅ Update test code to use helpers
3. ✅ Document pattern for team members

**Example implementation**:
```go
// In test helpers package:
func CheckCreateUserError(resp CreateUserRes, err error) error {
    if err != nil {
        return err
    }
    switch v := resp.(type) {
    case *CreateUserBadRequest:
        return fmt.Errorf("validation error: %s", v.Detail)
    case *CreateUserInternalServerError:
        return fmt.Errorf("server error: %s", v.Detail)
    case *User:
        return nil
    default:
        return fmt.Errorf("unexpected response: %T", resp)
    }
}
```

### Questions for ogen Community/Experts

Before committing long-term to the wrapper pattern:

1. **Is this ogen's intended behavior?** 
   - By design or oversight?
   
2. **Is there a configuration option** to treat spec-defined errors as Go errors?
   - Documentation doesn't mention one
   - But worth confirming with maintainers
   
3. **What do other ogen users do?**
   - Is the wrapper pattern standard practice?
   - Any community-accepted solutions?
   
4. **Should we file a feature request?**
   - Option to make spec-defined 4xx/5xx return errors
   - Or clarification in documentation

5. **Are alternative generators worth evaluating?**
   - `oapi-codegen`: Different approach to error handling?
   - `go-swagger`: More mature, different patterns?
   - Trade-offs vs benefits?

---

## Test Pattern Comparison

### Pattern A: Direct Repository (Not HTTP)

**Works with standard Go error handling**:
```go
// Direct database/repository call (no HTTP layer)
result, err := repo.CreateUser(ctx, &user)
if err != nil {  // ✅ Works - repository returns Go errors
    t.Fatalf("expected success, got error: %v", err)
}
```

### Pattern B: HTTP Client with ogen (Spec-Defined Errors)

**Requires special handling**:
```go
// ogen-generated HTTP client
resp, err := client.CreateUser(ctx, &invalidUser)
if err != nil {  // ❌ FAILS - err is nil for spec-defined status codes!
    t.Fatalf("expected validation error, got nil")
}

// Must check response type:
switch v := resp.(type) {
case *CreateUserBadRequest:
    // ✅ This is the validation error
    t.Logf("Got expected validation error: %s", v.Detail)
case *User:
    t.Fatalf("expected validation error, got success")
}
```

### Pattern B (Fixed): Using Error Wrapper

**Restores Go error conventions**:
```go
// ogen-generated HTTP client with wrapper
resp, err := client.CreateUser(ctx, &invalidUser)
err = CheckCreateUserError(resp, err)  // ← Convert to Go error
if err != nil {  // ✅ Works!
    t.Logf("Got expected validation error: %v", err)
}
```

---

## Conclusion

### ✅ Expert-Validated Design Pattern

**SME Confirmation**: ogen's behavior is **intentional and well-designed**, not a bug or oversight.

ogen treats **all OpenAPI spec-defined status codes** (including error codes like 400, 422, 500) as valid response types, not as Go errors. This reflects ogen's **type-safety-first philosophy**.

**Why this design choice**:
- ✅ Strict adherence to OpenAPI spec semantics
- ✅ Each defined status code gets full type safety
- ✅ Discriminated union pattern for all possible responses
- ✅ Only **undefined** status codes trigger error path
- ✅ Caller interprets which responses represent "success" vs "error"

**Trade-off**: Type safety over Go idioms (intentional design decision by maintainers).

### Why "Just Don't Define Errors" Doesn't Work

The error parser pattern (`parseOgenError`) works **only when error responses aren't in the spec**:

```go
// Spec only defines 201 → 4xx/5xx become "unexpected"
resp, err := client.CreateAudit(ctx, batch)
if err != nil {  // ✅ Works: err = "unexpected status code: 400"
    parseOgenError(err)  // Can parse status from error string
}

// Spec defines 400/500 (RFC 7807) → they're "expected" responses
resp, err := client.CreateUser(ctx, user)
if err != nil {  // ❌ Never executes - err is nil
    // Won't reach here for validation errors!
}
```

But removing error responses from the spec:
- ❌ Loses type safety (defeats ogen's main benefit)
- ❌ Violates RFC 7807 and API best practices
- ❌ Defeats the purpose of using OpenAPI

### ✅ Validated, Recommended Solution

**The error-checking wrapper is the community-standard, field-proven approach**:

✅ **Confirmed by SME**: "Standard among ogen users" and "fully compatible with ogen updates"

✅ **Benefits**:
- Preserves ogen's type safety benefits
- Follows OpenAPI and RFC 7807 best practices
- Restores Go error conventions where needed
- Allows access to structured error details
- Compatible with all ogen versions

✅ **Enhancement Available**: Generic `ogenx.NormalizeError()` utility reduces boilerplate

✅ **No Further Investigation Needed**: This is the right approach, confirmed by experts

---

## Complete Working Example

### OpenAPI Specification

```yaml
openapi: 3.0.3
info:
  title: User API
  version: 1.0.0
paths:
  /api/users:
    post:
      operationId: createUser
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/CreateUserRequest'
      responses:
        '201':
          description: User created successfully
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/User'
        '400':
          description: Validation error
          content:
            application/problem+json:
              schema:
                $ref: '#/components/schemas/ProblemDetails'
                
components:
  schemas:
    CreateUserRequest:
      type: object
      required: [email, username]
      properties:
        email:
          type: string
          format: email
        username:
          type: string
          minLength: 3
          
    User:
      type: object
      properties:
        id: {type: string, format: uuid}
        email: {type: string}
        username: {type: string}
        
    ProblemDetails:
      type: object
      required: [type, title, status]
      properties:
        type: {type: string, format: uri}
        title: {type: string}
        status: {type: integer}
        detail: {type: string}
        field_errors:
          type: object
          additionalProperties: {type: string}
```

### Generated ogen Client (Problematic)

```go
// Test that FAILS with ogen's default behavior
func TestCreateUser_ValidationError_FAILS(t *testing.T) {
    client, _ := userapi.NewClient("http://localhost:8080")
    
    invalidReq := &userapi.CreateUserRequest{
        Email:    "",  // Invalid - empty email
        Username: "ab", // Invalid - too short
    }
    
    resp, err := client.CreateUser(context.Background(), invalidReq)
    
    // ❌ ASSERTION FAILS
    // Expected: err != nil (validation error)
    // Actual:   err == nil, resp = &CreateUserBadRequest{...}
    if err != nil {
        t.Logf("Got expected error: %v", err)
    } else {
        t.Fatalf("Expected validation error, got nil")  // ← Test fails here!
    }
}
```

### Error Wrapper Solution

```go
// error_wrapper.go - Add to your test helpers
package testutil

import (
    "fmt"
    "your-module/userapi"  // Your generated client package
)

// CheckCreateUserError converts ogen response types to Go errors
func CheckCreateUserError(resp userapi.CreateUserRes, err error) error {
    // Network/transport errors (already Go errors)
    if err != nil {
        return err
    }
    
    // Check response type
    switch v := resp.(type) {
    case *userapi.CreateUserBadRequest:
        // Extract problem details (RFC 7807)
        detail := "validation error"
        if v.Detail.IsSet() {
            detail = v.Detail.Value
        }
        return fmt.Errorf("HTTP 400: %s", detail)
        
    case *userapi.CreateUserInternalServerError:
        detail := "server error"
        if v.Detail.IsSet() {
            detail = v.Detail.Value
        }
        return fmt.Errorf("HTTP 500: %s", detail)
        
    case *userapi.User:
        // Success - no error
        return nil
        
    default:
        return fmt.Errorf("unexpected response type: %T", resp)
    }
}
```

### Fixed Test with Wrapper

```go
// Test that WORKS with error wrapper
func TestCreateUser_ValidationError_WORKS(t *testing.T) {
    client, _ := userapi.NewClient("http://localhost:8080")
    
    invalidReq := &userapi.CreateUserRequest{
        Email:    "",
        Username: "ab",
    }
    
    resp, err := client.CreateUser(context.Background(), invalidReq)
    err = testutil.CheckCreateUserError(resp, err)  // ← Apply wrapper
    
    // ✅ ASSERTION PASSES
    if err != nil {
        t.Logf("Got expected validation error: %v", err)
    } else {
        t.Fatalf("Expected validation error, got success")
    }
}
```

### Accessing Structured Error Details

```go
// You can still access typed error details when needed
func TestCreateUser_ValidationErrorDetails(t *testing.T) {
    client, _ := userapi.NewClient("http://localhost:8080")
    
    invalidReq := &userapi.CreateUserRequest{
        Email:    "",
        Username: "ab",
    }
    
    resp, err := client.CreateUser(context.Background(), invalidReq)
    
    // Pattern: Check error first, then extract details if needed
    if err := testutil.CheckCreateUserError(resp, err); err != nil {
        t.Logf("Error occurred: %v", err)
        
        // Access structured details for specific assertions
        if badReq, ok := resp.(*userapi.CreateUserBadRequest); ok {
            if badReq.FieldErrors.IsSet() {
                fieldErrs := badReq.FieldErrors.Value
                assert.Contains(t, fieldErrs, "email")
                assert.Contains(t, fieldErrs, "username")
            }
        }
    }
}
```

---

## Next Steps

### ✅ Immediate Implementation (Validated)

1. **Use the error wrapper pattern** - This is the community-standard approach
   - Either endpoint-specific helpers (current implementation)
   - Or generic `ogenx.NormalizeError()` utility (SME recommendation)

2. **Update test code** to use wrappers consistently

3. **Document the pattern** for team members

### 🎯 Optional: Feature Request to ogen

The SME suggested filing a feature request for configurable error handling:

**Proposed Feature**: "Option: treat non-2xx defined responses as Go errors"

**Rationale**:
- Other generators (`oapi-codegen`) support this
- Would reduce boilerplate for Go-idiomatic usage
- Maintains backward compatibility (opt-in flag)

**Related Issues** (per SME): #1224, #1266, #1478

**Would you like a GitHub issue template drafted?** (See Appendix below)

### 📋 Implementation Checklist

- [x] Understand ogen's behavior (✅ Validated by SME)
- [x] Identify community-standard solution (✅ Error wrapper)
- [x] Implement error wrapper helper (✅ Done)
- [ ] Apply wrapper to validation tests
- [ ] Create `ogenx` utility package (optional enhancement)
- [ ] Document pattern in team guidelines
- [ ] File ogen feature request (optional)

---

## Appendix: Draft GitHub Issue for ogen

**Title**: Feature Request: Optional Go error return for non-2xx responses

**Body**:

```markdown
### Problem

ogen currently treats all spec-defined status codes (including 4xx/5xx) as valid response types, returning them as `(typedResponse, nil)` rather than `(nil, error)`.

While this provides excellent type safety and adheres strictly to OpenAPI semantics, it conflicts with Go's idiomatic error handling, particularly for testing validation errors.

**Example**:
```go
resp, err := client.CreateUser(ctx, &invalidUser)
// Expected: err != nil for HTTP 400
// Actual:   err = nil, resp = &CreateUserBadRequest{...}

// Test fails:
if err != nil {
    t.Fatal("expected validation error")
}
```

### Current Workaround

Users wrap responses with helpers:
```go
err = CheckError(resp, err)  // Convert typed errors to Go errors
if err != nil {
    // Now works as expected
}
```

This is **field-proven and works well**, but adds boilerplate.

### Proposed Solution

Add optional configuration to treat non-2xx spec-defined responses as errors:

**Option A: CLI flag**
```bash
ogen --treat-errors-as-errors openapi.yaml
```

**Option B: OpenAPI extension**
```yaml
x-ogen-config:
  error-handling: go-idiomatic  # or "strict" (current behavior)
```

**Option C: Response-level annotation**
```yaml
responses:
  '400':
    x-ogen-error: true
```

### Benefits

- ✅ Reduces boilerplate for Go-idiomatic usage
- ✅ Maintains backward compatibility (opt-in)
- ✅ Preserves type safety (typed error structs still available)
- ✅ Aligns with other generators (`oapi-codegen` does this by default)

### Related

Similar to discussions in #1224, #1266, #1478.

### Alternative

Users can continue using wrapper helpers (current community practice), but an opt-in flag would improve ergonomics for Go projects.
```

---

## References

- **ogen GitHub**: https://github.com/ogen-go/ogen
- **ogen Documentation**: https://ogen.dev/
- **RFC 7807** (Problem Details): https://www.rfc-editor.org/rfc/rfc7807
- **OpenAPI Spec v3**: https://swagger.io/specification/

### Related Discussions (if found)

- [ ] ogen GitHub issues about error handling
- [ ] Community discussions on error response patterns
- [ ] Feature requests for error return options

---

**Status**: ✅ **INVESTIGATION COMPLETE & SME-VALIDATED**  
Error wrapper pattern confirmed as **community-standard, field-proven approach**. Ready to proceed with implementation.
