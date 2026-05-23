package tools_test

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("ADVERSARIAL: kubernaut_list_approval_requests", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Context("Input validation bypass attempts", func() {
		It("ADV-AF-108-001: namespace with path traversal characters", func() {
			client := newDynamicFakeClient()
			_, err := tools.HandleListApprovalRequests(ctx, client, tools.ListApprovalRequestsArgs{
				Namespace: "../../kube-system",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid input"))
		})

		It("ADV-AF-108-002: namespace with shell metacharacters", func() {
			client := newDynamicFakeClient()
			_, err := tools.HandleListApprovalRequests(ctx, client, tools.ListApprovalRequestsArgs{
				Namespace: "payments; rm -rf /",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid input"))
		})

		It("ADV-AF-108-003: namespace exceeding 63 char limit", func() {
			client := newDynamicFakeClient()
			_, err := tools.HandleListApprovalRequests(ctx, client, tools.ListApprovalRequestsArgs{
				Namespace: strings.Repeat("a", 64),
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid input"))
		})

		It("ADV-AF-108-004: empty namespace string", func() {
			client := newDynamicFakeClient()
			_, err := tools.HandleListApprovalRequests(ctx, client, tools.ListApprovalRequestsArgs{
				Namespace: "",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid input"))
		})

		It("ADV-AF-108-005: namespace with unicode/null bytes", func() {
			client := newDynamicFakeClient()
			_, err := tools.HandleListApprovalRequests(ctx, client, tools.ListApprovalRequestsArgs{
				Namespace: "pay\x00ments",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid input"))
		})

		It("ADV-AF-108-006: namespace with uppercase (RFC 1123 violation)", func() {
			client := newDynamicFakeClient()
			_, err := tools.HandleListApprovalRequests(ctx, client, tools.ListApprovalRequestsArgs{
				Namespace: "PayMents",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid input"))
		})
	})

	Context("Decision filter injection attempts", func() {
		It("ADV-AF-108-010: decision with SQL injection payload", func() {
			client := newDynamicFakeClient(
				newFakeRARWithDecision("payments", "rar-1", "rr-1", "", 0.72, "medium"),
			)
			result, err := tools.HandleListApprovalRequests(ctx, client, tools.ListApprovalRequestsArgs{
				Namespace: "payments",
				Decision:  "'; DROP TABLE approvals; --",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Count).To(Equal(0))
		})

		It("ADV-AF-108-011: decision with extremely long string", func() {
			client := newDynamicFakeClient(
				newFakeRARWithDecision("payments", "rar-1", "rr-1", "", 0.72, "medium"),
			)
			result, err := tools.HandleListApprovalRequests(ctx, client, tools.ListApprovalRequestsArgs{
				Namespace: "payments",
				Decision:  strings.Repeat("A", 10000),
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Count).To(Equal(0))
		})

		It("ADV-AF-108-012: decision filter is case-insensitive", func() {
			client := newDynamicFakeClient(
				newFakeRARWithDecision("payments", "rar-1", "rr-1", "Approved", 0.72, "medium"),
			)
			result, err := tools.HandleListApprovalRequests(ctx, client, tools.ListApprovalRequestsArgs{
				Namespace: "payments",
				Decision:  "APPROVED",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Count).To(Equal(1))
		})

		It("ADV-AF-108-013: decision=PENDING with mixed case matches empty decision", func() {
			client := newDynamicFakeClient(
				newFakeRARWithDecision("payments", "rar-1", "rr-1", "", 0.72, "medium"),
			)
			result, err := tools.HandleListApprovalRequests(ctx, client, tools.ListApprovalRequestsArgs{
				Namespace: "payments",
				Decision:  "PENDING",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Count).To(Equal(1))
		})
	})

	Context("Information leakage via errors", func() {
		It("ADV-AF-108-020: 403 error does not expose internal resource paths", func() {
			client := newDynamicFakeClient()
			client.PrependReactor("list", "remediationapprovalrequests", func(action k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, newForbiddenError("remediationapprovalrequests")
			})
			_, err := tools.HandleListApprovalRequests(ctx, client, tools.ListApprovalRequestsArgs{Namespace: "secret-ns"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).NotTo(ContainSubstring("secret-ns"))
			Expect(err.Error()).NotTo(ContainSubstring("kubernaut.ai"))
			Expect(err.Error()).NotTo(ContainSubstring("v1alpha1"))
		})
	})

	Context("Resource exhaustion", func() {
		It("ADV-AF-108-030: handles moderate result set without panic", func() {
			objects := make([]runtime.Object, 50)
			for i := range objects {
				name := fmt.Sprintf("rar-%03d", i)
				objects[i] = newFakeRARWithDecision("payments", name, "rr-1", "", 0.5, "low")
			}
			client := newDynamicFakeClient(objects...)
			result, err := tools.HandleListApprovalRequests(ctx, client, tools.ListApprovalRequestsArgs{Namespace: "payments"})
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
			client := newDynamicFakeClient()
			_, err := tools.HandleGetApprovalRequest(ctx, client, tools.GetApprovalRequestArgs{
				RARID: "payments/rar/with/slashes",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid input"))
		})

		It("ADV-AF-109-002: rar_id with leading slash", func() {
			client := newDynamicFakeClient()
			_, err := tools.HandleGetApprovalRequest(ctx, client, tools.GetApprovalRequestArgs{
				RARID: "/rar-1",
			})
			Expect(err).To(HaveOccurred())
		})

		It("ADV-AF-109-003: rar_id with trailing slash", func() {
			client := newDynamicFakeClient()
			_, err := tools.HandleGetApprovalRequest(ctx, client, tools.GetApprovalRequestArgs{
				RARID: "payments/",
			})
			Expect(err).To(HaveOccurred())
		})

		It("ADV-AF-109-004: rar_id with path traversal in namespace part", func() {
			client := newDynamicFakeClient()
			_, err := tools.HandleGetApprovalRequest(ctx, client, tools.GetApprovalRequestArgs{
				RARID: "../kube-system/rar-1",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid input"))
		})

		It("ADV-AF-109-005: rar_id namespace part is valid but name has path traversal", func() {
			client := newDynamicFakeClient()
			_, err := tools.HandleGetApprovalRequest(ctx, client, tools.GetApprovalRequestArgs{
				RARID: "payments/../../etc/passwd",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid input"))
		})

		It("ADV-AF-109-006: simultaneous rar_id and namespace/name - rar_id takes precedence", func() {
			client := newDynamicFakeClient(newDetailedFakeRAR("payments", "rar-oom-1"))
			result, err := tools.HandleGetApprovalRequest(ctx, client, tools.GetApprovalRequestArgs{
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
			client := newDynamicFakeClient()
			_, err := tools.HandleGetApprovalRequest(ctx, client, tools.GetApprovalRequestArgs{
				Namespace: "payments",
				Name:      "sensitive-rar-name",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).NotTo(ContainSubstring("sensitive-rar-name"))
			Expect(err.Error()).NotTo(ContainSubstring("kubernaut.ai"))
			Expect(err.Error()).NotTo(ContainSubstring("remediationapprovalrequests"))
		})

		It("ADV-AF-109-011: 403 error does not expose user identity or groups", func() {
			client := newDynamicFakeClient()
			client.PrependReactor("get", "remediationapprovalrequests", func(action k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, newForbiddenError("remediationapprovalrequests")
			})
			_, err := tools.HandleGetApprovalRequest(ctx, client, tools.GetApprovalRequestArgs{
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

	Context("Malformed CRD objects", func() {
		It("ADV-AF-109-020: RAR with nil spec fields does not panic", func() {
			minimalRAR := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "kubernaut.ai/v1alpha1",
					"kind":       "RemediationApprovalRequest",
					"metadata": map[string]interface{}{
						"name":      "rar-minimal",
						"namespace": "payments",
					},
				},
			}
			client := newDynamicFakeClient(minimalRAR)
			result, err := tools.HandleGetApprovalRequest(ctx, client, tools.GetApprovalRequestArgs{
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

		It("ADV-AF-109-021: RAR with missing optional fields does not panic", func() {
			sparseRAR := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "kubernaut.ai/v1alpha1",
					"kind":       "RemediationApprovalRequest",
					"metadata": map[string]interface{}{
						"name":      "rar-sparse",
						"namespace": "payments",
					},
					"spec": map[string]interface{}{
						"confidence":            0.0,
						"remediationRequestRef": map[string]interface{}{"name": "rr-1"},
					},
					"status": map[string]interface{}{},
				},
			}
			client := newDynamicFakeClient(sparseRAR)
			result, err := tools.HandleGetApprovalRequest(ctx, client, tools.GetApprovalRequestArgs{
				Namespace: "payments",
				Name:      "rar-sparse",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Confidence).To(Equal(0.0))
			Expect(result.Expired).To(BeFalse())
			Expect(result.EvidenceCollected).To(BeEmpty())
			Expect(result.RecommendedActions).To(BeEmpty())
			Expect(result.AlternativesConsidered).To(BeEmpty())
			Expect(result.Decision).To(Equal("Pending"))
		})

		It("ADV-AF-109-022: RAR with deeply nested nil maps does not panic", func() {
			nestedNilRAR := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "kubernaut.ai/v1alpha1",
					"kind":       "RemediationApprovalRequest",
					"metadata": map[string]interface{}{
						"name":      "rar-nested-nil",
						"namespace": "payments",
					},
					"spec": map[string]interface{}{
						"remediationRequestRef": map[string]interface{}{},
						"aiAnalysisRef":         map[string]interface{}{},
						"recommendedWorkflow":   map[string]interface{}{},
						"recommendedActions":    []interface{}{map[string]interface{}{}},
						"alternativesConsidered": []interface{}{map[string]interface{}{}},
					},
				},
			}
			client := newDynamicFakeClient(nestedNilRAR)
			result, err := tools.HandleGetApprovalRequest(ctx, client, tools.GetApprovalRequestArgs{
				Namespace: "payments",
				Name:      "rar-nested-nil",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RemediationRequest).To(Equal(""))
			Expect(result.RecommendedWorkflow.Name).To(Equal(""))
			Expect(result.RecommendedActions).To(HaveLen(1))
			Expect(result.RecommendedActions[0].Action).To(Equal(""))
		})
	})

	Context("Context cancellation", func() {
		It("ADV-AF-109-030: handler accepts cancelled context without panicking", func() {
			cancelledCtx, cancel := context.WithCancel(context.Background())
			cancel()

			client := newDynamicFakeClient(newDetailedFakeRAR("payments", "rar-oom-1"))
			// K8s fake client does not respect context cancellation, so we verify no panic.
			// In production, the real K8s client propagates context.Canceled.
			Expect(func() {
				_, _ = tools.HandleGetApprovalRequest(cancelledCtx, client, tools.GetApprovalRequestArgs{
					Namespace: "payments",
					Name:      "rar-oom-1",
				})
			}).NotTo(Panic())
		})
	})
})
