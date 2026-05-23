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

func newFakeRARWithDecision(namespace, name, rrName, decision string, confidence float64, confidenceLevel string) *unstructured.Unstructured {
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
					"name": rrName,
				},
				"confidence":      confidence,
				"confidenceLevel": confidenceLevel,
				"reason":          "Confidence below auto-approve threshold",
			},
			"status": map[string]interface{}{
				"decision":      decision,
				"timeRemaining": "4m30s",
			},
		},
	}
}

var _ = Describe("kubernaut_list_approval_requests", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("UT-AF-108-001: lists all RARs in namespace", func() {
		client := newDynamicFakeClient(
			newFakeRARWithDecision("payments", "rar-1", "rr-1", "", 0.72, "medium"),
			newFakeRARWithDecision("payments", "rar-2", "rr-2", "Approved", 0.65, "low"),
		)
		result, err := tools.HandleListApprovalRequests(ctx, client, tools.ListApprovalRequestsArgs{Namespace: "payments"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(2))
		Expect(result.ApprovalRequests).To(HaveLen(2))
	})

	It("UT-AF-108-002: filters by decision=pending (empty string)", func() {
		client := newDynamicFakeClient(
			newFakeRARWithDecision("payments", "rar-1", "rr-1", "", 0.72, "medium"),
			newFakeRARWithDecision("payments", "rar-2", "rr-2", "Approved", 0.65, "low"),
		)
		result, err := tools.HandleListApprovalRequests(ctx, client, tools.ListApprovalRequestsArgs{
			Namespace: "payments",
			Decision:  "pending",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(1))
		Expect(result.ApprovalRequests[0].Name).To(Equal("rar-1"))
		Expect(result.ApprovalRequests[0].Decision).To(Equal("Pending"))
	})

	It("UT-AF-108-003: filters by decision=approved", func() {
		client := newDynamicFakeClient(
			newFakeRARWithDecision("payments", "rar-1", "rr-1", "", 0.72, "medium"),
			newFakeRARWithDecision("payments", "rar-2", "rr-2", "Approved", 0.65, "low"),
		)
		result, err := tools.HandleListApprovalRequests(ctx, client, tools.ListApprovalRequestsArgs{
			Namespace: "payments",
			Decision:  "approved",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(1))
		Expect(result.ApprovalRequests[0].Name).To(Equal("rar-2"))
	})

	It("UT-AF-108-004: returns empty list when no RARs match", func() {
		client := newDynamicFakeClient()
		result, err := tools.HandleListApprovalRequests(ctx, client, tools.ListApprovalRequestsArgs{Namespace: "empty"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(0))
		Expect(result.ApprovalRequests).To(BeEmpty())
	})

	It("UT-AF-108-005: returns user-friendly error on 403", func() {
		client := newDynamicFakeClient()
		client.PrependReactor("list", "remediationapprovalrequests", func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, newForbiddenError("remediationapprovalrequests")
		})
		_, err := tools.HandleListApprovalRequests(ctx, client, tools.ListApprovalRequestsArgs{Namespace: "forbidden"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("access denied"))
	})

	It("UT-AF-108-006: nil client returns ErrK8sUnavailable", func() {
		_, err := tools.HandleListApprovalRequests(ctx, nil, tools.ListApprovalRequestsArgs{Namespace: "default"})
		Expect(err).To(MatchError(tools.ErrK8sUnavailable))
	})

	It("UT-AF-108-007: invalid namespace returns ErrInvalidInput", func() {
		client := newDynamicFakeClient()
		_, err := tools.HandleListApprovalRequests(ctx, client, tools.ListApprovalRequestsArgs{Namespace: "../etc"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid input"))
	})

	It("UT-AF-108-008: summary includes expected fields", func() {
		client := newDynamicFakeClient(
			newFakeRARWithDecision("payments", "rar-1", "rr-1", "", 0.72, "medium"),
		)
		result, err := tools.HandleListApprovalRequests(ctx, client, tools.ListApprovalRequestsArgs{Namespace: "payments"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(1))
		summary := result.ApprovalRequests[0]
		Expect(summary.Name).To(Equal("rar-1"))
		Expect(summary.Namespace).To(Equal("payments"))
		Expect(summary.RemediationRequest).To(Equal("rr-1"))
		Expect(summary.Confidence).To(BeNumerically("~", 0.72, 0.001))
		Expect(summary.ConfidenceLevel).To(Equal("medium"))
		Expect(summary.Decision).To(Equal("Pending"))
		Expect(summary.TimeRemaining).To(Equal("4m30s"))
	})
})
