package auth

import (
	"net/http"
	"testing"
)

// #1999 (BR-SECURITY-1505): trustedSourceResolver must only trust forwarded
// client-IP headers when the immediate peer is a configured trusted proxy —
// otherwise an attacker replaying a stolen token could spoof the header to
// impersonate the original caller's source key and defeat replay detection
// entirely.

// untrustedPeerAddr is a direct-connecting peer never in the trusted-CIDR
// allowlists used below, shared across cases that assert its RemoteAddr IP
// is used as-is regardless of any forwarded header.
const untrustedPeerAddr = "203.0.113.7"

func newSourceRequest(remoteAddr string, headers map[string]string) *http.Request {
	req := &http.Request{RemoteAddr: remoteAddr, Header: http.Header{}}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req
}

func TestTrustedSourceResolver_NoCIDRsConfigured_AlwaysUsesRemoteAddr(t *testing.T) {
	// UT-AF-1999-001: fail-closed default — headers ignored entirely.
	res := newTrustedSourceResolver(nil)
	req := newSourceRequest(untrustedPeerAddr+":54321", map[string]string{
		"X-Forwarded-For": "10.0.0.99",
	})
	if got := res.sourceKey(req); got != untrustedPeerAddr {
		t.Errorf("expected RemoteAddr IP with no trusted CIDRs, got %q", got)
	}
}

func TestTrustedSourceResolver_UntrustedPeer_SpoofedHeaderIgnored(t *testing.T) {
	// UT-AF-1999-002: CIDRs configured, but this peer isn't in them — a
	// spoofed X-Forwarded-For from an untrusted direct caller must not be
	// trusted (this is the exact XFF-spoofing attack this fix must resist).
	res := newTrustedSourceResolver([]string{"10.0.0.0/8"})
	req := newSourceRequest(untrustedPeerAddr+":54321", map[string]string{
		"X-Forwarded-For": "192.168.1.1", // spoofed victim IP
	})
	if got := res.sourceKey(req); got != untrustedPeerAddr {
		t.Errorf("expected untrusted peer's RemoteAddr, got %q (header should be ignored)", got)
	}
}

func TestTrustedSourceResolver_TrustedPeer_UsesForwardedHeader(t *testing.T) {
	// UT-AF-1999-003: peer is a trusted proxy — the forwarded header is
	// authoritative for the real client IP.
	res := newTrustedSourceResolver([]string{"10.0.0.0/8"})
	req := newSourceRequest("10.0.0.5:443", map[string]string{
		"X-Forwarded-For": "198.51.100.23, 10.0.0.5",
	})
	if got := res.sourceKey(req); got != "198.51.100.23" {
		t.Errorf("expected forwarded client IP from trusted peer, got %q", got)
	}
}

func TestTrustedSourceResolver_HeaderPriority_TrueClientIPWins(t *testing.T) {
	// UT-AF-1999-004: header priority matches trusted_realip.go:
	// True-Client-IP > X-Real-IP > X-Forwarded-For.
	res := newTrustedSourceResolver([]string{"10.0.0.0/8"})
	req := newSourceRequest("10.0.0.5:443", map[string]string{
		"True-Client-IP":  "198.51.100.1",
		"X-Real-IP":       "198.51.100.2",
		"X-Forwarded-For": "198.51.100.3",
	})
	if got := res.sourceKey(req); got != "198.51.100.1" {
		t.Errorf("expected True-Client-IP to win, got %q", got)
	}
}

func TestTrustedSourceResolver_TrustedPeerNoHeader_FallsBackToRemoteAddr(t *testing.T) {
	// UT-AF-1999-005: trusted peer, but no forwarded header present at all.
	res := newTrustedSourceResolver([]string{"10.0.0.0/8"})
	req := newSourceRequest("10.0.0.5:443", nil)
	if got := res.sourceKey(req); got != "10.0.0.5" {
		t.Errorf("expected RemoteAddr fallback, got %q", got)
	}
}

func TestTrustedSourceResolver_MalformedCIDR_Skipped(t *testing.T) {
	// UT-AF-1999-006: a malformed CIDR entry must not crash construction and
	// must not be treated as trusting everything.
	res := newTrustedSourceResolver([]string{"not-a-cidr", "10.0.0.0/8"})
	req := newSourceRequest(untrustedPeerAddr+":1", map[string]string{"X-Forwarded-For": "1.2.3.4"})
	if got := res.sourceKey(req); got != untrustedPeerAddr {
		t.Errorf("expected untrusted peer fallback despite malformed CIDR entry, got %q", got)
	}
}
