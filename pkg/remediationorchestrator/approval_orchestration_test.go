// Package remediationorchestrator contains unit tests for the RemediationOrchestrator service.
package remediationorchestrator_test

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
)

// BR-ORCH-026: Approval Orchestration
// Tests for ApprovalCreator and RemediationApprovalRequest handling
// Reference: ADR-040, DAYS_02_07_PHASE_HANDLERS.md
var _ = Describe("ApprovalOrchestration", func() {
	var (
		fakeClient client.Client
		scheme     *runtime.Scheme
		ac         *creator.ApprovalCreator
		ctx        context.Context
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())
		fakeClient = fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&remediationv1.RemediationApprovalRequest{}).Build()
		ac = creator.NewApprovalCreator(fakeClient, scheme, nil, creator.DefaultApprovalTimeout)
		ctx = context.Background()
	})

	Describe("Create RemediationApprovalRequest", func() {
		Context("BR-ORCH-026: Approval Request Creation", func() {
			var (
				rr *remediationv1.RemediationRequest
				ai *aianalysisv1.AIAnalysis
			)

			BeforeEach(func() {
				rr = &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-rr",
						Namespace: "default",
						UID:       types.UID("test-uid-123"),
					},
					Spec: remediationv1.RemediationRequestSpec{
						SignalName:        "TestAlert",
						SignalFingerprint: "fp12345678901234567890123456789012345678901234567890123456789012",
						Severity:          "critical",
						SignalType:        "alert",
						TargetType:        "kubernetes",
						TargetResource: remediationv1.ResourceIdentifier{
							Kind:      "Deployment",
							Name:      "my-app",
							Namespace: "default",
						},
					},
				}
				Expect(fakeClient.Create(ctx, rr)).To(Succeed())

				ai = &aianalysisv1.AIAnalysis{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ai-test-rr",
						Namespace: "default",
					},
					Status: aianalysisv1.AIAnalysisStatus{
						Phase: "Completed",
						RCAResult: &aianalysisv1.RCAResult{
							SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
								WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
									WorkflowID:      "wf-restart-pods",
									WorkflowName:    "wf-restart-pods",
									ActionType:      "RestartPod",
									Version:         "v1.0.0",
									ExecutionBundle: "kubernaut/workflows:latest",
								},
								Confidence: 0.75,
								Rationale:  "Pod restart recommended based on OOM patterns",
								Parameters: typedParametersFromStrings(map[string]string{}),
							},
							RootCause: "Memory leak causing OOM kills",
						},
						Approval: &aianalysisv1.ApprovalStatus{
							ApprovalReason: "Confidence between 60-79%",
						},
					},
				}
				Expect(fakeClient.Create(ctx, ai)).To(Succeed())
			})

			It("UT-AA-2570-067: BR-ORCH-025 rejects approval creation when original parameter data is missing", func() {
				ai.Status.RCAResult.SelectedWorkflow.Parameters = nil
				Expect(ai.Status.RCAResult.SelectedWorkflow.Parameters).To(BeNil())
				_, err := ac.Create(ctx, rr, ai)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("parameter"))
				rar := &remediationv1.RemediationApprovalRequest{}
				Expect(apierrors.IsNotFound(fakeClient.Get(ctx, client.ObjectKey{Name: "rar-test-rr", Namespace: rr.Namespace}, rar))).To(BeTrue())
			})

			It("UT-RO-2570-101: BR-ORCH-025 does not require persisted declaration metadata", func() {
				Expect(ai.Status.RCAResult.SelectedWorkflow.Parameters).NotTo(BeNil())
				encoded, err := json.Marshal(ai.Status.RCAResult.SelectedWorkflow)
				Expect(err).NotTo(HaveOccurred())
				Expect(encoded).NotTo(ContainSubstring("declaredParameterNames"))
				_, err = ac.Create(ctx, rr, ai)
				Expect(err).NotTo(HaveOccurred())
			})

			It("UT-RO-2570-114: BR-ORCH-025 does not require a persisted parameter schema", func() {
				_, err := ac.Create(ctx, rr, ai)
				Expect(err).NotTo(HaveOccurred())
				rar := &remediationv1.RemediationApprovalRequest{}
				Expect(fakeClient.Get(ctx, client.ObjectKey{Name: "rar-test-rr", Namespace: rr.Namespace}, rar)).To(Succeed())
				raw, err := json.Marshal(rar.Spec)
				Expect(err).NotTo(HaveOccurred())
				Expect(raw).NotTo(ContainSubstring("parameterSchema"))
			})

			It("UT-RO-2570-115: BR-ORCH-025 omits an explicitly empty parameter schema from persistence", func() {
				name, err := ac.Create(ctx, rr, ai)
				Expect(err).NotTo(HaveOccurred())
				rar := &remediationv1.RemediationApprovalRequest{}
				Expect(fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, rar)).To(Succeed())
				encoded, err := json.Marshal(rar.Spec)
				Expect(err).NotTo(HaveOccurred())
				var spec map[string]json.RawMessage
				Expect(json.Unmarshal(encoded, &spec)).To(Succeed())
				Expect(spec).NotTo(HaveKey("parameterSchema"))
			})

			It("UT-RO-2570-102: BR-ORCH-025 permits explicitly empty declarations after serialization", func() {
				selected := ai.Status.RCAResult.SelectedWorkflow
				encoded, err := json.Marshal(selected)
				Expect(err).NotTo(HaveOccurred())
				var restored aianalysisv1.SelectedWorkflow
				Expect(json.Unmarshal(encoded, &restored)).To(Succeed())
				Expect(encoded).NotTo(ContainSubstring("declaredParameterNames"))
				ai.Status.RCAResult.SelectedWorkflow = &restored
				_, err = ac.Create(ctx, rr, ai)
				Expect(err).NotTo(HaveOccurred())
			})

			It("UT-AA-2570-068: BR-ORCH-025 preserves an explicitly empty original parameter map on the wire", func() {
				selected := ai.Status.RCAResult.SelectedWorkflow
				selected.Parameters = typedParametersFromStrings(map[string]string{})
				encoded, err := json.Marshal(selected)
				Expect(err).NotTo(HaveOccurred())
				var restored aianalysisv1.SelectedWorkflow
				Expect(json.Unmarshal(encoded, &restored)).To(Succeed())
				Expect(restored.Parameters).NotTo(BeNil())
				Expect(restored.Parameters).To(BeEmpty())
			})

			It("UT-AA-2570-068: BR-ORCH-025 permits an explicitly empty original parameter map at creation", func() {
				ai.Status.RCAResult.SelectedWorkflow.Parameters = typedParametersFromStrings(map[string]string{})
				_, err := ac.Create(ctx, rr, ai)
				Expect(err).NotTo(HaveOccurred())
			})

			It("UT-RO-2570-076: BR-ORCH-025 freezes workflow identity and original parameter values in the RAR", func() {
				ai.Status.RCAResult.SelectedWorkflow.Parameters = typedParametersFromStrings(map[string]string{"REPLICAS": "3", "MODE": "safe"})
				name, err := ac.Create(ctx, rr, ai)
				Expect(err).NotTo(HaveOccurred())
				ai.Status.RCAResult.SelectedWorkflow.WorkflowID = "changed-after-creation"
				ai.Status.RCAResult.SelectedWorkflow.Parameters["REPLICAS"] = typedParameterValue("9")
				rar := &remediationv1.RemediationApprovalRequest{}
				Expect(fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, rar)).To(Succeed())
				raw, err := json.Marshal(rar.Spec)
				Expect(err).NotTo(HaveOccurred())
				var spec map[string]json.RawMessage
				Expect(json.Unmarshal(raw, &spec)).To(Succeed())
				Expect(spec).To(HaveKey("workflow"))
				Expect(spec).To(HaveKey("parameters"))
				var workflow struct {
					WorkflowID string `json:"workflowId"`
				}
				var params sharedtypes.TypedParameterMap
				Expect(json.Unmarshal(spec["workflow"], &workflow)).To(Succeed())
				Expect(json.Unmarshal(spec["parameters"], &params)).To(Succeed())
				Expect(workflow.WorkflowID).To(Equal("wf-restart-pods"))
				Expect(params).To(Equal(typedParametersFromStrings(map[string]string{"REPLICAS": "3", "MODE": "safe"})))
			})

			It("UT-RO-2570-109: BR-ORCH-025 does not persist the selected workflow parameter schema", func() {
				name, err := ac.Create(ctx, rr, ai)
				Expect(err).NotTo(HaveOccurred())
				rar := &remediationv1.RemediationApprovalRequest{}
				Expect(fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, rar)).To(Succeed())
				raw, err := json.Marshal(rar.Spec)
				Expect(err).NotTo(HaveOccurred())
				Expect(raw).NotTo(ContainSubstring("parameterSchema"))
			})

			It("UT-RO-2570-028: BR-ORCH-026 freezes remediation and signal target context", func() {
				rr.Spec.ClusterID = "cluster-a"
				rr.Spec.TargetResource = remediationv1.ResourceIdentifier{APIVersion: "v1", Kind: "Pod", Namespace: "ns", Name: "pod-a"}
				ai.Status.RCAResult.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
					RemediationTarget: &aianalysisv1.RemediationTarget{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "ns", Name: "app-a"},
				}
				name, err := ac.Create(ctx, rr, ai)
				Expect(err).NotTo(HaveOccurred())
				ai.Status.RCAResult.RootCauseAnalysis.RemediationTarget.Name = "changed"
				rr.Spec.TargetResource.Name = "changed"
				rar := &remediationv1.RemediationApprovalRequest{}
				Expect(fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, rar)).To(Succeed())
				raw, err := json.Marshal(rar.Spec)
				Expect(err).NotTo(HaveOccurred())
				var spec struct {
					TargetContext struct {
						RemediationTarget *remediationv1.ResourceIdentifier `json:"remediationTarget"`
						SignalTarget      remediationv1.ResourceIdentifier  `json:"signalTarget"`
						SignalClusterID   string                            `json:"signalClusterId"`
					} `json:"targetContext"`
				}
				Expect(json.Unmarshal(raw, &spec)).To(Succeed())
				Expect(spec.TargetContext.RemediationTarget).NotTo(BeNil())
				Expect(spec.TargetContext.RemediationTarget.Name).To(Equal("app-a"))
				Expect(spec.TargetContext.RemediationTarget.APIVersion).To(Equal("apps/v1"))
				Expect(spec.TargetContext.SignalTarget.Name).To(Equal("pod-a"))
				Expect(spec.TargetContext.SignalClusterID).To(Equal("cluster-a"))
			})

			It("UT-RO-2570-099: BR-ORCH-026 preserves an absent remediation target for later fallback", func() {
				name, err := ac.Create(ctx, rr, ai)
				Expect(err).NotTo(HaveOccurred())
				rar := &remediationv1.RemediationApprovalRequest{}
				Expect(fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, rar)).To(Succeed())
				Expect(rar.Spec.TargetContext).NotTo(BeNil())
				Expect(rar.Spec.TargetContext.RemediationTarget).To(BeNil())
				Expect(rar.Spec.TargetContext.SignalTarget).To(Equal(rr.Spec.TargetResource))
			})

			It("UT-RO-2570-100: BR-ORCH-026 preserves an invalid supplied target rather than inferring a replacement", func() {
				ai.Status.RCAResult.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
					RemediationTarget: &aianalysisv1.RemediationTarget{Kind: "Deployment", Name: "", Namespace: "ns"},
				}
				name, err := ac.Create(ctx, rr, ai)
				Expect(err).NotTo(HaveOccurred())
				rar := &remediationv1.RemediationApprovalRequest{}
				Expect(fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, rar)).To(Succeed())
				Expect(rar.Spec.TargetContext.RemediationTarget).NotTo(BeNil())
				Expect(rar.Spec.TargetContext.RemediationTarget.Kind).To(Equal("Deployment"))
				Expect(rar.Spec.TargetContext.RemediationTarget.Name).To(BeEmpty())
			})

			It("should generate deterministic name rar-{rr.Name}", func() {
				name, err := ac.Create(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())
				Expect(name).To(Equal("rar-test-rr"))
			})

			It("should create RemediationApprovalRequest with correct spec", func() {
				name, err := ac.Create(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				rar := &remediationv1.RemediationApprovalRequest{}
				Expect(fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: "default"}, rar)).To(Succeed())

				// Verify spec fields
				Expect(rar.Spec.RemediationRequestRef.Name).To(Equal("test-rr"))
				Expect(rar.Spec.Confidence).To(BeNumerically("==", 0.75))
				Expect(rar.Spec.RecommendedWorkflow.WorkflowID).To(Equal("wf-restart-pods"))
			})

			It("should set owner reference for cascade deletion (BR-ORCH-031)", func() {
				name, err := ac.Create(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				rar := &remediationv1.RemediationApprovalRequest{}
				Expect(fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: "default"}, rar)).To(Succeed())

				Expect(rar.OwnerReferences).To(HaveLen(1))
				Expect(rar.OwnerReferences[0].Name).To(Equal("test-rr"))
				Expect(rar.OwnerReferences[0].Kind).To(Equal("RemediationRequest"))
				Expect(rar.Finalizers).To(ContainElement(creator.ApprovalRequestFinalizerName),
					"UT-RO-2564-001: the RAR finalizer must be present before the create is persisted")
			})

			It("UT-RO-2564-001: fails closed on any deterministic RAR collision", func() {
				// First creation
				name1, err := ac.Create(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				// A retry after a successful create must not reuse the existing review.
				name2, err := ac.Create(ctx, rr, ai)
				Expect(name2).To(Equal(name1))
				Expect(err).To(HaveOccurred())

				var creationErr *creator.ApprovalCreationError
				Expect(errors.As(err, &creationErr)).To(BeTrue())
				Expect(creationErr.Outcome).To(Equal(creator.ApprovalCreationCollision))

				existing := &remediationv1.RemediationApprovalRequest{}
				Expect(fakeClient.Get(ctx, client.ObjectKey{Name: name1, Namespace: rr.Namespace}, existing)).To(Succeed())
				Expect(existing.Status.Decision).To(Equal(remediationv1.ApprovalDecisionPending))
				Expect(existing.Finalizers).To(ContainElement(creator.ApprovalRequestFinalizerName))
			})

			It("UT-RO-2564-001: fails closed when an existing RAR has a mismatched parent UID", func() {
				name, err := ac.Create(ctx, rr, ai)
				Expect(err).NotTo(HaveOccurred())

				foreign := &remediationv1.RemediationApprovalRequest{}
				Expect(fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, foreign)).To(Succeed())
				foreign.Spec.RemediationRequestRef.UID = types.UID("foreign-parent-uid")
				Expect(fakeClient.Update(ctx, foreign)).To(Succeed())

				_, err = ac.Create(ctx, rr, ai)
				Expect(err).To(HaveOccurred())

				var creationErr *creator.ApprovalCreationError
				Expect(errors.As(err, &creationErr)).To(BeTrue())
				Expect(creationErr.Outcome).To(Equal(creator.ApprovalCreationCollision))
			})

			It("should not set kubernaut.ai labels (Issue #91: parent tracked via spec + ownerRef)", func() {
				name, err := ac.Create(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				rar := &remediationv1.RemediationApprovalRequest{}
				Expect(fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: "default"}, rar)).To(Succeed())

				Expect(rar.Labels).To(BeNil())
				Expect(rar.Spec.RemediationRequestRef.Name).To(Equal("test-rr"))
			})

			It("should set RequiredBy deadline", func() {
				name, err := ac.Create(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				rar := &remediationv1.RemediationApprovalRequest{}
				Expect(fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: "default"}, rar)).To(Succeed())

				// RequiredBy should be in the future (1 hour default)
				Expect(rar.Spec.RequiredBy.Time).To(BeTemporally(">", time.Now()))
				Expect(rar.Spec.RequiredBy.Time).To(BeTemporally("<", time.Now().Add(2*time.Hour)))
			})

			It("UT-RAR-CA-001: should populate Status.CreatedAt on creation (Issue #118 Gap 10)", func() {
				beforeCreate := time.Now().Add(-1 * time.Second)
				name, err := ac.Create(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				rar := &remediationv1.RemediationApprovalRequest{}
				Expect(fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: "default"}, rar)).To(Succeed())

				Expect(rar.Status.CreatedAt).ToNot(BeNil(), "Status.CreatedAt must be populated for audit trail")
				Expect(rar.Status.CreatedAt.Time).To(BeTemporally(">=", beforeCreate))
				Expect(rar.Status.CreatedAt.Time).To(BeTemporally("<=", time.Now()))
			})
		})

		Context("Precondition Validation", func() {
			It("should return error when AIAnalysis is nil", func() {
				rr := &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-rr",
						Namespace: "default",
					},
				}
				_, err := ac.Create(ctx, rr, nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("AIAnalysis"))
			})

			It("should return error when SelectedWorkflow is nil", func() {
				rr := &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-rr",
						Namespace: "default",
					},
				}
				ai := &aianalysisv1.AIAnalysis{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ai-test-rr",
						Namespace: "default",
					},
					Status: aianalysisv1.AIAnalysisStatus{},
				}
				_, err := ac.Create(ctx, rr, ai)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("SelectedWorkflow"))
			})
		})
	})

	Describe("RemediationApprovalRequest Status Handling", func() {
		Context("Decision States", func() {
			BeforeEach(func() {
				// Recreate fakeClient with status subresource support for RAR
				fakeClient = fake.NewClientBuilder().
					WithScheme(scheme).
					WithStatusSubresource(&remediationv1.RemediationApprovalRequest{}).
					Build()
			})

			It("should have Pending decision by default", func() {
				rar := &remediationv1.RemediationApprovalRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rar-test-pending",
						Namespace: "default",
					},
					Spec: remediationv1.RemediationApprovalRequestSpec{
						RequiredBy:           metav1.NewTime(time.Now().Add(1 * time.Hour)),
						Confidence:           0.75,
						ConfidenceLevel:      "medium",
						Reason:               "Confidence below auto-approve threshold",
						InvestigationSummary: "Test investigation",
						WhyApprovalRequired:  "Confidence too low",
						RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{
							WorkflowID:      "wf-test",
							Version:         "v1.0.0",
							ExecutionBundle: "test:latest",
							Rationale:       "Test rationale",
						},
						RecommendedActions: []remediationv1.ApprovalRecommendedAction{
							{Action: "approve", Rationale: "test"},
						},
					},
				}
				Expect(fakeClient.Create(ctx, rar)).To(Succeed())

				fetched := &remediationv1.RemediationApprovalRequest{}
				Expect(fakeClient.Get(ctx, client.ObjectKey{Name: "rar-test-pending", Namespace: "default"}, fetched)).To(Succeed())
				Expect(fetched.Status.Decision).To(Equal(remediationv1.ApprovalDecision("")))
			})

			It("should allow status update to Approved", func() {
				rar := &remediationv1.RemediationApprovalRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rar-test-approved",
						Namespace: "default",
					},
					Spec: remediationv1.RemediationApprovalRequestSpec{
						RequiredBy:           metav1.NewTime(time.Now().Add(1 * time.Hour)),
						Confidence:           0.75,
						ConfidenceLevel:      "medium",
						Reason:               "Confidence below auto-approve threshold",
						InvestigationSummary: "Test investigation",
						WhyApprovalRequired:  "Confidence too low",
						RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{
							WorkflowID:      "wf-test",
							Version:         "v1.0.0",
							ExecutionBundle: "test:latest",
							Rationale:       "Test rationale",
						},
						RecommendedActions: []remediationv1.ApprovalRecommendedAction{
							{Action: "approve", Rationale: "test"},
						},
					},
				}
				Expect(fakeClient.Create(ctx, rar)).To(Succeed())

				rar.Status.Decision = remediationv1.ApprovalDecisionApproved
				rar.Status.DecidedBy = "admin@corp.com"
				now := metav1.Now()
				rar.Status.DecidedAt = &now
				Expect(fakeClient.Status().Update(ctx, rar)).To(Succeed())

				fetched := &remediationv1.RemediationApprovalRequest{}
				Expect(fakeClient.Get(ctx, client.ObjectKey{Name: "rar-test-approved", Namespace: "default"}, fetched)).To(Succeed())
				Expect(fetched.Status.Decision).To(Equal(remediationv1.ApprovalDecisionApproved))
				Expect(fetched.Status.DecidedBy).To(Equal("admin@corp.com"))
			})

			It("should allow status update to Rejected", func() {
				rar := &remediationv1.RemediationApprovalRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rar-test-rejected",
						Namespace: "default",
					},
					Spec: remediationv1.RemediationApprovalRequestSpec{
						RequiredBy:           metav1.NewTime(time.Now().Add(1 * time.Hour)),
						Confidence:           0.75,
						ConfidenceLevel:      "medium",
						Reason:               "Confidence below auto-approve threshold",
						InvestigationSummary: "Test investigation",
						WhyApprovalRequired:  "Confidence too low",
						RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{
							WorkflowID:      "wf-test",
							Version:         "v1.0.0",
							ExecutionBundle: "test:latest",
							Rationale:       "Test rationale",
						},
						RecommendedActions: []remediationv1.ApprovalRecommendedAction{
							{Action: "approve", Rationale: "test"},
						},
					},
				}
				Expect(fakeClient.Create(ctx, rar)).To(Succeed())

				rar.Status.Decision = remediationv1.ApprovalDecisionRejected
				rar.Status.DecidedBy = "security@corp.com"
				rar.Status.DecisionMessage = "Too risky for production"
				now := metav1.Now()
				rar.Status.DecidedAt = &now
				Expect(fakeClient.Status().Update(ctx, rar)).To(Succeed())

				fetched := &remediationv1.RemediationApprovalRequest{}
				Expect(fakeClient.Get(ctx, client.ObjectKey{Name: "rar-test-rejected", Namespace: "default"}, fetched)).To(Succeed())
				Expect(fetched.Status.Decision).To(Equal(remediationv1.ApprovalDecisionRejected))
				Expect(fetched.Status.DecisionMessage).To(Equal("Too risky for production"))
			})

			It("should allow status update to Expired", func() {
				rar := &remediationv1.RemediationApprovalRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rar-test-expired",
						Namespace: "default",
					},
					Spec: remediationv1.RemediationApprovalRequestSpec{
						RequiredBy:           metav1.NewTime(time.Now().Add(1 * time.Hour)),
						Confidence:           0.75,
						ConfidenceLevel:      "medium",
						Reason:               "Confidence below auto-approve threshold",
						InvestigationSummary: "Test investigation",
						WhyApprovalRequired:  "Confidence too low",
						RecommendedWorkflow: remediationv1.RecommendedWorkflowSummary{
							WorkflowID:      "wf-test",
							Version:         "v1.0.0",
							ExecutionBundle: "test:latest",
							Rationale:       "Test rationale",
						},
						RecommendedActions: []remediationv1.ApprovalRecommendedAction{
							{Action: "approve", Rationale: "test"},
						},
					},
				}
				Expect(fakeClient.Create(ctx, rar)).To(Succeed())

				rar.Status.Decision = remediationv1.ApprovalDecisionExpired
				rar.Status.DecidedBy = "system"
				now := metav1.Now()
				rar.Status.DecidedAt = &now
				Expect(fakeClient.Status().Update(ctx, rar)).To(Succeed())

				fetched := &remediationv1.RemediationApprovalRequest{}
				Expect(fakeClient.Get(ctx, client.ObjectKey{Name: "rar-test-expired", Namespace: "default"}, fetched)).To(Succeed())
				Expect(fetched.Status.Decision).To(Equal(remediationv1.ApprovalDecisionExpired))
			})
		})
	})

	Describe("Issue #206: Approval reason propagation", func() {
		var (
			rr *remediationv1.RemediationRequest
		)

		BeforeEach(func() {
			rr = &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-rr-206",
					Namespace: "default",
					UID:       types.UID("uid-206"),
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalName:        "TestAlert",
					SignalFingerprint: "fp12345678901234567890123456789012345678901234567890123456789012",
					Severity:          "critical",
					SignalType:        "alert",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Deployment",
						Name:      "payment-api",
						Namespace: "default",
					},
				},
			}
			Expect(fakeClient.Create(ctx, rr)).To(Succeed())
		})

		It("UT-RAR-206-001: WhyApprovalRequired should use actual Rego reason, not hardcoded 'below threshold'", func() {
			zero := 0.0
			ai := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ai-test-rr-206-001",
					Namespace: "default",
				},
				Status: aianalysisv1.AIAnalysisStatus{
					Phase: "Completed",
					RCAResult: &aianalysisv1.RCAResult{
						RCAConfidence: &zero,
						SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
							WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
								WorkflowID:      "wf-restart-pods",
								WorkflowName:    "wf-restart-pods",
								ActionType:      "RestartPod",
								Version:         "v1.0.0",
								ExecutionBundle: "kubernaut/workflows:latest",
							},
							Confidence: 0.85,
							Rationale:  "Pod restart recommended",
							Parameters: typedParametersFromStrings(map[string]string{}),
						},
					},
					Approval: &aianalysisv1.ApprovalStatus{
						ApprovalRequired: true,
						ApprovalReason:   "Missing remediation target - cannot determine resource to remediate (BR-AI-085-005)",
					},
				},
			}
			Expect(fakeClient.Create(ctx, ai)).To(Succeed())

			name, err := ac.Create(ctx, rr, ai)
			Expect(err).ToNot(HaveOccurred())

			rar := &remediationv1.RemediationApprovalRequest{}
			Expect(fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: "default"}, rar)).To(Succeed())

			Expect(rar.Spec.WhyApprovalRequired).To(ContainSubstring("Missing remediation target"))
			Expect(rar.Spec.RCAConfidence).To(HaveValue(Equal(0.0)))
			Expect(rar.Spec.Confidence).To(Equal(.85))
			Expect(rar.Spec.WhyApprovalRequired).ToNot(ContainSubstring("is below"))
		})

		It("UT-RAR-206-002: RecommendedActions rationale should use actual Rego reason", func() {
			ai := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ai-test-rr-206-002",
					Namespace: "default",
				},
				Status: aianalysisv1.AIAnalysisStatus{
					Phase: "Completed",
					RCAResult: &aianalysisv1.RCAResult{
						SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
							WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
								WorkflowID:      "wf-restart-pods",
								WorkflowName:    "wf-restart-pods",
								ActionType:      "RestartPod",
								Version:         "v1.0.0",
								ExecutionBundle: "kubernaut/workflows:latest",
							},
							Confidence: 0.90,
							Rationale:  "Pod restart recommended",
							Parameters: typedParametersFromStrings(map[string]string{}),
						},
					},
					Approval: &aianalysisv1.ApprovalStatus{
						ApprovalRequired: true,
						ApprovalReason:   "Production environment with sensitive resource kind - requires manual approval",
					},
				},
			}
			Expect(fakeClient.Create(ctx, ai)).To(Succeed())

			name, err := ac.Create(ctx, rr, ai)
			Expect(err).ToNot(HaveOccurred())

			rar := &remediationv1.RemediationApprovalRequest{}
			Expect(fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: "default"}, rar)).To(Succeed())

			Expect(rar.Spec.RecommendedActions).To(HaveLen(1))
			Expect(rar.Spec.RecommendedActions[0].Rationale).To(ContainSubstring("sensitive resource"))
			Expect(rar.Spec.RecommendedActions[0].Rationale).ToNot(ContainSubstring("is below auto-approval threshold"))
		})

		It("UT-RAR-206-003: Reason field should match ApprovalReason from AIAnalysis", func() {
			ai := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ai-test-rr-206-003",
					Namespace: "default",
				},
				Status: aianalysisv1.AIAnalysisStatus{
					Phase: "Completed",
					RCAResult: &aianalysisv1.RCAResult{
						SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
							WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
								WorkflowID:      "wf-scale-hpa",
								WorkflowName:    "wf-scale-hpa",
								ActionType:      "RestartPod",
								Version:         "v1.0.0",
								ExecutionBundle: "kubernaut/workflows:latest",
							},
							Confidence: 0.65,
							Rationale:  "HPA max replicas reached",
							Parameters: typedParametersFromStrings(map[string]string{}),
						},
					},
					Approval: &aianalysisv1.ApprovalStatus{
						ApprovalRequired: true,
						ApprovalReason:   "Production environment requires manual approval",
					},
				},
			}
			Expect(fakeClient.Create(ctx, ai)).To(Succeed())

			name, err := ac.Create(ctx, rr, ai)
			Expect(err).ToNot(HaveOccurred())

			rar := &remediationv1.RemediationApprovalRequest{}
			Expect(fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: "default"}, rar)).To(Succeed())

			Expect(rar.Spec.Reason).To(Equal("Production environment requires manual approval"))
			Expect(rar.Spec.WhyApprovalRequired).To(ContainSubstring("Production environment requires manual approval"))
		})
	})

	// ========================================
	// BR-AI-059, BR-AI-076, BR-ORCH-026: ApprovalContext field mapping (#307)
	// ========================================
	Describe("ApprovalContext field mapping to RAR spec (#307)", func() {
		var (
			rr *remediationv1.RemediationRequest
		)

		BeforeEach(func() {
			rr = &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-rr-307",
					Namespace: "default",
					UID:       types.UID("uid-307"),
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalName:        "OOMKilled",
					SignalFingerprint: "fp12345678901234567890123456789012345678901234567890123456789012",
					Severity:          "critical",
					SignalType:        "alert",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Deployment",
						Name:      "payment-api",
						Namespace: "default",
					},
				},
			}
			Expect(fakeClient.Create(ctx, rr)).To(Succeed())
		})

		It("UT-RAR-307-001: should map EvidenceCollected from ApprovalContext to RAR spec", func() {
			ai := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ai-test-rr-307-001",
					Namespace: "default",
				},
				Status: aianalysisv1.AIAnalysisStatus{
					Phase: "Completed",
					RCAResult: &aianalysisv1.RCAResult{
						SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
							WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
								WorkflowID:      "wf-restart-pods",
								WorkflowName:    "wf-restart-pods",
								ActionType:      "RestartPod",
								Version:         "v1.0.0",
								ExecutionBundle: "kubernaut/workflows:latest",
							},
							Confidence: 0.72,
							Rationale:  "Pod restart recommended",
							Parameters: typedParametersFromStrings(map[string]string{}),
						},
					},
					Approval: &aianalysisv1.ApprovalStatus{
						ApprovalReason: "Confidence below auto-approve threshold",
						ApprovalContext: &aianalysisv1.ApprovalContext{
							EvidenceCollected: []string{
								"OOM killed event detected at 14:32 UTC",
								"Memory usage exceeded 95% of limit",
								"Container restart count: 3 in last 10 minutes",
							},
						},
					},
				},
			}
			Expect(fakeClient.Create(ctx, ai)).To(Succeed())

			name, err := ac.Create(ctx, rr, ai)
			Expect(err).ToNot(HaveOccurred())

			rar := &remediationv1.RemediationApprovalRequest{}
			Expect(fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: "default"}, rar)).To(Succeed())

			Expect(rar.Spec.EvidenceCollected).To(HaveLen(3),
				"BR-AI-076: EvidenceCollected must be mapped from ApprovalContext")
			Expect(rar.Spec.EvidenceCollected).To(ContainElement("OOM killed event detected at 14:32 UTC"))
		})

		It("UT-RAR-307-002: should map AlternativesConsidered from ApprovalContext to RAR spec", func() {
			ai := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ai-test-rr-307-002",
					Namespace: "default",
				},
				Status: aianalysisv1.AIAnalysisStatus{
					Phase: "Completed",
					RCAResult: &aianalysisv1.RCAResult{
						SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
							WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
								WorkflowID:      "wf-restart-pods",
								WorkflowName:    "wf-restart-pods",
								ActionType:      "RestartPod",
								Version:         "v1.0.0",
								ExecutionBundle: "kubernaut/workflows:latest",
							},
							Confidence: 0.72,
							Rationale:  "Pod restart recommended",
							Parameters: typedParametersFromStrings(map[string]string{}),
						},
					},
					Approval: &aianalysisv1.ApprovalStatus{
						ApprovalReason: "Confidence below auto-approve threshold",
						ApprovalContext: &aianalysisv1.ApprovalContext{
							AlternativesConsidered: []aianalysisv1.AlternativeApproach{
								{
									Approach: "wf-scale-up",
									ProsCons: "Pros: addresses root cause. Cons: higher resource cost",
								},
								{
									Approach: "wf-rollback",
									ProsCons: "Pros: quick recovery. Cons: loses recent changes",
								},
							},
						},
					},
				},
			}
			Expect(fakeClient.Create(ctx, ai)).To(Succeed())

			name, err := ac.Create(ctx, rr, ai)
			Expect(err).ToNot(HaveOccurred())

			rar := &remediationv1.RemediationApprovalRequest{}
			Expect(fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: "default"}, rar)).To(Succeed())

			Expect(rar.Spec.AlternativesConsidered).To(HaveLen(2),
				"BR-AI-076: AlternativesConsidered must be mapped from ApprovalContext")
			Expect(rar.Spec.AlternativesConsidered[0].Approach).To(Equal("wf-scale-up"))
			Expect(rar.Spec.AlternativesConsidered[0].ProsCons).To(ContainSubstring("higher resource cost"))
			Expect(rar.Spec.AlternativesConsidered[1].Approach).To(Equal("wf-rollback"))
		})

		It("UT-RAR-307-003: should map PolicyEvaluation from ApprovalContext to RAR spec", func() {
			ai := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ai-test-rr-307-003",
					Namespace: "default",
				},
				Status: aianalysisv1.AIAnalysisStatus{
					Phase: "Completed",
					RCAResult: &aianalysisv1.RCAResult{
						SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
							WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
								WorkflowID:      "wf-restart-pods",
								WorkflowName:    "wf-restart-pods",
								ActionType:      "RestartPod",
								Version:         "v1.0.0",
								ExecutionBundle: "kubernaut/workflows:latest",
							},
							Confidence: 0.85,
							Rationale:  "Pod restart recommended",
							Parameters: typedParametersFromStrings(map[string]string{}),
						},
					},
					Approval: &aianalysisv1.ApprovalStatus{
						ApprovalReason: "Production environment requires manual approval",
						ApprovalContext: &aianalysisv1.ApprovalContext{
							PolicyEvaluation: &aianalysisv1.PolicyEvaluation{
								PolicyName:   "production-approval-policy",
								MatchedRules: []string{"require_approval_for_production", "sensitive_namespace"},
								Decision:     aianalysisv1.PolicyDecisionManualReviewRequired,
							},
						},
					},
				},
			}
			Expect(fakeClient.Create(ctx, ai)).To(Succeed())

			name, err := ac.Create(ctx, rr, ai)
			Expect(err).ToNot(HaveOccurred())

			rar := &remediationv1.RemediationApprovalRequest{}
			Expect(fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: "default"}, rar)).To(Succeed())

			Expect(rar.Spec.PolicyEvaluation).ToNot(BeNil(),
				"BR-AI-059: PolicyEvaluation must be mapped from ApprovalContext")
			Expect(rar.Spec.PolicyEvaluation.PolicyName).To(Equal("production-approval-policy"))
			Expect(rar.Spec.PolicyEvaluation.MatchedRules).To(ConsistOf("require_approval_for_production", "sensitive_namespace"))
			Expect(rar.Spec.PolicyEvaluation.Decision).To(Equal(string(aianalysisv1.PolicyDecisionManualReviewRequired)))
		})

		// BR-AUDIT-006, BR-AI-076, Issue #1981: PolicyHash must reach RAR spec so an approval
		// decision can be attributed, after the fact, to the exact policy bundle revision that
		// produced it (tamper-evident approval decisions).
		It("UT-RAR-1981-001: should map PolicyHash from ApprovalContext to RAR spec", func() {
			const expectedHash = "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f9"
			ai := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ai-test-rr-1981-001",
					Namespace: "default",
				},
				Status: aianalysisv1.AIAnalysisStatus{
					Phase: "Completed",
					RCAResult: &aianalysisv1.RCAResult{
						SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
							WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
								WorkflowID:      "wf-restart-pods",
								WorkflowName:    "wf-restart-pods",
								ActionType:      "RestartPod",
								Version:         "v1.0.0",
								ExecutionBundle: "kubernaut/workflows:latest",
							},
							Confidence: 0.85,
							Rationale:  "Pod restart recommended",
							Parameters: typedParametersFromStrings(map[string]string{}),
						},
					},
					Approval: &aianalysisv1.ApprovalStatus{
						ApprovalReason: "Production environment requires manual approval",
						ApprovalContext: &aianalysisv1.ApprovalContext{
							PolicyEvaluation: &aianalysisv1.PolicyEvaluation{
								PolicyName:   "production-approval-policy",
								MatchedRules: []string{"require_approval_for_production"},
								Decision:     aianalysisv1.PolicyDecisionManualReviewRequired,
								PolicyHash:   expectedHash,
							},
						},
					},
				},
			}
			Expect(fakeClient.Create(ctx, ai)).To(Succeed())

			name, err := ac.Create(ctx, rr, ai)
			Expect(err).ToNot(HaveOccurred())

			rar := &remediationv1.RemediationApprovalRequest{}
			Expect(fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: "default"}, rar)).To(Succeed())

			Expect(rar.Spec.PolicyEvaluation).ToNot(BeNil(),
				"BR-AI-059: PolicyEvaluation must be mapped from ApprovalContext")
			Expect(rar.Spec.PolicyEvaluation.PolicyHash).To(Equal(expectedHash),
				"BR-AUDIT-006, Issue #1981: PolicyHash must be pinned onto the immutable RAR spec for tamper-evident attribution")
		})

		It("UT-RAR-307-004: should handle nil ApprovalContext gracefully", func() {
			ai := &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ai-test-rr-307-004",
					Namespace: "default",
				},
				Status: aianalysisv1.AIAnalysisStatus{
					Phase: "Completed",
					RCAResult: &aianalysisv1.RCAResult{
						SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
							WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
								WorkflowID:      "wf-restart-pods",
								WorkflowName:    "wf-restart-pods",
								ActionType:      "RestartPod",
								Version:         "v1.0.0",
								ExecutionBundle: "kubernaut/workflows:latest",
							},
							Confidence: 0.72,
							Rationale:  "Pod restart recommended",
							Parameters: typedParametersFromStrings(map[string]string{}),
						},
					},
					Approval: &aianalysisv1.ApprovalStatus{
						ApprovalReason:  "Confidence below auto-approve threshold",
						ApprovalContext: nil,
					},
				},
			}
			Expect(fakeClient.Create(ctx, ai)).To(Succeed())

			name, err := ac.Create(ctx, rr, ai)
			Expect(err).ToNot(HaveOccurred())

			rar := &remediationv1.RemediationApprovalRequest{}
			Expect(fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: "default"}, rar)).To(Succeed())

			Expect(rar.Spec.EvidenceCollected).To(BeNil())
			Expect(rar.Spec.AlternativesConsidered).To(BeNil())
			Expect(rar.Spec.PolicyEvaluation).To(BeNil())
		})
	})

	Describe("Confidence Level Mapping", func() {
		DescribeTable("should map confidence score to level",
			func(confidence float64, expectedLevel string) {
				var level string
				if confidence < 0.6 {
					level = "low"
				} else if confidence < 0.8 {
					level = "medium"
				} else {
					level = "high"
				}
				Expect(level).To(Equal(expectedLevel))
			},
			Entry("0.5 is low", 0.5, "low"),
			Entry("0.6 is medium", 0.6, "medium"),
			Entry("0.75 is medium", 0.75, "medium"),
			Entry("0.8 is high", 0.8, "high"),
			Entry("0.95 is high", 0.95, "high"),
		)
	})
})
