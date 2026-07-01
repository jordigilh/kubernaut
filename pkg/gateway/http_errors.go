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

package gateway

import (
	"encoding/json"
	"net/http"

	// BR-GATEWAY-093: Circuit breaker detection

	gwerrors "github.com/jordigilh/kubernaut/pkg/gateway/errors"

	// DD-AUDIT-003: Audit integration
	// Ogen generated audit types
	// BR-AUDIT-005 Gap #7: Standardized error details
	// BR-GATEWAY-036/037: Shared auth middleware
	// ADR-052 Addendum 001: Exponential backoff with jitter
	// Issue #753: Dedicated health server
	// Issue #756: FileWatcher for cert rotation
	// Issue #493/#678: Conditional TLS

	// BR-GATEWAY-190: Lease resources for distributed locking

	// BR-GATEWAY-036/037: K8s clientset for TokenReview/SAR

	// ADR-068: Federated scope checking factory

	"github.com/jordigilh/kubernaut/pkg/gateway/middleware" // BR-109: Request ID middleware
	// BR-HTTP-015: Shared CORS library
	"github.com/jordigilh/kubernaut/pkg/shared/sanitization" // DD-005: Shared sanitization library
	// BR-SCOPE-002: Resource scope management
)

// TDD REFACTOR: RFC7807Error moved to pkg/gateway/errors package to eliminate duplication

// writeJSONError writes an RFC 7807 compliant error response
// TDD GREEN: Updated to support BR-041 (RFC 7807 error format)
// TDD REFACTOR: Now uses shared gwerrors.RFC7807Error struct and constants
// BR-109: Added request ID extraction for request tracing
// BR-GATEWAY-078: Added error message sanitization to prevent sensitive data exposure
// Business Outcome: Clients receive standards-compliant, machine-readable error responses
func (s *Server) writeJSONError(w http.ResponseWriter, r *http.Request, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(statusCode)

	// BR-109: Extract request ID from context for tracing
	requestID := middleware.GetRequestID(r.Context())

	// BR-GATEWAY-078: Sanitize error message to prevent sensitive data exposure
	// DD-005: Use shared sanitization library directly
	sanitizedMessage := sanitization.SanitizeForLog(message)

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

// getErrorTypeAndTitle returns the RFC 7807 error type URI and title for a given HTTP status code
// BR-041: RFC 7807 error format
// TDD REFACTOR: Now uses shared gwerrors constants
func getErrorTypeAndTitle(statusCode int) (string, string) {
	switch statusCode {
	case http.StatusBadRequest:
		return gwerrors.ErrorTypeValidationError, gwerrors.TitleBadRequest
	case http.StatusMethodNotAllowed:
		return gwerrors.ErrorTypeMethodNotAllowed, gwerrors.TitleMethodNotAllowed
	case http.StatusInternalServerError:
		return gwerrors.ErrorTypeInternalError, gwerrors.TitleInternalServerError
	case http.StatusRequestEntityTooLarge:
		return gwerrors.ErrorTypePayloadTooLarge, gwerrors.TitlePayloadTooLarge
	case http.StatusGatewayTimeout:
		return gwerrors.ErrorTypeGatewayTimeout, gwerrors.TitleGatewayTimeout
	case http.StatusServiceUnavailable:
		return gwerrors.ErrorTypeServiceUnavailable, gwerrors.TitleServiceUnavailable
	default:
		return gwerrors.ErrorTypeUnknown, gwerrors.TitleUnknown
	}
}

// writeValidationError writes a 400 Bad Request error response
// TDD REFACTOR: Extracted common validation error pattern
// BR-109: Added request parameter for request ID tracing
// Business Outcome: Consistent validation error handling (BR-001)
func (s *Server) writeValidationError(w http.ResponseWriter, r *http.Request, message string) {
	s.writeJSONError(w, r, message, http.StatusBadRequest)
}

// writeInternalError writes a 500 Internal Server Error response
// TDD REFACTOR: Extracted common internal error pattern
// BR-109: Added request parameter for request ID tracing
// Business Outcome: Consistent internal error handling (BR-001)
func (s *Server) writeInternalError(w http.ResponseWriter, r *http.Request, message string) {
	s.writeJSONError(w, r, message, http.StatusInternalServerError)
}
