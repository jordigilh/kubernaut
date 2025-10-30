package middleware

import (
	"encoding/json"
	"mime"
	"net/http"
)

// RFC7807Error represents an RFC 7807 Problem Details error response
// BR-041: RFC 7807 error format
type RFC7807Error struct {
	Type     string `json:"type"`               // URI reference identifying the problem type
	Title    string `json:"title"`              // Short, human-readable summary
	Detail   string `json:"detail"`             // Human-readable explanation
	Status   int    `json:"status"`             // HTTP status code
	Instance string `json:"instance"`           // URI reference to specific occurrence
}

// ValidateContentType is a middleware that validates the Content-Type header
// BR-042: Content-Type validation
// BUSINESS OUTCOME: Reject invalid webhook payloads early, before processing
func ValidateContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType := r.Header.Get("Content-Type")

		// Allow missing Content-Type for now (backward compatibility during grace period)
		if contentType == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Parse media type to handle charset parameters
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err != nil {
			// Invalid Content-Type format - return RFC 7807 error
			writeRFC7807Error(w, r, "https://kubernaut.io/errors/validation-error", "Bad Request", "Invalid Content-Type header format", http.StatusBadRequest)
			return
		}

		// Validate that media type is application/json
		if mediaType != "application/json" {
			// Non-JSON Content-Type - return RFC 7807 error
			w.Header().Set("Accept", "application/json")
			writeRFC7807Error(w, r, "https://kubernaut.io/errors/unsupported-media-type", "Unsupported Media Type", "Content-Type must be application/json", http.StatusUnsupportedMediaType)
			return
		}

		// Content-Type is valid, proceed to next handler
		next.ServeHTTP(w, r)
	})
}

// writeRFC7807Error writes an RFC 7807 compliant error response
// BR-041: RFC 7807 error format
func writeRFC7807Error(w http.ResponseWriter, r *http.Request, errorType, title, detail string, statusCode int) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(statusCode)

	errorResponse := RFC7807Error{
		Type:     errorType,
		Title:    title,
		Detail:   detail,
		Status:   statusCode,
		Instance: r.URL.Path,
	}

	if err := json.NewEncoder(w).Encode(errorResponse); err != nil {
		// Fallback to plain text if JSON encoding fails
		http.Error(w, detail, statusCode)
	}
}
