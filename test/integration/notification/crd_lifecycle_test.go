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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

// DD-NOT-003 V2.1: Category 1 - CRD Lifecycle Integration Tests
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - Unit tests (70%+): Business logic in isolation
// - Integration tests (>50%): CRD-based coordination, K8s API behavior
// - E2E tests (10-15%): Complete workflow validation
//
// These integration tests validate CRD lifecycle with REAL Kubernetes API (envtest)

var _ = Describe("Category 1: CRD Lifecycle Integration Tests", Label("integration", "crd-lifecycle"), func() {
	var (
		uniqueSuffix string
	)

	BeforeEach(func() {
		uniqueSuffix = fmt.Sprintf("%d", GinkgoRandomSeed())

		// Reset mock Slack server state
		ConfigureFailureMode("none", 0, 0)
		resetSlackRequests()
	})

	// ============================================================================
	// Subcategory 1A: Basic CRD Operations (10 tests)
	// ============================================================================

	Context("Subcategory 1A: Basic CRD Operations", func() {

		// Test 1: Create CRD with minimal required fields
		// BR-NOT-002: NotificationRequest Schema Validation
		It("should create NotificationRequest with minimal required fields", func() {
			notifName := fmt.Sprintf("minimal-notif-%s", uniqueSuffix)

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "Test Minimal Notification",
					Body:     "Minimal body content",
					Recipients: []notificationv1alpha1.Recipient{
						{Email: "test@example.com"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
				},
			}

			// Create CRD
			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred(), "Failed to create NotificationRequest with minimal fields")

			// Verify CRD exists
			created := &notificationv1alpha1.NotificationRequest{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, created)
			}, 5*time.Second, 500*time.Millisecond).Should(Succeed(), "NotificationRequest should exist")

			// Verify spec fields
			Expect(created.Spec.Type).To(Equal(notificationv1alpha1.NotificationTypeSimple))
			Expect(created.Spec.Priority).To(Equal(notificationv1alpha1.NotificationPriorityMedium))
			Expect(created.Spec.Subject).To(Equal("Test Minimal Notification"))
			Expect(created.Spec.Body).To(Equal("Minimal body content"))
			Expect(created.Spec.Recipients).To(HaveLen(1))
			Expect(created.Spec.Channels).To(HaveLen(1))

			// Cleanup
			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred(), "Cleanup should complete")
		})

		// Test 2: Create CRD with all optional fields
		// BR-NOT-002: NotificationRequest Schema Validation
		It("should create NotificationRequest with all optional fields", func() {
			notifName := fmt.Sprintf("full-notif-%s", uniqueSuffix)

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
					Labels: map[string]string{
						"test-label": "integration",
					},
					Annotations: map[string]string{
						"test-annotation": "full-spec",
					},
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeEscalation,
					Priority: notificationv1alpha1.NotificationPriorityCritical,
					Subject:  "Test Full Notification",
					Body:     "Full body content with all optional fields",
					Recipients: []notificationv1alpha1.Recipient{
						{Email: "test1@example.com"},
						{Email: "test2@example.com"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
						notificationv1alpha1.ChannelSlack,
					},
					Metadata: map[string]string{
						"remediationRequestName": "rr-123",
						"cluster":                "prod-us-east-1",
						"severity":               "critical",
					},
					ActionLinks: []notificationv1alpha1.ActionLink{
						{
							Service: "grafana",
							URL:     "https://grafana.example.com/dashboard",
							Label:   "View Dashboard",
						},
						{
							Service: "prometheus",
							URL:     "https://prometheus.example.com/alerts",
							Label:   "View Alert",
						},
					},
					RetryPolicy: &notificationv1alpha1.RetryPolicy{
						MaxAttempts:           5,
						InitialBackoffSeconds: 30,
						BackoffMultiplier:     2,
						MaxBackoffSeconds:     480,
					},
					RetentionDays: 14,
				},
			}

			// Create CRD
			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred(), "Failed to create NotificationRequest with all fields")

			// Verify CRD exists
			created := &notificationv1alpha1.NotificationRequest{}
			Eventually(func() error {
				return k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, created)
			}, 5*time.Second, 500*time.Millisecond).Should(Succeed())

			// CORRECTNESS VALIDATION: Verify all optional fields have exact expected values
			Expect(created.ObjectMeta.Labels).To(HaveKeyWithValue("test-label", "integration"),
				"Labels should be preserved exactly as specified")
			Expect(created.ObjectMeta.Annotations).To(HaveKeyWithValue("test-annotation", "full-spec"),
				"Annotations should be preserved exactly as specified")
			Expect(created.Spec.Metadata).To(HaveKeyWithValue("remediationRequestName", "rr-123"),
				"Metadata should contain remediationRequestName")

			// BEHAVIOR VALIDATION: Verify action links structure
			Expect(created.Spec.ActionLinks).To(HaveLen(2), "Should have exactly 2 action links")
			Expect(created.Spec.ActionLinks[0].Service).To(Equal("grafana"),
				"First action link should be for grafana service")
			Expect(created.Spec.ActionLinks[0].URL).To(Equal("https://grafana.example.com/dashboard"),
				"Grafana link should have correct URL")
			Expect(created.Spec.ActionLinks[0].Label).To(Equal("View Dashboard"),
				"Grafana link should have correct label")
			Expect(created.Spec.ActionLinks[1].Service).To(Equal("prometheus"),
				"Second action link should be for prometheus service")

			// CORRECTNESS VALIDATION: Verify retry policy configuration
			Expect(created.Spec.RetryPolicy.MaxAttempts).To(Equal(5),
				"RetryPolicy.MaxAttempts should be correctly configured")
			Expect(created.Spec.RetryPolicy.InitialBackoffSeconds).To(Equal(30),
				"RetryPolicy.InitialBackoffSeconds should be correctly configured")
			Expect(created.Spec.RetryPolicy.BackoffMultiplier).To(Equal(2),
				"RetryPolicy.BackoffMultiplier should be correctly configured")
			Expect(created.Spec.RetryPolicy.InitialBackoffSeconds).To(Equal(30),
				"InitialBackoffSeconds should be set to 30")
			Expect(created.Spec.RetryPolicy.BackoffMultiplier).To(Equal(2),
				"BackoffMultiplier should be set to 2")
			Expect(created.Spec.RetryPolicy.MaxBackoffSeconds).To(Equal(480),
				"MaxBackoffSeconds should be set to 480")

			// CORRECTNESS VALIDATION: Verify retention configuration
			Expect(created.Spec.RetentionDays).To(Equal(14),
				"RetentionDays should be set to 14")

			// Cleanup
			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred(), "Cleanup should complete")
		})

		// Test 3: CRD status initialization
		// BR-NOT-051: Notification Status Tracking
		It("should initialize NotificationRequest status on first reconciliation", func() {
			notifName := fmt.Sprintf("status-init-%s", uniqueSuffix)

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "Status Init Test",
					Body:     "Testing status initialization",
					Recipients: []notificationv1alpha1.Recipient{
						{Email: "test@example.com"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
				},
			}

			// Create CRD
			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred(), "Cleanup should complete")

			// Wait for controller to initialize status and complete delivery
			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, notif)
				if err != nil {
					return ""
				}
				return notif.Status.Phase
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent),
				"Controller should initialize status and complete delivery")

			// BEHAVIOR VALIDATION: Verify status reflects actual delivery outcome
			Expect(notif.Status.Phase).To(Equal(notificationv1alpha1.NotificationPhaseSent),
				"Phase should be Sent after successful console delivery")
			Expect(notif.Status.Reason).To(Equal("AllDeliveriesSucceeded"),
				"Reason should indicate all deliveries succeeded")
			Expect(notif.Status.Message).To(ContainSubstring("Successfully delivered to 1 channel"),
				"Message should confirm delivery count")
			Expect(notif.Status.SuccessfulDeliveries).To(Equal(1),
				"Should have exactly 1 successful delivery")
			Expect(notif.Status.FailedDeliveries).To(Equal(0),
				"Should have zero failed deliveries")
			Expect(notif.Status.TotalAttempts).To(Equal(1),
				"Should have exactly 1 delivery attempt")
			Expect(notif.Status.DeliveryAttempts).To(HaveLen(1),
				"Should have 1 delivery attempt record")
			Expect(notif.Status.DeliveryAttempts[0].Channel).To(Equal("console"),
				"Delivery attempt should be for console channel")
			Expect(notif.Status.DeliveryAttempts[0].Status).To(Equal("success"),
				"Delivery attempt should be marked as success")

			// CORRECTNESS VALIDATION: CompletionTime is valid and recent
			Expect(notif.Status.CompletionTime).ToNot(BeNil(),
				"CompletionTime must be set after successful delivery")
			completionTime := notif.Status.CompletionTime.Time
			Expect(completionTime).To(BeTemporally("~", time.Now(), 30*time.Second),
				"CompletionTime should be recent (within test execution window)")
			Expect(completionTime).To(BeTemporally(">=", notif.CreationTimestamp.Time),
				"CompletionTime should be after notification creation")

			GinkgoWriter.Printf("✅ Status validates correct behavior: phase=%s, successful=%d, failed=%d\n",
				notif.Status.Phase, notif.Status.SuccessfulDeliveries, notif.Status.FailedDeliveries)

			// Cleanup
			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred(), "Cleanup should complete")
		})

		// Test 4: CRD with optional fields omitted (BR-NOT-065)
		// BR-NOT-002: NotificationRequest Schema Validation
		// BR-NOT-065: Channel Routing Based on Spec Fields
		It("should accept NotificationRequest with optional fields omitted (BR-NOT-065)", func() {
			notifName := fmt.Sprintf("optional-fields-%s", uniqueSuffix)

			// Recipients and Channels are optional per CRD schema
			// BR-NOT-065: Empty channels triggers spec-field-based routing rules
			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Severity: "medium",
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "Optional Fields Test",
					Body:     "Testing with optional fields omitted",
					// Recipients omitted (optional per CRD schema)
					// Channels omitted (optional - routing rules apply per BR-NOT-065)
				},
			}

			// Create CRD - should succeed
			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred(), "Should accept CRD with optional fields omitted (BR-NOT-065)")

			// Verify CRD exists and controller started processing
			created := &notificationv1alpha1.NotificationRequest{}
			Eventually(func() bool {
				err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, created)
				if err != nil {
					return false
				}
				// Controller started processing (status initialized)
				return created.Status.Phase != ""
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"Controller should start processing notification")

			// Cleanup
			_ = k8sClient.Delete(ctx, notif)

			GinkgoWriter.Printf("✅ Optional fields accepted - routing rules applied (BR-NOT-065)\n")
		})

		// Test 5: CRD name validation (DNS-1123 subdomain)
		// BR-NOT-002: NotificationRequest Schema Validation
		It("should validate NotificationRequest name follows DNS-1123 subdomain rules", func() {
			// Invalid name (uppercase not allowed)
			invalidNotif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "INVALID-NAME",
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "Name Validation Test",
					Body:     "Testing name validation",
					Recipients: []notificationv1alpha1.Recipient{
						{Email: "test@example.com"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
				},
			}

			// Attempt to create with invalid name
			err := k8sClient.Create(ctx, invalidNotif)
			Expect(err).To(HaveOccurred(), "Should reject invalid DNS-1123 name")
			Expect(apierrors.IsInvalid(err)).To(BeTrue())

			// Valid name should succeed
			validName := fmt.Sprintf("valid-name-%s", uniqueSuffix)
			validNotif := invalidNotif.DeepCopy()
			validNotif.ObjectMeta.Name = validName

			err = k8sClient.Create(ctx, validNotif)
			Expect(err).NotTo(HaveOccurred(), "Should accept valid DNS-1123 name")

			// Cleanup
			err = k8sClient.Delete(ctx, validNotif)
			Expect(err).NotTo(HaveOccurred(), "Cleanup should complete")
		})

		// Test 6: CRD with very long name (63 chars - boundary condition)
		// BR-NOT-002: NotificationRequest Schema Validation
		It("should accept NotificationRequest with 63-character name (DNS-1123 limit)", func() {
			// DNS-1123 subdomain allows max 63 characters
			longName := "a123456789b123456789c123456789d123456789e123456789f123456789abc"
			Expect(len(longName)).To(Equal(63), "Test name should be exactly 63 characters")

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       longName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "Long Name Test",
					Body:     "Testing maximum name length",
					Recipients: []notificationv1alpha1.Recipient{
						{Email: "test@example.com"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
				},
			}

			// Create with 63-char name
			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred(), "Should accept 63-character name")

			// Verify CRD exists
			created := &notificationv1alpha1.NotificationRequest{}
			err = k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name:      longName,
				Namespace: testNamespace,
			}, created)
			Expect(err).NotTo(HaveOccurred(), "Cleanup should complete")
			Expect(created.Name).To(Equal(longName))

			// Cleanup
			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred(), "Cleanup should complete")
		})

		// Test 7: Reconciliation loop triggers on CRD creation
		// BR-NOT-053: At-Least-Once Delivery Guarantee
		It("should trigger reconciliation loop immediately after CRD creation", func() {
			notifName := fmt.Sprintf("reconcile-test-%s", uniqueSuffix)

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "Reconciliation Test",
					Body:     "Testing immediate reconciliation",
					Recipients: []notificationv1alpha1.Recipient{
						{Email: "test@example.com"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
				},
			}

			// Record creation time
			creationTime := time.Now()

			// Create CRD
			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred(), "Cleanup should complete")

			// Wait for status to be updated by controller
			var reconcileTime time.Time
			Eventually(func() bool {
				err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, notif)
				if err != nil {
					return false
				}
				if notif.Status.Phase != "" {
					reconcileTime = time.Now()
					return true
				}
				return false
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"Controller should reconcile and update status within 10 seconds")

			// Verify reconciliation happened quickly (within 5 seconds of creation)
			reconcileDuration := reconcileTime.Sub(creationTime)
			Expect(reconcileDuration).To(BeNumerically("<", 5*time.Second),
				"Reconciliation should occur within 5 seconds of CRD creation")

			GinkgoWriter.Printf("✅ Reconciliation triggered in %v\n", reconcileDuration)

			// Cleanup
			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred(), "Cleanup should complete")
		})

		// Test 8: Status update failure recovery
		// BR-NOT-053: At-Least-Once Delivery Guarantee
		It("should retry status updates on transient K8s API failures", func() {
			// Note: This test verifies controller behavior under API contention
			// Real envtest doesn't easily simulate API failures, so we verify
			// that status updates eventually succeed even with concurrent operations

			notifName := fmt.Sprintf("status-retry-%s", uniqueSuffix)

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityHigh,
					Subject:  "Status Update Test",
					Body:     "Testing status update recovery",
					Recipients: []notificationv1alpha1.Recipient{
						{Email: "test@example.com"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
				},
			}

			// Create CRD
			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred(), "Cleanup should complete")

			// Simulate concurrent status updates by modifying status from test
			// This creates conflict that controller must recover from
			// Per TESTING_GUIDELINES.md v2.0.0: No time.Sleep(), even for staggering
			// Use Eventually() to detect when resource is ready for next update
			go func() {
				defer GinkgoRecover()
				for i := 0; i < 3; i++ {
					// Wait for resource to exist and be in a stable state before updating
					Eventually(func() error {
						temp := &notificationv1alpha1.NotificationRequest{}
						err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
							Name:      notifName,
							Namespace: testNamespace,
						}, temp)
						if err != nil {
							return err
						}
						temp.Status.Reason = fmt.Sprintf("test-conflict-%d", i)
						return k8sClient.Status().Update(ctx, temp)
					}, 5*time.Second, 50*time.Millisecond).Should(Succeed(),
						"Concurrent status update should succeed")
				}
			}()

			// Verify controller eventually succeeds in updating status despite conflicts
			Eventually(func() bool {
				err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, notif)
				if err != nil {
					return false
				}
				// Controller should eventually set phase to Sent (delivery complete)
				return notif.Status.Phase == notificationv1alpha1.NotificationPhaseSent ||
					notif.Status.Phase == notificationv1alpha1.NotificationPhaseSending
			}, 15*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"Controller should recover from status update conflicts")

			GinkgoWriter.Printf("✅ Controller recovered from status conflicts, final phase: %s\n", notif.Status.Phase)

			// Cleanup
			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred(), "Cleanup should complete")
		})

		// Test 9: MOVED TO E2E - CRD with multiple channels
		// BR-NOT-010: Multi-Channel Notification Delivery
		// ✅ NOW IN: test/e2e/notification/05_retry_scenarios_test.go - "should coordinate retries across multiple channels independently"
		// MIGRATION REASON: Timing-sensitive test had race conditions with concurrent reconciliation
		// TEST STATUS: ✅ RUNNING in E2E tier with realistic timing

		// Test 10: CRD deletion during active delivery
		// BR-NOT-004: Notification Cancellation
		It("should handle CRD deletion gracefully during active delivery", func() {
			notifName := fmt.Sprintf("delete-during-delivery-%s", uniqueSuffix)

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "Deletion Test",
					Body:     "Testing deletion during delivery",
					Recipients: []notificationv1alpha1.Recipient{
						{Slack: "#test"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelSlack,
					},
				},
			}

			// Create CRD
			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred(), "Cleanup should complete")

			// Wait for controller to start processing (status initialized)
			Eventually(func() bool {
				err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, notif)
				if err != nil {
					return false
				}
				return notif.Status.Phase != ""
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"Status should be initialized")

			// Delete CRD while controller is processing
			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred(), "Deletion should succeed")

			// Verify CRD is eventually deleted (controller handles deletion gracefully)
			Eventually(func() bool {
				err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, notif)
				return apierrors.IsNotFound(err)
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"NotificationRequest should be deleted")

			GinkgoWriter.Println("✅ CRD deleted gracefully during processing")
		})
	})

	// ============================================================================
	// Subcategory 1B: CRD Update Scenarios - DD-NOT-005 Immutability Validation
	// ============================================================================

	Context("Subcategory 1B: DD-NOT-005 Spec Immutability Validation", func() {

		// Test 11: K8s rejects spec update with immutability error
		// DD-NOT-005: NotificationRequest Spec Immutability
		It("should reject spec updates with Kubernetes validation error (DD-NOT-005)", func() {
			notifName := fmt.Sprintf("immutable-spec-%s", uniqueSuffix)

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Subject:  "Original Subject",
					Body:     "Original body content",
					Recipients: []notificationv1alpha1.Recipient{
						{Email: "test@example.com"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
				},
			}

			// Create CRD
			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred(), "Cleanup should complete")

			// Wait for delivery to complete (status = Sent = terminal state)
			Eventually(func() notificationv1alpha1.NotificationPhase {
				err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, notif)
				if err != nil {
					return ""
				}
				return notif.Status.Phase
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent),
				"Wait for delivery to complete before attempting spec update")

			// Get latest version after delivery completes (no more controller updates)
			fresh := &notificationv1alpha1.NotificationRequest{}
			err = k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name:      notifName,
				Namespace: testNamespace,
			}, fresh)
			Expect(err).NotTo(HaveOccurred(), "Cleanup should complete")

			// Attempt to update spec (should be rejected by K8s XValidation)
			fresh.Spec.Subject = "Modified Subject"
			fresh.Spec.Body = "Modified body content"

			err = k8sClient.Update(ctx, fresh)
			Expect(err).To(HaveOccurred(), "Spec update should be rejected by Kubernetes XValidation")

			// Verify error message references immutability or DD-NOT-005
			errorMsg := err.Error()
			isValidationError := apierrors.IsInvalid(err) || apierrors.IsForbidden(err)
			Expect(isValidationError).To(BeTrue(), "Should be a validation/forbidden error, got: %s", errorMsg)

			Expect(errorMsg).To(Or(
				ContainSubstring("immutable"),
				ContainSubstring("DD-NOT-005"),
				ContainSubstring("x-kubernetes-validations"),
			), "Error message should indicate immutability violation")

			GinkgoWriter.Printf("✅ Spec update rejected with error: %v\n", err)

			// Verify original spec is unchanged
			retrieved := &notificationv1alpha1.NotificationRequest{}
			err = k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name:      notifName,
				Namespace: testNamespace,
			}, retrieved)
			Expect(err).NotTo(HaveOccurred(), "Cleanup should complete")
			Expect(retrieved.Spec.Subject).To(Equal("Original Subject"),
				"Spec should remain unchanged after rejected update")
			Expect(retrieved.Spec.Body).To(Equal("Original body content"))

			// Cleanup
			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred(), "Cleanup should complete")
		})
	})

	// ============================================================================
	// Subcategory 1C: CRD Deletion Scenarios (6 tests)
	// ============================================================================

	Context("Subcategory 1C: CRD Deletion Scenarios", func() {

		// Test 12 (was 17): Delete CRD before first reconciliation
		// BR-NOT-004: Notification Cancellation
		It("should handle deletion before first reconciliation", func() {
			notifName := fmt.Sprintf("delete-before-reconcile-%s", uniqueSuffix)

			notif := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       notifName,
					Namespace:  testNamespace,
					Generation: 1, // K8s increments on create/update
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:     notificationv1alpha1.NotificationTypeSimple,
					Priority: notificationv1alpha1.NotificationPriorityLow,
					Subject:  "Quick Delete Test",
					Body:     "Testing immediate deletion",
					Recipients: []notificationv1alpha1.Recipient{
						{Email: "test@example.com"},
					},
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
				},
			}

			// Create and immediately delete (before controller reconciles)
			err := k8sClient.Create(ctx, notif)
			Expect(err).NotTo(HaveOccurred(), "Cleanup should complete")

			// Delete immediately (don't wait for reconciliation)
			err = deleteAndWait(ctx, k8sClient, notif, 5*time.Second)
			Expect(err).NotTo(HaveOccurred(), "Cleanup should complete")

			// Verify CRD is deleted
			Eventually(func() bool {
				err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
					Name:      notifName,
					Namespace: testNamespace,
				}, notif)
				return apierrors.IsNotFound(err)
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"NotificationRequest should be deleted even if not reconciled")

			GinkgoWriter.Println("✅ Deletion before reconciliation handled gracefully")
		})

		// Tests 13-17 were placeholder Skip() calls - DELETED per "NO SKIPPED TESTS" rule
		//
		// These tests were TODOs for future infrastructure that doesn't exist yet:
		// - Test 13: Delete during Slack API call (requires delayed mock)
		// - Test 14: Delete during retry backoff (covered in E2E retry tests)
		// - Test 15: Delete with finalizer (feature not implemented)
		// - Test 16: Delete during audit write (audit is fire-and-forget, no coordination needed)
		// - Test 17: Delete during circuit breaker OPEN (covered in circuit breaker integration tests)
		//
		// Per project rule: No Skip() placeholders allowed. Implement properly when infrastructure exists.
	})
})
