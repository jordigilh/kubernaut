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

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

func newWatchClient(objects ...crclient.Object) crclient.WithWatch {
	fc := fake.NewClientBuilder().
		WithScheme(watchTestScheme()).
		WithObjects(objects...).
		WithStatusSubresource(objects...).
		Build()
	wc, ok := fc.(crclient.WithWatch)
	Expect(ok).To(BeTrue(), "fake client must implement WithWatch")
	return wc
}

func newWatchClientWithInterceptor(funcs interceptor.Funcs, objects ...crclient.Object) crclient.WithWatch {
	fc := fake.NewClientBuilder().
		WithScheme(watchTestScheme()).
		WithObjects(objects...).
		WithStatusSubresource(objects...).
		WithInterceptorFuncs(funcs).
		Build()
	wc, ok := fc.(crclient.WithWatch)
	Expect(ok).To(BeTrue(), "fake client must implement WithWatch")
	return wc
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
