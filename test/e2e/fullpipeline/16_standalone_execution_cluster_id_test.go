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

package fullpipeline

import (
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// E2E-FP-2390-005 proves that catalog-declared execution metadata survives the
// full selection path, while standalone WorkflowExecution dispatch remains on
// the local Kubernetes client.
var _ = Describe("Standalone catalog execution cluster [BR-FLEET-054]", func() {
	It("preserves the declared cluster ID and completes a local Job execution", func() {
		Expect(workflowUUIDs).To(HaveKey("standalone-exec-cluster-id-v1:production"))
		targetNamespace, ok := fpRemediateNS["standalone-exec-cluster-id"]
		Expect(ok).To(BeTrue(), "standalone execution cluster test namespace must be provisioned")
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "e2e-2390-snapshot-secret", Namespace: infrastructure.ExecutionNamespace},
			StringData: map[string]string{"marker": "metadata-only"},
		})).To(Succeed())
		Expect(infrastructure.DeployMemoryEaterNamed(ctx, "memory-eater", targetNamespace, kubeconfigPath,
			"64Mi", "20Mi", GinkgoWriter)).To(Succeed())

		var targetPodName string
		Eventually(func() string {
			pods := &corev1.PodList{}
			if err := apiReader.List(ctx, pods,
				client.InNamespace(targetNamespace),
				client.MatchingLabels{"app": "memory-eater"}); err != nil {
				return ""
			}
			for _, pod := range pods.Items {
				if pod.DeletionTimestamp == nil {
					targetPodName = pod.Name
					return pod.Name
				}
			}
			return ""
		}, timeout, interval).ShouldNot(BeEmpty(),
			"memory-eater Deployment must have an active Pod before posting the signal")
		// Kubernetes generates the Pod name from the Deployment name; Gateway
		// owner resolution requires that concrete Pod identity.
		rrName := fpPostSignalToGateway(
			"StandaloneExecutionCluster2378",
			targetPodName,
			targetNamespace,
		)

		// The WorkflowExecution is created only after AA has copied the
		// catalog-selected execution cluster into the RO snapshot. Assert the
		// persisted CRD fields rather than relying on controller logs.
		var we workflowexecutionv1.WorkflowExecution
		Eventually(func() string {
			weList := &workflowexecutionv1.WorkflowExecutionList{}
			if err := apiReader.List(ctx, weList, client.InNamespace(namespace)); err != nil {
				return ""
			}
			for i := range weList.Items {
				candidate := &weList.Items[i]
				if candidate.Spec.RemediationRequestRef.Name != rrName {
					continue
				}
				we = *candidate
				return candidate.Spec.ClusterID
			}
			return ""
		}, timeout, interval).Should(Equal("standalone-declared-cluster"),
			"WorkflowExecution must preserve the catalog-declared cluster ID")

		Expect(we.Spec.WorkflowRef.ExecutionEngine).To(Equal("job"))
		Expect(we.Spec.WorkflowRef.WorkflowName).To(Equal("standalone-exec-cluster-id-v1"))
		Expect(we.Spec.WorkflowRef.ActionType).To(Equal("IncreaseMemoryLimits"))
		Expect(we.Spec.WorkflowRef.Version).To(Equal("1.0.0"))
		Expect(we.Spec.WorkflowRef.ExecutionBundle).To(ContainSubstring("oomkill-increase-memory-job:v1.0.0-exec@sha256:"))
		Expect(we.Spec.WorkflowRef.ServiceAccountName).To(Equal("workflow-job-executor"))
		Expect(we.Spec.WorkflowRef.Dependencies.Secrets).To(ConsistOf(
			sharedtypes.WorkflowResourceDependency{Name: "e2e-2390-snapshot-secret"}))
		Expect(we.Spec.WorkflowRef.Resources.Requests[corev1.ResourceCPU]).To(Equal(resource.MustParse("10m")))
		Expect(we.Spec.WorkflowRef.Resources.Limits[corev1.ResourceMemory]).To(Equal(resource.MustParse("64Mi")))
		Expect(we.Spec.WorkflowRef.DeclaredParameterNames).To(Equal(map[string]bool{
			"TARGET_RESOURCE_NAME": true, "TARGET_RESOURCE_KIND": true,
			"TARGET_RESOURCE_NAMESPACE": true, "MEMORY_LIMIT_NEW": true,
		}))
		Eventually(func() bool {
			jobs := &batchv1.JobList{}
			if err := apiReader.List(ctx, jobs,
				client.InNamespace("kubernaut-workflows"),
				client.MatchingLabels{"kubernaut.ai/workflow-execution": we.Name}); err != nil {
				return false
			}
			for _, job := range jobs.Items {
				if job.Status.Succeeded > 0 {
					return true
				}
			}
			return false
		}, timeout, interval).Should(BeTrue(), "standalone execution must complete a local Kubernetes Job")
		fpWaitForWEComplete(rrName, timeout)
	})
})
