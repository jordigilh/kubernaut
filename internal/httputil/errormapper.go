package httputil

import (
	"errors"
	"net/http"

	"github.com/jordigilh/kubernaut-apifrontend/internal/requestid"
)

const problemBaseURI = "https://kubernaut.ai/problems/"

// Well-known problem type URIs.
const (
	ProblemAuthenticationFailed = problemBaseURI + "authentication-failed"
	ProblemPermissionDenied     = problemBaseURI + "permission-denied"
	ProblemRateLimited          = problemBaseURI + "rate-limited"
	ProblemUpstreamTimeout      = problemBaseURI + "upstream-timeout"
	ProblemServiceUnavailable   = problemBaseURI + "service-unavailable"
	ProblemValidationFailed     = problemBaseURI + "validation-failed"
	ProblemPayloadTooLarge      = problemBaseURI + "payload-too-large"
	ProblemNotFound             = problemBaseURI + "not-found"
)

// ErrorClassification maps known error categories to HTTP problem details.
type ErrorClassification struct {
	TypeURI string
	Title   string
	Status  int
}

// Sentinel errors for RFC 7807 classification.
var (
	ErrAuthentication   = errors.New("authentication failed")
	ErrPermissionDenied = errors.New("permission denied")
	ErrRateLimited      = errors.New("rate limited")
	ErrUpstreamTimeout  = errors.New("upstream timeout")
	ErrUnavailable      = errors.New("service unavailable")
	ErrValidation       = errors.New("validation failed")
	ErrPayloadTooLarge  = errors.New("payload too large")
	ErrNotFound         = errors.New("not found")
)

var classifications = []struct {
	sentinel error
	class    ErrorClassification
}{
	{ErrAuthentication, ErrorClassification{ProblemAuthenticationFailed, "Authentication Failed", http.StatusUnauthorized}},
	{ErrPermissionDenied, ErrorClassification{ProblemPermissionDenied, "Permission Denied", http.StatusForbidden}},
	{ErrRateLimited, ErrorClassification{ProblemRateLimited, "Rate Limited", http.StatusTooManyRequests}},
	{ErrUpstreamTimeout, ErrorClassification{ProblemUpstreamTimeout, "Upstream Timeout", http.StatusGatewayTimeout}},
	{ErrUnavailable, ErrorClassification{ProblemServiceUnavailable, "Service Unavailable", http.StatusServiceUnavailable}},
	{ErrValidation, ErrorClassification{ProblemValidationFailed, "Validation Failed", http.StatusBadRequest}},
	{ErrPayloadTooLarge, ErrorClassification{ProblemPayloadTooLarge, "Payload Too Large", http.StatusRequestEntityTooLarge}},
	{ErrNotFound, ErrorClassification{ProblemNotFound, "Not Found", http.StatusNotFound}},
}

// MapToRFC7807 classifies an error into a ProblemDetail with appropriate
// type URI, title, and status. Unclassified errors map to 500 Internal Server Error
// with a generic message (no internal details leaked).
func MapToRFC7807(err error, r *http.Request) *ProblemDetail {
	detail := safeDetail(err)
	instance := ""
	requestID := ""
	if r != nil {
		instance = r.URL.Path
		requestID = requestid.FromContext(r.Context())
	}

	for _, c := range classifications {
		if errors.Is(err, c.sentinel) {
			return &ProblemDetail{
				Type:      c.class.TypeURI,
				Title:     c.class.Title,
				Status:    c.class.Status,
				Detail:    detail,
				Instance:  instance,
				RequestID: requestID,
			}
		}
	}

	return &ProblemDetail{
		Type:      "about:blank",
		Title:     "Internal Server Error",
		Status:    http.StatusInternalServerError,
		Detail:    "an unexpected error occurred",
		Instance:  instance,
		RequestID: requestID,
	}
}

// WriteError classifies the error and writes a full RFC 7807 response.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	p := MapToRFC7807(err, r)
	WriteProblemFull(w, p)
}

func safeDetail(err error) string {
	if err == nil {
		return ""
	}
	for _, c := range classifications {
		if errors.Is(err, c.sentinel) {
			return err.Error()
		}
	}
	return "an unexpected error occurred"
}
