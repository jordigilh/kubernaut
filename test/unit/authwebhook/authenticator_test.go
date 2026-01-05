package authwebhook_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	admissionv1 "k8s.io/api/admission/v1"
	authv1 "k8s.io/api/authentication/v1"
)

var _ = Describe("Authenticator", func() {
	var (
		authenticator *authwebhook.Authenticator
		ctx           context.Context
	)

	BeforeEach(func() {
		authenticator = authwebhook.NewAuthenticator()
		ctx = context.Background()
	})

	Describe("ExtractUser", func() {
		Context("when admission request has valid user info", func() {
			It("should extract username and UID", func() {
				req := &admissionv1.AdmissionRequest{
					UserInfo: authv1.UserInfo{
						Username: "admin@example.com",
						UID:      "abc-123-def",
					},
				}

				authCtx, err := authenticator.ExtractUser(ctx, req)
				Expect(err).ToNot(HaveOccurred())
				Expect(authCtx.Username).To(Equal("admin@example.com"))
				Expect(authCtx.UID).To(Equal("abc-123-def"))
			})

			It("should format authentication string correctly", func() {
				req := &admissionv1.AdmissionRequest{
					UserInfo: authv1.UserInfo{
						Username: "admin@example.com",
						UID:      "abc-123-def",
					},
				}

				authCtx, err := authenticator.ExtractUser(ctx, req)
				Expect(err).ToNot(HaveOccurred())
				Expect(authCtx.String()).To(Equal("admin@example.com (UID: abc-123-def)"))
			})
		})

		Context("when username is missing", func() {
			It("should return error", func() {
				req := &admissionv1.AdmissionRequest{
					UserInfo: authv1.UserInfo{
						UID: "abc-123",
					},
				}

				_, err := authenticator.ExtractUser(ctx, req)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("username is required"))
			})
		})

		Context("when UID is missing", func() {
			It("should return error", func() {
				req := &admissionv1.AdmissionRequest{
					UserInfo: authv1.UserInfo{
						Username: "admin@example.com",
					},
				}

				_, err := authenticator.ExtractUser(ctx, req)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("UID is required"))
			})
		})

		Context("when both username and UID are missing", func() {
			It("should return error", func() {
				req := &admissionv1.AdmissionRequest{
					UserInfo: authv1.UserInfo{},
				}

				_, err := authenticator.ExtractUser(ctx, req)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when admission request is nil", func() {
			It("should return error", func() {
				_, err := authenticator.ExtractUser(ctx, nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("admission request cannot be nil"))
			})
		})
	})
})

