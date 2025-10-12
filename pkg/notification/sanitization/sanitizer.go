package sanitization

import (
	"regexp"
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
