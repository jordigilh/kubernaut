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

// ExtractClientIP extracts the source IP address from an HTTP request.
//
// This function is essential for per-source rate limiting (BR-GATEWAY-004).
// It checks multiple sources in order of priority:
// 1. X-Forwarded-For header (production deployment behind Ingress/Load Balancer)
// 2. X-Real-IP header (alternative proxy header, used by NGINX)
// 3. RemoteAddr field (direct Pod-to-Pod communication, no proxy)
//
// # Deployment Scenarios
//
// Scenario 1: Production with Ingress (90% of deployments)
//   - Flow: Client → Ingress → Gateway Pod
//   - Ingress adds X-Forwarded-For: "client-ip, proxy-ip"
//   - ExtractClientIP returns: "client-ip" (first IP in list)
//
// Scenario 2: Direct ClusterIP Service (10% of deployments)
//   - Flow: AlertManager Pod → Gateway Pod (direct)
//   - No proxy, no X-Forwarded-For
//   - ExtractClientIP returns: IP from RemoteAddr (e.g., "10.244.0.5")
//
// # X-Forwarded-For Format
//
// Standard format: "client-ip, proxy1-ip, proxy2-ip"
// - First IP: Original client IP (what we want for rate limiting)
// - Subsequent IPs: Proxy chain (ignored for rate limiting)
//
// # Security Considerations
//
// X-Forwarded-For can be spoofed by malicious clients.
// In production, configure your Ingress/Load Balancer to:
// - Strip X-Forwarded-For headers from untrusted sources
// - Add trusted X-Forwarded-For header with real client IP
//
// Example (NGINX Ingress):
//
//	proxy_set_header X-Forwarded-For $remote_addr;
//
// # IPv6 Support
//
// Supports both IPv4 and IPv6 addresses:
// - IPv4: "203.0.113.45"
// - IPv6: "2001:db8::1"
// - IPv6 RemoteAddr: "[2001:db8::5]:45678" → "[2001:db8::5]"
//
// # Examples
//
//	// Scenario 1: Production with Ingress
//	req.Header.Set("X-Forwarded-For", "203.0.113.45, 198.51.100.10")
//	ip := ExtractClientIP(req) // Returns: "203.0.113.45"
//
//	// Scenario 2: Direct Pod-to-Pod
//	req.RemoteAddr = "10.244.0.5:45678"
//	ip := ExtractClientIP(req) // Returns: "10.244.0.5"
//
//	// Scenario 3: IPv6 localhost (integration tests)
//	req.RemoteAddr = "[::1]:54321"
//	ip := ExtractClientIP(req) // Returns: "[::1]"
func ExtractClientIP(r *http.Request) string {
	// Priority 1: X-Forwarded-For header
	// Format: "client-ip, proxy1-ip, proxy2-ip"
	// Extract first IP (client IP) from comma-separated list
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Find first comma (separates client IP from proxy IPs)
		for idx := 0; idx < len(xff); idx++ {
			if xff[idx] == ',' {
				return xff[:idx]
			}
		}
		// No comma found: single IP in header
		return xff
	}

	// Priority 2: X-Real-IP header
	// Alternative header used by some proxies (e.g., NGINX)
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Priority 3: RemoteAddr fallback
	// Used for direct Pod-to-Pod communication (no Ingress)
	// Format: "ip:port" or "[ipv6]:port"
	remoteAddr := r.RemoteAddr

	// Strip port from RemoteAddr
	// Works for both IPv4 ("10.0.0.1:45678") and IPv6 ("[2001:db8::1]:45678")
	// Algorithm: Find last colon, strip everything after it
	for i := len(remoteAddr) - 1; i >= 0; i-- {
		if remoteAddr[i] == ':' {
			return remoteAddr[:i]
		}
	}

	// No colon found: return as-is (edge case, shouldn't happen in HTTP)
	return remoteAddr
}
