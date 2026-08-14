package auth

import (
	"net"
	"net/http"
	"strings"
)

// trustedSourceResolver resolves a spoof-resistant source key for jti
// replay-cache source binding (#1999, BR-SECURITY-1505). It extracts the
// client IP from proxy headers (True-Client-IP, X-Real-IP,
// X-Forwarded-For) only when the immediate connection originates from a
// trusted proxy CIDR; otherwise it falls back to RemoteAddr.
//
// Deliberately isolated from httputil.ExtractClientIP (shared by rate
// limiting and audit logging, neither of which this fix has any reason to
// touch): unlike those callers, replay-cache source binding is a security
// decision (same-source vs. different-source jti reuse) where blindly
// trusting a spoofable header would silently defeat the whole control.
//
// Mirrors the already-proven pattern in
// pkg/gateway/middleware/trusted_realip.go (DD-AUTH-003, issue #673 L-1),
// duplicated in miniature rather than imported: pkg/apifrontend and
// pkg/gateway are intentionally independent services with no shared
// runtime dependency.
//
// Security properties (same as DD-AUTH-003):
//   - Fail-closed: no trusted CIDRs configured -> proxy headers are never
//     trusted -> the source key is always RemoteAddr's IP. Every request
//     then looks like the same source, so this fix detects no cross-source
//     replay, but it also never produces a false positive on a legitimate
//     client sitting behind an untrusted/unconfigured proxy.
//   - CIDRs are parsed once at construction time, not per request.
//   - Malformed CIDRs are silently skipped.
//   - Header priority: True-Client-IP > X-Real-IP > X-Forwarded-For
//     (first hop), matching trusted_realip.go.
type trustedSourceResolver struct {
	networks []*net.IPNet
}

// newTrustedSourceResolver parses trustedCIDRs once. A nil or empty slice
// yields a resolver that always falls back to RemoteAddr (fail-closed).
func newTrustedSourceResolver(trustedCIDRs []string) *trustedSourceResolver {
	var networks []*net.IPNet
	for _, cidr := range trustedCIDRs {
		if _, network, err := net.ParseCIDR(cidr); err == nil {
			networks = append(networks, network)
		}
	}
	return &trustedSourceResolver{networks: networks}
}

// sourceKey returns the replay-cache source-binding key for r.
func (res *trustedSourceResolver) sourceKey(r *http.Request) string {
	remoteIP := parseRemoteAddrIP(r.RemoteAddr)
	if len(res.networks) > 0 && remoteIP != nil && res.isTrustedPeer(remoteIP) {
		if fwd := forwardedClientIP(r); fwd != "" {
			return fwd
		}
	}
	if remoteIP != nil {
		return remoteIP.String()
	}
	return r.RemoteAddr
}

func (res *trustedSourceResolver) isTrustedPeer(ip net.IP) bool {
	for _, network := range res.networks {
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
	sourceHdrTrueClientIP  = http.CanonicalHeaderKey("True-Client-IP")
	sourceHdrXRealIP       = http.CanonicalHeaderKey("X-Real-IP")
	sourceHdrXForwardedFor = http.CanonicalHeaderKey("X-Forwarded-For")
)

// forwardedClientIP reads proxy headers in priority order and returns the
// first valid IP found, or "" if none are present or valid.
func forwardedClientIP(r *http.Request) string {
	var ip string
	switch {
	case r.Header.Get(sourceHdrTrueClientIP) != "":
		ip = r.Header.Get(sourceHdrTrueClientIP)
	case r.Header.Get(sourceHdrXRealIP) != "":
		ip = r.Header.Get(sourceHdrXRealIP)
	case r.Header.Get(sourceHdrXForwardedFor) != "":
		ip, _, _ = strings.Cut(r.Header.Get(sourceHdrXForwardedFor), ",")
		ip = strings.TrimSpace(ip)
	}
	if ip == "" || net.ParseIP(ip) == nil {
		return ""
	}
	return ip
}
