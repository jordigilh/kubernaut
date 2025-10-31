package errors

// RFC7807Error represents an RFC 7807 Problem Details error response
// Specification: https://tools.ietf.org/html/rfc7807
//
// DD-004: RFC 7807 Error Response Standard
// BR-CONTEXT-009: Consistent error responses
//
// GREEN PHASE: Minimal implementation to satisfy business requirements
type RFC7807Error struct {
	// REQUIRED FIELDS (per RFC 7807)
	Type   string `json:"type"`   // URI reference identifying the problem type
	Title  string `json:"title"`  // Short, human-readable summary
	Detail string `json:"detail"` // Human-readable explanation
	Status int    `json:"status"` // HTTP status code

	// OPTIONAL FIELDS (per RFC 7807)
	Instance string `json:"instance"` // URI reference to specific occurrence

	// EXTENSION MEMBERS (DD-004 Section 4.2)
	RequestID string `json:"request_id,omitempty"` // Request tracing
}

// Error type URI constants (DD-004: Error Type URI Convention)
const (
	ErrorTypeValidationError      = "https://kubernaut.io/errors/validation-error"
	ErrorTypeUnsupportedMediaType = "https://kubernaut.io/errors/unsupported-media-type"
	ErrorTypeMethodNotAllowed     = "https://kubernaut.io/errors/method-not-allowed"
	ErrorTypeInternalError        = "https://kubernaut.io/errors/internal-error"
	ErrorTypeServiceUnavailable   = "https://kubernaut.io/errors/service-unavailable"
	ErrorTypeGatewayTimeout       = "https://kubernaut.io/errors/gateway-timeout"
	ErrorTypeNotFound             = "https://kubernaut.io/errors/not-found"
	ErrorTypeUnknown              = "https://kubernaut.io/errors/unknown"
)

// Error title constants (RFC 7807 Section 3.1)
const (
	TitleBadRequest           = "Bad Request"
	TitleUnsupportedMediaType = "Unsupported Media Type"
	TitleMethodNotAllowed     = "Method Not Allowed"
	TitleInternalServerError  = "Internal Server Error"
	TitleServiceUnavailable   = "Service Unavailable"
	TitleGatewayTimeout       = "Gateway Timeout"
	TitleNotFound             = "Not Found"
	TitleUnknown              = "Error"
)

