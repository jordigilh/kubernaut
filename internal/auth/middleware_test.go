package auth

import (
	"testing"
)

func TestClassifyAuthError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{"expired", ErrTokenExpired, "token_expired"},
		{"audience", ErrInvalidAudience, "invalid_audience"},
		{"unknown issuer", ErrUnknownIssuer, "unknown_issuer"},
		{"malformed", ErrMalformedToken, "malformed_token"},
		{"circuit open", ErrCircuitOpen, "circuit_open"},
		{"cel", ErrCELValidation, "cel_rule_failed"},
		{"missing expiry", ErrMissingExpiry, "missing_expiry"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyAuthError(tt.err)
			if got != tt.expected {
				t.Errorf("classifyAuthError(%v) = %q, want %q", tt.err, got, tt.expected)
			}
		})
	}
}
