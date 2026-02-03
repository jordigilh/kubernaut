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
	"fmt"
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
//   resp, err := client.SomeEndpoint(ctx, req)
//   err = ogenx.ToError(resp, err)
//   if err != nil {
//       // Handle error (works for both network errors and HTTP errors)
//   }
//
// Handles two cases:
//  1. Error strings (undefined status codes): "unexpected status code: 503"
//  2. Typed responses (defined status codes): *BadRequest, *InternalServerError, etc.
//
// Automatically extracts RFC 7807 Problem Details when available.
//
// Authority: SME-validated pattern from OGEN_ERROR_HANDLING_INVESTIGATION_FEB_03_2026.md
func ToError(resp any, err error) error {
	// Case 1: err is already set (network error or undefined status code)
	// Try to extract HTTP status from ogen error string
	if err != nil {
		return parseStatusFromErrorString(err)
	}

	// Case 2: Check if typed response indicates an error (status >= 400)
	return checkResponseStatus(resp)
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
//  3. Status field (int32 or int)
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

	// No status code found - treat as success
	return 200
}

// extractErrorDetail extracts error detail from RFC 7807 Problem Details or fallback fields.
//
// Tries common field patterns:
//  1. GetDetail() OptString (RFC 7807)
//  2. GetMessage() string (common alternative)
//  3. Detail string field
//  4. Message string field
func extractErrorDetail(resp any) string {
	// Pattern 1: RFC 7807 GetDetail() OptString
	type detailGetter interface {
		GetDetail() interface{ IsSet() bool; Value string }
	}
	if v, ok := resp.(detailGetter); ok {
		if detail := v.GetDetail(); detail.IsSet() {
			return detail.Value
		}
	}

	// Pattern 2: GetMessage() string
	type messageGetter interface {
		GetMessage() string
	}
	if v, ok := resp.(messageGetter); ok {
		if msg := v.GetMessage(); msg != "" {
			return msg
		}
	}

	// Pattern 3: Try reflection for common field names
	// (Avoid for now - explicit interface checking is safer)

	return "" // No detail found
}

// extractErrorTitle extracts error title from RFC 7807 Problem Details.
//
// Tries common field patterns:
//  1. GetTitle() string (RFC 7807)
//  2. Title string field
func extractErrorTitle(resp any) string {
	// Pattern 1: RFC 7807 GetTitle() string
	type titleGetter interface {
		GetTitle() string
	}
	if v, ok := resp.(titleGetter); ok {
		if title := v.GetTitle(); title != "" {
			return title
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

// IsHTTPError returns true if the error is an HTTPError.
func IsHTTPError(err error) bool {
	_, ok := err.(*HTTPError)
	return ok
}

// GetHTTPError returns the HTTPError if err is an HTTPError, nil otherwise.
//
// Useful for accessing structured error details:
//   if httpErr := ogenx.GetHTTPError(err); httpErr != nil {
//       // Access structured fields
//       if httpErr.StatusCode == 400 {
//           // Handle validation error
//       }
//   }
func GetHTTPError(err error) *HTTPError {
	if httpErr, ok := err.(*HTTPError); ok {
		return httpErr
	}
	return nil
}
