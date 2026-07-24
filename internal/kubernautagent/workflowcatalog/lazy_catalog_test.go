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

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// fakeClientBuilder returns a controller-runtime fake client builder
// pre-wired with this package's scheme, so LazyCatalog tests can produce a
// real (empty) *Cache without an envtest/informer round-trip -- LazyCatalog's
// own responsibility (retry/Ready state machine) is orthogonal to Cache's
// cache-backed read logic, which is covered elsewhere (discovery_cache_test.go,
// list_cache_test.go, and the Phase 2a integration test).
func fakeClientBuilder() *fake.ClientBuilder {
	scheme, err := NewScheme()
	Expect(err).NotTo(HaveOccurred())
	return fake.NewClientBuilder().WithScheme(scheme)
}

// ============================================================================
// LazyCatalog -- #1677 hardening (DD-WORKFLOW-019).
//
// KA always runs in-cluster (no supported dev-mode-without-K8s): a missing
// or failed workflow catalog cache is a genuine startup fault, not an
// acceptable degraded steady state. LazyCatalog replaces the former
// "wfCatalog may be nil, feature silently disabled" design with an
// always-non-nil, eventually-consistent wrapper that starts Not-Ready and
// becomes Ready once its background retry loop completes a successful cache
// sync. readinessHandler gates /readyz on Ready() so the pod is kept out of
// service until the cache genuinely syncs.
// ============================================================================

var _ = Describe("LazyCatalog", func() {
	It("UT-KA-1677-LAZY-001: starts Not-Ready before Start is called", func() {
		lazy := NewLazyCatalog(logr.Discard())
		Expect(lazy.Ready()).To(BeFalse())
	})

	It("UT-KA-1677-LAZY-002: every read method returns ErrCatalogNotReady before the first successful build", func() {
		lazy := NewLazyCatalog(logr.Discard())

		_, err := lazy.GetByID(context.Background(), "wf-1")
		Expect(err).To(MatchError(ErrCatalogNotReady))

		_, _, err = lazy.List(context.Background(), nil, -1, 0)
		Expect(err).To(MatchError(ErrCatalogNotReady))

		_, _, err = lazy.ListActions(context.Background(), nil, 0, 10)
		Expect(err).To(MatchError(ErrCatalogNotReady))

		_, _, err = lazy.ListWorkflowsByActionType(context.Background(), "RestartPod", nil, 0, 10)
		Expect(err).To(MatchError(ErrCatalogNotReady))

		_, err = lazy.GetWorkflowWithContextFilters(context.Background(), "wf-1", nil)
		Expect(err).To(MatchError(ErrCatalogNotReady))
	})

	It("UT-KA-1677-LAZY-003: becomes Ready and delegates reads once the builder succeeds", func() {
		lazy := NewLazyCatalog(logr.Discard())
		fakeCache := NewCacheFromReader(fakeClientBuilder().Build())

		lazy.Start(func() (*Cache, context.CancelFunc, error) {
			return fakeCache, func() {}, nil
		})

		Eventually(lazy.Ready).Should(BeTrue())

		// List against an empty fake client returns (nil/empty, 0, nil) --
		// proves delegation reached the real Catalog rather than short-circuiting.
		workflows, total, err := lazy.List(context.Background(), nil, -1, 0)
		Expect(err).NotTo(HaveOccurred())
		Expect(workflows).To(BeEmpty())
		Expect(total).To(Equal(0))
	})

	It("UT-KA-1677-LAZY-004: retries with backoff until the builder succeeds, then becomes Ready", func() {
		lazy := NewLazyCatalog(logr.Discard())
		// Speed up the test: shrink the package-level backoff bounds for the
		// duration of this spec only.
		origInitial, origMax := initialRetryBackoff, maxRetryBackoff
		initialRetryBackoff = 0
		maxRetryBackoff = 0
		DeferCleanup(func() { initialRetryBackoff, maxRetryBackoff = origInitial, origMax })

		var attempts atomic.Int32
		fakeCache := NewCacheFromReader(fakeClientBuilder().Build())

		lazy.Start(func() (*Cache, context.CancelFunc, error) {
			n := attempts.Add(1)
			if n < 3 {
				return nil, nil, errors.New("simulated transient failure")
			}
			return fakeCache, func() {}, nil
		})

		Eventually(lazy.Ready).Should(BeTrue())
		Expect(attempts.Load()).To(BeNumerically(">=", 3))
	})

	It("UT-KA-1677-LAZY-005: retry loop stops (never becomes Ready) once its context is cancelled via Stop", func() {
		lazy := NewLazyCatalog(logr.Discard())
		origInitial, origMax := initialRetryBackoff, maxRetryBackoff
		initialRetryBackoff = 0
		maxRetryBackoff = 0
		DeferCleanup(func() { initialRetryBackoff, maxRetryBackoff = origInitial, origMax })

		var attempts atomic.Int32
		lazy.Start(func() (*Cache, context.CancelFunc, error) {
			attempts.Add(1)
			return nil, nil, errors.New("permanent failure")
		})

		Eventually(attempts.Load).Should(BeNumerically(">=", 1))
		lazy.Stop()

		Consistently(lazy.Ready).Should(BeFalse())
	})

	It("UT-KA-1677-LAZY-006: Stop is a no-op safe to call before Start / when never Ready", func() {
		lazy := NewLazyCatalog(logr.Discard())
		Expect(lazy.Stop).NotTo(Panic())
	})

	It("UT-KA-1677-LAZY-007: NewLazyCatalogReady is immediately Ready and delegates to the wrapped cache", func() {
		fakeCache := NewCacheFromReader(fakeClientBuilder().Build())
		lazy := NewLazyCatalogReady(fakeCache, logr.Discard())

		Expect(lazy.Ready()).To(BeTrue())
		workflows, total, err := lazy.List(context.Background(), nil, -1, 0)
		Expect(err).NotTo(HaveOccurred())
		Expect(workflows).To(BeEmpty())
		Expect(total).To(Equal(0))
	})
})
