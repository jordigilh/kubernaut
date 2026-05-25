package tools

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/validate"
)

// ListEventsArgs defines the input for kubectl_list_events.
type ListEventsArgs struct {
	Namespace string `json:"namespace"`
	Reason    string `json:"reason,omitempty"`
	Kind      string `json:"involved_kind,omitempty"`
}

// EventSummary is a compact view of a Kubernetes Event.
type EventSummary struct {
	Reason        string `json:"reason"`
	Message       string `json:"message"`
	Type          string `json:"type,omitempty"`
	InvolvedKind  string `json:"involved_kind"`
	InvolvedName  string `json:"involved_name"`
	Count         int64  `json:"count"`
	LastTimestamp string `json:"last_timestamp,omitempty"`
}

// ListEventsResult is the output of kubectl_list_events.
type ListEventsResult struct {
	Events    []EventSummary `json:"events"`
	Count     int            `json:"count"`
	Truncated bool           `json:"truncated,omitempty"`
}

var eventsGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}

// HandleListEvents implements the kubectl_list_events logic.
func HandleListEvents(ctx context.Context, client dynamic.Interface, args ListEventsArgs) (ListEventsResult, error) {
	if client == nil {
		return ListEventsResult{}, ErrK8sUnavailable
	}
	if err := validate.Namespace(args.Namespace); err != nil {
		return ListEventsResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	list, err := client.Resource(eventsGVR).Namespace(args.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return ListEventsResult{}, ToUserFriendlyError(err)
	}

	result := make([]EventSummary, 0, len(list.Items))
	for i := range list.Items {
		item := &list.Items[i]
		reason, _, _ := unstructured.NestedString(item.Object, "reason")
		involvedKind, _, _ := unstructured.NestedString(item.Object, "involvedObject", "kind")

		if args.Reason != "" && reason != args.Reason {
			continue
		}
		if args.Kind != "" && involvedKind != args.Kind {
			continue
		}

		message, _, _ := unstructured.NestedString(item.Object, "message")
		eventType, _, _ := unstructured.NestedString(item.Object, "type")
		involvedName, _, _ := unstructured.NestedString(item.Object, "involvedObject", "name")
		count, _, _ := unstructured.NestedInt64(item.Object, "count")
		lastTS, _, _ := unstructured.NestedString(item.Object, "lastTimestamp")

		result = append(result, EventSummary{
			Reason:        reason,
			Message:       message,
			Type:          eventType,
			InvolvedKind:  involvedKind,
			InvolvedName:  involvedName,
			Count:         count,
			LastTimestamp: lastTS,
		})
	}

	result, truncated := TrimSliceToFit(result)

	return ListEventsResult{
		Events:    result,
		Count:     len(result),
		Truncated: truncated,
	}, nil
}

// eventPriority maps K8s event reasons to severity priority tiers.
// Higher value = more severe. Events not in the map default to priority 0.
var eventPriority = map[string]int{
	"OOMKilling":         100,
	"OOMKilled":          100,
	"FailedScheduling":   90,
	"Evicted":            85,
	"FailedMount":        80,
	"FailedAttachVolume": 80,
	"NodeNotReady":       75,
	"BackOff":            70,
	"CrashLoopBackOff":   70,
	"Unhealthy":          60,
	"FailedCreate":       55,
	"FailedPullImage":    50,
	"ErrImagePull":       50,
}

// DominantEventReason selects the most operationally significant event reason
// from a slice of EventSummary. Selection is based on a tiered priority map;
// ties within the same priority are broken by event count.
func DominantEventReason(events []EventSummary) string {
	if len(events) == 0 {
		return ""
	}
	bestReason := ""
	bestPriority := -1
	var bestCount int64

	for i := range events {
		ev := &events[i]
		p := eventPriority[ev.Reason]
		if ev.Type == "Warning" && p == 0 {
			p = 1
		}
		if p > bestPriority || (p == bestPriority && ev.Count > bestCount) {
			bestPriority = p
			bestCount = ev.Count
			bestReason = ev.Reason
		}
	}
	return bestReason
}

// NewKubectlListEventsTool creates the kubectl_list_events tool.
// Uses DynamicClientFactory backed by AF's ServiceAccount (ADR-022).
func NewKubectlListEventsTool(factory auth.DynamicClientFactory) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubectl_list_events",
		Description: "List Kubernetes Events in a namespace, optionally filtered by reason or involved resource kind",
	}, func(ctx tool.Context, args ListEventsArgs) (ListEventsResult, error) {
		client, err := factory(ctx)
		if err != nil {
			return ListEventsResult{}, fmt.Errorf("%w", ErrK8sUnavailable)
		}
		return HandleListEvents(ctx, client, args)
	})
}
