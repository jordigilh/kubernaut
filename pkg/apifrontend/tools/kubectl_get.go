package tools

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

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
}

// KubectlGetResult is the output of kubectl_get.
type KubectlGetResult struct {
	Kind      string                 `json:"kind"`
	Name      string                 `json:"name"`
	Namespace string                 `json:"namespace"`
	Object    map[string]interface{} `json:"object"`
}

// HandleKubectlGet retrieves a single Kubernetes resource by kind/name/namespace.
func HandleKubectlGet(ctx context.Context, client dynamic.Interface, mapper meta.RESTMapper, args KubectlGetArgs) (KubectlGetResult, error) {
	if client == nil {
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

	gvr, err := resolveGVR(mapper, args.Kind)
	if err != nil {
		return KubectlGetResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	obj, err := client.Resource(gvr).Namespace(args.Namespace).Get(ctx, args.Name, metav1.GetOptions{})
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

// resolveGVR maps a Kind string to a GroupVersionResource using the static
// table in pkg/shared/k8s and UnsafeGuessKindToResource for pluralization.
func resolveGVR(mapper meta.RESTMapper, kind string) (schema.GroupVersionResource, error) {
	gvk, err := sharedK8s.ResolveGVKForKind(mapper, kind)
	if err != nil {
		return schema.GroupVersionResource{}, err
	}
	plural, _ := meta.UnsafeGuessKindToResource(gvk)
	return plural, nil
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
func NewKubectlGetTool(factory auth.DynamicClientFactory, mapper meta.RESTMapper) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubectl_get",
		Description: "Get a single Kubernetes resource by kind, name, and namespace. Secret .data is redacted.",
	}, func(ctx tool.Context, args KubectlGetArgs) (KubectlGetResult, error) {
		client, err := factory(ctx)
		if err != nil {
			return KubectlGetResult{}, fmt.Errorf("%w", ErrK8sUnavailable)
		}
		return HandleKubectlGet(ctx, client, mapper, args)
	})
}
