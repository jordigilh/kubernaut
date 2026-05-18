package tools_test

import (
	"context"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/jordigilh/kubernaut-apifrontend/internal/tools"
)

func newUnstructuredDeployment(ns, name string, desired, ready, available int64) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
			},
			"spec": map[string]interface{}{
				"replicas": desired,
			},
			"status": map[string]interface{}{
				"readyReplicas":     ready,
				"availableReplicas": available,
			},
		},
	}
}

func newUnstructuredStatefulSet(ns, name string, desired, ready int64) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "StatefulSet",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
			},
			"spec": map[string]interface{}{
				"replicas": desired,
			},
			"status": map[string]interface{}{
				"readyReplicas": ready,
			},
		},
	}
}

var _ = Describe("af_get_workloads", func() {
	var (
		deployGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
		ssGVR     = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}
	)

	It("UT-AF-052-020: happy path returns Deployment and StatefulSet summaries", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{
				deployGVR: "DeploymentList",
				ssGVR:     "StatefulSetList",
			},
			newUnstructuredDeployment("prod", "web", 3, 3, 3),
			newUnstructuredStatefulSet("prod", "db", 2, 2),
		)

		result, err := tools.HandleGetWorkloads(context.Background(), client, tools.GetWorkloadsArgs{Namespace: "prod"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(2))
		Expect(result.Workloads[0].Kind).To(Equal("Deployment"))
		Expect(result.Workloads[0].Name).To(Equal("web"))
		Expect(result.Workloads[0].Replicas.Desired).To(Equal(int64(3)))
		Expect(result.Workloads[1].Kind).To(Equal("StatefulSet"))
	})

	It("UT-AF-052-021: empty namespace rejected", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{deployGVR: "DeploymentList", ssGVR: "StatefulSetList"})
		_, err := tools.HandleGetWorkloads(context.Background(), client, tools.GetWorkloadsArgs{Namespace: ""})
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(ContainSubstring("invalid input")))
	})

	It("UT-AF-052-022: invalid namespace rejected", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{deployGVR: "DeploymentList", ssGVR: "StatefulSetList"})
		_, err := tools.HandleGetWorkloads(context.Background(), client, tools.GetWorkloadsArgs{Namespace: "../etc"})
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(ContainSubstring("invalid input")))
	})

	It("UT-AF-052-023: nil client returns ErrK8sUnavailable", func() {
		_, err := tools.HandleGetWorkloads(context.Background(), nil, tools.GetWorkloadsArgs{Namespace: "default"})
		Expect(err).To(MatchError(tools.ErrK8sUnavailable))
	})

	It("UT-AF-052-024: name filter returns only matching workload", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{
				deployGVR: "DeploymentList",
				ssGVR:     "StatefulSetList",
			},
			newUnstructuredDeployment("prod", "web", 3, 3, 3),
			newUnstructuredDeployment("prod", "api", 2, 1, 1),
		)

		result, err := tools.HandleGetWorkloads(context.Background(), client, tools.GetWorkloadsArgs{Namespace: "prod", Name: "web"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(1))
		Expect(result.Workloads[0].Name).To(Equal("web"))
	})

	It("UT-AF-052-025: invalid name rejected", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{deployGVR: "DeploymentList", ssGVR: "StatefulSetList"})
		_, err := tools.HandleGetWorkloads(context.Background(), client, tools.GetWorkloadsArgs{
			Namespace: "default",
			Name:      "INVALID_NAME!!",
		})
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(ContainSubstring("invalid input")))
	})

	It("UT-AF-052-026: empty namespace returns empty list", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{
				deployGVR: "DeploymentList",
				ssGVR:     "StatefulSetList",
			})

		result, err := tools.HandleGetWorkloads(context.Background(), client, tools.GetWorkloadsArgs{Namespace: "empty-ns"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(0))
	})

	It("UT-AF-052-027: concurrent calls are safe", func() {
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{
				deployGVR: "DeploymentList",
				ssGVR:     "StatefulSetList",
			},
			newUnstructuredDeployment("test", "svc", 1, 1, 1),
		)

		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := tools.HandleGetWorkloads(context.Background(), client, tools.GetWorkloadsArgs{Namespace: "test"})
				Expect(err).NotTo(HaveOccurred())
			}()
		}
		wg.Wait()
	})
})
