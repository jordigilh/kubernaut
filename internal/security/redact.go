package security

import (
	"regexp"
	"strings"
)

var sensitiveKeys = []string{
	"password", "token", "api_key", "apikey", "secret",
	"authorization", "bearer", "credential", "cookie",
	"private_key", "privatekey", "access_key", "accesskey",
}

var (
	urlPattern  = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://[^\s"']+`)
	pathPattern = regexp.MustCompile(`(/[a-zA-Z0-9._-]+){2,}`)

	// Value patterns for detecting secrets embedded in arbitrary values.
	jwtPattern    = regexp.MustCompile(`eyJ[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{20,}`)
	bearerPattern = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._~+/=-]{10,}`)
	base64Secret  = regexp.MustCompile(`[A-Za-z0-9+/]{40,}={0,2}`)
)

const redactedValue = "[REDACTED]"

// RedactMap scrubs values for keys that match known sensitive patterns,
// and also scrubs values whose content matches common secret formats
// (JWTs, bearer tokens, long base64 strings). Returns a new map.
func RedactMap(detail map[string]string) map[string]string {
	if detail == nil {
		return nil
	}
	out := make(map[string]string, len(detail))
	for k, v := range detail {
		if isSensitiveKey(k) {
			out[k] = redactedValue
		} else {
			out[k] = redactValue(v)
		}
	}
	return out
}

// RedactError strips URLs, file paths, and stack traces from error messages.
// Returns a safe summary suitable for client-facing responses and audit logs.
func RedactError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	msg = urlPattern.ReplaceAllString(msg, "[URL_REDACTED]")
	msg = pathPattern.ReplaceAllString(msg, "[PATH_REDACTED]")
	msg = jwtPattern.ReplaceAllString(msg, "[JWT_REDACTED]")
	msg = bearerPattern.ReplaceAllString(msg, "[BEARER_REDACTED]")
	if idx := strings.Index(msg, "\ngoroutine "); idx >= 0 {
		msg = msg[:idx]
	}
	if len(msg) > 256 {
		msg = msg[:256] + "..."
	}
	return msg
}

// redactValue applies value-level pattern matching to detect and redact
// embedded secrets regardless of the map key.
func redactValue(v string) string {
	if jwtPattern.MatchString(v) {
		v = jwtPattern.ReplaceAllString(v, "[JWT_REDACTED]")
	}
	if bearerPattern.MatchString(v) {
		v = bearerPattern.ReplaceAllString(v, "[BEARER_REDACTED]")
	}
	if base64Secret.MatchString(v) && looksLikeSecret(v) {
		return redactedValue
	}
	return v
}

// looksLikeSecret heuristically checks if a value is entirely (or nearly) a
// base64 secret vs legitimate data like a description or error message.
func looksLikeSecret(v string) bool {
	if len(v) < 40 {
		return false
	}
	nonAlnum := 0
	for _, r := range v {
		if r == ' ' || r == '\n' || r == '\t' {
			nonAlnum++
		}
	}
	return float64(nonAlnum)/float64(len(v)) < 0.05
}

func isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, s := range sensitiveKeys {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}
