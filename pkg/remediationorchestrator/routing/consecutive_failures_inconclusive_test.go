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

package routing_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/routing"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

// ============================================================================
// INCONCLUSIVE CHAIN-COUNTING TESTS (BR-ORCH-042.6, Issue #1091)
//
// Business Requirement: CheckConsecutiveFailures counts Completed+Inconclusive
// RRs as functional failures in the chain, instead of treating them as
// chain-breakers. After 3 consecutive Inconclusive outcomes, the routing
// engine blocks the signal.
// ============================================================================
var _ = Describe("CheckConsecutiveFailures - Inconclusive Chain Counting (BR-ORCH-042.6, #1091)", func() {
	var (
		ctx        context.Context
		fakeClient client.Client
		engine     *routing.RoutingEngine
		scheme     *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()

		scheme = runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())

		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithIndex(&remediationv1.RemediationRequest{}, "spec.signalFingerprint", func(obj client.Object) []string {
				rr := obj.(*remediationv1.RemediationRequest)
				if rr.Spec.SignalFingerprint == "" {
					return nil
				}
				return []string{rr.Spec.SignalFingerprint}
			}).
			WithIndex(&workflowexecutionv1.WorkflowExecution{}, "spec.targetResource", func(obj client.Object) []string {
				wfe := obj.(*workflowexecutionv1.WorkflowExecution)
				if wfe.Spec.TargetResource == "" {
					return nil
				}
				return []string{wfe.Spec.TargetResource}
			}).
			Build()

		config := routing.Config{
			ConsecutiveFailureThreshold: 3,
			ConsecutiveFailureCooldown:  3600,
			RecentlyRemediatedCooldown:  300,
			ExponentialBackoffBase:        60,
			ExponentialBackoffMax:         600,
			ExponentialBackoffMaxExponent: 4,
		}
		engine = routing.NewRoutingEngine(fakeClient, fakeClient, "default", config, &mocks.AlwaysManagedScopeChecker{})
	})

	// UT-RO-1091-007: Completed+Inconclusive counts as failure
	It("UT-RO-1091-007: should count Completed+Inconclusive RR as failure in chain", func() {
		fingerprint := "inconclusive-chain-fp"
		baseTime := time.Now().Add(-10 * time.Minute)

		// Create 2 Failed RRs + 1 Completed+Inconclusive RR = 3 total functional failures
		for i := 0; i < 2; i++ {
			failedRR := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:              fmt.Sprintf("failed-rr-%d", i),
					Namespace:         "default",
					UID:               types.UID(fmt.Sprintf("failed-uid-%d", i)),
					CreationTimestamp: metav1.Time{Time: baseTime.Add(time.Duration(i) * time.Minute)},
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: fingerprint,
				},
				Status: remediationv1.RemediationRequestStatus{
					OverallPhase: remediationv1.PhaseFailed,
				},
			}
			Expect(fakeClient.Create(ctx, failedRR)).To(Succeed())
		}

		inconclusiveRR := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "inconclusive-rr",
				Namespace:         "default",
				UID:               types.UID("inconclusive-uid"),
				CreationTimestamp: metav1.Time{Time: baseTime.Add(2 * time.Minute)},
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: fingerprint,
			},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseCompleted,
				Outcome:      "Inconclusive",
			},
		}
		Expect(fakeClient.Create(ctx, inconclusiveRR)).To(Succeed())

		incomingRR := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "incoming-rr",
				Namespace:         "default",
				UID:               types.UID("incoming-uid"),
				CreationTimestamp: metav1.Time{Time: time.Now()},
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: fingerprint,
			},
		}

		blocked := engine.CheckConsecutiveFailures(ctx, incomingRR)

		Expect(blocked).ToNot(BeNil(), "Should block: 2 Failed + 1 Inconclusive = 3 >= threshold")
		Expect(blocked.Blocked).To(BeTrue())
		Expect(blocked.Reason).To(Equal(string(remediationv1.BlockReasonConsecutiveFailures)))
	})

	// UT-RO-1091-008: Completed+Remediated still breaks chain (regression test)
	It("UT-RO-1091-008: should still break chain on Completed+Remediated", func() {
		fingerprint := "remediated-breaks-fp"
		baseTime := time.Now().Add(-10 * time.Minute)

		// Create 2 Failed RRs
		for i := 0; i < 2; i++ {
			failedRR := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:              fmt.Sprintf("failed-rr-%d", i),
					Namespace:         "default",
					UID:               types.UID(fmt.Sprintf("failed-uid-%d", i)),
					CreationTimestamp: metav1.Time{Time: baseTime.Add(time.Duration(i) * time.Minute)},
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: fingerprint,
				},
				Status: remediationv1.RemediationRequestStatus{
					OverallPhase: remediationv1.PhaseFailed,
				},
			}
			Expect(fakeClient.Create(ctx, failedRR)).To(Succeed())
		}

		// Insert a Completed+Remediated RR between the failures and more historical failures
		remediatedRR := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "remediated-rr",
				Namespace:         "default",
				UID:               types.UID("remediated-uid"),
				CreationTimestamp: metav1.Time{Time: baseTime.Add(2 * time.Minute)},
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: fingerprint,
			},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseCompleted,
				Outcome:      "Remediated",
			},
		}
		Expect(fakeClient.Create(ctx, remediatedRR)).To(Succeed())

		// Add 2 more old failures BEFORE the remediated one (should not count)
		for i := 0; i < 2; i++ {
			oldFailedRR := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:              fmt.Sprintf("old-failed-rr-%d", i),
					Namespace:         "default",
					UID:               types.UID(fmt.Sprintf("old-failed-uid-%d", i)),
					CreationTimestamp: metav1.Time{Time: baseTime.Add(time.Duration(3+i) * time.Minute)},
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: fingerprint,
				},
				Status: remediationv1.RemediationRequestStatus{
					OverallPhase: remediationv1.PhaseFailed,
				},
			}
			Expect(fakeClient.Create(ctx, oldFailedRR)).To(Succeed())
		}

		incomingRR := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "incoming-rr",
				Namespace:         "default",
				UID:               types.UID("incoming-uid"),
				CreationTimestamp: metav1.Time{Time: time.Now()},
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: fingerprint,
			},
		}

		blocked := engine.CheckConsecutiveFailures(ctx, incomingRR)

		Expect(blocked).To(BeNil(),
			"Remediated RR should break chain: only 2 failures counted (below threshold 3)")
	})

	// UT-RO-1091-009: 3 consecutive Inconclusive RRs trigger blocking
	It("UT-RO-1091-009: should block after 3 consecutive Completed+Inconclusive RRs", func() {
		fingerprint := "triple-inconclusive-fp"
		baseTime := time.Now().Add(-10 * time.Minute)

		for i := 0; i < 3; i++ {
			incRR := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:              fmt.Sprintf("inconclusive-rr-%d", i),
					Namespace:         "default",
					UID:               types.UID(fmt.Sprintf("inconclusive-uid-%d", i)),
					CreationTimestamp: metav1.Time{Time: baseTime.Add(time.Duration(i) * time.Minute)},
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: fingerprint,
				},
				Status: remediationv1.RemediationRequestStatus{
					OverallPhase: remediationv1.PhaseCompleted,
					Outcome:      "Inconclusive",
				},
			}
			Expect(fakeClient.Create(ctx, incRR)).To(Succeed())
		}

		incomingRR := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "incoming-rr",
				Namespace:         "default",
				UID:               types.UID("incoming-uid"),
				CreationTimestamp: metav1.Time{Time: time.Now()},
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: fingerprint,
			},
		}

		blocked := engine.CheckConsecutiveFailures(ctx, incomingRR)

		Expect(blocked).ToNot(BeNil(), "3 consecutive Inconclusive should trigger blocking")
		Expect(blocked.Blocked).To(BeTrue())
		Expect(blocked.Reason).To(Equal(string(remediationv1.BlockReasonConsecutiveFailures)))
		Expect(blocked.RequeueAfter).To(Equal(time.Duration(3600) * time.Second))
	})
})
