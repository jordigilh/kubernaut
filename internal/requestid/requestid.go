package requestid

import (
	"context"
	"net/http"
	"regexp"

	"github.com/google/uuid"
)

type requestIDKeyType struct{}

var requestIDKey = requestIDKeyType{}

const headerName = "X-Request-ID"

// maxIDLen limits accepted X-Request-ID values to prevent log injection.
const maxIDLen = 128

// validIDPattern accepts UUID-like strings: alphanumeric, dashes, underscores, and dots.
// Dots are allowed to support hierarchical correlation IDs (e.g., "req.sub-span.123").
// Note: some log aggregators (e.g., Elasticsearch) may expand dotted field values;
// consumers should use the raw string value, not interpret dots as path separators.
var validIDPattern = regexp.MustCompile(`^[a-zA-Z0-9\-_\.]{1,128}$`)

// Middleware generates or accepts an X-Request-ID, stores it in context,
// and returns it in the response header (DD-005).
// Client-provided IDs are validated: must match [a-zA-Z0-9\-_\.]{1,128}.
// Invalid or missing IDs are replaced with a fresh UUID.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(headerName)
		if !isValidID(id) {
			id = uuid.New().String()
		}

		ctx := context.WithValue(r.Context(), requestIDKey, id)
		w.Header().Set(headerName, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// FromContext extracts the request ID from context.
func FromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

func isValidID(id string) bool {
	if id == "" || len(id) > maxIDLen {
		return false
	}
	return validIDPattern.MatchString(id)
}
