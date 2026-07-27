package tools_test

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("ADVERSARIAL: kubernaut_list_approval_requests", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Context("Input validation bypass attempts", func() {
		It("ADV-AF-108-001: namespace with path traversal characters", func() {
			tc := newTypedFakeClient()
			_, err := tools.HandleListApprovalRequests(ctx, tc, tools.ListApprovalRequestsArgs{
				Namespace: "../../kube-system",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid input"))
		})

		It("ADV-AF-108-002: namespace with shell metacharacters", func() {
			tc := newTypedFakeClient()
			_, err := tools.HandleListApprovalRequests(ctx, tc, tools.ListApprovalRequestsArgs{
				Namespace: "payments; rm -rf /",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid input"))
		})

		It("ADV-AF-108-003: namespace exceeding 63 char limit", func() {
			tc := newTypedFakeClient()
			_, err := tools.HandleListApprovalRequests(ctx, tc, tools.ListApprovalRequestsArgs{
				Namespace: strings.Repeat("a", 64),
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid input"))
		})

		It("ADV-AF-108-004: empty namespace string", func() {
			tc := newTypedFakeClient()
			_, err := tools.HandleListApprovalRequests(ctx, tc, tools.ListApprovalRequestsArgs{
				Namespace: "",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid input"))
		})

		It("ADV-AF-108-005: namespace with unicode/null bytes", func() {
			tc := newTypedFakeClient()
			_, err := tools.HandleListApprovalRequests(ctx, tc, tools.ListApprovalRequestsArgs{
				Namespace: "pay\x00ments",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid input"))
		})

		It("ADV-AF-108-006: namespace with uppercase (RFC 1123 violation)", func() {
			tc := newTypedFakeClient()
			_, err := tools.HandleListApprovalRequests(ctx, tc, tools.ListApprovalRequestsArgs{
				Namespace: "PayMents",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid input"))
		})
	})

	Context("Decision filter injection attempts", func() {
		It("ADV-AF-108-010: decision with SQL injection payload", func() {
			tc := newTypedFakeClient(
				newTypedRARWithDecision("rar-1", "rr-1", "", 0.72, "medium"),
			)
			result, err := tools.HandleListApprovalRequests(ctx, tc, tools.ListApprovalRequestsArgs{
				Namespace: "payments",
				Decision:  "'; DROP TABLE approvals; --",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Count).To(Equal(0))
		})

		It("ADV-AF-108-011: decision with extremely long string", func() {
			tc := newTypedFakeClient(
				newTypedRARWithDecision("rar-1", "rr-1", "", 0.72, "medium"),
			)
			result, err := tools.HandleListApprovalRequests(ctx, tc, tools.ListApprovalRequestsArgs{
				Namespace: "payments",
				Decision:  strings.Repeat("A", 10000),
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Count).To(Equal(0))
		})

		It("ADV-AF-108-012: decision filter is case-insensitive", func() {
			tc := newTypedFakeClient(
				newTypedRARWithDecision("rar-1", "rr-1", remediationv1.ApprovalDecisionApproved, 0.72, "medium"),
			)
			result, err := tools.HandleListApprovalRequests(ctx, tc, tools.ListApprovalRequestsArgs{
				Namespace: "payments",
				Decision:  "APPROVED",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Count).To(Equal(1))
		})

		It("ADV-AF-108-013: decision=PENDING with mixed case matches empty decision", func() {
			tc := newTypedFakeClient(
				newTypedRARWithDecision("rar-1", "rr-1", "", 0.72, "medium"),
			)
			result, err := tools.HandleListApprovalRequests(ctx, tc, tools.ListApprovalRequestsArgs{
				Namespace: "payments",
				Decision:  "PENDING",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Count).To(Equal(1))
		})
	})

	Context("Information leakage via errors", func() {
		It("ADV-AF-108-020: 403 error does not expose internal resource paths", func() {
			tc := newTypedFakeClientWithInterceptor(interceptor.Funcs{
				List: func(ctx context.Context, client crclient.WithWatch, list crclient.ObjectList, opts ...crclient.ListOption) error {
					return newForbiddenError("remediationapprovalrequests")
				},
			})
			_, err := tools.HandleListApprovalRequests(ctx, tc, tools.ListApprovalRequestsArgs{Namespace: "secret-ns"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).NotTo(ContainSubstring("secret-ns"))
			Expect(err.Error()).NotTo(ContainSubstring("kubernaut.ai"))
			Expect(err.Error()).NotTo(ContainSubstring("v1alpha1"))
		})
	})

	Context("Resource exhaustion", func() {
		It("ADV-AF-108-030: handles moderate result set without panic", func() {
			objects := make([]crclient.Object, 50)
			for i := range objects {
				name := fmt.Sprintf("rar-%03d", i)
				objects[i] = newTypedRARWithDecision(name, "rr-1", "", 0.5, "low")
			}
			tc := newTypedFakeClient(objects...)
			result, err := tools.HandleListApprovalRequests(ctx, tc, tools.ListApprovalRequestsArgs{Namespace: "payments"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Count).To(Equal(50))
		})
	})
})

var _ = Describe("ADVERSARIAL: kubernaut_get_approval_request", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Context("rar_id parsing exploits", func() {
		It("ADV-AF-109-001: rar_id with multiple slashes is rejected by name validation", func() {
			tc := newTypedFakeClient()
			_, err := tools.HandleGetApprovalRequest(ctx, tc, tools.GetApprovalRequestArgs{
				RARID: "payments/rar/with/slashes",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid input"))
		})

		It("ADV-AF-109-002: rar_id with leading slash", func() {
			tc := newTypedFakeClient()
			_, err := tools.HandleGetApprovalRequest(ctx, tc, tools.GetApprovalRequestArgs{
				RARID: "/rar-1",
			})
			Expect(err).To(HaveOccurred())
		})

		It("ADV-AF-109-003: rar_id with trailing slash", func() {
			tc := newTypedFakeClient()
			_, err := tools.HandleGetApprovalRequest(ctx, tc, tools.GetApprovalRequestArgs{
				RARID: "payments/",
			})
			Expect(err).To(HaveOccurred())
		})

		It("ADV-AF-109-004: rar_id with path traversal in namespace part", func() {
			tc := newTypedFakeClient()
			_, err := tools.HandleGetApprovalRequest(ctx, tc, tools.GetApprovalRequestArgs{
				RARID: "../kube-system/rar-1",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid input"))
		})

		It("ADV-AF-109-005: rar_id namespace part is valid but name has path traversal", func() {
			tc := newTypedFakeClient()
			_, err := tools.HandleGetApprovalRequest(ctx, tc, tools.GetApprovalRequestArgs{
				RARID: "payments/../../etc/passwd",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid input"))
		})

		It("ADV-AF-109-006: simultaneous rar_id and namespace/name - rar_id takes precedence", func() {
			tc := newTypedFakeClient(newTypedDetailedRAR("payments", "rar-oom-1"))
			result, err := tools.HandleGetApprovalRequest(ctx, tc, tools.GetApprovalRequestArgs{
				RARID:     "payments/rar-oom-1",
				Namespace: "other-ns",
				Name:      "other-name",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Namespace).To(Equal("payments"))
			Expect(result.Name).To(Equal("rar-oom-1"))
		})
	})

	Context("Information leakage from error responses", func() {
		It("ADV-AF-109-010: not-found error does not reveal resource type or API group", func() {
			tc := newTypedFakeClient()
			_, err := tools.HandleGetApprovalRequest(ctx, tc, tools.GetApprovalRequestArgs{
				Namespace: "payments",
				Name:      "sensitive-rar-name",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).NotTo(ContainSubstring("sensitive-rar-name"))
			Expect(err.Error()).NotTo(ContainSubstring("kubernaut.ai"))
			Expect(err.Error()).NotTo(ContainSubstring("remediationapprovalrequests"))
		})

		It("ADV-AF-109-011: 403 error does not expose user identity or groups", func() {
			tc := newTypedFakeClientWithGetInterceptor(func(ctx context.Context, key crclient.ObjectKey, obj crclient.Object, opts ...crclient.GetOption) error {
				return newForbiddenError("remediationapprovalrequests")
			})
			_, err := tools.HandleGetApprovalRequest(ctx, tc, tools.GetApprovalRequestArgs{
				Namespace: "payments",
				Name:      "rar-1",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("access denied"))
			Expect(err.Error()).NotTo(ContainSubstring("payments"))
			Expect(err.Error()).NotTo(ContainSubstring("rar-1"))
			Expect(err.Error()).NotTo(ContainSubstring("v1alpha1"))
		})
	})
})
