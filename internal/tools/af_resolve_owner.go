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

const maxOwnerDepth = 10

// ResolveOwnerArgs defines the input for af_resolve_owner.
type ResolveOwnerArgs struct {
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
}

// OwnerChainEntry is one resource in an ownership chain.
type OwnerChainEntry struct {
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	APIVersion string `json:"apiVersion"`
}

// ResolveOwnerResult is the output of af_resolve_owner.
type ResolveOwnerResult struct {
	Chain    []OwnerChainEntry `json:"chain"`
	RootKind string            `json:"root_kind"`
	RootName string            `json:"root_name"`
}

func kindToGVR(kind string) (schema.GroupVersionResource, bool) {
	switch kind {
	case "Pod":
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}, true
	case "ReplicationController":
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "replicationcontrollers"}, true
	case "ReplicaSet":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}, true
	case "Deployment":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, true
	case "StatefulSet":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}, true
	case "DaemonSet":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}, true
	case "Job":
		return schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}, true
	case "CronJob":
		return schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "cronjobs"}, true
	default:
		return schema.GroupVersionResource{}, false
	}
}

func pickOwnerRef(obj map[string]interface{}) (kind, name string, ok bool) {
	refs, found, _ := unstructured.NestedSlice(obj, "metadata", "ownerReferences")
	if !found || len(refs) == 0 {
		return "", "", false
	}
	var chosen map[string]interface{}
	var first map[string]interface{}
	for _, r := range refs {
		ref, isMap := r.(map[string]interface{})
		if !isMap {
			continue
		}
		if first == nil {
			first = ref
		}
		if ctrl, has, _ := unstructured.NestedBool(ref, "controller"); has && ctrl {
			chosen = ref
			break
		}
	}
	if chosen == nil {
		chosen = first
	}
	if chosen == nil {
		return "", "", false
	}
	kind, _, _ = unstructured.NestedString(chosen, "kind")
	name, _, _ = unstructured.NestedString(chosen, "name")
	if kind == "" || name == "" {
		return "", "", false
	}
	return kind, name, true
}

// HandleResolveOwner implements the af_resolve_owner logic.
func HandleResolveOwner(ctx context.Context, client dynamic.Interface, args ResolveOwnerArgs) (ResolveOwnerResult, error) {
	var empty ResolveOwnerResult
	if client == nil {
		return empty, ErrK8sUnavailable
	}
	if err := validate.Namespace(args.Namespace); err != nil {
		return empty, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if err := validate.ResourceName(args.Name); err != nil {
		return empty, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if args.Kind == "" {
		return empty, fmt.Errorf("%w: kind must not be empty", ErrInvalidInput)
	}

	currentKind, currentName := args.Kind, args.Name
	visited := make(map[string]struct{})
	chain := make([]OwnerChainEntry, 0, maxOwnerDepth)

	for len(chain) < maxOwnerDepth {
		key := currentKind + "/" + currentName
		if _, seen := visited[key]; seen {
			break
		}

		gvr, known := kindToGVR(currentKind)
		if !known {
			if len(chain) == 0 {
				return empty, fmt.Errorf("%w: unsupported kind %q", ErrInvalidInput, currentKind)
			}
			break
		}
		visited[key] = struct{}{}

		obj, err := client.Resource(gvr).Namespace(args.Namespace).Get(ctx, currentName, metav1.GetOptions{})
		if err != nil {
			if len(chain) > 0 {
				break
			}
			return empty, ToUserFriendlyError(err)
		}

		chain = append(chain, OwnerChainEntry{
			Kind:       obj.GetKind(),
			Name:       obj.GetName(),
			APIVersion: obj.GetAPIVersion(),
		})

		nextKind, nextName, hasOwner := pickOwnerRef(obj.Object)
		if !hasOwner {
			break
		}
		currentKind, currentName = nextKind, nextName
	}

	result := ResolveOwnerResult{Chain: chain}
	if n := len(chain); n > 0 {
		result.RootKind = chain[n-1].Kind
		result.RootName = chain[n-1].Name
	}
	return result, nil
}

// NewResolveOwnerTool creates the af_resolve_owner tool.
// Uses DynamicClientFactory to obtain a per-request impersonated client (SEC-05).
func NewResolveOwnerTool(factory auth.DynamicClientFactory) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "af_resolve_owner",
		Description: "Resolve the ownership chain of a Kubernetes resource up to the root workload (max 10 hops)",
	}, func(ctx tool.Context, args ResolveOwnerArgs) (ResolveOwnerResult, error) {
		client, err := factory(ctx)
		if err != nil {
			return ResolveOwnerResult{}, fmt.Errorf("%w", ErrK8sUnavailable)
		}
		return HandleResolveOwner(ctx, client, args)
	})
}
