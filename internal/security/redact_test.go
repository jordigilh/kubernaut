package security_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/security"
)

var _ = Describe("RedactMap", func() {
	It("returns nil for nil input", func() {
		Expect(security.RedactMap(nil)).To(BeNil())
	})

	It("returns empty map for empty input", func() {
		result := security.RedactMap(map[string]string{})
		Expect(result).To(HaveLen(0))
	})

	It("redacts known sensitive keys (case-insensitive)", func() {
		input := map[string]string{
			"password":      "secret123",
			"API_KEY":       "key-abc",
			"Authorization": "Bearer tok",
			"safe_field":    "visible",
		}
		result := security.RedactMap(input)
		Expect(result["password"]).To(Equal("[REDACTED]"))
		Expect(result["API_KEY"]).To(Equal("[REDACTED]"))
		Expect(result["Authorization"]).To(Equal("[REDACTED]"))
		Expect(result["safe_field"]).To(Equal("visible"))
	})

	It("does not modify the original map", func() {
		input := map[string]string{"token": "original"}
		_ = security.RedactMap(input)
		Expect(input["token"]).To(Equal("original"))
	})

	It("handles keys containing sensitive substrings", func() {
		input := map[string]string{
			"x-api-token":         "val1",
			"db_password_hash":    "val2",
			"user_credential_ref": "val3",
		}
		result := security.RedactMap(input)
		Expect(result["x-api-token"]).To(Equal("[REDACTED]"))
		Expect(result["db_password_hash"]).To(Equal("[REDACTED]"))
		Expect(result["user_credential_ref"]).To(Equal("[REDACTED]"))
	})

	It("redacts JWT tokens in values regardless of key name", func() {
		jwt := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
		input := map[string]string{"trace_header": jwt}
		result := security.RedactMap(input)
		Expect(result["trace_header"]).To(ContainSubstring("[JWT_REDACTED]"))
		Expect(result["trace_header"]).NotTo(ContainSubstring("eyJ"))
	})

	It("redacts bearer tokens in values", func() {
		input := map[string]string{"debug_info": "got bearer aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789"}
		result := security.RedactMap(input)
		Expect(result["debug_info"]).To(ContainSubstring("[BEARER_REDACTED]"))
	})

	It("redacts long base64-only strings that look like secrets", func() {
		b64Secret := "YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXoxMjM0NTY3ODk="
		input := map[string]string{"payload": b64Secret}
		result := security.RedactMap(input)
		Expect(result["payload"]).To(Equal("[REDACTED]"))
	})

	It("preserves normal values that happen to contain some base64 chars", func() {
		input := map[string]string{"message": "the quick brown fox jumps over the lazy dog and it was great"}
		result := security.RedactMap(input)
		Expect(result["message"]).To(Equal("the quick brown fox jumps over the lazy dog and it was great"))
	})
})

var _ = Describe("RedactError", func() {
	It("returns empty string for nil error", func() {
		Expect(security.RedactError(nil)).To(BeEmpty())
	})

	It("strips URLs from error messages", func() {
		err := errors.New("connection to https://internal.svc:8443/api/v1/secrets failed")
		result := security.RedactError(err)
		Expect(result).NotTo(ContainSubstring("https://"))
		Expect(result).To(ContainSubstring("[URL_REDACTED]"))
	})

	It("strips file paths from error messages", func() {
		err := errors.New("open /etc/kubernetes/pki/ca.crt: permission denied")
		result := security.RedactError(err)
		Expect(result).NotTo(ContainSubstring("/etc/kubernetes/pki/ca.crt"))
		Expect(result).To(ContainSubstring("[PATH_REDACTED]"))
	})

	It("strips stack traces", func() {
		err := errors.New("panic: nil pointer\ngoroutine 1 [running]:\nmain.foo()\n\t/app/main.go:42")
		result := security.RedactError(err)
		Expect(result).NotTo(ContainSubstring("goroutine"))
		Expect(result).NotTo(ContainSubstring("main.go"))
	})

	It("truncates long messages", func() {
		longMsg := ""
		for i := 0; i < 300; i++ {
			longMsg += "x"
		}
		err := errors.New(longMsg)
		result := security.RedactError(err)
		Expect(len(result)).To(BeNumerically("<=", 260))
		Expect(result).To(HaveSuffix("..."))
	})
})
