package tools

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"

	sharedK8s "github.com/jordigilh/kubernaut/pkg/shared/k8s"
)

type restMapperContextKey struct{}

// ContextWithRESTMapper returns a new context carrying the given RESTMapper.
func ContextWithRESTMapper(ctx context.Context, mapper meta.RESTMapper) context.Context {
	if mapper == nil {
		return ctx
	}
	return context.WithValue(ctx, restMapperContextKey{}, mapper)
}

// RESTMapperFromContext extracts the RESTMapper stored in ctx, or nil if none.
func RESTMapperFromContext(ctx context.Context) meta.RESTMapper {
	v, _ := ctx.Value(restMapperContextKey{}).(meta.RESTMapper)
	return v
}

// ResolveEffectiveNamespace returns the namespace to use for a Kubernetes API
// call. When the RESTMapper confirms the kind is cluster-scoped and a namespace
// was provided (e.g., by the LLM), the namespace is stripped (returns "") and a
// warning is logged (AU-3). When no mapper is available or lookup fails, the
// original namespace is returned unchanged (fail-open).
func ResolveEffectiveNamespace(mapper meta.RESTMapper, kind, namespace string, logger logr.Logger) string {
	if mapper == nil || namespace == "" {
		return namespace
	}
	gvk, err := sharedK8s.ResolveGVKForKind(mapper, kind)
	if err != nil {
		return namespace
	}
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return namespace
	}
	if mapping.Scope.Name() != meta.RESTScopeNameNamespace {
		if logger.Enabled() {
			logger.Info("stripping namespace for cluster-scoped resource",
				"kind", kind,
				"apiVersion", gvk.GroupVersion().String(),
				"stripped_namespace", namespace,
			)
		}
		return ""
	}
	return namespace
}
