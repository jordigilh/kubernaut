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
	"net/http"
)

// Security header constants
const (
	headerXContentTypeOptions     = "X-Content-Type-Options"
	headerXFrameOptions           = "X-Frame-Options"
	headerXXSSProtection          = "X-XSS-Protection"
	headerStrictTransportSecurity = "Strict-Transport-Security"
	headerContentSecurityPolicy   = "Content-Security-Policy"
	headerReferrerPolicy          = "Referrer-Policy"
)

// Security header values
const (
	valueNoSniff    = "nosniff"
	valueDeny       = "DENY"
	valueXSSBlock   = "1; mode=block"
	valueHSTS       = "max-age=31536000; includeSubDomains"
	valueCSPNone    = "default-src 'none'"
	valueNoReferrer = "no-referrer"
)

// securityHeadersMap defines all security headers to be set
var securityHeadersMap = map[string]string{
	headerXContentTypeOptions:     valueNoSniff,
	headerXFrameOptions:           valueDeny,
	headerXXSSProtection:          valueXSSBlock,
	headerStrictTransportSecurity: valueHSTS,
	headerContentSecurityPolicy:   valueCSPNone,
	headerReferrerPolicy:          valueNoReferrer,
}

// SecurityHeaders adds security headers to all HTTP responses.
//
// Business Requirements:
// - BR-GATEWAY-073: Add security headers to prevent common web vulnerabilities
// - BR-GATEWAY-074: Implement defense-in-depth security measures
//
// Security Headers Added:
// - X-Content-Type-Options: nosniff (prevents MIME type sniffing)
// - X-Frame-Options: DENY (prevents clickjacking)
// - X-XSS-Protection: 1; mode=block (enables XSS filter)
// - Strict-Transport-Security: max-age=31536000; includeSubDomains (enforces HTTPS)
// - Content-Security-Policy: default-src 'none' (restricts resource loading)
// - Referrer-Policy: no-referrer (prevents referrer leakage)
//
// These headers provide defense-in-depth protection against:
// - Clickjacking attacks
// - MIME type confusion attacks
// - Cross-site scripting (XSS)
// - Man-in-the-middle attacks
// - Information leakage
func SecurityHeaders() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Set all security headers from map
			for header, value := range securityHeadersMap {
				w.Header().Set(header, value)
			}

			// Continue to next handler
			next.ServeHTTP(w, r)
		})
	}
}
