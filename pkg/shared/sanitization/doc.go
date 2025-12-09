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

// Package sanitization provides DD-005 compliant log sanitization utilities.
//
// This package consolidates sanitization logic.
//
// Migration Status (December 2025):
// - pkg/notification/sanitization/sanitizer.go → MIGRATED ✅ (uses this package)
// - pkg/gateway/middleware/log_sanitization.go → DELETED ✅ (migrated Dec 9, 2025)
//
// All services MUST use this package for log sanitization to ensure:
// - Consistent redaction patterns across the codebase
// - DD-005 compliance (Observability Standards)
// - Security: No sensitive data in logs (CVSS 5.3)
//
// # Usage
//
// For simple string sanitization:
//
//	sanitized := sanitization.SanitizeForLog(sensitiveData)
//	logger.Info("Processing request", "payload", sanitized)
//
// For HTTP middleware:
//
//	router.Use(sanitization.NewLoggingMiddleware(logger))
//
// For notification content:
//
//	sanitizer := sanitization.NewSanitizer()
//	clean, err := sanitizer.SanitizeWithFallback(content)
//
// # Patterns Covered
//
// The sanitizer redacts the following sensitive patterns:
//
//   - Passwords: password, passwd, pwd (JSON, URL, plain text)
//   - API Keys: api_key, apikey, OpenAI (sk-*), AWS keys
//   - Tokens: Bearer, GitHub (ghp_*), access_token, generic token
//   - Secrets: secret, client_secret, credential
//   - Database URLs: PostgreSQL, MySQL, MongoDB connection strings
//   - Certificates: PEM certificates, private keys
//   - Kubernetes: Secret data (base64 encoded)
//   - HTTP Headers: Authorization, Bearer, X-API-Key
//
// # DD-005 Compliance
//
// Authority: docs/architecture/decisions/DD-005-OBSERVABILITY-STANDARDS.md
//
// Per DD-005 Lines 519-555:
//
//	"Sensitive data MUST be redacted before logging"
//	"Sensitive Fields (MUST be redacted): password, token, api_key, secret, authorization"
//
// # Business Requirements
//
//   - BR-GATEWAY-078: Redact sensitive data from logs
//   - BR-GATEWAY-079: Prevent information disclosure through logs
//   - BR-NOT-055: Graceful degradation for sanitization failures
//   - BR-STORAGE-XXX: Log sanitization (pending implementation)
//
// # Security
//
//   - VULN-GATEWAY-004: Prevents sensitive data exposure in logs (CVSS 5.3)
package sanitization

