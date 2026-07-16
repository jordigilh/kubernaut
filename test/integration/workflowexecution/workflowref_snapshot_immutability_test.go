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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// ========================================
// IT-WFE-340-001 (Issue #1661 Change 11c, DD-WORKFLOW-018)
// ========================================
// Authority: DD-WORKFLOW-018. Proves, against a real envtest API server, that
// the pre-existing WorkflowExecutionSpec-level
// +kubebuilder:validation:XValidation:rule="self == oldSelf" (ADR-001)
// transparently covers WorkflowRef's five new CRD-embedded execution
// snapshot fields (ExecutionEngine/ServiceAccountName/Dependencies/
// Resources/DeclaredParameterNames) without any additional per-field CEL
// rule -- creating a WorkflowExecution with them populated succeeds, and any
// later attempt to mutate them is rejected the same way mutating WorkflowID/
// Version/ExecutionBundle already is.
//
// RED: WorkflowRef has none of these fields yet -- this file must fail to
// compile.
// ========================================
var _ = Describe("WorkflowRef CRD-embedded execution snapshot immutability (Issue #1661 Change 11c)", Label("integration", "workflowexecution"), func() {
	newWFE := func(name string) *workflowexecutionv1alpha1.WorkflowExecution {
		return &workflowexecutionv1alpha1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: DefaultNamespace,
			},
			Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
				RemediationRequestRef: corev1.ObjectReference{
					APIVersion: "kubernaut.ai/v1alpha1",
					Kind:       "RemediationRequest",
					Name:       "test-rr-" + name,
					Namespace:  DefaultNamespace,
				},
				WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
					WorkflowID:         "wf-oom-recovery",
					Version:            "v1.0.0",
					ExecutionBundle:    "quay.io/kubernaut/oom-recovery:v1",
					ExecutionEngine:    "job",
					ServiceAccountName: "kubernaut-workflow-runner",
					Dependencies: &sharedtypes.WorkflowDependencies{
						Secrets: []sharedtypes.WorkflowResourceDependency{{Name: "db-creds"}},
					},
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
					},
					DeclaredParameterNames: map[string]bool{"TARGET_NAMESPACE": true},
				},
				TargetResource: "default/deployment/test-app",
			},
		}
	}

	It("IT-WFE-340-001: creation succeeds with the snapshot fields populated, and any later mutation of them is rejected", func() {
		name := fmt.Sprintf("wfref-snapshot-%d", time.Now().UnixNano())
		wfe := newWFE(name)

		By("creating a WorkflowExecution with the CRD-embedded snapshot fields populated")
		Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
		defer func() { _ = k8sClient.Delete(ctx, wfe) }()

		By("mutating ExecutionEngine post-creation being rejected (ADR-001 spec immutability)")
		mutated := &workflowexecutionv1alpha1.WorkflowExecution{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: DefaultNamespace}, mutated)).To(Succeed())
		mutated.Spec.WorkflowRef.ExecutionEngine = "tekton"
		err := k8sClient.Update(ctx, mutated)
		Expect(err).To(HaveOccurred())
		Expect(apierrors.IsInvalid(err)).To(BeTrue())

		By("mutating DeclaredParameterNames post-creation being rejected")
		mutated2 := &workflowexecutionv1alpha1.WorkflowExecution{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: DefaultNamespace}, mutated2)).To(Succeed())
		mutated2.Spec.WorkflowRef.DeclaredParameterNames = map[string]bool{"OTHER": true}
		err = k8sClient.Update(ctx, mutated2)
		Expect(err).To(HaveOccurred())
		Expect(apierrors.IsInvalid(err)).To(BeTrue())

		By("re-submitting an identical spec succeeding (idempotent reconcile-retry safety)")
		identical := &workflowexecutionv1alpha1.WorkflowExecution{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: DefaultNamespace}, identical)).To(Succeed())
		Expect(k8sClient.Update(ctx, identical)).To(Succeed())
	})
})
