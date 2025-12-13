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

package remediationorchestrator_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// ========================================
// BR-ORCH-027/028: Timeout Management Integration Tests
// Business Value: Prevents stuck remediations from consuming resources indefinitely
// Test Type: Integration (envtest + controller)
//
// Per TESTING_GUIDELINES.md:
// - These are business requirement tests (BR-* prefix)
// - Validate business outcome (remediations terminate automatically)
// - Use real K8s API via envtest
// - NO Skip() allowed - tests must FAIL if infrastructure unavailable
//
// Reference: docs/requirements/BR-ORCH-027-028-timeout-management.md
// ========================================

var _ = Describe("BR-ORCH-027/028: Timeout Management", Label("integration", "timeout", "br-orch-027", "br-orch-028"), func() {

	// ========================================
	// Test 1: Global Timeout Enforcement
	// BR-ORCH-027 (P0 CRITICAL)
	// Business Outcome: Stuck remediations terminate after global timeout (1 hour default)
	// ========================================
	Describe("Global Timeout Enforcement (BR-ORCH-027)", func() {
		var namespace string

		BeforeEach(func() {
			namespace = createTestNamespace("timeout-global")
		})

		AfterEach(func() {
			deleteTestNamespace(namespace)
		})

		It("should transition to TimedOut when global timeout (1 hour) exceeded", func() {
			// TDD RED Phase: This test WILL FAIL until we implement timeout detection in controller
			//
			// Scenario: RemediationRequest created more than 1 hour ago
			// Business Outcome: RR transitions to TimedOut phase automatically
			// Confidence: 95% - Critical for production stability

			ctx := context.Background()

			By("Creating RemediationRequest")
			rrName := fmt.Sprintf("rr-timeout-%d", time.Now().UnixNano())
			now := metav1.Now()

			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      rrName,
					Namespace: namespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalName:        "timeout-test-signal",
					SignalFingerprint: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
					Severity:          "critical",
					SignalType:        "prometheus",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Deployment",
						Name:      "test-app",
						Namespace: namespace,
					},
					FiringTime:   now,
					ReceivedTime: now,
				},
			}

			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			By("Waiting for RR to be initialized by controller")
			Eventually(func() *metav1.Time {
				updated := &remediationv1.RemediationRequest{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, updated); err != nil {
					return nil
				}
				return updated.Status.StartTime
			}, timeout, interval).ShouldNot(BeNil(), "Controller should set status.StartTime")

			By("Manually setting status.StartTime to 61 minutes ago (simulates old RR)")
			pastTime := metav1.NewTime(time.Now().Add(-61 * time.Minute))
			Eventually(func() error {
				updated := &remediationv1.RemediationRequest{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, updated); err != nil {
					return err
				}
				updated.Status.StartTime = &pastTime
				return k8sClient.Status().Update(ctx, updated)
			}, timeout, interval).Should(Succeed())

			By("Triggering reconcile by adding annotation (forces controller to process)")
			Eventually(func() error {
				updated := &remediationv1.RemediationRequest{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, updated); err != nil {
					return err
				}
				if updated.Annotations == nil {
					updated.Annotations = make(map[string]string)
				}
				updated.Annotations["test.kubernaut.ai/trigger-reconcile"] = time.Now().String()
				return k8sClient.Update(ctx, updated)
			}, timeout, interval).Should(Succeed())

			By("Waiting for controller to detect global timeout on next reconcile")
			// Per TESTING_GUIDELINES.md: Use Eventually patterns for controller race conditions
			Eventually(func() remediationv1.RemediationPhase {
				updated := &remediationv1.RemediationRequest{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, updated); err != nil {
					return ""
				}
				return updated.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseTimedOut),
				"RR created 61 minutes ago should transition to TimedOut (BR-ORCH-027)")

			By("Verifying timeout metadata in status")
			final := &remediationv1.RemediationRequest{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, final)).To(Succeed())

			Expect(final.Status.TimeoutTime).ToNot(BeNil(),
				"TimeoutTime must be set when RR transitions to TimedOut")
			Expect(final.Status.TimeoutPhase).ToNot(BeNil(),
				"TimeoutPhase must track which phase was active when timeout occurred")
			Expect(*final.Status.TimeoutPhase).ToNot(BeEmpty(),
				"TimeoutPhase value must not be empty string")

			GinkgoWriter.Printf("✅ BR-ORCH-027: Global timeout enforced after 61 minutes\n")
		})

		It("should NOT timeout RR created less than 1 hour ago (negative test)", func() {
			// TDD RED Phase: Negative test to ensure timeout logic is correct
			//
			// Scenario: RemediationRequest created 30 minutes ago (within timeout)
			// Business Outcome: RR should NOT transition to TimedOut
			// Confidence: 95% - Validates timeout threshold is correct

			ctx := context.Background()

			By("Creating RemediationRequest")
			rrName := fmt.Sprintf("rr-notimeout-%d", time.Now().UnixNano())
			now := metav1.Now()

			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      rrName,
					Namespace: namespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalName:        "notimeout-test-signal",
					SignalFingerprint: "b1c2d3e4f5a6b1c2d3e4f5a6b1c2d3e4f5a6b1c2d3e4f5a6b1c2d3e4f5a6b1c2",
					Severity:          "warning",
					SignalType:        "prometheus",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Deployment",
						Name:      "test-app-2",
						Namespace: namespace,
					},
					FiringTime:   now,
					ReceivedTime: now,
				},
			}

			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			By("Waiting for RR to be initialized by controller")
			Eventually(func() *metav1.Time {
				updated := &remediationv1.RemediationRequest{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, updated); err != nil {
					return nil
				}
				return updated.Status.StartTime
			}, timeout, interval).ShouldNot(BeNil())

			By("Manually setting status.StartTime to 30 minutes ago (within timeout)")
			recentTime := metav1.NewTime(time.Now().Add(-30 * time.Minute))
			Eventually(func() error {
				updated := &remediationv1.RemediationRequest{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, updated); err != nil {
					return err
				}
				updated.Status.StartTime = &recentTime
				return k8sClient.Status().Update(ctx, updated)
			}, timeout, interval).Should(Succeed())

			By("Verifying RR progresses normally (NOT TimedOut)")
			// RR should progress to Processing phase normally
			Eventually(func() remediationv1.RemediationPhase {
				updated := &remediationv1.RemediationRequest{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, updated); err != nil {
					return ""
				}
				return updated.Status.OverallPhase
			}, timeout, interval).Should(Equal(remediationv1.PhaseProcessing),
				"RR created 30 minutes ago should progress normally, not timeout")

			By("Consistently verifying RR never transitions to TimedOut")
			Consistently(func() remediationv1.RemediationPhase {
				updated := &remediationv1.RemediationRequest{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: rrName, Namespace: namespace}, updated); err != nil {
					return ""
				}
				return updated.Status.OverallPhase
			}, "5s", "500ms").ShouldNot(Equal(remediationv1.PhaseTimedOut),
				"RR within timeout window must NOT transition to TimedOut")

			GinkgoWriter.Printf("✅ BR-ORCH-027: RR within timeout window progresses normally\n")
		})
	})

	// ========================================
	// Test 2: Per-Remediation Timeout Override
	// BR-ORCH-028 (P1 HIGH)
	// Business Outcome: Flexible timeout for different remediation types
	// ========================================
	Describe("Per-Remediation Timeout Override (BR-ORCH-028)", func() {
		var namespace string

		BeforeEach(func() {
			namespace = createTestNamespace("timeout-override")
		})

		AfterEach(func() {
			deleteTestNamespace(namespace)
		})

		PIt("should respect per-remediation timeout override (spec.timeoutConfig)", func() {
			// TDD RED Phase: PENDING - Requires spec.timeoutConfig field in CRD schema
			//
			// Scenario: RR with custom 2-hour timeout (vs default 1 hour)
			// Business Outcome: Timeout respects override, not default
			// Confidence: 90% - Important for timeout flexibility
			//
			// DEFERRED: Requires CRD schema update to add spec.timeoutConfig
			// Priority: P1 but blocked by schema change
			// Estimated Time: 1 hour after schema available

			ctx := context.Background()
			_ = ctx

			// Test implementation deferred until spec.timeoutConfig added to CRD
			// See: api/remediation/v1alpha1/remediationrequest_types.go
		})
	})

	// ========================================
	// Test 3: Per-Phase Timeout Detection
	// BR-ORCH-028 (P1 HIGH)
	// Business Outcome: Faster detection of stuck phases (e.g., AwaitingApproval)
	// ========================================
	Describe("Per-Phase Timeout Detection (BR-ORCH-028)", func() {
		var namespace string

		BeforeEach(func() {
			namespace = createTestNamespace("timeout-phase")
		})

		AfterEach(func() {
			deleteTestNamespace(namespace)
		})

		PIt("should detect per-phase timeout (e.g., AwaitingApproval > 15 min)", func() {
			// TDD RED Phase: PENDING - Requires phase timeout configuration
			//
			// Scenario: RR in AwaitingApproval for 16 minutes (default phase timeout: 15 min)
			// Business Outcome: RR transitions to Failed with timeout reason
			// Confidence: 90% - Reduces MTTR for stuck approvals
			//
			// DEFERRED: Requires phase timeout configuration mechanism
			// Priority: P1 but requires design decision on configuration
			// Estimated Time: 2 hours after configuration approach decided

			ctx := context.Background()
			_ = ctx

			// Test implementation deferred until phase timeout config available
			// See: BR-ORCH-028 for phase timeout configuration requirements
		})
	})

	// ========================================
	// Test 4: Timeout Notification Escalation
	// BR-ORCH-027 (P0 CRITICAL)
	// Business Outcome: Operators notified of timeout for manual intervention
	// ========================================
	Describe("Timeout Notification Escalation (BR-ORCH-027)", func() {
		var namespace string

		BeforeEach(func() {
			namespace = createTestNamespace("timeout-notification")
		})

		AfterEach(func() {
			deleteTestNamespace(namespace)
		})

	It("should create NotificationRequest on global timeout (escalation)", func() {
		// TDD GREEN Phase: Tests 1-2 passed, now implementing notification creation
		//
		// Scenario: RR times out after 61 minutes
		// Business Outcome: NotificationRequest created with escalation type
		// Confidence: 90% - Important for operational visibility
		//
		// Prerequisites: Tests 1-2 passing (timeout detection working)
		// Business Requirement: BR-ORCH-027 (Global Timeout Management)

		ctx := context.Background()

			By("Creating RemediationRequest that will timeout")
			rrName := fmt.Sprintf("rr-timeout-notify-%d", time.Now().UnixNano())
			now := metav1.Now()
			pastTime := metav1.NewTime(time.Now().Add(-61 * time.Minute))

			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:              rrName,
					Namespace:         namespace,
					CreationTimestamp: pastTime,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalName:        "timeout-notify-signal",
					SignalFingerprint: "timeoutnotify4567890abcdef1234567890abcdef1234567890abcdef1234567",
					Severity:          "critical",
					SignalType:        "prometheus",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Deployment",
						Name:      "test-app-timeout",
						Namespace: namespace,
					},
					FiringTime:   now,
					ReceivedTime: now,
				},
			}

			Expect(k8sClient.Create(ctx, rr)).To(Succeed())

			By("Waiting for NotificationRequest to be created on timeout")
			notificationName := fmt.Sprintf("timeout-%s", rrName)
			Eventually(func() error {
				nr := &notificationv1.NotificationRequest{}
				return k8sClient.Get(ctx, types.NamespacedName{Name: notificationName, Namespace: namespace}, nr)
			}, timeout, interval).Should(Succeed(),
				"NotificationRequest should be created when RR times out (BR-ORCH-027)")

			By("Verifying NotificationRequest has escalation type")
			nr := &notificationv1.NotificationRequest{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: notificationName, Namespace: namespace}, nr)).To(Succeed())

			Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeEscalation),
				"Timeout notification must be escalation type")
			Expect(nr.Spec.Subject).To(ContainSubstring("timeout"),
				"Notification subject must mention timeout")

			GinkgoWriter.Printf("✅ BR-ORCH-027: Timeout notification created for operator escalation\n")
		})
	})
})





