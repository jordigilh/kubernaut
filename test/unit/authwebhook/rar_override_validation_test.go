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

package authwebhook

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	remediationworkflowv1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	admissionv1 "k8s.io/api/admission/v1"
	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
			req.AdmissionRequest.OldObject = runtime.RawExtension{Raw: rawOld}
		} else {
			emptyRAR := &remediationv1.RemediationApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{Name: rar.Name, Namespace: rar.Namespace},
			}
			rawOld, err := json.Marshal(emptyRAR)
			Expect(err).NotTo(HaveOccurred())
			req.AdmissionRequest.OldObject = runtime.RawExtension{Raw: rawOld}
		}

		return req
	}

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
				RequiredBy:           metav1.Now(),
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
					Component:   "Node",
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

	BeforeEach(func() {
		ctx = context.Background()
		mockStore = &MockAuditStore{}

		scheme = runtime.NewScheme()
		_ = remediationv1.AddToScheme(scheme)
		_ = remediationworkflowv1.AddToScheme(scheme)

		decoder = admission.NewDecoder(scheme)
	})

	Describe("UT-AW-594-003: Approved + override + valid Active RW → allow", func() {
		BeforeEach(func() {
			rw := buildActiveRW("drain-restart", "kubernaut-system")
			reader = fake.NewClientBuilder().WithScheme(scheme).WithObjects(rw).
				WithStatusSubresource(rw).Build()
			handler = authwebhook.NewRemediationApprovalRequestAuthHandler(mockStore, reader)
			Expect(handler.InjectDecoder(decoder)).To(Succeed())
		})

		It("should allow the admission request and preserve override data", func() {
			rar := buildRAR(remediationv1.ApprovalDecisionApproved, &remediationv1.WorkflowOverride{
				WorkflowName: "drain-restart",
				Parameters:   map[string]string{"TIMEOUT": "30s"},
				Rationale:    "prefer safe restart",
			})

			resp := handler.Handle(ctx, buildAdmissionRequest(rar, nil))
			Expect(resp.Allowed).To(BeTrue(), "webhook should allow approved override with valid Active RW")
		})
	})

	Describe("UT-AW-594-004: Approved + override + non-existent RW → deny", func() {
		BeforeEach(func() {
			reader = fake.NewClientBuilder().WithScheme(scheme).Build()
			handler = authwebhook.NewRemediationApprovalRequestAuthHandler(mockStore, reader)
			Expect(handler.InjectDecoder(decoder)).To(Succeed())
		})

		It("should deny the admission request with 'not found' message", func() {
			rar := buildRAR(remediationv1.ApprovalDecisionApproved, &remediationv1.WorkflowOverride{
				WorkflowName: "nonexistent-workflow",
			})

			resp := handler.Handle(ctx, buildAdmissionRequest(rar, nil))
			Expect(resp.Allowed).To(BeFalse(), "webhook should deny override referencing non-existent RW")
			Expect(resp.Result).NotTo(BeNil())
			Expect(resp.Result.Message).To(ContainSubstring("not found"))
		})
	})

	Describe("UT-AW-594-005: Rejected + override → deny", func() {
		BeforeEach(func() {
			reader = fake.NewClientBuilder().WithScheme(scheme).Build()
			handler = authwebhook.NewRemediationApprovalRequestAuthHandler(mockStore, reader)
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
			handler = authwebhook.NewRemediationApprovalRequestAuthHandler(mockStore, reader)
			Expect(handler.InjectDecoder(decoder)).To(Succeed())
		})

		It("should deny override when RW is not in Active status", func() {
			rar := buildRAR(remediationv1.ApprovalDecisionApproved, &remediationv1.WorkflowOverride{
				WorkflowName: "pending-workflow",
			})

			resp := handler.Handle(ctx, buildAdmissionRequest(rar, nil))
			Expect(resp.Allowed).To(BeFalse(), "webhook should deny override for non-Active RW")
			Expect(resp.Result).NotTo(BeNil())
			Expect(resp.Result.Message).To(ContainSubstring("Active"))
		})
	})

	Describe("UT-AW-594-007: Approved + override (params only, no workflowName) → allow", func() {
		BeforeEach(func() {
			reader = fake.NewClientBuilder().WithScheme(scheme).Build()
			handler = authwebhook.NewRemediationApprovalRequestAuthHandler(mockStore, reader)
			Expect(handler.InjectDecoder(decoder)).To(Succeed())
		})

		It("should allow params-only override without RW lookup", func() {
			rar := buildRAR(remediationv1.ApprovalDecisionApproved, &remediationv1.WorkflowOverride{
				Parameters: map[string]string{"TIMEOUT": "60s"},
				Rationale:  "increase timeout for slow network",
			})

			resp := handler.Handle(ctx, buildAdmissionRequest(rar, nil))
			Expect(resp.Allowed).To(BeTrue(), "webhook should allow params-only override without workflowName")
		})
	})
})
