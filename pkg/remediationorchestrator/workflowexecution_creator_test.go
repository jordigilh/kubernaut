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
package remediationorchestrator_test

import (
	"context"
	"errors"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
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

			// Assert — creator is a concrete struct, verify interface compliance
			Expect(weCreator).To(BeAssignableToTypeOf(&creator.WorkflowExecutionCreator{}))
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

	Describe("CRD-embedded execution snapshot pass-through (Issue #1661 Change 11d/11f)", func() {
		// Authority: DD-WORKFLOW-018. Now that WorkflowRef carries these six
		// fields (Change 11c, ActionType added by Change 11f) and
		// WorkflowExecution will stop re-fetching them from DataStorage
		// (Change 11e), RO's buildWorkflowExecution must be the one
		// production call site that copies them from the CRD-embedded
		// AIAnalysis.Status.SelectedWorkflow snapshot into WorkflowRef.
		It("UT-RO-341-001: copies Dependencies/Resources/DeclaredParameterNames/ExecutionEngine/ServiceAccountName/ActionType/WorkflowName from SelectedWorkflow into WorkflowRef", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			weCreator := creator.NewWorkflowExecutionCreator(fakeClient, scheme, nil)
			rr := helpers.NewRemediationRequest("test-snapshot-passthrough", "default")
			ai := helpers.NewCompletedAIAnalysis("ai-test-snapshot-passthrough", "default")
			ai.Status.SelectedWorkflow.ExecutionEngine = "job"
			ai.Status.SelectedWorkflow.ServiceAccountName = "workflow-runner-sa"
			ai.Status.SelectedWorkflow.ActionType = "ScaleReplicas"
			ai.Status.SelectedWorkflow.WorkflowName = "scale-replicas-fix"
			ai.Status.SelectedWorkflow.Dependencies = &sharedtypes.WorkflowDependencies{
				Secrets: []sharedtypes.WorkflowResourceDependency{{Name: "db-creds"}},
			}
			ai.Status.SelectedWorkflow.Resources = &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
				Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m")},
			}
			ai.Status.SelectedWorkflow.DeclaredParameterNames = map[string]bool{"TARGET_POD": true}
			ctx := context.Background()

			name, err := weCreator.Create(ctx, rr, ai)
			Expect(err).ToNot(HaveOccurred())

			created := &workflowexecutionv1.WorkflowExecution{}
			Expect(fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, created)).To(Succeed())

			Expect(created.Spec.WorkflowRef.ExecutionEngine).To(Equal("job"),
				"Issue #1661 Change 11d: ExecutionEngine must pass through to WorkflowRef")
			Expect(created.Spec.WorkflowRef.ServiceAccountName).To(Equal("workflow-runner-sa"))
			Expect(created.Spec.WorkflowRef.ActionType).To(Equal("ScaleReplicas"),
				"Issue #1661 Change 11f: ActionType must pass through to WorkflowRef like its siblings")
			Expect(created.Spec.WorkflowRef.WorkflowName).To(Equal("scale-replicas-fix"),
				"Issue #1661 Change 12: WorkflowName must pass through to WorkflowRef like its siblings")
			Expect(created.Spec.WorkflowRef.Dependencies).To(Equal(ai.Status.SelectedWorkflow.Dependencies))
			Expect(created.Spec.WorkflowRef.Resources).To(Equal(ai.Status.SelectedWorkflow.Resources))
			Expect(created.Spec.WorkflowRef.DeclaredParameterNames).To(Equal(map[string]bool{"TARGET_POD": true}))
		})
	})

	Describe("resolveTargetResource (BR-HAPI-191)", func() {
		// resolveTargetResource is private; tested via Create() which sets WE.Spec.TargetResource.
		// BR-HAPI-191: Prefer LLM-identified RemediationTarget over RR's TargetResource.
		It("should use RCA RemediationTarget namespace/kind/name when namespaced (LLM identified Deployment)", func() {
			// Arrange: AI with RootCauseAnalysis.RemediationTarget (namespaced)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			weCreator := creator.NewWorkflowExecutionCreator(fakeClient, scheme, nil)
			rr := helpers.NewRemediationRequest("test-remediation", "default")
			ai := helpers.NewCompletedAIAnalysis("ai-test-remediation", "default")
			ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
				Summary: "Pod crash loop - parent Deployment needs rollback",
				RemediationTarget: &aianalysisv1.RemediationTarget{
					Namespace: "prod",
					Kind:      "Deployment",
					Name:      "my-app",
				},
			}
			ctx := context.Background()

			// Act
			name, err := weCreator.Create(ctx, rr, ai)

			// Assert: WE uses namespace/kind/name from RemediationTarget (not RR's Pod)
			Expect(err).ToNot(HaveOccurred())
			created := &workflowexecutionv1.WorkflowExecution{}
			err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, created)
			Expect(err).ToNot(HaveOccurred())
			Expect(created.Spec.TargetResource).To(Equal("prod/Deployment/my-app"),
				"BR-HAPI-191: WorkflowExecution must use LLM-identified Deployment for correct patching target")
		})

		It("should use RCA RemediationTarget kind/name when cluster-scoped (Namespace empty)", func() {
			// Arrange: AI with RemediationTarget cluster-scoped (e.g., Node)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			weCreator := creator.NewWorkflowExecutionCreator(fakeClient, scheme, nil)
			rr := helpers.NewRemediationRequest("test-remediation", "default")
			ai := helpers.NewCompletedAIAnalysis("ai-test-remediation", "default")
			ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
				Summary: "Node not ready",
				RemediationTarget: &aianalysisv1.RemediationTarget{
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

		It("should fall back to RR TargetResource when RemediationTarget Kind or Name is empty", func() {
			// Arrange: RCA has RemediationTarget but Kind empty - fallback to RR
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			weCreator := creator.NewWorkflowExecutionCreator(fakeClient, scheme, nil)
			rr := helpers.NewRemediationRequest("test-remediation", "default")
			ai := helpers.NewCompletedAIAnalysis("ai-test-remediation", "default")
			ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
				Summary: "Incomplete RCA",
				RemediationTarget: &aianalysisv1.RemediationTarget{
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
				"BR-HAPI-191: Fallback to RR TargetResource when RemediationTarget incomplete")
		})

		It("should fall back to RR TargetResource when RemediationTarget Name is empty", func() {
			// Arrange: RCA has RemediationTarget but Name empty - fallback to RR
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			weCreator := creator.NewWorkflowExecutionCreator(fakeClient, scheme, nil)
			rr := helpers.NewRemediationRequest("test-remediation", "default")
			ai := helpers.NewCompletedAIAnalysis("ai-test-remediation", "default")
			ai.Status.RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
				Summary: "Incomplete RCA",
				RemediationTarget: &aianalysisv1.RemediationTarget{
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
			Entry("empty ExecutionEngine",
				// UT-RO-341-002 (Issue #1661 Change 11d, DD-WORKFLOW-018): once WFE
				// stops resolving ExecutionEngine from DS at runtime (Change 11e),
				// an empty value here would silently default to the wrong engine
				// instead of failing closed -- so RO must reject it up front,
				// mirroring the existing WorkflowID/ExecutionBundle checks.
				"ExecutionEngine is empty",
				func(ai *aianalysisv1.AIAnalysis) { ai.Status.SelectedWorkflow.ExecutionEngine = "" },
				"executionEngine is required"),
			Entry("empty WorkflowName",
				// UT-RO-1711-001 (Issue #1711 cascade, DD-KA-001 v1.1): WorkflowName
				// is a Required (no-omitempty) field on sharedtypes.WorkflowSnapshot
				// -- its upstream source (RemediationWorkflow.metadata.name) is
				// Kubernetes-guaranteed non-empty, so an empty value here means the
				// snapshot never went through catalog enrichment and must not
				// silently create a WorkflowExecution with a missing name.
				"WorkflowName is empty",
				func(ai *aianalysisv1.AIAnalysis) { ai.Status.SelectedWorkflow.WorkflowName = "" },
				"workflowName is required"),
			Entry("empty ActionType",
				// UT-RO-1711-002 (Issue #1711 cascade, DD-KA-001 v1.1): ActionType is
				// likewise Required on WorkflowSnapshot -- catalog-authoritative,
				// never LLM-suppliable (Issue #1661 Change 12). Same rationale as
				// WorkflowName above.
				"ActionType is empty",
				func(ai *aianalysisv1.AIAnalysis) { ai.Status.SelectedWorkflow.ActionType = "" },
				"actionType is required"),
			Entry("empty Version",
				// UT-RO-1711-003 (Issue #1711 cascade, DD-KA-001 v1.1): Version is
				// likewise Required on WorkflowSnapshot.
				"Version is empty",
				func(ai *aianalysisv1.AIAnalysis) { ai.Status.SelectedWorkflow.Version = "" },
				"version is required"),
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

	Describe("ClusterID propagation (BR-FLEET-054)", func() {
		It("UT-WE-054-PROP-001: propagates ClusterID from RemediationRequest to WorkflowExecution", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			weCreator := creator.NewWorkflowExecutionCreator(fakeClient, scheme, nil)
			rr := helpers.NewRemediationRequest("test-remote-cluster", "default",
				helpers.RemediationRequestOpts{ClusterID: "prod-east-1"})
			ai := helpers.NewCompletedAIAnalysis("ai-test-remote-cluster", "default")
			ctx := context.Background()

			name, err := weCreator.Create(ctx, rr, ai)
			Expect(err).ToNot(HaveOccurred())

			created := &workflowexecutionv1.WorkflowExecution{}
			err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, created)
			Expect(err).ToNot(HaveOccurred())
			Expect(created.Spec.ClusterID).To(Equal("prod-east-1"),
				"BR-FLEET-054: ClusterID must be propagated from RR to WFE for remote execution routing")
		})

		It("UT-WE-054-PROP-002: leaves ClusterID empty for local hub cluster", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			weCreator := creator.NewWorkflowExecutionCreator(fakeClient, scheme, nil)
			rr := helpers.NewRemediationRequest("test-local-cluster", "default")
			ai := helpers.NewCompletedAIAnalysis("ai-test-local-cluster", "default")
			ctx := context.Background()

			name, err := weCreator.Create(ctx, rr, ai)
			Expect(err).ToNot(HaveOccurred())

			created := &workflowexecutionv1.WorkflowExecution{}
			err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, created)
			Expect(err).ToNot(HaveOccurred())
			Expect(created.Spec.ClusterID).To(BeEmpty(),
				"BR-FLEET-054: empty ClusterID indicates local hub cluster execution")
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
			Expect(created.Spec.ExecutionConfig).To(HaveValue(HaveField("Timeout", Not(BeNil()))))
			Expect(created.Spec.ExecutionConfig.Timeout.Duration).To(Equal(30 * time.Minute))
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
