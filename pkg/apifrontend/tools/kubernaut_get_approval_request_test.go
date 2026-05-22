package tools_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

func newDetailedFakeRAR(namespace, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kubernaut.ai/v1alpha1",
			"kind":       "RemediationApprovalRequest",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"remediationRequestRef": map[string]interface{}{
					"name": "rr-oom-1",
				},
				"aiAnalysisRef": map[string]interface{}{
					"name": "aia-oom-1",
				},
				"confidence":      0.72,
				"confidenceLevel": "medium",
				"reason":          "Confidence below auto-approve threshold",
				"recommendedWorkflow": map[string]interface{}{
					"workflowId":      "oomkill-increase-memory-v1",
					"version":         "1.0.0",
					"executionBundle": "oci://registry/oomkill:sha256-abc",
					"rationale":       "Best match for OOMKill scenario",
				},
				"investigationSummary": "Container exceeded memory limits under traffic spike",
				"evidenceCollected": []interface{}{
					"OOMKill event at 2026-01-15T10:30:00Z",
					"Memory usage 98% of limit",
				},
				"recommendedActions": []interface{}{
					map[string]interface{}{
						"action":    "Increase memory limit to 512Mi",
						"rationale": "Current limit 256Mi insufficient for traffic spike",
					},
				},
				"alternativesConsidered": []interface{}{
					map[string]interface{}{
						"approach": "Horizontal scaling",
						"prosCons": "Higher cost but better availability vs single node vertical scaling",
					},
				},
				"whyApprovalRequired": "Confidence 0.72 below auto-approve threshold of 0.80",
				"requiredBy":          "2026-01-15T10:45:00Z",
			},
			"status": map[string]interface{}{
				"decision":      "",
				"timeRemaining": "12m30s",
				"expired":       false,
			},
		},
	}
}

var _ = Describe("kubernaut_get_approval_request", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("UT-AF-109-001: returns full RAR detail by namespace/name", func() {
		client := newDynamicFakeClient(newDetailedFakeRAR("payments", "rar-oom-1"))
		result, err := tools.HandleGetApprovalRequest(ctx, client, tools.GetApprovalRequestArgs{
			Namespace: "payments",
			Name:      "rar-oom-1",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Name).To(Equal("rar-oom-1"))
		Expect(result.Namespace).To(Equal("payments"))
		Expect(result.RemediationRequest).To(Equal("rr-oom-1"))
		Expect(result.Confidence).To(BeNumerically("~", 0.72, 0.001))
		Expect(result.ConfidenceLevel).To(Equal("medium"))
		Expect(result.InvestigationSummary).To(ContainSubstring("memory limits"))
		Expect(result.WhyApprovalRequired).To(ContainSubstring("auto-approve"))
		Expect(result.Decision).To(Equal("Pending"))
		Expect(result.TimeRemaining).To(Equal("12m30s"))
		Expect(result.Expired).To(BeFalse())
	})

	It("UT-AF-109-002: returns full RAR detail by rar_id shorthand", func() {
		client := newDynamicFakeClient(newDetailedFakeRAR("payments", "rar-oom-1"))
		result, err := tools.HandleGetApprovalRequest(ctx, client, tools.GetApprovalRequestArgs{
			RARID: "payments/rar-oom-1",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Name).To(Equal("rar-oom-1"))
		Expect(result.Namespace).To(Equal("payments"))
	})

	It("UT-AF-109-003: returns evidence and recommended actions", func() {
		client := newDynamicFakeClient(newDetailedFakeRAR("payments", "rar-oom-1"))
		result, err := tools.HandleGetApprovalRequest(ctx, client, tools.GetApprovalRequestArgs{
			Namespace: "payments",
			Name:      "rar-oom-1",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.EvidenceCollected).To(HaveLen(2))
		Expect(result.EvidenceCollected[0]).To(ContainSubstring("OOMKill"))
		Expect(result.RecommendedActions).To(HaveLen(1))
		Expect(result.RecommendedActions[0].Action).To(ContainSubstring("512Mi"))
		Expect(result.AlternativesConsidered).To(HaveLen(1))
		Expect(result.AlternativesConsidered[0].Approach).To(Equal("Horizontal scaling"))
		Expect(result.AlternativesConsidered[0].ProsCons).To(ContainSubstring("Higher cost"))
	})

	It("UT-AF-109-004: returns not-found error for missing RAR", func() {
		client := newDynamicFakeClient()
		_, err := tools.HandleGetApprovalRequest(ctx, client, tools.GetApprovalRequestArgs{
			Namespace: "payments",
			Name:      "nonexistent",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("UT-AF-109-005: returns user-friendly error on 403", func() {
		client := newDynamicFakeClient()
		client.PrependReactor("get", "remediationapprovalrequests", func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, newForbiddenError("remediationapprovalrequests")
		})
		_, err := tools.HandleGetApprovalRequest(ctx, client, tools.GetApprovalRequestArgs{
			Namespace: "forbidden",
			Name:      "rar-1",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("access denied"))
	})

	It("UT-AF-109-006: nil client returns ErrK8sUnavailable", func() {
		_, err := tools.HandleGetApprovalRequest(ctx, nil, tools.GetApprovalRequestArgs{
			Namespace: "default",
			Name:      "rar-1",
		})
		Expect(err).To(MatchError(tools.ErrK8sUnavailable))
	})

	It("UT-AF-109-007: invalid namespace returns ErrInvalidInput", func() {
		client := newDynamicFakeClient()
		_, err := tools.HandleGetApprovalRequest(ctx, client, tools.GetApprovalRequestArgs{
			Namespace: "../etc",
			Name:      "rar-1",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid input"))
	})

	It("UT-AF-109-008: invalid rar_id format returns error", func() {
		client := newDynamicFakeClient()
		_, err := tools.HandleGetApprovalRequest(ctx, client, tools.GetApprovalRequestArgs{
			RARID: "bad-format-no-slash",
		})
		Expect(err).To(HaveOccurred())
	})

	It("UT-AF-109-010: missing namespace and name without rar_id returns error", func() {
		client := newDynamicFakeClient()
		_, err := tools.HandleGetApprovalRequest(ctx, client, tools.GetApprovalRequestArgs{})
		Expect(err).To(HaveOccurred())
	})

	It("UT-AF-109-009: includes recommended workflow info", func() {
		client := newDynamicFakeClient(newDetailedFakeRAR("payments", "rar-oom-1"))
		result, err := tools.HandleGetApprovalRequest(ctx, client, tools.GetApprovalRequestArgs{
			Namespace: "payments",
			Name:      "rar-oom-1",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RecommendedWorkflow.Name).To(Equal("oomkill-increase-memory-v1"))
		Expect(result.RecommendedWorkflow.Version).To(Equal("1.0.0"))
	})
})
