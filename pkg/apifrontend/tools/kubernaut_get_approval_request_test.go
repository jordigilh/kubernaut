package tools_test

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	remediationworkflowv1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("kubernaut_get_approval_request", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("UT-AF-2570-034: BR-ORCH-025 exposes the frozen workflow, exact values and complete schema", func() {
		rar := newTypedDetailedRAR("payments", "rar-oom-1")
		minimum, maximum := 1.0, 10.0
		rar.Spec.Workflow = &sharedtypes.WorkflowSnapshot{WorkflowID: "wf-reviewed", DeclaredParameterNames: map[string]bool{"P": true}}
		rar.Spec.Parameters = map[string]string{"P": "1.0"}
		rar.Spec.ParameterSchema = []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "float", Description: "threshold", Required: true, Enum: []string{"1.0"}, Pattern: "", Minimum: &minimum, Maximum: &maximum, Default: &apiextensionsv1.JSON{Raw: []byte(`1.0`)}, DependsOn: []string{"Q"}}}
		result, err := tools.HandleGetApprovalRequest(ctx, newTypedFakeClient(rar), tools.GetApprovalRequestArgs{Namespace: "payments", Name: rar.Name})
		Expect(err).NotTo(HaveOccurred())
		encoded, err := json.Marshal(result)
		Expect(err).NotTo(HaveOccurred())
		var fields map[string]json.RawMessage
		Expect(json.Unmarshal(encoded, &fields)).To(Succeed())
		Expect(string(fields["parameters"])).To(MatchJSON(`{"P":"1.0"}`))
		Expect(string(fields["parameter_schema"])).To(MatchJSON(`[{"name":"P","type":"float","required":true,"description":"threshold","enum":["1.0"],"minimum":1,"maximum":10,"default":1.0,"dependsOn":["Q"]}]`))
		Expect(string(fields["workflow"])).To(ContainSubstring("wf-reviewed"))
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
		Expect(result.WorkflowConfidence).To(BeNumerically("~", 0.72, 0.001))
		Expect(result.ConfidenceLevel).To(Equal("medium"))
		Expect(result.InvestigationSummary).To(ContainSubstring("memory limits"))
		Expect(result.WhyApprovalRequired).To(ContainSubstring("auto-approve"))
		Expect(result.Decision).To(Equal("Pending"))
		Expect(result.TimeRemaining).To(Equal("12m30s"))
		Expect(result.Expired).To(BeFalse())
	})

	It("UT-AF-2586-002: approval list and detail preserve explicit zero RCA independently (BR-AI-076, AU-3)", func() {
		rar := newTypedDetailedRAR("payments", "rar-2586")
		zero := 0.0
		rar.Spec.RCAConfidence = &zero
		tc := newTypedFakeClient(rar)
		detail, err := tools.HandleGetApprovalRequest(ctx, tc, tools.GetApprovalRequestArgs{Namespace: "payments", Name: rar.Name})
		Expect(err).NotTo(HaveOccurred())
		Expect(detail.RCAConfidence).To(HaveValue(Equal(0.0)))
		Expect(detail.WorkflowConfidence).To(Equal(.72))
		encoded, err := json.Marshal(detail)
		Expect(err).NotTo(HaveOccurred())
		var fields map[string]json.RawMessage
		Expect(json.Unmarshal(encoded, &fields)).To(Succeed())
		Expect(fields).To(HaveKey("rca_confidence"))
		Expect(fields).To(HaveKey("workflow_confidence"))
		Expect(fields).NotTo(HaveKey("confidence"))
		list, err := tools.HandleListApprovalRequests(ctx, tc, tools.ListApprovalRequestsArgs{Namespace: "payments"})
		Expect(err).NotTo(HaveOccurred())
		Expect(list.ApprovalRequests).To(HaveLen(1))
		Expect(list.ApprovalRequests[0].RCAConfidence).To(HaveValue(Equal(0.0)))
		Expect(list.ApprovalRequests[0].WorkflowConfidence).To(Equal(.72))
	})

	It("UT-AF-1959-003 (was UT-AF-109-002): returns full RAR detail by bare rar_id shorthand", func() {
		tc := newTypedFakeClient(newTypedDetailedRAR("payments", "rar-oom-1"))
		result, err := tools.HandleGetApprovalRequest(ctx, tc, tools.GetApprovalRequestArgs{
			RARID:     "rar-oom-1",
			Namespace: "payments",
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

	It("IT-AF-2565-008: returns bounded owner-validated parent evidence only when requested", func() {
		parent := newTypedRR("payments", "rr-oom-1", "AwaitingApproval")
		parent.UID = "rr-parent-uid"
		parent.Spec.SignalSource = "prometheus-adapter"
		parent.Spec.ClusterID = "spoke-a"
		parent.Spec.TargetResource = remediationv1.ResourceIdentifier{
			Kind: "Deployment", Name: "payments", Namespace: "payments", APIVersion: "apps/v1",
		}
		rar := newTypedDetailedRAR("payments", "rar-oom-1")
		rar.OwnerReferences = []metav1.OwnerReference{{
			APIVersion: remediationv1.GroupVersion.String(),
			Kind:       "RemediationRequest",
			Name:       parent.Name,
			UID:        parent.UID,
			Controller: boolPtr(true),
		}}

		tc := newTypedFakeClient(parent, rar)
		result, err := tools.HandleGetApprovalRequest(ctx, tc, tools.GetApprovalRequestArgs{
			Namespace: "payments", Name: "rar-oom-1", IncludeParentEvidence: true,
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(result.ParentEvidence).NotTo(BeNil())
		Expect(result.ParentEvidence.State).To(Equal("complete"))
		Expect(result.ParentEvidence.Complete).To(BeTrue())
		Expect(result.ParentEvidence.Truncated).To(BeFalse())
		Expect(result.ParentEvidence.Parent.Name).To(Equal("rr-oom-1"))
		Expect(result.ParentEvidence.Parent.TargetResource.Kind).To(Equal("Deployment"))

		encoded, marshalErr := json.Marshal(result.ParentEvidence)
		Expect(marshalErr).NotTo(HaveOccurred())
		Expect(len(encoded)).To(BeNumerically("<=", 16*1024))
	})

	It("IT-AF-2565-008: ignores a non-controller or wrong-group owner", func() {
		parent := newTypedRR("payments", "rr-oom-1", "AwaitingApproval")
		parent.UID = "rr-parent-uid"
		rar := newTypedDetailedRAR("payments", "rar-oom-1")
		rar.OwnerReferences = []metav1.OwnerReference{{
			APIVersion: "other.example/v1",
			Kind:       "RemediationRequest",
			Name:       parent.Name,
			UID:        parent.UID,
			Controller: boolPtr(false),
		}}

		result, err := tools.HandleGetApprovalRequest(ctx, newTypedFakeClient(parent, rar), tools.GetApprovalRequestArgs{
			Namespace: "payments", Name: "rar-oom-1", IncludeParentEvidence: true,
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(result.ParentEvidence).To(Equal(&tools.ParentEvidence{State: "missing", Complete: false}))
	})

	It("IT-AF-2565-008: omits parent evidence and performs no parent projection by default", func() {
		result, err := tools.HandleGetApprovalRequest(ctx, newTypedFakeClient(newTypedDetailedRAR("payments", "rar-oom-1")), tools.GetApprovalRequestArgs{
			Namespace: "payments", Name: "rar-oom-1",
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(result.ParentEvidence).To(BeNil())
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
		Expect(result.WorkflowConfidence).To(BeNumerically("~", 0.72, 0.001))
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
			Expect(result.WorkflowConfidence).To(Equal(0.0))
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

func boolPtr(value bool) *bool {
	return &value
}

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
