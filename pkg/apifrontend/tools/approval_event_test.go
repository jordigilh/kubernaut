package tools_test

import (
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

func newTypedDetailedRAR(namespace, name string) *remediationv1.RemediationApprovalRequest {
	return &remediationv1.RemediationApprovalRequest{
		ObjectMeta: objMeta(namespace, name),
		Spec: remediationv1.RemediationApprovalRequestSpec{
			RemediationRequestRef: corev1.ObjectReference{Name: "rr-oom-1"},
			AIAnalysisRef:         remediationv1.ObjectRef{Name: "aia-oom-1"},
			Confidence:            0.72,
			ConfidenceLevel:       "medium",
			Reason:                "Confidence below auto-approve threshold",
			RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{
				WorkflowID:      "oomkill-increase-memory-v1",
				Version:         "1.0.0",
				ExecutionBundle: "oci://registry/oomkill:sha256-abc",
				Rationale:       "Best match for OOMKill scenario",
			},
			InvestigationSummary: "Container exceeded memory limits under traffic spike",
			EvidenceCollected: []string{
				"OOMKill event at 2026-01-15T10:30:00Z",
				"Memory usage 98% of limit",
			},
			RecommendedActions: []remediationv1.ApprovalRecommendedAction{
				{Action: "Increase memory limit to 512Mi", Rationale: "Current limit 256Mi insufficient for traffic spike"},
			},
			AlternativesConsidered: []remediationv1.ApprovalAlternative{
				{Approach: "Horizontal scaling", ProsCons: "Higher cost but better availability vs single node vertical scaling"},
			},
			WhyApprovalRequired: "Confidence 0.72 below auto-approve threshold of 0.80",
			RequiredBy:          metav1.NewTime(time.Date(2026, 1, 15, 10, 45, 0, 0, time.UTC)),
		},
		Status: remediationv1.RemediationApprovalRequestStatus{
			TimeRemaining: "12m30s",
		},
	}
}

var _ = Describe("Approval Event Payload Marshaling — TP-1398", func() {

	It("UT-AF-1398-001: MarshalApprovalRequestPayload produces valid JSON with all spec fields", func() {
		rar := newTypedDetailedRAR("kubernaut-system", "rar-rr-oom-1")

		payload, err := tools.MarshalApprovalRequestPayload(rar)
		Expect(err).NotTo(HaveOccurred())
		Expect(payload).NotTo(BeEmpty())

		var parsed tools.ApprovalRequestEventPayload
		Expect(json.Unmarshal([]byte(payload), &parsed)).To(Succeed())

		Expect(parsed.Name).To(Equal("rar-rr-oom-1"))
		Expect(parsed.Namespace).To(Equal("kubernaut-system"))
		Expect(parsed.Confidence).To(BeNumerically("~", 0.72, 0.001))
		Expect(parsed.ConfidenceLevel).To(Equal("medium"))
		Expect(parsed.Reason).To(Equal("Confidence below auto-approve threshold"))
		Expect(parsed.WhyApprovalRequired).To(Equal("Confidence 0.72 below auto-approve threshold of 0.80"))
		Expect(parsed.InvestigationSummary).To(Equal("Container exceeded memory limits under traffic spike"))
		Expect(parsed.RequiredBy).To(Equal("2026-01-15T10:45:00Z"))

		Expect(parsed.RecommendedWorkflow).NotTo(BeNil())
		Expect(parsed.RecommendedWorkflow.WorkflowID).To(Equal("oomkill-increase-memory-v1"))
		Expect(parsed.RecommendedWorkflow.Version).To(Equal("1.0.0"))
		Expect(parsed.RecommendedWorkflow.Rationale).To(Equal("Best match for OOMKill scenario"))

		Expect(parsed.EvidenceCollected).To(HaveLen(2))
		Expect(parsed.EvidenceCollected[0]).To(ContainSubstring("OOMKill event"))

		Expect(parsed.RecommendedActions).To(HaveLen(1))
		Expect(parsed.RecommendedActions[0].Action).To(Equal("Increase memory limit to 512Mi"))
		Expect(parsed.RecommendedActions[0].Rationale).To(ContainSubstring("256Mi"))

		Expect(parsed.AlternativesConsidered).To(HaveLen(1))
		Expect(parsed.AlternativesConsidered[0].Approach).To(Equal("Horizontal scaling"))
	})

	It("UT-AF-1398-002: payload includes remediationRequestName from spec.remediationRequestRef", func() {
		rar := &remediationv1.RemediationApprovalRequest{
			ObjectMeta: objMeta("kubernaut-system", "rar-rr-gitops-drift-1"),
			Spec: remediationv1.RemediationApprovalRequestSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: "rr-gitops-drift-1"},
				Confidence:            0.65,
				ConfidenceLevel:       "medium",
				Reason:                "Policy requires approval",
				WhyApprovalRequired:   "Production namespace",
				InvestigationSummary:  "Drift detected",
				RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{
					WorkflowID: "drift-v1",
					Version:    "1.0.0",
					Rationale:  "Best match",
				},
				RecommendedActions: []remediationv1.ApprovalRecommendedAction{
					{Action: "Revert", Rationale: "Restore consistency"},
				},
				RequiredBy: metav1.NewTime(time.Date(2026, 6, 11, 16, 0, 0, 0, time.UTC)),
			},
		}

		payload, err := tools.MarshalApprovalRequestPayload(rar)
		Expect(err).NotTo(HaveOccurred())

		var parsed tools.ApprovalRequestEventPayload
		Expect(json.Unmarshal([]byte(payload), &parsed)).To(Succeed())
		Expect(parsed.RemediationRequestName).To(Equal("rr-gitops-drift-1"))
	})

	It("UT-AF-1398-003: MarshalApprovalResolvedPayload includes all decision fields + workflowOverride", func() {
		decidedAt := metav1.NewTime(time.Date(2026, 6, 11, 15, 50, 0, 0, time.UTC))
		rar := &remediationv1.RemediationApprovalRequest{
			ObjectMeta: objMeta("kubernaut-system", "rar-rr-oom-1"),
			Status: remediationv1.RemediationApprovalRequestStatus{
				Decision:        remediationv1.ApprovalDecisionApproved,
				DecidedBy:       "jane@acme.com",
				DecidedAt:       &decidedAt,
				DecisionMessage: "Reviewed evidence, proceeding with revert",
				WorkflowOverride: &remediationv1.WorkflowOverride{
					WorkflowName: "gitops-manual-sync-v1",
					Parameters:   map[string]string{"SYNC_PRUNE": "true"},
					Rationale:    "Prefer sync over revert",
				},
			},
		}

		payload, err := tools.MarshalApprovalResolvedPayload(rar)
		Expect(err).NotTo(HaveOccurred())

		var parsed tools.ApprovalResolvedEventPayload
		Expect(json.Unmarshal([]byte(payload), &parsed)).To(Succeed())

		Expect(parsed.Name).To(Equal("rar-rr-oom-1"))
		Expect(parsed.Decision).To(Equal("Approved"))
		Expect(parsed.DecidedBy).To(Equal("jane@acme.com"))
		Expect(parsed.DecidedAt).To(Equal("2026-06-11T15:50:00Z"))
		Expect(parsed.DecisionMessage).To(Equal("Reviewed evidence, proceeding with revert"))
		Expect(parsed.WorkflowOverride).NotTo(BeNil())
		Expect(parsed.WorkflowOverride.WorkflowName).To(Equal("gitops-manual-sync-v1"))
		Expect(parsed.WorkflowOverride.Parameters).To(HaveKeyWithValue("SYNC_PRUNE", "true"))
		Expect(parsed.WorkflowOverride.Rationale).To(Equal("Prefer sync over revert"))
	})

	It("UT-AF-1398-004: resolution payload omits workflowOverride when nil", func() {
		decidedAt := metav1.NewTime(time.Date(2026, 6, 11, 15, 55, 0, 0, time.UTC))
		rar := &remediationv1.RemediationApprovalRequest{
			ObjectMeta: objMeta("kubernaut-system", "rar-rr-oom-1"),
			Status: remediationv1.RemediationApprovalRequestStatus{
				Decision:  remediationv1.ApprovalDecisionRejected,
				DecidedBy: "bob@acme.com",
				DecidedAt: &decidedAt,
			},
		}

		payload, err := tools.MarshalApprovalResolvedPayload(rar)
		Expect(err).NotTo(HaveOccurred())

		Expect(payload).NotTo(ContainSubstring("workflowOverride"))
		Expect(payload).NotTo(ContainSubstring("null"))

		var parsed tools.ApprovalResolvedEventPayload
		Expect(json.Unmarshal([]byte(payload), &parsed)).To(Succeed())
		Expect(parsed.WorkflowOverride).To(BeNil())
	})

	It("UT-AF-1398-008: missing optional fields produces valid JSON", func() {
		rar := &remediationv1.RemediationApprovalRequest{
			ObjectMeta: objMeta("kubernaut-system", "rar-rr-minimal-1"),
			Spec: remediationv1.RemediationApprovalRequestSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: "rr-minimal-1"},
				Confidence:            0.55,
				ConfidenceLevel:       "low",
				Reason:                "Low confidence requires approval",
				WhyApprovalRequired:   "Below threshold",
				InvestigationSummary:  "Basic investigation",
				RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{
					WorkflowID: "basic-v1",
					Version:    "1.0.0",
					Rationale:  "Only option",
				},
				RecommendedActions: []remediationv1.ApprovalRecommendedAction{
					{Action: "Fix", Rationale: "Needed"},
				},
				RequiredBy: metav1.NewTime(time.Date(2026, 6, 11, 16, 0, 0, 0, time.UTC)),
			},
		}

		payload, err := tools.MarshalApprovalRequestPayload(rar)
		Expect(err).NotTo(HaveOccurred())

		var parsed tools.ApprovalRequestEventPayload
		Expect(json.Unmarshal([]byte(payload), &parsed)).To(Succeed())

		Expect(parsed.PolicyEvaluation).To(BeNil())
		Expect(parsed.EvidenceCollected).To(BeEmpty())
		Expect(parsed.AlternativesConsidered).To(BeEmpty())
		Expect(parsed.Confidence).To(BeNumerically("~", 0.55, 0.001))
	})

	It("UT-AF-1398-009: RAR with existing decision includes decision in request payload context", func() {
		rar := &remediationv1.RemediationApprovalRequest{
			ObjectMeta: objMeta("kubernaut-system", "rar-rr-fast-1"),
			Spec: remediationv1.RemediationApprovalRequestSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: "rr-fast-1"},
				Confidence:            0.95,
				ConfidenceLevel:       "high",
				Reason:                "Auto-approved by policy",
				WhyApprovalRequired:   "Policy audit trail",
				InvestigationSummary:  "Quick fix",
				RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{
					WorkflowID: "fast-v1",
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
				DecidedAt: func() *metav1.Time { t := metav1.NewTime(time.Date(2026, 6, 11, 15, 45, 1, 0, time.UTC)); return &t }(),
			},
		}

		reqPayload, err := tools.MarshalApprovalRequestPayload(rar)
		Expect(err).NotTo(HaveOccurred())
		var parsedReq tools.ApprovalRequestEventPayload
		Expect(json.Unmarshal([]byte(reqPayload), &parsedReq)).To(Succeed())
		Expect(parsedReq.Name).To(Equal("rar-rr-fast-1"))

		resPayload, err := tools.MarshalApprovalResolvedPayload(rar)
		Expect(err).NotTo(HaveOccurred())
		var parsedRes tools.ApprovalResolvedEventPayload
		Expect(json.Unmarshal([]byte(resPayload), &parsedRes)).To(Succeed())
		Expect(parsedRes.Decision).To(Equal("Approved"))
		Expect(parsedRes.DecidedBy).To(Equal("system"))
	})

	It("UT-AF-1398-010: realistic RAR payload within 8KB limit", func() {
		rar := &remediationv1.RemediationApprovalRequest{
			ObjectMeta: objMeta("kubernaut-system", "rar-rr-complex-scenario-1"),
			Spec: remediationv1.RemediationApprovalRequestSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: "rr-complex-scenario-1"},
				Confidence:            0.68,
				ConfidenceLevel:       "medium",
				Reason:                "Multiple factors indicate need for human review before proceeding with remediation",
				WhyApprovalRequired: "Production namespace requires approval per policy evaluation; " +
					"confidence below auto-approve threshold; multiple alternative approaches available",
				InvestigationSummary: "Kubernetes deployment nginx-frontend in production namespace " +
					"experienced repeated CrashLoopBackOff events due to misconfigured readiness probe. " +
					"Root cause traced to ConfigMap change deployed outside GitOps pipeline.",
				RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{
					WorkflowID:      "configmap-revert-v2",
					Version:         "2.1.0",
					ExecutionBundle: "oci://registry.kubernaut.ai/workflows/configmap-revert:v2.1.0@sha256:abc123def456",
					Rationale:       "Best match for ConfigMap drift scenario with GitOps restoration",
				},
				EvidenceCollected: []string{
					"CrashLoopBackOff events detected at 2026-06-11T14:30:00Z (5 occurrences in 10 minutes)",
					"ConfigMap production/nginx-config last modified 2026-06-11T14:25:00Z by kubectl",
					"ArgoCD Application out-of-sync since 2026-06-11T14:25:00Z",
					"Git HEAD for nginx-config differs from live state (3 field changes)",
					"No PDB violation detected — safe to restart pods",
				},
				RecommendedActions: []remediationv1.ApprovalRecommendedAction{
					{Action: "Revert ConfigMap to git HEAD state", Rationale: "Restore GitOps consistency; probe config in git is known-good"},
					{Action: "Trigger ArgoCD sync for nginx-frontend Application", Rationale: "Ensure all resources match declared state after ConfigMap fix"},
					{Action: "Verify pod readiness after rollout", Rationale: "Confirm fix resolved CrashLoopBackOff within 60s"},
				},
				AlternativesConsidered: []remediationv1.ApprovalAlternative{
					{Approach: "Update git to match live ConfigMap state", ProsCons: "Preserves the manual change but bypasses code review; risks introducing untested configuration"},
					{Approach: "Scale down deployment and investigate offline", ProsCons: "Zero risk of further damage but causes extended downtime for end users"},
				},
				PolicyEvaluation: &remediationv1.ApprovalPolicyEvaluation{
					PolicyName:   "approval.rego",
					MatchedRules: []string{"production_namespace_always_requires_approval", "confidence_below_threshold"},
					Decision:     "ManualReviewRequired",
				},
				RequiredBy: metav1.NewTime(time.Date(2026, 6, 11, 16, 0, 0, 0, time.UTC)),
			},
			Status: remediationv1.RemediationApprovalRequestStatus{
				TimeRemaining: "14m30s",
			},
		}

		payload, err := tools.MarshalApprovalRequestPayload(rar)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(payload)).To(BeNumerically("<", 8192), "payload must fit within 8KB structured meta limit")
		Expect(len(payload)).To(BeNumerically(">", 500), "realistic payload should be substantial")

		var parsed tools.ApprovalRequestEventPayload
		Expect(json.Unmarshal([]byte(payload), &parsed)).To(Succeed())
		Expect(parsed.PolicyEvaluation).NotTo(BeNil())
		Expect(parsed.PolicyEvaluation.MatchedRules).To(HaveLen(2))
	})
})
