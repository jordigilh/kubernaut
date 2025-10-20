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

// SanitizationMetrics tracks sanitization statistics
type SanitizationMetrics struct {
	RedactedCount int
	Patterns      []string
}

// NewSanitizer creates a new sanitizer with default patterns
func NewSanitizer() *Sanitizer {
	return &Sanitizer{
		secretPatterns: defaultSecretPatterns(),
	}
}

// Sanitize sanitizes content by applying all redaction rules
func (s *Sanitizer) Sanitize(content string) string {
	result, _ := s.SanitizeWithMetrics(content)
	return result
}

// SanitizeWithMetrics sanitizes content and returns metrics
func (s *Sanitizer) SanitizeWithMetrics(content string) (string, *SanitizationMetrics) {
	metrics := &SanitizationMetrics{
		Patterns: []string{},
	}

	result := content

	// Apply secret patterns
	for _, rule := range s.secretPatterns {
		if rule.Pattern.MatchString(result) {
			result = rule.Pattern.ReplaceAllString(result, rule.Replacement)
			metrics.RedactedCount++
			metrics.Patterns = append(metrics.Patterns, rule.Name)
		}
	}

	return result, metrics
}

// AddCustomPattern adds a custom sanitization pattern
func (s *Sanitizer) AddCustomPattern(rule *SanitizationRule) {
	s.secretPatterns = append(s.secretPatterns, rule)
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
		sanitized, metrics := s.SanitizeWithMetrics(content)
		
		// Check if any patterns matched (metrics.RedactedCount > 0 means patterns were applied)
		// Even if RedactedCount is 0, sanitization succeeded (just nothing to redact)
		if sanitizationErr == nil {
			result = sanitized
			_ = metrics // Use metrics to avoid unused variable
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
	// Use simple string matching instead of regex to avoid same failure mode
	output := content

	// Redact anything after common secret keywords
	secretPatterns := []string{
		"password:", "passwd:", "pwd:",
		"token:", "api_token:", "access_token:",
		"key:", "api_key:", "apikey:",
		"secret:", "client_secret:",
		"credential:", "credentials:",
	}

	for _, pattern := range secretPatterns {
		// Case-insensitive search
		lowerOutput := strings.ToLower(output)
		idx := strings.Index(lowerOutput, pattern)
		
		for idx != -1 {
			// Find the end of the secret value (next space, newline, or end of string)
			valueStart := idx + len(pattern)
			if valueStart >= len(output) {
				break
			}
			
			// Skip whitespace after the colon
			for valueStart < len(output) && (output[valueStart] == ' ' || output[valueStart] == '\t') {
				valueStart++
			}
			
			// Check if value is quoted and skip the opening quote
			isQuoted := false
			var quoteChar byte
			if valueStart < len(output) && (output[valueStart] == '"' || output[valueStart] == '\'') {
				isQuoted = true
				quoteChar = output[valueStart]
				valueStart++ // Skip opening quote
			}
			
			// Find the end of the value
			valueEnd := valueStart
			if isQuoted {
				// For quoted values, find the closing quote
				for valueEnd < len(output) && output[valueEnd] != quoteChar {
					valueEnd++
				}
			} else {
				// For unquoted values, stop at delimiters
				for valueEnd < len(output) {
					ch := output[valueEnd]
					// Stop at whitespace, newline, comma, quote, or bracket
					if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || 
					   ch == ',' || ch == '"' || ch == '\'' || ch == '}' || ch == ']' {
						break
					}
					valueEnd++
				}
			}
			
			// Redact the value
			if valueEnd > valueStart {
				// If quoted, include the closing quote in the redaction
				endPos := valueEnd
				if isQuoted && valueEnd < len(output) {
					endPos++ // Skip closing quote
				}
				output = output[:valueStart] + "[REDACTED]" + output[endPos:]
				// Adjust for the length change
				lowerOutput = strings.ToLower(output)
			}
			
			// Search for next occurrence
			searchStart := idx + len(pattern)
			if searchStart >= len(lowerOutput) {
				break
			}
			idx = strings.Index(lowerOutput[searchStart:], pattern)
			if idx != -1 {
				idx += searchStart
			}
		}
	}

	return output
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
