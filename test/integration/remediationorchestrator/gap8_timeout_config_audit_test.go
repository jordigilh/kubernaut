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

package remediationorchestrator

import (
	"context"
	"time"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Gap #8 Integration Tests - BR-AUDIT-005 Gap #8 (TimeoutConfig Audit)
// Business Requirement: Capture TimeoutConfig in audit events for RR reconstruction
//
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - Unit tests (70%+): Business logic with external mocks only
// - Integration tests (>50%): Infrastructure interaction, microservices coordination
// - E2E tests (10-15%): Complete workflow validation
//
// This test validates:
// - orchestrator.lifecycle.created event emission on RR creation
// - TimeoutConfig payload capture (both nil and custom configs)
// - Event correlation for SOC2 compliance (BR-AUTH-001, CC8.1)
var _ = Describe("Gap #8: TimeoutConfig Audit Capture", func() {
	var (
		ctx      context.Context
		dsClient *ogenclient.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		// Create Data Storage OpenAPI client using shared dataStorageBaseURL from suite_test.go
		// Per DD-TEST-001 v2.2: Avoids brittle hardcoded ports
		var err error
		dsClient, err = ogenclient.NewClient(dataStorageBaseURL)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Data Storage OpenAPI client")
	})

	// ========================================
	// SCENARIO 1: Default TimeoutConfig (Controller Defaults)
	// Business Outcome: RR with nil TimeoutConfig gets controller defaults captured in audit
	// ========================================
	Context("Scenario 1: Controller Initializes Default TimeoutConfig", func() {
		It("should emit orchestrator.lifecycle.created with default timeout_config", func() {
			// Given: RemediationRequest CRD without custom TimeoutConfig
			testNamespace := createTestNamespace("gap8-defaults")
			defer func() {
				// Async namespace cleanup
				go func() {
					deleteTestNamespace(testNamespace)
				}()
			}()

			fingerprint := GenerateTestFingerprint(testNamespace, "gap8-defaults")
			now := metav1.Now()
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-gap8-defaults",
					Namespace: testNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: fingerprint,
					SignalName:        "Gap8TestSignal",
					Severity:          "warning",
					SignalType:        "prometheus",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Pod",
						Name:      "test-pod",
						Namespace: "default",
					},
					FiringTime:   now,
					ReceivedTime: now,
				},
				// Note: Status.TimeoutConfig is nil - controller should initialize with defaults
			}

			// When: Create RemediationRequest CRD (controller will initialize defaults)
			err := k8sClient.Create(ctx, rr)
			Expect(err).ToNot(HaveOccurred())

			correlationID := string(rr.UID)

			// Then: orchestrator.lifecycle.created event should be emitted
			// BR-AUDIT-005 Gap #8: Must capture TimeoutConfig for RR reconstruction
			var events []ogenclient.AuditEvent
			eventType := "orchestrator.lifecycle.created"
			Eventually(func() []ogenclient.AuditEvent {
				var err error
				events, _, err = helpers.QueryAuditEvents(
					ctx,
					dsClient,
					&correlationID,
					&eventType,
					nil,
				)
				if err != nil {
					return nil
				}
				return events
			}, 10*time.Second, 500*time.Millisecond).Should(HaveLen(1),
				"orchestrator.lifecycle.created event should be emitted exactly once")

			event := events[0]

			// Validate event structure (ADR-034 compliance)
			Expect(event.EventType).To(Equal("orchestrator.lifecycle.created"))
			Expect(event.EventCategory).To(Equal(ogenclient.AuditEventEventCategoryOrchestration))
			Expect(event.EventAction).To(Equal("created"))
			Expect(event.EventOutcome).To(Equal(ogenclient.AuditEventEventOutcomeSuccess))
			Expect(event.CorrelationID).To(Equal(correlationID))
			Expect(event.Namespace.Value).To(Equal(testNamespace))

			// Validate TimeoutConfig payload (Gap #8 requirement)
			// Per DD-AUDIT-004: Use strongly-typed access (not type assertion)
			payload := event.EventData.RemediationOrchestratorAuditPayload
			Expect(payload.RrName).To(Equal("rr-gap8-defaults"))
			Expect(payload.Namespace).To(Equal(testNamespace))

			// Gap #8 Critical Validation: TimeoutConfig must be captured
			Expect(payload.TimeoutConfig.IsSet()).To(BeTrue(),
				"TimeoutConfig should be captured in audit event (Gap #8)")

			timeoutConfig := payload.TimeoutConfig.Value
			// Controller defaults should be initialized (30m, 10m, 5m, 15m per controller config)
			Expect(timeoutConfig.Global.IsSet()).To(BeTrue())
			Expect(timeoutConfig.Processing.IsSet()).To(BeTrue())
			Expect(timeoutConfig.Analyzing.IsSet()).To(BeTrue())
			Expect(timeoutConfig.Executing.IsSet()).To(BeTrue())

			// Validate RR status was initialized with TimeoutConfig
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: testNamespace,
					Name:      "rr-gap8-defaults",
				}, rr)
				if err != nil {
					return false
				}
				return rr.Status.TimeoutConfig != nil
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"RR status.timeoutConfig should be initialized by controller")

			Expect(rr.Status.TimeoutConfig.Global).ToNot(BeNil())
			Expect(rr.Status.TimeoutConfig.Processing).ToNot(BeNil())
			Expect(rr.Status.TimeoutConfig.Analyzing).ToNot(BeNil())
			Expect(rr.Status.TimeoutConfig.Executing).ToNot(BeNil())
		})
	})

	// ========================================
	// SCENARIO 2: Custom TimeoutConfig (Operator Mutation via Webhook)
	// Business Outcome: Operator-modified TimeoutConfig triggers webhook audit
	// STATUS: ⏸️ DEFERRED - Requires webhook server infrastructure (production feature)
	// Phase 3 implementation requires: webhook handler + MutatingWebhookConfiguration
	// ========================================
	Context("Scenario 2: Operator Modifies TimeoutConfig", func() {
		It("should emit webhook.remediationrequest.timeout_modified on operator mutation", func() {
			// Given: RemediationRequest CRD created with defaults
			testNamespace := createTestNamespace("gap8-webhook")
			defer func() {
				go func() {
					deleteTestNamespace(testNamespace)
				}()
			}()

			fingerprint := GenerateTestFingerprint(testNamespace, "gap8-webhook")
			now := metav1.Now()
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-gap8-webhook",
					Namespace: testNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: fingerprint,
					SignalName:        "Gap8WebhookSignal",
					Severity:          "critical",
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

			// When: Create RR (gets default TimeoutConfig from controller)
			err := k8sClient.Create(ctx, rr)
			Expect(err).ToNot(HaveOccurred())

			correlationID := string(rr.UID)

			// Wait for controller to initialize TimeoutConfig
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: testNamespace,
					Name:      "rr-gap8-webhook",
				}, rr)
				if err != nil {
					return false
				}
				return rr.Status.TimeoutConfig != nil &&
					rr.Status.TimeoutConfig.Global != nil
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"Controller should initialize default TimeoutConfig")

			// When: Operator modifies TimeoutConfig (simulates kubectl edit)
			// This triggers the webhook which should emit audit event
			rr.Status.TimeoutConfig = &remediationv1.TimeoutConfig{
				Global:     &metav1.Duration{Duration: 45 * time.Minute},
				Processing: &metav1.Duration{Duration: 12 * time.Minute},
				Analyzing:  &metav1.Duration{Duration: 8 * time.Minute},
				Executing:  &metav1.Duration{Duration: 20 * time.Minute},
			}
			// TODO: Populate LastModifiedBy/At (webhook responsibility)
			// For now, update directly to simulate operator action
			err = k8sClient.Status().Update(ctx, rr)
			Expect(err).ToNot(HaveOccurred())

			// Then: webhook.remediationrequest.timeout_modified event should be emitted
			var webhookEvents []ogenclient.AuditEvent
			webhookEventType := "webhook.remediationrequest.timeout_modified"
			Eventually(func() []ogenclient.AuditEvent {
				var err error
				webhookEvents, _, err = helpers.QueryAuditEvents(
					ctx,
					dsClient,
					&correlationID,
					&webhookEventType,
					nil,
				)
				if err != nil {
					return nil
				}
				return webhookEvents
			}, 10*time.Second, 500*time.Millisecond).Should(HaveLen(1),
				"webhook.remediationrequest.timeout_modified event should be emitted on mutation")

			webhookEvent := webhookEvents[0]

			// Validate webhook event structure (ADR-034 compliance)
			Expect(webhookEvent.EventType).To(Equal("webhook.remediationrequest.timeout_modified"))
			Expect(webhookEvent.EventCategory).To(Equal(ogenclient.AuditEventEventCategoryWebhook))
			Expect(webhookEvent.EventAction).To(Equal("timeout_modified"))
			Expect(webhookEvent.EventOutcome).To(Equal(ogenclient.AuditEventEventOutcomeSuccess))
			Expect(webhookEvent.CorrelationID).To(Equal(correlationID))

			// Validate webhook captured new TimeoutConfig values
			// TODO: Validate webhook audit payload structure
			// (depends on OpenAPI schema definition)
		})
	})

	// ========================================
	// SCENARIO 3: Event Ordering Validation
	// Business Outcome: lifecycle.created is emitted AFTER TimeoutConfig initialization
	// ========================================
	Context("Scenario 3: Event Timing Validation", func() {
		It("should emit lifecycle.created AFTER status.timeoutConfig initialization", func() {
			// Given: New RemediationRequest
			testNamespace := createTestNamespace("gap8-timing")
			defer func() {
				go func() {
					deleteTestNamespace(testNamespace)
				}()
			}()

			fingerprint := GenerateTestFingerprint(testNamespace, "gap8-timing")
			now := metav1.Now()
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-gap8-timing",
					Namespace: testNamespace,
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: fingerprint,
					SignalName:        "Gap8TimingSignal",
					Severity:          "warning",
					SignalType:        "prometheus",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Service",
						Name:      "test-svc",
						Namespace: "default",
					},
					FiringTime:   now,
					ReceivedTime: now,
				},
			}

			// When: Create RR
			err := k8sClient.Create(ctx, rr)
			Expect(err).ToNot(HaveOccurred())

			correlationID := string(rr.UID)

			// Then: Audit event should reflect initialized TimeoutConfig
			// (not nil, proving event was emitted AFTER initialization)
			eventType := "orchestrator.lifecycle.created"
			Eventually(func() bool {
				events, _, err := helpers.QueryAuditEvents(
					ctx,
					dsClient,
					&correlationID,
					&eventType,
					nil,
				)
				if err != nil || len(events) != 1 {
					return false
				}

				// Per DD-AUDIT-004: Use strongly-typed access
				payload := events[0].EventData.RemediationOrchestratorAuditPayload

				// Critical: TimeoutConfig should be non-nil, proving event was emitted AFTER init
				return payload.TimeoutConfig.IsSet()
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"Event should capture initialized TimeoutConfig (Gap #8 timing requirement)")
		})
	})
})
