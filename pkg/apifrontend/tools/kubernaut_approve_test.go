package tools_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

func newTypedRAR() *remediationv1.RemediationApprovalRequest {
	return &remediationv1.RemediationApprovalRequest{
		ObjectMeta: objMeta("payments", "rar-1"),
		Spec: remediationv1.RemediationApprovalRequestSpec{
			RemediationRequestRef: corev1.ObjectReference{Name: "rr-1"},
			RequiredBy:            metav1.NewTime(metav1.Now().Time),
		},
	}
}

var _ = Describe("kubernaut_approve", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("UT-AF-104-001: patches RAR status to Approved", func() {
		tc := newTypedFakeClientWithStatus(newTypedRAR())
		result, err := tools.HandleApprove(ctx, tc, tools.ApproveArgs{
			Namespace: "payments",
			RARName:   "rar-1",
			Decision:  "Approved",
		}, "bob")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("Approved"))
	})

	It("UT-AF-104-002: patches RAR status to Rejected", func() {
		tc := newTypedFakeClientWithStatus(newTypedRAR())
		result, err := tools.HandleApprove(ctx, tc, tools.ApproveArgs{
			Namespace: "payments",
			RARName:   "rar-1",
			Decision:  "Rejected",
			Reason:    "Too risky",
		}, "bob")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("Rejected"))
	})

	It("UT-AF-104-003: sets decidedBy in patch", func() {
		rar := newTypedRAR()
		tc := newTypedFakeClientWithStatus(rar)
		result, err := tools.HandleApprove(ctx, tc, tools.ApproveArgs{
			Namespace: "payments",
			RARName:   "rar-1",
			Decision:  "Approved",
		}, "bob")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Message).To(ContainSubstring("bob"))
	})

	It("UT-AF-104-004: returns error when RAR not found", func() {
		tc := newTypedFakeClient()
		_, err := tools.HandleApprove(ctx, tc, tools.ApproveArgs{
			Namespace: "payments",
			RARName:   "missing",
			Decision:  "Approved",
		}, "bob")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("UT-AF-104-005: supports workflowOverride in approval", func() {
		tc := newTypedFakeClientWithStatus(newTypedRAR())
		result, err := tools.HandleApprove(ctx, tc, tools.ApproveArgs{
			Namespace:        "payments",
			RARName:          "rar-1",
			Decision:         "Approved",
			WorkflowOverride: "fast-restart",
		}, "bob")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("Approved"))
	})

	It("UT-AF-104-006: nil client returns ErrK8sUnavailable", func() {
		_, err := tools.HandleApprove(ctx, nil, tools.ApproveArgs{
			Namespace: "default",
			RARName:   "rar-1",
			Decision:  "Approved",
		}, "bob")
		Expect(err).To(MatchError(tools.ErrK8sUnavailable))
	})

	It("UT-AF-104-007: invalid namespace returns ErrInvalidInput", func() {
		tc := newTypedFakeClient()
		_, err := tools.HandleApprove(ctx, tc, tools.ApproveArgs{
			Namespace: "../etc",
			RARName:   "rar-1",
			Decision:  "Approved",
		}, "bob")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid input"))
	})

	It("UT-AF-104-008: invalid RARName returns ErrInvalidInput", func() {
		tc := newTypedFakeClient()
		_, err := tools.HandleApprove(ctx, tc, tools.ApproveArgs{
			Namespace: "default",
			RARName:   "INVALID NAME!!",
			Decision:  "Approved",
		}, "bob")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid input"))
	})

	It("UT-AF-104-009: empty decision returns ErrInvalidInput", func() {
		tc := newTypedFakeClient()
		_, err := tools.HandleApprove(ctx, tc, tools.ApproveArgs{
			Namespace: "default",
			RARName:   "rar-1",
			Decision:  "",
		}, "bob")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid input"))
	})

	Context("strict decision enum validation (#1353)", func() {
		It("UT-AF-1353-001: rejects verb form 'Approve' with actionable error", func() {
			tc := newTypedFakeClientWithStatus(newTypedRAR())
			_, err := tools.HandleApprove(ctx, tc, tools.ApproveArgs{
				Namespace: "payments",
				RARName:   "rar-1",
				Decision:  "Approve",
			}, "bob")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid input"))
			Expect(err.Error()).To(ContainSubstring("Approved"))
			Expect(err.Error()).To(ContainSubstring("Rejected"))
		})

		It("UT-AF-1353-002: rejects verb form 'Reject' with actionable error", func() {
			tc := newTypedFakeClientWithStatus(newTypedRAR())
			_, err := tools.HandleApprove(ctx, tc, tools.ApproveArgs{
				Namespace: "payments",
				RARName:   "rar-1",
				Decision:  "Reject",
			}, "bob")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid input"))
			Expect(err.Error()).To(ContainSubstring("Approved"))
		})

		It("UT-AF-1353-003: rejects arbitrary string 'maybe'", func() {
			tc := newTypedFakeClientWithStatus(newTypedRAR())
			_, err := tools.HandleApprove(ctx, tc, tools.ApproveArgs{
				Namespace: "payments",
				RARName:   "rar-1",
				Decision:  "maybe",
			}, "bob")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid input"))
		})

		It("UT-AF-1353-004: accepts exact enum value 'Approved'", func() {
			tc := newTypedFakeClientWithStatus(newTypedRAR())
			result, err := tools.HandleApprove(ctx, tc, tools.ApproveArgs{
				Namespace: "payments",
				RARName:   "rar-1",
				Decision:  "Approved",
			}, "bob")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).To(Equal("Approved"))
		})

		It("UT-AF-1353-005: accepts exact enum value 'Rejected'", func() {
			tc := newTypedFakeClientWithStatus(newTypedRAR())
			result, err := tools.HandleApprove(ctx, tc, tools.ApproveArgs{
				Namespace: "payments",
				RARName:   "rar-1",
				Decision:  "Rejected",
			}, "bob")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).To(Equal("Rejected"))
		})
	})
})

func newTypedFakeClientWithStatus(objects ...crclient.Object) crclient.Client {
	return fake.NewClientBuilder().
		WithScheme(rrTestScheme()).
		WithObjects(objects...).
		WithStatusSubresource(objects...).
		Build()
}
