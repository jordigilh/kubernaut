package errors

// RFC 7807 Problem Details for HTTP APIs
// https://tools.ietf.org/html/rfc7807
//
// This package provides a standardized error response format for HTTP APIs.
// BR-TOOLSET-039: RFC 7807 error format
// Reference: Gateway Service (pkg/gateway/errors/rfc7807.go)
// Reference: Context API (pkg/contextapi/errors/rfc7807.go)

// RFC7807Error represents an RFC 7807 Problem Details error response
// BR-TOOLSET-039: RFC 7807 error format
type RFC7807Error struct {
	Type      string `json:"type"`                 // URI reference identifying the problem type
	Title     string `json:"title"`                // Short, human-readable summary
	Detail    string `json:"detail"`               // Human-readable explanation
	Status    int    `json:"status"`               // HTTP status code
	Instance  string `json:"instance"`             // URI reference to specific occurrence
	RequestID string `json:"request_id,omitempty"` // Request tracing (extension member)
}

// Error type URI constants
// BR-TOOLSET-039: RFC 7807 error format
// These URIs identify the problem type and can link to documentation
const (
	ErrorTypeValidationError      = "https://kubernaut.io/errors/validation-error"
	ErrorTypeUnsupportedMediaType = "https://kubernaut.io/errors/unsupported-media-type"
	ErrorTypeMethodNotAllowed     = "https://kubernaut.io/errors/method-not-allowed"
	ErrorTypeInternalError        = "https://kubernaut.io/errors/internal-error"
	ErrorTypeServiceUnavailable   = "https://kubernaut.io/errors/service-unavailable"
	ErrorTypeUnknown              = "https://kubernaut.io/errors/unknown"
)

// Error title constants
// BR-TOOLSET-039: RFC 7807 error format
const (
	TitleBadRequest           = "Bad Request"
	TitleUnsupportedMediaType = "Unsupported Media Type"
	TitleMethodNotAllowed     = "Method Not Allowed"
	TitleInternalServerError  = "Internal Server Error"
	TitleServiceUnavailable   = "Service Unavailable"
	TitleUnknown              = "Error"
)

// Error implements the error interface
func (e RFC7807Error) Error() string {
	return e.Detail
}

// NewRFC7807Error creates a new RFC 7807 error
func NewRFC7807Error(statusCode int, detail, instance string) RFC7807Error {
	errorType, title := getErrorTypeAndTitle(statusCode)
	return RFC7807Error{
		Type:     errorType,
		Title:    title,
		Detail:   detail,
		Status:   statusCode,
		Instance: instance,
	}
}

// getErrorTypeAndTitle maps HTTP status codes to RFC 7807 error types and titles
func getErrorTypeAndTitle(statusCode int) (string, string) {
	switch statusCode {
	case 400:
		return ErrorTypeValidationError, TitleBadRequest
	case 405:
		return ErrorTypeMethodNotAllowed, TitleMethodNotAllowed
	case 415:
		return ErrorTypeUnsupportedMediaType, TitleUnsupportedMediaType
	case 500:
		return ErrorTypeInternalError, TitleInternalServerError
	case 503:
		return ErrorTypeServiceUnavailable, TitleServiceUnavailable
	default:
		return ErrorTypeUnknown, TitleUnknown
	}
}
