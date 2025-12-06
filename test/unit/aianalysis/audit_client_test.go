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

package aianalysis

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	aiaudit "github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
	"github.com/jordigilh/kubernaut/pkg/audit"
)

// Day 5: Audit Client Unit Tests
// DD-AUDIT-002: Tests verify correct audit event generation
var _ = Describe("AIAnalysis Audit Client", func() {
	var (
		auditClient *aiaudit.AuditClient
		mockStore   *MockAuditStore
		ctx         context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockStore = NewMockAuditStore()
		auditClient = aiaudit.NewAuditClient(mockStore, ctrl.Log.WithName("test-audit"))
	})

	// Helper to create test AIAnalysis
	createTestAnalysis := func() *aianalysisv1.AIAnalysis {
		return &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-analysis",
				Namespace: "default",
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationRequestRef: corev1.ObjectReference{
					Kind:      "RemediationRequest",
					Name:      "test-rr",
					Namespace: "default",
				},
				RemediationID: "test-remediation-001",
				AnalysisRequest: aianalysisv1.AnalysisRequest{
					SignalContext: aianalysisv1.SignalContextInput{
						Fingerprint:      "test-fingerprint",
						Severity:         "warning",
						SignalType:       "OOMKilled",
						Environment:      "production",
						BusinessPriority: "P0",
						TargetResource: aianalysisv1.TargetResource{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: "default",
						},
					},
					AnalysisTypes: []string{"investigation"},
				},
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase:            "Completed",
				ApprovalRequired: true,
				ApprovalReason:   "Low confidence",
				SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
					WorkflowID: "restart-pod-v1",
					Confidence: 0.75,
				},
			},
		}
	}

	// ========================================
	// ANALYSIS COMPLETION AUDIT
	// ========================================
	Describe("RecordAnalysisComplete", func() {
		// DD-AUDIT-003: Business outcome - audit analysis completion for compliance
		It("should record analysis completion with all required fields", func() {
			analysis := createTestAnalysis()

			auditClient.RecordAnalysisComplete(ctx, analysis)

			Expect(mockStore.Events).To(HaveLen(1))
			event := mockStore.Events[0]

			// Verify required fields (DD-AUDIT-002)
			Expect(event.EventType).To(Equal("aianalysis.analysis.completed"))
			Expect(event.EventCategory).To(Equal("analysis"))
			Expect(event.EventAction).To(Equal("completed"))
			Expect(event.ActorType).To(Equal("service"))
			Expect(event.ActorID).To(Equal("aianalysis-controller"))
			Expect(event.ResourceType).To(Equal("AIAnalysis"))
			Expect(event.ResourceID).To(Equal("test-analysis"))
			Expect(event.CorrelationID).To(Equal("test-remediation-001"))
		})

		// DD-AUDIT-003: Track success vs failure outcomes
		It("should set outcome=success for completed analysis", func() {
			analysis := createTestAnalysis()
			analysis.Status.Phase = "Completed"

			auditClient.RecordAnalysisComplete(ctx, analysis)

			Expect(mockStore.Events).To(HaveLen(1))
			Expect(mockStore.Events[0].EventOutcome).To(Equal("success"))
		})

		It("should set outcome=failure for failed analysis", func() {
			analysis := createTestAnalysis()
			analysis.Status.Phase = "Failed"
			analysis.Status.Reason = "WorkflowResolutionFailed"
			analysis.Status.SubReason = "LowConfidence"

			auditClient.RecordAnalysisComplete(ctx, analysis)

			Expect(mockStore.Events).To(HaveLen(1))
			Expect(mockStore.Events[0].EventOutcome).To(Equal("failure"))
		})

		// DD-AUDIT-003: Include confidence and workflow info
		It("should include workflow and confidence in event data", func() {
			analysis := createTestAnalysis()

			auditClient.RecordAnalysisComplete(ctx, analysis)

			Expect(mockStore.Events).To(HaveLen(1))
			Expect(mockStore.Events[0].EventData).NotTo(BeEmpty())
			// EventData is JSON bytes - verify it contains expected fields
			eventDataStr := string(mockStore.Events[0].EventData)
			Expect(eventDataStr).To(ContainSubstring("confidence"))
			Expect(eventDataStr).To(ContainSubstring("workflow_id"))
		})
	})

	// ========================================
	// PHASE TRANSITION AUDIT
	// ========================================
	Describe("RecordPhaseTransition", func() {
		// DD-AUDIT-003: Track phase transitions for debugging
		It("should record phase transition with from/to phases", func() {
			analysis := createTestAnalysis()

			auditClient.RecordPhaseTransition(ctx, analysis, "Pending", "Investigating")

			Expect(mockStore.Events).To(HaveLen(1))
			event := mockStore.Events[0]

			Expect(event.EventType).To(Equal("aianalysis.phase.transition"))
			Expect(event.EventCategory).To(Equal("analysis"))
			Expect(event.EventAction).To(Equal("phase_transition"))
			Expect(event.EventOutcome).To(Equal("success"))

			// Verify from/to phases in event data
			eventDataStr := string(event.EventData)
			Expect(eventDataStr).To(ContainSubstring("from_phase"))
			Expect(eventDataStr).To(ContainSubstring("to_phase"))
			Expect(eventDataStr).To(ContainSubstring("Pending"))
			Expect(eventDataStr).To(ContainSubstring("Investigating"))
		})
	})

	// ========================================
	// ERROR AUDIT
	// ========================================
	Describe("RecordError", func() {
		// DD-AUDIT-003: Track errors for debugging and alerting
		It("should record error with phase and error message", func() {
			analysis := createTestAnalysis()
			testErr := &testError{message: "HolmesGPT-API timeout"}

			auditClient.RecordError(ctx, analysis, "Investigating", testErr)

			Expect(mockStore.Events).To(HaveLen(1))
			event := mockStore.Events[0]

			Expect(event.EventType).To(Equal("aianalysis.error.occurred"))
			Expect(event.EventCategory).To(Equal("analysis"))
			Expect(event.EventAction).To(Equal("error"))
			Expect(event.EventOutcome).To(Equal("failure"))
			Expect(event.ErrorMessage).NotTo(BeNil())
			Expect(*event.ErrorMessage).To(ContainSubstring("timeout"))
		})
	})

	// ========================================
	// GRACEFUL DEGRADATION
	// ========================================
	Describe("Graceful Degradation (DD-AUDIT-002)", func() {
		// DD-AUDIT-002: Audit failures MUST NOT block business logic
		It("should not panic when store fails", func() {
			mockStore.ShouldFail = true
			analysis := createTestAnalysis()

			// Should not panic even if store fails
			Expect(func() {
				auditClient.RecordAnalysisComplete(ctx, analysis)
			}).NotTo(Panic())
		})

		It("should continue processing after store error", func() {
			mockStore.ShouldFail = true
			analysis := createTestAnalysis()

			// First call fails
			auditClient.RecordAnalysisComplete(ctx, analysis)

			// Store starts working
			mockStore.ShouldFail = false

			// Second call should work
			auditClient.RecordAnalysisComplete(ctx, analysis)
			Expect(mockStore.Events).To(HaveLen(1))
		})
	})
})

// ========================================
// MOCK AUDIT STORE
// ========================================

// MockAuditStore implements audit.AuditStore for testing
type MockAuditStore struct {
	Events     []*audit.AuditEvent
	ShouldFail bool
}

func NewMockAuditStore() *MockAuditStore {
	return &MockAuditStore{
		Events: make([]*audit.AuditEvent, 0),
	}
}

func (m *MockAuditStore) StoreAudit(ctx context.Context, event *audit.AuditEvent) error {
	if m.ShouldFail {
		return &testError{message: "mock store failure"}
	}
	m.Events = append(m.Events, event)
	return nil
}

func (m *MockAuditStore) Close() error {
	return nil
}

// testError is a simple error implementation for testing
type testError struct {
	message string
}

func (e *testError) Error() string {
	return e.message
}

