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

	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
	"github.com/jordigilh/kubernaut-apifrontend/internal/validate"
)

// ListEventsArgs defines the input for af_list_events.
type ListEventsArgs struct {
	Namespace string `json:"namespace"`
	Reason    string `json:"reason,omitempty"`
	Kind      string `json:"involved_kind,omitempty"`
}

// EventSummary is a compact view of a Kubernetes Event.
type EventSummary struct {
	Reason        string `json:"reason"`
	Message       string `json:"message"`
	InvolvedKind  string `json:"involved_kind"`
	InvolvedName  string `json:"involved_name"`
	Count         int64  `json:"count"`
	LastTimestamp string `json:"last_timestamp,omitempty"`
}

// ListEventsResult is the output of af_list_events.
type ListEventsResult struct {
	Events    []EventSummary `json:"events"`
	Count     int            `json:"count"`
	Truncated bool           `json:"truncated,omitempty"`
}

var eventsGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}

// HandleListEvents implements the af_list_events logic.
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
		involvedName, _, _ := unstructured.NestedString(item.Object, "involvedObject", "name")
		count, _, _ := unstructured.NestedInt64(item.Object, "count")
		lastTS, _, _ := unstructured.NestedString(item.Object, "lastTimestamp")

		result = append(result, EventSummary{
			Reason:        reason,
			Message:       message,
			InvolvedKind:  involvedKind,
			InvolvedName:  involvedName,
			Count:         count,
			LastTimestamp:  lastTS,
		})
	}

	result, truncated := TrimSliceToFit(result)

	return ListEventsResult{
		Events:    result,
		Count:     len(result),
		Truncated: truncated,
	}, nil
}

// NewListEventsTool creates the af_list_events tool.
// Uses DynamicClientFactory to obtain a per-request impersonated client (SEC-05).
func NewListEventsTool(factory auth.DynamicClientFactory) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "af_list_events",
		Description: "List Kubernetes Events in a namespace, optionally filtered by reason or involved resource kind",
	}, func(ctx tool.Context, args ListEventsArgs) (ListEventsResult, error) {
		client, err := factory(ctx)
		if err != nil {
			return ListEventsResult{}, fmt.Errorf("%w", ErrK8sUnavailable)
		}
		return HandleListEvents(ctx, client, args)
	})
}
