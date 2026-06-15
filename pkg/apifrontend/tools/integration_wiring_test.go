package tools_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("Full Wiring Integration", func() {
	It("IT-AF-038-090: tool handler returns error when K8s API fails (typed client)", func() {
		fc := newTypedFakeClientWithError(fmt.Errorf("simulated API server error"))
		ctx := context.Background()

		_, err := tools.HandleListRemediations(ctx, fc, tools.ListRemediationsArgs{Namespace: "test"})
		Expect(err).To(HaveOccurred())
	})

	It("IT-AF-038-091: tool handler succeeds through typed client when K8s API healthy", func() {
		fc := newTypedFakeClient(newTypedRR("default", "rr-1", "Executing"))
		ctx := context.Background()

		result, err := tools.HandleListRemediations(ctx, fc, tools.ListRemediationsArgs{Namespace: "default"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(1))
		Expect(result.Remediations[0].Name).To(Equal("rr-1"))
	})

	It("IT-AF-1428-001: HandleListRemediations accepts crclient.Client (typed wiring)", func() {
		fc := newTypedFakeClient(newTypedRR("ns", "rr-typed-1", "Pending"))
		var c crclient.Client = fc
		result, err := tools.HandleListRemediations(context.Background(), c, tools.ListRemediationsArgs{Namespace: "ns"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(1))
	})

	It("IT-AF-1428-001b: HandleGetRemediation accepts crclient.Client (typed wiring)", func() {
		fc := newTypedFakeClient(newTypedRR("ns", "rr-typed-2", "Executing"))
		var c crclient.Client = fc
		result, err := tools.HandleGetRemediation(context.Background(), c, tools.GetRemediationArgs{Namespace: "ns", Name: "rr-typed-2"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Phase).To(Equal("Executing"))
	})

	It("IT-AF-1428-001c: HandleCheckExistingRR accepts crclient.Client (typed wiring)", func() {
		fc := newTypedFakeClient()
		var c crclient.Client = fc
		result, err := tools.HandleCheckExistingRR(context.Background(), c, "ns", tools.CheckExistingRRArgs{
			Namespace: "ns", Kind: "Deployment", Name: "web",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Exists).To(BeFalse())
	})

	It("IT-AF-1428-001d: error injection through interceptor propagates to handler", func() {
		fc := newTypedFakeClientWithInterceptor(interceptor.Funcs{
			List: func(ctx context.Context, client crclient.WithWatch, list crclient.ObjectList, opts ...crclient.ListOption) error {
				return fmt.Errorf("intercepted: API server error")
			},
		})
		_, err := tools.HandleListRemediations(context.Background(), fc, tools.ListRemediationsArgs{Namespace: "ns"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("API server error"))
	})
})
