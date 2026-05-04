package requestid

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

type requestIDKeyType struct{}

var requestIDKey = requestIDKeyType{}

const headerName = "X-Request-ID"

// Middleware generates or accepts an X-Request-ID, stores it in context,
// and returns it in the response header (DD-005).
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(headerName)
		if id == "" {
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
