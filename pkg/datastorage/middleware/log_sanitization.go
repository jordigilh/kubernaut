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

// Package middleware provides HTTP middleware for Data Storage service.
//
// DD-005 Compliance: Log Sanitization
// This package implements log sanitization per DD-005 Observability Standards.
// Uses the shared sanitization package from pkg/shared/sanitization.
//
// Business Requirements:
// - BR-STORAGE-XXX: Log sanitization for data storage operations
//
// Security:
// - Prevents sensitive data (passwords, tokens, secrets) from appearing in logs
// - CVSS 5.3 vulnerability prevention
package middleware

import (
	"bytes"
	"io"
	"net/http"

	"github.com/jordigilh/kubernaut/pkg/shared/sanitization"
)

// NewLoggingSanitizer creates middleware that sanitizes request data before logging.
//
// DD-005 Compliance: Per lines 519-555
// "Sensitive data MUST be redacted before logging"
// "Sensitive Fields (MUST be redacted): password, token, api_key, secret, authorization"
//
// Usage:
//
//	router := chi.NewRouter()
//	router.Use(middleware.NewLoggingSanitizer(logger))
func NewLoggingSanitizer(logWriter io.Writer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only log bodies for POST/PUT/PATCH requests with audit data
			if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
				body, err := readAndRestoreBody(r)
				if err == nil && logWriter != nil {
					// Sanitize body and headers before logging
					sanitizedBody := sanitization.SanitizeForLog(string(body))
					sanitizedHeaders := sanitization.SanitizeHeaders(r.Header)

					// Log sanitized request
					_, _ = logWriter.Write([]byte("Request: " + r.Method + " " + sanitization.NormalizePath(r.URL.Path) + "\n"))
					_, _ = logWriter.Write([]byte("Body (sanitized): " + sanitizedBody + "\n"))
					if len(sanitizedHeaders) > 0 {
						_, _ = logWriter.Write([]byte("Headers (sanitized): " + sanitizedHeaders + "\n"))
					}
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// readAndRestoreBody reads the request body and restores it for downstream handlers.
func readAndRestoreBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	// Restore body for downstream handlers
	r.Body = io.NopCloser(bytes.NewBuffer(body))
	return body, nil
}

// SanitizeForLog is a convenience function for sanitizing log data.
// Re-exported from shared package for ease of use.
//
// Example:
//
//	log.Info("Processing audit event", "payload", middleware.SanitizeForLog(payload))
func SanitizeForLog(data string) string {
	return sanitization.SanitizeForLog(data)
}

// NormalizePath normalizes URL paths for metrics to prevent high cardinality.
// Re-exported from shared package for ease of use.
//
// DD-005 Compliance: Per lines 689-710
// "Path normalization is MANDATORY for all services exposing HTTP metrics"
//
// Example:
//
//	normalizedPath := middleware.NormalizePath("/api/v1/events/550e8400-e29b-41d4-a716-446655440000")
//	// Returns: "/api/v1/events/:id"
func NormalizePath(path string) string {
	return sanitization.NormalizePath(path)
}

