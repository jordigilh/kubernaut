package tools

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/validate"
	sharedK8s "github.com/jordigilh/kubernaut/pkg/shared/k8s"
)

// KubectlGetArgs defines the input for kubectl_get.
type KubectlGetArgs struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	ClusterID string `json:"cluster_id,omitempty"`
}

// KubectlGetResult is the output of kubectl_get.
type KubectlGetResult struct {
	Kind      string                 `json:"kind"`
	Name      string                 `json:"name"`
	Namespace string                 `json:"namespace"`
	Object    map[string]interface{} `json:"object"`
}

// HandleKubectlGet retrieves a single Kubernetes resource by kind/name/namespace.
// Accepts ResourceReader to support both local (dynamic.Interface) and fleet (client.Reader) access.
func HandleKubectlGet(ctx context.Context, reader ResourceReader, mapper meta.RESTMapper, args KubectlGetArgs) (KubectlGetResult, error) {
	if reader == nil {
		return KubectlGetResult{}, ErrK8sUnavailable
	}
	if err := validate.Kind(args.Kind); err != nil {
		return KubectlGetResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if err := validate.ResourceName(args.Name); err != nil {
		return KubectlGetResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if err := validate.Namespace(args.Namespace); err != nil {
		return KubectlGetResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	gvr, gvk, err := resolveGVRAndGVK(mapper, args.Kind)
	if err != nil {
		return KubectlGetResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	ns := ResolveEffectiveNamespace(mapper, args.Kind, args.Namespace, logr.FromContextOrDiscard(ctx))

	obj, err := reader.GetResource(ctx, gvr, gvk, ns, args.Name)
	if err != nil {
		return KubectlGetResult{}, ToUserFriendlyError(err)
	}

	sanitized := sanitizeObject(obj.Object, args.Kind)

	return KubectlGetResult{
		Kind:      args.Kind,
		Name:      args.Name,
		Namespace: args.Namespace,
		Object:    sanitized,
	}, nil
}

// resolveGVRAndGVK maps a Kind string to both GroupVersionResource and
// GroupVersionKind using the static table in pkg/shared/k8s. Both are
// needed: GVR for dynamic.Interface and GVK for client.Reader dispatch.
func resolveGVRAndGVK(mapper meta.RESTMapper, kind string) (schema.GroupVersionResource, schema.GroupVersionKind, error) {
	gvk, err := sharedK8s.ResolveGVKForKind(mapper, kind)
	if err != nil {
		return schema.GroupVersionResource{}, schema.GroupVersionKind{}, err
	}
	plural, _ := meta.UnsafeGuessKindToResource(gvk)
	return plural, gvk, nil
}

// sanitizeObject removes sensitive fields from resource objects.
// For Secrets, .data and .stringData values are replaced with "REDACTED".
func sanitizeObject(obj map[string]interface{}, kind string) map[string]interface{} {
	if kind != "Secret" {
		return obj
	}
	redactMapField(obj, "data")
	redactMapField(obj, "stringData")
	return obj
}

func redactMapField(obj map[string]interface{}, field string) {
	raw, ok := obj[field]
	if !ok {
		return
	}
	dataMap, ok := raw.(map[string]interface{})
	if !ok {
		return
	}
	redacted := make(map[string]interface{}, len(dataMap))
	for k := range dataMap {
		redacted[k] = "REDACTED"
	}
	obj[field] = redacted
}

// NewKubectlGetTool creates the kubectl_get ADK tool.
// When readerFactory is non-nil, fleet routing is enabled: non-empty ClusterID
// in args routes the read through the MCP gateway to the target cluster.
func NewKubectlGetTool(factory auth.DynamicClientFactory, mapper meta.RESTMapper, readerFactory ResourceReaderFactory) (tool.Tool, error) {
	desc := "Get a single Kubernetes resource by kind, name, and namespace. Secret .data is redacted."
	if readerFactory != nil {
		desc += " Set cluster_id to read from a remote fleet cluster."
	}
	return functiontool.New(functiontool.Config{
		Name:        "kubectl_get",
		Description: desc,
	}, func(ctx tool.Context, args KubectlGetArgs) (KubectlGetResult, error) {
		var reader ResourceReader
		if readerFactory != nil && args.ClusterID != "" {
			r, err := readerFactory(ctx, args.ClusterID)
			if err != nil {
				return KubectlGetResult{}, fmt.Errorf("fleet reader for cluster %q: %w", args.ClusterID, err)
			}
			reader = r
		} else {
			client, err := factory(ctx)
			if err != nil {
				return KubectlGetResult{}, fmt.Errorf("%w", ErrK8sUnavailable)
			}
			reader = &DynamicResourceReader{Client: client}
		}
		return HandleKubectlGet(ctx, reader, mapper, args)
	})
}
