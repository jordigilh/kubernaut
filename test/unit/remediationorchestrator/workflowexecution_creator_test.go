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

// Package remediationorchestrator contains unit tests for the Remediation Orchestrator controller.
// BR-ORCH-025: WorkflowExecution Child CRD Creation with Data Pass-Through
// BR-ORCH-031: Cascade Deletion via Owner References
// BR-ORCH-032: Resource Lock Support
package remediationorchestrator

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	"github.com/jordigilh/kubernaut/pkg/testutil"
)

var _ = Describe("BR-ORCH-025: WorkflowExecution Child CRD Creation", func() {
	var (
		ctx        context.Context
		fakeClient client.Client
		scheme     *runtime.Scheme
		weCreator  *creator.WorkflowExecutionCreator
		rr         *remediationv1.RemediationRequest
		ai         *aianalysisv1.AIAnalysis
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()

		// Register schemes
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		weCreator = creator.NewWorkflowExecutionCreator(fakeClient, scheme)

		// Create test RemediationRequest using factory
		rr = testutil.NewRemediationRequest("test-rr", "default")
		Expect(fakeClient.Create(ctx, rr)).To(Succeed())

		// Create test AIAnalysis with selected workflow using factory
		ai = testutil.NewCompletedAIAnalysis("ai-test-rr", "default")
		Expect(fakeClient.Create(ctx, ai)).To(Succeed())
	})

	Describe("Create", func() {
		// DescribeTable: Consolidates data pass-through validation
		// Reference: SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md lines 1246-1306
		DescribeTable("should create WorkflowExecution CRD with correct data pass-through",
			func(fieldName string, validateFunc func(*workflowexecutionv1.WorkflowExecution)) {
				name, err := weCreator.Create(ctx, rr, ai)
				Expect(err).NotTo(HaveOccurred())
				Expect(name).To(Equal("we-test-rr"))

				we := &workflowexecutionv1.WorkflowExecution{}
				Expect(fakeClient.Get(ctx, client.ObjectKey{
					Name:      name,
					Namespace: rr.Namespace,
				}, we)).To(Succeed())

				validateFunc(we)
			},
			// RemediationRequestRef pass-through (BR-ORCH-025)
			Entry("RemediationRequestRef.Name pass-through",
				"RemediationRequestRef.Name",
				func(we *workflowexecutionv1.WorkflowExecution) {
					Expect(we.Spec.RemediationRequestRef.Name).To(Equal("test-rr"))
				}),
			Entry("RemediationRequestRef.Namespace pass-through",
				"RemediationRequestRef.Namespace",
				func(we *workflowexecutionv1.WorkflowExecution) {
					Expect(we.Spec.RemediationRequestRef.Namespace).To(Equal("default"))
				}),
			Entry("RemediationRequestRef.Kind pass-through",
				"RemediationRequestRef.Kind",
				func(we *workflowexecutionv1.WorkflowExecution) {
					Expect(we.Spec.RemediationRequestRef.Kind).To(Equal("RemediationRequest"))
				}),

			// WorkflowRef from AIAnalysis (DD-CONTRACT-001)
			Entry("WorkflowRef.WorkflowID pass-through",
				"WorkflowRef.WorkflowID",
				func(we *workflowexecutionv1.WorkflowExecution) {
					Expect(we.Spec.WorkflowRef.WorkflowID).To(Equal("pod-restart-workflow"))
				}),
			Entry("WorkflowRef.Version pass-through",
				"WorkflowRef.Version",
				func(we *workflowexecutionv1.WorkflowExecution) {
					Expect(we.Spec.WorkflowRef.Version).To(Equal("v1.0.0"))
				}),
			Entry("WorkflowRef.ContainerImage pass-through",
				"WorkflowRef.ContainerImage",
				func(we *workflowexecutionv1.WorkflowExecution) {
					Expect(we.Spec.WorkflowRef.ContainerImage).To(Equal("kubernaut/workflows/pod-restart:v1.0.0"))
				}),
			Entry("WorkflowRef.ContainerDigest pass-through",
				"WorkflowRef.ContainerDigest",
				func(we *workflowexecutionv1.WorkflowExecution) {
					Expect(we.Spec.WorkflowRef.ContainerDigest).To(Equal("sha256:abc123"))
				}),

			// Target resource for resource locking (DD-WE-001, BR-ORCH-032)
			Entry("TargetResource format (namespace/kind/name)",
				"TargetResource",
				func(we *workflowexecutionv1.WorkflowExecution) {
					Expect(we.Spec.TargetResource).To(Equal("default/Pod/test-pod"))
				}),

			// Parameters from LLM (DD-WORKFLOW-003)
			Entry("Parameters pass-through",
				"Parameters",
				func(we *workflowexecutionv1.WorkflowExecution) {
					Expect(we.Spec.Parameters).To(HaveKeyWithValue("TARGET_POD", "test-pod"))
				}),

			// Confidence and rationale for audit trail
			Entry("Confidence pass-through",
				"Confidence",
				func(we *workflowexecutionv1.WorkflowExecution) {
					Expect(we.Spec.Confidence).To(Equal(0.95))
				}),
			Entry("Rationale pass-through",
				"Rationale",
				func(we *workflowexecutionv1.WorkflowExecution) {
					Expect(we.Spec.Rationale).To(ContainSubstring("High confidence"))
				}),

			// Owner reference for cascade deletion (BR-ORCH-031)
			Entry("owner reference set for cascade deletion (BR-ORCH-031)",
				"OwnerReference",
				func(we *workflowexecutionv1.WorkflowExecution) {
					Expect(we.OwnerReferences).To(HaveLen(1))
					Expect(we.OwnerReferences[0].Name).To(Equal("test-rr"))
					Expect(we.OwnerReferences[0].Kind).To(Equal("RemediationRequest"))
					Expect(*we.OwnerReferences[0].Controller).To(BeTrue())
				}),

			// Labels for tracking
			Entry("remediation-request label set",
				"Label:remediation-request",
				func(we *workflowexecutionv1.WorkflowExecution) {
					Expect(we.Labels).To(HaveKeyWithValue("kubernaut.ai/remediation-request", "test-rr"))
				}),
			Entry("component label set",
				"Label:component",
				func(we *workflowexecutionv1.WorkflowExecution) {
					Expect(we.Labels).To(HaveKeyWithValue("kubernaut.ai/component", "workflow-execution"))
				}),
			Entry("workflow-id label set",
				"Label:workflow-id",
				func(we *workflowexecutionv1.WorkflowExecution) {
					Expect(we.Labels).To(HaveKeyWithValue("kubernaut.ai/workflow-id", "pod-restart-workflow"))
				}),

			// Execution config defaults
			Entry("ServiceAccountName default set",
				"ExecutionConfig.ServiceAccountName",
				func(we *workflowexecutionv1.WorkflowExecution) {
					Expect(we.Spec.ExecutionConfig.ServiceAccountName).To(Equal("kubernaut-workflow-runner"))
				}),
		)

		// Cluster-scoped resource handling
		Context("cluster-scoped resources (BR-ORCH-032)", func() {
			BeforeEach(func() {
				// Create cluster-scoped RemediationRequest
				rr = testutil.NewRemediationRequest("test-rr-node", "default", testutil.RemediationRequestOpts{
					TargetKind: "Node",
					TargetName: "worker-node-1",
				})
				// Override namespace to empty for cluster-scoped
				rr.Spec.TargetResource.Namespace = ""
				Expect(fakeClient.Create(ctx, rr)).To(Succeed())
			})

			It("should format TargetResource as kind/name for cluster-scoped resources", func() {
				name, err := weCreator.Create(ctx, rr, ai)
				Expect(err).NotTo(HaveOccurred())

				we := &workflowexecutionv1.WorkflowExecution{}
				Expect(fakeClient.Get(ctx, client.ObjectKey{
					Name:      name,
					Namespace: rr.Namespace,
				}, we)).To(Succeed())

				Expect(we.Spec.TargetResource).To(Equal("Node/worker-node-1"))
			})
		})

		// Error cases
		Context("error handling", func() {
			It("should return error if AIAnalysis has no selected workflow", func() {
				aiNoWorkflow := &aianalysisv1.AIAnalysis{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ai-no-workflow",
						Namespace: "default",
					},
					Status: aianalysisv1.AIAnalysisStatus{
						Phase:            "Completed",
						SelectedWorkflow: nil, // No workflow selected
					},
				}
				Expect(fakeClient.Create(ctx, aiNoWorkflow)).To(Succeed())

				_, err := weCreator.Create(ctx, rr, aiNoWorkflow)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("no selected workflow"))
			})
		})

		// Idempotency
		Context("idempotency (BR-ORCH-025)", func() {
			It("should return existing name if WorkflowExecution already exists", func() {
				name1, err := weCreator.Create(ctx, rr, ai)
				Expect(err).NotTo(HaveOccurred())

				name2, err := weCreator.Create(ctx, rr, ai)
				Expect(err).NotTo(HaveOccurred())
				Expect(name2).To(Equal(name1))

				weList := &workflowexecutionv1.WorkflowExecutionList{}
				Expect(fakeClient.List(ctx, weList, client.InNamespace(rr.Namespace))).To(Succeed())
				Expect(weList.Items).To(HaveLen(1))
			})
		})
	})
})
