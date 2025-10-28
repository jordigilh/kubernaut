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

package middleware

import (
	"bytes"
	"io"
	"net/http"
	"regexp"
	"strings"
)

const redactedPlaceholder = "[REDACTED]"

// sanitizationPattern represents a pattern to sanitize and its replacement
type sanitizationPattern struct {
	pattern     *regexp.Regexp
	replacement string
}

// Sensitive field patterns to redact
var (
	// Common sensitive field names (case-insensitive)
	sensitiveFieldNames = []string{
		"password",
		"token",
		"api_key",
		"apikey",
		"secret",
		"authorization",
		"bearer",
		"annotations",
		"generatorURL",
		"generatorUrl",
	}

	// REFACTORED: Consolidated patterns with their replacements
	// This eliminates duplication and makes it easier to add new patterns
	sanitizationPatterns = []sanitizationPattern{
		{
			pattern:     regexp.MustCompile(`(?i)"password"\s*:\s*"([^"]+)"`),
			replacement: `"password":"[REDACTED]"`,
		},
		{
			pattern:     regexp.MustCompile(`(?i)"token"\s*:\s*"([^"]+)"`),
			replacement: `"token":"[REDACTED]"`,
		},
		{
			pattern:     regexp.MustCompile(`(?i)"api_key"\s*:\s*"([^"]+)"`),
			replacement: `"api_key":"[REDACTED]"`,
		},
		{
			pattern:     regexp.MustCompile(`(?i)"annotations"\s*:\s*\{[^}]*\}`),
			replacement: `"annotations":[REDACTED]`,
		},
		{
			pattern:     regexp.MustCompile(`(?i)"generatorURL?"\s*:\s*"([^"]+)"`),
			replacement: `"generatorURL":"[REDACTED]"`,
		},
	}
)

// NewSanitizingLogger creates middleware that sanitizes sensitive data from logs.
//
// Business Requirements:
// - BR-GATEWAY-078: Redact sensitive data from logs
// - BR-GATEWAY-079: Prevent information disclosure through logs
//
// Security:
// - VULN-GATEWAY-004: Prevents sensitive data exposure in logs (CVSS 5.3)
//
// This middleware sanitizes:
// - Password fields
// - API keys and tokens
// - Authorization headers
// - Webhook annotations (may contain sensitive data)
// - Generator URLs (may contain internal endpoints)
//
// Sanitization Flow:
// 1. Capture request body for sanitization
// 2. Sanitize request body before logging
// 3. Sanitize response data if needed
// 4. Continue to next handler with original (unsanitized) data
//
// Note: This middleware only sanitizes what gets logged, not the actual request/response data.
//
// REFACTORED: Extracted helper functions for better separation of concerns
func NewSanitizingLogger(logWriter io.Writer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// REFACTORED: Extracted body reading logic
			body, err := readAndRestoreBody(r)
			if err != nil {
				// If we can't read the body, continue without sanitization
				next.ServeHTTP(w, r)
				return
			}

			// REFACTORED: Extracted logging logic
			logSanitizedRequest(logWriter, body, r.Header)

			// Continue to next handler with original (unsanitized) data
			next.ServeHTTP(w, r)
		})
	}
}

// readAndRestoreBody reads the request body and restores it for downstream handlers.
// REFACTORED: Extracted from NewSanitizingLogger for better testability
func readAndRestoreBody(r *http.Request) ([]byte, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	// Restore body for downstream handlers
	r.Body = io.NopCloser(bytes.NewBuffer(body))
	return body, nil
}

// logSanitizedRequest logs sanitized request body and headers.
// REFACTORED: Extracted from NewSanitizingLogger for better separation of concerns
func logSanitizedRequest(logWriter io.Writer, body []byte, headers http.Header) {
	if logWriter == nil {
		return
	}

	// Sanitize and log body
	sanitizedBody := sanitizeData(string(body))
	_, _ = logWriter.Write([]byte("Request body (sanitized): " + sanitizedBody + "\n"))

	// Sanitize and log headers
	sanitizedHeaders := sanitizeHeaders(headers)
	if len(sanitizedHeaders) > 0 {
		_, _ = logWriter.Write([]byte("Headers (sanitized): " + sanitizedHeaders + "\n"))
	}
}

// sanitizeData redacts sensitive information from data string.
// REFACTORED: Use consolidated patterns to eliminate duplication
func sanitizeData(data string) string {
	// Apply all sanitization patterns
	for _, sp := range sanitizationPatterns {
		data = sp.pattern.ReplaceAllString(data, sp.replacement)
	}
	return data
}

// sanitizeHeaders redacts sensitive information from HTTP headers.
// REFACTORED: Extracted helper function for sensitivity check
func sanitizeHeaders(headers http.Header) string {
	var sanitized []string

	for key, values := range headers {
		if isHeaderSensitive(key) {
			sanitized = append(sanitized, key+": "+redactedPlaceholder)
		} else {
			// Non-sensitive headers can be logged
			for _, value := range values {
				sanitized = append(sanitized, key+": "+value)
			}
		}
	}

	return strings.Join(sanitized, ", ")
}

// isHeaderSensitive checks if a header name contains sensitive keywords.
// REFACTORED: Extracted from sanitizeHeaders for better testability
func isHeaderSensitive(headerName string) bool {
	lowerKey := strings.ToLower(headerName)
	for _, sensitiveField := range sensitiveFieldNames {
		if strings.Contains(lowerKey, sensitiveField) {
			return true
		}
	}
	return false
}

// SanitizeForLog is a helper function to sanitize any string for logging.
// This can be used by other components to sanitize data before logging.
//
// Example:
//
//	logger.WithField("payload", middleware.SanitizeForLog(webhookData)).Info("Processing webhook")
func SanitizeForLog(data string) string {
	return sanitizeData(data)
}
