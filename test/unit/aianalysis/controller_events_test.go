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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/aianalysis"
	aiaudit "github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
	aaStatus "github.com/jordigilh/kubernaut/pkg/aianalysis/status"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

// drainEvents reads all available events from the FakeRecorder channel.
func drainEvents(recorder *record.FakeRecorder) []string {
	var collected []string
	for {
		select {
		case evt := <-recorder.Events:
			collected = append(collected, evt)
		default:
			return collected
		}
	}
}

// containsEvent checks if any event string contains ALL the given substrings.
func containsEvent(eventList []string, substrings ...string) bool {
	for _, evt := range eventList {
		allMatch := true
		for _, sub := range substrings {
			if !containsStr(evt, sub) {
				allMatch = false
				break
			}
		}
		if allMatch {
			return true
		}
	}
	return false
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// DD-EVENT-001 v1.1: K8s Event Observability for AIAnalysis Controller
// BR-AA-095: All AIAnalysis lifecycle events must be emitted via Recorder.Event
// Issue: #72
var _ = Describe("AIAnalysis Controller K8s Events [DD-EVENT-001]", func() {
	var (
		ctx    context.Context
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
	})

	// UT-AA-095-01: AIAnalysisCreated event on Pending → Investigating
	// Validates the existing v1.0 event is emitted correctly
	Context("UT-AA-095-01: AIAnalysisCreated event on Pending → Investigating", func() {
		It("should emit AIAnalysisCreated Normal event when reconciling Pending phase", func() {
			recorder := record.NewFakeRecorder(20)
			mockHolmes := mocks.NewMockHolmesGPTClient()
			mockRego := mocks.NewMockRegoEvaluator()
			mockAuditStore := NewMockAuditStore()
			auditClient := aiaudit.NewAuditClient(mockAuditStore, ctrl.Log.WithName("test-audit"))
			testMetrics := metrics.NewMetrics()

			testAnalysis := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-aa-evt-01",
					Namespace:  "default",
					Finalizers: []string{"kubernaut.ai/finalizer"},
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Kind: "RemediationRequest", Name: "test-rr", Namespace: "default",
					},
					RemediationID: "test-001",
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(testAnalysis).
				WithStatusSubresource(testAnalysis).
				Build()

			// Set the phase to Pending via status update
			testAnalysis.Status.Phase = aianalysis.PhasePending
			testAnalysis.Status.Message = "AIAnalysis created"
			Expect(fakeClient.Status().Update(ctx, testAnalysis)).To(Succeed())

			statusManager := aaStatus.NewManager(fakeClient, fakeClient)
			investigatingHandler := handlers.NewInvestigatingHandler(
				mockHolmes, ctrl.Log.WithName("test"), testMetrics, auditClient,
			)
			analyzingHandler := handlers.NewAnalyzingHandler(
				mockRego, ctrl.Log.WithName("test"), testMetrics, auditClient,
			)

			reconciler := &aianalysis.AIAnalysisReconciler{
				Client:               fakeClient,
				Scheme:               scheme,
				Recorder:             recorder,
				Log:                  ctrl.Log.WithName("test"),
				Metrics:              testMetrics,
				StatusManager:        statusManager,
				InvestigatingHandler: investigatingHandler,
				AnalyzingHandler:     analyzingHandler,
				AuditClient:          auditClient,
			}

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "test-aa-evt-01", Namespace: "default"},
			}

			// Reconcile Pending phase → transitions to Investigating
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			evts := drainEvents(recorder)
			Expect(containsEvent(evts, "Normal", events.EventReasonAIAnalysisCreated, "processing started")).
				To(BeTrue(), "Expected AIAnalysisCreated event, got: %v", evts)
		})
	})

	// UT-AA-095-02: InvestigationComplete event on Investigating → Analyzing
	Context("UT-AA-095-02: InvestigationComplete event on Investigating → Analyzing", func() {
		It("should emit InvestigationComplete Normal event on successful investigation", func() {
			recorder := record.NewFakeRecorder(20)
			mockHolmes := mocks.NewMockHolmesGPTClient().WithFullResponse(
				"Root cause identified: CrashLoopBackOff due to OOM",
				0.85, []string{},
				"OOM Kill detected", "high",
				"wf-restart-pod", "kubernaut.io/workflows/restart-pod:v1.0.0", 0.85,
				"Restart pod to resolve OOM", false,
			)
			mockRego := mocks.NewMockRegoEvaluator()
			mockAuditStore := NewMockAuditStore()
			auditClient := aiaudit.NewAuditClient(mockAuditStore, ctrl.Log.WithName("test-audit"))
			testMetrics := metrics.NewMetrics()

			testAnalysis := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-aa-evt-02",
					Namespace:  "default",
					Finalizers: []string{"kubernaut.ai/finalizer"},
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Kind: "RemediationRequest", Name: "test-rr", Namespace: "default",
					},
					RemediationID: "test-002",
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(testAnalysis).
				WithStatusSubresource(testAnalysis).
				Build()

			// Set phase to Investigating
			testAnalysis.Status.Phase = aianalysis.PhaseInvestigating
			testAnalysis.Status.Message = "Investigation in progress"
			Expect(fakeClient.Status().Update(ctx, testAnalysis)).To(Succeed())

			statusManager := aaStatus.NewManager(fakeClient, fakeClient)
			investigatingHandler := handlers.NewInvestigatingHandler(
				mockHolmes, ctrl.Log.WithName("test"), testMetrics, auditClient,
			)
			analyzingHandler := handlers.NewAnalyzingHandler(
				mockRego, ctrl.Log.WithName("test"), testMetrics, auditClient,
			)

			reconciler := &aianalysis.AIAnalysisReconciler{
				Client:               fakeClient,
				Scheme:               scheme,
				Recorder:             recorder,
				Log:                  ctrl.Log.WithName("test"),
				Metrics:              testMetrics,
				StatusManager:        statusManager,
				InvestigatingHandler: investigatingHandler,
				AnalyzingHandler:     analyzingHandler,
				AuditClient:          auditClient,
			}

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "test-aa-evt-02", Namespace: "default"},
			}

			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			evts := drainEvents(recorder)
			Expect(containsEvent(evts, "Normal", events.EventReasonInvestigationComplete)).
				To(BeTrue(), "Expected InvestigationComplete event, got: %v", evts)
		})
	})

	// UT-AA-095-03: AnalysisCompleted event on Analyzing → Completed
	Context("UT-AA-095-03: AnalysisCompleted event on Analyzing → Completed", func() {
		It("should emit AnalysisCompleted Normal event on successful analysis", func() {
			recorder := record.NewFakeRecorder(20)
			mockRego := mocks.NewMockRegoEvaluator() // Default: auto-approve
			mockHolmes := mocks.NewMockHolmesGPTClient()
			mockAuditStore := NewMockAuditStore()
			auditClient := aiaudit.NewAuditClient(mockAuditStore, ctrl.Log.WithName("test-audit"))
			testMetrics := metrics.NewMetrics()

			testAnalysis := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-aa-evt-03",
					Namespace:  "default",
					Finalizers: []string{"kubernaut.ai/finalizer"},
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Kind: "RemediationRequest", Name: "test-rr", Namespace: "default",
					},
					RemediationID: "test-003",
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(testAnalysis).
				WithStatusSubresource(testAnalysis).
				Build()

			// Set phase to Analyzing with a selected workflow (required by AnalyzingHandler)
			testAnalysis.Status.Phase = aianalysis.PhaseAnalyzing
			testAnalysis.Status.Message = "Analysis in progress"
			testAnalysis.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
				WorkflowID:     "wf-restart-pod",
				ContainerImage: "kubernaut.io/workflows/restart-pod:v1.0.0",
				Confidence:     0.85,
			}
			Expect(fakeClient.Status().Update(ctx, testAnalysis)).To(Succeed())

			statusManager := aaStatus.NewManager(fakeClient, fakeClient)
			investigatingHandler := handlers.NewInvestigatingHandler(
				mockHolmes, ctrl.Log.WithName("test"), testMetrics, auditClient,
			)
			analyzingHandler := handlers.NewAnalyzingHandler(
				mockRego, ctrl.Log.WithName("test"), testMetrics, auditClient,
			)

			reconciler := &aianalysis.AIAnalysisReconciler{
				Client:               fakeClient,
				Scheme:               scheme,
				Recorder:             recorder,
				Log:                  ctrl.Log.WithName("test"),
				Metrics:              testMetrics,
				StatusManager:        statusManager,
				InvestigatingHandler: investigatingHandler,
				AnalyzingHandler:     analyzingHandler,
				AuditClient:          auditClient,
			}

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "test-aa-evt-03", Namespace: "default"},
			}

			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			evts := drainEvents(recorder)
			Expect(containsEvent(evts, "Normal", events.EventReasonAnalysisCompleted)).
				To(BeTrue(), "Expected AnalysisCompleted event, got: %v", evts)
		})
	})

	// UT-AA-095-04: AnalysisFailed event on investigation failure
	Context("UT-AA-095-04: AnalysisFailed event on investigation failure", func() {
		It("should emit AnalysisFailed Warning event when investigation fails permanently", func() {
			recorder := record.NewFakeRecorder(20)
			mockHolmes := mocks.NewMockHolmesGPTClient().
				WithError(fmt.Errorf("permanent: HolmesGPT-API returned 500"))
			mockRego := mocks.NewMockRegoEvaluator()
			mockAuditStore := NewMockAuditStore()
			auditClient := aiaudit.NewAuditClient(mockAuditStore, ctrl.Log.WithName("test-audit"))
			testMetrics := metrics.NewMetrics()

			testAnalysis := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-aa-evt-04",
					Namespace:  "default",
					Finalizers: []string{"kubernaut.ai/finalizer"},
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Kind: "RemediationRequest", Name: "test-rr", Namespace: "default",
					},
					RemediationID: "test-004",
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(testAnalysis).
				WithStatusSubresource(testAnalysis).
				Build()

			// Set phase to Investigating
			testAnalysis.Status.Phase = aianalysis.PhaseInvestigating
			testAnalysis.Status.Message = "Investigation in progress"
			Expect(fakeClient.Status().Update(ctx, testAnalysis)).To(Succeed())

			statusManager := aaStatus.NewManager(fakeClient, fakeClient)
			investigatingHandler := handlers.NewInvestigatingHandler(
				mockHolmes, ctrl.Log.WithName("test"), testMetrics, auditClient,
			)
			analyzingHandler := handlers.NewAnalyzingHandler(
				mockRego, ctrl.Log.WithName("test"), testMetrics, auditClient,
			)

			reconciler := &aianalysis.AIAnalysisReconciler{
				Client:               fakeClient,
				Scheme:               scheme,
				Recorder:             recorder,
				Log:                  ctrl.Log.WithName("test"),
				Metrics:              testMetrics,
				StatusManager:        statusManager,
				InvestigatingHandler: investigatingHandler,
				AnalyzingHandler:     analyzingHandler,
				AuditClient:          auditClient,
			}

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "test-aa-evt-04", Namespace: "default"},
			}

			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			evts := drainEvents(recorder)
			Expect(containsEvent(evts, "Warning", events.EventReasonAnalysisFailed)).
				To(BeTrue(), "Expected AnalysisFailed Warning event, got: %v", evts)
		})
	})

	// UT-AA-095-05: AnalysisFailed event on analyzing failure
	Context("UT-AA-095-05: AnalysisFailed event on analyzing failure", func() {
		It("should emit AnalysisFailed Warning event when Rego evaluation errors", func() {
			recorder := record.NewFakeRecorder(20)
			mockRego := mocks.NewMockRegoEvaluator()
			mockRego.Err = fmt.Errorf("rego evaluation failed: policy compilation error")
			mockRego.Result = nil
			mockHolmes := mocks.NewMockHolmesGPTClient()
			mockAuditStore := NewMockAuditStore()
			auditClient := aiaudit.NewAuditClient(mockAuditStore, ctrl.Log.WithName("test-audit"))
			testMetrics := metrics.NewMetrics()

			testAnalysis := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-aa-evt-05",
					Namespace:  "default",
					Finalizers: []string{"kubernaut.ai/finalizer"},
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Kind: "RemediationRequest", Name: "test-rr", Namespace: "default",
					},
					RemediationID: "test-005",
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(testAnalysis).
				WithStatusSubresource(testAnalysis).
				Build()

			// Set phase to Analyzing with a selected workflow
			testAnalysis.Status.Phase = aianalysis.PhaseAnalyzing
			testAnalysis.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
				WorkflowID:     "wf-restart-pod",
				ContainerImage: "kubernaut.io/workflows/restart-pod:v1.0.0",
				Confidence:     0.85,
			}
			Expect(fakeClient.Status().Update(ctx, testAnalysis)).To(Succeed())

			statusManager := aaStatus.NewManager(fakeClient, fakeClient)
			investigatingHandler := handlers.NewInvestigatingHandler(
				mockHolmes, ctrl.Log.WithName("test"), testMetrics, auditClient,
			)
			analyzingHandler := handlers.NewAnalyzingHandler(
				mockRego, ctrl.Log.WithName("test"), testMetrics, auditClient,
			)

			reconciler := &aianalysis.AIAnalysisReconciler{
				Client:               fakeClient,
				Scheme:               scheme,
				Recorder:             recorder,
				Log:                  ctrl.Log.WithName("test"),
				Metrics:              testMetrics,
				StatusManager:        statusManager,
				InvestigatingHandler: investigatingHandler,
				AnalyzingHandler:     analyzingHandler,
				AuditClient:          auditClient,
			}

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "test-aa-evt-05", Namespace: "default"},
			}

			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			evts := drainEvents(recorder)
			Expect(containsEvent(evts, "Warning", events.EventReasonAnalysisFailed)).
				To(BeTrue(), "Expected AnalysisFailed Warning event, got: %v", evts)
		})
	})

	// UT-AA-095-06: ApprovalRequired event
	Context("UT-AA-095-06: ApprovalRequired event", func() {
		It("should emit ApprovalRequired Normal event when Rego requires approval", func() {
			recorder := record.NewFakeRecorder(20)
			mockRego := mocks.NewMockRegoEvaluator()
			mockRego.Result = &rego.PolicyResult{
				ApprovalRequired: true,
				Reason:           "Confidence below threshold (0.85 < 0.90)",
				Degraded:         false,
			}
			mockHolmes := mocks.NewMockHolmesGPTClient()
			mockAuditStore := NewMockAuditStore()
			auditClient := aiaudit.NewAuditClient(mockAuditStore, ctrl.Log.WithName("test-audit"))
			testMetrics := metrics.NewMetrics()

			testAnalysis := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-aa-evt-06",
					Namespace:  "default",
					Finalizers: []string{"kubernaut.ai/finalizer"},
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Kind: "RemediationRequest", Name: "test-rr", Namespace: "default",
					},
					RemediationID: "test-006",
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(testAnalysis).
				WithStatusSubresource(testAnalysis).
				Build()

			// Set phase to Analyzing with a selected workflow
			testAnalysis.Status.Phase = aianalysis.PhaseAnalyzing
			testAnalysis.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
				WorkflowID:     "wf-restart-pod",
				ContainerImage: "kubernaut.io/workflows/restart-pod:v1.0.0",
				Confidence:     0.85,
			}
			Expect(fakeClient.Status().Update(ctx, testAnalysis)).To(Succeed())

			statusManager := aaStatus.NewManager(fakeClient, fakeClient)
			investigatingHandler := handlers.NewInvestigatingHandler(
				mockHolmes, ctrl.Log.WithName("test"), testMetrics, auditClient,
			)
			analyzingHandler := handlers.NewAnalyzingHandler(
				mockRego, ctrl.Log.WithName("test"), testMetrics, auditClient,
			)

			reconciler := &aianalysis.AIAnalysisReconciler{
				Client:               fakeClient,
				Scheme:               scheme,
				Recorder:             recorder,
				Log:                  ctrl.Log.WithName("test"),
				Metrics:              testMetrics,
				StatusManager:        statusManager,
				InvestigatingHandler: investigatingHandler,
				AnalyzingHandler:     analyzingHandler,
				AuditClient:          auditClient,
			}

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "test-aa-evt-06", Namespace: "default"},
			}

			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			evts := drainEvents(recorder)
			Expect(containsEvent(evts, "Normal", events.EventReasonApprovalRequired)).
				To(BeTrue(), "Expected ApprovalRequired event, got: %v", evts)
		})
	})

	// UT-AA-095-07: HumanReviewRequired event
	Context("UT-AA-095-07: HumanReviewRequired event", func() {
		It("should emit HumanReviewRequired Warning event when HAPI flags human review", func() {
			recorder := record.NewFakeRecorder(20)
			mockHolmes := mocks.NewMockHolmesGPTClient().
				WithHumanReviewRequired([]string{"Investigation inconclusive, needs human review"})
			mockRego := mocks.NewMockRegoEvaluator()
			mockAuditStore := NewMockAuditStore()
			auditClient := aiaudit.NewAuditClient(mockAuditStore, ctrl.Log.WithName("test-audit"))
			testMetrics := metrics.NewMetrics()

			testAnalysis := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-aa-evt-07",
					Namespace:  "default",
					Finalizers: []string{"kubernaut.ai/finalizer"},
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Kind: "RemediationRequest", Name: "test-rr", Namespace: "default",
					},
					RemediationID: "test-007",
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(testAnalysis).
				WithStatusSubresource(testAnalysis).
				Build()

			// Set phase to Investigating
			testAnalysis.Status.Phase = aianalysis.PhaseInvestigating
			testAnalysis.Status.Message = "Investigation in progress"
			Expect(fakeClient.Status().Update(ctx, testAnalysis)).To(Succeed())

			statusManager := aaStatus.NewManager(fakeClient, fakeClient)
			investigatingHandler := handlers.NewInvestigatingHandler(
				mockHolmes, ctrl.Log.WithName("test"), testMetrics, auditClient,
			)
			analyzingHandler := handlers.NewAnalyzingHandler(
				mockRego, ctrl.Log.WithName("test"), testMetrics, auditClient,
			)

			reconciler := &aianalysis.AIAnalysisReconciler{
				Client:               fakeClient,
				Scheme:               scheme,
				Recorder:             recorder,
				Log:                  ctrl.Log.WithName("test"),
				Metrics:              testMetrics,
				StatusManager:        statusManager,
				InvestigatingHandler: investigatingHandler,
				AnalyzingHandler:     analyzingHandler,
				AuditClient:          auditClient,
			}

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "test-aa-evt-07", Namespace: "default"},
			}

			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			evts := drainEvents(recorder)
			Expect(containsEvent(evts, "Warning", events.EventReasonHumanReviewRequired)).
				To(BeTrue(), "Expected HumanReviewRequired Warning event, got: %v", evts)
		})
	})

	// UT-AA-095-08: PhaseTransition event (intermediate)
	Context("UT-AA-095-08: PhaseTransition event (intermediate)", func() {
		It("should emit PhaseTransition event with from/to phases on Pending → Investigating", func() {
			recorder := record.NewFakeRecorder(20)
			mockHolmes := mocks.NewMockHolmesGPTClient()
			mockRego := mocks.NewMockRegoEvaluator()
			mockAuditStore := NewMockAuditStore()
			auditClient := aiaudit.NewAuditClient(mockAuditStore, ctrl.Log.WithName("test-audit"))
			testMetrics := metrics.NewMetrics()

			testAnalysis := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-aa-evt-08",
					Namespace:  "default",
					Finalizers: []string{"kubernaut.ai/finalizer"},
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Kind: "RemediationRequest", Name: "test-rr", Namespace: "default",
					},
					RemediationID: "test-008",
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(testAnalysis).
				WithStatusSubresource(testAnalysis).
				Build()

			// Set phase to Pending
			testAnalysis.Status.Phase = aianalysis.PhasePending
			testAnalysis.Status.Message = "AIAnalysis created"
			Expect(fakeClient.Status().Update(ctx, testAnalysis)).To(Succeed())

			statusManager := aaStatus.NewManager(fakeClient, fakeClient)
			investigatingHandler := handlers.NewInvestigatingHandler(
				mockHolmes, ctrl.Log.WithName("test"), testMetrics, auditClient,
			)
			analyzingHandler := handlers.NewAnalyzingHandler(
				mockRego, ctrl.Log.WithName("test"), testMetrics, auditClient,
			)

			reconciler := &aianalysis.AIAnalysisReconciler{
				Client:               fakeClient,
				Scheme:               scheme,
				Recorder:             recorder,
				Log:                  ctrl.Log.WithName("test"),
				Metrics:              testMetrics,
				StatusManager:        statusManager,
				InvestigatingHandler: investigatingHandler,
				AnalyzingHandler:     analyzingHandler,
				AuditClient:          auditClient,
			}

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "test-aa-evt-08", Namespace: "default"},
			}

			// Reconcile: Pending → Investigating
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			evts := drainEvents(recorder)
			Expect(containsEvent(evts, "Normal", events.EventReasonPhaseTransition, "Pending", "Investigating")).
				To(BeTrue(), "Expected PhaseTransition event with Pending→Investigating, got: %v", evts)
		})
	})
})
