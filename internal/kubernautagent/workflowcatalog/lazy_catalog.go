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

package workflowcatalog

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ErrCatalogNotReady is returned by every LazyCatalog read method before its
// background retry loop completes a first successful cache sync.
var ErrCatalogNotReady = errors.New("workflow catalog cache not ready")

// CacheBuilder constructs (and starts) the informer-backed Cache a
// LazyCatalog wraps. Matches NewInformerCache's return shape; a function
// type so callers/tests can inject a fake builder without a real
// cluster/envtest.
type CacheBuilder func() (*Cache, context.CancelFunc, error)

// initialRetryBackoff/maxRetryBackoff bound LazyCatalog's background retry
// loop: capped exponential backoff (1s -> 2s -> 4s ... 30s ceiling) balances
// fast recovery from transient failures (e.g. the API-server discovery-call
// race staticRESTMapper works around, or a WaitForCacheSync timeout under
// startup contention) against not hammering the API server if the
// underlying fault is persistent. Package-level vars (not consts) so unit
// tests can shrink them for the duration of a single spec.
var (
	initialRetryBackoff = time.Second
	maxRetryBackoff     = 30 * time.Second
)

// LazyCatalog is an always-non-nil, eventually-consistent wrapper around
// Catalog. #1677 hardening (DD-WORKFLOW-019): KA always runs in-cluster --
// there is no supported dev-mode-without-K8s -- so a missing or failed
// workflow catalog cache is a genuine startup fault, not an acceptable
// degraded steady state that should be silently tolerated for the rest of
// the pod's lifetime. LazyCatalog starts Not-Ready; Start launches a
// background goroutine that retries its CacheBuilder with capped
// exponential backoff until it succeeds (or is stopped), at which point
// Ready() flips true and every read method delegates to the now-built
// Catalog. Callers (readinessHandler) gate /readyz on Ready() so the pod is
// kept out of Service endpoints until the cache genuinely syncs, instead of
// serving unvalidated/empty discovery results while quietly retrying.
type LazyCatalog struct {
	logger      logr.Logger
	ready       atomic.Bool
	catalog     atomic.Pointer[Catalog]
	loopCancel  atomic.Pointer[context.CancelFunc]
	cacheCancel atomic.Pointer[context.CancelFunc]
}

// NewLazyCatalog constructs a LazyCatalog in its initial Not-Ready state.
// Call Start to begin the background retry loop.
func NewLazyCatalog(logger logr.Logger) *LazyCatalog {
	return &LazyCatalog{logger: logger}
}

// NewLazyCatalogReady constructs a LazyCatalog that is immediately Ready,
// wrapping cache directly. A testing seam (mirrors NewCacheFromReader's
// role for Cache) for callers that need a Ready LazyCatalog without
// exercising the background retry loop; production code must use
// NewLazyCatalog + Start.
func NewLazyCatalogReady(cache *Cache, logger logr.Logger) *LazyCatalog {
	l := NewLazyCatalog(logger)
	l.catalog.Store(NewCatalog(cache, logger))
	l.ready.Store(true)
	return l
}

// Start launches the background goroutine that repeatedly invokes build
// until it succeeds or Stop is called. Call at most once per LazyCatalog.
func (l *LazyCatalog) Start(build CacheBuilder) {
	ctx, cancel := context.WithCancel(context.Background())
	l.loopCancel.Store(&cancel)
	go l.retryLoop(ctx, build)
}

func (l *LazyCatalog) retryLoop(ctx context.Context, build CacheBuilder) {
	backoff := initialRetryBackoff
	for {
		if ctx.Err() != nil {
			l.logger.Info("workflow catalog cache retry loop stopped (context cancelled)")
			return
		}

		cache, cancel, err := build()
		if err == nil {
			l.catalog.Store(NewCatalog(cache, l.logger))
			l.cacheCancel.Store(&cancel)
			l.ready.Store(true)
			l.logger.Info("workflow catalog cache ready (informer synced)")
			return
		}

		l.logger.Error(err, "workflow catalog cache construction failed, retrying in background",
			"backoff", backoff.String())
		select {
		case <-ctx.Done():
			l.logger.Info("workflow catalog cache retry loop stopped (context cancelled)")
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > maxRetryBackoff {
			backoff = maxRetryBackoff
		}
	}
}

// Ready reports whether the background retry loop has completed at least
// one successful cache sync. readinessHandler gates /readyz on this.
func (l *LazyCatalog) Ready() bool {
	return l.ready.Load()
}

// Stop cancels the background retry loop (if still retrying) and the
// underlying informer cache (if the build ever succeeded). Safe to call
// even when Start was never called or the cache never became Ready.
func (l *LazyCatalog) Stop() {
	if c := l.loopCancel.Load(); c != nil {
		(*c)()
	}
	if c := l.cacheCancel.Load(); c != nil {
		(*c)()
	}
}

// get returns the underlying Catalog, or ErrCatalogNotReady before the
// first successful build.
func (l *LazyCatalog) get() (*Catalog, error) {
	c := l.catalog.Load()
	if c == nil {
		return nil, ErrCatalogNotReady
	}
	return c, nil
}

// GetByID delegates to the underlying Catalog once Ready, or returns
// ErrCatalogNotReady beforehand.
func (l *LazyCatalog) GetByID(ctx context.Context, workflowID string) (*models.RemediationWorkflow, error) {
	c, err := l.get()
	if err != nil {
		return nil, err
	}
	return c.GetByID(ctx, workflowID)
}

// List delegates to the underlying Catalog once Ready, or returns
// ErrCatalogNotReady beforehand.
func (l *LazyCatalog) List(ctx context.Context, filters *models.WorkflowSearchFilters, limit, offset int) ([]models.RemediationWorkflow, int, error) {
	c, err := l.get()
	if err != nil {
		return nil, 0, err
	}
	return c.List(ctx, filters, limit, offset)
}

// ListActions delegates to the underlying Catalog once Ready, or returns
// ErrCatalogNotReady beforehand.
func (l *LazyCatalog) ListActions(ctx context.Context, filters *models.WorkflowDiscoveryFilters, offset, limit int) ([]models.ActionTypeEntry, int, error) {
	c, err := l.get()
	if err != nil {
		return nil, 0, err
	}
	return c.ListActions(ctx, filters, offset, limit)
}

// ListWorkflowsByActionType delegates to the underlying Catalog once Ready,
// or returns ErrCatalogNotReady beforehand.
func (l *LazyCatalog) ListWorkflowsByActionType(ctx context.Context, actionType string, filters *models.WorkflowDiscoveryFilters, offset, limit int) ([]models.RemediationWorkflow, int, error) {
	c, err := l.get()
	if err != nil {
		return nil, 0, err
	}
	return c.ListWorkflowsByActionType(ctx, actionType, filters, offset, limit)
}

// GetWorkflowWithContextFilters delegates to the underlying Catalog once
// Ready, or returns ErrCatalogNotReady beforehand.
func (l *LazyCatalog) GetWorkflowWithContextFilters(ctx context.Context, workflowID string, filters *models.WorkflowDiscoveryFilters) (*models.RemediationWorkflow, error) {
	c, err := l.get()
	if err != nil {
		return nil, err
	}
	return c.GetWorkflowWithContextFilters(ctx, workflowID, filters)
}
