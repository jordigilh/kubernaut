package tools_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var testAIAnalysisGVR = schema.GroupVersionResource{Group: "kubernaut.ai", Version: "v1alpha1", Resource: "aianalyses"}

func newUnstructuredAIAnalysis(ns, name, rrName, sessionID string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kubernaut.ai/v1alpha1",
			"kind":       "AIAnalysis",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
			},
			"spec": map[string]interface{}{
				"remediationRequestRef": map[string]interface{}{
					"name":      rrName,
					"namespace": ns,
				},
			},
		},
	}
	if sessionID != "" {
		obj.Object["status"] = map[string]interface{}{
			"investigationSession": map[string]interface{}{
				"id": sessionID,
			},
		}
	}
	return obj
}

func newSeededAIAnalysisClient(objects ...*unstructured.Unstructured) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			testAIAnalysisGVR: "AIAnalysisList",
		})
	for _, obj := range objects {
		ns := obj.GetNamespace()
		_, _ = client.Resource(testAIAnalysisGVR).Namespace(ns).Create(context.Background(), obj, metav1.CreateOptions{})
	}
	return client
}

var _ = Describe("kubernaut_await_session", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("HandleAwaitSession validation", func() {
		It("UT-AF-1293-SC8-003: returns error when client is nil", func() {
			_, err := tools.HandleAwaitSession(ctx, nil, tools.AwaitSessionArgs{
				Namespace: "default",
				RRName:    "rr-test",
			})
			Expect(err).To(HaveOccurred())
		})

		It("UT-AF-1293-SC8-004: returns error when namespace is empty", func() {
			client := newSeededAIAnalysisClient()
			_, err := tools.HandleAwaitSession(ctx, client, tools.AwaitSessionArgs{
				Namespace: "",
				RRName:    "rr-test",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid input"))
		})

		It("UT-AF-1293-SC8-005: returns error when rr_name is empty", func() {
			client := newSeededAIAnalysisClient()
			_, err := tools.HandleAwaitSession(ctx, client, tools.AwaitSessionArgs{
				Namespace: "default",
				RRName:    "",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("rr_name is required"))
		})
	})

	Describe("HandleAwaitSession fast-path", func() {
		It("UT-AF-1293-006: returns immediately when session already exists", func() {
			aa := newUnstructuredAIAnalysis("default", "aa-ready", "rr-ready", "session-xyz")
			client := newSeededAIAnalysisClient(aa)

			start := time.Now()
			result, err := tools.HandleAwaitSession(ctx, client, tools.AwaitSessionArgs{
				Namespace: "default",
				RRName:    "rr-ready",
			})
			elapsed := time.Since(start)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).To(Equal("ready"))
			Expect(result.SessionID).To(Equal("session-xyz"))
			Expect(elapsed).To(BeNumerically("<", 2*time.Second))
		})
	})

	Describe("HandleAwaitSession list filtering", func() {
		It("UT-AF-1293-007: list ignores AIAnalysis for different RR name", func() {
			aa := newUnstructuredAIAnalysis("default", "aa-other", "rr-other", "session-other")
			client := newSeededAIAnalysisClient(aa)

			list, err := client.Resource(testAIAnalysisGVR).Namespace("default").List(ctx, metav1.ListOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(list.Items).To(HaveLen(1))

			var found string
			for _, item := range list.Items {
				rrName, _, _ := unstructured.NestedString(item.Object, "spec", "remediationRequestRef", "name")
				if rrName != "rr-mine" {
					continue
				}
				sessionID, _, _ := unstructured.NestedString(item.Object, "status", "investigationSession", "id")
				if sessionID != "" {
					found = sessionID
				}
			}
			Expect(found).To(BeEmpty())
		})

		It("UT-AF-1293-008: list skips AIAnalysis with empty session ID", func() {
			aa := newUnstructuredAIAnalysis("default", "aa-nosession", "rr-nosession", "")
			client := newSeededAIAnalysisClient(aa)

			list, err := client.Resource(testAIAnalysisGVR).Namespace("default").List(ctx, metav1.ListOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(list.Items).To(HaveLen(1))

			var found string
			for _, item := range list.Items {
				rrName, _, _ := unstructured.NestedString(item.Object, "spec", "remediationRequestRef", "name")
				if rrName != "rr-nosession" {
					continue
				}
				sessionID, _, _ := unstructured.NestedString(item.Object, "status", "investigationSession", "id")
				if sessionID != "" {
					found = sessionID
				}
			}
			Expect(found).To(BeEmpty())
		})
	})
})
