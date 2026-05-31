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

	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
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
	return newUnstructuredEventWithType(ns, name, reason, message, involvedKind, involvedName, "Normal")
}

func newUnstructuredEventWithType(ns, name, reason, message, involvedKind, involvedName, eventType string) *unstructured.Unstructured {
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
			"type":    eventType,
			"involvedObject": map[string]interface{}{
				"kind": involvedKind,
				"name": involvedName,
			},
			"count":         int64(1),
			"lastTimestamp": "2026-05-08T10:00:00Z",
		},
	}
}

var _ = Describe("EventSummary.Type (#1282 F-EVT)", func() {
	It("UT-AF-1282-EVT-001: EventSummary includes Type field from K8s event", func() {
		ev := newUnstructuredEventWithType("prod", "ev-1", "OOMKilling", "killed", "Pod", "worker-1", "Warning")
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{eventsGVRTest: "EventList"}, ev)

		result, err := tools.HandleListEvents(context.Background(), client, tools.ListEventsArgs{Namespace: "prod"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Events).To(HaveLen(1))
		Expect(result.Events[0].Type).To(Equal("Warning"))
	})

	It("UT-AF-1282-EVT-002: missing type defaults to empty string", func() {
		ev := newUnstructuredEvent("prod", "ev-1", "Pulled", "pulled image", "Pod", "worker-1")
		delete(ev.Object, "type")
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{eventsGVRTest: "EventList"}, ev)

		result, err := tools.HandleListEvents(context.Background(), client, tools.ListEventsArgs{Namespace: "prod"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Events[0].Type).To(BeEmpty())
	})
})

var _ = Describe("DominantEventReason (#1282 F-SIG)", func() {
	It("UT-AF-1282-SIG-001: OOMKilling dominates BackOff", func() {
		events := []tools.EventSummary{
			{Reason: "BackOff", Type: "Warning", Count: 5},
			{Reason: "OOMKilling", Type: "Warning", Count: 1},
		}
		Expect(tools.DominantEventReason(events)).To(Equal("OOMKilling"))
	})

	It("UT-AF-1282-SIG-002: FailedScheduling dominates Pulled", func() {
		events := []tools.EventSummary{
			{Reason: "Pulled", Type: "Normal", Count: 3},
			{Reason: "FailedScheduling", Type: "Warning", Count: 1},
		}
		Expect(tools.DominantEventReason(events)).To(Equal("FailedScheduling"))
	})

	It("UT-AF-1282-SIG-003: highest-count Warning wins among same-priority", func() {
		events := []tools.EventSummary{
			{Reason: "BackOff", Type: "Warning", Count: 10},
			{Reason: "Unhealthy", Type: "Warning", Count: 3},
		}
		Expect(tools.DominantEventReason(events)).To(Equal("BackOff"))
	})

	It("UT-AF-1282-SIG-004: empty events returns empty string", func() {
		Expect(tools.DominantEventReason(nil)).To(BeEmpty())
	})

	It("UT-AF-1282-SIG-005: only Normal lifecycle events returns empty (not operationally significant)", func() {
		events := []tools.EventSummary{
			{Reason: "Pulled", Type: "Normal", Count: 1},
			{Reason: "Created", Type: "Normal", Count: 5},
		}
		Expect(tools.DominantEventReason(events)).To(BeEmpty())
	})

	It("UT-AF-1282-SIG-006: ScalingReplicaSet-only returns empty (FP E2E canary)", func() {
		events := []tools.EventSummary{
			{Reason: "ScalingReplicaSet", Type: "Normal", Count: 3},
			{Reason: "Scheduled", Type: "Normal", Count: 2},
			{Reason: "Pulling", Type: "Normal", Count: 1},
		}
		Expect(tools.DominantEventReason(events)).To(BeEmpty())
	})

	It("UT-AF-1282-SIG-007: 3-way priority: OOMKilling > BackOff > FailedScheduling count-ignored", func() {
		events := []tools.EventSummary{
			{Reason: "FailedScheduling", Type: "Warning", Count: 100},
			{Reason: "BackOff", Type: "Warning", Count: 50},
			{Reason: "OOMKilling", Type: "Warning", Count: 1},
		}
		Expect(tools.DominantEventReason(events)).To(Equal("OOMKilling"))
	})

	It("UT-AF-1282-SIG-008: Normal events filtered by Warning with lower count", func() {
		events := []tools.EventSummary{
			{Reason: "Pulled", Type: "Normal", Count: 100},
			{Reason: "BackOff", Type: "Warning", Count: 2},
		}
		Expect(tools.DominantEventReason(events)).To(Equal("BackOff"))
	})
})

var _ = Describe("FilterRelatedPodEvents (#1282 F-SIG)", func() {
	It("UT-AF-1282-SIG-016: filters pods by owner name prefix", func() {
		events := []tools.EventSummary{
			{Reason: "BackOff", InvolvedName: "web-abc123-xyz", InvolvedKind: "Pod", Type: "Warning", Count: 3},
			{Reason: "OOMKilling", InvolvedName: "database-def456", InvolvedKind: "Pod", Type: "Warning", Count: 1},
			{Reason: "Pulled", InvolvedName: "web-abc123-xyz", InvolvedKind: "Pod", Type: "Normal", Count: 1},
		}
		filtered := tools.FilterRelatedPodEvents(events, "web")
		Expect(filtered).To(HaveLen(2))
		for _, ev := range filtered {
			Expect(ev.InvolvedName).To(HavePrefix("web-"))
		}
	})

	It("UT-AF-1282-SIG-017: returns empty when no pods match", func() {
		events := []tools.EventSummary{
			{Reason: "BackOff", InvolvedName: "database-abc", InvolvedKind: "Pod", Type: "Warning", Count: 1},
		}
		filtered := tools.FilterRelatedPodEvents(events, "web")
		Expect(filtered).To(BeEmpty())
	})

	It("UT-AF-1282-SIG-018: exact name match without dash suffix is excluded", func() {
		events := []tools.EventSummary{
			{Reason: "BackOff", InvolvedName: "web", InvolvedKind: "Pod", Type: "Warning", Count: 1},
		}
		filtered := tools.FilterRelatedPodEvents(events, "web")
		Expect(filtered).To(BeEmpty(),
			"exact match 'web' should not match prefix 'web-' (pods always have hash suffix)")
	})
})

var _ = Describe("EventSummary mixed types (#1282 F-EVT)", func() {
	It("UT-AF-1282-EVT-003: mixed Warning/Normal events all have Type populated", func() {
		evWarn := newUnstructuredEventWithType("prod", "ev-w", "OOMKilling", "killed", "Pod", "p1", "Warning")
		evNorm := newUnstructuredEventWithType("prod", "ev-n", "Pulled", "pulled", "Pod", "p1", "Normal")
		scheme := runtime.NewScheme()
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{eventsGVRTest: "EventList"}, evWarn, evNorm)

		result, err := tools.HandleListEvents(context.Background(), client, tools.ListEventsArgs{Namespace: "prod"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Events).To(HaveLen(2))
		for _, ev := range result.Events {
			Expect(ev.Type).NotTo(BeEmpty(), "event %q should have Type set", ev.Reason)
		}
	})
})
