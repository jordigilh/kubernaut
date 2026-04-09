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
	"time"

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
// RO CREATOR WFE SPEC TESTS (#501 / #650)
// ========================================
// Authority: DD-WE-005, Issue #501 (ExecutionConfig from RR timeouts),
// Issue #650 (ServiceAccountName removed from WFE spec and AIAnalysis
// SelectedWorkflow; SA resolved from DS at runtime in WE controller).
// ========================================

var _ = Describe("WorkflowExecution Creator WFE spec [DD-WE-005] (#501/#650)", func() {

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

	buildAI := func() *aianalysisv1.AIAnalysis {
		return &aianalysisv1.AIAnalysis{
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
	}

	Context("WorkflowExecution spec (Issue #650: no SA on spec; #501 timeouts)", func() {
		var ctx context.Context

		BeforeEach(func() {
			ctx = context.Background()
		})

		It("UT-WE-501-010: should set ExecutionConfig.Timeout from RR Status.TimeoutConfig.Executing", func() {
			rr := buildRR()
			execTimeout := &metav1.Duration{Duration: 45 * time.Minute}
			rr.Status = remediationv1.RemediationRequestStatus{
				TimeoutConfig: &remediationv1.TimeoutConfig{
					Executing: execTimeout,
				},
			}
			ai := buildAI()
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rr).Build()
			wec := creator.NewWorkflowExecutionCreator(k8sClient, scheme, nil)

			name, err := wec.Create(ctx, rr, ai)
			Expect(err).ToNot(HaveOccurred())
			Expect(name).To(Equal("we-rr-test"))

			created := &workflowexecutionv1.WorkflowExecution{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, created)
			Expect(err).ToNot(HaveOccurred())
			Expect(created.Spec.ExecutionConfig.Timeout.Duration).To(Equal(execTimeout.Duration))
			Expect(created.Spec.WorkflowRef.WorkflowID).To(Equal("wf-uuid-123"))
			Expect(created.Spec.WorkflowRef.Version).To(Equal("1.0.0"))
			Expect(created.Spec.WorkflowRef.ExecutionBundle).To(Equal("quay.io/test:v1@sha256:abc123"))
		})

		It("UT-RO-481-002: should leave ExecutionConfig nil when no executing timeout (WFE spec has no SA per #650)", func() {
			rr := buildRR()
			ai := buildAI()
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rr).Build()
			wec := creator.NewWorkflowExecutionCreator(k8sClient, scheme, nil)

			name, err := wec.Create(ctx, rr, ai)
			Expect(err).ToNot(HaveOccurred())

			created := &workflowexecutionv1.WorkflowExecution{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, created)
			Expect(err).ToNot(HaveOccurred())
			Expect(created.Spec.ExecutionConfig).To(BeNil())
			Expect(created.Spec.WorkflowRef.WorkflowID).To(Equal("wf-uuid-123"))
			Expect(created.Spec.TargetResource).To(Equal("default/Deployment/nginx"))
		})
	})
})
