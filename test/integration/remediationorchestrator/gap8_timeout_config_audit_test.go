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
	roaudit "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/audit"
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
		// DD-AUTH-014: Use authenticated OpenAPI client from shared setup
		// dsClients is created in SynchronizedBeforeSuite with ServiceAccount token
		// Creating a new client here would bypass authentication!
		dsClient = dsClients.OpenAPIClient
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
					SignalType:        "alert",
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

	// DD-AUDIT-CORRELATION-002: Use rr.Name (not rr.UID) for audit event queries
// Per universal standard: RemediationRequest.Name is the correlation ID for all services
correlationID := rr.Name

	// Wait briefly for controller to emit event
	time.Sleep(500 * time.Millisecond)

	// Then: orchestrator.lifecycle.created event should be emitted
	// BR-AUDIT-005 Gap #8: Must capture TimeoutConfig for RR reconstruction
	// RCA FIX: Flush INSIDE Eventually to handle async HTTP write to DataStorage
	var events []ogenclient.AuditEvent
	eventType := roaudit.EventTypeLifecycleCreated
	Eventually(func() []ogenclient.AuditEvent {
			// Flush buffer on each retry (handles async HTTP POST timing)
			_ = auditStore.Flush(ctx)

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
			Expect(event.EventType).To(Equal(roaudit.EventTypeLifecycleCreated))
			Expect(event.EventCategory).To(Equal(ogenclient.AuditEventEventCategoryOrchestration))
			Expect(event.EventAction).To(Equal(roaudit.ActionCreated))
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
				err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
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
	// SCENARIO 2: Operator Mutation via Webhook - MOVED TO E2E
	// Business Outcome: Operator-modified TimeoutConfig triggers webhook audit
	// Location: test/e2e/authwebhook/02_gap8_remediationrequest_timeout_mutation_test.go
	// Reason: Webhooks require full Kubernetes API server with admission controller
	//         (not available in envtest used by integration tests)
	// Event: webhook.remediationrequest.timeout_modified
	// ========================================
	// NOTE: Scenario 2 removed from integration tests (January 13, 2026)
	// Webhook tests belong in E2E tier where full webhook infrastructure is available.
	// Integration tests validate business logic only (controller initialization).

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
					SignalType:        "alert",
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

	// DD-AUDIT-CORRELATION-002: Use rr.Name (not rr.UID) for audit event queries
// Per universal standard: RemediationRequest.Name is the correlation ID for all services
correlationID := rr.Name

	// Wait for controller to emit event, then flush once
	time.Sleep(500 * time.Millisecond)
	err = auditStore.Flush(ctx)
	Expect(err).ToNot(HaveOccurred())

	// Then: Audit event should reflect initialized TimeoutConfig
	// (not nil, proving event was emitted AFTER initialization)
	eventType := roaudit.EventTypeLifecycleCreated
	Eventually(func() bool {
			events, _, err := helpers.QueryAuditEvents(
				ctx,
				dsClient,
				&correlationID,
				&eventType,
				nil,
			)
			if err != nil {
				GinkgoWriter.Printf("Query error: %v\n", err)
				return false
			}
			if len(events) != 1 {
				GinkgoWriter.Printf("Found %d events (expected 1) for correlation_id=%s, event_type=%s\n",
					len(events), correlationID, eventType)
				return false
			}

			// Per DD-AUDIT-004: Use strongly-typed access
			payload := events[0].EventData.RemediationOrchestratorAuditPayload

			// Critical: TimeoutConfig should be non-nil, proving event was emitted AFTER init
			hasTimeoutConfig := payload.TimeoutConfig.IsSet()
			if !hasTimeoutConfig {
				GinkgoWriter.Printf("Event found but TimeoutConfig is not set\n")
			}
			return hasTimeoutConfig
		}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
			"Event should capture initialized TimeoutConfig (Gap #8 timing requirement)")
		})
	})
})
