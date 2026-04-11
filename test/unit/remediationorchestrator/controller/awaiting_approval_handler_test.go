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
	prometheus "github.com/prometheus/client_golang/prometheus"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	prodcontroller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/config"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/override"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/routing"
)

func noopAwaitingCallbacks() prodcontroller.AwaitingApprovalCallbacks {
	return prodcontroller.AwaitingApprovalCallbacks{
		RecordEvent: func(_ *remediationv1.RemediationRequest, _, _, _ string) {},
		UpdateRARConditions: func(_ context.Context, _ *remediationv1.RemediationRequest, _ *remediationv1.RemediationApprovalRequest, _ string) error {
			return nil
		},
		ResolveWorkflow: func(_ context.Context, _ *remediationv1.WorkflowOverride, _ *aianalysisv1.SelectedWorkflow, _ string) (*aianalysisv1.SelectedWorkflow, bool, error) {
			return nil, false, nil
		},
		CheckResourceBusy: func(_ context.Context, _ *remediationv1.RemediationRequest, _ string) (*routing.BlockingCondition, error) {
			return nil, nil
		},
		HandleBlocked: func(_ context.Context, _ *remediationv1.RemediationRequest, _ *routing.BlockingCondition, _, _ string) (ctrl.Result, error) {
			return ctrl.Result{}, nil
		},
		AcquireLock: func(_ context.Context, _ string) (bool, error) { return true, nil },
		ReleaseLock: func(_ context.Context, _ string) error { return nil },
		CapturePreRemediationHash: func(_ context.Context, _, _, _ string) (string, string, error) { return "", "", nil },
		ResolveDualTargets: func(_ *remediationv1.RemediationRequest, _ *aianalysisv1.AIAnalysis) prodcontroller.DualTargetResult {
			return prodcontroller.DualTargetResult{Remediation: prodcontroller.TargetRef{Kind: "Deployment", Name: "app", Namespace: "default"}}
		},
		PersistPreHash:     func(_ context.Context, _ *remediationv1.RemediationRequest, _ string) error { return nil },
		TransitionToFailed: func(_ context.Context, _ *remediationv1.RemediationRequest, _ remediationv1.FailurePhase, _ error) (ctrl.Result, error) { return ctrl.Result{}, nil },
		ExpireRAR:          func(_ context.Context, _ *remediationv1.RemediationApprovalRequest) error { return nil },
		UpdateRARTimeRemaining: func(_ context.Context, _ *remediationv1.RemediationApprovalRequest) error { return nil },
		WFECallbacks: prodcontroller.WFECreationCallbacks{
			EmitWorkflowCreatedAudit: func(_ context.Context, _ *remediationv1.RemediationRequest, _ *aianalysisv1.AIAnalysis, _ string) {},
			CreateWFE:                func(_ context.Context, _ *remediationv1.RemediationRequest, _ *aianalysisv1.AIAnalysis) (string, error) { return "wfe-test", nil },
			ResolveWorkflowName:      func(_ context.Context, _ string) string { return "test-wf" },
		},
	}
}

var _ = Describe("Issue #666: AwaitingApprovalHandler (BR-ORCH-026, ADR-040)", func() {

	var (
		ctx    context.Context
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())
	})

	newHandler := func(c client.Client, cbs prodcontroller.AwaitingApprovalCallbacks) *prodcontroller.AwaitingApprovalHandler {
		m := rometrics.NewMetricsWithRegistry(prometheus.NewRegistry())
		return prodcontroller.NewAwaitingApprovalHandler(c, m, cbs)
	}

	awaitingRR := func(name string) *remediationv1.RemediationRequest {
		rr := newRemediationRequest(name, "default", remediationv1.PhaseAwaitingApproval)
		rr.Status.AIAnalysisRef = &corev1.ObjectReference{Name: "ai-" + name, Namespace: "default"}
		return rr
	}

	makeRAR := func(rrName string, decision remediationv1.ApprovalDecision) *remediationv1.RemediationApprovalRequest {
		return &remediationv1.RemediationApprovalRequest{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("rar-%s", rrName), Namespace: "default"},
			Spec: remediationv1.RemediationApprovalRequestSpec{
				RequiredBy: metav1.NewTime(time.Now().Add(1 * time.Hour)),
			},
			Status: remediationv1.RemediationApprovalRequestStatus{
				Decision:  decision,
				DecidedBy: "admin@example.com",
			},
		}
	}

	makeAI := func(name string) *aianalysisv1.AIAnalysis {
		return &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase: "Completed",
				SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
					WorkflowID: "wf-restart",
					ActionType: "patch",
					Confidence: 0.95,
				},
				RootCauseAnalysis: &aianalysisv1.RootCauseAnalysis{
					RemediationTarget: &aianalysisv1.RemediationTarget{
						Kind: "Deployment", Name: "my-app", Namespace: "default",
					},
				},
			},
		}
	}

	// ========================================
	// Interface compliance
	// ========================================
	Describe("Interface compliance", func() {
		It("UT-APR-H-001: implements PhaseHandler interface", func() {
			var _ phase.PhaseHandler = &prodcontroller.AwaitingApprovalHandler{}
		})

		It("UT-APR-H-002: Phase() returns AwaitingApproval", func() {
			c := fake.NewClientBuilder().WithScheme(scheme).Build()
			h := newHandler(c, noopAwaitingCallbacks())
			Expect(h.Phase()).To(Equal(phase.AwaitingApproval))
		})
	})

	// ========================================
	// RAR lookup
	// ========================================
	Describe("RAR lookup", func() {
		It("UT-APR-H-003: RAR Get NotFound → requeue", func() {
			rr := awaitingRR("apr-nf")
			c := fake.NewClientBuilder().WithScheme(scheme).Build()

			h := newHandler(c, noopAwaitingCallbacks())
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(Equal(config.RequeueGenericError))
		})

		It("UT-APR-H-004: RAR Get error → propagates error", func() {
			rr := awaitingRR("apr-err")
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithInterceptorFuncs(fakeGetInterceptor(fmt.Sprintf("rar-%s", rr.Name))).
				Build()

			h := newHandler(c, noopAwaitingCallbacks())
			_, err := h.Handle(ctx, rr)
			Expect(err).To(HaveOccurred())
		})
	})

	// ========================================
	// Approved path
	// ========================================
	Describe("RAR Approved", func() {
		It("UT-APR-H-005: Approved → WFE created + Advance to Executing (BR-ORCH-095)", func() {
			rr := awaitingRR("apr-ok")
			rar := makeRAR(rr.Name, remediationv1.ApprovalDecisionApproved)
			ai := makeAI("ai-" + rr.Name)

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, rar, ai).
				WithStatusSubresource(&remediationv1.RemediationRequest{}, &remediationv1.RemediationApprovalRequest{}).
				Build()

			wfeCreated := false
			cbs := noopAwaitingCallbacks()
			cbs.WFECallbacks.CreateWFE = func(_ context.Context, _ *remediationv1.RemediationRequest, _ *aianalysisv1.AIAnalysis) (string, error) {
				wfeCreated = true
				return "wfe-approved", nil
			}

			h := newHandler(c, cbs)
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(wfeCreated).To(BeTrue())
			Expect(intent.Type).To(Equal(phase.TransitionAdvance))
			Expect(intent.TargetPhase).To(Equal(phase.Executing))
		})

		It("UT-APR-H-006: Approved + override permanent error → Failed", func() {
			rr := awaitingRR("apr-ovrfail")
			rar := makeRAR(rr.Name, remediationv1.ApprovalDecisionApproved)
			ai := makeAI("ai-" + rr.Name)

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, rar, ai).
				WithStatusSubresource(&remediationv1.RemediationRequest{}, &remediationv1.RemediationApprovalRequest{}).
				Build()

			failedCalled := false
			cbs := noopAwaitingCallbacks()
			cbs.ResolveWorkflow = func(_ context.Context, _ *remediationv1.WorkflowOverride, _ *aianalysisv1.SelectedWorkflow, _ string) (*aianalysisv1.SelectedWorkflow, bool, error) {
				return nil, false, override.NewOverrideNotFoundError("deleted-wf", "default", fmt.Errorf("workflow deleted"))
			}
			cbs.TransitionToFailed = func(_ context.Context, _ *remediationv1.RemediationRequest, fp remediationv1.FailurePhase, _ error) (ctrl.Result, error) {
				failedCalled = true
				Expect(fp).To(Equal(remediationv1.FailurePhaseApproval))
				return ctrl.Result{}, nil
			}

			h := newHandler(c, cbs)
			_, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(failedCalled).To(BeTrue())
		})

		It("UT-APR-H-007: Approved + routing blocked → Block intent", func() {
			rr := awaitingRR("apr-busy")
			rar := makeRAR(rr.Name, remediationv1.ApprovalDecisionApproved)
			ai := makeAI("ai-" + rr.Name)

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, rar, ai).
				WithStatusSubresource(&remediationv1.RemediationRequest{}, &remediationv1.RemediationApprovalRequest{}).
				Build()

			handleBlockedCalled := false
			cbs := noopAwaitingCallbacks()
			cbs.CheckResourceBusy = func(_ context.Context, _ *remediationv1.RemediationRequest, _ string) (*routing.BlockingCondition, error) {
				return &routing.BlockingCondition{Reason: "ResourceBusy"}, nil
			}
			cbs.HandleBlocked = func(_ context.Context, _ *remediationv1.RemediationRequest, _ *routing.BlockingCondition, _, _ string) (ctrl.Result, error) {
				handleBlockedCalled = true
				return ctrl.Result{}, nil
			}

			h := newHandler(c, cbs)
			_, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(handleBlockedCalled).To(BeTrue())
		})

		It("UT-APR-H-008: Approved + lock failure → requeue", func() {
			rr := awaitingRR("apr-lockfail")
			rar := makeRAR(rr.Name, remediationv1.ApprovalDecisionApproved)
			ai := makeAI("ai-" + rr.Name)

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, rar, ai).
				WithStatusSubresource(&remediationv1.RemediationRequest{}, &remediationv1.RemediationApprovalRequest{}).
				Build()

			cbs := noopAwaitingCallbacks()
			cbs.AcquireLock = func(_ context.Context, _ string) (bool, error) {
				return false, fmt.Errorf("redis down")
			}

			h := newHandler(c, cbs)
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(Equal(config.RequeueGenericError))
		})

		It("UT-APR-H-009: Approved + WFE creation failure → requeue", func() {
			rr := awaitingRR("apr-wfefail")
			rar := makeRAR(rr.Name, remediationv1.ApprovalDecisionApproved)
			ai := makeAI("ai-" + rr.Name)

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, rar, ai).
				WithStatusSubresource(&remediationv1.RemediationRequest{}, &remediationv1.RemediationApprovalRequest{}).
				Build()

			cbs := noopAwaitingCallbacks()
			cbs.WFECallbacks.CreateWFE = func(_ context.Context, _ *remediationv1.RemediationRequest, _ *aianalysisv1.AIAnalysis) (string, error) {
				return "", fmt.Errorf("namespace terminating")
			}

			h := newHandler(c, cbs)
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(BeNumerically(">", 0))
		})
	})

	// ========================================
	// Rejected / Expired
	// ========================================
	Describe("RAR Rejected / Expired", func() {
		It("UT-APR-H-010: Rejected → Failed transition (BR-ORCH-026)", func() {
			rr := awaitingRR("apr-rej")
			rar := makeRAR(rr.Name, remediationv1.ApprovalDecisionRejected)
			rar.Status.DecisionMessage = "Not safe to proceed"

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rar).
				WithStatusSubresource(&remediationv1.RemediationApprovalRequest{}).
				Build()

			failedCalled := false
			cbs := noopAwaitingCallbacks()
			cbs.TransitionToFailed = func(_ context.Context, _ *remediationv1.RemediationRequest, fp remediationv1.FailurePhase, _ error) (ctrl.Result, error) {
				failedCalled = true
				Expect(fp).To(Equal(remediationv1.FailurePhaseApproval))
				return ctrl.Result{}, nil
			}

			h := newHandler(c, cbs)
			_, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(failedCalled).To(BeTrue())
		})

		It("UT-APR-H-011: Expired → Failed transition", func() {
			rr := awaitingRR("apr-exp")
			rar := makeRAR(rr.Name, remediationv1.ApprovalDecisionExpired)

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rar).
				Build()

			failedCalled := false
			cbs := noopAwaitingCallbacks()
			cbs.TransitionToFailed = func(_ context.Context, _ *remediationv1.RemediationRequest, fp remediationv1.FailurePhase, _ error) (ctrl.Result, error) {
				failedCalled = true
				Expect(fp).To(Equal(remediationv1.FailurePhaseApproval))
				return ctrl.Result{}, nil
			}

			h := newHandler(c, cbs)
			_, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(failedCalled).To(BeTrue())
		})
	})

	// ========================================
	// Pending
	// ========================================
	Describe("RAR pending", func() {
		It("UT-APR-H-012: pending + deadline passed → expire RAR + Failed", func() {
			rr := awaitingRR("apr-deadpast")
			rar := makeRAR(rr.Name, "")
			rar.Status.Decision = ""
			rar.Spec.RequiredBy = metav1.NewTime(time.Now().Add(-1 * time.Hour))

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rar).
				WithStatusSubresource(&remediationv1.RemediationApprovalRequest{}).
				Build()

			expireCalled := false
			failedCalled := false
			cbs := noopAwaitingCallbacks()
			cbs.ExpireRAR = func(_ context.Context, _ *remediationv1.RemediationApprovalRequest) error {
				expireCalled = true
				return nil
			}
			cbs.TransitionToFailed = func(_ context.Context, _ *remediationv1.RemediationRequest, _ remediationv1.FailurePhase, _ error) (ctrl.Result, error) {
				failedCalled = true
				return ctrl.Result{}, nil
			}

			h := newHandler(c, cbs)
			_, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(expireCalled).To(BeTrue(), "should expire RAR")
			Expect(failedCalled).To(BeTrue(), "should transition to Failed")
		})

		It("UT-APR-H-013: pending + deadline active → update TimeRemaining + requeue", func() {
			rr := awaitingRR("apr-wait")
			rar := makeRAR(rr.Name, "")
			rar.Status.Decision = ""
			rar.Spec.RequiredBy = metav1.NewTime(time.Now().Add(30 * time.Minute))

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rar).
				WithStatusSubresource(&remediationv1.RemediationApprovalRequest{}).
				Build()

			timeUpdated := false
			cbs := noopAwaitingCallbacks()
			cbs.UpdateRARTimeRemaining = func(_ context.Context, _ *remediationv1.RemediationApprovalRequest) error {
				timeUpdated = true
				return nil
			}

			h := newHandler(c, cbs)
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(timeUpdated).To(BeTrue(), "should update RAR time remaining")
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(Equal(config.RequeueResourceBusy))
		})
	})
})
