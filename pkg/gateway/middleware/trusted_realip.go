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
	"net"
	"net/http"
	"strings"
)

// TrustedRealIP returns middleware that extracts the real client IP from proxy
// headers (True-Client-IP, X-Real-IP, X-Forwarded-For) only when the immediate
// connection originates from a trusted proxy CIDR.
//
// Issue #673 L-1: Replaces Chi's chimiddleware.RealIP which unconditionally
// trusts proxy headers from any source.
// DD-AUTH-003: Design pattern for trusted proxy validation.
//
// Security properties:
//   - Fail-closed: if trustedCIDRs is empty or nil, proxy headers are never trusted.
//   - CIDRs are parsed once at construction time (not per-request).
//   - Malformed CIDRs are silently skipped (logged at startup by caller).
//   - Header priority: True-Client-IP > X-Real-IP > X-Forwarded-For (matches Chi).
//   - Invalid IPs in headers are rejected (RemoteAddr unchanged).
func TrustedRealIP(trustedCIDRs []string) func(http.Handler) http.Handler {
	networks := parseCIDRs(trustedCIDRs)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(networks) > 0 && isTrustedSource(r.RemoteAddr, networks) {
				if rip := extractRealIP(r); rip != "" {
					r.RemoteAddr = rip
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// parseCIDRs converts CIDR strings to net.IPNet at construction time.
// Malformed entries are silently skipped.
func parseCIDRs(cidrs []string) []*net.IPNet {
	var networks []*net.IPNet
	for _, cidr := range cidrs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		networks = append(networks, network)
	}
	return networks
}

// isTrustedSource checks whether remoteAddr falls within any trusted network.
func isTrustedSource(remoteAddr string, networks []*net.IPNet) bool {
	ip := parseRemoteAddrIP(remoteAddr)
	if ip == nil {
		return false
	}
	for _, network := range networks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// parseRemoteAddrIP extracts the IP from a "host:port" or plain IP string.
func parseRemoteAddrIP(remoteAddr string) net.IP {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return net.ParseIP(remoteAddr)
	}
	return net.ParseIP(host)
}

var (
	headerTrueClientIP = http.CanonicalHeaderKey("True-Client-IP")
	headerXRealIP      = http.CanonicalHeaderKey("X-Real-IP")
	headerXForwardedFor = http.CanonicalHeaderKey("X-Forwarded-For")
)

// extractRealIP reads proxy headers in priority order and returns the first
// valid IP found. Mirrors Chi's RealIP header priority.
func extractRealIP(r *http.Request) string {
	var ip string

	if tcip := r.Header.Get(headerTrueClientIP); tcip != "" {
		ip = tcip
	} else if xrip := r.Header.Get(headerXRealIP); xrip != "" {
		ip = xrip
	} else if xff := r.Header.Get(headerXForwardedFor); xff != "" {
		ip, _, _ = strings.Cut(xff, ",")
		ip = strings.TrimSpace(ip)
	}

	if ip == "" || net.ParseIP(ip) == nil {
		return ""
	}
	return ip
}
