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
	"net/http"
	"strings"
)

// SensitiveHeaderNames contains header names that should always be redacted.
// Case-insensitive matching is used.
var SensitiveHeaderNames = []string{
	"authorization",
	"x-api-key",
	"x-auth-token",
	"x-access-token",
	"cookie",
	"set-cookie",
	"proxy-authorization",
	"www-authenticate",
	"bearer",
	"token",
	"password",
	"secret",
	"api_key",
	"apikey",
}

// SanitizeHeaders redacts sensitive information from HTTP headers.
// Returns a string representation suitable for logging.
//
// Example:
//
//	headers := http.Header{"Authorization": {"Bearer secret"}, "Content-Type": {"application/json"}}
//	sanitized := sanitization.SanitizeHeaders(headers)
//	// Returns: "Authorization: [REDACTED], Content-Type: application/json"
func SanitizeHeaders(headers http.Header) string {
	var sanitized []string

	for key, values := range headers {
		if IsHeaderSensitive(key) {
			sanitized = append(sanitized, key+": "+RedactedPlaceholder)
		} else {
			for _, value := range values {
				sanitized = append(sanitized, key+": "+value)
			}
		}
	}

	return strings.Join(sanitized, ", ")
}

// SanitizeHeadersToMap returns a sanitized copy of headers as a map.
// Useful when you need to preserve the map structure.
func SanitizeHeadersToMap(headers http.Header) map[string][]string {
	result := make(map[string][]string)

	for key, values := range headers {
		if IsHeaderSensitive(key) {
			result[key] = []string{RedactedPlaceholder}
		} else {
			result[key] = values
		}
	}

	return result
}

// IsHeaderSensitive checks if a header name contains sensitive keywords.
// Uses case-insensitive matching against known sensitive header patterns.
func IsHeaderSensitive(headerName string) bool {
	lowerKey := strings.ToLower(headerName)
	for _, sensitiveField := range SensitiveHeaderNames {
		if strings.Contains(lowerKey, sensitiveField) {
			return true
		}
	}
	return false
}

// AddSensitiveHeader adds a custom header name to the sensitive list.
// Use this for service-specific headers that should be redacted.
//
// Example:
//
//	sanitization.AddSensitiveHeader("x-custom-secret")
func AddSensitiveHeader(headerName string) {
	SensitiveHeaderNames = append(SensitiveHeaderNames, strings.ToLower(headerName))
}

