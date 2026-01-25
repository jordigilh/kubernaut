/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package sanitization

import (
	"fmt"
	"regexp"
	"strings"
)

// RedactedPlaceholder is the standard replacement for sensitive data.
// Use this constant for consistency across the codebase.
const RedactedPlaceholder = "[REDACTED]"

// Rule defines a pattern-based sanitization rule.
// Each rule matches a specific type of sensitive data and replaces it.
type Rule struct {
	Name        string         // Human-readable name for debugging
	Pattern     *regexp.Regexp // Regex pattern to match sensitive data
	Replacement string         // Replacement string (may use capture groups)
	Description string         // Description of what this rule redacts
}

// Sanitizer provides configurable log sanitization.
// Use NewSanitizer() to create with default patterns,
// or NewSanitizerWithRules() for custom patterns.
type Sanitizer struct {
	rules []*Rule
}

// NewSanitizer creates a sanitizer with comprehensive default patterns.
// Covers passwords, API keys, tokens, database URLs, certificates, etc.
func NewSanitizer() *Sanitizer {
	return &Sanitizer{
		rules: DefaultRules(),
	}
}

// NewSanitizerWithRules creates a sanitizer with custom rules.
// Use this when you need service-specific patterns.
func NewSanitizerWithRules(rules []*Rule) *Sanitizer {
	return &Sanitizer{
		rules: rules,
	}
}

// Sanitize applies all rules to redact sensitive data from content.
// Returns the sanitized content with sensitive data replaced.
//
// Example:
//
//	s := sanitization.NewSanitizer()
//	clean := s.Sanitize(`{"password":"secret123"}`)
//	// Returns: `{"password":"[REDACTED]"}`
func (s *Sanitizer) Sanitize(content string) string {
	result := content
	for _, rule := range s.rules {
		if rule.Pattern.MatchString(result) {
			result = rule.Pattern.ReplaceAllString(result, rule.Replacement)
		}
	}
	return result
}

// SanitizeWithFallback sanitizes content with automatic fallback on errors.
// If regex processing fails (e.g., panic), falls back to simple string matching.
//
// This implements graceful degradation per BR-NOT-055:
// - When: Redaction logic error, malformed data
// - Action: Log error, use fallback sanitization
// - Recovery: Automatic (degraded but safe)
//
// Returns the sanitized content and any error that occurred.
func (s *Sanitizer) SanitizeWithFallback(content string) (string, error) {
	var result string
	var sanitizationErr error

	func() {
		defer func() {
			if r := recover(); r != nil {
				sanitizationErr = fmt.Errorf("sanitization panic recovered: %v", r)
			}
		}()

		sanitized := s.Sanitize(content)
		if sanitizationErr == nil {
			result = sanitized
		}
	}()

	if sanitizationErr != nil {
		fallbackContent := s.SafeFallback(content)
		return fallbackContent, sanitizationErr
	}

	return result, nil
}

// SafeFallback provides simple string-based sanitization without regex.
// Used when regex engine fails or for ultra-safe processing.
// Uses simple string matching which cannot panic.
func (s *Sanitizer) SafeFallback(content string) string {
	output := content

	// Common secret patterns (simple string matching, no regex)
	patterns := []string{
		"password:", "passwd:", "pwd:",
		"token:", "api_token:", "access_token:",
		"key:", "api_key:", "apikey:",
		"secret:", "client_secret:",
		"credential:", "credentials:",
		"authorization:", "bearer:",
	}

	for _, pattern := range patterns {
		output = redactPatternSimple(output, pattern)
	}

	return output
}

// redactPatternSimple redacts values following a pattern using simple string ops.
// This is the fallback when regex is not safe to use.
func redactPatternSimple(content, pattern string) string {
	output := content
	lowerOutput := strings.ToLower(output)

	idx := strings.Index(lowerOutput, pattern)
	for idx != -1 {
		// Find the value after the pattern
		valueStart := idx + len(pattern)

		// Skip whitespace
		for valueStart < len(output) && (output[valueStart] == ' ' || output[valueStart] == '\t') {
			valueStart++
		}

		if valueStart >= len(output) {
			break
		}

		// Find end of value
		valueEnd := valueStart
		inQuotes := false
		quoteChar := byte(0)

		if output[valueStart] == '"' || output[valueStart] == '\'' {
			inQuotes = true
			quoteChar = output[valueStart]
			valueStart++
			valueEnd = valueStart
		}

		for valueEnd < len(output) {
			ch := output[valueEnd]
			if inQuotes {
				if ch == quoteChar {
					break
				}
			} else {
				if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' ||
					ch == ',' || ch == '"' || ch == '\'' || ch == '}' || ch == ']' {
					break
				}
			}
			valueEnd++
		}

		if valueEnd > valueStart {
			output = output[:valueStart] + RedactedPlaceholder + output[valueEnd:]
			lowerOutput = strings.ToLower(output)
		}

		// Search for next occurrence
		searchStart := idx + len(pattern) + len(RedactedPlaceholder)
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

// SanitizeForLog is a convenience function for quick sanitization.
// Use this for one-off sanitization without creating a Sanitizer instance.
//
// Example:
//
//	logger.Info("Request", "body", sanitization.SanitizeForLog(body))
func SanitizeForLog(data string) string {
	return defaultSanitizer.Sanitize(data)
}

// defaultSanitizer is a shared instance for SanitizeForLog.
var defaultSanitizer = NewSanitizer()

// DefaultRules returns the comprehensive set of sanitization rules.
// These cover the most common sensitive data patterns.
//
// IMPORTANT: Pattern order matters! Container patterns (generatorURL, annotations)
// must come FIRST to prevent sub-patterns from corrupting larger structures.
func DefaultRules() []*Rule {
	return []*Rule{
		// ========================================
		// PRIORITY: Container patterns (process first to prevent corruption)
		// These patterns match larger structures that may contain sub-patterns
		// ========================================
		{
			Name:        "generator-url",
			Pattern:     regexp.MustCompile(`(?i)"generatorURL?"\s*:\s*"([^"]+)"`),
			Replacement: `"generatorURL":"` + RedactedPlaceholder + `"`,
			Description: "Redact Prometheus/Alertmanager generator URLs",
		},
		{
			Name:        "annotations-json",
			Pattern:     regexp.MustCompile(`(?i)"annotations"\s*:\s*\{[^}]*\}`),
			Replacement: `"annotations":` + RedactedPlaceholder,
			Description: "Redact webhook annotations",
		},

		// ========================================
		// Password Patterns
		// ========================================
		{
			Name:        "password-json",
			Pattern:     regexp.MustCompile(`(?i)"(password|passwd|pwd)"\s*:\s*"([^"]+)"`),
			Replacement: `"${1}":"` + RedactedPlaceholder + `"`,
			Description: "Redact password in JSON",
		},
		{
			Name:        "password-plain",
			Pattern:     regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[:=]\s*["']?([^\s"',}]+)["']?`),
			Replacement: `${1}: ` + RedactedPlaceholder,
			Description: "Redact password fields",
		},
		{
			Name:        "password-url",
			Pattern:     regexp.MustCompile(`://([^:/@\s]+):([^@\s]+)@`),
			Replacement: `://${1}:` + RedactedPlaceholder + `@`,
			Description: "Redact passwords in URLs",
		},

		// ========================================
		// API Key Patterns
		// ========================================
		{
			Name:        "api-key-json",
			Pattern:     regexp.MustCompile(`(?i)"(api[_-]?key|apikey)"\s*:\s*"([^"]+)"`),
			Replacement: `"${1}":"` + RedactedPlaceholder + `"`,
			Description: "Redact API key in JSON",
		},
		{
			Name:        "api-key-plain",
			Pattern:     regexp.MustCompile(`(?i)(api[_-]?key|apikey)\s*[:=]\s*["']?([^\s"',}]+)["']?`),
			Replacement: `${1}: ` + RedactedPlaceholder,
			Description: "Redact API keys",
		},
		{
			Name:        "openai-key",
			Pattern:     regexp.MustCompile(`sk-[A-Za-z0-9_\-]{4,}`),
			Replacement: RedactedPlaceholder,
			Description: "Redact OpenAI API keys",
		},

		// ========================================
		// Token Patterns
		// ========================================
		{
			Name:        "bearer-token",
			Pattern:     regexp.MustCompile(`(?i)Bearer\s+([A-Za-z0-9\-_\.]+)`),
			Replacement: `Bearer ` + RedactedPlaceholder,
			Description: "Redact Bearer tokens",
		},
		{
			Name:        "github-token",
			Pattern:     regexp.MustCompile(`ghp_[A-Za-z0-9]{36,}`),
			Replacement: RedactedPlaceholder,
			Description: "Redact GitHub tokens",
		},
		{
			Name:        "token-json",
			Pattern:     regexp.MustCompile(`(?i)"(token|access[_-]?token)"\s*:\s*"([^"]+)"`),
			Replacement: `"${1}":"` + RedactedPlaceholder + `"`,
			Description: "Redact token in JSON",
		},
		{
			Name:        "token-plain",
			Pattern:     regexp.MustCompile(`(?i)\b(token|access[_-]?token)\s*[:=]\s*["']?([^\s"',}]+)["']?`),
			Replacement: `${1}: ` + RedactedPlaceholder,
			Description: "Redact generic tokens",
		},

		// ========================================
		// Secret Patterns
		// ========================================
		{
			Name:        "secret-json",
			Pattern:     regexp.MustCompile(`(?i)"(secret|client_secret)"\s*:\s*"([^"]+)"`),
			Replacement: `"${1}":"` + RedactedPlaceholder + `"`,
			Description: "Redact secret in JSON",
		},
		{
			Name:        "secret-plain",
			Pattern:     regexp.MustCompile(`(?i)(secret|client_secret)\s*[:=]\s*["']?([^\s"',}]+)["']?`),
			Replacement: `${1}: ` + RedactedPlaceholder,
			Description: "Redact secrets",
		},

		// ========================================
		// Authorization Headers
		// ========================================
		{
			Name:        "authorization-header",
			Pattern:     regexp.MustCompile(`(?i)(authorization)\s*:\s*["']?([^\s"',}]+)["']?`),
			Replacement: `${1}: ` + RedactedPlaceholder,
			Description: "Redact authorization headers",
		},

		// ========================================
		// Cloud Provider Credentials
		// ========================================
		{
			Name:        "aws-access-key",
			Pattern:     regexp.MustCompile(`(?i)(AWS_ACCESS_KEY_ID|aws_access_key)\s*[:=]\s*["']?([A-Z0-9]{20})["']?`),
			Replacement: `${1}=` + RedactedPlaceholder,
			Description: "Redact AWS access keys",
		},
		{
			Name:        "aws-secret-key",
			Pattern:     regexp.MustCompile(`(?i)(AWS_SECRET_ACCESS_KEY|aws_secret_key)\s*[:=]\s*["']?([A-Za-z0-9/+=]{40})["']?`),
			Replacement: `${1}=` + RedactedPlaceholder,
			Description: "Redact AWS secret keys",
		},

		// ========================================
		// Database Connection Strings
		// ========================================
		{
			Name:        "postgresql-url",
			Pattern:     regexp.MustCompile(`postgresql://([^:]+):([^@]+)@`),
			Replacement: `postgresql://${1}:` + RedactedPlaceholder + `@`,
			Description: "Redact PostgreSQL URLs",
		},
		{
			Name:        "mysql-url",
			Pattern:     regexp.MustCompile(`mysql://([^:]+):([^@]+)@`),
			Replacement: `mysql://${1}:` + RedactedPlaceholder + `@`,
			Description: "Redact MySQL URLs",
		},
		{
			Name:        "mongodb-url",
			Pattern:     regexp.MustCompile(`mongodb://([^:]+):([^@]+)@`),
			Replacement: `mongodb://${1}:` + RedactedPlaceholder + `@`,
			Description: "Redact MongoDB URLs",
		},
		{
			Name:        "redis-url",
			Pattern:     regexp.MustCompile(`redis://([^:]+):([^@]+)@`),
			Replacement: `redis://${1}:` + RedactedPlaceholder + `@`,
			Description: "Redact Redis URLs",
		},

		// ========================================
		// Certificates and Keys
		// ========================================
		{
			Name:        "pem-certificate",
			Pattern:     regexp.MustCompile(`-----BEGIN CERTIFICATE-----[\s\S]*?-----END CERTIFICATE-----`),
			Replacement: RedactedPlaceholder,
			Description: "Redact PEM certificates",
		},
		{
			Name:        "private-key",
			Pattern:     regexp.MustCompile(`-----BEGIN (?:RSA |EC )?PRIVATE KEY-----[\s\S]*?-----END (?:RSA |EC )?PRIVATE KEY-----`),
			Replacement: RedactedPlaceholder,
			Description: "Redact private keys",
		},

		// ========================================
		// Kubernetes Secrets
		// ========================================
		{
			Name:        "k8s-secret-data",
			Pattern:     regexp.MustCompile(`(?m)^\s*(username|password|token|key|secret|credential):\s*([A-Za-z0-9+/=]{8,})\s*$`),
			Replacement: `  ${1}: ` + RedactedPlaceholder,
			Description: "Redact Kubernetes secret data (base64)",
		},

		// ========================================
		// Prometheus/Alertmanager URLs (may contain internal info)
		// ========================================
		{
			Name:        "generator-url",
			Pattern:     regexp.MustCompile(`(?i)"generatorURL?"\s*:\s*"([^"]+)"`),
			Replacement: `"generatorURL":"` + RedactedPlaceholder + `"`,
			Description: "Redact Prometheus/Alertmanager generator URLs",
		},

		// ========================================
		// Webhook Annotations (may contain sensitive data)
		// ========================================
		{
			Name:        "annotations-json",
			Pattern:     regexp.MustCompile(`(?i)"annotations"\s*:\s*\{[^}]*\}`),
			Replacement: `"annotations":` + RedactedPlaceholder,
			Description: "Redact webhook annotations",
		},

		// ========================================
		// PII Patterns (BR-GATEWAY-042)
		// ========================================
		{
			Name:        "email-address",
			Pattern:     regexp.MustCompile(`\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Z|a-z]{2,}\b`),
			Replacement: RedactedPlaceholder,
			Description: "Redact email addresses (PII)",
		},
		{
			Name:        "ipv4-address",
			Pattern:     regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\b`),
			Replacement: RedactedPlaceholder,
			Description: "Redact IPv4 addresses (internal infrastructure)",
		},
	}
}
