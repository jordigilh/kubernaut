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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	batchv1 "k8s.io/api/batch/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationworkflowv1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
)

// ========================================
// #243 / F1 / F8: Parameter Filtering Integration Tests
// ========================================
// Authority: #243 (Defense-in-depth parameter filtering), F1/F8 (coverage gap)
// Pattern: Real envtest K8s API + live RemediationWorkflow.spec.parameters
//          + real executor — verifies the full reconciler→resolveSchemaMetadata
//          →CreateOptions→executor→filtered resource pipeline.
// ========================================

var _ = Describe("#243: Parameter Filtering", Label("integration", "243"), func() {
	var workflowID string
	BeforeEach(func() {
		workflowID = trustedWorkflowFixtureWithParameters("Parameter filtering fixture", "job", []remediationworkflowv1.RemediationWorkflowParameter{
			{Name: "NAMESPACE", Type: "string", Description: "execution namespace"},
			{Name: "REPLICAS", Type: "integer", Description: "replica count"},
		})
	})

	Context("Job executor with live workflow parameter declarations", func() {

		It("IT-WE-243-001: should reject undeclared params before creating a Job", func() {
			wfe := createUniqueJobWFE("pf-job-strip", "default/deployment/pf-job-strip-app")
			wfe.Spec.WorkflowRef.WorkflowID = workflowID
			wfe.Spec.Parameters = typedParametersFromStrings(map[string]string{
				"NAMESPACE":    "default",
				"REPLICAS":     "3",
				"HALLUCINATED": "should-be-stripped",
				"INJECTED":     "malicious-value",
			})
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
			defer cleanupJobWFE(wfe)

			failed, err := waitForWFEPhase(wfe.Name, wfe.Namespace, "Failed", 15*time.Second)
			Expect(err).NotTo(HaveOccurred())
			Expect(failed.Status.FailureDetails).NotTo(BeNil())
			Expect(failed.Status.FailureDetails.Message).To(ContainSubstring("not declared"))
			var jobs batchv1.JobList
			Expect(k8sClient.List(ctx, &jobs, client.InNamespace(WorkflowExecutionNS), client.MatchingLabels{"kubernaut.ai/workflow-execution": wfe.Name})).To(Succeed())
			Expect(jobs.Items).To(BeEmpty())
		})

		It("IT-WE-243-002: BR-ORCH-025 fails closed for an unverifiable Job workflow", func() {
			wfe := createUniqueJobWFE("pf-job-nil", "default/deployment/pf-job-nil-app")
			wfe.Spec.WorkflowRef.WorkflowID = "missing-workflow"
			wfe.Spec.Parameters = typedParametersFromStrings(map[string]string{
				"NAMESPACE": "default",
				"CUSTOM":    "value",
			})
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
			defer cleanupJobWFE(wfe)

			expectWFEIdentityRejectedBeforeMutation(wfe.Name, wfe.Namespace)
			var jobs batchv1.JobList
			Expect(k8sClient.List(ctx, &jobs, client.InNamespace(WorkflowExecutionNS), client.MatchingLabels{"kubernaut.ai/workflow-execution": wfe.Name})).To(Succeed())
			Expect(jobs.Items).To(BeEmpty())
		})

		It("IT-WE-243-003: should dispatch params declared by the live workflow", func() {
			wfe := createUniqueJobWFE("pf-job-empty", "default/deployment/pf-job-empty-app")
			wfe.Spec.WorkflowRef.WorkflowID = workflowID
			wfe.Spec.Parameters = typedParametersFromStrings(map[string]string{
				"NAMESPACE": "default",
				"REPLICAS":  "3",
			})
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
			defer cleanupJobWFE(wfe)

			job, err := waitForJobCreation(wfe.Name, 15*time.Second)
			Expect(err).ToNot(HaveOccurred(), "Job should be created")

			envMap := make(map[string]string)
			for _, env := range job.Spec.Template.Spec.Containers[0].Env {
				envMap[env.Name] = env.Value
			}

			Expect(envMap).To(HaveKeyWithValue("NAMESPACE", "default"))
			Expect(envMap).To(HaveKeyWithValue("REPLICAS", "3"))
			Expect(envMap).To(HaveKey("TARGET_RESOURCE"), "System-injected TARGET_RESOURCE should always be present")
		})
	})

	Context("Tekton executor with live workflow parameter declarations", func() {

		It("IT-WE-243-010: should reject undeclared params before creating a PipelineRun", func() {
			wfe := createUniqueWFEWithTypedParams("pf-tekton-strip", "default/deployment/pf-tekton-strip-app",
				typedParametersFromStrings(map[string]string{
					"NAMESPACE":    "default",
					"REPLICAS":     "3",
					"HALLUCINATED": "should-be-stripped",
				}))
			wfe.Spec.WorkflowRef.WorkflowID = workflowID
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
			defer cleanupWFE(wfe)

			failed, err := waitForWFEPhase(wfe.Name, wfe.Namespace, "Failed", 15*time.Second)
			Expect(err).NotTo(HaveOccurred())
			Expect(failed.Status.FailureDetails).NotTo(BeNil())
			Expect(failed.Status.FailureDetails.Message).To(ContainSubstring("not declared"))
			var runs tektonv1.PipelineRunList
			Expect(k8sClient.List(ctx, &runs, client.InNamespace(WorkflowExecutionNS), client.MatchingLabels{"kubernaut.ai/workflow-execution": wfe.Name})).To(Succeed())
			Expect(runs.Items).To(BeEmpty())
		})

		It("IT-WE-243-011: BR-ORCH-025 fails closed for an unverifiable Tekton workflow", func() {
			wfe := createUniqueWFEWithTypedParams("pf-tekton-nil", "default/deployment/pf-tekton-nil-app",
				typedParametersFromStrings(map[string]string{
					"NAMESPACE": "default",
					"CUSTOM":    "value",
				}))
			wfe.Spec.WorkflowRef.WorkflowID = "missing-workflow"
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
			defer cleanupWFE(wfe)

			expectWFEIdentityRejectedBeforeMutation(wfe.Name, wfe.Namespace)
			var runs tektonv1.PipelineRunList
			Expect(k8sClient.List(ctx, &runs, client.InNamespace(WorkflowExecutionNS), client.MatchingLabels{"kubernaut.ai/workflow-execution": wfe.Name})).To(Succeed())
			Expect(runs.Items).To(BeEmpty())
		})
	})
})
