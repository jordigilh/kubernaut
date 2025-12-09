package sanitization

import (
	"fmt"
	"strings"

	sharedsanitization "github.com/jordigilh/kubernaut/pkg/shared/sanitization"
)

// Sanitizer sanitizes notification content by redacting secrets and masking PII.
// This is a thin wrapper around the shared sanitization package with notification-specific
// behavior (***REDACTED*** placeholder instead of [REDACTED]).
type Sanitizer struct {
	sharedSanitizer *sharedsanitization.Sanitizer
}

// SanitizationRule defines a pattern-based sanitization rule.
// Deprecated: Use sharedsanitization.Rule from pkg/shared/sanitization instead.
type SanitizationRule = sharedsanitization.Rule

// NewSanitizer creates a new sanitizer with default patterns
func NewSanitizer() *Sanitizer {
	return &Sanitizer{
		sharedSanitizer: sharedsanitization.NewSanitizer(),
	}
}

// Sanitize sanitizes content by applying all redaction rules
// Returns the sanitized content with secrets replaced by ***REDACTED***
func (s *Sanitizer) Sanitize(content string) string {
	// Use shared sanitizer and convert placeholder format for backwards compatibility
	result := s.sharedSanitizer.Sanitize(content)
	// Notification uses ***REDACTED*** while shared uses [REDACTED]
	return strings.ReplaceAll(result, sharedsanitization.RedactedPlaceholder, "***REDACTED***")
}

// ==============================================
// v3.1 Enhancement: Category E - Data Sanitization Failure Handling
// ==============================================

// SanitizeWithFallback sanitizes content with automatic fallback on errors
// Category E: Data Sanitization Failures
// When: Redaction logic error, malformed notification data
// Action: Log error, send notification with "[SANITIZATION_ERROR]" prefix
// Recovery: Automatic (degraded delivery)
//
// BR-NOT-055: Graceful Degradation
func (s *Sanitizer) SanitizeWithFallback(content string) (string, error) {
	// Attempt normal sanitization with panic recovery
	var result string
	var sanitizationErr error

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Panic during sanitization - this indicates a regex engine error
				// or malformed pattern
				sanitizationErr = fmt.Errorf("sanitization panic recovered: %v", r)
			}
		}()

		// Try normal sanitization
		sanitized := s.Sanitize(content)

		// If no panic occurred, sanitization succeeded
		if sanitizationErr == nil {
			result = sanitized
		}
	}()

	// If sanitization failed (panic recovered), use safe fallback
	if sanitizationErr != nil {
		fallbackContent := s.SafeFallback(content)
		return fallbackContent, sanitizationErr
	}

	// Sanitization succeeded
	return result, nil
}

// SafeFallback provides a safe fallback when sanitization fails
// Uses simple string matching (no regex) to redact common secret patterns
// This ensures notification can still be delivered even if regex engine fails
//
// BR-NOT-055: Graceful Degradation
func (s *Sanitizer) SafeFallback(content string) string {
	// Use shared sanitizer's safe fallback and convert placeholder format
	result := s.sharedSanitizer.SafeFallback(content)
	return strings.ReplaceAll(result, sharedsanitization.RedactedPlaceholder, "***REDACTED***")
}
