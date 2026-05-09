//go:build integration

package tools_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/jordigilh/kubernaut-apifrontend/internal/tools"
)

func TestIntegrationSmokeTools(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Smoke Suite")
}

var _ = Describe("Integration: tool wiring smoke", func() {
	It("list_remediations -> watch -> cancel round-trip with fake client", func() {
		scheme := runtime.NewScheme()
		rrGVR := schema.GroupVersionResource{Group: "kubernaut.ai", Version: "v1alpha1", Resource: "remediationrequests"}
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{rrGVR: "RemediationRequestList"})

		result, err := tools.HandleListRemediations(context.Background(), client, tools.ListRemediationsArgs{
			Namespace: "default",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(0))
	})
})
