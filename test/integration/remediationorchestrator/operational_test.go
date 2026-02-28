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

package remediationorchestrator

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

// ========================================
// Phase 1 Integration Tests - Operational Visibility
//
// PHASE 1 PATTERN: RO Controller Only
// - RO controller creates child CRDs (SP, AI, WE)
// - Tests manually update child CRD status to simulate controller behavior
// - NO child controllers running (SP, AI, WE)
//
// This isolates RO's core logic:
// - Performance and scalability
// - Namespace isolation
// - Load handling
// ========================================

// ========================================
// Priority 3: Operational Visibility Tests
// TDD Integration Tests for operational behavior
// ========================================
var _ = Describe("Operational Visibility (Priority 3)", func() {

	// Phase 2 tests (Reconcile Performance, Namespace Isolation) moved to E2E suite:
	// - test/e2e/remediationorchestrator/operational_e2e_test.go

	// ========================================
	// Gap 3.2: High Load Scenarios
	// Business Value: Validates scalability
	// Confidence: 85% - Load testing critical path
	// ========================================
	//
	// ⚠️  CRITICAL: This test MUST run in Serial mode
	// Reason: Generates ~300-400 audit events that can crash DataStorage
	//         under default 2GB memory limit, causing cascade failures
	// See: RO_DATASTORAGE_CRASH_ROOT_CAUSE_DEC_24_2025.md
	// For production load testing, use performance tier with increased resources
	Describe("High Load Behavior (Gap 3.2)", Serial, func() {
		var namespace string

		BeforeEach(func() {
			namespace = createTestNamespace("ro-load")
		})

		AfterEach(func() {
			deleteTestNamespace(namespace)
		})

		It("should handle 100 concurrent RRs without degradation", func() {
			// Scenario: 100 RRs created simultaneously (load test)
			// Business Outcome: All process successfully, no rate limiting
			// Confidence: 85% - Validates scalability
			//
			// NOTE: This test generates high load (~300-400 audit events in 30s)
			// and may destabilize DataStorage if run in parallel with other tests

			ctx := context.Background()

			// Given: 100 RemediationRequests to create
			const numRRs = 100

			// When: Creating all RRs simultaneously
			now := metav1.Now()
			for i := 0; i < numRRs; i++ {
				rr := &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("load-rr-%d", i),
						Namespace: ROControllerNamespace,
					},
					Spec: remediationv1.RemediationRequestSpec{
						SignalName:        fmt.Sprintf("load-signal-%d", i),
						SignalFingerprint: fmt.Sprintf("%064d", i), // Valid 64-char fingerprint
						Severity:          "info",
						SignalType:        "test",
						TargetType:        "kubernetes",
						FiringTime:        now,
						ReceivedTime:      now,
						TargetResource: remediationv1.ResourceIdentifier{
							Kind:      "Deployment",
							Name:      fmt.Sprintf("test-app-%d", i),
							Namespace: namespace,
						},
					},
				}
				Expect(k8sClient.Create(ctx, rr)).To(Succeed())
			}

			// Then: All RRs should start processing (not rate limited)
			Eventually(func() int {
				rrList := &remediationv1.RemediationRequestList{}
				if err := k8sManager.GetAPIReader().List(ctx, rrList, client.InNamespace(ROControllerNamespace)); err != nil {
					return 0
				}

				processingCount := 0
				for _, rr := range rrList.Items {
					if rr.Status.OverallPhase != "" && rr.Status.OverallPhase != remediationv1.PhasePending {
						processingCount++
					}
				}
				return processingCount
			}, "30s", "500ms").Should(BeNumerically(">=", numRRs),
				"All %d RRs should start processing (no rate limiting)", numRRs)

			// Then: All RRs should have SignalProcessing created
			// Filter by SP name prefix (sp-load-rr-*) to avoid pollution from parallel tests
			Eventually(func() int {
				spList := &signalprocessingv1.SignalProcessingList{}
				if err := k8sManager.GetAPIReader().List(ctx, spList, client.InNamespace(ROControllerNamespace)); err != nil {
					return 0
				}
				count := 0
				prefix := "sp-load-rr-"
				for i := range spList.Items {
					if len(spList.Items[i].Name) >= len(prefix) && spList.Items[i].Name[:len(prefix)] == prefix {
						count++
					}
				}
				return count
			}, "60s", "1s").Should(Equal(numRRs),
				"All %d SignalProcessing CRDs should be created for load-rr-* RRs", numRRs)
		})
	})
})
