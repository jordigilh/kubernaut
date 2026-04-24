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

package controller

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	remediationworkflowv1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	prodcontroller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/routing"
	"github.com/prometheus/client_golang/prometheus"
)

// ErrorRoutingEngine returns configurable errors/blocks from routing methods.
type ErrorRoutingEngine struct {
	MockRoutingEngine
	PreAnalysisErr       error
	PostAnalysisErr      error
	PostAnalysisBlock    *routing.BlockingCondition
}

func (e *ErrorRoutingEngine) CheckPreAnalysisConditions(_ context.Context, _ *remediationv1.RemediationRequest) (*routing.BlockingCondition, error) {
	return nil, e.PreAnalysisErr
}

func (e *ErrorRoutingEngine) CheckPostAnalysisConditions(_ context.Context, _ *remediationv1.RemediationRequest, _ string, _ string, _ string, _ string) (*routing.BlockingCondition, error) {
	if e.PostAnalysisErr != nil {
		return nil, e.PostAnalysisErr
	}
	return e.PostAnalysisBlock, nil
}

func newCharScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = remediationv1.AddToScheme(scheme)
	_ = signalprocessingv1.AddToScheme(scheme)
	_ = aianalysisv1.AddToScheme(scheme)
	_ = workflowexecutionv1.AddToScheme(scheme)
	_ = notificationv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = eav1.AddToScheme(scheme)
	_ = remediationworkflowv1.AddToScheme(scheme)
	return scheme
}

func newCharTimeouts() prodcontroller.TimeoutConfig {
	return prodcontroller.TimeoutConfig{
		Global:     1 * time.Hour,
		Processing: 5 * time.Minute,
		Analyzing:  10 * time.Minute,
		Executing:  30 * time.Minute,
	}
}

func newCharReconciler(c client.Client, apiReader client.Reader, scheme *runtime.Scheme, routingEngine routing.Engine) (*prodcontroller.Reconciler, *record.FakeRecorder) {
	recorder := record.NewFakeRecorder(20)
	r := prodcontroller.NewReconciler(
		c,
		apiReader,
		scheme,
		nil,
		recorder,
		rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
		newCharTimeouts(),
		routingEngine,
	)
	return r, recorder
}

var _ = Describe("Issue #666: Characterization Tests for RO Phase Handler Migration", func() {
	var (
		ctx    context.Context
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = newCharScheme()
	})

	// ========================================================================
	// Group 1: handlePendingPhase characterization tests
	// ========================================================================
	Describe("Group 1: handlePendingPhase", func() {

		It("UT-RO-CHAR-PND-001: routing engine error returns requeue", func() {
			rr := newRemediationRequest("pnd001", "default", remediationv1.PhasePending)
			rr.Status.StartTime = ptrMetaTime(time.Now())

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			routingErr := fmt.Errorf("simulated routing failure")
			reconciler, _ := newCharReconciler(fakeClient, fakeClient, scheme, &ErrorRoutingEngine{
				PreAnalysisErr: routingErr,
			})

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "pnd001", Namespace: "default"},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(5 * time.Second), "should requeue after RequeueGenericError (5s)")

			var finalRR remediationv1.RemediationRequest
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "pnd001", Namespace: "default"}, &finalRR)).To(Succeed())
			Expect(finalRR.Status.OverallPhase).To(Equal(remediationv1.PhasePending), "should remain Pending on routing error")

			var spList signalprocessingv1.SignalProcessingList
			Expect(fakeClient.List(ctx, &spList, client.InNamespace("default"))).To(Succeed())
			Expect(spList.Items).To(BeEmpty(), "should NOT create SignalProcessing on routing error")
		})

		It("UT-RO-CHAR-PND-002: SP create error returns 5s requeue and remains Pending", func() {
			rr := newRemediationRequest("pnd002", "default", remediationv1.PhasePending)
			rr.Status.StartTime = ptrMetaTime(time.Now())

			spCreateErr := fmt.Errorf("simulated SP creation failure")
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						if _, ok := obj.(*signalprocessingv1.SignalProcessing); ok {
							return spCreateErr
						}
						return c.Create(ctx, obj, opts...)
					},
				}).
				Build()

			reconciler, _ := newCharReconciler(fakeClient, fakeClient, scheme, &MockRoutingEngine{})

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "pnd002", Namespace: "default"},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(5 * time.Second), "should requeue after RequeueGenericError (5s)")

			var finalRR remediationv1.RemediationRequest
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "pnd002", Namespace: "default"}, &finalRR)).To(Succeed())
			Expect(finalRR.Status.OverallPhase).To(Equal(remediationv1.PhasePending), "should remain Pending on SP create error")
		})

		It("UT-RO-CHAR-PND-003: SP create in terminating namespace returns no requeue", func() {
			rr := newRemediationRequest("pnd003", "default", remediationv1.PhasePending)
			rr.Status.StartTime = ptrMetaTime(time.Now())

			nsTerminatingErr := fmt.Errorf("namespace default is being terminated")
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						if _, ok := obj.(*signalprocessingv1.SignalProcessing); ok {
							return nsTerminatingErr
						}
						return c.Create(ctx, obj, opts...)
					},
				}).
				Build()

			reconciler, _ := newCharReconciler(fakeClient, fakeClient, scheme, &MockRoutingEngine{})

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "pnd003", Namespace: "default"},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}), "should return empty result (no requeue) for terminating namespace")
		})

		It("UT-RO-CHAR-PND-004: UpdateRemediationRequestStatus failure after SP create returns requeue", func() {
			rr := newRemediationRequest("pnd004", "default", remediationv1.PhasePending)
			rr.Status.StartTime = ptrMetaTime(time.Now())

			statusUpdateCallCount := 0
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr).
				WithStatusSubresource(
					&remediationv1.RemediationRequest{},
					&signalprocessingv1.SignalProcessing{},
				).
				WithInterceptorFuncs(interceptor.Funcs{
					SubResourceUpdate: func(ctx context.Context, c client.Client, subResourceName string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
						if _, ok := obj.(*remediationv1.RemediationRequest); ok {
							statusUpdateCallCount++
							// The first status update after SP create sets the SignalProcessingRef.
							// We need to figure out which call this is. The SP create succeeds,
							// then the ref-setting update fails.
							// AtomicStatusUpdate for init may have already run if phase was empty,
							// but our RR has PhasePending + ObservedGeneration set, so init is skipped.
							// The first SubResourceUpdate for RR after SP create is the ref-setting call.
							if statusUpdateCallCount >= 1 {
								return fmt.Errorf("simulated status update failure")
							}
						}
						return c.SubResource(subResourceName).Update(ctx, obj, opts...)
					},
				}).
				Build()

			reconciler, _ := newCharReconciler(fakeClient, fakeClient, scheme, &MockRoutingEngine{})

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "pnd004", Namespace: "default"},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(5 * time.Second), "should requeue after RequeueGenericError (5s)")
		})
	})

	// ========================================================================
	// Group 2: handleProcessingPhase characterization tests
	// ========================================================================
	Describe("Group 2: handleProcessingPhase", func() {

		It("UT-RO-CHAR-PRC-001: SP ref set but SP object missing returns 5s requeue", func() {
			rr := newRemediationRequestWithChildRefs("prc001", "default",
				remediationv1.PhaseProcessing, "sp-prc001", "", "")

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			reconciler, _ := newCharReconciler(fakeClient, fakeClient, scheme, &MockRoutingEngine{})

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "prc001", Namespace: "default"},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(5*time.Second), "should requeue at RequeueGenericError (5s) when SP object is missing")

			var finalRR remediationv1.RemediationRequest
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "prc001", Namespace: "default"}, &finalRR)).To(Succeed())
			Expect(finalRR.Status.OverallPhase).To(Equal(remediationv1.PhaseProcessing),
				"should remain Processing when SP is not found (will retry)")
		})

		It("UT-RO-CHAR-PRC-002: AI create error returns 5s requeue and remains Processing", func() {
			rr := newRemediationRequestWithChildRefs("prc002", "default",
				remediationv1.PhaseProcessing, "sp-prc002", "", "")

			sp := newSignalProcessingCompleted("sp-prc002", "default", "prc002")

			aiCreateErr := fmt.Errorf("simulated AI creation failure")
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr, sp).
				WithStatusSubresource(
					&remediationv1.RemediationRequest{},
					&signalprocessingv1.SignalProcessing{},
				).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						if _, ok := obj.(*aianalysisv1.AIAnalysis); ok {
							return aiCreateErr
						}
						return c.Create(ctx, obj, opts...)
					},
				}).
				Build()

			reconciler, _ := newCharReconciler(fakeClient, fakeClient, scheme, &MockRoutingEngine{})

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "prc002", Namespace: "default"},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(5 * time.Second), "should requeue after RequeueGenericError (5s)")

			var finalRR remediationv1.RemediationRequest
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "prc002", Namespace: "default"}, &finalRR)).To(Succeed())
			Expect(finalRR.Status.OverallPhase).To(Equal(remediationv1.PhaseProcessing),
				"should remain Processing on AI create error")

			var aiList aianalysisv1.AIAnalysisList
			Expect(fakeClient.List(ctx, &aiList, client.InNamespace("default"))).To(Succeed())
			Expect(aiList.Items).To(BeEmpty(), "should NOT have created an AIAnalysis object")
		})
	})

	// ========================================================================
	// Group 3: handleAnalyzingPhase characterization tests
	// ========================================================================
	Describe("Group 3: handleAnalyzingPhase", func() {

		It("UT-RO-CHAR-ANZ-001: AI Get NotFound (ref set, object missing) returns 5s requeue", func() {
			rr := newRemediationRequestWithChildRefs("anz001", "default",
				remediationv1.PhaseAnalyzing, "sp-anz001", "ai-anz001", "")

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			reconciler, _ := newCharReconciler(fakeClient, fakeClient, scheme, &MockRoutingEngine{})

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "anz001", Namespace: "default"},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(5*time.Second), "AI NotFound should requeue at RequeueGenericError (5s)")

			var finalRR remediationv1.RemediationRequest
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "anz001", Namespace: "default"}, &finalRR)).To(Succeed())
			Expect(finalRR.Status.OverallPhase).To(Equal(remediationv1.PhaseAnalyzing), "should remain Analyzing")
		})

		It("UT-RO-CHAR-ANZ-002: AI in-progress with ref set returns 10s requeue", func() {
			rr := newRemediationRequestWithChildRefs("anz002", "default",
				remediationv1.PhaseAnalyzing, "sp-anz002", "ai-anz002", "")

			ai := newAIAnalysis("ai-anz002", "default", "anz002", "Investigating")

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr, ai).
				WithStatusSubresource(&remediationv1.RemediationRequest{}, &aianalysisv1.AIAnalysis{}).
				Build()

			reconciler, _ := newCharReconciler(fakeClient, fakeClient, scheme, &MockRoutingEngine{})

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "anz002", Namespace: "default"},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(10*time.Second), "AI in-progress should requeue at 10s")

			var finalRR remediationv1.RemediationRequest
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "anz002", Namespace: "default"}, &finalRR)).To(Succeed())
			Expect(finalRR.Status.OverallPhase).To(Equal(remediationv1.PhaseAnalyzing), "should remain Analyzing")
		})

		It("UT-RO-CHAR-ANZ-003: CheckPostAnalysisConditions error returns 5s requeue", func() {
			rr := newRemediationRequestWithChildRefs("anz003", "default",
				remediationv1.PhaseAnalyzing, "sp-anz003", "ai-anz003", "")

			ai := newAIAnalysisCompleted("ai-anz003", "default", "anz003", 0.95, "wf-test")

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr, ai).
				WithStatusSubresource(&remediationv1.RemediationRequest{}, &aianalysisv1.AIAnalysis{}).
				Build()

			reconciler, _ := newCharReconciler(fakeClient, fakeClient, scheme, &ErrorRoutingEngine{
				PostAnalysisErr: fmt.Errorf("simulated post-analysis routing failure"),
			})
			reconciler.SetRESTMapper(newTestRESTMapper())

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "anz003", Namespace: "default"},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(5*time.Second), "routing error should requeue at RequeueGenericError (5s)")

			var finalRR remediationv1.RemediationRequest
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "anz003", Namespace: "default"}, &finalRR)).To(Succeed())
			Expect(finalRR.Status.OverallPhase).To(Equal(remediationv1.PhaseAnalyzing), "should remain Analyzing on routing error")
		})

		It("UT-RO-CHAR-ANZ-005: WE create failure returns 5s requeue and remains Analyzing", func() {
			rr := newRemediationRequestWithChildRefs("anz005", "default",
				remediationv1.PhaseAnalyzing, "sp-anz005", "ai-anz005", "")

			ai := newAIAnalysisCompleted("ai-anz005", "default", "anz005", 0.95, "wf-test")

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr, ai).
				WithStatusSubresource(&remediationv1.RemediationRequest{}, &aianalysisv1.AIAnalysis{}).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						if _, ok := obj.(*workflowexecutionv1.WorkflowExecution); ok {
							return fmt.Errorf("simulated WE creation failure")
						}
						return c.Create(ctx, obj, opts...)
					},
				}).
				Build()

			reconciler, _ := newCharReconciler(fakeClient, fakeClient, scheme, &MockRoutingEngine{})
			reconciler.SetRESTMapper(newTestRESTMapper())

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "anz005", Namespace: "default"},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(5*time.Second), "WE create failure should requeue at RequeueGenericError (5s)")

			var finalRR remediationv1.RemediationRequest
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "anz005", Namespace: "default"}, &finalRR)).To(Succeed())
			Expect(finalRR.Status.OverallPhase).To(Equal(remediationv1.PhaseAnalyzing), "should remain Analyzing on WE create error")

			var weList workflowexecutionv1.WorkflowExecutionList
			Expect(fakeClient.List(ctx, &weList, client.InNamespace("default"))).To(Succeed())
			Expect(weList.Items).To(BeEmpty(), "should NOT have created a WorkflowExecution object")
		})
	})

	// ========================================================================
	// Group 4: handleAwaitingApprovalPhase characterization tests
	// ========================================================================
	Describe("Group 4: handleAwaitingApprovalPhase", func() {

		It("UT-RO-CHAR-APR-001: RAR Get non-NotFound error returns error", func() {
			rr := newRemediationRequestWithChildRefs("apr001", "default",
				remediationv1.PhaseAwaitingApproval, "sp-apr001", "ai-apr001", "")

			rarGetErr := fmt.Errorf("simulated API server failure")
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				WithInterceptorFuncs(interceptor.Funcs{
					Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						if _, ok := obj.(*remediationv1.RemediationApprovalRequest); ok {
							return rarGetErr
						}
						return c.Get(ctx, key, obj, opts...)
					},
				}).
				Build()

			reconciler, _ := newCharReconciler(fakeClient, fakeClient, scheme, &MockRoutingEngine{})

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "apr001", Namespace: "default"},
			})

			Expect(err).To(HaveOccurred(), "non-NotFound RAR Get error should be propagated")
			Expect(err.Error()).To(ContainSubstring("simulated API server failure"))
		})

		It("UT-RO-CHAR-APR-002: AIAnalysisRef nil after approval returns 5s requeue", func() {
			rr := newRemediationRequestWithChildRefs("apr002", "default",
				remediationv1.PhaseAwaitingApproval, "sp-apr002", "", "")

			rar := newRemediationApprovalRequestApproved("rar-apr002", "default", "apr002", "admin")

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr, rar).
				WithStatusSubresource(
					&remediationv1.RemediationRequest{},
					&remediationv1.RemediationApprovalRequest{},
				).
				Build()

			reconciler, _ := newCharReconciler(fakeClient, fakeClient, scheme, &MockRoutingEngine{})

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "apr002", Namespace: "default"},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(5*time.Second),
				"AIAnalysisRef nil should requeue at RequeueGenericError (5s)")

			var finalRR remediationv1.RemediationRequest
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "apr002", Namespace: "default"}, &finalRR)).To(Succeed())
			Expect(finalRR.Status.OverallPhase).To(Equal(remediationv1.PhaseAwaitingApproval),
				"should remain AwaitingApproval")
		})

		It("UT-RO-CHAR-APR-003: WE create failure after approval returns 5s requeue and remains AwaitingApproval", func() {
			rr := newRemediationRequestWithChildRefs("apr003", "default",
				remediationv1.PhaseAwaitingApproval, "sp-apr003", "ai-apr003", "")

			rar := newRemediationApprovalRequestApproved("rar-apr003", "default", "apr003", "admin")
			ai := newAIAnalysisCompleted("ai-apr003", "default", "apr003", 0.95, "wf-test")

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr, rar, ai).
				WithStatusSubresource(
					&remediationv1.RemediationRequest{},
					&remediationv1.RemediationApprovalRequest{},
					&aianalysisv1.AIAnalysis{},
				).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						if _, ok := obj.(*workflowexecutionv1.WorkflowExecution); ok {
							return fmt.Errorf("simulated WE creation failure")
						}
						return c.Create(ctx, obj, opts...)
					},
				}).
				Build()

			reconciler, _ := newCharReconciler(fakeClient, fakeClient, scheme, &MockRoutingEngine{})
			reconciler.SetRESTMapper(newTestRESTMapper())

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "apr003", Namespace: "default"},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(5*time.Second),
				"WE create failure after approval should requeue at RequeueGenericError (5s)")

			var finalRR remediationv1.RemediationRequest
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "apr003", Namespace: "default"}, &finalRR)).To(Succeed())
			Expect(finalRR.Status.OverallPhase).To(Equal(remediationv1.PhaseAwaitingApproval),
				"should remain AwaitingApproval on WE create error")
		})
	})

	// ========================================================================
	// Group 5: handleExecutingPhase characterization tests
	// ========================================================================
	Describe("Group 5: handleExecutingPhase", func() {

		It("UT-RO-CHAR-EXE-001: aggregator WE phase empty + unhealthy transitions to Failed", func() {
			rr := newRemediationRequestWithChildRefs("exe001", "default",
				remediationv1.PhaseExecuting, "sp-exe001", "ai-exe001", "we-exe001")

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			reconciler, _ := newCharReconciler(fakeClient, fakeClient, scheme, &MockRoutingEngine{})

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "exe001", Namespace: "default"},
			})

			Expect(err).ToNot(HaveOccurred())

			var finalRR remediationv1.RemediationRequest
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "exe001", Namespace: "default"}, &finalRR)).To(Succeed())
			Expect(finalRR.Status.OverallPhase).To(Equal(remediationv1.PhaseFailed),
				"WE missing (phase empty + unhealthy) should transition to Failed")
			Expect(finalRR.Status.FailurePhase).ToNot(BeNil(), "FailurePhase should be set")
			Expect(*finalRR.Status.FailurePhase).To(Equal(remediationv1.FailurePhaseWorkflowExecution),
				"FailurePhase should be WorkflowExecution")
			Expect(result).To(Equal(ctrl.Result{}), "transitionToFailed returns empty result (no requeue)")
		})
	})

	// ========================================================================
	// Group 6: handleVerifyingPhase characterization tests
	// ========================================================================
	Describe("Group 6: handleVerifyingPhase", func() {

		It("UT-RO-CHAR-VER-001: EA Get error returns 30s requeue", func() {
			rr := newRemediationRequestWithChildRefs("ver001", "default",
				remediationv1.PhaseVerifying, "sp-ver001", "ai-ver001", "we-ver001")
			rr.Status.Outcome = "Remediated"
			eaRef := corev1.ObjectReference{
				APIVersion: eav1.GroupVersion.String(),
				Kind:       "EffectivenessAssessment",
				Name:       "ea-ver001",
				Namespace:  "default",
			}
			rr.Status.EffectivenessAssessmentRef = &eaRef

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			reconciler, _ := newCharReconciler(fakeClient, fakeClient, scheme, &MockRoutingEngine{})

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "ver001", Namespace: "default"},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(30*time.Second), "EA Get error should requeue at RequeueResourceBusy (30s)")

			var finalRR remediationv1.RemediationRequest
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "ver001", Namespace: "default"}, &finalRR)).To(Succeed())
			Expect(finalRR.Status.OverallPhase).To(Equal(remediationv1.PhaseVerifying), "should remain Verifying")
		})

		It("UT-RO-CHAR-VER-002: EA in progress (non-terminal phase) returns 30s requeue", func() {
			rr := newRemediationRequestWithChildRefs("ver002", "default",
				remediationv1.PhaseVerifying, "sp-ver002", "ai-ver002", "we-ver002")
			rr.Status.Outcome = "Remediated"
			eaRef := corev1.ObjectReference{
				APIVersion: eav1.GroupVersion.String(),
				Kind:       "EffectivenessAssessment",
				Name:       "ea-ver002",
				Namespace:  "default",
			}
			rr.Status.EffectivenessAssessmentRef = &eaRef
			deadline := metav1.NewTime(time.Now().Add(10 * time.Minute))
			rr.Status.VerificationDeadline = &deadline

			ea := &eav1.EffectivenessAssessment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ea-ver002",
					Namespace: "default",
				},
				Status: eav1.EffectivenessAssessmentStatus{
					Phase: eav1.PhasePending,
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr, ea).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			reconciler, _ := newCharReconciler(fakeClient, fakeClient, scheme, &MockRoutingEngine{})

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "ver002", Namespace: "default"},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(30*time.Second), "EA in progress should requeue at 30s")

			var finalRR remediationv1.RemediationRequest
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "ver002", Namespace: "default"}, &finalRR)).To(Succeed())
			Expect(finalRR.Status.OverallPhase).To(Equal(remediationv1.PhaseVerifying), "should remain Verifying")
		})

		It("UT-RO-CHAR-VER-003: nil EA creator causes EA ref unset path, returns 30s requeue", func() {
			rr := newRemediationRequestWithChildRefs("ver003", "default",
				remediationv1.PhaseVerifying, "sp-ver003", "ai-ver003", "we-ver003")
			rr.Status.Outcome = "Remediated"

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			reconciler, _ := newCharReconciler(fakeClient, fakeClient, scheme, &MockRoutingEngine{})

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "ver003", Namespace: "default"},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(30*time.Second), "should requeue at RequeueResourceBusy (30s) even with tracking errors")

			var finalRR remediationv1.RemediationRequest
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "ver003", Namespace: "default"}, &finalRR)).To(Succeed())
			Expect(finalRR.Status.OverallPhase).To(Equal(remediationv1.PhaseVerifying), "should remain Verifying (non-fatal)")
		})

		It("UT-RO-CHAR-VER-004: ValidityDeadline not yet set, age under timeout, returns 30s requeue", func() {
			rr := newRemediationRequestWithChildRefs("ver004", "default",
				remediationv1.PhaseVerifying, "sp-ver004", "ai-ver004", "we-ver004")
			rr.Status.Outcome = "Remediated"
			eaRef := corev1.ObjectReference{
				APIVersion: eav1.GroupVersion.String(),
				Kind:       "EffectivenessAssessment",
				Name:       "ea-ver004",
				Namespace:  "default",
			}
			rr.Status.EffectivenessAssessmentRef = &eaRef

			ea := &eav1.EffectivenessAssessment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ea-ver004",
					Namespace: "default",
				},
				Status: eav1.EffectivenessAssessmentStatus{
					Phase: eav1.PhasePending,
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr, ea).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			reconciler, _ := newCharReconciler(fakeClient, fakeClient, scheme, &MockRoutingEngine{})

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "ver004", Namespace: "default"},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(30*time.Second), "ValidityDeadline not set yet should requeue at 30s")

			var finalRR remediationv1.RemediationRequest
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "ver004", Namespace: "default"}, &finalRR)).To(Succeed())
			Expect(finalRR.Status.OverallPhase).To(Equal(remediationv1.PhaseVerifying), "should remain Verifying")
			Expect(finalRR.Status.VerificationDeadline).To(BeNil(), "VerificationDeadline should not be set yet")
		})
	})

	// ========================================================================
	// Group 7: handleBlockedPhase characterization tests
	// ========================================================================
	Describe("Group 7: handleBlockedPhase", func() {

		It("UT-RO-CHAR-BLK-001: ResourceBusy with terminal blocking WFE clears block", func() {
			rr := newRemediationRequestWithChildRefs("blk001", "default",
				remediationv1.PhaseBlocked, "sp-blk001", "ai-blk001", "")
			rr.Status.BlockReason = remediationv1.BlockReasonResourceBusy
			rr.Status.BlockingWorkflowExecution = "we-blocking-blk001"
			rr.Status.BlockMessage = "Target resource busy"

			blockingWE := newWorkflowExecutionCompleted("we-blocking-blk001", "default", "other-rr")

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr, blockingWE).
				WithStatusSubresource(
					&remediationv1.RemediationRequest{},
					&workflowexecutionv1.WorkflowExecution{},
				).
				Build()

			reconciler, _ := newCharReconciler(fakeClient, fakeClient, scheme, &MockRoutingEngine{})

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "blk001", Namespace: "default"},
			})

			Expect(err).ToNot(HaveOccurred())

			var finalRR remediationv1.RemediationRequest
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "blk001", Namespace: "default"}, &finalRR)).To(Succeed())
			Expect(finalRR.Status.OverallPhase).To(Equal(remediationv1.PhaseAnalyzing),
				"should clear ResourceBusy block and return to Analyzing")
			Expect(string(finalRR.Status.BlockReason)).To(BeEmpty(),
				"BlockReason should be cleared")
			Expect(result).To(Equal(ctrl.Result{Requeue: true}),
				"clearEventBasedBlock returns Requeue:true for immediate re-reconcile")
		})

		It("UT-RO-CHAR-BLK-002: ResourceBusy with missing WFE clears block", func() {
			rr := newRemediationRequestWithChildRefs("blk002", "default",
				remediationv1.PhaseBlocked, "sp-blk002", "ai-blk002", "")
			rr.Status.BlockReason = remediationv1.BlockReasonResourceBusy
			rr.Status.BlockingWorkflowExecution = "we-gone-blk002"
			rr.Status.BlockMessage = "Target resource busy"

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			reconciler, _ := newCharReconciler(fakeClient, fakeClient, scheme, &MockRoutingEngine{})

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "blk002", Namespace: "default"},
			})

			Expect(err).ToNot(HaveOccurred())

			var finalRR remediationv1.RemediationRequest
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "blk002", Namespace: "default"}, &finalRR)).To(Succeed())
			Expect(finalRR.Status.OverallPhase).To(Equal(remediationv1.PhaseAnalyzing),
				"should clear ResourceBusy block (WFE gone) and return to Analyzing")
			Expect(string(finalRR.Status.BlockReason)).To(BeEmpty(),
				"BlockReason should be cleared")
			Expect(result).To(Equal(ctrl.Result{Requeue: true}),
				"clearEventBasedBlock returns Requeue:true for immediate re-reconcile")
		})

		It("UT-RO-CHAR-BLK-003: ResourceBusy with active WFE requeues at 30s", func() {
			rr := newRemediationRequestWithChildRefs("blk003", "default",
				remediationv1.PhaseBlocked, "sp-blk003", "ai-blk003", "")
			rr.Status.BlockReason = remediationv1.BlockReasonResourceBusy
			rr.Status.BlockingWorkflowExecution = "we-active-blk003"
			rr.Status.BlockMessage = "Target resource busy"

			activeWE := newWorkflowExecution("we-active-blk003", "default", "other-rr", workflowexecutionv1.PhaseRunning)

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr, activeWE).
				WithStatusSubresource(
					&remediationv1.RemediationRequest{},
					&workflowexecutionv1.WorkflowExecution{},
				).
				Build()

			reconciler, _ := newCharReconciler(fakeClient, fakeClient, scheme, &MockRoutingEngine{})

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "blk003", Namespace: "default"},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(30*time.Second),
				"active blocking WFE should requeue at RequeueResourceBusy (30s)")

			var finalRR remediationv1.RemediationRequest
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "blk003", Namespace: "default"}, &finalRR)).To(Succeed())
			Expect(finalRR.Status.OverallPhase).To(Equal(remediationv1.PhaseBlocked),
				"should remain Blocked")
		})

		It("UT-RO-CHAR-BLK-004: IneffectiveChain via post-analysis sets Outcome+RequiresManualReview", func() {
			rr := newRemediationRequestWithChildRefs("blk004", "default",
				remediationv1.PhaseAnalyzing, "sp-blk004", "ai-blk004", "")

			ai := newAIAnalysisCompleted("ai-blk004", "default", "blk004", 0.95, "wf-test")

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr, ai).
				WithStatusSubresource(&remediationv1.RemediationRequest{}, &aianalysisv1.AIAnalysis{}).
				Build()

			reconciler, _ := newCharReconciler(fakeClient, fakeClient, scheme, &ErrorRoutingEngine{
				PostAnalysisBlock: &routing.BlockingCondition{
					Blocked:      true,
					Reason:       string(remediationv1.BlockReasonIneffectiveChain),
					Message:      "Consecutive remediation attempts ineffective",
					RequeueAfter: 0,
				},
			})
			reconciler.SetRESTMapper(newTestRESTMapper())

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "blk004", Namespace: "default"},
			})

			Expect(err).ToNot(HaveOccurred())

			var finalRR remediationv1.RemediationRequest
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "blk004", Namespace: "default"}, &finalRR)).To(Succeed())
			Expect(finalRR.Status.OverallPhase).To(Equal(remediationv1.PhaseBlocked),
				"should transition to Blocked on IneffectiveChain")
			Expect(string(finalRR.Status.BlockReason)).To(Equal(string(remediationv1.BlockReasonIneffectiveChain)),
				"BlockReason should be IneffectiveChain")
			Expect(finalRR.Status.Outcome).To(Equal("ManualReviewRequired"),
				"Outcome should be ManualReviewRequired for IneffectiveChain")
			Expect(finalRR.Status.RequiresManualReview).To(BeTrue(),
				"RequiresManualReview should be true for IneffectiveChain")
			Expect(result).To(Equal(ctrl.Result{}),
				"handleBlocked with RequeueAfter=0 should return empty result")
		})
	})

	// ========================================================================
	// Group 9: Cross-cutting characterization tests
	// ========================================================================
	Describe("Group 9: Cross-cutting", func() {

		It("UT-RO-CHAR-XCT-001: terminal Ready safety net sets Ready on terminal RR without Ready condition", func() {
			rr := newRemediationRequest("xct001", "default", remediationv1.PhaseFailed)
			rr.Status.StartTime = ptrMetaTime(time.Now())
			reason := "test failure"
			rr.Status.FailureReason = &reason

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			reconciler, _ := newCharReconciler(fakeClient, fakeClient, scheme, &MockRoutingEngine{})

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "xct001", Namespace: "default"},
			})
			Expect(err).ToNot(HaveOccurred())

			var finalRR remediationv1.RemediationRequest
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "xct001", Namespace: "default"}, &finalRR)).To(Succeed())
			Expect(finalRR.Status.OverallPhase).To(Equal(remediationv1.PhaseFailed), "should remain Failed")

			hasReady := false
			for _, c := range finalRR.Status.Conditions {
				if c.Type == "Ready" {
					hasReady = true
					Expect(c.Status).To(Equal(metav1.ConditionFalse), "Failed RR should have Ready=False")
				}
			}
			Expect(hasReady).To(BeTrue(), "terminal RR without Ready condition should get one via safety net")
		})

		It("UT-RO-CHAR-XCT-002: phase start times set on transitions", func() {
			rr := newRemediationRequest("xct002", "default", remediationv1.PhasePending)
			rr.Status.StartTime = ptrMetaTime(time.Now())

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr).
				WithStatusSubresource(
					&remediationv1.RemediationRequest{},
					&signalprocessingv1.SignalProcessing{},
				).
				Build()

			reconciler, _ := newCharReconciler(fakeClient, fakeClient, scheme, &MockRoutingEngine{})

			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "xct002", Namespace: "default"},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(5*time.Second),
				"transitionPhase to Processing requeues at RequeueGenericError (5s)")

			var finalRR remediationv1.RemediationRequest
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "xct002", Namespace: "default"}, &finalRR)).To(Succeed())
			Expect(finalRR.Status.OverallPhase).To(Equal(remediationv1.PhaseProcessing),
				"should transition from Pending to Processing")
			Expect(finalRR.Status.ProcessingStartTime).ToNot(BeNil(),
				"ProcessingStartTime should be set on Pending->Processing transition")
		})
	})
})

func ptrMetaTime(t time.Time) *metav1.Time {
	mt := metav1.NewTime(t)
	return &mt
}
