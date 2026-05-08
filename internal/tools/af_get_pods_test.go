package tools_test

import (
	"context"
	"fmt"
	"strings"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/jordigilh/kubernaut-apifrontend/internal/tools"
)

var podsGVRTest = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}

var _ = Describe("af_get_pods", func() {
	var (
		ctx    context.Context
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
	})

	It("UT-AF-052-010: returns pod summaries with container states", func() {
		pod := newUnstructuredPodObj("prod", "my-pod", "Running", []interface{}{
			map[string]interface{}{"name": "app", "ready": true, "state": map[string]interface{}{"running": map[string]interface{}{}}, "restartCount": int64(0)},
			map[string]interface{}{"name": "sidecar", "ready": false, "state": map[string]interface{}{"waiting": map[string]interface{}{"reason": "CrashLoopBackOff"}}, "restartCount": int64(5)},
		})
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{podsGVRTest: "PodList"}, pod)

		result, err := tools.HandleGetPods(ctx, client, tools.GetPodsArgs{Namespace: "prod"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(1))
		Expect(result.Pods[0].Name).To(Equal("my-pod"))
		Expect(result.Pods[0].Phase).To(Equal("Running"))
		Expect(result.Pods[0].Containers).To(HaveLen(2))
		Expect(result.Pods[0].Containers[0].State).To(Equal("running"))
		Expect(result.Pods[0].Containers[1].State).To(Equal("waiting"))
		Expect(result.Pods[0].Containers[1].Reason).To(Equal("CrashLoopBackOff"))
		Expect(result.Pods[0].Containers[1].Restarts).To(Equal(int64(5)))
	})

	It("UT-AF-052-011: empty namespace rejected", func() {
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{podsGVRTest: "PodList"})
		_, err := tools.HandleGetPods(ctx, client, tools.GetPodsArgs{Namespace: ""})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid input"))
	})

	It("UT-AF-052-012: unicode namespace rejected", func() {
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{podsGVRTest: "PodList"})
		_, err := tools.HandleGetPods(ctx, client, tools.GetPodsArgs{Namespace: "名前空間"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid input"))
	})

	It("UT-AF-052-013: max-length+1 namespace rejected", func() {
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{podsGVRTest: "PodList"})
		longNS := strings.Repeat("a", 64)
		_, err := tools.HandleGetPods(ctx, client, tools.GetPodsArgs{Namespace: longNS})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid input"))
	})

	It("UT-AF-052-014: large result is trimmed", func() {
		var objs []runtime.Object
		for i := range 100 {
			pod := newUnstructuredPodObj("prod", fmt.Sprintf("pod-%s-%d", strings.Repeat("x", 30), i), "Running", []interface{}{
				map[string]interface{}{"name": "c1", "ready": true, "state": map[string]interface{}{"running": map[string]interface{}{}}, "restartCount": int64(0)},
				map[string]interface{}{"name": "c2", "ready": true, "state": map[string]interface{}{"running": map[string]interface{}{}}, "restartCount": int64(0)},
			})
			objs = append(objs, pod)
		}
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{podsGVRTest: "PodList"}, objs...)

		result, err := tools.HandleGetPods(ctx, client, tools.GetPodsArgs{Namespace: "prod"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Truncated).To(BeTrue())
		Expect(result.Count).To(BeNumerically("<", 100))
	})

	It("UT-AF-052-015: nil client returns ErrK8sUnavailable", func() {
		_, err := tools.HandleGetPods(ctx, nil, tools.GetPodsArgs{Namespace: "prod"})
		Expect(err).To(MatchError(tools.ErrK8sUnavailable))
	})

	It("UT-AF-052-016: concurrent calls safe", func() {
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{podsGVRTest: "PodList"})
		var wg sync.WaitGroup
		errs := make([]error, 10)
		for i := range 10 {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				_, errs[idx] = tools.HandleGetPods(ctx, client, tools.GetPodsArgs{Namespace: "prod"})
			}(i)
		}
		wg.Wait()
		for _, e := range errs {
			Expect(e).NotTo(HaveOccurred())
		}
	})

	It("UT-AF-052-017: passes label selector to API call", func() {
		// Pods with matching labels set in metadata
		pod1 := &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "web-1",
				"namespace": "prod",
				"labels":    map[string]interface{}{"app": "web"},
			},
			"status": map[string]interface{}{"phase": "Running"},
		}}
		pod2 := &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      "worker-1",
				"namespace": "prod",
				"labels":    map[string]interface{}{"app": "worker"},
			},
			"status": map[string]interface{}{"phase": "Running"},
		}}
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{podsGVRTest: "PodList"}, pod1, pod2)

		result, err := tools.HandleGetPods(ctx, client, tools.GetPodsArgs{
			Namespace:     "prod",
			LabelSelector: "app=web",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(1))
		Expect(result.Pods[0].Name).To(Equal("web-1"))
	})
})

func newUnstructuredPodObj(ns, name, phase string, containerStatuses []interface{}) *unstructured.Unstructured {
	obj := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": ns,
		},
		"spec": map[string]interface{}{
			"nodeName": "node-1",
		},
		"status": map[string]interface{}{
			"phase": phase,
		},
	}
	if containerStatuses != nil {
		obj["status"].(map[string]interface{})["containerStatuses"] = containerStatuses
	}
	return &unstructured.Unstructured{Object: obj}
}
