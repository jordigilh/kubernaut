package tools_test

import (
	"context"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/jordigilh/kubernaut-apifrontend/internal/tools"
)

var _ = Describe("af_create_rr", func() {
	rrGVR := schema.GroupVersionResource{Group: "kubernaut.ai", Version: "v1alpha1", Resource: "remediationrequests"}

	It("UT-AF-052-050: creates RR when none exists", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{rrGVR: "RemediationRequestList"})

		result, err := tools.HandleCreateRR(context.Background(), client, tools.CreateRRArgs{
			Namespace:   "prod",
			Kind:        "Deployment",
			Name:        "web",
			Severity:    "high",
			Description: "Pod CrashLoopBackOff detected",
		}, "sre-user")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RRID).NotTo(BeEmpty())
		Expect(result.AlreadyExists).To(BeFalse())
		Expect(result.Message).To(ContainSubstring("created"))
	})

	It("UT-AF-052-051: returns existing RR when non-terminal match found", func() {
		rr := newUnstructuredRR("prod", "rr-deploy-web-existing", "Executing", "Deployment", "web")
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{rrGVR: "RemediationRequestList"},
			rr,
		)

		result, err := tools.HandleCreateRR(context.Background(), client, tools.CreateRRArgs{
			Namespace:   "prod",
			Kind:        "Deployment",
			Name:        "web",
			Description: "duplicate",
		}, "sre-user")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.AlreadyExists).To(BeTrue())
		Expect(result.RRID).To(Equal("prod/rr-deploy-web-existing"))
	})

	It("UT-AF-052-052: empty namespace rejected", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{rrGVR: "RemediationRequestList"})

		_, err := tools.HandleCreateRR(context.Background(), client, tools.CreateRRArgs{
			Namespace: "", Kind: "Deployment", Name: "web", Description: "x",
		}, "user")
		Expect(err).To(MatchError(ContainSubstring("invalid input")))
	})

	It("UT-AF-052-053: empty kind rejected", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{rrGVR: "RemediationRequestList"})

		_, err := tools.HandleCreateRR(context.Background(), client, tools.CreateRRArgs{
			Namespace: "prod", Kind: "", Name: "web", Description: "x",
		}, "user")
		Expect(err).To(MatchError(ContainSubstring("invalid input")))
	})

	It("UT-AF-052-054: empty name rejected", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{rrGVR: "RemediationRequestList"})

		_, err := tools.HandleCreateRR(context.Background(), client, tools.CreateRRArgs{
			Namespace: "prod", Kind: "Deployment", Name: "", Description: "x",
		}, "user")
		Expect(err).To(MatchError(ContainSubstring("invalid input")))
	})

	It("UT-AF-052-055: nil client returns ErrK8sUnavailable", func() {
		_, err := tools.HandleCreateRR(context.Background(), nil, tools.CreateRRArgs{
			Namespace: "prod", Kind: "Deployment", Name: "web", Description: "x",
		}, "user")
		Expect(err).To(MatchError(tools.ErrK8sUnavailable))
	})

	It("UT-AF-052-056: long description is truncated not rejected", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{rrGVR: "RemediationRequestList"})

		longDesc := make([]byte, 4096)
		for i := range longDesc {
			longDesc[i] = 'a'
		}

		result, err := tools.HandleCreateRR(context.Background(), client, tools.CreateRRArgs{
			Namespace:   "prod",
			Kind:        "Deployment",
			Name:        "web",
			Description: string(longDesc),
		}, "user")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RRID).NotTo(BeEmpty())
	})

	It("UT-AF-052-057: concurrent calls with same fingerprint are deduplicated", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{rrGVR: "RemediationRequestList"})

		var wg sync.WaitGroup
		results := make([]tools.CreateRRResult, 5)
		errs := make([]error, 5)

		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				results[idx], errs[idx] = tools.HandleCreateRR(context.Background(), client, tools.CreateRRArgs{
					Namespace:   "prod",
					Kind:        "Deployment",
					Name:        "dedup-target",
					Description: "concurrent test",
				}, "user")
			}(i)
		}
		wg.Wait()

		for _, err := range errs {
			Expect(err).NotTo(HaveOccurred())
		}

		firstRRID := results[0].RRID
		for _, r := range results[1:] {
			Expect(r.RRID).To(Equal(firstRRID))
		}
	})

	It("UT-AF-052-058: invalid namespace (path traversal) rejected", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{rrGVR: "RemediationRequestList"})

		_, err := tools.HandleCreateRR(context.Background(), client, tools.CreateRRArgs{
			Namespace: "../../etc", Kind: "Deployment", Name: "web", Description: "x",
		}, "user")
		Expect(err).To(MatchError(ContainSubstring("invalid input")))
	})
})
