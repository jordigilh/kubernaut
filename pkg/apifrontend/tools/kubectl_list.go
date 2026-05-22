package tools

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/dynamic"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/validate"
)

// KubectlListArgs defines the input for kubectl_list.
type KubectlListArgs struct {
	Kind          string `json:"kind"`
	Namespace     string `json:"namespace"`
	LabelSelector string `json:"label_selector,omitempty"`
}

// KubectlListResult is the output of kubectl_list.
type KubectlListResult struct {
	Items     []map[string]interface{} `json:"items"`
	Count     int                      `json:"count"`
	Truncated bool                     `json:"truncated,omitempty"`
}

// HandleKubectlList lists Kubernetes resources by kind/namespace with optional label selector.
func HandleKubectlList(ctx context.Context, client dynamic.Interface, mapper meta.RESTMapper, args KubectlListArgs) (KubectlListResult, error) {
	if client == nil {
		return KubectlListResult{}, ErrK8sUnavailable
	}
	if err := validate.Kind(args.Kind); err != nil {
		return KubectlListResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if err := validate.Namespace(args.Namespace); err != nil {
		return KubectlListResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	gvr, err := resolveGVR(mapper, args.Kind)
	if err != nil {
		return KubectlListResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	opts := metav1.ListOptions{}
	if args.LabelSelector != "" {
		opts.LabelSelector = args.LabelSelector
	}

	list, err := client.Resource(gvr).Namespace(args.Namespace).List(ctx, opts)
	if err != nil {
		return KubectlListResult{}, ToUserFriendlyError(err)
	}

	items := make([]map[string]interface{}, 0, len(list.Items))
	for i := range list.Items {
		sanitized := sanitizeObject(list.Items[i].Object, args.Kind)
		items = append(items, sanitized)
	}

	items, truncated := TrimSliceToFit(items)

	return KubectlListResult{
		Items:     items,
		Count:     len(items),
		Truncated: truncated,
	}, nil
}

// NewKubectlListTool creates the kubectl_list ADK tool.
func NewKubectlListTool(factory auth.DynamicClientFactory, mapper meta.RESTMapper) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubectl_list",
		Description: "List Kubernetes resources by kind and namespace with optional label selector. Secret .data is redacted.",
	}, func(ctx tool.Context, args KubectlListArgs) (KubectlListResult, error) {
		client, err := factory(ctx)
		if err != nil {
			return KubectlListResult{}, fmt.Errorf("%w", ErrK8sUnavailable)
		}
		return HandleKubectlList(ctx, client, mapper, args)
	})
}
