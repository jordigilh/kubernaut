/*
Copyright 2025 Jordi Gil.

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

package routing

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/routing"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

// ========================================
// RO SCOPE BLOCKING UNIT TESTS
// ========================================
//
// Business Requirements:
//   - BR-SCOPE-010: RO Scope Blocking
//   - ADR-053: Resource Scope Management Architecture
//
// Test Plan: docs/services/crd-controllers/05-remediationorchestrator/RO_SCOPE_VALIDATION_TEST_PLAN_V1.0.md
//
// Test IDs: UT-RO-010-001 through UT-RO-010-009

var _ = Describe("BR-SCOPE-010: RO Scope Blocking", func() {
	var (
		ctx    context.Context
		config routing.Config
	)

	BeforeEach(func() {
		ctx = context.Background()
		config = routing.Config{
			ConsecutiveFailureThreshold: 3,
			ConsecutiveFailureCooldown:  3600, // 1 hour
			RecentlyRemediatedCooldown:  300,  // 5 minutes
			ExponentialBackoffBase:      60,   // 1 minute
			ExponentialBackoffMax:       600,  // 10 minutes
			ExponentialBackoffMaxExponent: 4,
			ScopeBackoffBase:            5,    // 5 seconds (ADR-053)
			ScopeBackoffMax:             300,  // 5 minutes (ADR-053)
		}
	})

	// makeRR creates a test RemediationRequest with the given target resource
	makeRR := func(namespace, name, kind, targetName string) *remediationv1.RemediationRequest {
		return &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      name,
				UID:       types.UID(fmt.Sprintf("uid-%s", name)),
			},
			Spec: remediationv1.RemediationRequestSpec{
				TargetResource: remediationv1.ResourceIdentifier{
					Kind: kind,
					Name: targetName,
				},
				SignalFingerprint: "fp-scope-test",
			},
		}
	}

	// makeFakeClient creates a minimal fake client for scope tests
	makeFakeClient := func() *fake.ClientBuilder {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())
		return fake.NewClientBuilder().WithScheme(scheme)
	}

	// ─────────────────────────────────────────────
	// CheckUnmanagedResource Tests
	// ─────────────────────────────────────────────

	Context("CheckUnmanagedResource", func() {

		// UT-RO-010-001: Block unmanaged resource
		It("UT-RO-010-001: should block when resource is unmanaged", func() {
			fakeClient := makeFakeClient().Build()
			engine := routing.NewRoutingEngine(fakeClient, fakeClient, "default", config, &mocks.NeverManagedScopeChecker{})
			rr := makeRR("test-ns", "test-rr", "Deployment", "payment-api")

			blocked := engine.CheckUnmanagedResource(ctx, rr)
			Expect(blocked.Reason).To(Equal(string(remediationv1.BlockReasonUnmanagedResource)))
			Expect(blocked.Blocked).To(BeTrue())
			// Backoff should be >= 4s (5s base with -10% jitter)
			Expect(blocked.RequeueAfter).To(BeNumerically(">=", 4*time.Second))
			Expect(blocked.RequeueAfter).To(BeNumerically("<=", 6*time.Second))
		})

		// UT-RO-010-002: Pass managed resource
		It("UT-RO-010-002: should pass when resource is managed", func() {
			fakeClient := makeFakeClient().Build()
			engine := routing.NewRoutingEngine(fakeClient, fakeClient, "default", config, &mocks.AlwaysManagedScopeChecker{})
			rr := makeRR("test-ns", "test-rr", "Deployment", "payment-api")

			blocked := engine.CheckUnmanagedResource(ctx, rr)
			Expect(blocked).To(BeNil(), "Managed resource should not be blocked")
		})

		// UT-RO-010-003: Check #1 Priority — scope evaluated before all other checks
		It("UT-RO-010-003: should evaluate scope before all other checks", func() {
			fakeClient := makeFakeClient().Build()
			// Use NeverManagedScopeChecker — scope should reject before ConsecutiveFailures
			engine := routing.NewRoutingEngine(fakeClient, fakeClient, "default", config, &mocks.NeverManagedScopeChecker{})
			rr := makeRR("test-ns", "test-rr", "Deployment", "payment-api")
			// Set high consecutive failures that would trigger ConsecutiveFailures check
			rr.Status.ConsecutiveFailureCount = 100

			blocked, err := engine.CheckPostAnalysisConditions(ctx, rr, "wf-001", "test-ns/deployment/payment-api")
			Expect(err).ToNot(HaveOccurred())
			// MUST be UnmanagedResource, NOT ConsecutiveFailures — proving scope is Check #1
			Expect(blocked.Reason).To(Equal(string(remediationv1.BlockReasonUnmanagedResource)))
		})

		// UT-RO-010-004: Backoff increases with retry count
		It("UT-RO-010-004: backoff should increase with retry count", func() {
			fakeClient := makeFakeClient().Build()
			engine := routing.NewRoutingEngine(fakeClient, fakeClient, "default", config, &mocks.NeverManagedScopeChecker{})

			// Test escalating backoff: 5s, 10s, 20s, 40s, 80s, 160s, 300s (cap)
			var previousBackoff time.Duration
			for _, retryCount := range []int32{0, 1, 2, 3, 4, 5, 6, 7} {
				rr := makeRR("test-ns", fmt.Sprintf("rr-%d", retryCount), "Deployment", "app")
				rr.Status.ConsecutiveFailureCount = retryCount

				blocked := engine.CheckUnmanagedResource(ctx, rr)
				Expect(blocked.Reason).To(Equal(string(remediationv1.BlockReasonUnmanagedResource)))

				if retryCount > 0 && previousBackoff < 300*time.Second {
					// Each step should be >= previous (with jitter tolerance)
					Expect(blocked.RequeueAfter).To(BeNumerically(">=", previousBackoff*80/100),
						"Backoff should increase: retry %d got %s, previous %s",
						retryCount, blocked.RequeueAfter, previousBackoff)
				}
				previousBackoff = blocked.RequeueAfter
			}

			// Final backoff should be capped at ~300s (with jitter)
			Expect(previousBackoff).To(BeNumerically("<=", 330*time.Second),
				"Backoff should be capped at 300s (5 min)")
		})

		// UT-RO-010-005: Scope infra error passes through
		It("UT-RO-010-005: should pass through when scope checker returns error", func() {
			fakeClient := makeFakeClient().Build()
			engine := routing.NewRoutingEngine(fakeClient, fakeClient, "default", config,
				&mocks.ErrorScopeChecker{Err: fmt.Errorf("cache not ready")})
			rr := makeRR("test-ns", "test-rr", "Deployment", "payment-api")

			blocked := engine.CheckUnmanagedResource(ctx, rr)
			Expect(blocked).To(BeNil(), "Scope infra error should not block — graceful degradation")
		})

		// UT-RO-010-006: NewRoutingEngine panics on nil ScopeChecker
		It("UT-RO-010-006: should panic when ScopeChecker is nil", func() {
			fakeClient := makeFakeClient().Build()
			Expect(func() {
				routing.NewRoutingEngine(fakeClient, fakeClient, "default", config, nil)
			}).To(Panic())
		})

		// UT-RO-010-007: BlockingCondition has correct reason and message
		It("UT-RO-010-007: should produce correct blocking condition fields", func() {
			fakeClient := makeFakeClient().Build()
			engine := routing.NewRoutingEngine(fakeClient, fakeClient, "default", config, &mocks.NeverManagedScopeChecker{})
			rr := makeRR("production", "rr-prod", "Deployment", "payment-api")

			blocked := engine.CheckUnmanagedResource(ctx, rr)
			Expect(blocked.Reason).To(Equal("UnmanagedResource"))
			Expect(blocked.Message).To(ContainSubstring("production/Deployment/payment-api"))
			Expect(blocked.Message).To(ContainSubstring("kubernaut.ai/managed=true"))
		})

		// UT-RO-010-008: BlockingCondition has BlockedUntil set
		It("UT-RO-010-008: should set BlockedUntil in blocking condition", func() {
			fakeClient := makeFakeClient().Build()
			engine := routing.NewRoutingEngine(fakeClient, fakeClient, "default", config, &mocks.NeverManagedScopeChecker{})
			rr := makeRR("test-ns", "test-rr", "Deployment", "payment-api")

			blocked := engine.CheckUnmanagedResource(ctx, rr)
			// BlockedUntil should be in the future
			Expect(blocked.BlockedUntil.After(time.Now().Add(-1 * time.Second))).To(BeTrue())
		})

		// UT-RO-010-009: Backoff uses default config when not specified
		It("UT-RO-010-009: should use default backoff when config not specified", func() {
			fakeClient := makeFakeClient().Build()
			// Config with zero scope backoff values — should use defaults (5s/300s)
			zeroConfig := routing.Config{
				ConsecutiveFailureThreshold: 3,
				ConsecutiveFailureCooldown:  3600,
				// ScopeBackoffBase and ScopeBackoffMax left as 0 — defaults should apply
			}
			engine := routing.NewRoutingEngine(fakeClient, fakeClient, "default", zeroConfig, &mocks.NeverManagedScopeChecker{})
			rr := makeRR("test-ns", "test-rr", "Deployment", "app")

			blocked := engine.CheckUnmanagedResource(ctx, rr)
			// Default base is 5s, so with 10% jitter: 4.5s - 5.5s
			Expect(blocked.RequeueAfter).To(BeNumerically(">=", 4*time.Second))
			Expect(blocked.RequeueAfter).To(BeNumerically("<=", 6*time.Second))
		})
	})
})
