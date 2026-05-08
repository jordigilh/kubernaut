package auth_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
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
