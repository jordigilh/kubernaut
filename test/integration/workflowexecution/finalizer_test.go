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

package workflowexecution

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

var _ = Describe("Integration: Finalizer Cleanup", func() {
	var (
		wfeName        string
		namespace      string
		targetResource string
	)

	BeforeEach(func() {
		wfeName = "test-finalizer-" + randomString(5)
		namespace = "kubernaut-test"
		targetResource = "production/deployment/app-" + randomString(5)
	})

	AfterEach(func() {
		// Cleanup WFE if exists
		wfe := &workflowexecutionv1.WorkflowExecution{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      wfeName,
			Namespace: namespace,
		}, wfe); err == nil {
			_ = k8sClient.Delete(ctx, wfe)
		}

		// Cleanup PipelineRun
		prName := pipelineRunName(targetResource)
		pr := &tektonv1.PipelineRun{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      prName,
			Namespace: "kubernaut-workflows",
		}, pr); err == nil {
			_ = k8sClient.Delete(ctx, pr)
		}
	})

	It("should add finalizer when WFE is created", func() {
		By("Creating a WorkflowExecution")
		wfe := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{
				Name:      wfeName,
				Namespace: namespace,
			},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				TargetResource: targetResource,
				WorkflowRef: workflowexecutionv1.WorkflowRef{
					WorkflowID:     "disk-cleanup",
					Version:        "v1.0.0",
					ContainerImage: "ghcr.io/kubernaut/workflows/disk-cleanup@sha256:abc123def456",
				},
			},
		}
		Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

		By("Waiting for finalizer to be added")
		Eventually(func() bool {
			updated := &workflowexecutionv1.WorkflowExecution{}
			_ = k8sClient.Get(ctx, types.NamespacedName{
				Name:      wfeName,
				Namespace: namespace,
			}, updated)
			return controllerutil.ContainsFinalizer(updated, "workflowexecution.kubernaut.ai/finalizer")
		}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())
	})

	It("should cleanup PipelineRun when WFE is deleted while Running", func() {
		By("Creating a WorkflowExecution")
		wfe := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{
				Name:      wfeName,
				Namespace: namespace,
			},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				TargetResource: targetResource,
				WorkflowRef: workflowexecutionv1.WorkflowRef{
					WorkflowID:     "disk-cleanup",
					Version:        "v1.0.0",
					ContainerImage: "ghcr.io/kubernaut/workflows/disk-cleanup@sha256:abc123def456",
				},
			},
		}
		Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

		By("Waiting for Running phase")
		Eventually(func() string {
			updated := &workflowexecutionv1.WorkflowExecution{}
			_ = k8sClient.Get(ctx, types.NamespacedName{
				Name:      wfeName,
				Namespace: namespace,
			}, updated)
			return updated.Status.Phase
		}, 10*time.Second, 500*time.Millisecond).Should(Equal(workflowexecutionv1.PhaseRunning))

		By("Verifying PipelineRun exists")
		prName := pipelineRunName(targetResource)
		pr := &tektonv1.PipelineRun{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      prName,
			Namespace: "kubernaut-workflows",
		}, pr)).To(Succeed())

		By("Deleting the WorkflowExecution")
		wfe = &workflowexecutionv1.WorkflowExecution{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      wfeName,
			Namespace: namespace,
		}, wfe)).To(Succeed())
		Expect(k8sClient.Delete(ctx, wfe)).To(Succeed())

		By("Waiting for WFE to be deleted")
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      wfeName,
				Namespace: namespace,
			}, &workflowexecutionv1.WorkflowExecution{})
			return apierrors.IsNotFound(err)
		}, 30*time.Second, 1*time.Second).Should(BeTrue())

		By("Verifying PipelineRun was cleaned up by finalizer")
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      prName,
				Namespace: "kubernaut-workflows",
			}, &tektonv1.PipelineRun{})
			return apierrors.IsNotFound(err)
		}, 30*time.Second, 1*time.Second).Should(BeTrue())
	})
})

