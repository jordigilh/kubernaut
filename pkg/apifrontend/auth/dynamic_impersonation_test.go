package auth_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/dynamic"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
)

var _ = Describe("DynamicClientFactory", func() {
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
