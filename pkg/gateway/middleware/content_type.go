package middleware

import (
	"mime"
	"net/http"
)

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
			// Invalid Content-Type format
			w.Header().Set("Accept", "application/json")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnsupportedMediaType)
			w.Write([]byte(`{"error":"Invalid Content-Type header format","status":415}`))
			return
		}

		// Validate that media type is application/json
		if mediaType != "application/json" {
			w.Header().Set("Accept", "application/json")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnsupportedMediaType)
			w.Write([]byte(`{"error":"Content-Type must be application/json","status":415}`))
			return
		}

		// Content-Type is valid, proceed to next handler
		next.ServeHTTP(w, r)
	})
}
