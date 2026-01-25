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

// Package audit_test contains unit tests for AIAnalysis audit client.
//
// Business Requirements:
// - BR-AI-050: Error audit trail completeness
//
// Test Strategy:
// - Unit tests with mock audit store for RecordError functionality
// - Tests event structure, correlation ID handling, and payload construction
package audit_test

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

func TestAuditClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AIAnalysis Audit Client Suite")
}

// ========================================
// Mock Audit Store
// ========================================

// MockAuditStore implements audit.AuditStore interface for testing
type MockAuditStore struct {
	StoredEvents []*ogenclient.AuditEventRequest
	StoreError   error
	FlushError   error
}

// NewMockAuditStore creates a new mock audit store
func NewMockAuditStore() *MockAuditStore {
	return &MockAuditStore{
		StoredEvents: make([]*ogenclient.AuditEventRequest, 0),
	}
}

// StoreAudit implements audit.AuditStore interface
func (m *MockAuditStore) StoreAudit(ctx context.Context, event *ogenclient.AuditEventRequest) error {
	if m.StoreError != nil {
		return m.StoreError
	}
	m.StoredEvents = append(m.StoredEvents, event)
	return nil
}

// Flush implements audit.AuditStore interface
func (m *MockAuditStore) Flush(ctx context.Context) error {
	return m.FlushError
}

// Close implements audit.AuditStore interface
func (m *MockAuditStore) Close() error {
	return nil
}

// ========================================
// Unit Tests
// ========================================

var _ = Describe("AuditClient RecordError", func() {
	var (
		mockStore   *MockAuditStore
		auditClient *audit.AuditClient
		analysis    *aianalysisv1.AIAnalysis
		ctx         context.Context
	)

	BeforeEach(func() {
		mockStore = NewMockAuditStore()
		auditClient = audit.NewAuditClient(mockStore, logr.Discard())
		ctx = context.Background()

		analysis = &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-analysis",
				Namespace: "default",
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationRequestRef: corev1.ObjectReference{
					Name:      "test-rr",
					Namespace: "default",
				},
				RemediationID: "test-remediation-id",
			},
		}
	})

	Context("Basic error recording", func() {
		It("should emit aianalysis.error event with correct structure", func() {
			// Given: An error during Investigating phase
			testErr := errors.New("HAPI request timeout")

			// When: RecordError is called
			auditClient.RecordError(ctx, analysis, "Investigating", testErr)

			// Then: Audit store receives exactly one event
			Expect(mockStore.StoredEvents).To(HaveLen(1), "Should store exactly one audit event")
			event := mockStore.StoredEvents[0]

			// Verify event metadata
			Expect(event.EventType).To(Equal("aianalysis.error.occurred"),
				"Event type should be aianalysis.error.occurred")
			Expect(event.EventCategory).To(Equal(ogenclient.AuditEventRequestEventCategoryAnalysis),
				"Event category should be 'analysis'")
			Expect(event.EventAction).To(Equal("error"),
				"Event action should be 'error'")
			Expect(event.EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcomeFailure),
				"Event outcome should be 'failure'")

			// Verify actor information
			Expect(event.ActorType.Value).To(Equal("service"),
				"Actor type should be 'service'")
			Expect(event.ActorID.Value).To(Equal("aianalysis-controller"),
				"Actor ID should be 'aianalysis-controller'")

			// Verify resource information
			Expect(event.ResourceType.Value).To(Equal("AIAnalysis"),
				"Resource type should be 'AIAnalysis'")
			Expect(event.ResourceID.Value).To(Equal("test-analysis"),
				"Resource ID should match analysis name")

			// Verify namespace
			Expect(event.Namespace.Value).To(Equal("default"),
				"Namespace should be 'default'")
		})

		It("should include error details in event payload", func() {
			// Given: An error with specific message
			testErr := errors.New("HAPI connection refused")

			// When: RecordError is called
			auditClient.RecordError(ctx, analysis, "Analyzing", testErr)

			// Then: Event data contains correct payload
			event := mockStore.StoredEvents[0]

			// Verify payload structure using OpenAPI discriminated union
			Expect(event.EventData.IsAIAnalysisErrorPayload()).To(BeTrue(),
				"EventData should be AIAnalysisErrorPayload type")

			payload, ok := event.EventData.GetAIAnalysisErrorPayload()
			Expect(ok).To(BeTrue(), "Should be able to extract AIAnalysisErrorPayload")

			// Verify payload fields
			Expect(payload.Phase).To(Equal("Analyzing"),
				"Phase should match the provided phase")
			Expect(payload.ErrorMessage).To(Equal("HAPI connection refused"),
				"Error message should match the error text")
		})

		It("should use correlation_id from RemediationRequestRef.Name", func() {
			// Given: Analysis with RemediationRequestRef
			testErr := errors.New("test error")

			// When: RecordError is called
			auditClient.RecordError(ctx, analysis, "Investigating", testErr)

			// Then: Correlation ID matches RemediationRequestRef.Name
			event := mockStore.StoredEvents[0]
			Expect(event.CorrelationID).To(Equal("test-rr"),
				"Correlation ID should be RemediationRequestRef.Name per DD-AUDIT-CORRELATION-001")
		})

		It("should fall back to RemediationID when RemediationRequestRef is empty", func() {
			// Given: Analysis without RemediationRequestRef but with RemediationID
			analysis.Spec.RemediationRequestRef.Name = ""
			testErr := errors.New("test error")

			// When: RecordError is called
			auditClient.RecordError(ctx, analysis, "Pending", testErr)

			// Then: Correlation ID falls back to RemediationID
			event := mockStore.StoredEvents[0]
			Expect(event.CorrelationID).To(Equal("test-remediation-id"),
				"Correlation ID should fall back to RemediationID when RemediationRequestRef is empty")
		})
	})

	Context("Error recording for different phases", func() {
		It("should record error in Pending phase", func() {
			testErr := errors.New("initialization error")
			auditClient.RecordError(ctx, analysis, "Pending", testErr)

			Expect(mockStore.StoredEvents).To(HaveLen(1))
			payload, _ := mockStore.StoredEvents[0].EventData.GetAIAnalysisErrorPayload()
			Expect(payload.Phase).To(Equal("Pending"))
		})

		It("should record error in Investigating phase", func() {
			testErr := errors.New("HAPI timeout")
			auditClient.RecordError(ctx, analysis, "Investigating", testErr)

			Expect(mockStore.StoredEvents).To(HaveLen(1))
			payload, _ := mockStore.StoredEvents[0].EventData.GetAIAnalysisErrorPayload()
			Expect(payload.Phase).To(Equal("Investigating"))
		})

		It("should record error in Analyzing phase", func() {
			testErr := errors.New("Rego policy evaluation failed")
			auditClient.RecordError(ctx, analysis, "Analyzing", testErr)

			Expect(mockStore.StoredEvents).To(HaveLen(1))
			payload, _ := mockStore.StoredEvents[0].EventData.GetAIAnalysisErrorPayload()
			Expect(payload.Phase).To(Equal("Analyzing"))
		})
	})

	Context("Error types and messages", func() {
		It("should capture timeout errors", func() {
			testErr := errors.New("context deadline exceeded")
			auditClient.RecordError(ctx, analysis, "Investigating", testErr)

			payload, _ := mockStore.StoredEvents[0].EventData.GetAIAnalysisErrorPayload()
			Expect(payload.ErrorMessage).To(ContainSubstring("context deadline exceeded"))
		})

		It("should capture HTTP errors", func() {
			testErr := errors.New("HTTP 503 Service Unavailable")
			auditClient.RecordError(ctx, analysis, "Investigating", testErr)

			payload, _ := mockStore.StoredEvents[0].EventData.GetAIAnalysisErrorPayload()
			Expect(payload.ErrorMessage).To(ContainSubstring("503"))
		})

		It("should capture validation errors", func() {
			testErr := errors.New("invalid workflow parameter: memory limit exceeds maximum")
			auditClient.RecordError(ctx, analysis, "Analyzing", testErr)

			payload, _ := mockStore.StoredEvents[0].EventData.GetAIAnalysisErrorPayload()
			Expect(payload.ErrorMessage).To(ContainSubstring("invalid workflow parameter"))
		})
	})

	Context("Graceful degradation", func() {
		It("should not fail reconciliation when audit store fails", func() {
			// Given: Mock store that returns error
			mockStore.StoreError = errors.New("audit store unavailable")
			testErr := errors.New("HAPI error")

			// When: RecordError is called
			// Then: Should not panic (fire-and-forget pattern)
			Expect(func() {
				auditClient.RecordError(ctx, analysis, "Investigating", testErr)
			}).NotTo(Panic(), "RecordError should not panic on audit store failure")

			// Verify attempt was made (event not stored due to mock error)
			Expect(mockStore.StoredEvents).To(HaveLen(0),
				"Event should not be stored when StoreError is set")
		})
	})

	Context("Multiple error recordings", func() {
		It("should record multiple errors independently", func() {
			// Given: Multiple errors in different phases
			err1 := errors.New("error 1")
			err2 := errors.New("error 2")
			err3 := errors.New("error 3")

			// When: Multiple errors are recorded
			auditClient.RecordError(ctx, analysis, "Pending", err1)
			auditClient.RecordError(ctx, analysis, "Investigating", err2)
			auditClient.RecordError(ctx, analysis, "Analyzing", err3)

			// Then: All errors are stored independently
			Expect(mockStore.StoredEvents).To(HaveLen(3), "Should store all three error events")

			// Verify each event has correct phase
			payload1, _ := mockStore.StoredEvents[0].EventData.GetAIAnalysisErrorPayload()
			Expect(payload1.Phase).To(Equal("Pending"))

			payload2, _ := mockStore.StoredEvents[1].EventData.GetAIAnalysisErrorPayload()
			Expect(payload2.Phase).To(Equal("Investigating"))

			payload3, _ := mockStore.StoredEvents[2].EventData.GetAIAnalysisErrorPayload()
			Expect(payload3.Phase).To(Equal("Analyzing"))
		})
	})
})
