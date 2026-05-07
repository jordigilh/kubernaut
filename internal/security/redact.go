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
	urlPattern  = regexp.MustCompile(`https?://[^\s"']+`)
	pathPattern = regexp.MustCompile(`(/[a-zA-Z0-9._-]+){2,}`)
)

const redactedValue = "[REDACTED]"

// RedactMap scrubs values for keys that match known sensitive patterns.
// Returns a new map; the original is not modified.
func RedactMap(detail map[string]string) map[string]string {
	if detail == nil {
		return nil
	}
	out := make(map[string]string, len(detail))
	for k, v := range detail {
		if isSensitiveKey(k) {
			out[k] = redactedValue
		} else {
			out[k] = v
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
	if idx := strings.Index(msg, "\ngoroutine "); idx >= 0 {
		msg = msg[:idx]
	}
	if len(msg) > 256 {
		msg = msg[:256] + "..."
	}
	return msg
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
