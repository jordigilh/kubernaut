package sanitization

import (
	"fmt"
	"regexp"
	"strings"
)

// Sanitizer sanitizes notification content by redacting secrets and masking PII
type Sanitizer struct {
	secretPatterns []*SanitizationRule
}

// SanitizationRule defines a pattern-based sanitization rule
type SanitizationRule struct {
	Name        string
	Pattern     *regexp.Regexp
	Replacement string
	Description string
}

// NewSanitizer creates a new sanitizer with default patterns
func NewSanitizer() *Sanitizer {
	return &Sanitizer{
		secretPatterns: defaultSecretPatterns(),
	}
}

// Sanitize sanitizes content by applying all redaction rules
// Returns the sanitized content with secrets replaced by ***REDACTED***
func (s *Sanitizer) Sanitize(content string) string {
	result := content

	// Apply secret patterns
	for _, rule := range s.secretPatterns {
		if rule.Pattern.MatchString(result) {
			result = rule.Pattern.ReplaceAllString(result, rule.Replacement)
		}
	}

	return result
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
	output := content

	// Common secret patterns to redact (using simple string matching, not regex)
	secretPatterns := []string{
		"password:", "passwd:", "pwd:",
		"token:", "api_token:", "access_token:",
		"key:", "api_key:", "apikey:",
		"secret:", "client_secret:",
		"credential:", "credentials:",
	}

	// Redact each pattern found in the content
	for _, pattern := range secretPatterns {
		output = s.redactPattern(output, pattern)
	}

	return output
}

// redactPattern redacts secret values for a specific pattern using simple string matching
// Returns the content with all occurrences of the pattern's value redacted
func (s *Sanitizer) redactPattern(content, pattern string) string {
	output := content
	lowerOutput := strings.ToLower(output)

	// Find all occurrences of the pattern (case-insensitive)
	idx := strings.Index(lowerOutput, pattern)
	for idx != -1 {
		// Extract and redact the secret value after the pattern
		valueStart, valueEnd := s.findSecretValueBounds(output, idx+len(pattern))

		if valueEnd > valueStart {
			// Replace the secret value with [REDACTED]
			output = output[:valueStart] + "[REDACTED]" + output[valueEnd:]
			lowerOutput = strings.ToLower(output)
		}

		// Search for next occurrence after the redacted section
		searchStart := idx + len(pattern)
		if searchStart >= len(lowerOutput) {
			break
		}

		remainingIdx := strings.Index(lowerOutput[searchStart:], pattern)
		if remainingIdx == -1 {
			break
		}
		idx = searchStart + remainingIdx
	}

	return output
}

// findSecretValueBounds identifies the start and end positions of a secret value
// Handles quoted and unquoted values, returning the bounds to redact
func (s *Sanitizer) findSecretValueBounds(content string, startPos int) (valueStart, valueEnd int) {
	valueStart = startPos

	// Skip leading whitespace
	valueStart = s.skipWhitespace(content, valueStart)
	if valueStart >= len(content) {
		return valueStart, valueStart
	}

	// Check if value is quoted
	isQuoted, quoteChar := s.isQuotedValue(content, valueStart)
	if isQuoted {
		valueStart++ // Skip opening quote
		valueEnd = s.findClosingQuote(content, valueStart, quoteChar)
		if valueEnd < len(content) {
			valueEnd++ // Include closing quote
		}
	} else {
		valueEnd = s.findValueEnd(content, valueStart)
	}

	return valueStart, valueEnd
}

// skipWhitespace advances the position past any whitespace characters
func (s *Sanitizer) skipWhitespace(content string, pos int) int {
	for pos < len(content) && (content[pos] == ' ' || content[pos] == '\t') {
		pos++
	}
	return pos
}

// isQuotedValue checks if the value at the given position starts with a quote
func (s *Sanitizer) isQuotedValue(content string, pos int) (bool, byte) {
	if pos < len(content) && (content[pos] == '"' || content[pos] == '\'') {
		return true, content[pos]
	}
	return false, 0
}

// findClosingQuote finds the position of the closing quote for a quoted value
func (s *Sanitizer) findClosingQuote(content string, startPos int, quoteChar byte) int {
	for pos := startPos; pos < len(content); pos++ {
		if content[pos] == quoteChar {
			return pos
		}
	}
	return len(content)
}

// findValueEnd finds the end position of an unquoted secret value
// Stops at whitespace, newlines, or common delimiters
func (s *Sanitizer) findValueEnd(content string, startPos int) int {
	for pos := startPos; pos < len(content); pos++ {
		ch := content[pos]
		// Stop at delimiter characters
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' ||
			ch == ',' || ch == '"' || ch == '\'' || ch == '}' || ch == ']' {
			return pos
		}
	}
	return len(content)
}

// defaultSecretPatterns returns built-in secret redaction patterns
func defaultSecretPatterns() []*SanitizationRule {
	return []*SanitizationRule{
		// Password patterns (case-insensitive)
		{
			Name:        "password-json",
			Pattern:     regexp.MustCompile(`(?i)"(password|passwd|pwd)"\s*:\s*"([^"]+)"`),
			Replacement: `"${1}":"***REDACTED***"`,
			Description: "Redact password in JSON",
		},
		{
			Name:        "password",
			Pattern:     regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[:=]\s*["']?([^\s"',}]+)["']?`),
			Replacement: `${1}: ***REDACTED***`,
			Description: "Redact password fields",
		},
		{
			Name:        "password-url",
			Pattern:     regexp.MustCompile(`://([^:/@\s]+):([^@\s]+)@`),
			Replacement: `://${1}:***REDACTED***@`,
			Description: "Redact passwords in URLs",
		},

		// API key patterns
		{
			Name:        "apiKey",
			Pattern:     regexp.MustCompile(`(?i)(api[_-]?key|apikey)\s*[:=]\s*["']?([^\s"',}]+)["']?`),
			Replacement: `${1}: ***REDACTED***`,
			Description: "Redact API keys",
		},
		{
			Name:        "openai-key",
			Pattern:     regexp.MustCompile(`sk-[A-Za-z0-9_\-]{4,}`),
			Replacement: `***REDACTED***`,
			Description: "Redact OpenAI API keys",
		},

		// Token patterns
		{
			Name:        "bearer-token",
			Pattern:     regexp.MustCompile(`(?i)Bearer\s+([A-Za-z0-9\-_\.]+)`),
			Replacement: `Bearer ***REDACTED***`,
			Description: "Redact Bearer tokens",
		},
		{
			Name:        "github-token",
			Pattern:     regexp.MustCompile(`ghp_[A-Za-z0-9]{36,}`),
			Replacement: `***REDACTED***`,
			Description: "Redact GitHub tokens",
		},
		{
			Name:        "access-token",
			Pattern:     regexp.MustCompile(`(?i)(access[_-]?token|accesstoken)\s*[:=]\s*["']?([^\s"',}]+)["']?`),
			Replacement: `${1}=***REDACTED***`,
			Description: "Redact access tokens",
		},
		{
			Name:        "token",
			Pattern:     regexp.MustCompile(`(?i)\btoken\s*[:=]\s*["']?([^\s"',}]+)["']?`),
			Replacement: `token: ***REDACTED***`,
			Description: "Redact generic tokens",
		},

		// Cloud provider credentials
		{
			Name:        "aws-access-key",
			Pattern:     regexp.MustCompile(`(?i)(AWS_ACCESS_KEY_ID|aws_access_key)\s*[:=]\s*["']?([A-Z0-9]{20})["']?`),
			Replacement: `${1}=***REDACTED***`,
			Description: "Redact AWS access keys",
		},
		{
			Name:        "aws-secret-key",
			Pattern:     regexp.MustCompile(`(?i)(AWS_SECRET_ACCESS_KEY|aws_secret_key)\s*[:=]\s*["']?([A-Za-z0-9/+=]{40})["']?`),
			Replacement: `${1}=***REDACTED***`,
			Description: "Redact AWS secret keys",
		},

		// Database connection strings
		{
			Name:        "postgresql-url",
			Pattern:     regexp.MustCompile(`postgresql://([^:]+):([^@]+)@`),
			Replacement: `postgresql://${1}:***REDACTED***@`,
			Description: "Redact PostgreSQL URLs",
		},
		{
			Name:        "mysql-url",
			Pattern:     regexp.MustCompile(`mysql://([^:]+):([^@]+)@`),
			Replacement: `mysql://${1}:***REDACTED***@`,
			Description: "Redact MySQL URLs",
		},
		{
			Name:        "mongodb-url",
			Pattern:     regexp.MustCompile(`mongodb://([^:]+):([^@]+)@`),
			Replacement: `mongodb://${1}:***REDACTED***@`,
			Description: "Redact MongoDB URLs",
		},

		// Certificate patterns
		{
			Name:        "pem-certificate",
			Pattern:     regexp.MustCompile(`-----BEGIN CERTIFICATE-----[\s\S]*?-----END CERTIFICATE-----`),
			Replacement: `***REDACTED***`,
			Description: "Redact PEM certificates",
		},
		{
			Name:        "private-key",
			Pattern:     regexp.MustCompile(`-----BEGIN (?:RSA |EC )?PRIVATE KEY-----[\s\S]*?-----END (?:RSA |EC )?PRIVATE KEY-----`),
			Replacement: `***REDACTED***`,
			Description: "Redact private keys",
		},

		// Kubernetes secrets (base64 encoded values)
		{
			Name:        "k8s-secret-data",
			Pattern:     regexp.MustCompile(`(?m)^\s*(username|password|token|key|secret|credential):\s*([A-Za-z0-9+/=]{8,})\s*$`),
			Replacement: `  ${1}: ***REDACTED***`,
			Description: "Redact Kubernetes secret data",
		},
	}
}

