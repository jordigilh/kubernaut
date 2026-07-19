package tools_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

// newTypedRARWithDecision builds a RemediationApprovalRequest in the fixed
// "payments" namespace (the only namespace used across all kubernaut_list_approval_requests tests).
func newTypedRARWithDecision(name, rrName string, decision remediationv1.ApprovalDecision, confidence float64, confidenceLevel string) *remediationv1.RemediationApprovalRequest {
	return &remediationv1.RemediationApprovalRequest{
		ObjectMeta: objMeta("payments", name),
		Spec: remediationv1.RemediationApprovalRequestSpec{
			RemediationRequestRef: corev1.ObjectReference{Name: rrName},
			Confidence:            confidence,
			ConfidenceLevel:       confidenceLevel,
			Reason:                "Confidence below auto-approve threshold",
			RequiredBy:            metav1.NewTime(metav1.Now().Time),
		},
		Status: remediationv1.RemediationApprovalRequestStatus{
			Decision:      decision,
			TimeRemaining: "4m30s",
		},
	}
}

var _ = Describe("kubernaut_list_approval_requests", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("UT-AF-108-001: lists all RARs in namespace", func() {
		tc := newTypedFakeClient(
			newTypedRARWithDecision("rar-1", "rr-1", "", 0.72, "medium"),
			newTypedRARWithDecision("rar-2", "rr-2", remediationv1.ApprovalDecisionApproved, 0.65, "low"),
		)
		result, err := tools.HandleListApprovalRequests(ctx, tc, tools.ListApprovalRequestsArgs{Namespace: "payments"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(2))
		Expect(result.ApprovalRequests).To(HaveLen(2))
	})

	It("UT-AF-108-002: filters by decision=pending (empty string)", func() {
		tc := newTypedFakeClient(
			newTypedRARWithDecision("rar-1", "rr-1", "", 0.72, "medium"),
			newTypedRARWithDecision("rar-2", "rr-2", remediationv1.ApprovalDecisionApproved, 0.65, "low"),
		)
		result, err := tools.HandleListApprovalRequests(ctx, tc, tools.ListApprovalRequestsArgs{
			Namespace: "payments",
			Decision:  "pending",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(1))
		Expect(result.ApprovalRequests[0].Name).To(Equal("rar-1"))
		Expect(result.ApprovalRequests[0].Decision).To(Equal("Pending"))
	})

	It("UT-AF-108-003: filters by decision=approved", func() {
		tc := newTypedFakeClient(
			newTypedRARWithDecision("rar-1", "rr-1", "", 0.72, "medium"),
			newTypedRARWithDecision("rar-2", "rr-2", remediationv1.ApprovalDecisionApproved, 0.65, "low"),
		)
		result, err := tools.HandleListApprovalRequests(ctx, tc, tools.ListApprovalRequestsArgs{
			Namespace: "payments",
			Decision:  "approved",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(1))
		Expect(result.ApprovalRequests[0].Name).To(Equal("rar-2"))
	})

	It("UT-AF-108-004: returns empty list when no RARs match", func() {
		tc := newTypedFakeClient()
		result, err := tools.HandleListApprovalRequests(ctx, tc, tools.ListApprovalRequestsArgs{Namespace: "empty"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(0))
		Expect(result.ApprovalRequests).To(BeEmpty())
	})

	It("UT-AF-108-005: returns user-friendly error on 403", func() {
		tc := newTypedFakeClientWithInterceptor(interceptor.Funcs{
			List: func(ctx context.Context, client crclient.WithWatch, list crclient.ObjectList, opts ...crclient.ListOption) error {
				return newForbiddenError("remediationapprovalrequests")
			},
		})
		_, err := tools.HandleListApprovalRequests(ctx, tc, tools.ListApprovalRequestsArgs{Namespace: "forbidden"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("access denied"))
	})

	It("UT-AF-108-006: nil client returns ErrK8sUnavailable", func() {
		_, err := tools.HandleListApprovalRequests(ctx, nil, tools.ListApprovalRequestsArgs{Namespace: "default"})
		Expect(err).To(MatchError(tools.ErrK8sUnavailable))
	})

	It("UT-AF-108-007: invalid namespace returns ErrInvalidInput", func() {
		tc := newTypedFakeClient()
		_, err := tools.HandleListApprovalRequests(ctx, tc, tools.ListApprovalRequestsArgs{Namespace: "../etc"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid input"))
	})

	It("UT-AF-108-008: summary includes expected fields", func() {
		tc := newTypedFakeClient(
			newTypedRARWithDecision("rar-1", "rr-1", "", 0.72, "medium"),
		)
		result, err := tools.HandleListApprovalRequests(ctx, tc, tools.ListApprovalRequestsArgs{Namespace: "payments"})
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
