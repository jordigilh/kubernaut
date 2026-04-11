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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	prodcontroller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/config"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/routing"
)

func noopAnalyzingCallbacks() prodcontroller.AnalyzingCallbacks {
	return prodcontroller.AnalyzingCallbacks{
		AtomicStatusUpdate: func(_ context.Context, _ *remediationv1.RemediationRequest, fn func() error) error { return fn() },
		IsWorkflowNotNeeded: func(_ *aianalysisv1.AIAnalysis) bool { return false },
		HandleWorkflowNotNeeded: func(_ context.Context, _ *remediationv1.RemediationRequest, _ *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
			return ctrl.Result{}, nil
		},
		CreateApproval: func(_ context.Context, _ *remediationv1.RemediationRequest, _ *aianalysisv1.AIAnalysis) (string, error) {
			return "rar-test", nil
		},
		HandleAIAnalysisStatus: func(_ context.Context, _ *remediationv1.RemediationRequest, _ *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
			return ctrl.Result{}, nil
		},
		HandleRemediationTargetMissing: func(_ context.Context, _ *remediationv1.RemediationRequest, _ *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
			return ctrl.Result{}, nil
		},
		EmitApprovalRequestedAudit: func(_ context.Context, _ *remediationv1.RemediationRequest, _ float64, _ string) {},
		RecordEvent:                func(_ *remediationv1.RemediationRequest, _ string, _ string, _ string) {},
		FetchFreshRR: func(_ context.Context, _ client.ObjectKey) (*remediationv1.RemediationRequest, error) {
			return nil, fmt.Errorf("not implemented")
		},
		CheckPostAnalysisConditions: func(_ context.Context, _ *remediationv1.RemediationRequest, _, _, _, _ string) (*routing.BlockingCondition, error) {
			return nil, nil
		},
		HandleBlocked: func(_ context.Context, _ *remediationv1.RemediationRequest, _ *routing.BlockingCondition, _, _ string) (ctrl.Result, error) {
			return ctrl.Result{}, nil
		},
		AcquireLock: func(_ context.Context, _ string) (bool, error) { return true, nil },
		ReleaseLock: func(_ context.Context, _ string) error { return nil },
		CapturePreRemediationHash: func(_ context.Context, _, _, _ string) (string, string, error) { return "", "", nil },
		ResolveDualTargets: func(_ *remediationv1.RemediationRequest, _ *aianalysisv1.AIAnalysis) prodcontroller.DualTargetResult {
			return prodcontroller.DualTargetResult{
				Remediation: prodcontroller.TargetRef{Kind: "Deployment", Name: "app", Namespace: "default"},
			}
		},
		PersistPreHash: func(_ context.Context, _ *remediationv1.RemediationRequest, _ string) error { return nil },
		WFECallbacks: prodcontroller.WFECreationCallbacks{
			EmitWorkflowCreatedAudit: func(_ context.Context, _ *remediationv1.RemediationRequest, _ *aianalysisv1.AIAnalysis, _ string) {},
			CreateWFE:                func(_ context.Context, _ *remediationv1.RemediationRequest, _ *aianalysisv1.AIAnalysis) (string, error) { return "wfe-test", nil },
			ResolveWorkflowName:      func(_ context.Context, _ string) string { return "test-wf" },
		},
	}
}

var _ = Describe("Issue #666: AnalyzingHandler (BR-ORCH-036/037)", func() {

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

	newHandler := func(c client.Client, cbs prodcontroller.AnalyzingCallbacks) *prodcontroller.AnalyzingHandler {
		m := rometrics.NewMetricsWithRegistry(prometheus.NewRegistry())
		return prodcontroller.NewAnalyzingHandler(c, m, cbs)
	}

	analyzingRR := func(name string, aiRefName string) *remediationv1.RemediationRequest {
		rr := newRemediationRequest(name, "default", remediationv1.PhaseAnalyzing)
		if aiRefName != "" {
			rr.Status.AIAnalysisRef = &corev1.ObjectReference{Name: aiRefName, Namespace: "default"}
		}
		return rr
	}

	completedAI := func(name string, approvalRequired bool, workflowNotNeeded bool) *aianalysisv1.AIAnalysis {
		ai := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase:            "Completed",
				ApprovalRequired: approvalRequired,
				SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
					WorkflowID: "wf-restart",
					ActionType: "patch",
					Confidence: 0.95,
				},
				RootCauseAnalysis: &aianalysisv1.RootCauseAnalysis{
					RemediationTarget: &aianalysisv1.RemediationTarget{
						Kind:      "Deployment",
						Name:      "my-app",
						Namespace: "default",
					},
				},
			},
		}
		if workflowNotNeeded {
			ai.Status.SelectedWorkflow = nil
		}
		return ai
	}

	// ========================================
	// Interface compliance
	// ========================================
	Describe("Interface compliance", func() {
		It("UT-ANZ-H-001: implements PhaseHandler interface", func() {
			var _ phase.PhaseHandler = &prodcontroller.AnalyzingHandler{}
		})

		It("UT-ANZ-H-002: Phase() returns Analyzing", func() {
			c := fake.NewClientBuilder().WithScheme(scheme).Build()
			h := newHandler(c, noopAnalyzingCallbacks())
			Expect(h.Phase()).To(Equal(phase.Analyzing))
		})
	})

	// ========================================
	// AI not ready paths
	// ========================================
	Describe("AI not ready", func() {
		It("UT-ANZ-H-003: No AI ref → requeue at RequeueGenericError", func() {
			rr := analyzingRR("anz-noref", "")
			c := fake.NewClientBuilder().WithScheme(scheme).Build()

			h := newHandler(c, noopAnalyzingCallbacks())
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(Equal(config.RequeueGenericError))
		})

		It("UT-ANZ-H-004: AI Get NotFound → requeue", func() {
			rr := analyzingRR("anz-notfound", "ai-missing")
			c := fake.NewClientBuilder().WithScheme(scheme).Build()

			h := newHandler(c, noopAnalyzingCallbacks())
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(Equal(config.RequeueGenericError))
		})

		It("UT-ANZ-H-005: AI Get error → propagates error", func() {
			rr := analyzingRR("anz-geterr", "ai-err")

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithInterceptorFuncs(fakeGetInterceptor("ai-err")).
				Build()

			h := newHandler(c, noopAnalyzingCallbacks())
			_, err := h.Handle(ctx, rr)
			Expect(err).To(HaveOccurred())
		})
	})

	// ========================================
	// AI Completed paths
	// ========================================
	Describe("AI Completed", func() {
		It("UT-ANZ-H-006: WorkflowNotNeeded → delegates to handler (BR-ORCH-036)", func() {
			ai := completedAI("ai-nowork", false, true)
			rr := analyzingRR("anz-nowork", ai.Name)
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ai).Build()

			delegated := false
			cbs := noopAnalyzingCallbacks()
			cbs.IsWorkflowNotNeeded = func(_ *aianalysisv1.AIAnalysis) bool { return true }
			cbs.HandleWorkflowNotNeeded = func(_ context.Context, _ *remediationv1.RemediationRequest, _ *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
				delegated = true
				return ctrl.Result{}, nil
			}

			h := newHandler(c, cbs)
			_, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(delegated).To(BeTrue(), "should delegate to HandleWorkflowNotNeeded")
		})

		It("UT-ANZ-H-007: approval required → RAR created + Advance to AwaitingApproval (BR-ORCH-026)", func() {
			ai := completedAI("ai-approve", true, false)
			rr := analyzingRR("anz-approve", ai.Name)
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ai).Build()

			rarCreated := false
			cbs := noopAnalyzingCallbacks()
			cbs.CreateApproval = func(_ context.Context, _ *remediationv1.RemediationRequest, _ *aianalysisv1.AIAnalysis) (string, error) {
				rarCreated = true
				return "rar-test", nil
			}

			h := newHandler(c, cbs)
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(rarCreated).To(BeTrue(), "should create RAR")
			Expect(intent.Type).To(Equal(phase.TransitionAdvance))
			Expect(intent.TargetPhase).To(Equal(phase.AwaitingApproval))
		})

		It("UT-ANZ-H-008: direct execution → WFE created + Advance to Executing (BR-ORCH-037)", func() {
			ai := completedAI("ai-direct", false, false)
			rr := analyzingRR("anz-direct", ai.Name)
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(ai, rr).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			wfeCreated := false
			cbs := noopAnalyzingCallbacks()
			cbs.WFECallbacks.CreateWFE = func(_ context.Context, _ *remediationv1.RemediationRequest, _ *aianalysisv1.AIAnalysis) (string, error) {
				wfeCreated = true
				return "wfe-test", nil
			}
			cbs.FetchFreshRR = func(_ context.Context, _ client.ObjectKey) (*remediationv1.RemediationRequest, error) {
				return rr, nil
			}

			h := newHandler(c, cbs)
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(wfeCreated).To(BeTrue(), "should create WFE")
			Expect(intent.Type).To(Equal(phase.TransitionAdvance))
			Expect(intent.TargetPhase).To(Equal(phase.Executing))
		})

		It("UT-ANZ-H-009: stale cache (phase mismatch) → NoOp", func() {
			ai := completedAI("ai-stale", false, false)
			rr := analyzingRR("anz-stale", ai.Name)
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ai).Build()

			cbs := noopAnalyzingCallbacks()
			freshRR := rr.DeepCopy()
			freshRR.Status.OverallPhase = remediationv1.PhaseExecuting
			cbs.FetchFreshRR = func(_ context.Context, _ client.ObjectKey) (*remediationv1.RemediationRequest, error) {
				return freshRR, nil
			}

			h := newHandler(c, cbs)
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionNone))
		})

		It("UT-ANZ-H-010: missing RemediationTarget → delegates to HandleRemediationTargetMissing", func() {
			ai := completedAI("ai-notarget", false, false)
			ai.Status.RootCauseAnalysis = nil
			rr := analyzingRR("anz-notarget", ai.Name)
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ai).Build()

			delegated := false
			cbs := noopAnalyzingCallbacks()
			cbs.FetchFreshRR = func(_ context.Context, _ client.ObjectKey) (*remediationv1.RemediationRequest, error) {
				return rr, nil
			}
			cbs.HandleRemediationTargetMissing = func(_ context.Context, _ *remediationv1.RemediationRequest, _ *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
				delegated = true
				return ctrl.Result{}, nil
			}

			h := newHandler(c, cbs)
			_, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(delegated).To(BeTrue(), "should delegate to HandleRemediationTargetMissing")
		})

		It("UT-ANZ-H-011: routing blocked post-analysis → Block intent", func() {
			ai := completedAI("ai-blocked", false, false)
			rr := analyzingRR("anz-blocked", ai.Name)
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ai).Build()

			cbs := noopAnalyzingCallbacks()
			cbs.FetchFreshRR = func(_ context.Context, _ client.ObjectKey) (*remediationv1.RemediationRequest, error) {
				return rr, nil
			}
			cbs.CheckPostAnalysisConditions = func(_ context.Context, _ *remediationv1.RemediationRequest, _, _, _, _ string) (*routing.BlockingCondition, error) {
				return &routing.BlockingCondition{
					Reason:       "ResourceBusy",
					Message:      "target in use",
					RequeueAfter: 30 * time.Second,
				}, nil
			}
			handleBlockedCalled := false
			cbs.HandleBlocked = func(_ context.Context, _ *remediationv1.RemediationRequest, _ *routing.BlockingCondition, _, _ string) (ctrl.Result, error) {
				handleBlockedCalled = true
				return ctrl.Result{}, nil
			}

			h := newHandler(c, cbs)
			_, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(handleBlockedCalled).To(BeTrue(), "should delegate to handleBlocked")
		})

		It("UT-ANZ-H-012: lock acquisition failure → requeue", func() {
			ai := completedAI("ai-lockfail", false, false)
			rr := analyzingRR("anz-lockfail", ai.Name)
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ai).Build()

			cbs := noopAnalyzingCallbacks()
			cbs.FetchFreshRR = func(_ context.Context, _ client.ObjectKey) (*remediationv1.RemediationRequest, error) {
				return rr, nil
			}
			cbs.AcquireLock = func(_ context.Context, _ string) (bool, error) {
				return false, fmt.Errorf("redis unavailable")
			}

			h := newHandler(c, cbs)
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(Equal(config.RequeueGenericError))
		})

		It("UT-ANZ-H-013: lock contention → requeue at 5s", func() {
			ai := completedAI("ai-lockbusy", false, false)
			rr := analyzingRR("anz-lockbusy", ai.Name)
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ai).Build()

			cbs := noopAnalyzingCallbacks()
			cbs.FetchFreshRR = func(_ context.Context, _ client.ObjectKey) (*remediationv1.RemediationRequest, error) {
				return rr, nil
			}
			cbs.AcquireLock = func(_ context.Context, _ string) (bool, error) {
				return false, nil
			}

			h := newHandler(c, cbs)
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(Equal(5 * time.Second))
		})

		It("UT-ANZ-H-014: WFE creation failure → requeue (via shared utility)", func() {
			ai := completedAI("ai-wfefail", false, false)
			rr := analyzingRR("anz-wfefail", ai.Name)
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(ai, rr).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			cbs := noopAnalyzingCallbacks()
			cbs.FetchFreshRR = func(_ context.Context, _ client.ObjectKey) (*remediationv1.RemediationRequest, error) {
				return rr, nil
			}
			cbs.WFECallbacks.CreateWFE = func(_ context.Context, _ *remediationv1.RemediationRequest, _ *aianalysisv1.AIAnalysis) (string, error) {
				return "", fmt.Errorf("namespace terminating")
			}

			h := newHandler(c, cbs)
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(BeNumerically(">", 0))
		})

		It("UT-ANZ-H-015: pre-hash hard error → Failed", func() {
			ai := completedAI("ai-hasherr", false, false)
			rr := analyzingRR("anz-hasherr", ai.Name)
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(ai, rr).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			cbs := noopAnalyzingCallbacks()
			cbs.FetchFreshRR = func(_ context.Context, _ client.ObjectKey) (*remediationv1.RemediationRequest, error) {
				return rr, nil
			}
			cbs.CapturePreRemediationHash = func(_ context.Context, _, _, _ string) (string, string, error) {
				return "", "", fmt.Errorf("hash computation failed")
			}

			h := newHandler(c, cbs)
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionFailed))
		})

		It("UT-ANZ-H-016: pre-hash soft degradation → continues", func() {
			ai := completedAI("ai-hashwarn", false, false)
			rr := analyzingRR("anz-hashwarn", ai.Name)
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(ai, rr).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			cbs := noopAnalyzingCallbacks()
			cbs.FetchFreshRR = func(_ context.Context, _ client.ObjectKey) (*remediationv1.RemediationRequest, error) {
				return rr, nil
			}
			cbs.CapturePreRemediationHash = func(_ context.Context, _, _, _ string) (string, string, error) {
				return "", "resource has no spec", nil
			}

			h := newHandler(c, cbs)
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionAdvance))
			Expect(intent.TargetPhase).To(Equal(phase.Executing))
		})
	})

	// ========================================
	// AI Failed
	// ========================================
	Describe("AI Failed", func() {
		It("UT-ANZ-H-017: AI Failed → delegates to HandleAIAnalysisStatus", func() {
			ai := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{Name: "ai-failed", Namespace: "default"},
				Status: aianalysisv1.AIAnalysisStatus{
					Phase:   "Failed",
					Message: "LLM timeout",
				},
			}
			rr := analyzingRR("anz-failed", ai.Name)
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ai).Build()

			delegated := false
			cbs := noopAnalyzingCallbacks()
			cbs.HandleAIAnalysisStatus = func(_ context.Context, _ *remediationv1.RemediationRequest, _ *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
				delegated = true
				return ctrl.Result{}, nil
			}

			h := newHandler(c, cbs)
			_, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(delegated).To(BeTrue(), "should delegate to HandleAIAnalysisStatus")
		})
	})

	// ========================================
	// AI in progress / unknown
	// ========================================
	Describe("AI in progress / unknown", func() {
		It("UT-ANZ-H-018: AI in progress (Pending) → requeue at 10s", func() {
			ai := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{Name: "ai-pending", Namespace: "default"},
				Status:     aianalysisv1.AIAnalysisStatus{Phase: "Pending"},
			}
			rr := analyzingRR("anz-pending", ai.Name)
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ai).Build()

			h := newHandler(c, noopAnalyzingCallbacks())
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(Equal(10 * time.Second))
		})

		It("UT-ANZ-H-019: AI unknown phase → requeue at 10s", func() {
			ai := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{Name: "ai-unknown", Namespace: "default"},
				Status:     aianalysisv1.AIAnalysisStatus{Phase: "SomethingNew"},
			}
			rr := analyzingRR("anz-unknown", ai.Name)
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ai).Build()

			h := newHandler(c, noopAnalyzingCallbacks())
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(Equal(10 * time.Second))
		})
	})

	// ========================================
	// Error paths
	// ========================================
	Describe("Error paths", func() {
		It("UT-ANZ-H-020: RAR creation failure → requeue (non-fatal)", func() {
			ai := completedAI("ai-rarfail", true, false)
			rr := analyzingRR("anz-rarfail", ai.Name)
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ai).Build()

			cbs := noopAnalyzingCallbacks()
			cbs.CreateApproval = func(_ context.Context, _ *remediationv1.RemediationRequest, _ *aianalysisv1.AIAnalysis) (string, error) {
				return "", fmt.Errorf("quota exceeded")
			}

			h := newHandler(c, cbs)
			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(Equal(config.RequeueGenericError))
		})
	})
})

// fakeGetInterceptor returns an interceptor.Funcs that returns an error for Gets of an object with the given name.
func fakeGetInterceptor(name string) interceptor.Funcs {
	return interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if key.Name == name {
				return apierrors.NewInternalError(fmt.Errorf("injected error"))
			}
			return c.Get(ctx, key, obj, opts...)
		},
	}
}
