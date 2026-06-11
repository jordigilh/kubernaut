package tools_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("Approval Event Payload Marshaling — TP-1398", func() {

	It("UT-AF-1398-001: MarshalApprovalRequestPayload produces valid JSON with all spec fields", func() {
		rar := newDetailedFakeRAR("kubernaut-system", "rar-rr-oom-1")

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
		rar := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "kubernaut.ai/v1alpha1",
				"kind":       "RemediationApprovalRequest",
				"metadata": map[string]interface{}{
					"name":      "rar-rr-gitops-drift-1",
					"namespace": "kubernaut-system",
				},
				"spec": map[string]interface{}{
					"remediationRequestRef": map[string]interface{}{
						"name": "rr-gitops-drift-1",
					},
					"confidence":           0.65,
					"confidenceLevel":      "medium",
					"reason":               "Policy requires approval",
					"whyApprovalRequired":  "Production namespace",
					"investigationSummary": "Drift detected",
					"recommendedWorkflow": map[string]interface{}{
						"workflowId": "drift-v1",
						"version":    "1.0.0",
						"rationale":  "Best match",
					},
					"recommendedActions": []interface{}{
						map[string]interface{}{"action": "Revert", "rationale": "Restore consistency"},
					},
					"requiredBy": "2026-06-11T16:00:00Z",
				},
				"status": map[string]interface{}{},
			},
		}

		payload, err := tools.MarshalApprovalRequestPayload(rar)
		Expect(err).NotTo(HaveOccurred())

		var parsed tools.ApprovalRequestEventPayload
		Expect(json.Unmarshal([]byte(payload), &parsed)).To(Succeed())
		Expect(parsed.RemediationRequestName).To(Equal("rr-gitops-drift-1"))
	})

	It("UT-AF-1398-003: MarshalApprovalResolvedPayload includes all decision fields + workflowOverride", func() {
		rar := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "kubernaut.ai/v1alpha1",
				"kind":       "RemediationApprovalRequest",
				"metadata": map[string]interface{}{
					"name":      "rar-rr-oom-1",
					"namespace": "kubernaut-system",
				},
				"status": map[string]interface{}{
					"decision":        "Approved",
					"decidedBy":       "jane@acme.com",
					"decidedAt":       "2026-06-11T15:50:00Z",
					"decisionMessage": "Reviewed evidence, proceeding with revert",
					"workflowOverride": map[string]interface{}{
						"workflowName": "gitops-manual-sync-v1",
						"parameters": map[string]interface{}{
							"SYNC_PRUNE": "true",
						},
						"rationale": "Prefer sync over revert",
					},
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
		rar := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "kubernaut.ai/v1alpha1",
				"kind":       "RemediationApprovalRequest",
				"metadata": map[string]interface{}{
					"name":      "rar-rr-oom-1",
					"namespace": "kubernaut-system",
				},
				"status": map[string]interface{}{
					"decision":  "Rejected",
					"decidedBy": "bob@acme.com",
					"decidedAt": "2026-06-11T15:55:00Z",
				},
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
		rar := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "kubernaut.ai/v1alpha1",
				"kind":       "RemediationApprovalRequest",
				"metadata": map[string]interface{}{
					"name":      "rar-rr-minimal-1",
					"namespace": "kubernaut-system",
				},
				"spec": map[string]interface{}{
					"remediationRequestRef": map[string]interface{}{
						"name": "rr-minimal-1",
					},
					"confidence":           0.55,
					"confidenceLevel":      "low",
					"reason":               "Low confidence requires approval",
					"whyApprovalRequired":  "Below threshold",
					"investigationSummary": "Basic investigation",
					"recommendedWorkflow": map[string]interface{}{
						"workflowId": "basic-v1",
						"version":    "1.0.0",
						"rationale":  "Only option",
					},
					"recommendedActions": []interface{}{
						map[string]interface{}{"action": "Fix", "rationale": "Needed"},
					},
					"requiredBy": "2026-06-11T16:00:00Z",
					// No policyEvaluation, no evidenceCollected, no alternativesConsidered
				},
				"status": map[string]interface{}{},
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
		rar := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "kubernaut.ai/v1alpha1",
				"kind":       "RemediationApprovalRequest",
				"metadata": map[string]interface{}{
					"name":      "rar-rr-fast-1",
					"namespace": "kubernaut-system",
				},
				"spec": map[string]interface{}{
					"remediationRequestRef": map[string]interface{}{
						"name": "rr-fast-1",
					},
					"confidence":           0.95,
					"confidenceLevel":      "high",
					"reason":               "Auto-approved by policy",
					"whyApprovalRequired":  "Policy audit trail",
					"investigationSummary": "Quick fix",
					"recommendedWorkflow": map[string]interface{}{
						"workflowId": "fast-v1",
						"version":    "1.0.0",
						"rationale":  "Auto-selected",
					},
					"recommendedActions": []interface{}{
						map[string]interface{}{"action": "Apply", "rationale": "Safe"},
					},
					"requiredBy": "2026-06-11T16:00:00Z",
				},
				"status": map[string]interface{}{
					"decision":  "Approved",
					"decidedBy": "system",
					"decidedAt": "2026-06-11T15:45:01Z",
				},
			},
		}

		// Request payload still works even when decision exists
		reqPayload, err := tools.MarshalApprovalRequestPayload(rar)
		Expect(err).NotTo(HaveOccurred())
		var parsedReq tools.ApprovalRequestEventPayload
		Expect(json.Unmarshal([]byte(reqPayload), &parsedReq)).To(Succeed())
		Expect(parsedReq.Name).To(Equal("rar-rr-fast-1"))

		// Resolution payload also works
		resPayload, err := tools.MarshalApprovalResolvedPayload(rar)
		Expect(err).NotTo(HaveOccurred())
		var parsedRes tools.ApprovalResolvedEventPayload
		Expect(json.Unmarshal([]byte(resPayload), &parsedRes)).To(Succeed())
		Expect(parsedRes.Decision).To(Equal("Approved"))
		Expect(parsedRes.DecidedBy).To(Equal("system"))
	})

	It("UT-AF-1398-010: realistic RAR payload within 8KB limit", func() {
		rar := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "kubernaut.ai/v1alpha1",
				"kind":       "RemediationApprovalRequest",
				"metadata": map[string]interface{}{
					"name":      "rar-rr-complex-scenario-1",
					"namespace": "kubernaut-system",
				},
				"spec": map[string]interface{}{
					"remediationRequestRef": map[string]interface{}{
						"name": "rr-complex-scenario-1",
					},
					"confidence":      0.68,
					"confidenceLevel": "medium",
					"reason":          "Multiple factors indicate need for human review before proceeding with remediation",
					"whyApprovalRequired": "Production namespace requires approval per policy evaluation; " +
						"confidence below auto-approve threshold; multiple alternative approaches available",
					"investigationSummary": "Kubernetes deployment nginx-frontend in production namespace " +
						"experienced repeated CrashLoopBackOff events due to misconfigured readiness probe. " +
						"Root cause traced to ConfigMap change deployed outside GitOps pipeline.",
					"recommendedWorkflow": map[string]interface{}{
						"workflowId":      "configmap-revert-v2",
						"version":         "2.1.0",
						"executionBundle": "oci://registry.kubernaut.ai/workflows/configmap-revert:v2.1.0@sha256:abc123def456",
						"rationale":       "Best match for ConfigMap drift scenario with GitOps restoration",
					},
					"evidenceCollected": []interface{}{
						"CrashLoopBackOff events detected at 2026-06-11T14:30:00Z (5 occurrences in 10 minutes)",
						"ConfigMap production/nginx-config last modified 2026-06-11T14:25:00Z by kubectl",
						"ArgoCD Application out-of-sync since 2026-06-11T14:25:00Z",
						"Git HEAD for nginx-config differs from live state (3 field changes)",
						"No PDB violation detected — safe to restart pods",
					},
					"recommendedActions": []interface{}{
						map[string]interface{}{
							"action":    "Revert ConfigMap to git HEAD state",
							"rationale": "Restore GitOps consistency; probe config in git is known-good",
						},
						map[string]interface{}{
							"action":    "Trigger ArgoCD sync for nginx-frontend Application",
							"rationale": "Ensure all resources match declared state after ConfigMap fix",
						},
						map[string]interface{}{
							"action":    "Verify pod readiness after rollout",
							"rationale": "Confirm fix resolved CrashLoopBackOff within 60s",
						},
					},
					"alternativesConsidered": []interface{}{
						map[string]interface{}{
							"approach": "Update git to match live ConfigMap state",
							"prosCons": "Preserves the manual change but bypasses code review; risks introducing untested configuration",
						},
						map[string]interface{}{
							"approach": "Scale down deployment and investigate offline",
							"prosCons": "Zero risk of further damage but causes extended downtime for end users",
						},
					},
					"policyEvaluation": map[string]interface{}{
						"policyName":   "approval.rego",
						"matchedRules": []interface{}{"production_namespace_always_requires_approval", "confidence_below_threshold"},
						"decision":     "ManualReviewRequired",
					},
					"requiredBy": "2026-06-11T16:00:00Z",
				},
				"status": map[string]interface{}{
					"decision":      "",
					"timeRemaining": "14m30s",
					"expired":       false,
				},
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
