// Package remediationorchestrator contains unit tests for the RemediationOrchestrator service.
package remediationorchestrator

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
		fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
		ac = creator.NewApprovalCreator(fakeClient, scheme, nil)
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
						SignalType:        "prometheus",
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
						SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
							WorkflowID:     "wf-restart-pods",
							Version:        "v1.0.0",
							Confidence:     0.75,
							ExecutionBundle: "kubernaut/workflows:latest",
							Rationale:      "Pod restart recommended based on OOM patterns",
						},
						ApprovalReason: "Confidence between 60-79%",
						RootCause:      "Memory leak causing OOM kills",
					},
				}
				Expect(fakeClient.Create(ctx, ai)).To(Succeed())
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
			})

			It("should be idempotent - return existing name without error", func() {
				// First creation
				name1, err := ac.Create(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())

				// Second creation - should return same name
				name2, err := ac.Create(ctx, rr, ai)
				Expect(err).ToNot(HaveOccurred())
				Expect(name2).To(Equal(name1))
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
					Status: aianalysisv1.AIAnalysisStatus{
						SelectedWorkflow: nil,
					},
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
							WorkflowID:     "wf-test",
							Version:        "v1.0.0",
							ExecutionBundle: "test:latest",
							Rationale:      "Test rationale",
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
							WorkflowID:     "wf-test",
							Version:        "v1.0.0",
							ExecutionBundle: "test:latest",
							Rationale:      "Test rationale",
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
							WorkflowID:     "wf-test",
							Version:        "v1.0.0",
							ExecutionBundle: "test:latest",
							Rationale:      "Test rationale",
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
							WorkflowID:     "wf-test",
							Version:        "v1.0.0",
							ExecutionBundle: "test:latest",
							Rationale:      "Test rationale",
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
