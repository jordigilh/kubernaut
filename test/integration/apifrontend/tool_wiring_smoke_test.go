package apifrontend_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("Integration: tool wiring smoke", func() {
	It("list_remediations round-trip with fake client", func() {
		s := runtime.NewScheme()
		rrGVR := schema.GroupVersionResource{Group: "kubernaut.ai", Version: "v1alpha1", Resource: "remediationrequests"}
		dynClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(s,
			map[schema.GroupVersionResource]string{rrGVR: "RemediationRequestList"})

		result, err := tools.HandleListRemediations(context.Background(), dynClient, tools.ListRemediationsArgs{
			Namespace: "default",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(0))
	})
})
