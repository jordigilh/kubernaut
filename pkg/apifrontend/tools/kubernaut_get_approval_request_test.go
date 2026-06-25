package tools_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("kubernaut_get_approval_request", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("UT-AF-109-001: returns full RAR detail by namespace/name", func() {
		tc := newTypedFakeClient(newTypedDetailedRAR("payments", "rar-oom-1"))
		result, err := tools.HandleGetApprovalRequest(ctx, tc, tools.GetApprovalRequestArgs{
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
		tc := newTypedFakeClient(newTypedDetailedRAR("payments", "rar-oom-1"))
		result, err := tools.HandleGetApprovalRequest(ctx, tc, tools.GetApprovalRequestArgs{
			RARID: "payments/rar-oom-1",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Name).To(Equal("rar-oom-1"))
		Expect(result.Namespace).To(Equal("payments"))
	})

	It("UT-AF-109-003: returns evidence and recommended actions", func() {
		tc := newTypedFakeClient(newTypedDetailedRAR("payments", "rar-oom-1"))
		result, err := tools.HandleGetApprovalRequest(ctx, tc, tools.GetApprovalRequestArgs{
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
		tc := newTypedFakeClient()
		_, err := tools.HandleGetApprovalRequest(ctx, tc, tools.GetApprovalRequestArgs{
			Namespace: "payments",
			Name:      "nonexistent",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("UT-AF-109-005: returns user-friendly error on 403", func() {
		tc := newTypedFakeClientWithGetInterceptor(func(ctx context.Context, key crclient.ObjectKey, obj crclient.Object, opts ...crclient.GetOption) error {
			return newForbiddenError("remediationapprovalrequests")
		})
		_, err := tools.HandleGetApprovalRequest(ctx, tc, tools.GetApprovalRequestArgs{
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
		tc := newTypedFakeClient()
		_, err := tools.HandleGetApprovalRequest(ctx, tc, tools.GetApprovalRequestArgs{
			Namespace: "../etc",
			Name:      "rar-1",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid input"))
	})

	It("UT-AF-109-008: bare rar_id without namespace returns not-found (no matching RAR)", func() {
		tc := newTypedFakeClient()
		_, err := tools.HandleGetApprovalRequest(ctx, tc, tools.GetApprovalRequestArgs{
			RARID: "bad-format-no-slash",
		})
		Expect(err).To(HaveOccurred())
	})

	It("UT-AF-109-010: missing namespace and name without rar_id returns error", func() {
		tc := newTypedFakeClient()
		_, err := tools.HandleGetApprovalRequest(ctx, tc, tools.GetApprovalRequestArgs{})
		Expect(err).To(HaveOccurred())
	})

	It("UT-AF-109-009: includes recommended workflow info", func() {
		tc := newTypedFakeClient(newTypedDetailedRAR("payments", "rar-oom-1"))
		result, err := tools.HandleGetApprovalRequest(ctx, tc, tools.GetApprovalRequestArgs{
			Namespace: "payments",
			Name:      "rar-oom-1",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RecommendedWorkflow.Name).To(Equal("oomkill-increase-memory-v1"))
		Expect(result.RecommendedWorkflow.Version).To(Equal("1.0.0"))
	})

	It("UT-AF-1493-005: returns full RAR detail with bare rar_id and injected namespace", func() {
		tc := newTypedFakeClient(newTypedDetailedRAR("payments", "rar-oom-1"))
		result, err := tools.HandleGetApprovalRequest(ctx, tc, tools.GetApprovalRequestArgs{
			RARID:     "rar-oom-1",
			Namespace: "payments",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Name).To(Equal("rar-oom-1"))
		Expect(result.Namespace).To(Equal("payments"))
		Expect(result.RemediationRequest).To(Equal("rr-oom-1"))
	})

	It("IT-AF-1493-001: bare rar_id dispatches through handler to K8s lookup", func() {
		tc := newTypedFakeClient(newTypedDetailedRAR("payments", "rar-oom-1"))
		result, err := tools.HandleGetApprovalRequest(ctx, tc, tools.GetApprovalRequestArgs{
			RARID:     "rar-oom-1",
			Namespace: "payments",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Name).To(Equal("rar-oom-1"))
		Expect(result.Namespace).To(Equal("payments"))
		Expect(result.Confidence).To(BeNumerically("~", 0.72, 0.001))
		Expect(result.EvidenceCollected).To(HaveLen(2))
	})

	Context("ADVERSARIAL tests", func() {
		It("ADV-AF-109-020: RAR with empty spec fields does not panic", func() {
			minimalRAR := &remediationv1.RemediationApprovalRequest{
				ObjectMeta: objMeta("payments", "rar-minimal"),
			}
			tc := newTypedFakeClient(minimalRAR)
			result, err := tools.HandleGetApprovalRequest(ctx, tc, tools.GetApprovalRequestArgs{
				Namespace: "payments",
				Name:      "rar-minimal",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Name).To(Equal("rar-minimal"))
			Expect(result.Confidence).To(Equal(0.0))
			Expect(result.EvidenceCollected).To(BeEmpty())
			Expect(result.RecommendedActions).To(BeEmpty())
			Expect(result.AlternativesConsidered).To(BeEmpty())
			Expect(result.Decision).To(Equal("Pending"))
		})

		It("ADV-AF-109-030: handler accepts cancelled context without panicking", func() {
			cancelledCtx, cancel := context.WithCancel(context.Background())
			cancel()

			tc := newTypedFakeClient(newTypedDetailedRAR("payments", "rar-oom-1"))
			Expect(func() {
				_, _ = tools.HandleGetApprovalRequest(cancelledCtx, tc, tools.GetApprovalRequestArgs{
					Namespace: "payments",
					Name:      "rar-oom-1",
				})
			}).NotTo(Panic())
		})
	})
})

// Test for the HandleGetApprovalRequest 403 error interceptor uses Get, not List
var _ = Describe("kubernaut_get_approval_request 403 via Get interceptor", func() {
	It("UT-AF-109-005b: returns user-friendly error on 403 via Get interceptor", func() {
		tc := newTypedFakeClientWithGetInterceptor(func(ctx context.Context, key crclient.ObjectKey, obj crclient.Object, opts ...crclient.GetOption) error {
			return newForbiddenError("remediationapprovalrequests")
		})
		_, err := tools.HandleGetApprovalRequest(context.Background(), tc, tools.GetApprovalRequestArgs{
			Namespace: "forbidden",
			Name:      "rar-1",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("access denied"))
	})
})

func newTypedFakeClientWithGetInterceptor(getFn func(ctx context.Context, key crclient.ObjectKey, obj crclient.Object, opts ...crclient.GetOption) error) crclient.Client {
	return fake.NewClientBuilder().
		WithScheme(rrTestScheme()).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, client crclient.WithWatch, key crclient.ObjectKey, obj crclient.Object, opts ...crclient.GetOption) error {
				return getFn(ctx, key, obj, opts...)
			},
		}).
		Build()
}
