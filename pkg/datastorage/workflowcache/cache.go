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

// Package workflowcache provides a read-only, informer-backed view of
// RemediationWorkflow and ActionType CRDs for DataStorage.
//
// DD-WORKFLOW-018 (Issue #1661): etcd is the sole source of truth for
// workflow/action-type definitions. DataStorage no longer maintains a
// Postgres catalog for these -- it watches etcd via this cache instead.
// DataStorage never mutates RemediationWorkflow/ActionType CRDs; AuthWebhook
// is the sole write path (ADR-058, ADR-059).
package workflowcache

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	atv1alpha1 "github.com/jordigilh/kubernaut/api/actiontype/v1alpha1"
	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
)

// actionTypeFieldIndex indexes RemediationWorkflow CRDs by spec.actionType,
// enabling ListWorkflowsByActionType to do an indexed lookup instead of a
// full scan (Change 6, Phase 31-33 will be the primary consumer).
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

// syncTimeout bounds how long NewInformerCache waits for the initial
// informer List+Watch sync before failing fast. Mirrors Gateway's
// buildGatewayCache (pkg/gateway/server_constructors.go), the established
// pattern for a standalone controller-runtime cache outside a full Manager.
const syncTimeout = 30 * time.Second

// Cache is a read-only, informer-backed view of RemediationWorkflow and
// ActionType CRDs. All reads are served from the local cache; DataStorage
// issues zero etcd round-trips per discovery query.
type Cache struct {
	reader client.Reader
}

// NewScheme builds the minimal runtime.Scheme this cache needs: just the
// RemediationWorkflow and ActionType CRD types. Mirrors Gateway's
// buildGatewayScheme (pkg/gateway/server_constructors.go) -- callers (e.g.
// cmd/datastorage/main.go) build the scheme once and pass it to
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
	k8sCache, err := cache.New(kubeConfig, cache.Options{Scheme: scheme})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create workflow cache: %w", err)
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
			logger.Error(err, "workflow cache stopped unexpectedly")
		}
	}()

	syncCtx, syncCancel := context.WithTimeout(ctx, syncTimeout)
	defer syncCancel()
	if !k8sCache.WaitForCacheSync(syncCtx) {
		cancel()
		return nil, nil, fmt.Errorf("failed to sync workflow cache within %s", syncTimeout)
	}

	return &Cache{reader: k8sCache}, cancel, nil
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
// not-found convention (pkg/datastorage/repository/workflow/crud.go GetByID).
// Lookup is cluster-wide by metadata.name: workflow names are globally
// unique across the catalog.
func (c *Cache) GetWorkflow(ctx context.Context, name string) (*rwv1alpha1.RemediationWorkflow, error) {
	var list rwv1alpha1.RemediationWorkflowList
	if err := c.reader.List(ctx, &list, client.MatchingFields{workflowNameFieldIndex: name}); err != nil {
		return nil, fmt.Errorf("failed to get RemediationWorkflow %s: %w", name, err)
	}
	if len(list.Items) == 0 {
		return nil, nil
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
		return nil, nil
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
