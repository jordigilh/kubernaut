package handler_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	eav1alpha1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/handler"
)

var _ = Describe("BuildPhaseMetadata", func() {
	It("UT-AF-1460-005: Executing phase returns workflow_id, started_at", func() {
		now := metav1.Now()
		fp := remediationv1.FailurePhaseWorkflowExecution
		rr := &remediationv1.RemediationRequest{
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase:       remediationv1.PhaseExecuting,
				ExecutingStartTime: &now,
				SelectedWorkflowRef: &remediationv1.WorkflowReference{
					WorkflowID: "git-revert-v2",
				},
				FailurePhase: &fp,
			},
		}

		meta := handler.BuildPhaseMetadata(rr, nil)

		Expect(meta).To(HaveKey("workflow_id"))
		Expect(meta["workflow_id"]).To(Equal("git-revert-v2"))
		Expect(meta).To(HaveKey("started_at"))
	})

	It("UT-AF-1460-006: Verifying phase returns verification_deadline, started_at, ea_phase, stabilization_deadline", func() {
		now := metav1.Now()
		deadline := metav1.NewTime(now.Add(10 * time.Minute))
		rr := &remediationv1.RemediationRequest{
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase:         remediationv1.PhaseVerifying,
				VerificationDeadline: &deadline,
			},
		}
		stabilizationEnd := metav1.NewTime(now.Add(5 * time.Minute))
		ea := &eav1alpha1.EffectivenessAssessment{
			Status: eav1alpha1.EffectivenessAssessmentStatus{
				Phase:                "Stabilizing",
				PrometheusCheckAfter: &stabilizationEnd,
			},
		}

		meta := handler.BuildPhaseMetadata(rr, ea)

		Expect(meta).To(HaveKey("verification_deadline"))
		Expect(meta).To(HaveKey("ea_phase"))
		Expect(meta["ea_phase"]).To(Equal("Stabilizing"))
		Expect(meta).To(HaveKey("stabilization_deadline"))
	})

	It("UT-AF-1460-007: Blocked phase returns blocked_until, block_reason, block_message", func() {
		blockedTime := metav1.NewTime(time.Now().Add(1 * time.Hour))
		rr := &remediationv1.RemediationRequest{
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseBlocked,
				BlockReason:  remediationv1.BlockReasonConsecutiveFailures,
				BlockMessage: "3 consecutive failures. Cooldown expires: 2026-06-18T14:00:00Z",
				BlockedUntil: &blockedTime,
			},
		}

		meta := handler.BuildPhaseMetadata(rr, nil)

		Expect(meta).To(HaveKey("blocked_until"))
		Expect(meta).To(HaveKey("block_reason"))
		Expect(meta["block_reason"]).To(Equal(string(remediationv1.BlockReasonConsecutiveFailures)))
		Expect(meta).To(HaveKey("block_message"))
	})

	It("UT-AF-1493-006 (was UT-AF-1460-008a): AwaitingApproval phase returns approval_request_name with namespace prefix", func() {
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rr-approval-test",
				Namespace: "kubernaut-system",
			},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseAwaitingApproval,
			},
		}
		meta := handler.BuildPhaseMetadata(rr, nil)
		Expect(meta).To(HaveKey("approval_request_name"))
		Expect(meta["approval_request_name"]).To(Equal("kubernaut-system/rar-rr-approval-test"))
	})

	It("IT-AF-1493-002: namespace prefix flows through BuildPhaseMetadata for AwaitingApproval", func() {
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rr-payment-fix",
				Namespace: "payments",
			},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseAwaitingApproval,
			},
		}
		meta := handler.BuildPhaseMetadata(rr, nil)
		Expect(meta).To(HaveKey("approval_request_name"))
		Expect(meta["approval_request_name"]).To(Equal("payments/rar-rr-payment-fix"))
	})

	It("UT-AF-1468-001: metadata contains investigation identity fields from RR spec (AU-3)", func() {
		rr := &remediationv1.RemediationRequest{
			Spec: remediationv1.RemediationRequestSpec{
				SignalName: "KubePodCrashLooping",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind:      "Deployment",
					Name:      "worker",
					Namespace: "demo-storefront",
				},
			},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseExecuting,
			},
		}

		meta := handler.BuildPhaseMetadata(rr, nil)

		Expect(meta).To(HaveKeyWithValue("namespace", "demo-storefront"),
			"AU-3: namespace enables scoping audit records to the correct tenant boundary")
		Expect(meta).To(HaveKeyWithValue("target", "Deployment/worker"),
			"AU-3: target must use Kind/Name format so audit records unambiguously identify the resource without requiring namespace inference")
		Expect(meta).To(HaveKeyWithValue("kind", "Deployment"),
			"AU-3: kind preserved as a separate structured field for programmatic consumers")
		Expect(meta).To(HaveKeyWithValue("alert_name", "KubePodCrashLooping"),
			"AU-3: alert_name links the remediation to its triggering signal for causal traceability")
	})

	It("UT-AF-1468-002: empty spec fields are omitted from metadata (SI-10)", func() {
		rr := &remediationv1.RemediationRequest{
			Spec: remediationv1.RemediationRequestSpec{
				SignalName: "NodeNotReady",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind: "Node",
					Name: "node-1",
					// Namespace intentionally empty (cluster-scoped resource)
				},
			},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseExecuting,
			},
		}

		meta := handler.BuildPhaseMetadata(rr, nil)

		Expect(meta).NotTo(HaveKey("namespace"),
			"SI-10: cluster-scoped resources must not emit empty namespace to minimize data exposure")
		Expect(meta).To(HaveKeyWithValue("kind", "Node"))
		Expect(meta).To(HaveKeyWithValue("target", "Node/node-1"),
			"AU-3: cluster-scoped target still uses Kind/Name for unambiguous identification")
		Expect(meta).To(HaveKeyWithValue("alert_name", "NodeNotReady"))
	})

	It("UT-AF-1468-003: spec context coexists with phase-specific fields (AU-3)", func() {
		now := metav1.Now()
		rr := &remediationv1.RemediationRequest{
			Spec: remediationv1.RemediationRequestSpec{
				SignalName: "KubePodCrashLooping",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind:      "Deployment",
					Name:      "api-server",
					Namespace: "production",
				},
			},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase:       remediationv1.PhaseExecuting,
				ExecutingStartTime: &now,
				SelectedWorkflowRef: &remediationv1.WorkflowReference{
					WorkflowID: "git-revert-v2",
				},
			},
		}

		meta := handler.BuildPhaseMetadata(rr, nil)

		Expect(meta).To(HaveKeyWithValue("namespace", "production"),
			"AU-3: namespace present alongside phase fields for complete audit record")
		Expect(meta).To(HaveKeyWithValue("target", "Deployment/api-server"),
			"AU-3: Kind/Name target persists through phase transitions for continuous traceability")
		Expect(meta).To(HaveKeyWithValue("kind", "Deployment"))
		Expect(meta).To(HaveKeyWithValue("alert_name", "KubePodCrashLooping"))
		Expect(meta).To(HaveKeyWithValue("workflow_id", "git-revert-v2"))
		Expect(meta).To(HaveKey("started_at"))
	})

	It("UT-AF-1460-008: terminal phases return outcome/failure_reason/skip_reason", func() {
		rr := &remediationv1.RemediationRequest{
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseCompleted,
				Outcome:      "Remediated",
			},
		}
		meta := handler.BuildPhaseMetadata(rr, nil)
		Expect(meta).To(HaveKey("outcome"))
		Expect(meta["outcome"]).To(Equal("Remediated"))

		failReason := "workflow execution error"
		fpPhase := remediationv1.FailurePhaseWorkflowExecution
		rrFailed := &remediationv1.RemediationRequest{
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase:  remediationv1.PhaseFailed,
				FailureReason: &failReason,
				FailurePhase:  &fpPhase,
			},
		}
		metaFailed := handler.BuildPhaseMetadata(rrFailed, nil)
		Expect(metaFailed).To(HaveKey("failure_reason"))
		Expect(metaFailed["failure_reason"]).To(Equal(failReason))
		Expect(metaFailed).To(HaveKey("failure_phase"))

		rrSkipped := &remediationv1.RemediationRequest{
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseSkipped,
				SkipReason:   remediationv1.SkipReasonRecentlyRemediated,
			},
		}
		metaSkipped := handler.BuildPhaseMetadata(rrSkipped, nil)
		Expect(metaSkipped).To(HaveKey("skip_reason"))
		Expect(metaSkipped["skip_reason"]).To(Equal(string(remediationv1.SkipReasonRecentlyRemediated)))
	})
})
