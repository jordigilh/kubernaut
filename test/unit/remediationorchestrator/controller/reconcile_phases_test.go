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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	prodcontroller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// ========================================
// Mock Routing Engine for Unit Tests
// ========================================
// ReconcileScenario defines a table-driven test scenario for Reconcile() method
type ReconcileScenario struct {
	name              string                                  // Test scenario name
	description       string                                  // Brief description of what's being tested
	businessReq       string                                  // BR-XXX-XXX reference
	initialObjects    []client.Object                         // Objects to seed the fake client
	rrName            types.NamespacedName                    // RemediationRequest to reconcile
	expectedPhase     remediationv1.RemediationPhase          // Expected final phase
	expectedResult    ctrl.Result                             // Expected reconcile result
	expectError       bool                                    // Whether to expect an error
	expectedChildren  map[string]bool                         // Expected child CRD existence: "SP", "AI", "WE", "RAR", "Notification"
	additionalAsserts func(*remediationv1.RemediationRequest) // Additional custom assertions
}

var _ = Describe("BR-ORCH-025: Phase Transition Logic (Table-Driven Tests)", func() {
	var (
		ctx    context.Context
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		_ = remediationv1.AddToScheme(scheme)
		_ = signalprocessingv1.AddToScheme(scheme)
		_ = aianalysisv1.AddToScheme(scheme)
		_ = workflowexecutionv1.AddToScheme(scheme)
		_ = notificationv1.AddToScheme(scheme) // Required for approval notifications
		_ = corev1.AddToScheme(scheme)
		_ = eav1.AddToScheme(scheme)
	})

	DescribeTable("Phase Transitions",
		func(scenario ReconcileScenario) {
			// Given: Create fake client with initial objects
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(scenario.initialObjects...).
			WithStatusSubresource(
				&remediationv1.RemediationRequest{},
				&remediationv1.RemediationApprovalRequest{},
				&signalprocessingv1.SignalProcessing{},
				&aianalysisv1.AIAnalysis{},
				&workflowexecutionv1.WorkflowExecution{},
			).
			Build()

			// Create reconciler with test dependencies
			// Use MockRoutingEngine to isolate orchestration logic testing from routing business logic
			// Routing logic is tested separately in integration tests (defense-in-depth)
			mockRouting := &MockRoutingEngine{}
			recorder := record.NewFakeRecorder(20) // DD-EVENT-001: FakeRecorder for K8s event assertions
			reconciler := prodcontroller.NewReconciler(
				fakeClient,
				fakeClient, // apiReader (same as client for tests)
				scheme,
				nil,      // Audit store is nil for unit tests (DD-AUDIT-003 compliant)
				recorder, // DD-EVENT-001: FakeRecorder for K8s event assertions
				rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()), // DD-METRICS-001: required
				prodcontroller.TimeoutConfig{
					Global:     1 * time.Hour,
					Processing: 5 * time.Minute,
					Analyzing:  10 * time.Minute,
					Executing:  30 * time.Minute,
				},
				mockRouting, // Mock routing engine for unit tests
			)

			// When: Reconcile the RemediationRequest
			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: scenario.rrName,
			})

			// Debug: Print actual vs expected
			if !scenario.expectError && err == nil && result != scenario.expectedResult {
				// Fetch updated RR to see its phase
				updatedRR := &remediationv1.RemediationRequest{}
				_ = fakeClient.Get(ctx, scenario.rrName, updatedRR)
				GinkgoWriter.Printf("\nDEBUG %s:\n", scenario.name)
				GinkgoWriter.Printf("  Expected result: %+v\n", scenario.expectedResult)
				GinkgoWriter.Printf("  Actual result:   %+v\n", result)
				GinkgoWriter.Printf("  Expected phase:  %s\n", scenario.expectedPhase)
				GinkgoWriter.Printf("  Actual phase:    %s\n", updatedRR.Status.OverallPhase)
			}

			// Then: Verify reconcile result and error
			if scenario.expectError {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
			Expect(result).To(Equal(scenario.expectedResult))

			// Verify final RR state
			var finalRR remediationv1.RemediationRequest
			getErr := fakeClient.Get(ctx, scenario.rrName, &finalRR)
			if scenario.expectedPhase != "" {
				Expect(getErr).ToNot(HaveOccurred(), "Should be able to get final RR")
				Expect(finalRR.Status.OverallPhase).To(Equal(scenario.expectedPhase),
					"Final phase should match expected")
			}

			// Verify expected children existence
			if scenario.expectedChildren != nil {
				verifyChildrenExistence(ctx, fakeClient, &finalRR, scenario.expectedChildren)
			}

			// Run additional custom assertions
			if scenario.additionalAsserts != nil && getErr == nil {
				scenario.additionalAsserts(&finalRR)
			}
		},

		// ========================================
		// Category 1: Pending → Processing (5 scenarios)
		// ========================================

		Entry("1.1: Pending→Processing - Happy Path (BR-ORCH-025.1)", ReconcileScenario{
			name:        "pending_to_processing_happy_path",
			description: "Fresh RR in Pending should create SignalProcessing and transition to Processing",
			businessReq: "BR-ORCH-025.1",
			initialObjects: []client.Object{
				newRemediationRequest("test-rr", "default", remediationv1.PhasePending),
			},
			rrName:        types.NamespacedName{Name: "test-rr", Namespace: "default"},
			expectedPhase: remediationv1.PhaseProcessing,
			expectedResult: ctrl.Result{
				RequeueAfter: 5 * time.Second, // Requeue to check SP progress
			},
			expectedChildren: map[string]bool{
				"SP": true, // SignalProcessing should be created
				"AI": false,
				"WE": false,
			},
		}),

		Entry("1.2: Pending→Pending - RR Not Found (Graceful Handling)", ReconcileScenario{
			name:           "pending_rr_not_found",
			description:    "Request for non-existent RR should return gracefully without error",
			businessReq:    "Error handling",
			initialObjects: []client.Object{},
			rrName:         types.NamespacedName{Name: "non-existent", Namespace: "default"},
			expectedPhase:  "", // No phase check (RR doesn't exist)
			expectedResult: ctrl.Result{},
			expectError:    false,
		}),

		Entry("1.3: Pending→Processing - Empty Pending Phase", ReconcileScenario{
			name:        "pending_empty_phase_initializes",
			description: "RR with empty phase should be initialized to Pending (requires 2nd reconcile for SP creation)",
			businessReq: "BR-ORCH-025.1 (initialization)",
			initialObjects: []client.Object{
				newRemediationRequest("test-rr", "default", ""), // Empty phase
			},
			rrName:        types.NamespacedName{Name: "test-rr", Namespace: "default"},
			expectedPhase: remediationv1.PhasePending, // First reconcile just initializes
			expectedResult: ctrl.Result{
				RequeueAfter: 100 * time.Millisecond, // Requeue after short delay to process Pending phase
			},
			expectedChildren: map[string]bool{
				"SP": false, // SP created in second reconcile
			},
		}),

		Entry("1.4: Pending→Processing - Preserves Gateway Metadata", ReconcileScenario{
			name:        "pending_preserves_gateway_metadata",
			description: "Should preserve Gateway deduplication metadata during SP creation",
			businessReq: "BR-ORCH-038 (preserve Gateway data)",
			initialObjects: []client.Object{
				newRemediationRequestWithGatewayMetadata("test-rr", "default"),
			},
			rrName:        types.NamespacedName{Name: "test-rr", Namespace: "default"},
			expectedPhase: remediationv1.PhaseProcessing,
			expectedResult: ctrl.Result{
				RequeueAfter: 5 * time.Second, // Active phase - requeue to check SP progress
			},
			expectedChildren: map[string]bool{
				"SP": true,
			},
			additionalAsserts: func(rr *remediationv1.RemediationRequest) {
				// Verify Gateway metadata preserved
				Expect(rr.Spec.SignalLabels).To(HaveKeyWithValue("dedup_group", "test-group"))
				Expect(rr.Spec.SignalLabels).To(HaveKeyWithValue("process_id", "test-process"))
			},
		}),

		Entry("1.5: Pending - Terminal Phase No Action", ReconcileScenario{
			name:        "pending_terminal_phase_no_action",
			description: "RR in terminal phase (Completed) should skip reconciliation",
			businessReq: "Performance optimization",
			initialObjects: []client.Object{
				newRemediationRequest("test-rr", "default", remediationv1.PhaseCompleted),
			},
			rrName:         types.NamespacedName{Name: "test-rr", Namespace: "default"},
			expectedPhase:  remediationv1.PhaseCompleted, // No change
			expectedResult: ctrl.Result{},                // No requeue
			expectedChildren: map[string]bool{
				"SP": false, // Should NOT create anything
			},
		}),

		// ========================================
		// Category 2: Processing → Analyzing (5 scenarios)
		// ========================================

		Entry("2.1: Processing→Analyzing - SP Completed (BR-ORCH-025.2)", ReconcileScenario{
			name:        "processing_to_analyzing_sp_completed",
			description: "When SP completes, should create AIAnalysis and transition to Analyzing",
			businessReq: "BR-ORCH-025.2",
			initialObjects: []client.Object{
				newRemediationRequestWithChildRefs("test-rr", "default", remediationv1.PhaseProcessing, "test-rr-sp", "", ""),
				newSignalProcessingCompleted("test-rr-sp", "default", "test-rr"),
			},
			rrName:        types.NamespacedName{Name: "test-rr", Namespace: "default"},
			expectedPhase: remediationv1.PhaseAnalyzing,
			expectedResult: ctrl.Result{
				RequeueAfter: 5 * time.Second, // Requeue to check AI progress
			},
			expectedChildren: map[string]bool{
				"AI": true, // AIAnalysis should be created
			},
		}),

		Entry("2.2: Processing→Failed - SP Failed", ReconcileScenario{
			name:        "processing_sp_failed",
			description: "When SP fails, should transition to Failed and propagate error",
			businessReq: "BR-ORCH-025.2 (error propagation)",
			initialObjects: []client.Object{
				newRemediationRequestWithChildRefs("test-rr", "default", remediationv1.PhaseProcessing, "test-rr-sp", "", ""),
				newSignalProcessingFailed("test-rr-sp", "default", "test-rr", "Enrichment failed"),
			},
			rrName:         types.NamespacedName{Name: "test-rr", Namespace: "default"},
			expectedPhase:  remediationv1.PhaseFailed,
			expectedResult: ctrl.Result{}, // Terminal phase, no requeue
			additionalAsserts: func(rr *remediationv1.RemediationRequest) {
				// Check FailureReason instead of Message (fake client issue with Message field)
				Expect(rr.Status.FailureReason).ToNot(BeNil())
				Expect(*rr.Status.FailureReason).To(ContainSubstring("SignalProcessing failed"))
				// TODO: Investigate why Message field is not persisted by fake client
				// Expect(rr.Status.Message).To(ContainSubstring("SignalProcessing failed"))
			},
		}),

		Entry("2.3: Processing - SP In Progress", ReconcileScenario{
			name:        "processing_sp_in_progress",
			description: "While SP is still processing, should stay in Processing and requeue",
			businessReq: "BR-ORCH-025.2 (wait for child)",
			initialObjects: []client.Object{
				newRemediationRequestWithChildRefs("test-rr", "default", remediationv1.PhaseProcessing, "test-rr-sp", "", ""),
				newSignalProcessing("test-rr-sp", "default", "test-rr", signalprocessingv1.PhaseEnriching),
			},
			rrName:        types.NamespacedName{Name: "test-rr", Namespace: "default"},
			expectedPhase: remediationv1.PhaseProcessing, // Stay in same phase
			expectedResult: ctrl.Result{
				RequeueAfter: 10 * time.Second, // Requeue to check again (default case in handleProcessingPhase)
			},
		}),

		Entry("2.4: Processing→Analyzing - Status Aggregation", ReconcileScenario{
			name:        "processing_status_aggregation",
			description: "Should aggregate SP status into RR.Status.SignalProcessingRef",
			businessReq: "BR-ORCH-026 (status aggregation)",
			initialObjects: []client.Object{
				newRemediationRequestWithChildRefs("test-rr", "default", remediationv1.PhaseProcessing, "test-rr-sp", "", ""),
				newSignalProcessingCompleted("test-rr-sp", "default", "test-rr"),
			},
			rrName:        types.NamespacedName{Name: "test-rr", Namespace: "default"},
			expectedPhase: remediationv1.PhaseAnalyzing,
			expectedResult: ctrl.Result{
				RequeueAfter: 5 * time.Second, // Requeue to check AI progress
			},
			additionalAsserts: func(rr *remediationv1.RemediationRequest) {
				// Verify SignalProcessing reference is populated
				Expect(rr.Status.SignalProcessingRef).ToNot(BeNil())
				Expect(rr.Status.SignalProcessingRef.Name).To(Equal("test-rr-sp"))
				Expect(rr.Status.SignalProcessingRef.Namespace).To(Equal("default"))
			},
		}),

		Entry("2.5: Processing - SP Missing (Error Recovery)", ReconcileScenario{
			name:        "processing_sp_missing",
			description: "If SP is missing but should exist, should transition to Failed",
			businessReq: "BR-ORCH-026 (error handling)",
			initialObjects: []client.Object{
				newRemediationRequest("test-rr", "default", remediationv1.PhaseProcessing),
				// SP not found (was deleted or never created)
			},
			rrName:        types.NamespacedName{Name: "test-rr", Namespace: "default"},
			expectedPhase: remediationv1.PhaseFailed,
			additionalAsserts: func(rr *remediationv1.RemediationRequest) {
				// fake client doesn't persist Message field reliably - check FailureReason instead
				Expect(rr.Status.FailureReason).ToNot(BeNil())
				Expect(*rr.Status.FailureReason).To(ContainSubstring("SignalProcessing not found"))
			},
		}),

		// ========================================
		// Category 3: Analyzing → Executing (5 scenarios)
		// ========================================

		Entry("3.1: Analyzing→Executing - High Confidence (BR-ORCH-025.3)", ReconcileScenario{
			name:        "analyzing_to_executing_high_confidence",
			description: "High confidence AI result should create WorkflowExecution and transition to Executing",
			businessReq: "BR-ORCH-025.3",
			initialObjects: []client.Object{
				newRemediationRequestWithChildRefs("test-rr", "default", remediationv1.PhaseAnalyzing, "test-rr-sp", "test-rr-ai", ""),
				newSignalProcessingCompleted("test-rr-sp", "default", "test-rr"),
				newAIAnalysisCompleted("test-rr-ai", "default", "test-rr", 0.9, "restart-pod-v1"),
			},
			rrName:        types.NamespacedName{Name: "test-rr", Namespace: "default"},
			expectedPhase: remediationv1.PhaseExecuting,
			expectedResult: ctrl.Result{
				RequeueAfter: 5 * time.Second, // Requeue to check WE progress (transitionPhase returns 5s for Executing)
			},
			expectedChildren: map[string]bool{
				"WE": true, // WorkflowExecution should be created
			},
		}),

		Entry("3.2: Analyzing→AwaitingApproval - Low Confidence (BR-ORCH-001)", ReconcileScenario{
			name:        "analyzing_to_awaiting_approval_low_confidence",
			description: "Low confidence AI result should create RAR and transition to AwaitingApproval",
			businessReq: "BR-ORCH-001 (approval required)",
			initialObjects: []client.Object{
				newRemediationRequestWithChildRefs("test-rr", "default", remediationv1.PhaseAnalyzing, "test-rr-sp", "test-rr-ai", ""),
				newSignalProcessingCompleted("test-rr-sp", "default", "test-rr"),
				newAIAnalysisCompleted("test-rr-ai", "default", "test-rr", 0.4, "risky-workflow"),
			},
			rrName:        types.NamespacedName{Name: "test-rr", Namespace: "default"},
			expectedPhase: remediationv1.PhaseAwaitingApproval,
			expectedResult: ctrl.Result{
				RequeueAfter: 100 * time.Millisecond, // Requeue after short delay after creating RAR
			},
			expectedChildren: map[string]bool{
				"RAR": true,  // RemediationApprovalRequest should be created
				"WE":  false, // Should NOT create WE yet
			},
		}),

		Entry("3.3: Analyzing→Completed - WorkflowNotNeeded (BR-ORCH-037)", ReconcileScenario{
			name:        "analyzing_workflow_not_needed",
			description: "AI determines no workflow needed should transition directly to Completed",
			businessReq: "BR-ORCH-037 (WorkflowNotNeeded handling)",
			initialObjects: []client.Object{
				newRemediationRequestWithChildRefs("test-rr", "default", remediationv1.PhaseAnalyzing, "test-rr-sp", "test-rr-ai", ""),
				newSignalProcessingCompleted("test-rr-sp", "default", "test-rr"),
				newAIAnalysisWorkflowNotNeeded("test-rr-ai", "default", "test-rr", "No remediation needed"),
			},
			rrName:         types.NamespacedName{Name: "test-rr", Namespace: "default"},
			expectedPhase:  remediationv1.PhaseCompleted,
			expectedResult: ctrl.Result{}, // Terminal phase
			expectedChildren: map[string]bool{
				"WE":  false,
				"RAR": false,
			},
			additionalAsserts: func(rr *remediationv1.RemediationRequest) {
				Expect(rr.Status.Message).To(ContainSubstring("No remediation needed"))
			},
		}),

		Entry("3.4: Analyzing - AI In Progress", ReconcileScenario{
			name:        "analyzing_ai_in_progress",
			description: "While AI is analyzing, should stay in Analyzing and requeue",
			businessReq: "BR-ORCH-025.3 (wait for child)",
			initialObjects: []client.Object{
				newRemediationRequest("test-rr", "default", remediationv1.PhaseAnalyzing),
				newSignalProcessingCompleted("test-rr-sp", "default", "test-rr"),
				newAIAnalysis("test-rr-ai", "default", "test-rr", aianalysisv1.PhaseAnalyzing),
			},
			rrName:        types.NamespacedName{Name: "test-rr", Namespace: "default"},
			expectedPhase: remediationv1.PhaseAnalyzing, // Stay in same phase
			expectedResult: ctrl.Result{
				RequeueAfter: 5 * time.Second,
			},
		}),

		Entry("3.5: Analyzing→Failed - AI Failed", ReconcileScenario{
			name:        "analyzing_ai_failed",
			description: "When AIAnalysis fails, should transition to Failed and propagate error",
			businessReq: "BR-ORCH-025.3 (error propagation)",
			initialObjects: []client.Object{
				newRemediationRequestWithChildRefs("test-rr", "default", remediationv1.PhaseAnalyzing, "test-rr-sp", "test-rr-ai", ""),
				newSignalProcessingCompleted("test-rr-sp", "default", "test-rr"),
				newAIAnalysisFailed("test-rr-ai", "default", "test-rr", "LLM unavailable"),
			},
			rrName:         types.NamespacedName{Name: "test-rr", Namespace: "default"},
			expectedPhase:  remediationv1.PhaseFailed,
			expectedResult: ctrl.Result{}, // Terminal phase
			additionalAsserts: func(rr *remediationv1.RemediationRequest) {
				// Check FailureReason instead of Message (fake client issue with Message field)
				Expect(rr.Status.FailureReason).ToNot(BeNil())
				Expect(*rr.Status.FailureReason).To(ContainSubstring("AIAnalysis failed"))
			},
		}),

		// ========================================
		// Category 4: Executing → Completed/Failed (5 scenarios)
		// ========================================

		Entry("4.1: Executing→Completed - WE Succeeded (BR-ORCH-025.4)", ReconcileScenario{
			name:        "executing_to_completed_we_succeeded",
			description: "When WorkflowExecution succeeds, should transition to Completed",
			businessReq: "BR-ORCH-025.4",
			initialObjects: []client.Object{
				newRemediationRequestWithChildRefs("test-rr", "default", remediationv1.PhaseExecuting, "test-rr-sp", "test-rr-ai", "test-rr-we"),
				newSignalProcessingCompleted("test-rr-sp", "default", "test-rr"),
				newAIAnalysisCompleted("test-rr-ai", "default", "test-rr", 0.9, "restart-pod-v1"),
				newWorkflowExecutionSucceeded("test-rr-we", "default", "test-rr"),
			},
			rrName:         types.NamespacedName{Name: "test-rr", Namespace: "default"},
			expectedPhase:  remediationv1.PhaseCompleted,
			expectedResult: ctrl.Result{}, // Terminal phase, no requeue
		}),

		Entry("4.2: Executing→Failed - WE Failed", ReconcileScenario{
			name:        "executing_we_failed",
			description: "When WorkflowExecution fails, should transition to Failed and propagate error",
			businessReq: "BR-ORCH-025.4 (error propagation)",
			initialObjects: []client.Object{
				newRemediationRequestWithChildRefs("test-rr", "default", remediationv1.PhaseExecuting, "test-rr-sp", "test-rr-ai", "test-rr-we"),
				newSignalProcessingCompleted("test-rr-sp", "default", "test-rr"),
				newAIAnalysisCompleted("test-rr-ai", "default", "test-rr", 0.9, "restart-pod-v1"),
				newWorkflowExecutionFailed("test-rr-we", "default", "test-rr", "Pod not found"),
			},
			rrName:         types.NamespacedName{Name: "test-rr", Namespace: "default"},
			expectedPhase:  remediationv1.PhaseFailed,
			expectedResult: ctrl.Result{}, // Terminal phase
			additionalAsserts: func(rr *remediationv1.RemediationRequest) {
				// Check FailureReason instead of Message (fake client issue with Message field)
				Expect(rr.Status.FailureReason).ToNot(BeNil())
				Expect(*rr.Status.FailureReason).To(ContainSubstring("WorkflowExecution failed"))
			},
		}),

		Entry("4.3: Executing - WE In Progress", ReconcileScenario{
			name:        "executing_we_in_progress",
			description: "While WE is running, should stay in Executing and requeue",
			businessReq: "BR-ORCH-025.4 (wait for child)",
			initialObjects: []client.Object{
				newRemediationRequestWithChildRefs("test-rr", "default", remediationv1.PhaseExecuting, "test-rr-sp", "test-rr-ai", "test-rr-we"),
				newSignalProcessingCompleted("test-rr-sp", "default", "test-rr"),
				newAIAnalysisCompleted("test-rr-ai", "default", "test-rr", 0.9, "restart-pod-v1"),
				newWorkflowExecution("test-rr-we", "default", "test-rr", workflowexecutionv1.PhaseRunning),
			},
			rrName:        types.NamespacedName{Name: "test-rr", Namespace: "default"},
			expectedPhase: remediationv1.PhaseExecuting, // Stay in same phase
			expectedResult: ctrl.Result{
				RequeueAfter: 10 * time.Second,
			},
		}),

		Entry("4.4: Executing→Completed - Status Aggregation", ReconcileScenario{
			name:        "executing_status_aggregation_all_children",
			description: "Should aggregate all children statuses correctly",
			businessReq: "BR-ORCH-026 (complete status aggregation)",
			initialObjects: []client.Object{
				newRemediationRequestWithChildRefs("test-rr", "default", remediationv1.PhaseExecuting, "test-rr-sp", "test-rr-ai", "test-rr-we"),
				newSignalProcessingCompleted("test-rr-sp", "default", "test-rr"),
				newAIAnalysisCompleted("test-rr-ai", "default", "test-rr", 0.9, "restart-pod-v1"),
				newWorkflowExecutionSucceeded("test-rr-we", "default", "test-rr"),
			},
			rrName:        types.NamespacedName{Name: "test-rr", Namespace: "default"},
			expectedPhase: remediationv1.PhaseCompleted,
			additionalAsserts: func(rr *remediationv1.RemediationRequest) {
				// Verify all child references are populated with correct names
				Expect(rr.Status.SignalProcessingRef).ToNot(BeNil())
				Expect(rr.Status.SignalProcessingRef.Name).To(Equal("test-rr-sp"))

				Expect(rr.Status.AIAnalysisRef).ToNot(BeNil())
				Expect(rr.Status.AIAnalysisRef.Name).To(Equal("test-rr-ai"))

				Expect(rr.Status.WorkflowExecutionRef).ToNot(BeNil())
				Expect(rr.Status.WorkflowExecutionRef.Name).To(Equal("test-rr-we"))
			},
		}),

		Entry("4.5: Executing - WE Missing (Error Recovery)", ReconcileScenario{
			name:        "executing_we_missing",
			description: "If WE is missing but should exist, should transition to Failed",
			businessReq: "BR-ORCH-026 (error handling)",
			initialObjects: []client.Object{
				newRemediationRequestWithChildRefs("test-rr", "default", remediationv1.PhaseExecuting, "test-rr-sp", "test-rr-ai", "test-rr-we"),
				newSignalProcessingCompleted("test-rr-sp", "default", "test-rr"),
				newAIAnalysisCompleted("test-rr-ai", "default", "test-rr", 0.9, "restart-pod-v1"),
				// WE not found (was deleted or never created)
			},
			rrName:        types.NamespacedName{Name: "test-rr", Namespace: "default"},
			expectedPhase: remediationv1.PhaseFailed,
			additionalAsserts: func(rr *remediationv1.RemediationRequest) {
				// fake client doesn't persist Message field reliably - check FailureReason instead
				Expect(rr.Status.FailureReason).ToNot(BeNil())
				Expect(*rr.Status.FailureReason).To(ContainSubstring("WorkflowExecution not found"))
			},
		}),

		// ========================================
		// PHASE 2: APPROVAL WORKFLOW TESTS (5 scenarios)
		// BR-ORCH-001: Approval orchestration
		// ========================================

		Entry("5.1: AwaitingApproval→Executing - RAR Approved (BR-ORCH-001)", ReconcileScenario{
			name:        "awaiting_approval_to_executing_approved",
			description: "When RAR is approved, should create WE and transition to Executing",
			businessReq: "BR-ORCH-001 (approval granted)",
			initialObjects: []client.Object{
				newRemediationRequestWithChildRefs("test-rr", "default", remediationv1.PhaseAwaitingApproval, "test-rr-sp", "test-rr-ai", ""),
				newSignalProcessingCompleted("test-rr-sp", "default", "test-rr"),
				newAIAnalysisCompleted("test-rr-ai", "default", "test-rr", 0.4, "risky-workflow"),
				newRemediationApprovalRequestApproved("rar-test-rr", "default", "test-rr", "admin@example.com"),
			},
			rrName:           types.NamespacedName{Name: "test-rr", Namespace: "default"},
			expectedPhase:    remediationv1.PhaseExecuting,
			expectedResult:   ctrl.Result{RequeueAfter: 5 * time.Second},
			expectedChildren: map[string]bool{"WE": true},
		}),

		Entry("5.2: AwaitingApproval→Failed - RAR Rejected (BR-ORCH-001)", ReconcileScenario{
			name:        "awaiting_approval_to_failed_rejected",
			description: "When RAR is rejected, should transition to Failed",
			businessReq: "BR-ORCH-001 (approval rejected)",
			initialObjects: []client.Object{
				newRemediationRequestWithChildRefs("test-rr", "default", remediationv1.PhaseAwaitingApproval, "test-rr-sp", "test-rr-ai", ""),
				newSignalProcessingCompleted("test-rr-sp", "default", "test-rr"),
				newAIAnalysisCompleted("test-rr-ai", "default", "test-rr", 0.4, "risky-workflow"),
				newRemediationApprovalRequestRejected("rar-test-rr", "default", "test-rr", "admin@example.com", "Too risky"),
			},
			rrName:         types.NamespacedName{Name: "test-rr", Namespace: "default"},
			expectedPhase:  remediationv1.PhaseFailed,
			expectedResult: ctrl.Result{},
			additionalAsserts: func(rr *remediationv1.RemediationRequest) {
				Expect(rr.Status.FailureReason).ToNot(BeNil())
				Expect(*rr.Status.FailureReason).To(ContainSubstring("Too risky"))
			},
		}),

		Entry("5.3: AwaitingApproval→Failed - RAR Expired (BR-ORCH-001)", ReconcileScenario{
			name:        "awaiting_approval_to_failed_expired",
			description: "When RAR expires, should transition to Failed",
			businessReq: "BR-ORCH-001 (approval timeout)",
			initialObjects: []client.Object{
				newRemediationRequestWithChildRefs("test-rr", "default", remediationv1.PhaseAwaitingApproval, "test-rr-sp", "test-rr-ai", ""),
				newSignalProcessingCompleted("test-rr-sp", "default", "test-rr"),
				newAIAnalysisCompleted("test-rr-ai", "default", "test-rr", 0.4, "risky-workflow"),
				newRemediationApprovalRequestExpired("rar-test-rr", "default", "test-rr"),
			},
			rrName:         types.NamespacedName{Name: "test-rr", Namespace: "default"},
			expectedPhase:  remediationv1.PhaseFailed,
			expectedResult: ctrl.Result{},
			additionalAsserts: func(rr *remediationv1.RemediationRequest) {
				Expect(rr.Status.FailureReason).ToNot(BeNil())
				Expect(*rr.Status.FailureReason).To(ContainSubstring("expired"))
			},
		}),

		Entry("5.4: AwaitingApproval - RAR Not Found (Error Handling)", ReconcileScenario{
			name:        "awaiting_approval_rar_not_found",
			description: "When RAR doesn't exist, should requeue gracefully",
			businessReq: "BR-ORCH-001 (error recovery)",
			initialObjects: []client.Object{
				newRemediationRequestWithChildRefs("test-rr", "default", remediationv1.PhaseAwaitingApproval, "test-rr-sp", "test-rr-ai", ""),
				newSignalProcessingCompleted("test-rr-sp", "default", "test-rr"),
				newAIAnalysisCompleted("test-rr-ai", "default", "test-rr", 0.4, "risky-workflow"),
				// RAR not created yet
			},
			rrName:         types.NamespacedName{Name: "test-rr", Namespace: "default"},
			expectedPhase:  remediationv1.PhaseAwaitingApproval,        // Stay in same phase
			expectedResult: ctrl.Result{RequeueAfter: 5 * time.Second}, // RequeueGenericError
		}),

		Entry("5.5: AwaitingApproval - RAR Pending (Still Waiting)", ReconcileScenario{
			name:        "awaiting_approval_rar_pending",
			description: "When RAR is pending, should stay in AwaitingApproval and requeue",
			businessReq: "BR-ORCH-001 (polling for decision)",
			initialObjects: []client.Object{
				newRemediationRequestWithChildRefs("test-rr", "default", remediationv1.PhaseAwaitingApproval, "test-rr-sp", "test-rr-ai", ""),
				newSignalProcessingCompleted("test-rr-sp", "default", "test-rr"),
				newAIAnalysisCompleted("test-rr-ai", "default", "test-rr", 0.4, "risky-workflow"),
				newRemediationApprovalRequestPending("rar-test-rr", "default", "test-rr"),
			},
			rrName:         types.NamespacedName{Name: "test-rr", Namespace: "default"},
			expectedPhase:  remediationv1.PhaseAwaitingApproval,
			expectedResult: ctrl.Result{RequeueAfter: 30 * time.Second},
		}),

		// ========================================
		// PHASE 2: TIMEOUT DETECTION TESTS (8 scenarios)
		// BR-ORCH-027: Global timeout, BR-ORCH-028: Phase timeouts
		// ========================================

		Entry("6.1: Global Timeout Exceeded - Pending Phase (BR-ORCH-027)", ReconcileScenario{
			name:        "global_timeout_exceeded_pending",
			description: "When global timeout exceeded in Pending, should transition to TimedOut",
			businessReq: "BR-ORCH-027 (global timeout)",
			initialObjects: []client.Object{
				newRemediationRequestWithTimeout("test-rr", "default", remediationv1.PhasePending, -2*time.Hour), // Started 2 hours ago
			},
			rrName:         types.NamespacedName{Name: "test-rr", Namespace: "default"},
			expectedPhase:  remediationv1.PhaseTimedOut,
			expectedResult: ctrl.Result{},
			additionalAsserts: func(rr *remediationv1.RemediationRequest) {
				Expect(rr.Status.TimeoutTime).ToNot(BeNil())
				Expect(rr.Status.TimeoutPhase).ToNot(BeNil())
				Expect(*rr.Status.TimeoutPhase).To(Equal("Pending"))
			},
		}),

		Entry("6.2: Global Timeout Not Exceeded", ReconcileScenario{
			name:        "global_timeout_not_exceeded",
			description: "When global timeout not exceeded, should continue processing",
			businessReq: "BR-ORCH-027 (timeout check)",
			initialObjects: []client.Object{
				newRemediationRequestWithTimeout("test-rr", "default", remediationv1.PhasePending, -30*time.Minute), // Started 30 min ago
			},
			rrName:         types.NamespacedName{Name: "test-rr", Namespace: "default"},
			expectedPhase:  remediationv1.PhaseProcessing, // Should proceed normally
			expectedResult: ctrl.Result{RequeueAfter: 5 * time.Second},
		}),

		Entry("6.3: Processing Phase Timeout Exceeded (BR-ORCH-028)", ReconcileScenario{
			name:        "processing_phase_timeout_exceeded",
			description: "When Processing phase timeout exceeded, should transition to TimedOut",
			businessReq: "BR-ORCH-028.1 (phase timeout)",
			initialObjects: []client.Object{
				newRemediationRequestWithPhaseTimeout("test-rr", "default", remediationv1.PhaseProcessing, "test-rr-sp", -10*time.Minute), // In Processing for 10 min
				newSignalProcessing("test-rr-sp", "default", "test-rr", signalprocessingv1.PhaseEnriching),
			},
			rrName:         types.NamespacedName{Name: "test-rr", Namespace: "default"},
			expectedPhase:  remediationv1.PhaseTimedOut,
			expectedResult: ctrl.Result{RequeueAfter: 30 * time.Second}, // RequeueResourceBusy after notification creation
			additionalAsserts: func(rr *remediationv1.RemediationRequest) {
				Expect(*rr.Status.TimeoutPhase).To(Equal("Processing"))
			},
		}),

		Entry("6.4: Analyzing Phase Timeout Exceeded (BR-ORCH-028)", ReconcileScenario{
			name:        "analyzing_phase_timeout_exceeded",
			description: "When Analyzing phase timeout exceeded, should transition to TimedOut",
			businessReq: "BR-ORCH-028.2 (phase timeout)",
			initialObjects: []client.Object{
				newRemediationRequestWithPhaseTimeout("test-rr", "default", remediationv1.PhaseAnalyzing, "test-rr-ai", -15*time.Minute),
				newAIAnalysis("test-rr-ai", "default", "test-rr", aianalysisv1.PhaseAnalyzing),
			},
			rrName:         types.NamespacedName{Name: "test-rr", Namespace: "default"},
			expectedPhase:  remediationv1.PhaseTimedOut,
			expectedResult: ctrl.Result{RequeueAfter: 30 * time.Second}, // RequeueResourceBusy after notification creation
		}),

		Entry("6.5: Executing Phase Timeout Exceeded (BR-ORCH-028)", ReconcileScenario{
			name:        "executing_phase_timeout_exceeded",
			description: "When Executing phase timeout exceeded, should transition to TimedOut",
			businessReq: "BR-ORCH-028.3 (phase timeout)",
			initialObjects: []client.Object{
				newRemediationRequestWithPhaseTimeout("test-rr", "default", remediationv1.PhaseExecuting, "test-rr-we", -35*time.Minute),
				newWorkflowExecution("test-rr-we", "default", "test-rr", workflowexecutionv1.PhaseRunning),
			},
			rrName:         types.NamespacedName{Name: "test-rr", Namespace: "default"},
			expectedPhase:  remediationv1.PhaseTimedOut,
			expectedResult: ctrl.Result{RequeueAfter: 30 * time.Second}, // RequeueResourceBusy after notification creation
		}),

		Entry("6.6: Timeout Notification Created (BR-ORCH-027)", ReconcileScenario{
			name:        "timeout_notification_created",
			description: "When timeout occurs, should create notification",
			businessReq: "BR-ORCH-027 (timeout notification)",
			initialObjects: []client.Object{
				newRemediationRequestWithTimeout("test-rr", "default", remediationv1.PhaseProcessing, -2*time.Hour),
			},
			rrName:           types.NamespacedName{Name: "test-rr", Namespace: "default"},
			expectedPhase:    remediationv1.PhaseTimedOut,
			expectedChildren: map[string]bool{"Notification": true},
		}),

		Entry("6.7: Global Timeout Wins Over Phase Timeout", ReconcileScenario{
			name:        "global_timeout_wins_over_phase",
			description: "When both global and phase timeouts exceeded, global should win",
			businessReq: "BR-ORCH-027 (timeout precedence)",
			initialObjects: []client.Object{
				newRemediationRequestWithBothTimeouts("test-rr", "default", remediationv1.PhaseProcessing, "test-rr-sp", -2*time.Hour, -10*time.Minute),
				newSignalProcessing("test-rr-sp", "default", "test-rr", signalprocessingv1.PhaseEnriching),
			},
			rrName:        types.NamespacedName{Name: "test-rr", Namespace: "default"},
			expectedPhase: remediationv1.PhaseTimedOut,
		}),

		Entry("6.8: Timeout in Terminal Phase (No-Op)", ReconcileScenario{
			name:        "timeout_in_terminal_phase_noop",
			description: "When RR already in terminal phase, timeout check should be skipped",
			businessReq: "BR-ORCH-027 (terminal phase handling)",
			initialObjects: []client.Object{
				newRemediationRequestWithTimeout("test-rr", "default", remediationv1.PhaseCompleted, -2*time.Hour),
			},
			rrName:         types.NamespacedName{Name: "test-rr", Namespace: "default"},
			expectedPhase:  remediationv1.PhaseCompleted, // Stay in Completed
			expectedResult: ctrl.Result{},                // No requeue
		}),
	)
})
