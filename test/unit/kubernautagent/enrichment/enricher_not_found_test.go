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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var _ = Describe("Enricher NotFound Handling — Issue #1039", func() {
	var (
		logger     = logr.Discard()
		auditStore *recordingAuditStore
		ds         *fakeDataStorageClient
		ctx        context.Context
	)

	BeforeEach(func() {
		auditStore = &recordingAuditStore{}
		ds = &fakeDataStorageClient{history: &enrichment.RemediationHistoryResult{}}
		ctx = context.Background()
	})

	retryConfig := enrichment.RetryConfig{
		MaxRetries:  3,
		BaseBackoff: 1 * time.Millisecond,
	}

	Describe("UT-KA-1039-001: NotFound does not trigger HardFail", func() {
		It("should set HardFail=false when GetOwnerChain returns NotFound", func() {
			notFoundErr := apierrors.NewNotFound(
				schema.GroupResource{Resource: "pods"}, "deleted-pod")
			k8s := &fakeK8sClient{err: notFoundErr}

			e := enrichment.NewEnricher(k8s, ds, auditStore, logger).
				WithRetryConfig(retryConfig)

			result, err := e.Enrich(ctx, "Pod", "deleted-pod", "production", "", "inc-1039-001")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(result.HardFail).To(BeFalse(),
				"UT-KA-1039-001: NotFound must NOT trigger HardFail")
			Expect(result.OwnerChainError).NotTo(BeNil(),
				"UT-KA-1039-001: OwnerChainError must still be recorded for observability")
		})
	})

	Describe("UT-KA-1039-002: NotFound sets TargetResourceDeleted", func() {
		It("should set TargetResourceDeleted=true when resource is not found", func() {
			notFoundErr := apierrors.NewNotFound(
				schema.GroupResource{Resource: "clusterserviceversions"}, "etcdoperator.v0.9.4")
			k8s := &fakeK8sClient{err: notFoundErr}

			e := enrichment.NewEnricher(k8s, ds, auditStore, logger).
				WithRetryConfig(retryConfig)

			result, err := e.Enrich(ctx, "ClusterServiceVersion", "etcdoperator.v0.9.4", "demo-operator", "", "inc-1039-002")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(result.TargetResourceDeleted).To(BeTrue(),
				"UT-KA-1039-002: TargetResourceDeleted must be true when resource is NotFound")
		})
	})

	Describe("UT-KA-1039-003: NotFound skips retries", func() {
		It("should make exactly 1 call (no retries) when error is NotFound", func() {
			notFoundErr := apierrors.NewNotFound(
				schema.GroupResource{Resource: "deployments"}, "deleted-deploy")
			k8s := &countingK8sClient{
				errSeq: []error{notFoundErr, notFoundErr, notFoundErr, notFoundErr},
			}

			e := enrichment.NewEnricher(k8s, ds, auditStore, logger).
				WithRetryConfig(retryConfig)

			result, err := e.Enrich(ctx, "Deployment", "deleted-deploy", "production", "", "inc-1039-003")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(k8s.CallCount()).To(Equal(1),
				"UT-KA-1039-003: NotFound must skip retries — only 1 call expected")
		})
	})

	Describe("UT-KA-1039-004: Non-NotFound errors still trigger HardFail", func() {
		It("should set HardFail=true for InternalError (existing behavior)", func() {
			internalErr := apierrors.NewInternalError(fmt.Errorf("etcd timeout"))
			k8s := &countingK8sClient{
				errSeq: []error{internalErr, internalErr, internalErr, internalErr},
			}

			e := enrichment.NewEnricher(k8s, ds, auditStore, logger).
				WithRetryConfig(retryConfig)

			result, err := e.Enrich(ctx, "Pod", "failing-pod", "production", "", "inc-1039-004")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(result.HardFail).To(BeTrue(),
				"UT-KA-1039-004: InternalError must still trigger HardFail")
			Expect(result.TargetResourceDeleted).To(BeFalse(),
				"UT-KA-1039-004: TargetResourceDeleted must be false for non-NotFound errors")
		})
	})

	Describe("UT-KA-1039-005: NoMatchError still exempt from HardFail", func() {
		It("should set HardFail=false for NoMatchError (existing behavior preserved)", func() {
			noMatchErr := &meta.NoResourceMatchError{
				PartialResource: schema.GroupVersionResource{
					Group: "custom.example.com", Version: "v1", Resource: "widgets",
				},
			}
			k8s := &fakeK8sClient{err: noMatchErr}

			e := enrichment.NewEnricher(k8s, ds, auditStore, logger).
				WithRetryConfig(retryConfig)

			result, err := e.Enrich(ctx, "Widget", "test-widget", "default", "", "inc-1039-005")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(result.HardFail).To(BeFalse(),
				"UT-KA-1039-005: NoMatchError must NOT trigger HardFail (existing behavior)")
			Expect(result.TargetResourceDeleted).To(BeFalse(),
				"UT-KA-1039-005: TargetResourceDeleted must be false for NoMatchError")
		})
	})

	Describe("UT-KA-1039-006: IsNotFoundError detects wrapped NotFound", func() {
		It("should return true for NotFound wrapped by K8sAdapter", func() {
			inner := apierrors.NewNotFound(
				schema.GroupResource{Resource: "clusterserviceversions"}, "etcdoperator.v0.9.4")
			wrapped := fmt.Errorf("k8s adapter: get ClusterServiceVersion/etcdoperator.v0.9.4 in demo-operator: %w", inner)

			Expect(enrichment.IsNotFoundError(wrapped)).To(BeTrue(),
				"UT-KA-1039-006: must detect NotFound through fmt.Errorf wrapping")
		})

		It("should return true for bare NotFound (unwrapped)", func() {
			bare := apierrors.NewNotFound(
				schema.GroupResource{Resource: "pods"}, "deleted-pod")

			Expect(enrichment.IsNotFoundError(bare)).To(BeTrue(),
				"UT-KA-1039-006: must detect bare NotFound")
		})
	})

	Describe("UT-KA-1039-007: IsNotFoundError rejects non-NotFound errors", func() {
		It("should return false for Forbidden error", func() {
			forbidden := apierrors.NewForbidden(
				schema.GroupResource{Resource: "secrets"}, "my-secret",
				fmt.Errorf("RBAC denied"))

			Expect(enrichment.IsNotFoundError(forbidden)).To(BeFalse(),
				"UT-KA-1039-007: Forbidden must not be detected as NotFound")
		})

		It("should return false for InternalError", func() {
			internal := apierrors.NewInternalError(fmt.Errorf("etcd timeout"))

			Expect(enrichment.IsNotFoundError(internal)).To(BeFalse(),
				"UT-KA-1039-007: InternalError must not be detected as NotFound")
		})

		It("should return false for generic (non-API) error", func() {
			generic := fmt.Errorf("network unreachable")

			Expect(enrichment.IsNotFoundError(generic)).To(BeFalse(),
				"UT-KA-1039-007: generic error must not be detected as NotFound")
		})

		It("should return false for nil error", func() {
			Expect(enrichment.IsNotFoundError(nil)).To(BeFalse(),
				"UT-KA-1039-007: nil must not be detected as NotFound")
		})
	})
})
