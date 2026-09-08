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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// E2E-FP-2378-001 proves that catalog-declared execution metadata survives the
// full selection path, while standalone WorkflowExecution dispatch remains on
// the local Kubernetes client.
var _ = Describe("Standalone catalog execution cluster [BR-FLEET-054]", func() {
	It("preserves the declared cluster ID and completes a local Job execution", func() {
		Expect(workflowUUIDs).To(HaveKey("standalone-exec-cluster-id-v1:production"))

		rrName := fpPostSignalToGateway(
			"StandaloneExecutionCluster2378",
			"memory-eater",
			namespace,
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
