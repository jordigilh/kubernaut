package tools_test

import (
	"context"
	"fmt"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("kubectl_list", func() {
	It("UT-AF-1230-010: returns multiple resources of the same kind", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, kubectlGVRs,
			newUnstructuredService("prod", "web-svc", "10.0.0.1"),
			newUnstructuredService("prod", "api-svc", "10.0.0.2"),
		)

		result, err := tools.HandleKubectlList(context.Background(), client, nil, tools.KubectlListArgs{
			Kind:      "Service",
			Namespace: "prod",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(2))
		Expect(result.Items[0]["metadata"]).NotTo(BeNil())
	})

	It("UT-AF-1230-011: returns empty list for namespace with no matching resources", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, kubectlGVRs)

		result, err := tools.HandleKubectlList(context.Background(), client, nil, tools.KubectlListArgs{
			Kind:      "Service",
			Namespace: "empty-ns",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(0))
		Expect(result.Items).To(BeEmpty())
	})

	It("UT-AF-1230-012: label selector filters results", func() {
		svcWithLabel := newUnstructuredService("prod", "web-svc", "10.0.0.1")
		svcWithLabel.SetLabels(map[string]string{"app": "web"})
		svcNoLabel := newUnstructuredService("prod", "api-svc", "10.0.0.2")
		svcNoLabel.SetLabels(map[string]string{"app": "api"})

		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, kubectlGVRs,
			svcWithLabel, svcNoLabel,
		)

		result, err := tools.HandleKubectlList(context.Background(), client, nil, tools.KubectlListArgs{
			Kind:          "Service",
			Namespace:     "prod",
			LabelSelector: "app=web",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(1))
	})

	It("UT-AF-1230-013: redacts Secret .data in list results", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, kubectlGVRs,
			newUnstructuredSecret("prod", "db-creds"),
			newUnstructuredSecret("prod", "api-keys"),
		)

		result, err := tools.HandleKubectlList(context.Background(), client, nil, tools.KubectlListArgs{
			Kind:      "Secret",
			Namespace: "prod",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(2))
		for _, item := range result.Items {
			data, exists := item["data"]
			if exists {
				dataMap, ok := data.(map[string]interface{})
				Expect(ok).To(BeTrue())
				for _, v := range dataMap {
					Expect(v).To(Equal("REDACTED"))
				}
			}
		}
	})

	It("UT-AF-1230-014: rejects empty kind", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, kubectlGVRs)

		_, err := tools.HandleKubectlList(context.Background(), client, nil, tools.KubectlListArgs{
			Kind:      "",
			Namespace: "prod",
		})
		Expect(err).To(MatchError(ContainSubstring("invalid input")))
	})

	It("UT-AF-1230-015: rejects invalid namespace", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, kubectlGVRs)

		_, err := tools.HandleKubectlList(context.Background(), client, nil, tools.KubectlListArgs{
			Kind:      "Service",
			Namespace: "../etc",
		})
		Expect(err).To(MatchError(ContainSubstring("invalid input")))
	})

	It("UT-AF-1230-016: nil client returns ErrK8sUnavailable", func() {
		_, err := tools.HandleKubectlList(context.Background(), nil, nil, tools.KubectlListArgs{
			Kind:      "Service",
			Namespace: "prod",
		})
		Expect(err).To(MatchError(tools.ErrK8sUnavailable))
	})

	It("UT-AF-1230-018: large list triggers truncation", func() {
		scheme := runtime.NewScheme()
		objects := make([]runtime.Object, 0, 200)
		for i := 0; i < 200; i++ {
			objects = append(objects, newUnstructuredService("prod", fmt.Sprintf("svc-%03d", i), "10.0.0.1"))
		}
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, kubectlGVRs, objects...)

		result, err := tools.HandleKubectlList(context.Background(), client, nil, tools.KubectlListArgs{
			Kind:      "Service",
			Namespace: "prod",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Truncated).To(BeTrue())
		Expect(result.Count).To(BeNumerically("<", 200))
	})

	It("UT-AF-1230-017: concurrent calls are safe", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, kubectlGVRs,
			newUnstructuredService("test", "svc", "10.0.0.1"),
		)

		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := tools.HandleKubectlList(context.Background(), client, nil, tools.KubectlListArgs{
					Kind:      "Service",
					Namespace: "test",
				})
				Expect(err).NotTo(HaveOccurred())
			}()
		}
		wg.Wait()
	})
})
