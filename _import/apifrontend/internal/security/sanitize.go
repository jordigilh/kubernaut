package security

import (
	"errors"
	"strings"
	"unicode"
)

// ErrControlChars is returned when a header value contains control characters.
var ErrControlChars = errors.New("header value contains control characters")

// ValidateHeaderValue checks that the given header value contains no control characters
// (0x00-0x1F except HT 0x09, and DEL 0x7F). Returns ErrControlChars if invalid.
func ValidateHeaderValue(s string) error {
	for _, r := range s {
		if r == '\t' {
			continue
		}
		if unicode.IsControl(r) {
			return ErrControlChars
		}
	}
	return nil
}

// SanitizeClaimValue strips control characters (CR, LF, NUL, etc.) from
// extracted JWT claim values to prevent log injection and header corruption
// when used in impersonation headers or structured logs (SEC-6).
func SanitizeClaimValue(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}
