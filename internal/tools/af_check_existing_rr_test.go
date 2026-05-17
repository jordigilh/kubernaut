package tools_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/jordigilh/kubernaut-apifrontend/internal/tools"
)

func testFingerprint(ns, kind, name string) string {
	h := sha256.Sum256([]byte(ns + "/" + kind + "/" + name))
	return fmt.Sprintf("%x", h)
}

func newUnstructuredRR(ns, name, phase, targetKind, targetName string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kubernaut.ai/v1alpha1",
			"kind":       "RemediationRequest",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
			},
			"spec": map[string]interface{}{
				"signalFingerprint": testFingerprint(ns, targetKind, targetName),
				"targetResource": map[string]interface{}{
					"kind": targetKind,
					"name": targetName,
				},
			},
			"status": map[string]interface{}{
				"overallPhase": phase,
			},
		},
	}
}

var _ = Describe("af_check_existing_rr", func() {
	rrGVR := schema.GroupVersionResource{Group: "kubernaut.ai", Version: "v1alpha1", Resource: "remediationrequests"}

	It("UT-AF-052-040: finds active RR for matching fingerprint", func() {
		rr := newUnstructuredRR("prod", "rr-deploy-web-1", "Executing", "Deployment", "web")
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{rrGVR: "RemediationRequestList"},
			rr,
		)

		result, err := tools.HandleCheckExistingRR(context.Background(), client, tools.CheckExistingRRArgs{
			Namespace: "prod", Kind: "Deployment", Name: "web",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Exists).To(BeTrue())
		Expect(result.RRID).To(Equal("prod/rr-deploy-web-1"))
		Expect(result.Phase).To(Equal("Executing"))
	})

	It("UT-AF-052-041: terminal RR not reported as existing", func() {
		rr := newUnstructuredRR("prod", "rr-deploy-web-1", "Completed", "Deployment", "web")
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{rrGVR: "RemediationRequestList"},
			rr,
		)

		result, err := tools.HandleCheckExistingRR(context.Background(), client, tools.CheckExistingRRArgs{
			Namespace: "prod", Kind: "Deployment", Name: "web",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Exists).To(BeFalse())
	})

	It("UT-AF-052-042: no RRs at all returns exists=false", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{rrGVR: "RemediationRequestList"})

		result, err := tools.HandleCheckExistingRR(context.Background(), client, tools.CheckExistingRRArgs{
			Namespace: "prod", Kind: "Deployment", Name: "web",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Exists).To(BeFalse())
	})

	It("UT-AF-052-043: empty namespace rejected", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{rrGVR: "RemediationRequestList"})

		_, err := tools.HandleCheckExistingRR(context.Background(), client, tools.CheckExistingRRArgs{
			Namespace: "", Kind: "Deployment", Name: "web",
		})
		Expect(err).To(MatchError(ContainSubstring("invalid input")))
	})

	It("UT-AF-052-044: empty kind rejected", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{rrGVR: "RemediationRequestList"})

		_, err := tools.HandleCheckExistingRR(context.Background(), client, tools.CheckExistingRRArgs{
			Namespace: "prod", Kind: "", Name: "web",
		})
		Expect(err).To(MatchError(ContainSubstring("invalid input")))
	})

	It("UT-AF-052-045: empty name rejected", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{rrGVR: "RemediationRequestList"})

		_, err := tools.HandleCheckExistingRR(context.Background(), client, tools.CheckExistingRRArgs{
			Namespace: "prod", Kind: "Deployment", Name: "",
		})
		Expect(err).To(MatchError(ContainSubstring("invalid input")))
	})

	It("UT-AF-052-046: nil client returns ErrK8sUnavailable", func() {
		_, err := tools.HandleCheckExistingRR(context.Background(), nil, tools.CheckExistingRRArgs{
			Namespace: "prod", Kind: "Deployment", Name: "web",
		})
		Expect(err).To(MatchError(tools.ErrK8sUnavailable))
	})

	It("UT-AF-052-047: concurrent calls safe", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{rrGVR: "RemediationRequestList"})

		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := tools.HandleCheckExistingRR(context.Background(), client, tools.CheckExistingRRArgs{
					Namespace: "prod", Kind: "Deployment", Name: "web",
				})
				Expect(err).NotTo(HaveOccurred())
			}()
		}
		wg.Wait()
	})

	It("UT-AF-052-048: mismatched fingerprint not reported as existing", func() {
		rr := newUnstructuredRR("prod", "rr-deploy-web-1", "Executing", "Deployment", "web")
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{rrGVR: "RemediationRequestList"},
			rr,
		)

		result, err := tools.HandleCheckExistingRR(context.Background(), client, tools.CheckExistingRRArgs{
			Namespace: "prod", Kind: "Deployment", Name: "other-target",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Exists).To(BeFalse())
	})
})
