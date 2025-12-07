package errors

// RFC 7807 Problem Details for HTTP APIs
// https://tools.ietf.org/html/rfc7807
//
// This package provides a standardized error response format for HTTP APIs.
// BR-041: RFC 7807 error format
// TDD REFACTOR: Extracted from middleware and server to eliminate duplication

// RFC7807Error represents an RFC 7807 Problem Details error response
// BR-041: RFC 7807 error format
type RFC7807Error struct {
	Type      string `json:"type"`                 // URI reference identifying the problem type
	Title     string `json:"title"`                // Short, human-readable summary
	Detail    string `json:"detail"`               // Human-readable explanation
	Status    int    `json:"status"`               // HTTP status code
	Instance  string `json:"instance"`             // URI reference to specific occurrence
	RequestID string `json:"request_id,omitempty"` // BR-109: Request tracing (extension member)
}

// Error type URI constants
// BR-041: RFC 7807 error format
// These URIs identify the problem type and can link to documentation
const (
	ErrorTypeValidationError      = "https://kubernaut.ai/errors/validation-error"
	ErrorTypeUnsupportedMediaType = "https://kubernaut.ai/errors/unsupported-media-type"
	ErrorTypeMethodNotAllowed     = "https://kubernaut.ai/errors/method-not-allowed"
	ErrorTypeInternalError        = "https://kubernaut.ai/errors/internal-error"
	ErrorTypeServiceUnavailable   = "https://kubernaut.ai/errors/service-unavailable"
	ErrorTypeTooManyRequests      = "https://kubernaut.ai/errors/too-many-requests"
	ErrorTypeUnknown              = "https://kubernaut.ai/errors/unknown"
)

// Error title constants
// BR-041: RFC 7807 error format
const (
	TitleBadRequest           = "Bad Request"
	TitleUnsupportedMediaType = "Unsupported Media Type"
	TitleMethodNotAllowed     = "Method Not Allowed"
	TitleInternalServerError  = "Internal Server Error"
	TitleServiceUnavailable   = "Service Unavailable"
	TitleTooManyRequests      = "Too Many Requests"
	TitleUnknown              = "Error"
)
