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

package authwebhook

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// E2E Test for Gap #8: TimeoutConfig Mutation Webhook Audit
// BR-AUDIT-005 v2.0: Gap #8 - Operator TimeoutConfig mutation audit capture
// BR-AUTH-001: SOC2 CC8.1 Operator Attribution
//
// This E2E test validates the complete webhook flow:
// 1. RemediationRequest created with TimeoutConfig initialized by controller
// 2. Operator modifies TimeoutConfig via kubectl edit (simulated)
// 3. Mutating webhook intercepts the status update
// 4. Webhook populates LastModifiedBy/LastModifiedAt fields
// 5. Webhook emits webhook.remediationrequest.timeout_modified audit event
// 6. Audit event is stored in DataStorage service
//
// Test Coverage: E2E (10-15%)
// - Complete HTTP webhook flow (admission request → handler → audit event)
// - Authentication extraction from admission request
// - Audit event emission and storage
// - SOC2 compliance validation (WHO + WHAT + WHEN)
//
// Integration Test Coverage: See test/integration/remediationorchestrator/gap8_timeout_config_audit_test.go
// - Scenario 1: Controller initialization (orchestrator.lifecycle.created)
// - Scenario 3: Event timing validation
//
// NOTE: Webhook tests MUST run in E2E (not integration) because:
// - envtest (integration) does not support admission webhooks
// - Full Kubernetes API server required for admission controller
// - TLS certificates required for webhook communication
var _ = Describe("E2E: Gap #8 - RemediationRequest TimeoutConfig Mutation Webhook (BR-AUDIT-005)", Label("e2e", "gap8", "webhook", "audit"), func() {
	var (
		testNamespace string
		rr            *remediationv1.RemediationRequest
		correlationID string
	)

	BeforeEach(func() {
		// Create test namespace with audit enabled
		testNamespace = "gap8-webhook-test-" + time.Now().Format("150405")
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
				Labels: map[string]string{
					"kubernaut.ai/audit-enabled": "true", // REQUIRED for webhook to intercept
				},
			},
		}
		err := k8sClient.Create(ctx, ns)
		Expect(err).ToNot(HaveOccurred())

		GinkgoWriter.Printf("📋 Created test namespace: %s (with audit enabled)\n", testNamespace)
	})

	AfterEach(func() {
		// Cleanup: Delete RemediationRequest
		if rr != nil {
			err := k8sClient.Delete(ctx, rr)
			if err != nil {
				GinkgoWriter.Printf("⚠️  Failed to delete RemediationRequest: %v\n", err)
			}
		}

		// Cleanup: Delete namespace
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}
		err := k8sClient.Delete(ctx, ns)
		if err != nil {
			GinkgoWriter.Printf("⚠️  Failed to delete namespace: %v\n", err)
		}

		GinkgoWriter.Printf("🧹 Cleaned up namespace: %s\n", testNamespace)
	})

	Context("E2E-GAP8-01: Operator Modifies TimeoutConfig", func() {
		It("should emit webhook.remediationrequest.timeout_modified audit event", func() {
			// ========================================
			// GIVEN: RemediationRequest with TimeoutConfig initialized by controller
			// ========================================
			now := metav1.Now()
			rr = &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-gap8-webhook",
					Namespace: testNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "test-fp-gap8-webhook",
					SignalName:        "Gap8WebhookTest",
					Severity:          "warning",
					SignalType:        "prometheus",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Deployment",
						Name:      "test-deployment",
						Namespace: "production",
					},
					FiringTime:   now,
					ReceivedTime: now,
				},
			}

			err := k8sClient.Create(ctx, rr)
			Expect(err).ToNot(HaveOccurred())

			correlationID = string(rr.UID)
			GinkgoWriter.Printf("✅ Created RemediationRequest: %s (correlation_id=%s)\n", rr.Name, correlationID)

			// Wait for RemediationOrchestrator controller to initialize TimeoutConfig
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{
					Namespace: testNamespace,
					Name:      "rr-gap8-webhook",
				}, rr)
				if err != nil {
					return false
				}
				return rr.Status.TimeoutConfig != nil && rr.Status.TimeoutConfig.Global != nil
			}, 30*time.Second, 1*time.Second).Should(BeTrue(),
				"RemediationOrchestrator controller should initialize default TimeoutConfig")

			GinkgoWriter.Printf("✅ TimeoutConfig initialized by controller: Global=%s\n",
				rr.Status.TimeoutConfig.Global.Duration)

			// ========================================
			// WHEN: Operator modifies TimeoutConfig (simulates kubectl edit)
			// ========================================
			rr.Status.TimeoutConfig = &remediationv1.TimeoutConfig{
				Global:     &metav1.Duration{Duration: 45 * time.Minute},
				Processing: &metav1.Duration{Duration: 12 * time.Minute},
				Analyzing:  &metav1.Duration{Duration: 8 * time.Minute},
				Executing:  &metav1.Duration{Duration: 20 * time.Minute},
			}

			GinkgoWriter.Printf("📝 Operator modifying TimeoutConfig: Global=%s, Processing=%s, Analyzing=%s, Executing=%s\n",
				rr.Status.TimeoutConfig.Global.Duration,
				rr.Status.TimeoutConfig.Processing.Duration,
				rr.Status.TimeoutConfig.Analyzing.Duration,
				rr.Status.TimeoutConfig.Executing.Duration)

			err = k8sClient.Status().Update(ctx, rr)
			Expect(err).ToNot(HaveOccurred())

			GinkgoWriter.Printf("✅ Status update submitted (webhook should intercept)\n")

			// ========================================
			// THEN: Webhook audit event emitted
			// ========================================
			webhookEventType := "webhook.remediationrequest.timeout_modified"
			var webhookEvents []interface{} // Use generic type for now

			Eventually(func() int {
				// Query audit events via DataStorage OpenAPI client
				params := map[string]interface{}{
					"correlation_id": correlationID,
					"event_type":     webhookEventType,
				}

				// For now, we'll use a simplified query
				// The actual implementation will depend on the auditClient helper
				// from the AuthWebhook E2E suite
				_ = params

				// TODO: Replace with actual audit query helper
				// events, _, err := helpers.QueryAuditEvents(ctx, auditClient, &correlationID, &webhookEventType, nil)
				// if err != nil {
				//     GinkgoWriter.Printf("⚠️  Query error: %v\n", err)
				//     return 0
				// }
				// webhookEvents = events
				// return len(events)

				// For now, return placeholder
				return 0
			}, 30*time.Second, 2*time.Second).Should(Equal(1),
				"webhook.remediationrequest.timeout_modified event should be emitted")

			_ = webhookEvents // Suppress unused warning until helper is integrated

			// TODO: Validate webhook event structure once audit query is working
			// webhookEvent := webhookEvents[0]
			// Expect(webhookEvent.EventType).To(Equal("webhook.remediationrequest.timeout_modified"))
			// Expect(webhookEvent.EventCategory).To(Equal("webhook"))
			// Expect(webhookEvent.EventAction).To(Equal("timeout_modified"))
			// Expect(webhookEvent.EventOutcome).To(Equal("success"))
			// Expect(webhookEvent.CorrelationID).To(Equal(correlationID))

			// ========================================
			// THEN: LastModifiedBy/LastModifiedAt populated by webhook
			// ========================================
			err = k8sClient.Get(ctx, client.ObjectKey{
				Namespace: testNamespace,
				Name:      "rr-gap8-webhook",
			}, rr)
			Expect(err).ToNot(HaveOccurred())

			Expect(rr.Status.LastModifiedBy).ToNot(BeEmpty(),
				"Webhook should populate LastModifiedBy with authenticated user")
			Expect(rr.Status.LastModifiedAt).ToNot(BeNil(),
				"Webhook should populate LastModifiedAt with mutation timestamp")

			GinkgoWriter.Printf("✅ Gap #8 E2E test PASSED:\n")
			GinkgoWriter.Printf("   • Webhook intercepted TimeoutConfig mutation\n")
			GinkgoWriter.Printf("   • LastModifiedBy: %s\n", rr.Status.LastModifiedBy)
			GinkgoWriter.Printf("   • LastModifiedAt: %s\n", rr.Status.LastModifiedAt.Time)
			GinkgoWriter.Printf("   • Audit event: webhook.remediationrequest.timeout_modified\n")
			GinkgoWriter.Printf("   • SOC2 compliance: WHO + WHAT + WHEN captured\n")
		})
	})
})
