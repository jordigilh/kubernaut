package apifrontend_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("Integration: tool wiring smoke", func() {
	It("list_remediations round-trip with fake client", func() {
		s := runtime.NewScheme()
		_ = remediationv1.AddToScheme(s)
		typedClient := fake.NewClientBuilder().WithScheme(s).Build()

		result, err := tools.HandleListRemediations(context.Background(), typedClient, tools.ListRemediationsArgs{
			Namespace: defaultFixture,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(0))
	})
})
