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

var eventsGVRTest = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}

var _ = Describe("af_list_events", func() {
	var (
		ctx    context.Context
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
	})

	It("UT-AF-052-001: returns events in namespace", func() {
		ev := newUnstructuredEvent("prod", "pod-crash", "BackOff", "Back-off restarting failed container", "Pod", "my-pod")
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{eventsGVRTest: "EventList"}, ev)

		result, err := tools.HandleListEvents(ctx, client, tools.ListEventsArgs{Namespace: "prod"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(1))
		Expect(result.Events[0].Reason).To(Equal("BackOff"))
		Expect(result.Events[0].InvolvedName).To(Equal("my-pod"))
	})

	It("UT-AF-052-002: empty namespace rejected", func() {
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{eventsGVRTest: "EventList"})
		_, err := tools.HandleListEvents(ctx, client, tools.ListEventsArgs{Namespace: ""})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid input"))
	})

	It("UT-AF-052-003: path traversal namespace rejected", func() {
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{eventsGVRTest: "EventList"})
		_, err := tools.HandleListEvents(ctx, client, tools.ListEventsArgs{Namespace: "../../etc/passwd"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid input"))
	})

	It("UT-AF-052-004: namespace with no events returns empty list", func() {
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{eventsGVRTest: "EventList"})
		result, err := tools.HandleListEvents(ctx, client, tools.ListEventsArgs{Namespace: "empty-ns"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(0))
		Expect(result.Events).To(BeEmpty())
	})

	It("UT-AF-052-005: large result is trimmed", func() {
		var objs []runtime.Object
		for i := range 200 {
			objs = append(objs, newUnstructuredEvent("prod", fmt.Sprintf("ev-%d", i),
				"Warning", strings.Repeat("long message content ", 20), "Pod", "pod-"+strings.Repeat("x", 50)))
		}
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{
				{Group: "", Version: "v1", Resource: "events"}: "EventList",
			}, objs...)

		result, err := tools.HandleListEvents(ctx, client, tools.ListEventsArgs{Namespace: "prod"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Truncated).To(BeTrue())
		Expect(result.Count).To(BeNumerically("<", 200))
	})

	It("UT-AF-052-006: nil client returns ErrK8sUnavailable", func() {
		_, err := tools.HandleListEvents(ctx, nil, tools.ListEventsArgs{Namespace: "prod"})
		Expect(err).To(MatchError(tools.ErrK8sUnavailable))
	})

	It("UT-AF-052-007: concurrent calls are safe", func() {
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{eventsGVRTest: "EventList"})
		var wg sync.WaitGroup
		errs := make([]error, 10)
		for i := range 10 {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				_, errs[idx] = tools.HandleListEvents(ctx, client, tools.ListEventsArgs{Namespace: "prod"})
			}(i)
		}
		wg.Wait()
		for _, e := range errs {
			Expect(e).NotTo(HaveOccurred())
		}
	})

	It("UT-AF-052-008: filters by reason when provided", func() {
		ev1 := newUnstructuredEvent("prod", "ev-1", "BackOff", "msg1", "Pod", "pod1")
		ev2 := newUnstructuredEvent("prod", "ev-2", "Scheduled", "msg2", "Pod", "pod2")
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{eventsGVRTest: "EventList"}, ev1, ev2)

		result, err := tools.HandleListEvents(ctx, client, tools.ListEventsArgs{
			Namespace: "prod",
			Reason:    "BackOff",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(1))
		Expect(result.Events[0].Reason).To(Equal("BackOff"))
	})

	It("UT-AF-052-009: filters by involved_kind when provided", func() {
		evPod := newUnstructuredEvent("prod", "ev-1", "BackOff", "container crash", "Pod", "web-abc")
		evDeploy := newUnstructuredEvent("prod", "ev-2", "ScalingReplicaSet", "scaled up", "Deployment", "web")
		evPod2 := newUnstructuredEvent("prod", "ev-3", "Pulled", "image pulled", "Pod", "web-def")
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{eventsGVRTest: "EventList"}, evPod, evDeploy, evPod2)

		result, err := tools.HandleListEvents(ctx, client, tools.ListEventsArgs{
			Namespace: "prod",
			Kind:      "Pod",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(2))
		for _, ev := range result.Events {
			Expect(ev.InvolvedKind).To(Equal("Pod"))
		}
	})

	It("UT-AF-052-010: filters by both reason and involved_kind", func() {
		ev1 := newUnstructuredEvent("prod", "ev-1", "BackOff", "crash", "Pod", "pod1")
		ev2 := newUnstructuredEvent("prod", "ev-2", "BackOff", "scale fail", "Deployment", "web")
		ev3 := newUnstructuredEvent("prod", "ev-3", "Pulled", "ok", "Pod", "pod2")
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{eventsGVRTest: "EventList"}, ev1, ev2, ev3)

		result, err := tools.HandleListEvents(ctx, client, tools.ListEventsArgs{
			Namespace: "prod",
			Reason:    "BackOff",
			Kind:      "Pod",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(1))
		Expect(result.Events[0].InvolvedName).To(Equal("pod1"))
	})
})

func newUnstructuredEvent(ns, name, reason, message, involvedKind, involvedName string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Event",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
			},
			"reason":  reason,
			"message": message,
			"involvedObject": map[string]interface{}{
				"kind": involvedKind,
				"name": involvedName,
			},
			"count":         int64(1),
			"lastTimestamp": "2026-05-08T10:00:00Z",
		},
	}
}
