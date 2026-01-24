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

package notification

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/routing"
)

// =============================================================================
// Skip-Reason Routing Integration Tests (BR-NOT-065, DD-WE-004)
// =============================================================================
//
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - Unit tests (70%+): Business logic in isolation (routing_config_test.go)
// - Integration tests (>50%): CRD-based coordination, controller behavior
// - E2E tests (10-15%): Complete workflow validation
//
// These integration tests validate:
// - Controller receives NotificationRequest CRD with skip-reason label
// - Label is correctly extracted from CRD metadata
// - Routing rules (when configured) would determine channels
// - End-to-end label → controller → delivery flow
//
// Cross-Team Reference:
// - DD-WE-004: WorkflowExecution Exponential Backoff
// - NOTICE_WE_EXPONENTIAL_BACKOFF_DD_WE_004.md (v1.5)
// =============================================================================

var _ = Describe("Skip-Reason Routing Integration (BR-NOT-065, DD-WE-004)", Label("integration", "routing", "skip-reason"), func() {
	var (
		uniqueSuffix string
	)

	BeforeEach(func() {
		uniqueSuffix = fmt.Sprintf("%d-%d", time.Now().UnixNano(), GinkgoParallelProcess())

		// Reset mock Slack server state for clean test
		ConfigureFailureMode("none", 0, 0)
		resetSlackRequests()
	})

	// ============================================================================
	// Subcategory: CRD Label Propagation
	// ============================================================================

	Context("CRD Label Propagation", func() {

		// Test 1: Verify skip-reason label is preserved through CRD lifecycle
		// BR-NOT-065: Labels on NotificationRequest CRD drive routing decisions
		It("should preserve skip-reason label through CRD creation and retrieval", func() {
			notifName := fmt.Sprintf("skip-reason-label-test-%s", uniqueSuffix)

			// Create NotificationRequest with skip-reason label
			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
					Labels: map[string]string{
						routing.LabelSkipReason:  routing.SkipReasonPreviousExecutionFailed,
						routing.LabelEnvironment: routing.EnvironmentProduction,
						routing.LabelSeverity:    routing.SeverityCritical,
						routing.LabelComponent:   "remediation-orchestrator",
					},
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeEscalation,
					Priority: notificationv1alpha1.NotificationPriorityCritical,
					Subject:  fmt.Sprintf("Test Skip-Reason: PreviousExecutionFailed [%s]", uniqueSuffix),
					Body:     "Workflow execution failed - cluster state unknown. Manual intervention required.",
					Recipients: []notificationv1alpha1.Recipient{
						{Email: "oncall@example.com"},
					},
					// Channels intentionally empty - routing rules should apply
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole, // Use console for deterministic testing
					},
				},
			}

			// Create the CRD
			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred(), "Failed to create NotificationRequest with skip-reason label")

			// Retrieve and verify labels are preserved
			created := &notificationv1alpha1.NotificationRequest{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, created)
			}, 5*time.Second, 500*time.Millisecond).Should(Succeed(), "NotificationRequest should exist")

			// Verify skip-reason label is preserved
			Expect(created.Labels).To(HaveKeyWithValue(
				routing.LabelSkipReason,
				routing.SkipReasonPreviousExecutionFailed,
			), "Skip-reason label should be preserved")

			// Verify other routing labels are preserved
			Expect(created.Labels).To(HaveKeyWithValue(
				routing.LabelEnvironment,
				routing.EnvironmentProduction,
			), "Environment label should be preserved")

			Expect(created.Labels).To(HaveKeyWithValue(
				routing.LabelSeverity,
				routing.SeverityCritical,
			), "Severity label should be preserved")

			// Wait for controller to process (should reach Sent phase)
			err = waitForReconciliationComplete(ctx, k8sClient, notifName, testNamespace,
				notificationv1alpha1.NotificationPhaseSent, 30*time.Second)
			Expect(err).NotTo(HaveOccurred(), "Notification should reach Sent phase")

			// Cleanup
			err = deleteAndWait(ctx, k8sClient, notif, 10*time.Second)
			Expect(err).NotTo(HaveOccurred(), "Cleanup should succeed")

			GinkgoWriter.Printf("✅ Skip-reason label preserved through CRD lifecycle\n")
		})

		// Test 2: Verify all DD-WE-004 skip reasons can be set as labels
		// DD-WE-004: Defines 4 skip reasons for WorkflowExecution failures
		DescribeTable("should accept all DD-WE-004 skip reason values as labels",
			func(skipReason string, skipReasonSlug string, severity string, expectedPhase notificationv1alpha1.NotificationPhase) {
				// Use slug for CRD name (lowercase, RFC 1123 compliant)
				notifName := fmt.Sprintf("skip-reason-%s-%s", skipReasonSlug, uniqueSuffix)

				notif := &notificationv1alpha1.NotificationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:       notifName,
						Namespace:  testNamespace,
						Generation: 1, // K8s increments on create/update
						Labels: map[string]string{
							routing.LabelSkipReason: skipReason,
							routing.LabelSeverity:   severity,
						},
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Type:     notificationv1alpha1.NotificationTypeStatusUpdate,
						Priority: notificationv1alpha1.NotificationPriorityMedium,
						Subject:  fmt.Sprintf("Skip Reason Test: %s [%s]", skipReason, uniqueSuffix),
						Body:     fmt.Sprintf("Testing skip reason: %s", skipReason),
						Channels: []notificationv1alpha1.Channel{
							notificationv1alpha1.ChannelConsole,
						},
					},
				}

				// Create and verify
				err := k8sClient.Create(ctx, notif)
				Expect(err).NotTo(HaveOccurred(), "Failed to create NotificationRequest with skip-reason: %s", skipReason)

				// Verify label is set correctly
				created := &notificationv1alpha1.NotificationRequest{}
				Eventually(func() string {
					_ = k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
						Name:      notifName,
						Namespace: testNamespace,
					}, created)
					return created.Labels[routing.LabelSkipReason]
				}, 5*time.Second, 500*time.Millisecond).Should(Equal(skipReason),
					"Skip-reason label should be: %s", skipReason)

				// Wait for reconciliation
				err = waitForReconciliationComplete(ctx, k8sClient, notifName, testNamespace,
					expectedPhase, 30*time.Second)
				Expect(err).NotTo(HaveOccurred(), "Notification with skip-reason %s should complete", skipReason)

				// Cleanup
				err = deleteAndWait(ctx, k8sClient, notif, 10*time.Second)
				Expect(err).NotTo(HaveOccurred())

				GinkgoWriter.Printf("✅ Skip-reason '%s' accepted and processed\n", skipReason)
			},
			// CRITICAL: PreviousExecutionFailed - cluster state unknown
			Entry("PreviousExecutionFailed (CRITICAL)",
				routing.SkipReasonPreviousExecutionFailed,
				"prev-exec-failed", // RFC 1123 slug
				routing.SeverityCritical,
				notificationv1alpha1.NotificationPhaseSent),

			// HIGH: ExhaustedRetries - infrastructure issues
			Entry("ExhaustedRetries (HIGH)",
				routing.SkipReasonExhaustedRetries,
				"exhausted-retries", // RFC 1123 slug
				routing.SeverityHigh,
				notificationv1alpha1.NotificationPhaseSent),

			// LOW: ResourceBusy - temporary condition
			Entry("ResourceBusy (LOW)",
				routing.SkipReasonResourceBusy,
				"resource-busy", // RFC 1123 slug
				routing.SeverityLow,
				notificationv1alpha1.NotificationPhaseSent),

			// LOW: RecentlyRemediated - cooldown active
			Entry("RecentlyRemediated (LOW)",
				routing.SkipReasonRecentlyRemediated,
				"recently-remediated", // RFC 1123 slug
				routing.SeverityLow,
				notificationv1alpha1.NotificationPhaseSent),
		)
	})

	// ============================================================================
	// Subcategory: Controller Label Extraction
	// ============================================================================

	Context("Controller Label Extraction", func() {

		// Test 3: Controller processes notification with combined routing labels
		// BR-NOT-065: Multiple labels can be combined for fine-grained routing
		It("should process notification with combined skip-reason and environment labels", func() {
			notifName := fmt.Sprintf("combined-labels-test-%s", uniqueSuffix)

			// Create NotificationRequest with multiple routing labels
			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
					Labels: map[string]string{
						routing.LabelSkipReason:         routing.SkipReasonPreviousExecutionFailed,
						routing.LabelEnvironment:        routing.EnvironmentProduction,
						routing.LabelSeverity:           routing.SeverityCritical,
						routing.LabelComponent:          "remediation-orchestrator",
						routing.LabelRemediationRequest: "rr-test-12345",
					},
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeEscalation,
					Priority: notificationv1alpha1.NotificationPriorityCritical,
					Subject:  fmt.Sprintf("Combined Labels Test [%s]", uniqueSuffix),
					Body:     "Testing combined label routing in production environment",
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
				},
			}

			// Create
			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred())

			// Wait for processing
			err = waitForReconciliationComplete(ctx, k8sClient, notifName, testNamespace,
				notificationv1alpha1.NotificationPhaseSent, 30*time.Second)
			Expect(err).NotTo(HaveOccurred(), "Notification with combined labels should complete")

			// Verify all labels are preserved after processing
			processed := &notificationv1alpha1.NotificationRequest{}
			err = k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name:      notifName,
				Namespace: testNamespace,
			}, processed)
			Expect(err).NotTo(HaveOccurred())

			// All routing labels should be intact
			Expect(processed.Labels).To(HaveLen(5), "All 5 routing labels should be preserved")
			Expect(processed.Labels[routing.LabelSkipReason]).To(Equal(routing.SkipReasonPreviousExecutionFailed))
			Expect(processed.Labels[routing.LabelEnvironment]).To(Equal(routing.EnvironmentProduction))
			Expect(processed.Labels[routing.LabelSeverity]).To(Equal(routing.SeverityCritical))
			Expect(processed.Labels[routing.LabelComponent]).To(Equal("remediation-orchestrator"))
			Expect(processed.Labels[routing.LabelRemediationRequest]).To(Equal("rr-test-12345"))

			// Cleanup
			err = deleteAndWait(ctx, k8sClient, notif, 10*time.Second)
			Expect(err).NotTo(HaveOccurred())

			GinkgoWriter.Printf("✅ Combined routing labels processed correctly\n")
		})

		// Test 4: Controller handles notification without skip-reason label
		// BR-NOT-065: Fallback to default routing when no skip-reason label
		It("should process notification without skip-reason label (fallback routing)", func() {
			notifName := fmt.Sprintf("no-skip-reason-test-%s", uniqueSuffix)

			// Create NotificationRequest WITHOUT skip-reason label
			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
					Labels: map[string]string{
						routing.LabelEnvironment: routing.EnvironmentStaging,
						routing.LabelSeverity:    routing.SeverityMedium,
					},
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  fmt.Sprintf("No Skip-Reason Test [%s]", uniqueSuffix),
					Body:     "Testing fallback routing without skip-reason label",
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
				},
			}

			// Create
			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred())

			// Should still complete successfully (fallback routing)
			err = waitForReconciliationComplete(ctx, k8sClient, notifName, testNamespace,
				notificationv1alpha1.NotificationPhaseSent, 30*time.Second)
			Expect(err).NotTo(HaveOccurred(), "Notification without skip-reason should still complete")

			// Verify skip-reason label is NOT present
			processed := &notificationv1alpha1.NotificationRequest{}
			err = k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name:      notifName,
				Namespace: testNamespace,
			}, processed)
			Expect(err).NotTo(HaveOccurred())
			Expect(processed.Labels).NotTo(HaveKey(routing.LabelSkipReason),
				"Skip-reason label should not be present")

			// Cleanup
			err = deleteAndWait(ctx, k8sClient, notif, 10*time.Second)
			Expect(err).NotTo(HaveOccurred())

			GinkgoWriter.Printf("✅ Notification without skip-reason processed (fallback routing)\n")
		})
	})

	// ============================================================================
	// Subcategory: Routing Label Consistency
	// ============================================================================

	Context("Routing Label Consistency", func() {

		// Test 5: Verify label domain is kubernaut.ai (not kubernaut.io)
		// Per: NOTICE_LABEL_DOMAIN_AND_NOTIFICATION_ROUTING.md
		It("should use kubernaut.ai domain for all routing labels", func() {
			notifName := fmt.Sprintf("label-domain-test-%s", uniqueSuffix)

			// Create NotificationRequest with all routing labels
			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
					Labels: map[string]string{
						routing.LabelSkipReason:         routing.SkipReasonExhaustedRetries,
						routing.LabelNotificationType:   routing.NotificationTypeEscalation,
						routing.LabelSeverity:           routing.SeverityHigh,
						routing.LabelEnvironment:        routing.EnvironmentProduction,
						routing.LabelPriority:           "P1",
						routing.LabelComponent:          "workflow-execution",
						routing.LabelRemediationRequest: "rr-domain-test",
						routing.LabelNamespace:          "production",
					},
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeEscalation,
					Priority: notificationv1alpha1.NotificationPriorityHigh,
					Subject:  fmt.Sprintf("Label Domain Test [%s]", uniqueSuffix),
					Body:     "Testing kubernaut.ai label domain consistency",
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
				},
			}

			// Create
			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred())

			// Retrieve
			created := &notificationv1alpha1.NotificationRequest{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, created)
			}, 5*time.Second, 500*time.Millisecond).Should(Succeed())

			// Verify ALL label keys use kubernaut.ai domain
			for key := range created.Labels {
				if key != routing.LabelSkipReason &&
					key != routing.LabelNotificationType &&
					key != routing.LabelSeverity &&
					key != routing.LabelEnvironment &&
					key != routing.LabelPriority &&
					key != routing.LabelComponent &&
					key != routing.LabelRemediationRequest &&
					key != routing.LabelNamespace {
					// Skip non-routing labels (if any system labels exist)
					continue
				}

				// Verify domain is kubernaut.ai, NOT kubernaut.io
				Expect(key).To(HavePrefix("kubernaut.ai/"),
					"Routing label %s should use kubernaut.ai domain", key)
				Expect(key).NotTo(HavePrefix("kubernaut.io/"),
					"Routing label %s should NOT use kubernaut.io domain", key)
			}

			// Wait for processing
			err = waitForReconciliationComplete(ctx, k8sClient, notifName, testNamespace,
				notificationv1alpha1.NotificationPhaseSent, 30*time.Second)
			Expect(err).NotTo(HaveOccurred())

			// Cleanup
			err = deleteAndWait(ctx, k8sClient, notif, 10*time.Second)
			Expect(err).NotTo(HaveOccurred())

			GinkgoWriter.Printf("✅ All routing labels use kubernaut.ai domain\n")
		})
	})
})
