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
	"encoding/json"
	"mime"
	"net/http"

	"github.com/jordigilh/kubernaut/pkg/toolset/errors"
)

// ValidateContentType is a middleware that validates the Content-Type header for POST, PUT, and PATCH requests
// BR-TOOLSET-043: Content-Type Validation Middleware
// BUSINESS OUTCOME: Reject invalid Content-Type early, preventing MIME confusion attacks and malformed request processing
//
// TDD GREEN Phase: Minimal implementation to pass 8 unit tests
// - Validates Content-Type for POST, PUT, PATCH requests only
// - Accepts "application/json" (with or without charset parameter)
// - Returns 415 Unsupported Media Type for invalid/missing Content-Type
// - Returns RFC 7807 compliant error responses
// - Propagates request ID from context
func ValidateContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only validate Content-Type for POST, PUT, PATCH requests
		// GET, DELETE, HEAD, OPTIONS do not require Content-Type
		if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch {
			next.ServeHTTP(w, r)
			return
		}

		contentType := r.Header.Get("Content-Type")

		// Missing Content-Type is an error for POST/PUT/PATCH
		if contentType == "" {
			writeRFC7807Error(w, r, http.StatusUnsupportedMediaType, "Content-Type header is missing; must be 'application/json'")
			return
		}

		// Parse media type to handle charset parameters (e.g., "application/json; charset=utf-8")
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err != nil {
			// Invalid Content-Type format - return RFC 7807 error
			writeRFC7807Error(w, r, http.StatusBadRequest, "Invalid Content-Type header format")
			return
		}

		// Validate that media type is application/json
		if mediaType != "application/json" {
			// Non-JSON Content-Type - return RFC 7807 error
			detail := "Content-Type must be 'application/json', got '" + contentType + "'"
			writeRFC7807Error(w, r, http.StatusUnsupportedMediaType, detail)
			return
		}

		// Content-Type is valid, proceed to next handler
		next.ServeHTTP(w, r)
	})
}

// writeRFC7807Error writes an RFC 7807 compliant error response
// BR-TOOLSET-043: Content-Type validation errors
// BR-TOOLSET-039: RFC 7807 error format
//
// TDD GREEN Phase: Minimal error response implementation
// - Sets Content-Type: application/problem+json
// - Uses RFC7807Error struct from pkg/toolset/errors
// - Propagates request ID from X-Request-ID header
func writeRFC7807Error(w http.ResponseWriter, r *http.Request, statusCode int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(statusCode)

	// Get request ID from header (set by RequestIDMiddleware)
	requestID := r.Header.Get("X-Request-ID")

	// Create RFC 7807 error response
	errorResponse := errors.NewRFC7807Error(statusCode, detail, r.URL.Path)
	errorResponse.RequestID = requestID

	// Encode and write error response
	if err := json.NewEncoder(w).Encode(errorResponse); err != nil {
		// Fallback to plain text if JSON encoding fails
		http.Error(w, detail, statusCode)
	}
}

