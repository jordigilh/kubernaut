package tools

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	sharedK8s "github.com/jordigilh/kubernaut/pkg/shared/k8s"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
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

type scopeCheckerContextKey struct{}

// ContextWithScopeChecker returns a new context carrying the given ScopeChecker.
// Used by kubernaut_investigate (both AF transports — agent/root.go and
// handler/mcp_bridge.go) and kubernaut_remediate to thread the optional
// scope-checking dependency into HandleInvestigationMCPWithRegistry/
// HandleRemediate without widening their already-large positional
// signatures (#2022). Mirrors ContextWithRESTMapper above.
func ContextWithScopeChecker(ctx context.Context, checker scope.ScopeChecker) context.Context {
	if checker == nil {
		return ctx
	}
	return context.WithValue(ctx, scopeCheckerContextKey{}, checker)
}

// ScopeCheckerFromContext extracts the ScopeChecker stored in ctx, or nil if none.
func ScopeCheckerFromContext(ctx context.Context) scope.ScopeChecker {
	v, _ := ctx.Value(scopeCheckerContextKey{}).(scope.ScopeChecker)
	return v
}

// checkRRScope validates that namespace/kind/name are within Kubernaut's
// management scope (ADR-053) before HandleInvestigateAlert, HandleRemediate,
// or HandleInvestigationMCPWithRegistry create an RR — closing the gap where
// this was previously only caught downstream by RO's CheckUnmanagedResource,
// after an RR (and, for interactive tools, an InvestigationSession) had
// already been wastefully created (#2022, ADR-053 Addendum "Point 3").
//
// nil checker = always managed (graceful degradation, matches the nil-safe
// Mapper/PromClient convention elsewhere in this package) — returns
// managed=true with an empty message.
//
// On a scope-infrastructure error, fails closed (managed=false), mirroring
// RO's CheckUnmanagedResource fail-closed behavior (routing/blocking.go) —
// callers cannot distinguish "explicitly unmanaged" from "scope check
// errored" because both must equally prevent RR creation.
//
// On rejection, the message mirrors RO's exact wording (routing/blocking.go)
// so an agent gets identical guidance whether rejected here (fail-fast) or
// downstream by RO (temporal-drift re-check), and an EventRRScopeRejected
// audit event is emitted (AU-3/AU-12) when auditor is non-nil.
func checkRRScope(ctx context.Context, checker scope.ScopeChecker, auditor audit.Emitter, username, namespace, kind, name string) (managed bool, message string) {
	if checker == nil {
		return true, ""
	}

	managed, err := checker.IsManaged(ctx, namespace, kind, name)
	if err != nil {
		logr.FromContextOrDiscard(ctx).Error(err, "scope validation failed — rejecting RR creation (fail-closed)",
			"namespace", namespace, "kind", kind, "name", name)
		managed = false
	}
	if managed {
		return true, ""
	}

	message = fmt.Sprintf("Resource %s/%s/%s not managed by Kubernaut. "+
		"Add label kubernaut.ai/managed=true to namespace or resource.", namespace, kind, name)

	if auditor != nil {
		auditor.Emit(ctx, &audit.Event{
			Type:   audit.EventRRScopeRejected,
			UserID: username,
			Detail: map[string]string{
				"namespace": namespace,
				"kind":      kind,
				"name":      name,
			},
		})
	}

	return false, message
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
