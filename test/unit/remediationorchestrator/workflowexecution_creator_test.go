/*
Copyright 2025 Jordi Gil.

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

// Package remediationorchestrator contains unit tests for the Remediation Orchestrator.
package remediationorchestrator

import (
	"context"
	"errors"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

var _ = Describe("WorkflowExecutionCreator", func() {
	var (
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		_ = remediationv1.AddToScheme(scheme)
		_ = aianalysisv1.AddToScheme(scheme)
		_ = workflowexecutionv1.AddToScheme(scheme)
	})

	Describe("NewWorkflowExecutionCreator", func() {
		It("should return a non-nil creator to enable BR-ORCH-025 workflow pass-through", func() {
			// Arrange
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

			// Act
			weCreator := creator.NewWorkflowExecutionCreator(fakeClient, scheme, nil)

			// Assert
			Expect(weCreator).ToNot(BeNil())
		})
	})

	Describe("Create", func() {
		It("should generate deterministic name 'we-{rr.Name}' per BR-ORCH-025 pass-through pattern", func() {
			// Arrange
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			weCreator := creator.NewWorkflowExecutionCreator(fakeClient, scheme, nil)
			rr := helpers.NewRemediationRequest("test-remediation", "default")
			ai := helpers.NewCompletedAIAnalysis("ai-test-remediation", "default")
			ctx := context.Background()

			// Act
			name, err := weCreator.Create(ctx, rr, ai)

			// Assert
			Expect(err).ToNot(HaveOccurred())
			Expect(name).To(Equal("we-test-remediation"))
		})

		It("should set owner reference for cascade deletion per BR-ORCH-031", func() {
			// Arrange
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			weCreator := creator.NewWorkflowExecutionCreator(fakeClient, scheme, nil)
			rr := helpers.NewRemediationRequest("test-remediation", "default")
			ai := helpers.NewCompletedAIAnalysis("ai-test-remediation", "default")
			ctx := context.Background()

			// Act
			name, err := weCreator.Create(ctx, rr, ai)

			// Assert
			Expect(err).ToNot(HaveOccurred())

			// Verify owner reference is set
			created := &workflowexecutionv1.WorkflowExecution{}
			err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, created)
			Expect(err).ToNot(HaveOccurred())
			Expect(created.OwnerReferences).To(HaveLen(1))
			Expect(created.OwnerReferences[0].Name).To(Equal(rr.Name))
			Expect(created.OwnerReferences[0].Kind).To(Equal("RemediationRequest"))
		})

		It("should return existing name when WorkflowExecution already exists per BR-ORCH-025 idempotency", func() {
			// Arrange
			rr := helpers.NewRemediationRequest("test-remediation", "default")
			ai := helpers.NewCompletedAIAnalysis("ai-test-remediation", "default")
			existingWE := helpers.NewWorkflowExecution("we-test-remediation", "default")
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingWE).Build()
			weCreator := creator.NewWorkflowExecutionCreator(fakeClient, scheme, nil)
			ctx := context.Background()

			// Act
			name, err := weCreator.Create(ctx, rr, ai)

			// Assert
			Expect(err).ToNot(HaveOccurred())
			Expect(name).To(Equal("we-test-remediation"))
		})

		It("should build WorkflowExecution spec with all required fields per BR-ORCH-025", func() {
			// Arrange
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			weCreator := creator.NewWorkflowExecutionCreator(fakeClient, scheme, nil)
			rr := helpers.NewRemediationRequest("test-remediation", "default")
			ai := helpers.NewCompletedAIAnalysis("ai-test-remediation", "default")
			ctx := context.Background()

			// Act
			name, err := weCreator.Create(ctx, rr, ai)

			// Assert
			Expect(err).ToNot(HaveOccurred())

			created := &workflowexecutionv1.WorkflowExecution{}
			err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, created)
			Expect(err).ToNot(HaveOccurred())

			// Verify RemediationRequestRef
			Expect(created.Spec.RemediationRequestRef.Name).To(Equal(rr.Name))
			Expect(created.Spec.RemediationRequestRef.Namespace).To(Equal(rr.Namespace))
			Expect(created.Spec.RemediationRequestRef.Kind).To(Equal("RemediationRequest"))

			// Verify WorkflowRef pass-through from AIAnalysis
			Expect(created.Spec.WorkflowRef.WorkflowID).To(Equal(ai.Status.SelectedWorkflow.WorkflowID))
			Expect(created.Spec.WorkflowRef.Version).To(Equal(ai.Status.SelectedWorkflow.Version))
			Expect(created.Spec.WorkflowRef.ExecutionBundle).To(Equal(ai.Status.SelectedWorkflow.ExecutionBundle))

			// Verify audit fields
			Expect(created.Spec.Confidence).To(Equal(ai.Status.SelectedWorkflow.Confidence))
			Expect(created.Spec.Rationale).To(Equal(ai.Status.SelectedWorkflow.Rationale))

			// Issue #91: labels removed; parent tracked via spec.remediationRequestRef + ownerRef
			Expect(created.Labels).To(BeNil())
			Expect(created.Spec.RemediationRequestRef.Name).To(Equal(rr.Name))
		})
	})

	Describe("ExecutionEngine pass-through", func() {
		// BR-WE-014: ExecutionEngine must be derived from AIAnalysis.Status.SelectedWorkflow,
		// NOT hardcoded. This ensures the workflow catalog controls the execution backend.
		It("should use executionEngine from AIAnalysis SelectedWorkflow when set to 'job'", func() {
			// Arrange
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			weCreator := creator.NewWorkflowExecutionCreator(fakeClient, scheme, nil)
			rr := helpers.NewRemediationRequest("test-engine-job", "default")
			ai := helpers.NewCompletedAIAnalysis("ai-test-engine-job", "default")
			ai.Status.SelectedWorkflow.ExecutionEngine = "job"
			ctx := context.Background()

			// Act
			name, err := weCreator.Create(ctx, rr, ai)

			// Assert
			Expect(err).ToNot(HaveOccurred())
			created := &workflowexecutionv1.WorkflowExecution{}
			err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, created)
			Expect(err).ToNot(HaveOccurred())
			Expect(created.Spec.ExecutionEngine).To(Equal("job"))
		})

		It("should use executionEngine from AIAnalysis SelectedWorkflow when set to 'tekton'", func() {
			// Arrange
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			weCreator := creator.NewWorkflowExecutionCreator(fakeClient, scheme, nil)
			rr := helpers.NewRemediationRequest("test-engine-tekton", "default")
			ai := helpers.NewCompletedAIAnalysis("ai-test-engine-tekton", "default")
			ai.Status.SelectedWorkflow.ExecutionEngine = "tekton"
			ctx := context.Background()

			// Act
			name, err := weCreator.Create(ctx, rr, ai)

			// Assert
			Expect(err).ToNot(HaveOccurred())
			created := &workflowexecutionv1.WorkflowExecution{}
			err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, created)
			Expect(err).ToNot(HaveOccurred())
			Expect(created.Spec.ExecutionEngine).To(Equal("tekton"))
		})

		It("should default to 'tekton' when executionEngine is empty (backwards compatibility)", func() {
			// Arrange
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			weCreator := creator.NewWorkflowExecutionCreator(fakeClient, scheme, nil)
			rr := helpers.NewRemediationRequest("test-engine-default", "default")
			ai := helpers.NewCompletedAIAnalysis("ai-test-engine-default", "default")
			// Explicitly clear - NewCompletedAIAnalysis doesn't set ExecutionEngine
			ai.Status.SelectedWorkflow.ExecutionEngine = ""
			ctx := context.Background()

			// Act
			name, err := weCreator.Create(ctx, rr, ai)

			// Assert
			Expect(err).ToNot(HaveOccurred())
			created := &workflowexecutionv1.WorkflowExecution{}
			err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, created)
			Expect(err).ToNot(HaveOccurred())
			Expect(created.Spec.ExecutionEngine).To(Equal("tekton"))
		})
	})

	Describe("resolveTargetResource (BR-HAPI-191)", func() {
		// resolveTargetResource is private; tested via Create() which sets WE.Spec.TargetResource.
		// BR-HAPI-191: Prefer LLM-identified AffectedResource over RR's TargetResource.
		It("should use RCA AffectedResource namespace/kind/name when namespaced (LLM identified Deployment)", func() {
			// Arrange: AI with RootCauseAnalysis.AffectedResource (namespaced)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			weCreator := creator.NewWorkflowExecutionCreator(fakeClient, scheme, nil)
			rr := helpers.NewRemediationRequest("test-remediation", "default")
			ai := helpers.NewCompletedAIAnalysis("ai-test-remediation", "default")
			ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
				Summary: "Pod crash loop - parent Deployment needs rollback",
				AffectedResource: &aianalysisv1.AffectedResource{
					Namespace: "prod",
					Kind:      "Deployment",
					Name:      "my-app",
				},
			}
			ctx := context.Background()

			// Act
			name, err := weCreator.Create(ctx, rr, ai)

			// Assert: WE uses namespace/kind/name from AffectedResource (not RR's Pod)
			Expect(err).ToNot(HaveOccurred())
			created := &workflowexecutionv1.WorkflowExecution{}
			err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, created)
			Expect(err).ToNot(HaveOccurred())
			Expect(created.Spec.TargetResource).To(Equal("prod/Deployment/my-app"),
				"BR-HAPI-191: WorkflowExecution must use LLM-identified Deployment for correct patching target")
		})

		It("should use RCA AffectedResource kind/name when cluster-scoped (Namespace empty)", func() {
			// Arrange: AI with AffectedResource cluster-scoped (e.g., Node)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			weCreator := creator.NewWorkflowExecutionCreator(fakeClient, scheme, nil)
			rr := helpers.NewRemediationRequest("test-remediation", "default")
			ai := helpers.NewCompletedAIAnalysis("ai-test-remediation", "default")
			ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
				Summary: "Node not ready",
				AffectedResource: &aianalysisv1.AffectedResource{
					Namespace: "", // Cluster-scoped
					Kind:      "Node",
					Name:      "worker-1",
				},
			}
			ctx := context.Background()

			// Act
			name, err := weCreator.Create(ctx, rr, ai)

			// Assert: WE uses kind/name (no namespace)
			Expect(err).ToNot(HaveOccurred())
			created := &workflowexecutionv1.WorkflowExecution{}
			err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, created)
			Expect(err).ToNot(HaveOccurred())
			Expect(created.Spec.TargetResource).To(Equal("Node/worker-1"),
				"BR-HAPI-191: Cluster-scoped resources use kind/name format")
		})

		It("should fall back to RR TargetResource when AffectedResource Kind or Name is empty", func() {
			// Arrange: RCA has AffectedResource but Kind empty - fallback to RR
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			weCreator := creator.NewWorkflowExecutionCreator(fakeClient, scheme, nil)
			rr := helpers.NewRemediationRequest("test-remediation", "default")
			ai := helpers.NewCompletedAIAnalysis("ai-test-remediation", "default")
			ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
				Summary: "Incomplete RCA",
				AffectedResource: &aianalysisv1.AffectedResource{
					Namespace: "prod",
					Kind:      "", // Empty - triggers fallback
					Name:      "my-app",
				},
			}
			ctx := context.Background()

			// Act
			name, err := weCreator.Create(ctx, rr, ai)

			// Assert: WE uses RR's target (default Pod)
			Expect(err).ToNot(HaveOccurred())
			created := &workflowexecutionv1.WorkflowExecution{}
			err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, created)
			Expect(err).ToNot(HaveOccurred())
			Expect(created.Spec.TargetResource).To(Equal("default/Pod/test-pod"),
				"BR-HAPI-191: Fallback to RR TargetResource when AffectedResource incomplete")
		})

		It("should fall back to RR TargetResource when AffectedResource Name is empty", func() {
			// Arrange: RCA has AffectedResource but Name empty - fallback to RR
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			weCreator := creator.NewWorkflowExecutionCreator(fakeClient, scheme, nil)
			rr := helpers.NewRemediationRequest("test-remediation", "default")
			ai := helpers.NewCompletedAIAnalysis("ai-test-remediation", "default")
			ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
				Summary: "Incomplete RCA",
				AffectedResource: &aianalysisv1.AffectedResource{
					Namespace: "prod",
					Kind:      "Deployment",
					Name:      "", // Empty - triggers fallback
				},
			}
			ctx := context.Background()

			// Act
			name, err := weCreator.Create(ctx, rr, ai)

			// Assert: WE uses RR's target
			Expect(err).ToNot(HaveOccurred())
			created := &workflowexecutionv1.WorkflowExecution{}
			err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, created)
			Expect(err).ToNot(HaveOccurred())
			Expect(created.Spec.TargetResource).To(Equal("default/Pod/test-pod"))
		})
	})

	Describe("BuildTargetResourceString", func() {
		It("should format namespaced resources as 'namespace/kind/name' per BR-ORCH-025", func() {
			// Arrange
			rr := helpers.NewRemediationRequest("test-remediation", "default")

			// Act
			result := creator.BuildTargetResourceString(rr)

			// Assert - default testutil creates a Pod in default namespace
			Expect(result).To(Equal("default/Pod/test-pod"))
		})

		It("should format cluster-scoped Node as 'kind/name' per BR-ORCH-025", func() {
			// Arrange - Node is cluster-scoped, factory sets empty namespace
			rr := helpers.NewRemediationRequest("test-remediation", "default",
				helpers.RemediationRequestOpts{TargetKind: "Node", TargetName: "worker-1"})

			// Act
			result := creator.BuildTargetResourceString(rr)

			// Assert
			Expect(result).To(Equal("Node/worker-1"))
		})

		It("should format cluster-scoped PersistentVolume as 'kind/name' per BR-ORCH-025", func() {
			// Arrange - PersistentVolume is cluster-scoped, factory sets empty namespace
			rr := helpers.NewRemediationRequest("pv-test", "default",
				helpers.RemediationRequestOpts{TargetKind: "PersistentVolume", TargetName: "my-pv"})

			// Act
			result := creator.BuildTargetResourceString(rr)

			// Assert
			Expect(result).To(Equal("PersistentVolume/my-pv"))
		})
	})

	Describe("Create error handling", func() {
		DescribeTable("should return error for invalid AIAnalysis per BR-ORCH-025",
			func(description string, setupAI func(*aianalysisv1.AIAnalysis), expectedError string) {
				// Arrange
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				weCreator := creator.NewWorkflowExecutionCreator(fakeClient, scheme, nil)
				rr := helpers.NewRemediationRequest("test-remediation", "default")
				ai := helpers.NewCompletedAIAnalysis("ai-test-remediation", "default")
				setupAI(ai)
				ctx := context.Background()

				// Act
				_, err := weCreator.Create(ctx, rr, ai)

				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedError))
			},
			Entry("nil SelectedWorkflow",
				"SelectedWorkflow is nil",
				func(ai *aianalysisv1.AIAnalysis) { ai.Status.SelectedWorkflow = nil },
				"no selectedWorkflow"),
			Entry("empty WorkflowID",
				"WorkflowID is empty",
				func(ai *aianalysisv1.AIAnalysis) { ai.Status.SelectedWorkflow.WorkflowID = "" },
				"workflowId is required"),
			Entry("empty ExecutionBundle",
				"ExecutionBundle is empty",
				func(ai *aianalysisv1.AIAnalysis) { ai.Status.SelectedWorkflow.ExecutionBundle = "" },
				"executionBundle is required"),
		)

		It("should return error when client Create fails per BR-ORCH-025", func() {
			// Arrange - Use interceptor to simulate Create failure
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, client client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						return errors.New("simulated create failure")
					},
				}).Build()
			weCreator := creator.NewWorkflowExecutionCreator(fakeClient, scheme, nil)
			rr := helpers.NewRemediationRequest("test-remediation", "default")
			ai := helpers.NewCompletedAIAnalysis("ai-test-remediation", "default")
			ctx := context.Background()

			// Act
			_, err := weCreator.Create(ctx, rr, ai)

			// Assert
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create WorkflowExecution"))
		})

		It("should return error when client Get fails with non-NotFound error per BR-ORCH-025", func() {
			// Arrange - Use interceptor to simulate Get failure
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						return errors.New("simulated get failure")
					},
				}).Build()
			weCreator := creator.NewWorkflowExecutionCreator(fakeClient, scheme, nil)
			rr := helpers.NewRemediationRequest("test-remediation", "default")
			ai := helpers.NewCompletedAIAnalysis("ai-test-remediation", "default")
			ctx := context.Background()

			// Act
			_, err := weCreator.Create(ctx, rr, ai)

			// Assert
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to check existing WorkflowExecution"))
		})
	})

	Describe("buildExecutionConfig", func() {
		It("should set timeout when TimeoutConfig is provided per BR-ORCH-028", func() {
			// Arrange
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			weCreator := creator.NewWorkflowExecutionCreator(fakeClient, scheme, nil)
			executingTimeout := metav1.Duration{Duration: 30 * 60 * 1000000000} // 30 minutes
			rr := helpers.NewRemediationRequest("test-remediation", "default",
				helpers.RemediationRequestOpts{
					TimeoutConfig: &remediationv1.TimeoutConfig{
						Executing: &executingTimeout,
					},
				})
			ai := helpers.NewCompletedAIAnalysis("ai-test-remediation", "default")
			ctx := context.Background()

			// Act
			name, err := weCreator.Create(ctx, rr, ai)

			// Assert
			Expect(err).ToNot(HaveOccurred())

			created := &workflowexecutionv1.WorkflowExecution{}
			err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, created)
			Expect(err).ToNot(HaveOccurred())
			Expect(created.Spec.ExecutionConfig).ToNot(BeNil())
			Expect(created.Spec.ExecutionConfig.Timeout).ToNot(BeNil())
			Expect(created.Spec.ExecutionConfig.Timeout.Duration).To(Equal(30 * 60 * 1000000000 * time.Nanosecond))
		})

		It("should return nil ExecutionConfig when no timeout configured per BR-ORCH-028", func() {
			// Arrange
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			weCreator := creator.NewWorkflowExecutionCreator(fakeClient, scheme, nil)
			rr := helpers.NewRemediationRequest("test-remediation", "default")
			ai := helpers.NewCompletedAIAnalysis("ai-test-remediation", "default")
			ctx := context.Background()

			// Act
			name, err := weCreator.Create(ctx, rr, ai)

			// Assert
			Expect(err).ToNot(HaveOccurred())

			created := &workflowexecutionv1.WorkflowExecution{}
			err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, created)
			Expect(err).ToNot(HaveOccurred())
			Expect(created.Spec.ExecutionConfig).To(BeNil())
		})

		It("should return nil ExecutionConfig when timeout duration is zero per BR-ORCH-028", func() {
			// Arrange
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			weCreator := creator.NewWorkflowExecutionCreator(fakeClient, scheme, nil)
			rr := helpers.NewRemediationRequest("test-remediation", "default",
				helpers.RemediationRequestOpts{
					TimeoutConfig: &remediationv1.TimeoutConfig{
						// No timeout override (uses defaults)
					},
				})
			ai := helpers.NewCompletedAIAnalysis("ai-test-remediation", "default")
			ctx := context.Background()

			// Act
			name, err := weCreator.Create(ctx, rr, ai)

			// Assert
			Expect(err).ToNot(HaveOccurred())

			created := &workflowexecutionv1.WorkflowExecution{}
			err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, created)
			Expect(err).ToNot(HaveOccurred())
			Expect(created.Spec.ExecutionConfig).To(BeNil())
		})
	})
})
