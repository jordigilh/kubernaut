package httputil_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/httputil"
)

var _ = Describe("ExtractClientIP", func() {
	newReq := func(remoteAddr string, xff string) *http.Request {
		r, _ := http.NewRequest("GET", "/", http.NoBody)
		r.RemoteAddr = remoteAddr
		if xff != "" {
			r.Header.Set("X-Forwarded-For", xff)
		}
		return r
	}

	It("UT-AF-CIP-001: returns first XFF hop when present", func() {
		ip := httputil.ExtractClientIP(newReq("127.0.0.1:9999", "10.0.0.1, 10.0.0.2, 10.0.0.3"))
		Expect(ip).To(Equal("10.0.0.1"))
	})

	It("UT-AF-CIP-002: returns single XFF entry without port", func() {
		ip := httputil.ExtractClientIP(newReq("127.0.0.1:9999", "192.168.1.1"))
		Expect(ip).To(Equal("192.168.1.1"))
	})

	It("UT-AF-CIP-003: strips port from XFF if present", func() {
		ip := httputil.ExtractClientIP(newReq("127.0.0.1:9999", "10.0.0.1:8080, 10.0.0.2"))
		Expect(ip).To(Equal("10.0.0.1"))
	})

	It("UT-AF-CIP-004: falls back to RemoteAddr when no XFF", func() {
		ip := httputil.ExtractClientIP(newReq("172.16.0.5:12345", ""))
		Expect(ip).To(Equal("172.16.0.5"))
	})

	It("UT-AF-CIP-005: handles RemoteAddr without port", func() {
		ip := httputil.ExtractClientIP(newReq("172.16.0.5", ""))
		Expect(ip).To(Equal("172.16.0.5"))
	})

	It("UT-AF-CIP-006: handles IPv6 RemoteAddr with brackets and port", func() {
		ip := httputil.ExtractClientIP(newReq("[::1]:8080", ""))
		Expect(ip).To(Equal("::1"))
	})

	It("UT-AF-CIP-007: handles IPv6 in XFF", func() {
		ip := httputil.ExtractClientIP(newReq("127.0.0.1:80", "2001:db8::1, 10.0.0.2"))
		Expect(ip).To(Equal("2001:db8::1"))
	})

	It("UT-AF-CIP-008: trims whitespace in XFF entries", func() {
		ip := httputil.ExtractClientIP(newReq("127.0.0.1:80", "  10.0.0.1 , 10.0.0.2"))
		Expect(ip).To(Equal("10.0.0.1"))
	})
})
