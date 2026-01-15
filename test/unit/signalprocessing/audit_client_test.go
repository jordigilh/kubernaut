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

// Package signalprocessing_test contains unit tests for the SignalProcessing audit client.
//
// BR Coverage:
//   - BR-SP-090: Categorization Audit Trail
//
// Test Categories:
//   - AC-HP-XX: Happy Path tests
//   - AC-EC-XX: Edge Case tests
//   - AC-ER-XX: Error Handling tests
//
// Per 03-testing-strategy.mdc: Unit tests mock external dependencies only.
// The pkg/audit.AuditStore is mocked as it's an external I/O boundary.
package signalprocessing

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/audit"
)

// MockAuditStore implements pkg/audit.AuditStore for testing
// Updated for DD-AUDIT-002 V2.0.1 to use OpenAPI types directly
type MockAuditStore struct {
	StoredEvents []*ogenclient.AuditEventRequest
	StoreError   error
}

func (m *MockAuditStore) StoreAudit(ctx context.Context, event *ogenclient.AuditEventRequest) error {
	if m.StoreError != nil {
		return m.StoreError
	}
	m.StoredEvents = append(m.StoredEvents, event)
	return nil
}

func (m *MockAuditStore) Flush(ctx context.Context) error {
	// Mock: no-op - events already stored synchronously
	return nil
}

func (m *MockAuditStore) Close() error {
	return nil
}

func (m *MockAuditStore) Reset() {
	m.StoredEvents = nil
	m.StoreError = nil
}

var _ = Describe("SignalProcessing AuditClient", func() {
	var (
		ctx         context.Context
		mockStore   *MockAuditStore
		auditClient *audit.AuditClient
		logger      = zap.New(zap.WriteTo(GinkgoWriter))
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockStore = &MockAuditStore{}
		auditClient = audit.NewAuditClient(mockStore, logger)
	})

	AfterEach(func() {
		mockStore.Reset()
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// BR-SP-090: Categorization Audit Trail - Happy Path
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Describe("BR-SP-090: RecordSignalProcessed", func() {
		Context("Happy Path", func() {
			// AC-HP-01: Record completed signal processing
			It("AC-HP-01: should record completed signal with all classification data", func() {
				sp := createTestSignalProcessing("completed")
				sp.Status.Phase = signalprocessingv1alpha1.PhaseCompleted
				sp.Status.EnvironmentClassification = &signalprocessingv1alpha1.EnvironmentClassification{
					Environment: "production",
					Source:      "namespace-labels",
				}
				sp.Status.PriorityAssignment = &signalprocessingv1alpha1.PriorityAssignment{
					Priority: "P0",
					Source:   "rego-policy",
				}
				sp.Status.BusinessClassification = &signalprocessingv1alpha1.BusinessClassification{
					Criticality:    "high",
					SLARequirement: "gold",
				}

				auditClient.RecordSignalProcessed(ctx, sp)

				Expect(mockStore.StoredEvents).To(HaveLen(1))
				event := mockStore.StoredEvents[0]
				Expect(event.EventType).To(Equal("signalprocessing.signal.processed"))
				Expect(event.EventCategory).To(Equal(ogenclient.AuditEventRequestEventCategory("signalprocessing")))
				Expect(event.EventAction).To(Equal("processed"))
				Expect(event.EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcome("success")))
				Expect(event.ResourceType.Value).To(Equal("SignalProcessing"))
				Expect(event.ResourceID.Value).To(Equal("completed"))
			})

			// AC-HP-02: Record failed signal processing
			It("AC-HP-02: should record failed signal with failure outcome", func() {
				sp := createTestSignalProcessing("failed")
				sp.Status.Phase = signalprocessingv1alpha1.PhaseFailed
				sp.Status.Error = "enrichment timeout"

				auditClient.RecordSignalProcessed(ctx, sp)

				Expect(mockStore.StoredEvents).To(HaveLen(1))
				event := mockStore.StoredEvents[0]
				Expect(event.EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcome("failure")))
			})
		})
	})

	Describe("BR-SP-090: RecordPhaseTransition", func() {
		Context("Happy Path", func() {
			// AC-HP-03: Record phase transition
			It("AC-HP-03: should record phase transition with from/to phases", func() {
				sp := createTestSignalProcessing("transition-test")

				auditClient.RecordPhaseTransition(ctx, sp, string(signalprocessingv1alpha1.PhasePending), string(signalprocessingv1alpha1.PhaseEnriching))

				Expect(mockStore.StoredEvents).To(HaveLen(1))
				event := mockStore.StoredEvents[0]
				Expect(event.EventType).To(Equal("signalprocessing.phase.transition"))
				Expect(event.EventAction).To(Equal("phase_transition"))
			})
		})
	})

	Describe("BR-SP-090: RecordClassificationDecision", func() {
		Context("Happy Path", func() {
			// AC-HP-04: Record classification decision
			It("AC-HP-04: should record classification with all decisions", func() {
				sp := createTestSignalProcessing("classification-test")
				sp.Status.EnvironmentClassification = &signalprocessingv1alpha1.EnvironmentClassification{
					Environment: "staging",
					Source:      "namespace-labels",
				}
				sp.Status.PriorityAssignment = &signalprocessingv1alpha1.PriorityAssignment{
					Priority: "P2",
					Source:   "rego-policy",
				}
				sp.Status.BusinessClassification = &signalprocessingv1alpha1.BusinessClassification{
					Criticality:    "medium",
					SLARequirement: "silver",
				}

				auditClient.RecordClassificationDecision(ctx, sp, 125) // 125ms duration

				Expect(mockStore.StoredEvents).To(HaveLen(1))
				event := mockStore.StoredEvents[0]
				Expect(event.EventType).To(Equal("signalprocessing.classification.decision"))
				Expect(event.EventAction).To(Equal("classification"))
			})
		})
	})

	Describe("BR-SP-090: RecordEnrichmentComplete", func() {
		Context("Happy Path", func() {
			// AC-HP-05: Record enrichment completion
			It("AC-HP-05: should record enrichment with duration and context indicators", func() {
				sp := createTestSignalProcessing("enrichment-test")
				sp.Status.KubernetesContext = &signalprocessingv1alpha1.KubernetesContext{
					Namespace: &signalprocessingv1alpha1.NamespaceContext{
						Name: "test-ns",
					},
					Pod: &signalprocessingv1alpha1.PodDetails{
						Labels: map[string]string{"app": "test"},
					},
					OwnerChain: []signalprocessingv1alpha1.OwnerChainEntry{
						{Kind: "ReplicaSet", Name: "test-rs"},
					},
				}

				auditClient.RecordEnrichmentComplete(ctx, sp, 150)

				Expect(mockStore.StoredEvents).To(HaveLen(1))
				event := mockStore.StoredEvents[0]
				Expect(event.EventType).To(Equal("signalprocessing.enrichment.completed"))
				Expect(event.DurationMs.IsSet()).To(BeTrue())
				Expect(event.DurationMs.Value).To(Equal(150))
			})
		})
	})

	Describe("BR-SP-090: RecordError", func() {
		Context("Happy Path", func() {
			// AC-HP-06: Record error with phase and error message
			It("AC-HP-06: should record error with phase and error message", func() {
				sp := createTestSignalProcessing("error-test")
				testErr := errors.New("connection timeout")

				auditClient.RecordError(ctx, sp, "Enriching", testErr)

				Expect(mockStore.StoredEvents).To(HaveLen(1))
				event := mockStore.StoredEvents[0]
				Expect(event.EventType).To(Equal("signalprocessing.error.occurred"))
				Expect(event.EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcome("failure")))
				// Error message is stored in structured EventData (DD-AUDIT-004 V2.2)
				// EventData is a discriminated union, access via GetSignalProcessingAuditPayload
				payload, ok := event.EventData.GetSignalProcessingAuditPayload()
				Expect(ok).To(BeTrue(), "EventData should be SignalProcessingAuditPayload")
				Expect(payload.Error.IsSet()).To(BeTrue())
				Expect(payload.Error.Value).To(Equal("connection timeout"))
				Expect(payload.Phase).To(Equal(ogenclient.SignalProcessingAuditPayloadPhaseEnriching))
			})
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// Edge Cases
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Describe("Edge Cases", func() {
		// AC-EC-01: Nil classifications
		It("AC-EC-01: should handle nil classification fields gracefully", func() {
			sp := createTestSignalProcessing("nil-test")
			// No classifications set

			auditClient.RecordSignalProcessed(ctx, sp)

			Expect(mockStore.StoredEvents).To(HaveLen(1))
			// Should not panic
		})

		// AC-EC-02: Empty owner chain
		It("AC-EC-02: should handle empty owner chain", func() {
			sp := createTestSignalProcessing("empty-chain")
			sp.Status.KubernetesContext = &signalprocessingv1alpha1.KubernetesContext{
				OwnerChain: []signalprocessingv1alpha1.OwnerChainEntry{},
			}

			auditClient.RecordEnrichmentComplete(ctx, sp, 100)

			Expect(mockStore.StoredEvents).To(HaveLen(1))
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// Error Handling (Graceful Degradation)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Describe("Error Handling", func() {
		// AC-ER-01: Store error should not panic
		It("AC-ER-01: should handle store error gracefully (fire-and-forget)", func() {
			mockStore.StoreError = errors.New("database connection failed")
			sp := createTestSignalProcessing("error-handling")

			// Should not panic - fire-and-forget pattern
			Expect(func() {
				auditClient.RecordSignalProcessed(ctx, sp)
			}).ToNot(Panic())

			// Event was attempted but failed
			Expect(mockStore.StoredEvents).To(BeEmpty())
		})

		// AC-ER-02: Multiple audit calls with intermittent failures
		It("AC-ER-02: should continue after intermittent store failures", func() {
			sp := createTestSignalProcessing("intermittent")

			// First call succeeds
			auditClient.RecordSignalProcessed(ctx, sp)
			Expect(mockStore.StoredEvents).To(HaveLen(1))

			// Second call fails
			mockStore.StoreError = errors.New("temporary failure")
			auditClient.RecordPhaseTransition(ctx, sp, "a", "b")
			Expect(mockStore.StoredEvents).To(HaveLen(1)) // Still 1

			// Third call succeeds
			mockStore.StoreError = nil
			auditClient.RecordClassificationDecision(ctx, sp, 100) // 100ms duration
			Expect(mockStore.StoredEvents).To(HaveLen(2))
		})
	})
})

// createTestSignalProcessing creates a minimal SignalProcessing for testing
func createTestSignalProcessing(name string) *signalprocessingv1alpha1.SignalProcessing {
	return &signalprocessingv1alpha1.SignalProcessing{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test-namespace",
		},
		Spec: signalprocessingv1alpha1.SignalProcessingSpec{
			RemediationRequestRef: signalprocessingv1alpha1.ObjectReference{
				Name:      "remediation-" + name,
				Namespace: "test-namespace",
			},
			Signal: signalprocessingv1alpha1.SignalData{
				Fingerprint: "test-fingerprint-abc123def456abc123def456abc123def456abc123d",
				Name:        "TestSignal",
				Severity:    "warning",
				Type:        "prometheus",
				TargetType:  "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "test-pod",
					Namespace: "test-namespace",
				},
			},
		},
		Status: signalprocessingv1alpha1.SignalProcessingStatus{
			Phase: signalprocessingv1alpha1.PhasePending,
		},
	}
}
