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

	"github.com/jordigilh/kubernaut/pkg/shared/sanitization"
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
// REFACTORED: Now uses shared sanitization package (pkg/shared/sanitization)
func NewSanitizingLogger(logWriter io.Writer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := readAndRestoreBody(r)
			if err != nil {
				// If we can't read the body, continue without sanitization
				next.ServeHTTP(w, r)
				return
			}

			logSanitizedRequest(logWriter, body, r.Header)

			// Continue to next handler with original (unsanitized) data
			next.ServeHTTP(w, r)
		})
	}
}

// readAndRestoreBody reads the request body and restores it for downstream handlers.
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
func logSanitizedRequest(logWriter io.Writer, body []byte, headers http.Header) {
	if logWriter == nil {
		return
	}

	// Sanitize and log body using shared package
	sanitizedBody := sanitization.SanitizeForLog(string(body))
	_, _ = logWriter.Write([]byte("Request body (sanitized): " + sanitizedBody + "\n"))

	// Sanitize and log headers using shared package
	sanitizedHeaders := sanitization.SanitizeHeaders(headers)
	if len(sanitizedHeaders) > 0 {
		_, _ = logWriter.Write([]byte("Headers (sanitized): " + sanitizedHeaders + "\n"))
	}
}

// SanitizeForLog is a helper function to sanitize any string for logging.
// This can be used by other components to sanitize data before logging.
//
// Deprecated: Use sanitization.SanitizeForLog from pkg/shared/sanitization instead.
// This function is kept for backwards compatibility.
//
// Example:
//
//	logger.WithField("payload", middleware.SanitizeForLog(webhookData)).Info("Processing webhook")
func SanitizeForLog(data string) string {
	return sanitization.SanitizeForLog(data)
}
