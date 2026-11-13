package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	remediationworkflowv1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
	"github.com/jordigilh/kubernaut/pkg/shared/contenthash"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
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

	It("UT-AF-2570-121: BR-ORCH-025 accepts a structured parameter-only override", func() {
		rar := newTypedRAR()
		rar.Spec.Workflow = &sharedtypes.WorkflowSnapshot{WorkflowID: "wf"}
		rar.Spec.Parameters = typedParameters(`{"P":2}`)
		schema := []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "float", Description: "threshold"}}
		rar.Spec.TargetContext = &remediationv1.ApprovalTargetContext{}
		tc := newTypedFakeClientWithWorkflow(rar, schema)
		var args tools.ApproveArgs
		Expect(json.Unmarshal([]byte(`{"rar_name":"rar-1","decision":"Approved","workflow_override":{"parameters":{"P":1.0},"rationale":"reviewed"}}`), &args)).To(Succeed())
		args.Namespace = "payments"
		_, err := tools.HandleApprove(ctx, tc, args, "bob")
		Expect(err).NotTo(HaveOccurred())
		persisted := &remediationv1.RemediationApprovalRequest{}
		Expect(tc.Get(ctx, crclient.ObjectKeyFromObject(rar), persisted)).To(Succeed())
		Expect(persisted.Status.WorkflowOverride.Parameters).To(Equal(typedParameters(`{"P":1.0}`)))
		Expect(persisted.Status.WorkflowOverride.Rationale).To(Equal("reviewed"))
	})

	DescribeTable("UT-AF-2570-122: BR-KA-191 rejects invalid approval changes before status update", func(decision string, changes map[string]string) {
		rar := newTypedRAR()
		rar.Spec.Workflow = &sharedtypes.WorkflowSnapshot{WorkflowID: "wf"}
		rar.Spec.Parameters = typedParameters(`{"P":3,"TARGET_RESOURCE_NAME":"pod"}`)
		schema := []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "integer", Description: "count"}, {Name: "TARGET_RESOURCE_NAME", Type: "string", Description: "target"}}
		rar.Spec.TargetContext = &remediationv1.ApprovalTargetContext{}
		tc := newTypedFakeClientWithWorkflow(rar, schema)
		_, err := tools.HandleApprove(ctx, tc, tools.ApproveArgs{Namespace: "payments", RARName: "rar-1", Decision: decision, WorkflowOverride: &remediationv1.WorkflowOverride{Parameters: typedParametersFromStrings(changes)}}, "bob")
		Expect(err).To(HaveOccurred())
		persisted := &remediationv1.RemediationApprovalRequest{}
		Expect(tc.Get(ctx, crclient.ObjectKeyFromObject(rar), persisted)).To(Succeed())
		Expect(persisted.Status.Decision).To(Equal(remediationv1.ApprovalDecisionPending))
		Expect(persisted.Status.WorkflowOverride).To(BeNil())
	},
		Entry("decimal integer", "Approved", map[string]string{"P": "1.0"}),
		Entry("unknown key", "Approved", map[string]string{"NEW": "value"}),
		Entry("changed target", "Approved", map[string]string{"TARGET_RESOURCE_NAME": "other"}),
		Entry("rejection with override", "Rejected", map[string]string{"P": "4"}),
	)

	DescribeTable("UT-AF-2570-123: BR-KA-191 validates shared vectors before persisting reviewer text", func(parameters []remediationworkflowv1.RemediationWorkflowParameter, original, changes map[string]string, valid bool) {
		rar := newTypedRAR()
		rar.Spec.Workflow = &sharedtypes.WorkflowSnapshot{WorkflowID: "wf"}
		typedOriginal := typedParametersFromStrings(original)
		typedChanges := typedParametersFromStrings(changes)
		rar.Spec.Parameters = typedOriginal
		rar.Spec.TargetContext = &remediationv1.ApprovalTargetContext{}
		tc := newTypedFakeClientWithWorkflow(rar, parameters)
		_, err := tools.HandleApprove(ctx, tc, tools.ApproveArgs{Namespace: "payments", RARName: "rar-1", Decision: "Approved", WorkflowOverride: &remediationv1.WorkflowOverride{Parameters: typedChanges}}, "bob")
		persisted := &remediationv1.RemediationApprovalRequest{}
		Expect(tc.Get(ctx, crclient.ObjectKeyFromObject(rar), persisted)).To(Succeed())
		Expect(persisted.Spec.Parameters).To(Equal(typedOriginal))
		if valid {
			Expect(err).NotTo(HaveOccurred())
			Expect(persisted.Status.Decision).To(Equal(remediationv1.ApprovalDecisionApproved))
			Expect(persisted.Status.WorkflowOverride.Parameters).To(Equal(typedChanges))
		} else {
			Expect(err).To(HaveOccurred())
			Expect(persisted.Status.Decision).To(Equal(remediationv1.ApprovalDecisionPending))
			Expect(persisted.Status.WorkflowOverride).To(BeNil())
		}
	},
		Entry("V-01 partial override", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "string"}, {Name: "Q", Type: "string", Required: true}}, map[string]string{"P": "old", "Q": "keep"}, map[string]string{"P": "new"}, true),
		Entry("V-02 empty override", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "string"}}, map[string]string{"P": "old"}, map[string]string{}, true),
		Entry("V-03 decimal integer", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "integer"}}, map[string]string{"P": "3"}, map[string]string{"P": "1.0"}, false),
		Entry("V-04 exact float", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "float"}}, map[string]string{"P": "2"}, map[string]string{"P": "1.0"}, true),
		Entry("V-05 exact integer bound", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "integer", Maximum: ptr.To(float64(9007199254740992))}}, map[string]string{"P": "9007199254740992"}, map[string]string{"P": "9007199254740993"}, false),
		Entry("V-06 integer overflow", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "integer"}}, map[string]string{"P": "3"}, map[string]string{"P": "9223372036854775808"}, false),
		Entry("V-07 float nonfinite", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "float"}}, map[string]string{"P": "1"}, map[string]string{"P": "NaN"}, false),
		Entry("V-07 float overflow", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "float"}}, map[string]string{"P": "1"}, map[string]string{"P": "1e400"}, false),
		Entry("V-07 float underflow", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "float"}}, map[string]string{"P": "1"}, map[string]string{"P": "1e-4000"}, false),
		Entry("V-08 boolean", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "boolean"}}, map[string]string{"P": "false"}, map[string]string{"P": "true"}, true),
		Entry("V-08 invalid boolean", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "boolean"}}, map[string]string{"P": "false"}, map[string]string{"P": "notbool"}, false),
		Entry("V-09 exact array", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "array"}}, map[string]string{"P": "[]"}, map[string]string{"P": `["a","b"]`}, true),
		Entry("V-09 null array", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "array"}}, map[string]string{"P": "[]"}, map[string]string{"P": "null"}, false),
		Entry("V-10 enum violation", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "string", Enum: []string{"safe", "fast"}}}, map[string]string{"P": "safe"}, map[string]string{"P": "other"}, false),
		Entry("V-11 required missing", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "string", Required: true}}, map[string]string{}, map[string]string{}, false),
		Entry("V-12 optional default remains omitted", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "string", Default: &apiextensionsv1.JSON{Raw: []byte(`"auto"`)}}}, map[string]string{}, map[string]string{}, true),
		Entry("V-13 missing dependency", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "string", DependsOn: []string{"Q"}}, {Name: "Q", Type: "string"}}, map[string]string{"P": "x"}, map[string]string{}, false),
		Entry("V-13 present dependency", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "string", DependsOn: []string{"Q"}}, {Name: "Q", Type: "string"}}, map[string]string{"P": "x", "Q": "y"}, map[string]string{}, true),
		Entry("V-14 malformed schema", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "string", Pattern: "["}}, map[string]string{"P": "x"}, map[string]string{}, false),
		Entry("V-15 unknown key", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "string"}}, map[string]string{"P": "old"}, map[string]string{"NEW": "x"}, false),
		Entry("V-16 unchanged target", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "TARGET_RESOURCE_NAME", Type: "string"}}, map[string]string{"TARGET_RESOURCE_NAME": "pod-a"}, map[string]string{"TARGET_RESOURCE_NAME": "pod-a"}, true),
		Entry("V-16 changed target", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "TARGET_RESOURCE_NAME", Type: "string"}}, map[string]string{"TARGET_RESOURCE_NAME": "pod-a"}, map[string]string{"TARGET_RESOURCE_NAME": "pod-b"}, false),
		Entry("V-17 pattern violation", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "string", Pattern: "^[a-z]+$"}}, map[string]string{"P": "safe"}, map[string]string{"P": "FAST"}, false),
	)

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

	It("UT-AF-104-005: rejects workflow switching in approval", func() {
		tc := newTypedFakeClientWithStatus(newTypedRAR())
		_, err := tools.HandleApprove(ctx, tc, tools.ApproveArgs{
			Namespace:        "payments",
			RARName:          "rar-1",
			Decision:         "Approved",
			WorkflowOverride: &remediationv1.WorkflowOverride{WorkflowName: "fast-restart"},
		}, "bob")
		Expect(err).To(HaveOccurred())
		var denied *tools.ApprovalDecisionDeniedError
		Expect(errors.As(err, &denied)).To(BeTrue(), "workflow override admission denials must preserve structured approval context")
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

	It("IT-AF-2564-010: definitive status rejection is returned as a user-facing error", func() {
		rar := newTypedRAR()
		tc := fake.NewClientBuilder().
			WithScheme(rrTestScheme()).
			WithObjects(rar).
			WithStatusSubresource(&remediationv1.RemediationApprovalRequest{}).
			WithInterceptorFuncs(interceptor.Funcs{
				SubResourceUpdate: func(_ context.Context, _ crclient.Client, subresource string, _ crclient.Object, _ ...crclient.SubResourceUpdateOption) error {
					Expect(subresource).To(Equal("status"))
					return apierrors.NewForbidden(
						schema.GroupResource{Group: "remediation.kubernaut.ai", Resource: "remediationapprovalrequests/status"},
						"rar-1",
						errors.New("forbidden by test"),
					)
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
		Expect(errors.As(err, &writeErr)).To(BeFalse(), "a definitive API rejection is not an unknown write outcome")
		Expect(err.Error()).To(ContainSubstring("lack access"))
	})

	It("UT-AF-2570-025: BR-AUTH-001 reports a webhook-denied concurrent decision as a conflict after readback", func() {
		rar := newTypedRAR()
		rar.UID = "rar-webhook-race"
		tc := fake.NewClientBuilder().WithScheme(rrTestScheme()).WithObjects(rar).
			WithStatusSubresource(&remediationv1.RemediationApprovalRequest{}).
			WithInterceptorFuncs(interceptor.Funcs{
				SubResourceUpdate: func(ctx context.Context, underlying crclient.Client, subresource string, obj crclient.Object, _ ...crclient.SubResourceUpdateOption) error {
					winner := obj.(*remediationv1.RemediationApprovalRequest).DeepCopy()
					winner.Status.Decision = remediationv1.ApprovalDecisionRejected
					winner.Status.DecidedBy = "alice"
					Expect(underlying.SubResource(subresource).Update(ctx, winner)).To(Succeed())
					return apierrors.NewForbidden(schema.GroupResource{Group: "remediation.kubernaut.ai", Resource: "remediationapprovalrequests/status"}, "rar-1", errors.New("admission webhook denied the request: terminal decision is immutable"))
				},
			}).Build()
		_, err := tools.HandleApprove(ctx, tc, tools.ApproveArgs{Namespace: "payments", RARName: "rar-1", Decision: "Approved"}, "bob")
		var writeErr *tools.ApprovalDecisionWriteError
		Expect(errors.As(err, &writeErr)).To(BeTrue())
		Expect(writeErr.Outcome).To(Equal("conflict"))
	})

	DescribeTable("IT-AF-2564-010: preserves known AuthWebhook denial reasons", func(denial, expected string) {
		rar := newTypedRAR()
		tc := fake.NewClientBuilder().
			WithScheme(rrTestScheme()).
			WithObjects(rar).
			WithStatusSubresource(&remediationv1.RemediationApprovalRequest{}).
			WithInterceptorFuncs(interceptor.Funcs{
				SubResourceUpdate: func(_ context.Context, _ crclient.Client, subresource string, _ crclient.Object, _ ...crclient.SubResourceUpdateOption) error {
					Expect(subresource).To(Equal("status"))
					return apierrors.NewForbidden(
						schema.GroupResource{Group: "remediation.kubernaut.ai", Resource: "remediationapprovalrequests/status"},
						"rar-1",
						errors.New("admission webhook \"rar-auth\" denied the request: "+denial),
					)
				},
			}).
			Build()

		_, err := tools.HandleApprove(ctx, tc, tools.ApproveArgs{
			Namespace: "payments",
			RARName:   "rar-1",
			Decision:  "Approved",
		}, "bob")

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(expected))
		Expect(err.Error()).NotTo(ContainSubstring("lack access"))
	},
		Entry("expired deadline", "approval decision deadline has passed", "approval decision deadline has passed"),
		Entry("invalid workflow override", "override rejected: workflowOverride is only valid when decision is Approved", "workflow override was rejected"),
	)

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
	return fake.NewClientBuilder().WithScheme(rrTestScheme()).WithObjects(objects...).WithStatusSubresource(objects...).Build()
}

func newTypedFakeClientWithWorkflow(rar *remediationv1.RemediationApprovalRequest, parameters []remediationworkflowv1.RemediationWorkflowParameter) crclient.Client {
	rw := &remediationworkflowv1.RemediationWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "workflow-" + rar.Name, Namespace: rar.Namespace},
		Spec: remediationworkflowv1.RemediationWorkflowSpec{
			Version:    "1.0",
			ActionType: "Test",
			Description: remediationworkflowv1.RemediationWorkflowDescription{
				What: "test workflow", WhenToUse: "approval test",
			},
			Execution:  remediationworkflowv1.RemediationWorkflowExecution{Engine: "tekton", Bundle: "oci://example.test/workflow:1"},
			Parameters: parameters,
		},
	}
	content, err := contenthash.MarshalCleanCRDContent(rw)
	if err != nil {
		panic(err)
	}
	rw.Status.ContentHash = contenthash.ComputeContentHash(string(content))
	rw.Status.WorkflowID = contenthash.DeterministicUUID(rw.Status.ContentHash)
	rar.Spec.Workflow.WorkflowID = rw.Status.WorkflowID
	augmented := []crclient.Object{rar, rw}
	return fake.NewClientBuilder().
		WithScheme(rrTestScheme()).
		WithObjects(augmented...).
		WithStatusSubresource(augmented...).
		WithIndex(&remediationworkflowv1.RemediationWorkflow{}, "status.workflowId", func(obj crclient.Object) []string {
			return []string{obj.(*remediationworkflowv1.RemediationWorkflow).Status.WorkflowID}
		}).
		Build()
}
