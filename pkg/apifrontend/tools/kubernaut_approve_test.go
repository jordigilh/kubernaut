package tools_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

func newTypedRAR() *remediationv1.RemediationApprovalRequest {
	return &remediationv1.RemediationApprovalRequest{
		ObjectMeta: objMeta("payments", "rar-1"),
		Spec: remediationv1.RemediationApprovalRequestSpec{
			RemediationRequestRef: corev1.ObjectReference{Name: "rr-1"},
			RequiredBy:            metav1.NewTime(time.Now().Add(15 * time.Minute)),
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

	It("IT-AF-2564-006: same terminal tuple is an idempotent no-op", func() {
		decidedAt := metav1.NewTime(time.Now().Add(-1 * time.Minute))
		rar := newTypedRAR()
		rar.Status = remediationv1.RemediationApprovalRequestStatus{
			Decision:        remediationv1.ApprovalDecisionApproved,
			DecidedBy:       "bob",
			DecidedAt:       &decidedAt,
			DecisionMessage: "approved",
		}
		tc := newTypedFakeClientWithStatus(rar)

		result, err := tools.HandleApprove(ctx, tc, tools.ApproveArgs{
			Namespace: "payments",
			RARName:   "rar-1",
			Decision:  "Approved",
			Reason:    "approved",
		}, "bob")

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("Approved"))
	})

	It("IT-AF-2564-006: opposite terminal decision is rejected without mutation", func() {
		decidedAt := metav1.NewTime(time.Now().Add(-1 * time.Minute))
		rar := newTypedRAR()
		rar.Status = remediationv1.RemediationApprovalRequestStatus{
			Decision:  remediationv1.ApprovalDecisionApproved,
			DecidedBy: "alice",
			DecidedAt: &decidedAt,
		}
		tc := newTypedFakeClientWithStatus(rar)

		_, err := tools.HandleApprove(ctx, tc, tools.ApproveArgs{
			Namespace: "payments",
			RARName:   "rar-1",
			Decision:  "Rejected",
		}, "bob")

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("terminal"))
		var current remediationv1.RemediationApprovalRequest
		Expect(tc.Get(ctx, crclient.ObjectKey{Namespace: "payments", Name: "rar-1"}, &current)).To(Succeed())
		Expect(current.Status.Decision).To(Equal(remediationv1.ApprovalDecisionApproved))
	})

	It("IT-AF-2564-006: omitted reason does not match a stored terminal reason", func() {
		decidedAt := metav1.NewTime(time.Now().Add(-1 * time.Minute))
		rar := newTypedRAR()
		rar.Status = remediationv1.RemediationApprovalRequestStatus{
			Decision:        remediationv1.ApprovalDecisionApproved,
			DecidedBy:       "bob",
			DecidedAt:       &decidedAt,
			DecisionMessage: "stored reason",
		}
		tc := newTypedFakeClientWithStatus(rar)

		_, err := tools.HandleApprove(ctx, tc, tools.ApproveArgs{
			Namespace: "payments",
			RARName:   "rar-1",
			Decision:  "Approved",
		}, "bob")

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("terminal"))
	})

	It("IT-AF-2564-006: omitted override does not match a stored override", func() {
		decidedAt := metav1.NewTime(time.Now().Add(-1 * time.Minute))
		rar := newTypedRAR()
		rar.Status = remediationv1.RemediationApprovalRequestStatus{
			Decision:  remediationv1.ApprovalDecisionApproved,
			DecidedBy: "bob",
			DecidedAt: &decidedAt,
			WorkflowOverride: &remediationv1.WorkflowOverride{
				WorkflowName: "stored-workflow",
				Rationale:    "User override via kubernaut_approve",
			},
		}
		tc := newTypedFakeClientWithStatus(rar)

		_, err := tools.HandleApprove(ctx, tc, tools.ApproveArgs{
			Namespace: "payments",
			RARName:   "rar-1",
			Decision:  "Approved",
		}, "bob")

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("terminal"))
	})

	It("IT-AF-2564-010: ambiguous committed status write is proven by one readback", func() {
		rar := newTypedRAR()
		rar.UID = "rar-original"
		var statusWrites int
		tc := fake.NewClientBuilder().
			WithScheme(rrTestScheme()).
			WithObjects(rar).
			WithStatusSubresource(&remediationv1.RemediationApprovalRequest{}).
			WithInterceptorFuncs(interceptor.Funcs{
				SubResourceUpdate: func(ctx context.Context, underlying crclient.Client, subresource string, obj crclient.Object, _ ...crclient.SubResourceUpdateOption) error {
					statusWrites++
					Expect(subresource).To(Equal("status"))
					Expect(underlying.SubResource(subresource).Update(ctx, obj)).To(Succeed())
					return errors.New("transport interrupted after commit")
				},
			}).
			Build()

		result, err := tools.HandleApprove(ctx, tc, tools.ApproveArgs{
			Namespace: "payments",
			RARName:   "rar-1",
			Decision:  "Approved",
		}, "bob")

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("Approved"))
		Expect(statusWrites).To(Equal(1))
	})

	It("IT-AF-2564-010: ambiguous readback for a replacement UID is unknown", func() {
		rar := newTypedRAR()
		rar.UID = "rar-original"
		replacement := rar.DeepCopy()
		replacement.UID = "rar-replacement"
		replacement.Status.Decision = remediationv1.ApprovalDecisionRejected
		replacement.Status.DecidedBy = "alice"
		getCount := 0
		tc := fake.NewClientBuilder().
			WithScheme(rrTestScheme()).
			WithObjects(rar).
			WithStatusSubresource(&remediationv1.RemediationApprovalRequest{}).
			WithInterceptorFuncs(interceptor.Funcs{
				Get: func(ctx context.Context, underlying crclient.WithWatch, key crclient.ObjectKey, obj crclient.Object, opts ...crclient.GetOption) error {
					getCount++
					if getCount == 1 {
						return underlying.Get(ctx, key, obj, opts...)
					}
					replacementObject := obj.(*remediationv1.RemediationApprovalRequest)
					replacementObject.TypeMeta = replacement.TypeMeta
					replacementObject.ObjectMeta = *replacement.ObjectMeta.DeepCopy()
					replacementObject.Status = *replacement.Status.DeepCopy()
					return nil
				},
				SubResourceUpdate: func(_ context.Context, _ crclient.Client, _ string, _ crclient.Object, _ ...crclient.SubResourceUpdateOption) error {
					return errors.New("transport outcome unknown")
				},
			}).
			Build()

		_, err := tools.HandleApprove(ctx, tc, tools.ApproveArgs{
			Namespace: "payments",
			RARName:   "rar-1",
			Decision:  "Approved",
		}, "bob")

		Expect(err).To(HaveOccurred())
		var writeErr *tools.ApprovalDecisionWriteError
		Expect(errors.As(err, &writeErr)).To(BeTrue())
		Expect(writeErr.Outcome).To(Equal("unknown"))
		Expect(getCount).To(Equal(2))
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
