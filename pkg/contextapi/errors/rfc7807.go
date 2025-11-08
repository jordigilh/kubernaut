package errors

import "fmt"

// RFC 7807 Problem Details for HTTP APIs
// https://tools.ietf.org/html/rfc7807
//
// This package provides structured RFC 7807 error handling for Context API,
// maintaining quality parity with Gateway and Data Storage services.
//
// BR-CONTEXT-014: RFC 7807 error propagation from Data Storage Service

// RFC7807Error represents a structured RFC 7807 Problem Details error
// This allows Context API consumers to access all error fields, not just the message.
//
// Fields:
// - Type: URI reference identifying the problem type (e.g., "https://api.kubernaut.io/problems/invalid-pagination")
// - Title: Short, human-readable summary (e.g., "Invalid Pagination Parameters")
// - Detail: Human-readable explanation specific to this occurrence
// - Status: HTTP status code from the upstream service
// - Instance: URI reference to specific occurrence
// - RequestID: Request tracing ID (extension member)
type RFC7807Error struct {
	Type      string `json:"type"`                 // URI identifying problem type
	Title     string `json:"title"`                // Short summary
	Detail    string `json:"detail"`               // Detailed explanation
	Status    int    `json:"status"`               // HTTP status code
	Instance  string `json:"instance,omitempty"`   // URI to specific occurrence
	RequestID string `json:"request_id,omitempty"` // BR-CONTEXT-012: Request tracing
}

// Error type URI constants
// BR-CONTEXT-014: RFC 7807 error format
// These URIs identify the problem type and can link to documentation
// Note: Using kubernaut.io domain (not api.kubernaut.io) per test requirements
const (
	ErrorTypeValidationError      = "https://kubernaut.io/errors/validation-error"
	ErrorTypeNotFound             = "https://kubernaut.io/errors/not-found"
	ErrorTypeMethodNotAllowed     = "https://kubernaut.io/errors/method-not-allowed"
	ErrorTypeUnsupportedMediaType = "https://kubernaut.io/errors/unsupported-media-type"
	ErrorTypeInternalError        = "https://kubernaut.io/errors/internal-error"
	ErrorTypeServiceUnavailable   = "https://kubernaut.io/errors/service-unavailable"
	ErrorTypeGatewayTimeout       = "https://kubernaut.io/errors/gateway-timeout"
	ErrorTypeUnknown              = "https://kubernaut.io/errors/unknown"
)

// Error title constants
// BR-CONTEXT-014: RFC 7807 error format
const (
	TitleBadRequest           = "Bad Request"
	TitleNotFound             = "Not Found"
	TitleMethodNotAllowed     = "Method Not Allowed"
	TitleUnsupportedMediaType = "Unsupported Media Type"
	TitleInternalServerError  = "Internal Server Error"
	TitleServiceUnavailable   = "Service Unavailable"
	TitleGatewayTimeout       = "Gateway Timeout"
	TitleUnknown              = "Error"
)

// Error implements the error interface
// Returns a formatted error message including title and detail
func (e *RFC7807Error) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("%s: %s", e.Title, e.Detail)
	}
	return e.Title
}

// IsRFC7807Error checks if an error is an RFC7807Error
// This allows consumers to type-check errors and access structured fields
func IsRFC7807Error(err error) (*RFC7807Error, bool) {
	if err == nil {
		return nil, false
	}

	rfc7807Err, ok := err.(*RFC7807Error)
	return rfc7807Err, ok
}
