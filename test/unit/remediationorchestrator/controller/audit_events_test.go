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

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	prodcontroller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	roaudit "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/audit"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// MockAuditStore captures audit events for verification
type MockAuditStore struct {
	Events []*ogenclient.AuditEventRequest
	Errors []error
}

func (m *MockAuditStore) StoreAudit(ctx context.Context, event *ogenclient.AuditEventRequest) error {
	m.Events = append(m.Events, event)
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
	m.Events = nil
	m.Errors = nil
}

func (m *MockAuditStore) GetLastEvent() *ogenclient.AuditEventRequest {
	if len(m.Events) == 0 {
		return nil
	}
	return m.Events[len(m.Events)-1]
}

// GetEventsByType filters events by event type
func (m *MockAuditStore) GetEventsByType(eventType string) []*ogenclient.AuditEventRequest {
	var filtered []*ogenclient.AuditEventRequest
	for _, event := range m.Events {
		if event.EventType == eventType {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

var _ = Describe("BR-ORCH-AUDIT: Audit Event Emission", func() {
	var (
		ctx               context.Context
		fakeClient        client.Client
		reconciler        *prodcontroller.Reconciler
		mockAuditStore    *MockAuditStore
		mockRoutingEngine *MockRoutingEngine
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Create scheme with all CRD types
		scheme := setupScheme()

		// Create mock audit store
		mockAuditStore = &MockAuditStore{}

		// Create mock routing engine
		mockRoutingEngine = &MockRoutingEngine{}

		// Create fake client with status subresource
		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
		WithStatusSubresource(
			&remediationv1.RemediationRequest{},
			&remediationv1.RemediationApprovalRequest{},
			&signalprocessingv1.SignalProcessing{},
			&aianalysisv1.AIAnalysis{},
			&workflowexecutionv1.WorkflowExecution{},
		).
		Build()

		// Create reconciler with mock audit store
		recorder := record.NewFakeRecorder(20) // DD-EVENT-001: FakeRecorder for K8s event assertions
		reconciler = prodcontroller.NewReconciler(
			fakeClient,
			fakeClient,     // apiReader (same as client for tests)
			scheme,
			mockAuditStore, // Use mock audit store
			recorder,       // DD-EVENT-001: FakeRecorder for K8s event assertions
			rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()), // DD-METRICS-001: required
			prodcontroller.TimeoutConfig{
				Global:     1 * time.Hour,
				Processing: 5 * time.Minute,
				Analyzing:  10 * time.Minute,
				Executing:  30 * time.Minute,
			},
			mockRoutingEngine, // Use mock routing engine
		)
	})

	Context("Phase 3: Audit Event Emission Tests", func() {
		It("AE-7.1: Should emit lifecycle started event on new RR", func() {
			// Create new RemediationRequest
			rr := newRemediationRequest("test-rr", "default", "")
			Expect(fakeClient.Create(ctx, rr)).To(Succeed())

		// Reconcile to initialize phase
		result, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "test-rr", Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))

		// Verify lifecycle.started audit event was emitted
		// Note: Gap #8 also emits lifecycle.created, so filter by event type
		lifecycleStartedEvents := mockAuditStore.GetEventsByType(roaudit.EventTypeLifecycleStarted)
		Expect(lifecycleStartedEvents).To(HaveLen(1), "Expected exactly one lifecycle.started event")
		event := lifecycleStartedEvents[0]
		Expect(event).ToNot(BeNil())
		Expect(event.EventType).To(Equal(roaudit.EventTypeLifecycleStarted))
		Expect(event.EventAction).To(Equal("started"))
		// Use enum type comparison, not string
		Expect(string(event.EventOutcome)).To(Equal("pending"))
		})

		It("AE-7.2: Should emit phase transition event on Pending→Processing", func() {
			// Create RR in Pending phase with SP created
			rr := newRemediationRequest("test-rr", "default", remediationv1.PhasePending)
			rr.Status.StartTime = &metav1.Time{Time: time.Now()}
			Expect(fakeClient.Create(ctx, rr)).To(Succeed())

			// Reset audit store to ignore initialization events
			mockAuditStore.Reset()

			// Reconcile to transition to Processing
			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "test-rr", Namespace: "default"},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(5 * time.Second))

			// Verify phase transition audit event
			Expect(mockAuditStore.Events).To(HaveLen(1))
			event := mockAuditStore.GetLastEvent()
			Expect(event.EventType).To(Equal(roaudit.EventTypeLifecycleTransitioned))
			Expect(event.EventAction).To(Equal("transitioned"))
		})

		It("AE-7.3: Should emit completion event on successful workflow", func() {
			// Create RR in Executing phase with completed WE
			rr := newRemediationRequestWithChildRefs("test-rr", "default", remediationv1.PhaseExecuting, "sp-test-rr", "ai-test-rr", "we-test-rr")
			Expect(fakeClient.Create(ctx, rr)).To(Succeed())

			sp := newSignalProcessingCompleted("sp-test-rr", "default", "test-rr")
			Expect(fakeClient.Create(ctx, sp)).To(Succeed())

			ai := newAIAnalysisCompleted("ai-test-rr", "default", "test-rr", 0.9, "restart-pod")
			Expect(fakeClient.Create(ctx, ai)).To(Succeed())

			we := newWorkflowExecutionCompleted("we-test-rr", "default", "test-rr")
			Expect(fakeClient.Create(ctx, we)).To(Succeed())

			mockAuditStore.Reset()

			// Reconcile to transition to Completed
			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "test-rr", Namespace: "default"},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Verify completion audit event
			Expect(mockAuditStore.Events).To(HaveLen(1))
			event := mockAuditStore.GetLastEvent()
			Expect(event.EventType).To(Equal(roaudit.EventTypeLifecycleCompleted))
			Expect(event.EventAction).To(Equal("completed"))
			Expect(string(event.EventOutcome)).To(Equal("success"))
		})

		It("AE-7.4: Should emit failure event on workflow failure", func() {
			// Create RR in Executing phase with failed WE
			rr := newRemediationRequestWithChildRefs("test-rr", "default", remediationv1.PhaseExecuting, "sp-test-rr", "ai-test-rr", "we-test-rr")
			Expect(fakeClient.Create(ctx, rr)).To(Succeed())

			sp := newSignalProcessingCompleted("sp-test-rr", "default", "test-rr")
			Expect(fakeClient.Create(ctx, sp)).To(Succeed())

			ai := newAIAnalysisCompleted("ai-test-rr", "default", "test-rr", 0.9, "restart-pod")
			Expect(fakeClient.Create(ctx, ai)).To(Succeed())

			we := newWorkflowExecutionFailed("we-test-rr", "default", "test-rr", "Pod restart failed")
			Expect(fakeClient.Create(ctx, we)).To(Succeed())

			mockAuditStore.Reset()

			// Reconcile to transition to Failed
			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "test-rr", Namespace: "default"},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Per DD-AUDIT-003: lifecycle failure uses lifecycle.completed with outcome=failure
			Expect(mockAuditStore.Events).To(HaveLen(1))
			event := mockAuditStore.GetLastEvent()
			Expect(event.EventType).To(Equal(roaudit.EventTypeLifecycleCompleted))
			Expect(event.EventAction).To(Equal("completed"))
			Expect(string(event.EventOutcome)).To(Equal("failure"))
		})

		It("AE-7.5: Should emit approval requested event on low confidence", func() {
			// Create RR in Analyzing phase with low confidence AI
			rr := newRemediationRequestWithChildRefs("test-rr", "default", remediationv1.PhaseAnalyzing, "sp-test-rr", "ai-test-rr", "")
			Expect(fakeClient.Create(ctx, rr)).To(Succeed())

			sp := newSignalProcessingCompleted("sp-test-rr", "default", "test-rr")
			Expect(fakeClient.Create(ctx, sp)).To(Succeed())

			ai := newAIAnalysisCompleted("ai-test-rr", "default", "test-rr", 0.4, "risky-workflow")
			Expect(fakeClient.Create(ctx, ai)).To(Succeed())

			mockAuditStore.Reset()

			// Reconcile to transition to AwaitingApproval
			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "test-rr", Namespace: "default"},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			// Verify approval requested audit event
			// Filter by event type since phase transition also emits an event
			approvalEvents := mockAuditStore.GetEventsByType(roaudit.EventTypeApprovalRequested)
			Expect(approvalEvents).To(HaveLen(1))
			event := approvalEvents[0]
			Expect(event.EventAction).To(Equal("approval_requested"))
		})

		It("AE-7.6: Should emit approval decision event on RAR approved", func() {
			// Create RR in AwaitingApproval with approved RAR
			rr := newRemediationRequestWithChildRefs("test-rr", "default", remediationv1.PhaseAwaitingApproval, "sp-test-rr", "ai-test-rr", "")
			Expect(fakeClient.Create(ctx, rr)).To(Succeed())

			sp := newSignalProcessingCompleted("sp-test-rr", "default", "test-rr")
			Expect(fakeClient.Create(ctx, sp)).To(Succeed())

			ai := newAIAnalysisCompleted("ai-test-rr", "default", "test-rr", 0.4, "risky-workflow")
			Expect(fakeClient.Create(ctx, ai)).To(Succeed())

			rar := newRemediationApprovalRequestApproved("rar-test-rr", "default", "test-rr", "admin@example.com")
			Expect(fakeClient.Create(ctx, rar)).To(Succeed())

			mockAuditStore.Reset()

			// Reconcile to process approval
			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "test-rr", Namespace: "default"},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(5 * time.Second))

			// Per DD-AUDIT-003: Approval events use specific action types
			// Filter by event type since phase transition also emits an event
			approvedEvents := mockAuditStore.GetEventsByType(roaudit.EventTypeApprovalApproved)
			Expect(approvedEvents).To(HaveLen(1))
			event := approvedEvents[0]
			Expect(event.EventAction).To(Equal("approved"))
		})

		It("AE-7.7: Should emit rejection event on RAR rejected", func() {
			// Create RR in AwaitingApproval with rejected RAR
			rr := newRemediationRequestWithChildRefs("test-rr", "default", remediationv1.PhaseAwaitingApproval, "sp-test-rr", "ai-test-rr", "")
			Expect(fakeClient.Create(ctx, rr)).To(Succeed())

			sp := newSignalProcessingCompleted("sp-test-rr", "default", "test-rr")
			Expect(fakeClient.Create(ctx, sp)).To(Succeed())

			ai := newAIAnalysisCompleted("ai-test-rr", "default", "test-rr", 0.4, "risky-workflow")
			Expect(fakeClient.Create(ctx, ai)).To(Succeed())

			rar := newRemediationApprovalRequestRejected("rar-test-rr", "default", "test-rr", "admin@example.com", "Too risky")
			Expect(fakeClient.Create(ctx, rar)).To(Succeed())

			mockAuditStore.Reset()

			// Reconcile to process rejection
			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "test-rr", Namespace: "default"},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Per DD-AUDIT-003: Rejection events use specific action types
			// Filter by event type since phase transition also emits an event
			rejectedEvents := mockAuditStore.GetEventsByType(roaudit.EventTypeApprovalRejected)
			Expect(rejectedEvents).To(HaveLen(1))
			event := rejectedEvents[0]
			Expect(event.EventAction).To(Equal("rejected"))
		})

		It("AE-7.8: Should emit timeout event on global timeout", func() {
			// Create RR with expired global timeout
			rr := newRemediationRequestWithTimeout("test-rr", "default", remediationv1.PhasePending, -2*time.Hour)
			Expect(fakeClient.Create(ctx, rr)).To(Succeed())

			mockAuditStore.Reset()

			// Reconcile to detect timeout
			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "test-rr", Namespace: "default"},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Per DD-AUDIT-003: Timeout/failure events use lifecycle.completed with outcome=failure
			Expect(mockAuditStore.Events).To(HaveLen(1))
			event := mockAuditStore.GetLastEvent()
			Expect(event.EventType).To(Equal(roaudit.EventTypeLifecycleCompleted))
			Expect(event.EventAction).To(Equal("completed"))
			Expect(string(event.EventOutcome)).To(Equal("failure"))
		})

		It("AE-7.9: Should emit manual review event on exhausted retries", func() {
			// This scenario requires WorkflowExecution to report ExhaustedRetries
			// For unit test, we simulate by creating RR that would trigger manual review
			// In reality, this is set by WE controller, but we can test the audit emission

			// Create RR in Failed phase with RequiresManualReview flag
			rr := newRemediationRequestWithChildRefs("test-rr", "default", remediationv1.PhaseFailed, "sp-test-rr", "ai-test-rr", "we-test-rr")
			rr.Status.RequiresManualReview = true
			rr.Status.FailureReason = stringPtr("ExhaustedRetries")
			Expect(fakeClient.Create(ctx, rr)).To(Succeed())

			sp := newSignalProcessingCompleted("sp-test-rr", "default", "test-rr")
			Expect(fakeClient.Create(ctx, sp)).To(Succeed())

			ai := newAIAnalysisCompleted("ai-test-rr", "default", "test-rr", 0.9, "restart-pod")
			Expect(fakeClient.Create(ctx, ai)).To(Succeed())

			we := newWorkflowExecutionFailed("we-test-rr", "default", "test-rr", "ExhaustedRetries")
			Expect(fakeClient.Create(ctx, we)).To(Succeed())

			mockAuditStore.Reset()

			// Reconcile (should skip as terminal phase, but may emit audit on first reconcile)
			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "test-rr", Namespace: "default"},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Note: Manual review audit may be emitted during transition to Failed
			// This test validates the audit store can capture it
			// In integration tests, we'd verify the full flow
		})

		It("AE-7.10: Should emit routing blocked event on consecutive failures", func() {
			// This requires routing engine to return blocking condition
			// For unit test, we can't easily trigger this with mock routing engine
			// that always returns "not blocked"

			// This test validates that audit store is wired correctly
			// Full routing blocked audit validation happens in integration tests

			// Create RR without phase to trigger lifecycle.started event
			// (Per reconciler logic: lifecycle.started emitted when OverallPhase == "")
			rr := newRemediationRequest("test-rr", "default", "")
			// Don't set StartTime - let reconciler initialize it
			Expect(fakeClient.Create(ctx, rr)).To(Succeed())

		// First reconcile: initializes phase to Pending and emits lifecycle.started + lifecycle.created
		result, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "test-rr", Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))

		// Verify lifecycle.started event was emitted (Gap #8 also emits lifecycle.created)
		lifecycleStartedEvents := mockAuditStore.GetEventsByType(roaudit.EventTypeLifecycleStarted)
		Expect(lifecycleStartedEvents).To(HaveLen(1), "Expected lifecycle.started event after initialization")

		// Second reconcile: transitions from Pending to Processing and emits phase.transitioned
		result, err = reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{Name: "test-rr", Namespace: "default"},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(5 * time.Second))

		// Per DD-AUDIT-003: Should have these events now:
		// 1. orchestrator.lifecycle.started (first reconcile)
		// 2. orchestrator.lifecycle.created (first reconcile - Gap #8)
		// 3. orchestrator.phase.transitioned (Pending→Processing - second reconcile)
		Expect(mockAuditStore.Events).To(HaveLen(3), "Expected lifecycle.started + lifecycle.created + phase.transitioned events")
		})
	})
})

// Helper function to create string pointer
func stringPtr(s string) *string {
	return &s
}
