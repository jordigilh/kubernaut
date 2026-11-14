/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package authwebhook_test

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	remediationworkflowv1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	"github.com/jordigilh/kubernaut/pkg/shared/contenthash"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	admissionv1 "k8s.io/api/admission/v1"
	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// TDD Phase: RED — Issue #594 Authwebhook Override Validation Tests
// BR-ORCH-031: Webhook validates override references valid, Active RW CRD
// G5: Override validation ordered AFTER authentication (SOC2 CC8.1)
//
// These tests validate that the RAR webhook correctly validates
// WorkflowOverride when present on an approval decision.

var _ = Describe("BR-ORCH-031: RAR Webhook Override Validation (#594)", func() {
	var (
		ctx       context.Context
		handler   *authwebhook.RemediationApprovalRequestAuthHandler
		mockStore *MockAuditStore
		scheme    *runtime.Scheme
		decoder   admission.Decoder
		reader    client.Reader
	)

	buildAdmissionRequest := func(rar *remediationv1.RemediationApprovalRequest, oldRAR *remediationv1.RemediationApprovalRequest) admission.Request {
		rawObj, err := json.Marshal(rar)
		Expect(err).NotTo(HaveOccurred())

		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				UID:       "test-uid",
				Kind:      metav1.GroupVersionKind{Group: "remediation.kubernaut.ai", Version: "v1alpha1", Kind: "RemediationApprovalRequest"},
				Resource:  metav1.GroupVersionResource{Group: "remediation.kubernaut.ai", Version: "v1alpha1", Resource: "remediationapprovalrequests"},
				Name:      rar.Name,
				Namespace: rar.Namespace,
				Operation: admissionv1.Update,
				Object:    runtime.RawExtension{Raw: rawObj},
				UserInfo: authv1.UserInfo{
					Username: "operator@kubernaut.ai",
					UID:      "k8s-user-594",
				},
			},
		}

		if oldRAR != nil {
			rawOld, err := json.Marshal(oldRAR)
			Expect(err).NotTo(HaveOccurred())
			req.OldObject = runtime.RawExtension{Raw: rawOld}
		} else {
			emptyRAR := &remediationv1.RemediationApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{Name: rar.Name, Namespace: rar.Namespace},
			}
			rawOld, err := json.Marshal(emptyRAR)
			Expect(err).NotTo(HaveOccurred())
			req.OldObject = runtime.RawExtension{Raw: rawOld}
		}

		return req
	}

	//nolint:unparam // Ginkgo closures exercise pending, approved, and rejected decisions.
	buildRAR := func(decision remediationv1.ApprovalDecision, override *remediationv1.WorkflowOverride) *remediationv1.RemediationApprovalRequest {
		return &remediationv1.RemediationApprovalRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rar-test-594",
				Namespace: "kubernaut-system",
			},
			Spec: remediationv1.RemediationApprovalRequestSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: "rr-test-594"},
				AIAnalysisRef:         remediationv1.ObjectRef{Name: "ai-test-594"},
				Confidence:            0.72,
				ConfidenceLevel:       "medium",
				Reason:                "test",
				RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{
					WorkflowID:      "wf-original",
					Version:         "1.0",
					ExecutionBundle: "original-bundle",
					Rationale:       "AI selected",
				},
				InvestigationSummary: "test investigation",
				RecommendedActions:   []remediationv1.ApprovalRecommendedAction{{Action: "test", Rationale: "test"}},
				WhyApprovalRequired:  "confidence below threshold",
				RequiredBy:           metav1.NewTime(time.Now().Add(15 * time.Minute)),
			},
			Status: remediationv1.RemediationApprovalRequestStatus{
				Decision:         decision,
				WorkflowOverride: override,
			},
		}
	}

	buildActiveRW := func(name, namespace string) *remediationworkflowv1.RemediationWorkflow {
		return &remediationworkflowv1.RemediationWorkflow{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: remediationworkflowv1.RemediationWorkflowSpec{
				Version:    "2.0",
				ActionType: "DrainRestart",
				Description: remediationworkflowv1.RemediationWorkflowDescription{
					What:      "Drains and restarts node",
					WhenToUse: "When node is unhealthy",
				},
				Labels: remediationworkflowv1.RemediationWorkflowLabels{
					Severity:    []string{"critical"},
					Environment: []string{"production"},
					Component:   []string{"v1/Node"},
					Priority:    "P1",
				},
				Execution: remediationworkflowv1.RemediationWorkflowExecution{
					Engine: "tekton",
					Bundle: "new-bundle:v2.0@sha256:abc",
				},
				Parameters: []remediationworkflowv1.RemediationWorkflowParameter{
					{Name: "TIMEOUT", Type: "string", Required: true, Description: "timeout"},
				},
			},
			Status: remediationworkflowv1.RemediationWorkflowStatus{
				WorkflowID:    "wf-override-002",
				CatalogStatus: sharedtypes.CatalogStatusActive,
			},
		}
	}

	buildVerifiedActiveRW := func(name, namespace string, parameters []remediationworkflowv1.RemediationWorkflowParameter) *remediationworkflowv1.RemediationWorkflow {
		rw := buildActiveRW(name, namespace)
		rw.Spec.Parameters = parameters
		content, err := contenthash.MarshalCleanCRDContent(rw)
		Expect(err).NotTo(HaveOccurred())
		rw.Status.ContentHash = contenthash.ComputeContentHash(string(content))
		rw.Status.WorkflowID = contenthash.DeterministicUUID(rw.Status.ContentHash)
		return rw
	}

	workflowReader := func(scheme *runtime.Scheme, rw *remediationworkflowv1.RemediationWorkflow) client.Reader {
		return fake.NewClientBuilder().WithScheme(scheme).WithObjects(rw).WithStatusSubresource(rw).
			WithIndex(&remediationworkflowv1.RemediationWorkflow{}, "status.workflowId", func(obj client.Object) []string {
				return []string{obj.(*remediationworkflowv1.RemediationWorkflow).Status.WorkflowID}
			}).Build()
	}

	BeforeEach(func() {
		ctx = context.Background()
		mockStore = &MockAuditStore{}

		scheme = runtime.NewScheme()
		_ = remediationv1.AddToScheme(scheme)
		_ = remediationworkflowv1.AddToScheme(scheme)

		decoder = admission.NewDecoder(scheme)
	})

	It("UT-AW-2570-003: BR-ORCH-025 preserves explicit empty override parameters distinctly from omission", func() {
		for _, params := range []map[string]string{nil, {}} {
			original := remediationv1.WorkflowOverride{Parameters: typedParametersFromStrings(params)}
			encoded, err := json.Marshal(original)
			Expect(err).NotTo(HaveOccurred())
			var decoded remediationv1.WorkflowOverride
			Expect(json.Unmarshal(encoded, &decoded)).To(Succeed())
			if params == nil {
				Expect(decoded.Parameters).To(BeNil())
			} else {
				Expect(decoded.Parameters).NotTo(BeNil())
				Expect(decoded.Parameters).To(BeEmpty())
			}
		}
	})

	It("UT-AW-2570-099: BR-KA-191 validates original parameters on an ordinary approval", func() {
		oldRAR := buildRAR(remediationv1.ApprovalDecisionPending, nil)
		oldRAR.Spec.Workflow = &sharedtypes.WorkflowSnapshot{
			WorkflowID: "wf-original",
		}
		oldRAR.Spec.Parameters = typedParametersFromStrings(map[string]string{"P": "1.0"})
		schema := []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "integer"}}
		oldRAR.Spec.TargetContext = &remediationv1.ApprovalTargetContext{}
		rw := buildVerifiedActiveRW("original-workflow", "kubernaut-system", schema)
		oldRAR.Spec.Workflow.WorkflowID = rw.Status.WorkflowID
		reader = workflowReader(scheme, rw)
		handler = authwebhook.NewRemediationApprovalRequestAuthHandler(mockStore, reader, "kubernaut-system")
		Expect(handler.InjectDecoder(decoder)).To(Succeed())
		rar := oldRAR.DeepCopy()
		rar.Status.Decision = remediationv1.ApprovalDecisionApproved

		response := handler.Handle(ctx, buildAdmissionRequest(rar, oldRAR))

		Expect(response.Allowed).To(BeFalse())
		Expect(response.Result.Message).To(ContainSubstring("P"))
	})

	It("UT-AW-2570-087: BR-ORCH-025 rejects legacy workflow switching before approval", func() {
		rw := buildActiveRW("drain-restart", "kubernaut-system")
		reader = fake.NewClientBuilder().WithScheme(scheme).WithObjects(rw).WithStatusSubresource(rw).Build()
		handler = authwebhook.NewRemediationApprovalRequestAuthHandler(mockStore, reader, "kubernaut-system")
		Expect(handler.InjectDecoder(decoder)).To(Succeed())
		oldRAR := buildRAR(remediationv1.ApprovalDecisionPending, nil)
		rar := oldRAR.DeepCopy()
		rar.Status.Decision = remediationv1.ApprovalDecisionApproved
		rar.Status.WorkflowOverride = &remediationv1.WorkflowOverride{WorkflowName: "drain-restart"}
		response := handler.Handle(ctx, buildAdmissionRequest(rar, oldRAR))
		Expect(response.Allowed).To(BeFalse())
		Expect(response.Result).NotTo(BeNil())
		Expect(response.Result.Message).To(ContainSubstring("workflow"))
	})

	DescribeTable("UT-AW-2570-066: BR-KA-191 rejects invalid reviewed parameter values", func(changes map[string]string, wantError string) {
		oldRAR := buildRAR(remediationv1.ApprovalDecisionPending, nil)
		oldRAR.Spec.Workflow = &sharedtypes.WorkflowSnapshot{WorkflowID: "wf-original"}
		oldRAR.Spec.Parameters = typedParametersFromStrings(map[string]string{"P": "3"})
		schema := []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "integer", Required: true, Description: "count"}}
		oldRAR.Spec.TargetContext = &remediationv1.ApprovalTargetContext{}
		rw := buildVerifiedActiveRW("invalid-override", "kubernaut-system", schema)
		oldRAR.Spec.Workflow.WorkflowID = rw.Status.WorkflowID
		reader = workflowReader(scheme, rw)
		handler = authwebhook.NewRemediationApprovalRequestAuthHandler(mockStore, reader, "kubernaut-system")
		Expect(handler.InjectDecoder(decoder)).To(Succeed())
		rar := oldRAR.DeepCopy()
		rar.Status.Decision = remediationv1.ApprovalDecisionApproved
		rar.Status.WorkflowOverride = &remediationv1.WorkflowOverride{Parameters: typedParametersFromStrings(changes)}
		response := handler.Handle(ctx, buildAdmissionRequest(rar, oldRAR))
		Expect(response.Allowed).To(BeFalse())
		Expect(response.Result.Message).To(ContainSubstring(wantError))
	},
		Entry("decimal for integer", map[string]string{"P": "1.0"}, "P"),
		Entry("UT-AW-2570-004: unknown key", map[string]string{"NEW": "value"}, "NEW"),
	)

	It("UT-AW-2570-103: BR-AUDIT-005 records an audit event for an invalid override denial", func() {
		oldRAR := buildRAR(remediationv1.ApprovalDecisionPending, nil)
		oldRAR.Spec.Workflow = &sharedtypes.WorkflowSnapshot{
			WorkflowID: "wf-invalid-override-audit",
		}
		oldRAR.Spec.Parameters = typedParametersFromStrings(map[string]string{"P": "original"})
		schema := []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "string"}}
		oldRAR.Spec.TargetContext = &remediationv1.ApprovalTargetContext{}
		rw := buildVerifiedActiveRW("invalid-override-audit", "kubernaut-system", schema)
		oldRAR.Spec.Workflow.WorkflowID = rw.Status.WorkflowID
		reader = workflowReader(scheme, rw)
		handler = authwebhook.NewRemediationApprovalRequestAuthHandler(mockStore, reader, "kubernaut-system")
		Expect(handler.InjectDecoder(decoder)).To(Succeed())
		rar := oldRAR.DeepCopy()
		rar.Status.Decision = remediationv1.ApprovalDecisionApproved
		rar.Status.WorkflowOverride = &remediationv1.WorkflowOverride{Parameters: typedParametersFromStrings(map[string]string{"NEW": "value"})}

		response := handler.Handle(ctx, buildAdmissionRequest(rar, oldRAR))

		Expect(response.Allowed).To(BeFalse())
		Expect(mockStore.StoredEvents).To(HaveLen(1))
		Expect(mockStore.StoredEvents[0].EventAction).To(Equal("approval_decision_denied"))
		Expect(string(mockStore.StoredEvents[0].EventOutcome)).To(Equal("failure"))
	})

	It("UT-AW-2570-017: BR-AUTH-001 rejects a valid override when the trusted workflow ID is absent", func() {
		reader = fake.NewClientBuilder().WithScheme(scheme).WithIndex(&remediationworkflowv1.RemediationWorkflow{}, "status.workflowId", func(obj client.Object) []string {
			return []string{obj.(*remediationworkflowv1.RemediationWorkflow).Status.WorkflowID}
		}).Build()
		handler = authwebhook.NewRemediationApprovalRequestAuthHandler(mockStore, reader, "kubernaut-system")
		Expect(handler.InjectDecoder(decoder)).To(Succeed())
		oldRAR := buildRAR(remediationv1.ApprovalDecisionPending, nil)
		oldRAR.Spec.Workflow = &sharedtypes.WorkflowSnapshot{WorkflowID: "wf-missing"}
		oldRAR.Spec.Parameters = typedParametersFromStrings(map[string]string{"P": "3"})
		oldRAR.Spec.TargetContext = &remediationv1.ApprovalTargetContext{}
		rar := oldRAR.DeepCopy()
		rar.Status.Decision = remediationv1.ApprovalDecisionApproved
		rar.Status.WorkflowOverride = &remediationv1.WorkflowOverride{Parameters: typedParametersFromStrings(map[string]string{"P": "4"})}
		response := handler.Handle(ctx, buildAdmissionRequest(rar, oldRAR))
		Expect(response.Allowed).To(BeFalse())
		Expect(response.Result.Message).To(ContainSubstring("workflow"))
	})

	DescribeTable("UT-AW-2570-124: BR-AUTH-001 verifies trusted catalog content before approval", func(foreign, stale bool, allowed bool) {
		rw := buildVerifiedActiveRW("trusted-restart", "kubernaut-system", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "integer", Description: "count"}})
		workflowID := rw.Status.WorkflowID
		if foreign {
			rw.Namespace = "foreign-namespace"
		}
		if stale {
			rw.Spec.Version = "changed-after-registration"
		}
		reader = fake.NewClientBuilder().WithScheme(scheme).WithObjects(rw).WithStatusSubresource(rw).
			WithIndex(&remediationworkflowv1.RemediationWorkflow{}, "status.workflowId", func(obj client.Object) []string {
				return []string{obj.(*remediationworkflowv1.RemediationWorkflow).Status.WorkflowID}
			}).Build()
		handler = authwebhook.NewRemediationApprovalRequestAuthHandler(mockStore, reader, "kubernaut-system")
		Expect(handler.InjectDecoder(decoder)).To(Succeed())
		oldRAR := buildRAR(remediationv1.ApprovalDecisionPending, nil)
		oldRAR.Spec.Workflow = &sharedtypes.WorkflowSnapshot{WorkflowID: workflowID}
		oldRAR.Spec.Parameters = typedParametersFromStrings(map[string]string{"P": "3"})
		oldRAR.Spec.TargetContext = &remediationv1.ApprovalTargetContext{}
		rar := oldRAR.DeepCopy()
		rar.Status.Decision = remediationv1.ApprovalDecisionApproved
		rar.Status.WorkflowOverride = &remediationv1.WorkflowOverride{Parameters: typedParametersFromStrings(map[string]string{"P": "4"})}
		response := handler.Handle(ctx, buildAdmissionRequest(rar, oldRAR))
		reason := ""
		if response.Result != nil {
			reason = response.Result.Message
		}
		Expect(response.Allowed).To(Equal(allowed), reason)
		if !allowed {
			Expect(response.Result.Message).To(ContainSubstring("workflow"))
		}
	},
		Entry("matching content", false, false, true),
		Entry("foreign namespace only", true, false, false),
		Entry("changed content", false, true, false),
	)

	DescribeTable("BR-KA-191: admission validates the complete effective parameter map without rewriting review text", func(schema []remediationworkflowv1.RemediationWorkflowParameter, original, changes map[string]string, allowed bool) {
		rw := buildVerifiedActiveRW("trusted-parameters", "kubernaut-system", schema)
		reader = fake.NewClientBuilder().WithScheme(scheme).WithObjects(rw).WithStatusSubresource(rw).
			WithIndex(&remediationworkflowv1.RemediationWorkflow{}, "status.workflowId", func(obj client.Object) []string {
				return []string{obj.(*remediationworkflowv1.RemediationWorkflow).Status.WorkflowID}
			}).Build()
		handler = authwebhook.NewRemediationApprovalRequestAuthHandler(mockStore, reader, "kubernaut-system")
		Expect(handler.InjectDecoder(decoder)).To(Succeed())
		oldRAR := buildRAR(remediationv1.ApprovalDecisionPending, nil)
		oldRAR.Spec.Workflow = &sharedtypes.WorkflowSnapshot{WorkflowID: rw.Status.WorkflowID}
		oldRAR.Spec.Parameters = typedParametersFromStrings(original)
		oldRAR.Spec.TargetContext = &remediationv1.ApprovalTargetContext{}
		rar := oldRAR.DeepCopy()
		rar.Status.Decision = remediationv1.ApprovalDecisionApproved
		rar.Status.WorkflowOverride = &remediationv1.WorkflowOverride{Parameters: typedParametersFromStrings(changes), Rationale: "reviewed"}
		response := handler.Handle(ctx, buildAdmissionRequest(rar, oldRAR))
		reason := ""
		if response.Result != nil {
			reason = response.Result.Message
		}
		Expect(response.Allowed).To(Equal(allowed), reason)
		Expect(rar.Status.WorkflowOverride.Parameters).To(Equal(typedParametersFromStrings(changes)))
		if allowed {
			for _, patch := range response.Patches {
				Expect(patch.Path).NotTo(HavePrefix("/status/workflowOverride"), "admission must not rewrite reviewer input")
			}
		}
	},
		Entry("UT-AW-2570-065: float 1.0 remains exact", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "float"}}, map[string]string{"P": "2"}, map[string]string{"P": "1.0"}, true),
		Entry("UT-AW-2570-001: partial override retains required omitted value", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "string"}, {Name: "Q", Type: "string", Required: true}}, map[string]string{"P": "old", "Q": "keep"}, map[string]string{"P": "new"}, true),
		Entry("UT-AW-2570-095: rationale-only keeps the original required value", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "string", Required: true}}, map[string]string{"P": "keep"}, nil, true),
		Entry("UT-AW-2570-078: boolean true", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "boolean"}}, map[string]string{"P": "false"}, map[string]string{"P": "true"}, true),
		Entry("UT-AW-2570-078: malformed boolean", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "boolean"}}, map[string]string{"P": "false"}, map[string]string{"P": "notbool"}, false),
		Entry("UT-AW-2570-079: missing required value", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "string", Required: true}}, map[string]string{}, map[string]string{}, false),
		Entry("UT-AW-2570-005: locked target changes", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "TARGET_RESOURCE_NAME", Type: "string"}}, map[string]string{"TARGET_RESOURCE_NAME": "pod-a"}, map[string]string{"TARGET_RESOURCE_NAME": "pod-b"}, false),
		Entry("UT-AW-2570-015: malformed schema", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "string", Pattern: "["}}, map[string]string{"P": "x"}, map[string]string{}, false),
		Entry("V-03: decimal integer is rejected", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "integer"}}, map[string]string{"P": "3"}, map[string]string{"P": "1.0"}, false),
		Entry("V-05: exact integer bound", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "integer", Maximum: ptr.To(float64(9007199254740992))}}, map[string]string{"P": "9007199254740992"}, map[string]string{"P": "9007199254740993"}, false),
		Entry("V-06: integer overflow is rejected", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "integer"}}, map[string]string{"P": "3"}, map[string]string{"P": "9223372036854775808"}, false),
		Entry("V-07: nonfinite float is rejected", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "float"}}, map[string]string{"P": "1"}, map[string]string{"P": "NaN"}, false),
		Entry("V-07: float overflow is rejected", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "float"}}, map[string]string{"P": "1"}, map[string]string{"P": "1e400"}, false),
		Entry("V-07: float underflow is rejected", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "float"}}, map[string]string{"P": "1"}, map[string]string{"P": "1e-4000"}, false),
		Entry("V-09: array text is preserved", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "array"}}, map[string]string{"P": "[]"}, map[string]string{"P": `["a","b"]`}, true),
		Entry("V-09: null is not an array", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "array"}}, map[string]string{"P": "[]"}, map[string]string{"P": "null"}, false),
		Entry("V-10: enum violation", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "string", Enum: []string{"safe", "fast"}}}, map[string]string{"P": "safe"}, map[string]string{"P": "other"}, false),
		Entry("V-12: optional default remains omitted", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "string", Default: &apiextensionsv1.JSON{Raw: []byte(`"auto"`)}}}, map[string]string{}, map[string]string{}, true),
		Entry("V-13: missing dependency", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "string", DependsOn: []string{"Q"}}, {Name: "Q", Type: "string"}}, map[string]string{"P": "x"}, map[string]string{}, false),
		Entry("V-13: present dependency", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "string", DependsOn: []string{"Q"}}, {Name: "Q", Type: "string"}}, map[string]string{"P": "x", "Q": "y"}, map[string]string{}, true),
		Entry("V-16: unchanged target is permitted", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "TARGET_RESOURCE_NAME", Type: "string"}}, map[string]string{"TARGET_RESOURCE_NAME": "pod-a"}, map[string]string{"TARGET_RESOURCE_NAME": "pod-a"}, true),
		Entry("V-17: independent pattern violation", []remediationworkflowv1.RemediationWorkflowParameter{{Name: "P", Type: "string", Pattern: "^[a-z]+$"}}, map[string]string{"P": "safe"}, map[string]string{"P": "FAST"}, false),
	)

	Describe("UT-AW-594-003: Approved + workflow switch → deny (#2570)", func() {
		BeforeEach(func() {
			rw := buildActiveRW("drain-restart", "kubernaut-system")
			reader = fake.NewClientBuilder().WithScheme(scheme).WithObjects(rw).
				WithStatusSubresource(rw).Build()
			handler = authwebhook.NewRemediationApprovalRequestAuthHandler(mockStore, reader, "kubernaut-system")
			Expect(handler.InjectDecoder(decoder)).To(Succeed())
		})

		It("should reject workflow switching even when the workflow is active", func() {
			rar := buildRAR(remediationv1.ApprovalDecisionApproved, &remediationv1.WorkflowOverride{
				WorkflowName: "drain-restart",
				Parameters:   typedParametersFromStrings(map[string]string{"TIMEOUT": "30s"}),
				Rationale:    "prefer safe restart",
			})

			resp := handler.Handle(ctx, buildAdmissionRequest(rar, nil))
			Expect(resp.Allowed).To(BeFalse())
			Expect(resp.Result.Message).To(ContainSubstring("workflow switching"))
		})
	})

	Describe("UT-AW-594-004: Approved + override + non-existent RW → deny", func() {
		BeforeEach(func() {
			reader = fake.NewClientBuilder().WithScheme(scheme).Build()
			handler = authwebhook.NewRemediationApprovalRequestAuthHandler(mockStore, reader, "kubernaut-system")
			Expect(handler.InjectDecoder(decoder)).To(Succeed())
		})

		It("should deny workflow switching before the catalog lookup", func() {
			rar := buildRAR(remediationv1.ApprovalDecisionApproved, &remediationv1.WorkflowOverride{
				WorkflowName: "nonexistent-workflow",
			})

			resp := handler.Handle(ctx, buildAdmissionRequest(rar, nil))
			Expect(resp.Allowed).To(BeFalse(), "webhook should deny override referencing non-existent RW")
			Expect(resp.Result).NotTo(BeNil())
			Expect(resp.Result.Message).To(ContainSubstring("workflow switching"))
		})
	})

	Describe("UT-AW-594-005: Rejected + override → deny", func() {
		BeforeEach(func() {
			reader = fake.NewClientBuilder().WithScheme(scheme).Build()
			handler = authwebhook.NewRemediationApprovalRequestAuthHandler(mockStore, reader, "kubernaut-system")
			Expect(handler.InjectDecoder(decoder)).To(Succeed())
		})

		It("should deny override when decision is not Approved", func() {
			rar := buildRAR(remediationv1.ApprovalDecisionRejected, &remediationv1.WorkflowOverride{
				WorkflowName: "drain-restart",
			})

			resp := handler.Handle(ctx, buildAdmissionRequest(rar, nil))
			Expect(resp.Allowed).To(BeFalse(), "webhook should deny override on Rejected decision")
			Expect(resp.Result).NotTo(BeNil())
			Expect(resp.Result.Message).To(ContainSubstring("Approved"))
		})
	})

	Describe("UT-AW-594-006: Approved + override + RW in Pending status → deny", func() {
		BeforeEach(func() {
			rw := buildActiveRW("pending-workflow", "kubernaut-system")
			rw.Status.CatalogStatus = sharedtypes.CatalogStatusPending

			reader = fake.NewClientBuilder().WithScheme(scheme).WithObjects(rw).
				WithStatusSubresource(rw).Build()
			handler = authwebhook.NewRemediationApprovalRequestAuthHandler(mockStore, reader, "kubernaut-system")
			Expect(handler.InjectDecoder(decoder)).To(Succeed())
		})

		It("should deny workflow switching without checking catalog status", func() {
			rar := buildRAR(remediationv1.ApprovalDecisionApproved, &remediationv1.WorkflowOverride{
				WorkflowName: "pending-workflow",
			})

			resp := handler.Handle(ctx, buildAdmissionRequest(rar, nil))
			Expect(resp.Allowed).To(BeFalse(), "webhook should deny override for non-Active RW")
			Expect(resp.Result).NotTo(BeNil())
			Expect(resp.Result.Message).To(ContainSubstring("workflow switching"))
		})
	})

	Describe("UT-AW-594-007: Approved + override (params only, no workflowName) → allow", func() {
		var workflowID string
		BeforeEach(func() {
			rw := buildActiveRW("trusted-timeout", "kubernaut-system")
			content, err := contenthash.MarshalCleanCRDContent(rw)
			Expect(err).NotTo(HaveOccurred())
			workflowID = contenthash.DeterministicUUID(contenthash.ComputeContentHash(string(content)))
			rw.Status.WorkflowID = workflowID
			reader = fake.NewClientBuilder().WithScheme(scheme).WithObjects(rw).WithStatusSubresource(rw).
				WithIndex(&remediationworkflowv1.RemediationWorkflow{}, "status.workflowId", func(obj client.Object) []string {
					return []string{obj.(*remediationworkflowv1.RemediationWorkflow).Status.WorkflowID}
				}).Build()
			handler = authwebhook.NewRemediationApprovalRequestAuthHandler(mockStore, reader, "kubernaut-system")
			Expect(handler.InjectDecoder(decoder)).To(Succeed())
		})

		It("should accept schema-valid changes to an existing parameter", func() {
			rar := buildRAR(remediationv1.ApprovalDecisionApproved, &remediationv1.WorkflowOverride{
				Parameters: typedParametersFromStrings(map[string]string{"TIMEOUT": "60s"}),
				Rationale:  "increase timeout for slow network",
			})
			rar.Spec.Workflow = &sharedtypes.WorkflowSnapshot{WorkflowID: workflowID}
			rar.Spec.Parameters = typedParametersFromStrings(map[string]string{"TIMEOUT": "30s"})
			rar.Spec.TargetContext = &remediationv1.ApprovalTargetContext{}

			resp := handler.Handle(ctx, buildAdmissionRequest(rar, nil))
			Expect(resp.Allowed).To(BeTrue())
		})
	})
})
