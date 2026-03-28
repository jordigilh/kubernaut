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

package remediationorchestrator

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
)

// ========================================
// RO CREATOR SA PROPAGATION TESTS (#501)
// ========================================
// Authority: DD-WE-005 v2.0, Issue #501
// Validates that ServiceAccountName from AIAnalysis.Status.SelectedWorkflow
// is propagated to WorkflowExecution.Spec.ServiceAccountName (top-level).
// ========================================

var _ = Describe("WorkflowExecution Creator SA Propagation [DD-WE-005] (#501)", func() {

	var scheme *runtime.Scheme

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())
	})

	buildRR := func() *remediationv1.RemediationRequest {
		return &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rr-test",
				Namespace: "kubernaut-system",
				UID:       "rr-uid-123",
			},
			Spec: remediationv1.RemediationRequestSpec{
				TargetResource: remediationv1.ResourceIdentifier{
					Kind:      "Deployment",
					Name:      "nginx",
					Namespace: "default",
				},
			},
		}
	}

	buildAI := func(saName string) *aianalysisv1.AIAnalysis {
		ai := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "aa-test",
				Namespace: "kubernaut-system",
			},
			Status: aianalysisv1.AIAnalysisStatus{
				SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
					WorkflowID:      "wf-uuid-123",
					Version:         "1.0.0",
					ExecutionBundle: "quay.io/test:v1@sha256:abc123",
				},
			},
		}
		if saName != "" {
			ai.Status.SelectedWorkflow.ServiceAccountName = saName
		}
		return ai
	}

	Context("ServiceAccountName propagation (Issue #501)", func() {
		var ctx context.Context

		BeforeEach(func() {
			ctx = context.Background()
		})

		It("UT-WE-501-010: should set Spec.ServiceAccountName and ExecutionConfig with only Timeout", func() {
			rr := buildRR()
			ai := buildAI("my-workflow-sa")
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rr).Build()
			wec := creator.NewWorkflowExecutionCreator(k8sClient, scheme, nil)

			name, err := wec.Create(ctx, rr, ai)
			Expect(err).ToNot(HaveOccurred())
			Expect(name).To(Equal("we-rr-test"))

			created := &workflowexecutionv1.WorkflowExecution{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, created)
			Expect(err).ToNot(HaveOccurred())
			Expect(created.Spec.ServiceAccountName).To(Equal("my-workflow-sa"),
				"SA should be at Spec top level, not inside ExecutionConfig")
			Expect(created.Spec.ExecutionConfig).To(BeNil(),
				"ExecutionConfig should be nil when no timeout is configured")
		})

		It("UT-RO-481-002: should leave ExecutionConfig nil and SA empty when no SA and no timeout", func() {
			rr := buildRR()
			ai := buildAI("")
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rr).Build()
			wec := creator.NewWorkflowExecutionCreator(k8sClient, scheme, nil)

			name, err := wec.Create(ctx, rr, ai)
			Expect(err).ToNot(HaveOccurred())

			created := &workflowexecutionv1.WorkflowExecution{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, created)
			Expect(err).ToNot(HaveOccurred())
			Expect(created.Spec.ServiceAccountName).To(Equal(""),
				"SA should be empty when no SA specified")
			Expect(created.Spec.ExecutionConfig).To(BeNil())
		})
	})
})
