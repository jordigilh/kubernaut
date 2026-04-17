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

package enrichment_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// countingK8sClient tracks call counts and returns per-call errors/chains.
type countingK8sClient struct {
	calls    int32
	errSeq   []error
	chainSeq [][]enrichment.OwnerChainEntry
}

func (c *countingK8sClient) GetOwnerChain(_ context.Context, _, _, _ string) ([]enrichment.OwnerChainEntry, error) {
	idx := int(atomic.AddInt32(&c.calls, 1) - 1)
	var err error
	if idx < len(c.errSeq) {
		err = c.errSeq[idx]
	} else if len(c.errSeq) > 0 {
		err = c.errSeq[len(c.errSeq)-1]
	}
	if err != nil {
		return nil, err
	}
	if idx < len(c.chainSeq) {
		return c.chainSeq[idx], nil
	}
	return nil, nil
}

func (c *countingK8sClient) GetSpecHash(_ context.Context, _, _, _ string) (string, error) {
	return "", nil
}

func (c *countingK8sClient) CallCount() int {
	return int(atomic.LoadInt32(&c.calls))
}

var _ = Describe("Enricher Retry Infrastructure — BR-HAPI-261/264 #704", func() {
	var (
		logger     *slog.Logger
		auditStore *recordingAuditStore
		ds         *fakeDataStorageClient
		ctx        context.Context
	)

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
		auditStore = &recordingAuditStore{}
		ds = &fakeDataStorageClient{history: &enrichment.RemediationHistoryResult{}}
		ctx = context.Background()
	})

	Describe("UT-704-E-001: Transient error retried, HardFail after exhaustion", func() {
		It("should retry 3 times on transient error and set HardFail=true", func() {
			transientErr := apierrors.NewInternalError(fmt.Errorf("etcd timeout"))
			k8s := &countingK8sClient{
				errSeq: []error{transientErr, transientErr, transientErr, transientErr},
			}
			e := enrichment.NewEnricher(k8s, ds, auditStore, logger).
				WithRetryConfig(enrichment.RetryConfig{
					MaxRetries:  3,
					BaseBackoff: 1 * time.Millisecond,
				})

			result, err := e.Enrich(ctx, "Pod", "test-pod", "production", "", "inc-001")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(k8s.CallCount()).To(Equal(4),
				"UT-704-E-001: initial call + 3 retries = 4 total calls")
			Expect(result.OwnerChainError).NotTo(BeNil(),
				"UT-704-E-001: OwnerChainError must be set after retry exhaustion")
			Expect(result.HardFail).To(BeTrue(),
				"UT-704-E-001: HardFail must be true after retry exhaustion")
		})
	})

	Describe("UT-704-E-002: Transient error succeeds on retry", func() {
		It("should succeed on 2nd attempt and set HardFail=false", func() {
			transientErr := apierrors.NewServiceUnavailable("api-server overloaded")
			successChain := []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "api-server", Namespace: "production"},
			}
			k8s := &countingK8sClient{
				errSeq:   []error{transientErr, nil},
				chainSeq: [][]enrichment.OwnerChainEntry{nil, successChain},
			}
			e := enrichment.NewEnricher(k8s, ds, auditStore, logger).
				WithRetryConfig(enrichment.RetryConfig{
					MaxRetries:  3,
					BaseBackoff: 1 * time.Millisecond,
				})

			result, err := e.Enrich(ctx, "Pod", "test-pod", "production", "", "inc-002")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(k8s.CallCount()).To(Equal(2),
				"UT-704-E-002: should succeed on 2nd attempt")
			Expect(result.OwnerChainError).To(BeNil(),
				"UT-704-E-002: OwnerChainError must be nil after successful retry")
			Expect(result.HardFail).To(BeFalse(),
				"UT-704-E-002: HardFail must be false when retry succeeds")
			Expect(result.OwnerChain).To(HaveLen(1),
				"UT-704-E-002: owner chain must be populated from successful retry")
		})
	})

	Describe("UT-704-E-003: NotFound retried and HardFail after exhaustion (HAPI-aligned)", func() {
		It("should retry NotFound 3 times and set HardFail=true after exhaustion", func() {
			notFoundErr := apierrors.NewNotFound(
				schema.GroupResource{Resource: "pods"}, "test-pod")
			k8s := &countingK8sClient{
				errSeq: []error{notFoundErr, notFoundErr, notFoundErr, notFoundErr},
			}
			e := enrichment.NewEnricher(k8s, ds, auditStore, logger).
				WithRetryConfig(enrichment.RetryConfig{
					MaxRetries:  3,
					BaseBackoff: 1 * time.Millisecond,
				})

			result, err := e.Enrich(ctx, "Pod", "test-pod", "production", "", "inc-003")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(k8s.CallCount()).To(Equal(4),
				"UT-704-E-003: HAPI retries all errors — initial + 3 retries = 4 calls")
			Expect(result.OwnerChainError).NotTo(BeNil(),
				"UT-704-E-003: OwnerChainError must be set after retry exhaustion")
			Expect(result.HardFail).To(BeTrue(),
				"UT-704-E-003: HardFail must be true after retry exhaustion")
		})
	})

	Describe("UT-704-E-005: Forbidden retried and HardFail after exhaustion (HAPI-aligned)", func() {
		It("should retry Forbidden 3 times and set HardFail=true after exhaustion", func() {
			forbiddenErr := apierrors.NewForbidden(
				schema.GroupResource{Resource: "pods"}, "test-pod", fmt.Errorf("RBAC: access denied"))
			k8s := &countingK8sClient{
				errSeq: []error{forbiddenErr, forbiddenErr, forbiddenErr, forbiddenErr},
			}
			e := enrichment.NewEnricher(k8s, ds, auditStore, logger).
				WithRetryConfig(enrichment.RetryConfig{
					MaxRetries:  3,
					BaseBackoff: 1 * time.Millisecond,
				})

			result, err := e.Enrich(ctx, "Pod", "test-pod", "production", "", "inc-005")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(k8s.CallCount()).To(Equal(4),
				"UT-704-E-005: HAPI retries all errors — initial + 3 retries = 4 calls")
			Expect(result.OwnerChainError).NotTo(BeNil(),
				"UT-704-E-005: OwnerChainError must be set after retry exhaustion")
			Expect(result.HardFail).To(BeTrue(),
				"UT-704-E-005: HardFail must be true after retry exhaustion (Forbidden)")
		})
	})

	Describe("UT-704-E-004: Best-effort mode (no WithRetryConfig, MaxRetries=0)", func() {
		It("should set OwnerChainError but HardFail=false when retries not configured", func() {
			transientErr := apierrors.NewInternalError(fmt.Errorf("etcd timeout"))
			k8s := &countingK8sClient{
				errSeq: []error{transientErr},
			}
			e := enrichment.NewEnricher(k8s, ds, auditStore, logger)

			result, err := e.Enrich(ctx, "Pod", "test-pod", "production", "", "inc-004")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(k8s.CallCount()).To(Equal(1),
				"UT-704-E-004: no retries with default config")
			Expect(result.OwnerChainError).NotTo(BeNil(),
				"UT-704-E-004: OwnerChainError must be set for observability")
			Expect(result.HardFail).To(BeFalse(),
				"UT-704-E-004: HardFail must be false in best-effort mode (retries=0)")
		})
	})

	Describe("UT-704-E-006: GVR-not-found (unknown Kind) should NOT trigger HardFail", func() {
		It("should NOT set HardFail when the Kind is unknown to the API server", func() {
			// Simulates cert-manager Certificate when CRD is not installed.
			// The RESTMapper returns NoResourceMatchError for unknown resource types.
			noMatchErr := fmt.Errorf("k8s adapter: resolve GVR for Certificate: %w",
				&meta.NoResourceMatchError{PartialResource: schema.GroupVersionResource{Resource: "certificates"}})
			k8s := &countingK8sClient{
				errSeq: []error{noMatchErr, noMatchErr, noMatchErr, noMatchErr},
			}
			e := enrichment.NewEnricher(k8s, ds, auditStore, logger).
				WithRetryConfig(enrichment.RetryConfig{
					MaxRetries:  3,
					BaseBackoff: 1 * time.Millisecond,
				})

			result, err := e.Enrich(ctx, "Certificate", "demo-app-cert", "default", "", "inc-006")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(result.OwnerChainError).NotTo(BeNil(),
				"UT-704-E-006: OwnerChainError should be set for observability")
			Expect(result.HardFail).To(BeFalse(),
				"UT-704-E-006: HardFail must be false — unknown Kind is a schema limitation, not an RCA failure")
		})

		It("should still HardFail when a known Kind's instance is not found", func() {
			notFoundErr := apierrors.NewNotFound(
				schema.GroupResource{Resource: "pods"}, "unreachable-pod")
			k8s := &countingK8sClient{
				errSeq: []error{notFoundErr, notFoundErr, notFoundErr, notFoundErr},
			}
			e := enrichment.NewEnricher(k8s, ds, auditStore, logger).
				WithRetryConfig(enrichment.RetryConfig{
					MaxRetries:  3,
					BaseBackoff: 1 * time.Millisecond,
				})

			result, err := e.Enrich(ctx, "Pod", "unreachable-pod", "default", "", "inc-006b")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(result.HardFail).To(BeTrue(),
				"UT-704-E-006: HardFail must be true for known Kinds — the resource is unreachable, RCA is incomplete")
		})
	})

	Describe("UT-704-E-007: IsNoMatchError classification", func() {
		It("should return true for NoResourceMatchError", func() {
			err := fmt.Errorf("wrapped: %w",
				&meta.NoResourceMatchError{PartialResource: schema.GroupVersionResource{Resource: "certificates"}})
			Expect(enrichment.IsNoMatchError(err)).To(BeTrue())
		})

		It("should return true for NoKindMatchError", func() {
			err := fmt.Errorf("wrapped: %w",
				&meta.NoKindMatchError{GroupKind: schema.GroupKind{Kind: "Certificate"}})
			Expect(enrichment.IsNoMatchError(err)).To(BeTrue())
		})

		It("should return false for NotFound API error", func() {
			err := apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, "test")
			Expect(enrichment.IsNoMatchError(err)).To(BeFalse())
		})

		It("should return false for Forbidden API error", func() {
			err := apierrors.NewForbidden(
				schema.GroupResource{Resource: "pods"}, "test", fmt.Errorf("denied"))
			Expect(enrichment.IsNoMatchError(err)).To(BeFalse())
		})
	})
})
