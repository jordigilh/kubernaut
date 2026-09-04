package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	sharedK8s "github.com/jordigilh/kubernaut/pkg/shared/k8s"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// ClusterLister lists the IDs of known fleet clusters for scope-refusal
// messaging when a target carries no cluster attribution (#2362). Satisfied
// structurally by registry.ClusterRegistry (List method) with no adapter;
// fakes implement one method. Nil-safe: a nil lister behaves as no known
// remotes (legacy message).
type ClusterLister interface {
	List() []registry.ClusterInfo
}

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

// checkRRScope validates that target is within Kubernaut's management scope
// (ADR-053/ADR-068) before HandleInvestigateAlert, HandleRemediate, or
// HandleInvestigationMCPWithRegistry create an RR — closing the gap where
// this was previously only caught downstream by RO's CheckUnmanagedResource,
// after an RR (and, for interactive tools, an InvestigationSession) had
// already been wastefully created (#2025, main clone of #2022; ADR-053
// Addendum "Point 3"). target is passed as a scope.ResourceIdentity
// (rather than four separate positional strings) to stay within this
// package's argument-count convention (revive argument-limit) and because
// it is exactly the shape checker.IsManagedResource already expects.
//
// nil checker = always managed (graceful degradation, matches the nil-safe
// Mapper/PromClient convention elsewhere in this package) — returns
// managed=true with an empty message.
//
// On a scope-infrastructure error, fails closed (managed=false), mirroring
// RO's CheckUnmanagedResource fail-closed behavior
// (pkg/remediationorchestrator/routing/blocking.go) — callers cannot
// distinguish "explicitly unmanaged" from "scope check errored" because both
// must equally prevent RR creation.
//
// On rejection, the message mirrors RO's exact wording (blocking.go) so an
// agent gets identical guidance whether rejected here (fail-fast) or
// downstream by RO (temporal-drift re-check), and an EventRRScopeRejected
// audit event is emitted (AU-3/AU-12) when auditor is non-nil.
//
// lister names known fleet clusters for the unattributed-refusal message
// (#2362). Nil-safe: a nil lister (fleet disabled, single cluster) preserves
// the legacy message byte-for-byte -- with no remotes, local is the only
// possible cluster and there is nothing to disambiguate.
func checkRRScope(ctx context.Context, checker scope.ScopeChecker, auditor audit.Emitter, username string, target scope.ResourceIdentity, lister ClusterLister) (managed bool, message string) {
	if checker == nil {
		return true, ""
	}

	clusterCtx := "local"
	if target.ClusterID != "" {
		clusterCtx = target.ClusterID
	}

	managed, err := checker.IsManagedResource(ctx, target)
	if err != nil {
		logr.FromContextOrDiscard(ctx).Error(err, "scope validation failed — rejecting RR creation (fail-closed)",
			"cluster", clusterCtx, "namespace", target.Namespace, "kind", target.Kind, "name", target.Name)
		managed = false
	}
	if managed {
		return true, ""
	}

	detail := map[string]string{
		"namespace": target.Namespace,
		"kind":      target.Kind,
		"name":      target.Name,
	}
	if target.ClusterID == "" {
		if ids := knownRemoteClusterIDs(lister); len(ids) > 0 {
			detail["candidate_clusters"] = strings.Join(ids, ",")
			message = fmt.Sprintf("Resource %s/%s/%s has no cluster attribution; cannot determine "+
				"which of [local hub, %s] it belongs to. Specify cluster_id (e.g. from the alert's "+
				"cluster label); not acting.", target.Namespace, target.Kind, target.Name,
				strings.Join(ids, ", "))
			emitScopeRejected(ctx, auditor, username, target, detail)
			return false, message
		}
	}

	message = fmt.Sprintf("Resource %s/%s/%s (cluster=%s) not managed by Kubernaut. "+
		"Add label kubernaut.ai/managed=true to namespace or resource.", target.Namespace, target.Kind, target.Name, clusterCtx)

	emitScopeRejected(ctx, auditor, username, target, detail)
	return false, message
}

// knownRemoteClusterIDs returns the sorted IDs of fleet clusters known to
// lister, or nil when lister is nil or knows none. Sorted for deterministic
// refusal messages.
func knownRemoteClusterIDs(lister ClusterLister) []string {
	if lister == nil {
		return nil
	}
	var ids []string
	for _, ci := range lister.List() {
		if ci.ID != "" {
			ids = append(ids, ci.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

func emitScopeRejected(ctx context.Context, auditor audit.Emitter, username string, target scope.ResourceIdentity, detail map[string]string) {
	if auditor == nil {
		// No emitter to record the refusal -- fail LOUD rather than silent:
		// audit traces are not optional, so an unaudited denial is logged at
		// error level with the full refusal context. (Deliberately not a
		// behavior change: nil-auditor nil-safe convention is pervasive
		// across the tool layer and production always wires a real emitter;
		// fail-closed denial itself never depends on the auditor.)
		logr.FromContextOrDiscard(ctx).Error(nil, "scope refusal emitted without auditor -- no audit trace recorded",
			"user", username, "namespace", target.Namespace, "kind", target.Kind,
			"name", target.Name, "cluster", target.ClusterID, "detail", detail)
		return
	}
	auditor.Emit(ctx, &audit.Event{
		Type:      audit.EventRRScopeRejected,
		UserID:    username,
		ClusterID: target.ClusterID,
		Detail:    detail,
	})
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
