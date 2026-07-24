/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package workflowcatalog provides KubernautAgent's read-only,
// informer-backed view of RemediationWorkflow and ActionType CRDs, plus
// (Phase 2b) the discovery/scoring logic that serves KA's MCP tools.
//
// DD-WORKFLOW-018 (Issue #1661): etcd is the sole source of truth for
// workflow/action-type definitions. DD-WORKFLOW-019 (Issue #1677): KA -- the
// sole production consumer of the discovery protocol -- owns this cache
// directly instead of proxying every lookup through DataStorage. KA never
// mutates RemediationWorkflow/ActionType CRDs; AuthWebhook is the sole write
// path (ADR-058, ADR-059).
//
// This file is a near-verbatim port of
// pkg/datastorage/workflowcache/cache.go (Change 5, Issue #1661 Phase 28).
package workflowcatalog

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	atv1alpha1 "github.com/jordigilh/kubernaut/api/actiontype/v1alpha1"
	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
)

// actionTypeFieldIndex indexes RemediationWorkflow CRDs by spec.actionType,
// enabling ListWorkflowsByActionType to do an indexed lookup instead of a
// full scan (Step 2 of the discovery protocol).
const actionTypeFieldIndex = ".spec.actionType"

// actionTypeNameFieldIndex indexes ActionType CRDs by spec.name (the
// PascalCase action-type identifier), which may differ from metadata.name.
// Mirrors AuthWebhook's existing ".spec.name" indexer
// (cmd/authwebhook/main.go buildAuthWebhookManager) so both services agree
// on how an ActionType is looked up by its business identifier.
const actionTypeNameFieldIndex = ".spec.name"

// workflowNameFieldIndex indexes RemediationWorkflow CRDs by metadata.name,
// so GetWorkflow can resolve a workflow cluster-wide without requiring a
// namespace -- workflow names are globally unique across the catalog
// (the same invariant the retired Postgres workflow_name column enforced).
const workflowNameFieldIndex = "metadata.name"

// workflowIDFieldIndex indexes RemediationWorkflow CRDs by status.workflowId
// (the deterministic content-hash UUID AuthWebhook computes and stamps),
// enabling GetWorkflowByID to do an indexed lookup instead of a full scan.
// Step 3 of the discovery protocol (GetWorkflowByID/GetWorkflowWithContext
// Filters): unlike metadata.name, workflow_id is a content identity, not a
// naming identity (DD-WORKFLOW-018), which is exactly why callers look
// workflows up by it instead of by name.
const workflowIDFieldIndex = "status.workflowId"

// syncTimeout bounds how long NewInformerCache waits for the initial
// informer List+Watch sync before failing fast. Mirrors Gateway's
// buildGatewayCache (pkg/gateway/server_constructors.go), the established
// pattern for a standalone controller-runtime cache outside a full Manager.
const syncTimeout = 30 * time.Second

// Cache is a read-only, informer-backed view of RemediationWorkflow and
// ActionType CRDs. All reads are served from the local cache; KA issues
// zero etcd round-trips per discovery query.
type Cache struct {
	reader client.Reader
}

// NewScheme builds the minimal runtime.Scheme this cache needs: just the
// RemediationWorkflow and ActionType CRD types. Mirrors Gateway's
// buildGatewayScheme (pkg/gateway/server_constructors.go) -- callers (e.g.
// cmd/kubernautagent/bootstrap.go) build the scheme once and pass it to
// NewInformerCache, rather than this package reaching for the shared
// client-go scheme.Scheme global.
func NewScheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	if err := rwv1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to register RemediationWorkflow scheme: %w", err)
	}
	if err := atv1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to register ActionType scheme: %w", err)
	}
	return scheme, nil
}

// NewInformerCache builds and starts a controller-runtime cache watching
// RemediationWorkflow and ActionType CRDs cluster-wide, blocking until the
// initial sync completes (bounded by syncTimeout). The returned
// context.CancelFunc stops the underlying informers and MUST be called
// during graceful shutdown.
func NewInformerCache(kubeConfig *rest.Config, scheme *runtime.Scheme, logger logr.Logger) (*Cache, context.CancelFunc, error) {
	// A static Mapper is supplied so cache.New never performs live cluster
	// discovery (GET /api, /apis -- ServerGroups) to resolve these 2 known
	// GVKs. Without it, controller-runtime lazily builds a dynamic
	// DiscoveryRESTMapper and resolves it on the *first* IndexField/
	// GetInformer call below -- a single unretried discovery request that,
	// if it lands during a cluster's brief post-boot auth/aggregation-layer
	// warmup window, fails the entire cache construction outright (observed
	// in CI: "failed to get server groups: the server has asked for the
	// client to provide credentials"), well before the informer's own
	// resilient List+Watch retry loop (started below) ever gets a chance to
	// run. Bypassing discovery for these 2 fixed, always-present CRD types
	// removes that unprotected single point of failure entirely -- add any
	// newly-watched type's mapping here too.
	k8sCache, err := cache.New(kubeConfig, cache.Options{Scheme: scheme, Mapper: staticRESTMapper()})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create workflow catalog cache: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	if err := indexWorkflowCacheFields(ctx, k8sCache); err != nil {
		cancel()
		return nil, nil, err
	}

	watchedKinds := []client.Object{&rwv1alpha1.RemediationWorkflow{}, &atv1alpha1.ActionType{}}
	for _, obj := range watchedKinds {
		if _, err := k8sCache.GetInformer(ctx, obj); err != nil {
			cancel()
			return nil, nil, fmt.Errorf("failed to start informer for %T: %w", obj, err)
		}
	}

	go func() {
		if err := k8sCache.Start(ctx); err != nil {
			logger.Error(err, "workflow catalog cache stopped unexpectedly")
		}
	}()

	syncCtx, syncCancel := context.WithTimeout(ctx, syncTimeout)
	defer syncCancel()
	if !k8sCache.WaitForCacheSync(syncCtx) {
		cancel()
		return nil, nil, fmt.Errorf("failed to sync workflow catalog cache within %s", syncTimeout)
	}

	return &Cache{reader: k8sCache}, cancel, nil
}

// staticRESTMapper builds a fixed GVK<->GVR mapping for the exact 2 CRD
// types this cache ever watches (RemediationWorkflow, ActionType), both
// namespace-scoped (config/crd/bases/kubernaut.ai_{remediationworkflows,
// actiontypes}.yaml). Passed as cache.Options.Mapper to NewInformerCache so
// controller-runtime never falls back to its default dynamic,
// discovery-backed mapper -- see the doc comment on that call site for why.
func staticRESTMapper() meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{rwv1alpha1.GroupVersion, atv1alpha1.GroupVersion})
	mapper.AddSpecific(
		rwv1alpha1.GroupVersion.WithKind("RemediationWorkflow"),
		rwv1alpha1.GroupVersion.WithResource("remediationworkflows"),
		rwv1alpha1.GroupVersion.WithResource("remediationworkflow"),
		meta.RESTScopeNamespace,
	)
	mapper.AddSpecific(
		atv1alpha1.GroupVersion.WithKind("ActionType"),
		atv1alpha1.GroupVersion.WithResource("actiontypes"),
		atv1alpha1.GroupVersion.WithResource("actiontype"),
		meta.RESTScopeNamespace,
	)
	return mapper
}

// NewCacheFromReader builds a Cache backed by an arbitrary client.Reader --
// for tests that need to exercise real Catalog/Cache logic (List/GetByID/
// filtering/conversion) against a controller-runtime fake client, without
// paying for a full informer sync or envtest. Production code must use
// NewInformerCache; this constructor exists solely as a testing seam.
func NewCacheFromReader(reader client.Reader) *Cache {
	return &Cache{reader: reader}
}

// indexWorkflowCacheFields registers the field indexers NewInformerCache
// needs (actionType/name lookups). Extracted for readability -- one place
// to see every index this cache maintains.
func indexWorkflowCacheFields(ctx context.Context, k8sCache cache.Cache) error {
	if err := k8sCache.IndexField(ctx, &rwv1alpha1.RemediationWorkflow{}, actionTypeFieldIndex,
		func(obj client.Object) []string {
			rw, ok := obj.(*rwv1alpha1.RemediationWorkflow)
			if !ok || rw.Spec.ActionType == "" {
				return nil
			}
			return []string{rw.Spec.ActionType}
		}); err != nil {
		return fmt.Errorf("failed to index RemediationWorkflow by %s: %w", actionTypeFieldIndex, err)
	}

	if err := k8sCache.IndexField(ctx, &rwv1alpha1.RemediationWorkflow{}, workflowNameFieldIndex,
		func(obj client.Object) []string {
			return []string{obj.GetName()}
		}); err != nil {
		return fmt.Errorf("failed to index RemediationWorkflow by %s: %w", workflowNameFieldIndex, err)
	}

	if err := k8sCache.IndexField(ctx, &rwv1alpha1.RemediationWorkflow{}, workflowIDFieldIndex,
		func(obj client.Object) []string {
			rw, ok := obj.(*rwv1alpha1.RemediationWorkflow)
			if !ok || rw.Status.WorkflowID == "" {
				return nil
			}
			return []string{rw.Status.WorkflowID}
		}); err != nil {
		return fmt.Errorf("failed to index RemediationWorkflow by %s: %w", workflowIDFieldIndex, err)
	}

	if err := k8sCache.IndexField(ctx, &atv1alpha1.ActionType{}, actionTypeNameFieldIndex,
		func(obj client.Object) []string {
			at, ok := obj.(*atv1alpha1.ActionType)
			if !ok || at.Spec.Name == "" {
				return nil
			}
			return []string{at.Spec.Name}
		}); err != nil {
		return fmt.Errorf("failed to index ActionType by %s: %w", actionTypeNameFieldIndex, err)
	}

	return nil
}

// GetWorkflow returns the RemediationWorkflow CRD named name, or (nil, nil)
// if it does not exist -- matching the retired Postgres repository's
// not-found convention. Lookup is cluster-wide by metadata.name: workflow
// names are globally unique across the catalog.
func (c *Cache) GetWorkflow(ctx context.Context, name string) (*rwv1alpha1.RemediationWorkflow, error) {
	var list rwv1alpha1.RemediationWorkflowList
	if err := c.reader.List(ctx, &list, client.MatchingFields{workflowNameFieldIndex: name}); err != nil {
		return nil, fmt.Errorf("failed to get RemediationWorkflow %s: %w", name, err)
	}
	if len(list.Items) == 0 {
		return nil, nil //nolint:nilnil // intentional "not found, no error" cache-layer contract, see doc comment above
	}
	return &list.Items[0], nil
}

// GetWorkflowByID returns the RemediationWorkflow CRD whose status.workflowId
// matches workflowID, or (nil, nil) if none exists -- matching the retired
// Postgres repository's not-found convention. Lookup is cluster-wide by the
// content-hash identity (DD-WORKFLOW-018), not by name: this is Step 3 of
// the discovery protocol.
func (c *Cache) GetWorkflowByID(ctx context.Context, workflowID string) (*rwv1alpha1.RemediationWorkflow, error) {
	var list rwv1alpha1.RemediationWorkflowList
	if err := c.reader.List(ctx, &list, client.MatchingFields{workflowIDFieldIndex: workflowID}); err != nil {
		return nil, fmt.Errorf("failed to get RemediationWorkflow by workflow_id %s: %w", workflowID, err)
	}
	if len(list.Items) == 0 {
		return nil, nil //nolint:nilnil // intentional "not found, no error" cache-layer contract, see doc comment above
	}
	return &list.Items[0], nil
}

// ListWorkflowsByActionType returns every RemediationWorkflow CRD whose
// spec.actionType matches actionType, using the indexed field lookup instead
// of a full scan.
func (c *Cache) ListWorkflowsByActionType(ctx context.Context, actionType string) ([]rwv1alpha1.RemediationWorkflow, error) {
	var list rwv1alpha1.RemediationWorkflowList
	if err := c.reader.List(ctx, &list, client.MatchingFields{actionTypeFieldIndex: actionType}); err != nil {
		return nil, fmt.Errorf("failed to list RemediationWorkflows for action type %s: %w", actionType, err)
	}
	return list.Items, nil
}

// ListWorkflows returns every RemediationWorkflow CRD across all namespaces,
// unfiltered -- the cache-backed source for the full-catalog listing
// (Phase 2b's list_workflows MCP tool).
func (c *Cache) ListWorkflows(ctx context.Context) ([]rwv1alpha1.RemediationWorkflow, error) {
	var list rwv1alpha1.RemediationWorkflowList
	if err := c.reader.List(ctx, &list); err != nil {
		return nil, fmt.Errorf("failed to list RemediationWorkflows: %w", err)
	}
	return list.Items, nil
}

// GetActionType returns the ActionType CRD whose spec.name equals name, or
// (nil, nil) if none exists. Lookup is keyed by the business identifier
// (spec.name), not metadata.name -- mirrors AuthWebhook's existing
// findActionTypeKey convention (pkg/authwebhook/rw_reconciler.go).
func (c *Cache) GetActionType(ctx context.Context, name string) (*atv1alpha1.ActionType, error) {
	var list atv1alpha1.ActionTypeList
	if err := c.reader.List(ctx, &list, client.MatchingFields{actionTypeNameFieldIndex: name}); err != nil {
		return nil, fmt.Errorf("failed to get ActionType %s: %w", name, err)
	}
	if len(list.Items) == 0 {
		return nil, nil //nolint:nilnil // intentional "not found, no error" cache-layer contract, see doc comment above
	}
	return &list.Items[0], nil
}

// ListActionTypes returns every ActionType CRD across all namespaces.
func (c *Cache) ListActionTypes(ctx context.Context) ([]atv1alpha1.ActionType, error) {
	var list atv1alpha1.ActionTypeList
	if err := c.reader.List(ctx, &list); err != nil {
		return nil, fmt.Errorf("failed to list ActionTypes: %w", err)
	}
	return list.Items, nil
}
