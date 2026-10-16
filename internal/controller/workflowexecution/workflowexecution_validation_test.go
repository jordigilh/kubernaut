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

package workflowexecution

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("WorkflowExecution input validation [SI-10, ASVS V5.1.x]", func() {
	newValidationWFE := func(target, image string) *workflowexecutionv1alpha1.WorkflowExecution {
		return &workflowexecutionv1alpha1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{Name: "validation-wfe", Namespace: "kubernaut-system"},
			Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
				RemediationRequestRef: corev1.ObjectReference{
					Name: "rr-validation",
					UID:  types.UID("550e8400-e29b-41d4-a716-446655440099"),
				},
				WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
					ExecutionEngine: executionEngineJob,
					ExecutionBundle: image,
				}},
				TargetResource: target,
			},
		}
	}

	It("UT-WE-2512-010: accepts Kubernetes-shaped target identity and OCI bundle", func() {
		wfe := newValidationWFE("prod/Deployment/api-7", "registry.example.com/workflows/restart:v1")
		Expect((&WorkflowExecutionReconciler{}).ValidateSpec(wfe)).To(Succeed())
	})

	It("UT-WE-2512-011: rejects traversal and malformed target names", func() {
		wfe := newValidationWFE("prod/Deployment/secret..", "registry.example.com/workflows/restart:v1")
		Expect((&WorkflowExecutionReconciler{}).ValidateSpec(wfe)).To(MatchError(ContainSubstring("targetResource name is invalid")))
	})

	It("UT-WE-2512-012: rejects malformed execution bundle references", func() {
		wfe := newValidationWFE("prod/Deployment/api", "not a valid image")
		Expect((&WorkflowExecutionReconciler{}).ValidateSpec(wfe)).To(MatchError(ContainSubstring("executionBundle must be a valid OCI image reference")))
	})

	It("UT-WE-2512-013: rejects new Job WFEs without an RR UID", func() {
		wfe := newValidationWFE("prod/Deployment/api", "registry.example.com/workflows/restart:v1")
		wfe.Spec.RemediationRequestRef.UID = ""
		Expect((&WorkflowExecutionReconciler{}).ValidateSpec(wfe)).To(MatchError(ContainSubstring("remediationRequestRef.uid is required")))
	})
})
