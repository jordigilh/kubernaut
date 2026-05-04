package security_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/security"
)

func TestSecuritySuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Security Suite")
}

var _ = Describe("Sanitize", func() {
	Describe("ValidateHeaderValue", func() {
		It("UT-AF-SEC-001: accepts normal header values", func() {
			Expect(security.ValidateHeaderValue("Bearer eyJhbGciOiJSUzI1NiJ9")).To(Succeed())
		})

		It("UT-AF-SEC-002: rejects null bytes", func() {
			Expect(security.ValidateHeaderValue("Bearer \x00token")).To(MatchError(security.ErrControlChars))
		})

		It("UT-AF-SEC-003: allows horizontal tabs", func() {
			Expect(security.ValidateHeaderValue("Bearer\ttoken")).To(Succeed())
		})

		It("UT-AF-SEC-004: rejects CRLF injection", func() {
			Expect(security.ValidateHeaderValue("Bearer token\r\nX-Injected: yes")).To(MatchError(security.ErrControlChars))
		})
	})

	Describe("SanitizeClaimValue", func() {
		It("UT-AF-SEC-005: passes through normal strings unchanged", func() {
			Expect(security.SanitizeClaimValue("alice")).To(Equal("alice"))
		})

		It("UT-AF-SEC-006: strips CRLF from claim values", func() {
			Expect(security.SanitizeClaimValue("alice\r\nX-Injected: yes")).To(Equal("aliceX-Injected: yes"))
		})

		It("UT-AF-SEC-007: strips null bytes from claim values", func() {
			Expect(security.SanitizeClaimValue("alice\x00admin")).To(Equal("aliceadmin"))
		})

		It("UT-AF-SEC-008: preserves tabs in claim values", func() {
			Expect(security.SanitizeClaimValue("alice\tbob")).To(Equal("alice\tbob"))
		})
	})
})
