package tools

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
	ClusterID     string `json:"cluster_id,omitempty"`
}

// KubectlListResult is the output of kubectl_list.
type KubectlListResult struct {
	Items     []map[string]interface{} `json:"items"`
	Count     int                      `json:"count"`
	Truncated bool                     `json:"truncated,omitempty"`
}

// HandleKubectlList lists Kubernetes resources by kind/namespace with optional label selector.
// Accepts ResourceReader to support both local (dynamic.Interface) and fleet (client.Reader) access.
func HandleKubectlList(ctx context.Context, reader ResourceReader, mapper meta.RESTMapper, args KubectlListArgs) (KubectlListResult, error) {
	if reader == nil {
		return KubectlListResult{}, ErrK8sUnavailable
	}
	if err := validate.Kind(args.Kind); err != nil {
		return KubectlListResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if err := validate.Namespace(args.Namespace); err != nil {
		return KubectlListResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	gvr, gvk, err := resolveGVRAndGVK(mapper, args.Kind)
	if err != nil {
		return KubectlListResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	ns := ResolveEffectiveNamespace(mapper, args.Kind, args.Namespace, logr.FromContextOrDiscard(ctx))

	opts := metav1.ListOptions{}
	if args.LabelSelector != "" {
		opts.LabelSelector = args.LabelSelector
	}

	list, err := reader.ListResources(ctx, gvr, gvk, ns, opts)
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
// When readerFactory is non-nil, fleet routing is enabled: non-empty ClusterID
// in args routes the read through the MCP gateway to the target cluster.
func NewKubectlListTool(factory auth.DynamicClientFactory, mapper meta.RESTMapper, readerFactory ResourceReaderFactory) (tool.Tool, error) {
	desc := "List Kubernetes resources by kind and namespace with optional label selector. Secret .data is redacted."
	if readerFactory != nil {
		desc += " Set cluster_id to list from a remote fleet cluster."
	}
	return functiontool.New(functiontool.Config{
		Name:        "kubectl_list",
		Description: desc,
	}, func(ctx tool.Context, args KubectlListArgs) (KubectlListResult, error) {
		var reader ResourceReader
		if readerFactory != nil && args.ClusterID != "" {
			r, err := readerFactory(ctx, args.ClusterID)
			if err != nil {
				return KubectlListResult{}, fmt.Errorf("fleet reader for cluster %q: %w", args.ClusterID, err)
			}
			reader = r
		} else {
			client, err := factory(ctx)
			if err != nil {
				return KubectlListResult{}, fmt.Errorf("%w", ErrK8sUnavailable)
			}
			reader = &DynamicResourceReader{Client: client}
		}
		return HandleKubectlList(ctx, reader, mapper, args)
	})
}
