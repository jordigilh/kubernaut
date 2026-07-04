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

// Package ogenx provides utilities for working with ogen-generated OpenAPI clients.
//
// Authority: OGEN_ERROR_HANDLING_INVESTIGATION_FEB_03_2026.md
// SME-Validated: Community-standard pattern for ogen error handling
package ogenx

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// ToError converts ogen client responses to Go errors following idiomatic error handling.
//
// ogen-generated clients treat all spec-defined status codes (including 4xx/5xx) as
// valid response types, returning (typedResponse, nil) instead of (nil, error).
// This function converts those typed error responses into Go errors.
//
// Usage:
//
//	resp, err := client.SomeEndpoint(ctx, req)
//	err = ogenx.ToError(resp, err)
//	if err != nil {
//	    // Handle error (works for both network errors and HTTP errors)
//	}
//
// Handles two cases:
//  1. Error strings (undefined status codes): "unexpected status code: 503"
//  2. Typed responses (defined status codes): *BadRequest, *InternalServerError, etc.
//
// Automatically extracts RFC 7807 Problem Details when available:
//   - Status code (via GetStatus() int32)
//   - Title (via GetTitle() string)
//   - Message (via GetMessage() string)
//
// Note: Detailed RFC 7807 "detail" field extraction has limitations with current
// implementation. The HTTPError preserves the original typed response in the Response
// field for manual inspection if detailed error messages are needed.
//
// Authority: SME-validated pattern from OGEN_ERROR_HANDLING_INVESTIGATION_FEB_03_2026.md
func ToError(resp any, err error) error {
	if err != nil {
		// Case 1a: ogen "convenient error" wraps a typed error response via errors.Wrap.
		// Unwrap to find a type that exposes GetStatusCode()/GetResponse().
		if httpErr := extractConvenientError(err); httpErr != nil {
			return httpErr
		}
		// Case 1b: ogen "unexpected status code" string for undefined status codes.
		return parseStatusFromErrorString(err)
	}

	// Case 2: Check if typed response indicates an error (status >= 400)
	return checkResponseStatus(resp)
}

// statusCodeError matches ogen's convenient error wrapper (*RFC7807ProblemStatusCode).
type statusCodeError interface {
	error
	GetStatusCode() int
}

// extractConvenientError walks the error chain looking for ogen's convenient
// error type (*RFC7807ProblemStatusCode) which wraps a default RFC 7807 response.
func extractConvenientError(err error) *HTTPError {
	var sce statusCodeError
	if !errors.As(err, &sce) {
		return nil
	}
	code := sce.GetStatusCode()
	if code < 400 {
		return nil
	}
	detail := extractErrorDetail(sce)
	title := extractErrorTitle(sce)
	return &HTTPError{
		StatusCode: code,
		Title:      title,
		Detail:     detail,
		Response:   sce,
	}
}

// parseStatusFromErrorString extracts HTTP status codes from ogen error strings.
//
// ogen returns errors for undefined status codes with format:
// "decode response: unexpected status code: 503"
//
// This preserves the original error but adds HTTP status context.
func parseStatusFromErrorString(err error) error {
	errMsg := err.Error()

	// Check for ogen's "unexpected status code" format
	if strings.Contains(errMsg, "unexpected status code:") {
		parts := strings.Split(errMsg, "unexpected status code:")
		if len(parts) == 2 {
			statusStr := strings.TrimSpace(parts[1])
			// Extract just the numeric code (e.g., "503 Service Unavailable" -> "503")
			statusStr = strings.Fields(statusStr)[0]

			if statusCode, parseErr := strconv.Atoi(statusStr); parseErr == nil {
				return &HTTPError{
					StatusCode: statusCode,
					Message:    errMsg,
				}
			}
		}
	}

	// Not an HTTP status error - return original error
	return err
}

// checkResponseStatus checks if a typed response represents an HTTP error.
//
// Uses interface detection to identify responses with status codes >= 400.
// Automatically extracts RFC 7807 Problem Details when available.
func checkResponseStatus(resp any) error {
	if resp == nil {
		return nil
	}

	// Try to get status code from response
	statusCode := getStatusCode(resp)
	if statusCode < 400 {
		return nil // Success response (2xx/3xx)
	}

	// Extract error details (RFC 7807 or generic)
	detail := extractErrorDetail(resp)
	title := extractErrorTitle(resp)

	return &HTTPError{
		StatusCode: statusCode,
		Title:      title,
		Detail:     detail,
		Response:   resp, // Preserve typed response for detailed inspection
	}
}

// getStatusCode attempts to extract the HTTP status code from a typed response.
//
// Tries multiple common patterns used by ogen-generated code:
//  1. GetStatus() int32
//  2. GetStatus() int
//  3. Status field (int32 or int) via reflection
func getStatusCode(resp any) int {
	// Pattern 1: GetStatus() int32 (most common in ogen)
	type statusGetter32 interface {
		GetStatus() int32
	}
	if v, ok := resp.(statusGetter32); ok {
		return int(v.GetStatus())
	}

	// Pattern 2: GetStatus() int
	type statusGetter interface {
		GetStatus() int
	}
	if v, ok := resp.(statusGetter); ok {
		return v.GetStatus()
	}

	// Pattern 3: Status field (KA uses this)
	// Use reflection to access struct fields when method doesn't exist
	val := reflect.ValueOf(resp)

	// Handle pointer types
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return 200 // Treat nil as success
		}
		val = val.Elem()
	}

	// Check if it's a struct with a Status field
	if val.Kind() == reflect.Struct {
		statusField := val.FieldByName("Status")
		if statusField.IsValid() {
			switch statusField.Kind() {
			case reflect.Int32:
				return int(statusField.Int())
			case reflect.Int:
				return int(statusField.Int())
			}
		}
	}

	// No status code found - treat as success
	return 200
}

// OptionalString is an interface matching ogen-generated optional string types.
// ogen typically generates: type OptString struct { Value string; Set bool }
// with methods like IsSet() bool.
type OptionalString interface {
	IsSet() bool
}

// optionalString matches ogen-generated OptString types.
// ogen generates: type OptString struct { Value string; Set bool }
// with methods: IsSet() bool
type optionalString interface {
	IsSet() bool
}

// extractErrorDetail extracts error detail from RFC 7807 Problem Details or fallback fields.
//
// Tries common field patterns:
//  1. GetDetail() returning optional string (RFC 7807)
//  2. Detail field via reflection (KA uses this)
//  3. GetMessage() string (common alternative)
//
// Uses reflection to access ogen-generated OptString.Value field when available.
func extractErrorDetail(resp any) string {
	if detail := extractDetailViaGetter(resp); detail != "" {
		return detail
	}
	if detail := extractDetailViaField(resp); detail != "" {
		return detail
	}
	return extractDetailViaMessage(resp)
}

// extractDetailViaGetter handles Pattern 1: RFC 7807 GetDetail() returning
// an "any" value that (when set) wraps an ogen-generated OptString.
func extractDetailViaGetter(resp any) string {
	detailResp, ok := resp.(interface {
		GetDetail() any
	})
	if !ok {
		return ""
	}
	optDetail := detailResp.GetDetail()
	if optDetail == nil {
		return ""
	}
	opt, ok := optDetail.(optionalString)
	if !ok || !opt.IsSet() {
		return ""
	}
	return stringValueField(reflect.ValueOf(optDetail))
}

// extractDetailViaField handles Pattern 2: an exported "Detail" struct field,
// either a plain string (KA RFC 7807) or an OptString-like nested struct.
func extractDetailViaField(resp any) string {
	val := reflect.ValueOf(resp)
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return ""
		}
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return ""
	}

	detailField := val.FieldByName("Detail")
	if !detailField.IsValid() {
		return ""
	}
	if detailField.Kind() == reflect.String {
		return detailField.String()
	}
	if !detailField.CanInterface() {
		return ""
	}

	opt, ok := detailField.Interface().(optionalString)
	if !ok || !opt.IsSet() {
		return ""
	}
	return stringValueField(reflect.ValueOf(detailField.Interface()))
}

// extractDetailViaMessage handles Pattern 3: GetMessage() string fallback.
func extractDetailViaMessage(resp any) string {
	type messageGetter interface {
		GetMessage() string
	}
	v, ok := resp.(messageGetter)
	if !ok {
		return ""
	}
	return v.GetMessage()
}

// stringValueField extracts the "Value" field from an ogen-generated OptString
// struct (or pointer to one) via reflection, returning "" if not found.
func stringValueField(val reflect.Value) string {
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return ""
		}
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return ""
	}
	valueField := val.FieldByName("Value")
	if valueField.IsValid() && valueField.Kind() == reflect.String {
		return valueField.String()
	}
	return ""
}

// extractErrorTitle extracts error title from RFC 7807 Problem Details.
//
// Tries common field patterns:
//  1. GetTitle() string (RFC 7807)
//  2. Title string field (KA uses this)
func extractErrorTitle(resp any) string {
	// Pattern 1: RFC 7807 GetTitle() string (method-based)
	type titleGetter interface {
		GetTitle() string
	}
	if v, ok := resp.(titleGetter); ok {
		if title := v.GetTitle(); title != "" {
			return title
		}
	}

	// Pattern 2: Title field (KA uses this)
	// Use reflection to access struct fields when GetTitle() method doesn't exist
	val := reflect.ValueOf(resp)

	// Handle pointer types
	if val.Kind() == reflect.Ptr {
		if !val.IsNil() {
			val = val.Elem()
		}
	}

	// Check if it's a struct with a Title field
	if val.Kind() == reflect.Struct {
		titleField := val.FieldByName("Title")
		if titleField.IsValid() && titleField.Kind() == reflect.String {
			return titleField.String()
		}
	}

	return "" // No title found
}

// HTTPError represents an HTTP error response from an ogen client.
//
// Contains structured error information extracted from the response,
// while preserving the original typed response for detailed inspection.
type HTTPError struct {
	StatusCode int    // HTTP status code (400, 422, 500, etc.)
	Title      string // Error title (RFC 7807)
	Detail     string // Error detail/message (RFC 7807)
	Message    string // Original error message (for undefined status codes)
	Response   any    // Original typed response (for accessing structured fields)
}

// Error implements the error interface.
func (e *HTTPError) Error() string {
	if e.Message != "" {
		// Error from undefined status code (already has full message)
		return e.Message
	}

	// Error from typed response - construct message
	var parts []string
	parts = append(parts, fmt.Sprintf("HTTP %d", e.StatusCode))

	if e.Title != "" {
		parts = append(parts, e.Title)
	}

	if e.Detail != "" {
		parts = append(parts, e.Detail)
	} else if e.Response != nil {
		// No detail extracted - show response type
		parts = append(parts, fmt.Sprintf("(%T)", e.Response))
	}

	return strings.Join(parts, ": ")
}

// IsHTTPError returns true if the error is (or wraps) an HTTPError.
func IsHTTPError(err error) bool {
	var target *HTTPError
	return errors.As(err, &target)
}

// GetHTTPError returns the HTTPError if err is (or wraps) an HTTPError, nil otherwise.
//
// Useful for accessing structured error details:
//
//	if httpErr := ogenx.GetHTTPError(err); httpErr != nil {
//	    // Access structured fields
//	    if httpErr.StatusCode == 400 {
//	        // Handle validation error
//	    }
//	}
func GetHTTPError(err error) *HTTPError {
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr
	}
	return nil
}
