package auth

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxClaimLen = 256

// SanitizeClaimValue sanitizes a claim value from TokenReview or JWT claims.
// Strips null bytes, control characters (except space), bidi overrides, invalid
// UTF-8 sequences, and truncates to maxClaimLen.
func SanitizeClaimValue(s string) string {
	if s == "" {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))

	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size <= 1 {
			i++
			continue
		}
		if r == 0 {
			i += size
			continue
		}
		if unicode.IsControl(r) && r != ' ' && r != '\t' {
			i += size
			continue
		}
		if isBidiOverride(r) {
			i += size
			continue
		}
		b.WriteRune(r)
		i += size
		if b.Len() >= maxClaimLen {
			break
		}
	}

	result := b.String()
	if len(result) > maxClaimLen {
		result = result[:maxClaimLen]
	}
	return result
}

func isBidiOverride(r rune) bool {
	switch r {
	case '\u202A', '\u202B', '\u202C', '\u202D', '\u202E',
		'\u2066', '\u2067', '\u2068', '\u2069':
		return true
	}
	return false
}
