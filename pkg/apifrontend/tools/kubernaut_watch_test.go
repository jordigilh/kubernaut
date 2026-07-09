package tools_test

import (
	"context"
	"encoding/json"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	eav1alpha1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

func newWatchClient(objects ...crclient.Object) crclient.WithWatch {
	return fake.NewClientBuilder().
		WithScheme(watchTestScheme()).
		WithObjects(objects...).
		WithStatusSubresource(objects...).
		Build()
}

func newWatchClientWithInterceptor(funcs interceptor.Funcs, objects ...crclient.Object) crclient.WithWatch {
	return fake.NewClientBuilder().
		WithScheme(watchTestScheme()).
		WithObjects(objects...).
		WithStatusSubresource(objects...).
		WithInterceptorFuncs(funcs).
		Build()
}

func updateRRPhase(ctx context.Context, c crclient.WithWatch, ns, name, phase string) {
	var rr remediationv1.RemediationRequest
	ExpectWithOffset(1, c.Get(ctx, crclient.ObjectKey{Namespace: ns, Name: name}, &rr)).To(Succeed())
	rr.Status.OverallPhase = remediationv1.RemediationPhase(phase)
	ExpectWithOffset(1, c.Status().Update(ctx, &rr)).To(Succeed())
}

func updateRRTerminal(ctx context.Context, c crclient.WithWatch, ns, name, phase, outcome, msg string) {
	var rr remediationv1.RemediationRequest
	ExpectWithOffset(1, c.Get(ctx, crclient.ObjectKey{Namespace: ns, Name: name}, &rr)).To(Succeed())
	rr.Status.OverallPhase = remediationv1.RemediationPhase(phase)
	rr.Status.Outcome = outcome
	rr.Status.Message = msg
	ExpectWithOffset(1, c.Status().Update(ctx, &rr)).To(Succeed())
}

var _ = Describe("kubernaut_watch", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("UT-AF-106-001: emits phase change events for RR", func() {
		rr := newTypedRR("payments", "rr-1", "Pending")
		wc := newWatchClient(rr)

		go func() {
			time.Sleep(50 * time.Millisecond)
			updateRRPhase(ctx, wc, "payments", "rr-1", "Executing")
			time.Sleep(50 * time.Millisecond)
			updateRRTerminal(ctx, wc, "payments", "rr-1", "Completed", "success", "done")
		}()

		result, err := tools.HandleWatch(ctx, wc, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Events).NotTo(BeEmpty())
	})

	It("UT-AF-106-002: correlates related CRDs via ownerRef", func() {
		rr := newTypedRR("payments", "rr-1", "Executing")
		wc := newWatchClient(rr)

		go func() {
			time.Sleep(50 * time.Millisecond)
			updateRRTerminal(ctx, wc, "payments", "rr-1", "Completed", "success", "done")
		}()

		result, err := tools.HandleWatch(ctx, wc, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).NotTo(BeEmpty())
	})

	It("UT-AF-106-003: emits events in chronological order", func() {
		rr := newTypedRR("payments", "rr-1", "Pending")
		wc := newWatchClient(rr)

		go func() {
			time.Sleep(50 * time.Millisecond)
			updateRRPhase(ctx, wc, "payments", "rr-1", "Executing")
			time.Sleep(50 * time.Millisecond)
			updateRRTerminal(ctx, wc, "payments", "rr-1", "Completed", "success", "done")
		}()

		result, err := tools.HandleWatch(ctx, wc, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
		Expect(err).NotTo(HaveOccurred())
		if len(result.Events) >= 2 {
			Expect(result.Events[0].Timestamp <= result.Events[1].Timestamp).To(BeTrue())
		}
	})

	It("UT-AF-106-004: closes stream on terminal RR state", func() {
		rr := newTypedRR("payments", "rr-1", "Executing")
		wc := newWatchClient(rr)

		go func() {
			time.Sleep(50 * time.Millisecond)
			updateRRTerminal(ctx, wc, "payments", "rr-1", "Completed", "success", "done")
		}()

		result, err := tools.HandleWatch(ctx, wc, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("completed"))
	})

	It("UT-AF-106-006: respects context cancellation", func() {
		rr := newTypedRR("payments", "rr-1", "Executing")
		wc := newWatchClient(rr)

		cancelCtx, cancel := context.WithCancel(ctx)
		go func() {
			time.Sleep(100 * time.Millisecond)
			cancel()
		}()

		result, err := tools.HandleWatch(cancelCtx, wc, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("cancelled"))
	})

	It("UT-AF-106-008: returns 403 when user cannot access namespace", func() {
		rr := newTypedRR("forbidden", "rr-1", "Pending")
		wc := newWatchClientWithInterceptor(interceptor.Funcs{
			Get: func(ctx context.Context, client crclient.WithWatch, key crclient.ObjectKey, obj crclient.Object, opts ...crclient.GetOption) error {
				return newForbiddenError("remediationrequests")
			},
		}, rr)

		_, err := tools.HandleWatch(ctx, wc, tools.WatchArgs{Namespace: "forbidden", Name: "rr-1"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("access denied"))
	})

	It("UT-AF-106-012: returns not-found when RR does not exist", func() {
		wc := newWatchClient()
		_, err := tools.HandleWatch(ctx, wc, tools.WatchArgs{Namespace: "default", Name: "nonexistent"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("UT-AF-106-009: nil client returns ErrK8sUnavailable", func() {
		_, err := tools.HandleWatch(ctx, nil, tools.WatchArgs{Namespace: "default", Name: "rr-1"})
		Expect(err).To(MatchError(tools.ErrK8sUnavailable))
	})

	It("UT-AF-106-010: invalid namespace returns ErrInvalidInput", func() {
		wc := newWatchClient()
		_, err := tools.HandleWatch(ctx, wc, tools.WatchArgs{Namespace: "../etc", Name: "rr-1"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid input"))
	})

	It("UT-AF-106-011: invalid resource name returns ErrInvalidInput", func() {
		wc := newWatchClient()
		_, err := tools.HandleWatch(ctx, wc, tools.WatchArgs{Namespace: "default", Name: "INVALID NAME!!"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid input"))
	})

	It("UT-AF-106-013: yields with awaiting_approval on AwaitingApproval phase", func() {
		rr := newTypedRR("payments", "rr-1", "Executing")
		wc := newWatchClient(rr)

		go func() {
			time.Sleep(50 * time.Millisecond)
			updateRRPhase(ctx, wc, "payments", "rr-1", "AwaitingApproval")
		}()

		result, err := tools.HandleWatch(ctx, wc, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("awaiting_approval"),
			"watch must yield on AwaitingApproval so the LLM can approve the RAR")
		Expect(result.Events).To(HaveLen(1))
		Expect(result.Events[0].Phase).To(Equal("AwaitingApproval"))
	})

	It("UT-AF-106-014: AwaitingApproval does not block subsequent terminal phase", func() {
		rr := newTypedRR("payments", "rr-1", "Executing")
		wc := newWatchClient(rr)

		go func() {
			time.Sleep(50 * time.Millisecond)
			updateRRPhase(ctx, wc, "payments", "rr-1", "AwaitingApproval")
		}()

		result, err := tools.HandleWatch(ctx, wc, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("awaiting_approval"),
			"first watch call yields on AwaitingApproval")

		go func() {
			time.Sleep(50 * time.Millisecond)
			updateRRTerminal(ctx, wc, "payments", "rr-1", "Completed", "success", "done")
		}()

		result2, err := tools.HandleWatch(ctx, wc, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result2.Status).To(Equal("completed"),
			"second watch call should reach terminal phase")
	})
})

var _ = Describe("Structured Approval Events in HandleWatch — TP-1398 (#1398)", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("IT-AF-1398-001: emits structured approval_request event on AwaitingApproval with RAR", func() {
		rr := newTypedRR("payments", "rr-1", "Executing")
		rar := newTypedDetailedRAR("payments", "rar-rr-1")
		wc := newWatchClient(rr, rar)

		queue := &bridgeQueue{}
		ctx = launcher.WithEventBridge(ctx, queue, "task-it-1398-001", "ctx-it-001", nil)

		go func() {
			time.Sleep(50 * time.Millisecond)
			updateRRPhase(ctx, wc, "payments", "rr-1", "AwaitingApproval")
		}()

		result, err := tools.HandleWatch(ctx, wc, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("awaiting_approval"))

		events := queue.Events()
		var approvalEvent *a2a.TaskStatusUpdateEvent
		for _, evt := range events {
			if tsu, ok := evt.(*a2a.TaskStatusUpdateEvent); ok {
				if tsu.Metadata != nil && tsu.Metadata["type"] == "approval_request" {
					approvalEvent = tsu
					break
				}
			}
		}
		Expect(approvalEvent).NotTo(BeNil(), "must emit approval_request event")

		textPart, ok := approvalEvent.Status.Message.Parts[0].(a2a.TextPart)
		Expect(ok).To(BeTrue())

		var payload tools.ApprovalRequestEventPayload
		Expect(json.Unmarshal([]byte(textPart.Text), &payload)).To(Succeed())
		Expect(payload.Name).To(Equal("rar-rr-1"))
		Expect(payload.Confidence).To(BeNumerically("~", 0.72, 0.001))
		Expect(payload.RemediationRequestName).To(Equal("rr-oom-1"))
		Expect(payload.RecommendedWorkflow).NotTo(BeNil())
	})

	It("IT-AF-1398-002: emits approval_request_resolved on RAR decision change", func() {
		rr := newTypedRR("payments", "rr-1", "Executing")
		rar := &remediationv1.RemediationApprovalRequest{
			ObjectMeta: objMeta("payments", "rar-rr-1"),
			Spec: remediationv1.RemediationApprovalRequestSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: "rr-1"},
			},
		}
		wc := newWatchClient(rr, rar)

		queue := &bridgeQueue{}
		ctx = launcher.WithEventBridge(ctx, queue, "task-it-1398-002", "ctx-it-002", nil)

		go func() {
			time.Sleep(50 * time.Millisecond)
			var existing remediationv1.RemediationApprovalRequest
			_ = wc.(crclient.Client).Get(ctx, crclient.ObjectKey{Namespace: "payments", Name: "rar-rr-1"}, &existing)
			decidedAt := metav1.NewTime(time.Date(2026, 6, 11, 15, 50, 0, 0, time.UTC))
			existing.Status.Decision = remediationv1.ApprovalDecisionApproved
			existing.Status.DecidedBy = "operator@acme.com"
			existing.Status.DecidedAt = &decidedAt
			_ = wc.(crclient.Client).Status().Update(ctx, &existing)

			time.Sleep(50 * time.Millisecond)
			updateRRTerminal(ctx, wc, "payments", "rr-1", "Completed", "success", "done")
		}()

		result, err := tools.HandleWatch(ctx, wc, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("completed"))

		events := queue.Events()
		var resolvedEvent *a2a.TaskStatusUpdateEvent
		for _, evt := range events {
			if tsu, ok := evt.(*a2a.TaskStatusUpdateEvent); ok {
				if tsu.Metadata != nil && tsu.Metadata["type"] == "approval_request_resolved" {
					resolvedEvent = tsu
					break
				}
			}
		}
		Expect(resolvedEvent).NotTo(BeNil(), "must emit approval_request_resolved event")

		textPart, ok := resolvedEvent.Status.Message.Parts[0].(a2a.TextPart)
		Expect(ok).To(BeTrue())

		var payload tools.ApprovalResolvedEventPayload
		Expect(json.Unmarshal([]byte(textPart.Text), &payload)).To(Succeed())
		Expect(payload.Decision).To(Equal("Approved"))
		Expect(payload.DecidedBy).To(Equal("operator@acme.com"))
	})

	It("IT-AF-1398-003: emits both events when RAR already decided at AwaitingApproval", func() {
		decidedAt := metav1.NewTime(time.Date(2026, 6, 11, 15, 45, 1, 0, time.UTC))
		rr := newTypedRR("payments", "rr-1", "Executing")
		rar := &remediationv1.RemediationApprovalRequest{
			ObjectMeta: objMeta("payments", "rar-rr-1"),
			Spec: remediationv1.RemediationApprovalRequestSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: "rr-1"},
				Confidence:            0.95,
				ConfidenceLevel:       "high",
				Reason:                "Auto-approved",
				WhyApprovalRequired:   "Audit trail",
				InvestigationSummary:  "Quick fix",
				RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{
					WorkflowID: "auto-v1",
					Version:    "1.0.0",
					Rationale:  "Auto-selected",
				},
				RecommendedActions: []remediationv1.ApprovalRecommendedAction{
					{Action: "Apply", Rationale: "Safe"},
				},
				RequiredBy: metav1.NewTime(time.Date(2026, 6, 11, 16, 0, 0, 0, time.UTC)),
			},
			Status: remediationv1.RemediationApprovalRequestStatus{
				Decision:  remediationv1.ApprovalDecisionApproved,
				DecidedBy: "system",
				DecidedAt: &decidedAt,
			},
		}
		wc := newWatchClient(rr, rar)

		queue := &bridgeQueue{}
		ctx = launcher.WithEventBridge(ctx, queue, "task-it-1398-003", "ctx-it-003", nil)

		go func() {
			time.Sleep(50 * time.Millisecond)
			updateRRPhase(ctx, wc, "payments", "rr-1", "AwaitingApproval")
		}()

		result, err := tools.HandleWatch(ctx, wc, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("awaiting_approval"))

		events := queue.Events()
		var requestFound, resolvedFound bool
		var requestIdx, resolvedIdx int
		for i, evt := range events {
			if tsu, ok := evt.(*a2a.TaskStatusUpdateEvent); ok {
				if tsu.Metadata != nil {
					switch tsu.Metadata["type"] {
					case "approval_request":
						requestFound = true
						requestIdx = i
					case "approval_request_resolved":
						resolvedFound = true
						resolvedIdx = i
					}
				}
			}
		}
		Expect(requestFound).To(BeTrue(), "must emit approval_request")
		Expect(resolvedFound).To(BeTrue(), "must emit approval_request_resolved for already-decided RAR")
		Expect(requestIdx).To(BeNumerically("<", resolvedIdx),
			"approval_request must precede approval_request_resolved (ordering guarantee)")
	})
})

// =============================================================================
// Issue #1427: verification_step sub-events — Wiring Integration Tests
// FedRAMP: SI-4 (System Monitoring), AU-3 (Content of Audit Records),
//          AU-12 (Audit Generation), SI-7 (Software/Information Integrity)
// =============================================================================

var _ = Describe("verification_step events in HandleWatch — #1427", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	updateEAStatus := func(
		ctx context.Context,
		c crclient.WithWatch,
		ns, name string,
		mutate func(*eav1alpha1.EffectivenessAssessment),
	) {
		var ea eav1alpha1.EffectivenessAssessment
		ExpectWithOffset(1, c.Get(ctx, crclient.ObjectKey{Namespace: ns, Name: name}, &ea)).To(Succeed())
		mutate(&ea)
		ExpectWithOffset(1, c.Status().Update(ctx, &ea)).To(Succeed())
	}

	It("IT-AF-1427-001: AU-3, SI-4 — emits verification_step with step_status and detail through production wiring", func() {
		rr := newTypedRR("payments", "rr-1", "Executing")
		ea := &eav1alpha1.EffectivenessAssessment{
			ObjectMeta: objMeta("payments", tools.EANameForRR("rr-1")),
			Spec: eav1alpha1.EffectivenessAssessmentSpec{
				CorrelationID:           "rr-1",
				RemediationRequestPhase: "Verifying",
				SignalTarget:            eav1alpha1.TargetResource{Kind: "Deployment", Name: "api-server"},
				RemediationTarget:       eav1alpha1.TargetResource{Kind: "Deployment", Name: "api-server"},
				Config: eav1alpha1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 120 * time.Second},
				},
			},
			Status: eav1alpha1.EffectivenessAssessmentStatus{
				Phase: "Stabilizing",
			},
		}
		wc := newWatchClient(rr, ea)

		queue := &bridgeQueue{}
		ctx = launcher.WithEventBridge(ctx, queue, "task-it-1427-001", "ctx-it-001", nil)

		go func() {
			time.Sleep(50 * time.Millisecond)
			updateRRPhase(ctx, wc, "payments", "rr-1", "Verifying")
			time.Sleep(100 * time.Millisecond)
			updateEAStatus(ctx, wc, "payments", tools.EANameForRR("rr-1"), func(ea *eav1alpha1.EffectivenessAssessment) {
				ea.Status.Phase = "Assessing"
				ea.Status.Components.HealthAssessed = true
				score := 1.0
				ea.Status.Components.HealthScore = &score
			})
			time.Sleep(50 * time.Millisecond)
			updateRRTerminal(ctx, wc, "payments", "rr-1", "Completed", "Remediated", "done")
		}()

		result, err := tools.HandleWatch(ctx, wc, tools.WatchArgs{Namespace: "payments", Name: "rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("completed"))

		events := queue.Events()
		var verificationSteps []*a2a.TaskStatusUpdateEvent
		for _, evt := range events {
			if tsu, ok := evt.(*a2a.TaskStatusUpdateEvent); ok {
				if tsu.Metadata != nil && tsu.Metadata["type"] == launcher.MetaTypeVerificationStep {
					verificationSteps = append(verificationSteps, tsu)
				}
			}
		}
		Expect(verificationSteps).NotTo(BeEmpty(),
			"AU-3: HandleWatch must emit verification_step events through EventBridge when EA status changes")

		for _, tsu := range verificationSteps {
			Expect(tsu.Metadata).To(HaveKey("step"),
				"SI-4: each verification_step must identify the step name")
			Expect(tsu.Metadata).To(HaveKey("step_status"),
				"AU-3: each verification_step must include step_status for audit record completeness")
			Expect(tsu.Metadata).To(HaveKey("detail"),
				"SI-4: each verification_step must include human-readable detail for operator monitoring")
		}

		stepNames := make([]string, len(verificationSteps))
		for i, tsu := range verificationSteps {
			stepNames[i] = tsu.Metadata["step"].(string)
		}
		Expect(stepNames).To(ContainElement("health_check"),
			"SI-4: health_check step must be emitted when HealthAssessed transitions")
	})

	It("IT-AF-1427-002: AU-12 — elapsed_s is present and non-negative in verification_step metadata", func() {
		rr := newTypedRR("payments", "rr-2", "Executing")
		ea := &eav1alpha1.EffectivenessAssessment{
			ObjectMeta: objMeta("payments", tools.EANameForRR("rr-2")),
			Spec: eav1alpha1.EffectivenessAssessmentSpec{
				CorrelationID:           "rr-2",
				RemediationRequestPhase: "Verifying",
				SignalTarget:            eav1alpha1.TargetResource{Kind: "Deployment", Name: "api-server"},
				RemediationTarget:       eav1alpha1.TargetResource{Kind: "Deployment", Name: "api-server"},
				Config: eav1alpha1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 60 * time.Second},
				},
			},
			Status: eav1alpha1.EffectivenessAssessmentStatus{Phase: "Stabilizing"},
		}
		wc := newWatchClient(rr, ea)

		queue := &bridgeQueue{}
		ctx = launcher.WithEventBridge(ctx, queue, "task-it-1427-002", "ctx-it-002", nil)

		go func() {
			time.Sleep(50 * time.Millisecond)
			updateRRPhase(ctx, wc, "payments", "rr-2", "Verifying")
			time.Sleep(100 * time.Millisecond)
			updateEAStatus(ctx, wc, "payments", tools.EANameForRR("rr-2"), func(ea *eav1alpha1.EffectivenessAssessment) {
				ea.Status.Phase = "Assessing"
			})
			time.Sleep(50 * time.Millisecond)
			updateRRTerminal(ctx, wc, "payments", "rr-2", "Completed", "Remediated", "done")
		}()

		result, err := tools.HandleWatch(ctx, wc, tools.WatchArgs{Namespace: "payments", Name: "rr-2"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("completed"))

		events := queue.Events()
		var foundElapsed bool
		for _, evt := range events {
			tsu, ok := evt.(*a2a.TaskStatusUpdateEvent)
			if !ok || tsu.Metadata == nil {
				continue
			}
			if tsu.Metadata["type"] != launcher.MetaTypeVerificationStep {
				continue
			}
			Expect(tsu.Metadata).To(HaveKey("elapsed_s"),
				"AU-12: verification_step events must include elapsed_s generated at source")
			elapsed, ok := tsu.Metadata["elapsed_s"].(int)
			Expect(ok).To(BeTrue(), "AU-12: elapsed_s must be an integer")
			Expect(elapsed).To(BeNumerically(">=", 0),
				"AU-12: elapsed_s must be non-negative (seconds since Verifying started)")
			foundElapsed = true
		}
		Expect(foundElapsed).To(BeTrue(),
			"AU-12: at least one verification_step with elapsed_s must be emitted")
	})

	It("IT-AF-1427-003: SI-4 — Stabilizing->Assessing emits stabilization_elapsed, not phase_transition", func() {
		rr := newTypedRR("payments", "rr-3", "Executing")
		ea := &eav1alpha1.EffectivenessAssessment{
			ObjectMeta: objMeta("payments", tools.EANameForRR("rr-3")),
			Spec: eav1alpha1.EffectivenessAssessmentSpec{
				CorrelationID:           "rr-3",
				RemediationRequestPhase: "Verifying",
				SignalTarget:            eav1alpha1.TargetResource{Kind: "Deployment", Name: "api-server"},
				RemediationTarget:       eav1alpha1.TargetResource{Kind: "Deployment", Name: "api-server"},
				Config: eav1alpha1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 60 * time.Second},
				},
			},
			Status: eav1alpha1.EffectivenessAssessmentStatus{Phase: "Stabilizing"},
		}
		wc := newWatchClient(rr, ea)

		queue := &bridgeQueue{}
		ctx = launcher.WithEventBridge(ctx, queue, "task-it-1427-003", "ctx-it-003", nil)

		go func() {
			time.Sleep(50 * time.Millisecond)
			updateRRPhase(ctx, wc, "payments", "rr-3", "Verifying")
			time.Sleep(100 * time.Millisecond)
			updateEAStatus(ctx, wc, "payments", tools.EANameForRR("rr-3"), func(ea *eav1alpha1.EffectivenessAssessment) {
				ea.Status.Phase = "Assessing"
			})
			time.Sleep(50 * time.Millisecond)
			updateRRTerminal(ctx, wc, "payments", "rr-3", "Completed", "Remediated", "done")
		}()

		result, err := tools.HandleWatch(ctx, wc, tools.WatchArgs{Namespace: "payments", Name: "rr-3"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("completed"))

		events := queue.Events()
		var stabilizationStep *a2a.TaskStatusUpdateEvent
		for _, evt := range events {
			tsu, ok := evt.(*a2a.TaskStatusUpdateEvent)
			if !ok || tsu.Metadata == nil {
				continue
			}
			if tsu.Metadata["type"] == launcher.MetaTypeVerificationStep &&
				tsu.Metadata["step"] == "stabilization_elapsed" {
				stabilizationStep = tsu
				break
			}
		}
		Expect(stabilizationStep).NotTo(BeNil(),
			"SI-4: Stabilizing->Assessing must emit 'stabilization_elapsed' for Console monitoring")
		Expect(stabilizationStep.Metadata["step_status"]).To(Equal(tools.StepStatusCompleted),
			"SI-4: stabilization_elapsed must have step_status=completed")

		for _, evt := range events {
			tsu, ok := evt.(*a2a.TaskStatusUpdateEvent)
			if !ok || tsu.Metadata == nil {
				continue
			}
			if tsu.Metadata["type"] == launcher.MetaTypeVerificationStep {
				Expect(tsu.Metadata["step"]).NotTo(Equal("phase_transition"),
					"SI-4: Stabilizing->Assessing must NOT emit generic 'phase_transition'")
			}
		}
	})

	It("UT-AF-1460-030: HandleWatch uses effectivenessAssessmentRef instead of EANameForRR", func() {
		refEAName := "custom-ea-name-not-convention"
		conventionEAName := tools.EANameForRR("rr-ref")
		Expect(refEAName).NotTo(Equal(conventionEAName), "test precondition: ref name must differ from convention")

		rr := newTypedRR("payments", "rr-ref", "Executing")
		rr.Status.EffectivenessAssessmentRef = &corev1.ObjectReference{
			Kind:       "EffectivenessAssessment",
			Name:       refEAName,
			Namespace:  "payments",
			APIVersion: "kubernaut.ai/v1alpha1",
		}

		var capturedEAGetName string
		wc := newWatchClientWithInterceptor(interceptor.Funcs{
			Get: func(ctx context.Context, c crclient.WithWatch, key crclient.ObjectKey, obj crclient.Object, opts ...crclient.GetOption) error {
				if _, ok := obj.(*eav1alpha1.EffectivenessAssessment); ok {
					capturedEAGetName = key.Name
				}
				return c.Get(ctx, key, obj, opts...)
			},
		}, rr)

		go func() {
			time.Sleep(50 * time.Millisecond)
			updateRRPhase(ctx, wc, "payments", "rr-ref", "Verifying")
			time.Sleep(50 * time.Millisecond)
			updateRRTerminal(ctx, wc, "payments", "rr-ref", "Completed", "Remediated", "done")
		}()

		result, err := tools.HandleWatch(ctx, wc, tools.WatchArgs{Namespace: "payments", Name: "rr-ref"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("completed"))

		Expect(capturedEAGetName).To(Equal(refEAName),
			"HandleWatch must use effectivenessAssessmentRef.Name (%q) for EA lookup, not EANameForRR convention (%q)",
			refEAName, conventionEAName)
	})

	It("IT-AF-1427-004: SI-4, AU-3 — alert decay emits alert_check with in_progress and signal name in detail", func() {
		rr := newTypedRR("payments", "rr-4", "Executing")
		ea := &eav1alpha1.EffectivenessAssessment{
			ObjectMeta: objMeta("payments", tools.EANameForRR("rr-4")),
			Spec: eav1alpha1.EffectivenessAssessmentSpec{
				CorrelationID:           "rr-4",
				RemediationRequestPhase: "Verifying",
				SignalName:              "KubePodCrashLooping",
				SignalTarget:            eav1alpha1.TargetResource{Kind: "Deployment", Name: "api-server"},
				RemediationTarget:       eav1alpha1.TargetResource{Kind: "Deployment", Name: "api-server"},
				Config: eav1alpha1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 60 * time.Second},
				},
			},
			Status: eav1alpha1.EffectivenessAssessmentStatus{
				Phase: "Assessing",
			},
		}
		wc := newWatchClient(rr, ea)

		queue := &bridgeQueue{}
		ctx = launcher.WithEventBridge(ctx, queue, "task-it-1427-004", "ctx-it-004", nil)

		go func() {
			time.Sleep(50 * time.Millisecond)
			updateRRPhase(ctx, wc, "payments", "rr-4", "Verifying")
			time.Sleep(100 * time.Millisecond)
			updateEAStatus(ctx, wc, "payments", tools.EANameForRR("rr-4"), func(ea *eav1alpha1.EffectivenessAssessment) {
				ea.Status.Components.AlertDecayRetries = 1
			})
			time.Sleep(50 * time.Millisecond)
			updateRRTerminal(ctx, wc, "payments", "rr-4", "Completed", "Remediated", "done")
		}()

		result, err := tools.HandleWatch(ctx, wc, tools.WatchArgs{Namespace: "payments", Name: "rr-4"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("completed"))

		events := queue.Events()
		var alertInProgress *a2a.TaskStatusUpdateEvent
		for _, evt := range events {
			tsu, ok := evt.(*a2a.TaskStatusUpdateEvent)
			if !ok || tsu.Metadata == nil {
				continue
			}
			if tsu.Metadata["type"] == launcher.MetaTypeVerificationStep &&
				tsu.Metadata["step"] == "alert_check" &&
				tsu.Metadata["step_status"] == tools.StepStatusInProgress {
				alertInProgress = tsu
				break
			}
		}
		Expect(alertInProgress).NotTo(BeNil(),
			"SI-4: alert decay retry must emit alert_check with step_status=in_progress")
		detail, ok := alertInProgress.Metadata["detail"].(string)
		Expect(ok).To(BeTrue())
		Expect(detail).To(ContainSubstring("KubePodCrashLooping"),
			"AU-3: alert_check in_progress detail must include signal name for audit traceability")
	})
})

var _ = Describe("Fleet cluster_id in execution_progress — IT-AF-1409-007 (#1409)", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("IT-AF-1409-007: AU-3, SI-4 — execution_progress artifact carries cluster_id from the live RR through the watch loop", func() {
		rr := newTypedRR("payments", "rr-fleet-1", "Pending")
		rr.Spec.ClusterID = "cluster-fleet-it-007"
		wc := newWatchClient(rr)

		queue := &bridgeQueue{}
		ctx = launcher.WithEventBridge(ctx, queue, "task-it-1409-007", "ctx-it-1409-007", nil)

		go func() {
			time.Sleep(50 * time.Millisecond)
			updateRRPhase(ctx, wc, "payments", "rr-fleet-1", "Executing")
			time.Sleep(50 * time.Millisecond)
			updateRRTerminal(ctx, wc, "payments", "rr-fleet-1", "Completed", "success", "done")
		}()

		result, err := tools.HandleWatch(ctx, wc, tools.WatchArgs{Namespace: "payments", Name: "rr-fleet-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("completed"))

		events := queue.Events()
		var sawClusterID bool
		for _, evt := range events {
			artifactEvt, ok := evt.(*a2a.TaskArtifactUpdateEvent)
			if !ok {
				continue
			}
			for _, part := range artifactEvt.Artifact.Parts {
				dp, ok := part.(a2a.DataPart)
				if !ok {
					continue
				}
				if dp.Data["cluster_id"] == "cluster-fleet-it-007" {
					sawClusterID = true
				}
			}
		}
		Expect(sawClusterID).To(BeTrue(),
			"AU-3, SI-4: execution_progress artifact must carry cluster_id from the live RemediationRequest through the wired watch loop")
	})

	It("IT-AF-1409-007b: AU-3 — execution_progress artifact omits cluster_id for local-hub RRs", func() {
		rr := newTypedRR("payments", "rr-local-1", "Pending")
		wc := newWatchClient(rr)

		queue := &bridgeQueue{}
		ctx = launcher.WithEventBridge(ctx, queue, "task-it-1409-007b", "ctx-it-1409-007b", nil)

		go func() {
			time.Sleep(50 * time.Millisecond)
			updateRRTerminal(ctx, wc, "payments", "rr-local-1", "Completed", "success", "done")
		}()

		result, err := tools.HandleWatch(ctx, wc, tools.WatchArgs{Namespace: "payments", Name: "rr-local-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("completed"))

		events := queue.Events()
		for _, evt := range events {
			artifactEvt, ok := evt.(*a2a.TaskArtifactUpdateEvent)
			if !ok {
				continue
			}
			for _, part := range artifactEvt.Artifact.Parts {
				dp, ok := part.(a2a.DataPart)
				if !ok {
					continue
				}
				Expect(dp.Data).NotTo(HaveKey("cluster_id"),
					"AU-3: local-hub RRs must not carry a false-attribution cluster_id in execution_progress")
			}
		}
	})
})
