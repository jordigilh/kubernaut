package httputil

import (
	"net"
	"net/http"
	"strings"
)

// ExtractClientIP returns the client IP from the request, preferring
// X-Forwarded-For (first hop). The port is always stripped.
func ExtractClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.Index(xff, ","); idx != -1 {
			xff = strings.TrimSpace(xff[:idx])
		}
		if ip, _, err := net.SplitHostPort(xff); err == nil {
			return ip
		}
		return xff
	}
	if ip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return ip
	}
	return r.RemoteAddr
}
