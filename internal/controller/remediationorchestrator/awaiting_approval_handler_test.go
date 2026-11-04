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

package controller_test

import (
	"context"
	"fmt"
	"time"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"

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
	remediationworkflowv1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
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
		AcquireLock:               func(_ context.Context, _ string) (bool, error) { return true, nil },
		ReleaseLock:               func(_ context.Context, _ string) error { return nil },
		CapturePreRemediationHash: func(_ context.Context, _, _, _, _ string) (string, string, error) { return "", "", nil },
		ResolveDualTargets: func(_ *remediationv1.RemediationRequest, _ *aianalysisv1.AIAnalysis) prodcontroller.DualTargetResult {
			return prodcontroller.DualTargetResult{Remediation: prodcontroller.TargetRef{Kind: "Deployment", Name: "app", Namespace: defaultFixture}}
		},
		PersistPreHash: func(_ context.Context, _ *remediationv1.RemediationRequest, _ string) error { return nil },
		TransitionToFailed: func(_ context.Context, _ *remediationv1.RemediationRequest, _ remediationv1.FailurePhase, _ error) (ctrl.Result, error) {
			return ctrl.Result{}, nil
		},
		ExpireRAR: func(_ context.Context, _ *remediationv1.RemediationApprovalRequest) (prodcontroller.ExpiryOutcome, error) {
			return prodcontroller.ExpiredConfirmed, nil
		},
		UpdateRARTimeRemaining: func(_ context.Context, _ *remediationv1.RemediationApprovalRequest) error { return nil },
		WFECallbacks: prodcontroller.WFECreationCallbacks{
			EmitWorkflowCreatedAudit: func(_ context.Context, _ *remediationv1.RemediationRequest, _ *aianalysisv1.AIAnalysis, _ string) {},
			CreateWFE: func(_ context.Context, _ *remediationv1.RemediationRequest, _ *aianalysisv1.AIAnalysis) (string, bool, error) {
				return "wfe-test", true, nil
			},
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
		rr := newRemediationRequest(name, defaultFixture, remediationv1.PhaseAwaitingApproval)
		rr.Status.EnsurePhaseProgress().AIAnalysisRef = &corev1.ObjectReference{Name: "ai-" + name, Namespace: defaultFixture}
		return rr
	}

	makeRAR := func(rrName string, decision remediationv1.ApprovalDecision) *remediationv1.RemediationApprovalRequest {
		now := metav1.Now()
		requiredBy := metav1.NewTime(now.Add(1 * time.Hour))
		decidedBy := "admin@example.com"
		expired := false
		if decision == remediationv1.ApprovalDecisionExpired {
			requiredBy = metav1.NewTime(now.Add(-1 * time.Hour))
			decidedBy = "system"
			expired = true
		}
		return &remediationv1.RemediationApprovalRequest{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("rar-%s", rrName), Namespace: defaultFixture},
			Spec: remediationv1.RemediationApprovalRequestSpec{
				RequiredBy:      requiredBy,
				Workflow:        &sharedtypes.WorkflowSnapshot{WorkflowID: "wf-restart", DeclaredParameterNames: map[string]bool{}},
				Parameters:      map[string]string{},
				ParameterSchema: []remediationworkflowv1.RemediationWorkflowParameter{},
				TargetContext:   &remediationv1.ApprovalTargetContext{RemediationTarget: &remediationv1.ResourceIdentifier{Kind: "Deployment", Name: "my-app", Namespace: defaultFixture}},
			},
			Status: remediationv1.RemediationApprovalRequestStatus{
				Decision:  decision,
				DecidedBy: decidedBy,
				DecidedAt: &now,
				Expired:   expired,
			},
		}
	}

	makeAI := func(name string) *aianalysisv1.AIAnalysis {
		return &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: defaultFixture},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase: "Completed",
				RCAResult: &aianalysisv1.RCAResult{
					SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
						WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
							WorkflowID:   "wf-restart",
							WorkflowName: "wf-restart",
							ActionType:   "patch",
						},
						Confidence: 0.95,
					},
					RootCauseAnalysis: &aianalysisv1.RootCauseAnalysis{
						RemediationTarget: &aianalysisv1.RemediationTarget{
							Kind: "Deployment", Name: "my-app", Namespace: defaultFixture,
						},
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
		It("UT-RO-2570-131: BR-ORCH-025 does not fail on a stale cache miss", func() {
			rr := awaitingRR("apr-stale")
			cached := fake.NewClientBuilder().WithScheme(scheme).Build()
			live := fake.NewClientBuilder().WithScheme(scheme).WithObjects(makeRAR(rr.Name, remediationv1.ApprovalDecisionPending)).Build()
			cbs := noopAwaitingCallbacks()
			cbs.TransitionToFailed = func(_ context.Context, _ *remediationv1.RemediationRequest, _ remediationv1.FailurePhase, _ error) (ctrl.Result, error) {
				Fail("a cache miss must not fail a present approval")
				return ctrl.Result{}, nil
			}
			h := newHandler(cached, cbs)
			h.SetAPIReader(live)
			intent, err := h.Handle(ctx, rr)
			Expect(err).NotTo(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(BeNumerically(">", 0))
		})

		It("UT-RO-2570-132: BR-ORCH-025 propagates an authoritative read error", func() {
			rr := awaitingRR("apr-live-error")
			cached := fake.NewClientBuilder().WithScheme(scheme).Build()
			live := fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(fakeGetInterceptor("rar-" + rr.Name)).Build()
			cbs := noopAwaitingCallbacks()
			cbs.TransitionToFailed = func(_ context.Context, _ *remediationv1.RemediationRequest, _ remediationv1.FailurePhase, _ error) (ctrl.Result, error) {
				Fail("an API error must not be treated as missing")
				return ctrl.Result{}, nil
			}
			h := newHandler(cached, cbs)
			h.SetAPIReader(live)
			_, err := h.Handle(ctx, rr)
			Expect(err).To(HaveOccurred())
		})

		It("UT-RO-2570-130: BR-ORCH-025 fails approval when the RAR is absent", func() {
			rr := awaitingRR("apr-nf")
			c := fake.NewClientBuilder().WithScheme(scheme).Build()
			cbs := noopAwaitingCallbacks()
			failed := false
			cbs.TransitionToFailed = func(_ context.Context, _ *remediationv1.RemediationRequest, p remediationv1.FailurePhase, reason error) (ctrl.Result, error) {
				failed = true
				Expect(p).To(Equal(remediationv1.FailurePhaseApproval))
				Expect(reason.Error()).To(ContainSubstring("missing"))
				return ctrl.Result{}, nil
			}
			h := newHandler(c, cbs)
			_, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(failed).To(BeTrue())
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
		It("UT-RO-2570-108: BR-ORCH-025 creates execution without reading live AIAnalysis", func() {
			rr := awaitingRR("apr-snapshot-only")
			rr.Status.EnsurePhaseProgress().AIAnalysisRef = nil
			rar := makeRAR(rr.Name, remediationv1.ApprovalDecisionApproved)
			rar.Spec.Parameters = map[string]string{"MODE": "reviewed"}
			rar.Spec.Workflow.DeclaredParameterNames = map[string]bool{"MODE": true}
			rar.Spec.TargetContext.SignalTarget = remediationv1.ResourceIdentifier{Kind: "Pod", Name: "original-pod", Namespace: defaultFixture}
			rar.Spec.TargetContext.SignalClusterID = "reviewed-cluster"
			rar.Spec.TargetContext.RemediationTarget = &remediationv1.ResourceIdentifier{Kind: "Deployment", Name: "reviewed-app", Namespace: defaultFixture}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rr, rar).
				WithStatusSubresource(&remediationv1.RemediationRequest{}, &remediationv1.RemediationApprovalRequest{}).Build()
			cbs := noopAwaitingCallbacks()
			cbs.ResolveWorkflow = func(_ context.Context, _ *remediationv1.WorkflowOverride, sw *aianalysisv1.SelectedWorkflow, _ string) (*aianalysisv1.SelectedWorkflow, bool, error) {
				return sw, false, nil
			}
			cbs.ResolveDualTargets = func(inputRR *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) prodcontroller.DualTargetResult {
				Expect(inputRR.Spec.TargetResource.Name).To(Equal("original-pod"))
				Expect(inputRR.Spec.ClusterID).To(Equal("reviewed-cluster"))
				Expect(ai.Status.RCAResult.RootCauseAnalysis.RemediationTarget.Name).To(Equal("reviewed-app"))
				return prodcontroller.DualTargetResult{Remediation: prodcontroller.TargetRef{Kind: "Deployment", Name: "reviewed-app", Namespace: defaultFixture}}
			}
			hashed := false
			cbs.CapturePreRemediationHash = func(_ context.Context, kind, name, namespace, cluster string) (string, string, error) {
				hashed = true
				Expect([]string{kind, name, namespace, cluster}).To(Equal([]string{"Deployment", "reviewed-app", defaultFixture, "reviewed-cluster"}))
				return "", "", nil
			}
			created := false
			cbs.WFECallbacks.CreateWFE = func(_ context.Context, inputRR *remediationv1.RemediationRequest, ai *aianalysisv1.AIAnalysis) (string, bool, error) {
				created = true
				Expect(ai.Status.RCAResult.SelectedWorkflow.WorkflowID).To(Equal("wf-restart"))
				Expect(ai.Status.RCAResult.SelectedWorkflow.Parameters).To(Equal(map[string]string{"MODE": "reviewed"}))
				Expect(ai.Status.RCAResult.RootCauseAnalysis.RemediationTarget.Name).To(Equal("reviewed-app"))
				Expect(inputRR.Spec.TargetResource.Name).To(Equal("original-pod"))
				Expect(inputRR.Spec.ClusterID).To(Equal("reviewed-cluster"))
				return "wfe-approved", true, nil
			}
			_, err := newHandler(c, cbs).Handle(ctx, rr)
			Expect(err).NotTo(HaveOccurred())
			Expect(created).To(BeTrue())
			Expect(hashed).To(BeTrue())
		})

		DescribeTable("UT-RO-2570-107: BR-ORCH-025 rejects unsafe approved RARs before execution", func(mutate func(*remediationv1.RemediationApprovalRequest)) {
			rr := awaitingRR("apr-incomplete")
			rar := makeRAR(rr.Name, remediationv1.ApprovalDecisionApproved)
			mutate(rar)
			ai := makeAI("ai-" + rr.Name)
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, rar, ai).
				WithStatusSubresource(&remediationv1.RemediationRequest{}, &remediationv1.RemediationApprovalRequest{}).
				Build()
			cbs := noopAwaitingCallbacks()
			created := false
			failed := false
			cbs.WFECallbacks.CreateWFE = func(_ context.Context, _ *remediationv1.RemediationRequest, _ *aianalysisv1.AIAnalysis) (string, bool, error) {
				created = true
				return "we-unexpected", true, nil
			}
			cbs.TransitionToFailed = func(_ context.Context, _ *remediationv1.RemediationRequest, failurePhase remediationv1.FailurePhase, _ error) (ctrl.Result, error) {
				Expect(failurePhase).To(Equal(remediationv1.FailurePhaseApproval))
				failed = true
				return ctrl.Result{}, nil
			}
			_, err := newHandler(c, cbs).Handle(ctx, rr)
			Expect(err).NotTo(HaveOccurred())
			Expect(created).To(BeFalse())
			Expect(failed).To(BeTrue())
		},
			Entry("missing original parameters", func(rar *remediationv1.RemediationApprovalRequest) { rar.Spec.Parameters = nil }),
			Entry("missing parameter schema", func(rar *remediationv1.RemediationApprovalRequest) { rar.Spec.ParameterSchema = nil }),
			Entry("missing declarations", func(rar *remediationv1.RemediationApprovalRequest) { rar.Spec.Workflow.DeclaredParameterNames = nil }),
			Entry("missing target context", func(rar *remediationv1.RemediationApprovalRequest) { rar.Spec.TargetContext = nil }),
			Entry("missing workflow identity", func(rar *remediationv1.RemediationApprovalRequest) { rar.Spec.Workflow.WorkflowID = "" }),
			Entry("UT-RO-2570-042: approved legacy workflow switch", func(rar *remediationv1.RemediationApprovalRequest) {
				rar.Status.WorkflowOverride = &remediationv1.WorkflowOverride{WorkflowName: "unreviewed-workflow"}
			}),
		)

		It("UT-RO-2570-106: BR-ORCH-025 fails closed when an approved RAR lacks its snapshot", func() {
			rr := awaitingRR("apr-missing-snapshot")
			rar := makeRAR(rr.Name, remediationv1.ApprovalDecisionApproved)
			rar.Spec.Workflow = nil
			ai := makeAI("ai-" + rr.Name)
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, rar, ai).
				WithStatusSubresource(&remediationv1.RemediationRequest{}, &remediationv1.RemediationApprovalRequest{}).
				Build()
			cbs := noopAwaitingCallbacks()
			created := false
			failed := false
			cbs.WFECallbacks.CreateWFE = func(_ context.Context, _ *remediationv1.RemediationRequest, _ *aianalysisv1.AIAnalysis) (string, bool, error) {
				created = true
				return "we-unexpected", true, nil
			}
			cbs.TransitionToFailed = func(_ context.Context, _ *remediationv1.RemediationRequest, failurePhase remediationv1.FailurePhase, _ error) (ctrl.Result, error) {
				Expect(failurePhase).To(Equal(remediationv1.FailurePhaseApproval))
				failed = true
				return ctrl.Result{}, nil
			}
			h := newHandler(c, cbs)
			_, err := h.Handle(ctx, rr)
			Expect(err).NotTo(HaveOccurred())
			Expect(created).To(BeFalse())
			Expect(failed).To(BeTrue())
		})

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
			cbs.WFECallbacks.CreateWFE = func(_ context.Context, _ *remediationv1.RemediationRequest, _ *aianalysisv1.AIAnalysis) (string, bool, error) {
				wfeCreated = true
				return "wfe-approved", true, nil
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
				return nil, false, override.NewOverrideNotFoundError("deleted-wf", defaultFixture, fmt.Errorf("workflow deleted"))
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
			cbs.WFECallbacks.CreateWFE = func(_ context.Context, _ *remediationv1.RemediationRequest, _ *aianalysisv1.AIAnalysis) (string, bool, error) {
				return "", false, fmt.Errorf("namespace terminating")
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

		It("UT-RO-2564-012: invalid terminal timestamps requeue without changing the parent", func() {
			rr := awaitingRR("apr-invalid-terminal")
			rar := makeRAR(rr.Name, remediationv1.ApprovalDecisionApproved)
			tar := rar.Spec.RequiredBy
			rar.Status.DecidedAt = &tar

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rar).
				Build()

			failedCalled := false
			cbs := noopAwaitingCallbacks()
			cbs.TransitionToFailed = func(_ context.Context, _ *remediationv1.RemediationRequest, _ remediationv1.FailurePhase, _ error) (ctrl.Result, error) {
				failedCalled = true
				return ctrl.Result{}, nil
			}

			h := newHandler(c, cbs)
			intent, err := h.Handle(ctx, rr)
			Expect(err).NotTo(HaveOccurred())
			Expect(failedCalled).To(BeFalse())
			Expect(intent.RequeueAfter).To(Equal(config.RequeueGenericError))
		})

		It("UT-RO-2564-012: missing actor on Rejected requeues without failing the parent", func() {
			rr := awaitingRR("apr-invalid-rejected")
			rar := makeRAR(rr.Name, remediationv1.ApprovalDecisionRejected)
			rar.Status.DecidedBy = ""

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rar).
				Build()

			failedCalled := false
			cbs := noopAwaitingCallbacks()
			cbs.TransitionToFailed = func(_ context.Context, _ *remediationv1.RemediationRequest, _ remediationv1.FailurePhase, _ error) (ctrl.Result, error) {
				failedCalled = true
				return ctrl.Result{}, nil
			}

			h := newHandler(c, cbs)
			intent, err := h.Handle(ctx, rr)
			Expect(err).NotTo(HaveOccurred())
			Expect(failedCalled).To(BeFalse())
			Expect(intent.RequeueAfter).To(Equal(config.RequeueGenericError))
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
			cbs.ExpireRAR = func(_ context.Context, _ *remediationv1.RemediationApprovalRequest) (prodcontroller.ExpiryOutcome, error) {
				expireCalled = true
				return prodcontroller.ExpiredConfirmed, nil
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

		It("UT-RO-2564-012: expiry race with invalid winning decision requeues safely", func() {
			rr := awaitingRR("apr-invalid-race")
			rar := makeRAR(rr.Name, remediationv1.ApprovalDecisionPending)
			tar := metav1.NewTime(time.Now().Add(-1 * time.Hour))
			rar.Spec.RequiredBy = tar

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rar).
				Build()

			failedCalled := false
			cbs := noopAwaitingCallbacks()
			cbs.ExpireRAR = func(_ context.Context, current *remediationv1.RemediationApprovalRequest) (prodcontroller.ExpiryOutcome, error) {
				current.Status.Decision = remediationv1.ApprovalDecisionApproved
				current.Status.DecidedBy = "system"
				decidedAt := metav1.Now()
				current.Status.DecidedAt = &decidedAt
				return prodcontroller.DecisionWon, nil
			}
			cbs.TransitionToFailed = func(_ context.Context, _ *remediationv1.RemediationRequest, _ remediationv1.FailurePhase, _ error) (ctrl.Result, error) {
				failedCalled = true
				return ctrl.Result{}, nil
			}

			h := newHandler(c, cbs)
			intent, err := h.Handle(ctx, rr)
			Expect(err).NotTo(HaveOccurred())
			Expect(failedCalled).To(BeFalse())
			Expect(intent.RequeueAfter).To(Equal(config.RequeueGenericError))
		})
	})
})
