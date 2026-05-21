package auth_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
)

var _ = Describe("DynamicClientFactory", func() {
	Describe("NewImpersonatingDynamicFactory", func() {
		var baseCfg *rest.Config

		BeforeEach(func() {
			baseCfg = &rest.Config{
				Host: "https://fake-api-server:6443",
			}
		})

		It("UT-AF-IMP-001: returns error when no identity in context", func() {
			factory := auth.NewImpersonatingDynamicFactory(baseCfg)
			_, err := factory(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("impersonation requires authenticated user identity"))
		})

		It("UT-AF-IMP-002: returns error when username is empty", func() {
			factory := auth.NewImpersonatingDynamicFactory(baseCfg)
			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "",
				Groups:   []string{"sre"},
			})
			_, err := factory(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("impersonation requires authenticated user identity"))
		})

		It("UT-AF-IMP-003: creates client successfully with valid identity", func() {
			factory := auth.NewImpersonatingDynamicFactory(baseCfg)
			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "alice",
				Groups:   []string{"sre", "ops"},
			})
			client, err := factory(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil())
		})

		It("UT-AF-IMP-004: applies client wrappers in order", func() {
			var wrapperCalled bool
			wrapper := auth.ClientWrapper(func(c dynamic.Interface) dynamic.Interface {
				wrapperCalled = true
				return c
			})

			factory := auth.NewImpersonatingDynamicFactory(baseCfg, wrapper)
			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "bob",
				Groups:   []string{"sre"},
			})
			client, err := factory(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil())
			Expect(wrapperCalled).To(BeTrue())
		})
	})

	Describe("NewOIDCDirectDynamicFactory", func() {
		var baseCfg *rest.Config

		BeforeEach(func() {
			baseCfg = &rest.Config{
				Host:            "https://fake-api-server:6443",
				BearerTokenFile: "/var/run/secrets/token",
			}
		})

		It("UT-AF-1226-001: returns error when no identity in context", func() {
			factory := auth.NewOIDCDirectDynamicFactory(baseCfg)
			_, err := factory(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("OIDC-direct requires authenticated user identity"))
		})

		It("UT-AF-1226-002: returns error when username is empty", func() {
			factory := auth.NewOIDCDirectDynamicFactory(baseCfg)
			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "",
				RawToken: "eyJhbGciOiJSUzI1NiJ9.valid",
			})
			_, err := factory(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("OIDC-direct requires authenticated user identity"))
		})

		It("UT-AF-1226-003: returns error when RawToken is empty", func() {
			factory := auth.NewOIDCDirectDynamicFactory(baseCfg)
			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "alice",
				RawToken: "",
			})
			_, err := factory(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("OIDC-direct requires a raw JWT"))
		})

		It("UT-AF-1226-004: returns error when token is expired", func() {
			factory := auth.NewOIDCDirectDynamicFactory(baseCfg)
			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username:  "alice",
				RawToken:  "eyJhbGciOiJSUzI1NiJ9.expired",
				ExpiresAt: time.Now().Add(-1 * time.Minute),
			})
			_, err := factory(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expired"))
		})

		It("UT-AF-1226-005: creates client successfully with valid identity and token", func() {
			factory := auth.NewOIDCDirectDynamicFactory(baseCfg)
			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username:  "alice",
				Groups:    []string{"sre"},
				RawToken:  "eyJhbGciOiJSUzI1NiJ9.valid-token",
				ExpiresAt: time.Now().Add(10 * time.Minute),
			})
			client, err := factory(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil())
		})

		It("UT-AF-1226-006: applies client wrappers in order", func() {
			var wrapperCalled bool
			wrapper := auth.ClientWrapper(func(c dynamic.Interface) dynamic.Interface {
				wrapperCalled = true
				return c
			})

			factory := auth.NewOIDCDirectDynamicFactory(baseCfg, wrapper)
			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username:  "bob",
				RawToken:  "eyJhbGciOiJSUzI1NiJ9.bobs-token",
				ExpiresAt: time.Now().Add(10 * time.Minute),
			})
			client, err := factory(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil())
			Expect(wrapperCalled).To(BeTrue())
		})

		It("UT-AF-1226-007: does not mutate the base rest.Config", func() {
			factory := auth.NewOIDCDirectDynamicFactory(baseCfg)
			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username:  "alice",
				RawToken:  "eyJhbGciOiJSUzI1NiJ9.alices-token",
				ExpiresAt: time.Now().Add(10 * time.Minute),
			})
			_, err := factory(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(baseCfg.BearerToken).To(BeEmpty(), "base config BearerToken must remain empty")
			Expect(baseCfg.BearerTokenFile).To(Equal("/var/run/secrets/token"), "base config BearerTokenFile must remain unchanged")
		})
	})

	Describe("StaticDynamicFactory", func() {
		It("UT-AF-IMP-005: returns error when client is nil", func() {
			factory := auth.StaticDynamicFactory(nil)
			_, err := factory(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("kubernetes cluster is not available"))
		})

		It("UT-AF-IMP-006: returns the static client when non-nil", func() {
			fakeClient := &fakeDynamicInterface{}
			factory := auth.StaticDynamicFactory(fakeClient)
			client, err := factory(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(client).To(Equal(fakeClient))
		})
	})
})

// fakeDynamicInterface is a minimal stub satisfying dynamic.Interface for tests.
type fakeDynamicInterface struct{ dynamic.Interface }
