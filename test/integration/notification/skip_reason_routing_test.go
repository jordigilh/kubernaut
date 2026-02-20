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

	corev1 "k8s.io/api/core/v1"
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

			// Issue #91: Create NotificationRequest with spec fields + metadata for routing
			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeEscalation,
					Priority: notificationv1alpha1.NotificationPriorityCritical,
					Severity: routing.SeverityCritical,
					Subject:  fmt.Sprintf("Test Skip-Reason: PreviousExecutionFailed [%s]", uniqueSuffix),
					Body:     "Workflow execution failed - cluster state unknown. Manual intervention required.",
					Recipients: []notificationv1alpha1.Recipient{
						{Email: "oncall@example.com"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
					Metadata: map[string]string{
						routing.AttrSkipReason:  routing.SkipReasonPreviousExecutionFailed,
						routing.AttrEnvironment: routing.EnvironmentProduction,
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

			// Issue #91: Verify routing data is in spec fields + metadata, not labels
			Expect(created.Spec.Severity).To(Equal(routing.SeverityCritical),
				"Severity should be in spec field")
			Expect(created.Spec.Metadata).To(HaveKeyWithValue(
				routing.AttrSkipReason,
				routing.SkipReasonPreviousExecutionFailed,
			), "Skip-reason should be in spec.metadata")
			Expect(created.Spec.Metadata).To(HaveKeyWithValue(
				routing.AttrEnvironment,
				routing.EnvironmentProduction,
			), "Environment should be in spec.metadata")

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
					},
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Type:     notificationv1alpha1.NotificationTypeStatusUpdate,
						Priority: notificationv1alpha1.NotificationPriorityMedium,
						Severity: severity,
						Subject:  fmt.Sprintf("Skip Reason Test: %s [%s]", skipReason, uniqueSuffix),
						Body:     fmt.Sprintf("Testing skip reason: %s", skipReason),
						Channels: []notificationv1alpha1.Channel{
							notificationv1alpha1.ChannelConsole,
						},
						Metadata: map[string]string{
							routing.AttrSkipReason: skipReason,
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
					return created.Spec.Metadata[routing.AttrSkipReason]
				}, 5*time.Second, 500*time.Millisecond).Should(Equal(skipReason),
					"Skip-reason should be in spec.metadata: %s", skipReason)

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

		// Test 3: Controller processes notification with combined routing attributes
		// BR-NOT-065: Multiple attributes can be combined for fine-grained routing
		It("should process notification with combined skip-reason and environment attributes", func() {
			notifName := fmt.Sprintf("combined-labels-test-%s", uniqueSuffix)

			// Issue #91: Create NR with combined spec fields + metadata for routing
			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeEscalation,
					Priority: notificationv1alpha1.NotificationPriorityCritical,
					Severity: routing.SeverityCritical,
					RemediationRequestRef: &corev1.ObjectReference{
						Name: "rr-test-12345",
					},
					Subject: fmt.Sprintf("Combined Labels Test [%s]", uniqueSuffix),
					Body:    "Testing combined spec field routing in production environment",
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
					Metadata: map[string]string{
						routing.AttrSkipReason:  routing.SkipReasonPreviousExecutionFailed,
						routing.AttrEnvironment: routing.EnvironmentProduction,
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

			// Issue #91: Routing data is in spec fields + metadata, not labels
			Expect(processed.Spec.Severity).To(Equal(routing.SeverityCritical))
			Expect(processed.Spec.Type).To(Equal(notificationv1alpha1.NotificationTypeEscalation))
			Expect(processed.Spec.RemediationRequestRef).ToNot(BeNil())
			Expect(processed.Spec.RemediationRequestRef.Name).To(Equal("rr-test-12345"))
			Expect(processed.Spec.Metadata[routing.AttrSkipReason]).To(Equal(routing.SkipReasonPreviousExecutionFailed))
			Expect(processed.Spec.Metadata[routing.AttrEnvironment]).To(Equal(routing.EnvironmentProduction))

			// Cleanup
			err = deleteAndWait(ctx, k8sClient, notif, 10*time.Second)
			Expect(err).NotTo(HaveOccurred())

			GinkgoWriter.Printf("✅ Combined routing attributes processed correctly\n")
		})

		// Test 4: Controller handles notification without skip-reason label
		// BR-NOT-065: Fallback to default routing when no skip-reason label
		It("should process notification without skip-reason label (fallback routing)", func() {
			notifName := fmt.Sprintf("no-skip-reason-test-%s", uniqueSuffix)

			// Issue #91: Create NR with spec fields + metadata (no skip-reason)
			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Severity: routing.SeverityMedium,
					Subject:  fmt.Sprintf("No Skip-Reason Test [%s]", uniqueSuffix),
					Body:     "Testing fallback routing without skip-reason",
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
					Metadata: map[string]string{
						routing.AttrEnvironment: routing.EnvironmentStaging,
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
			Expect(processed.Spec.Metadata).NotTo(HaveKey(routing.AttrSkipReason),
				"Skip-reason should not be in spec.metadata for this test")

			// Cleanup
			err = deleteAndWait(ctx, k8sClient, notif, 10*time.Second)
			Expect(err).NotTo(HaveOccurred())

			GinkgoWriter.Printf("✅ Notification without skip-reason processed (fallback routing)\n")
		})
	})

	// ============================================================================
	// Subcategory: Routing Attribute Consistency
	// ============================================================================

	Context("Routing Attribute Consistency", func() {

		// Test 5: Issue #91 - Verify routing attributes come from spec fields, no kubernaut.ai/* labels
		It("should use spec fields for routing, not kubernaut.ai labels", func() {
			notifName := fmt.Sprintf("spec-routing-test-%s", uniqueSuffix)

			// Issue #91: Create NR with routing data in spec fields + metadata
			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeEscalation,
					Priority: notificationv1alpha1.NotificationPriorityHigh,
					Severity: routing.SeverityHigh,
					RemediationRequestRef: &corev1.ObjectReference{
						Name: "rr-domain-test",
					},
					Subject: fmt.Sprintf("Spec Routing Test [%s]", uniqueSuffix),
					Body:    "Testing spec-field-based routing consistency",
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
					Metadata: map[string]string{
						routing.AttrSkipReason:  routing.SkipReasonExhaustedRetries,
						routing.AttrEnvironment: routing.EnvironmentProduction,
						routing.AttrNamespace:   "production",
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

			// Issue #91: Verify routing data is in spec fields, not labels
			Expect(created.Spec.Type).ToNot(BeEmpty(), "Routing type should be in spec.type")
			// No kubernaut.ai/* routing labels should be set anymore
			for key := range created.Labels {
				Expect(key).NotTo(HavePrefix("kubernaut.ai/"),
					"Issue #91: routing label %s should no longer be set; use spec fields instead", key)
			}

			// Wait for processing
			err = waitForReconciliationComplete(ctx, k8sClient, notifName, testNamespace,
				notificationv1alpha1.NotificationPhaseSent, 30*time.Second)
			Expect(err).NotTo(HaveOccurred())

			// Cleanup
			err = deleteAndWait(ctx, k8sClient, notif, 10*time.Second)
			Expect(err).NotTo(HaveOccurred())

			GinkgoWriter.Printf("✅ All routing data in spec fields, no kubernaut.ai labels\n")
		})
	})
})
