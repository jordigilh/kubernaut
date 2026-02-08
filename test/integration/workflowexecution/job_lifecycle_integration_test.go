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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
)

// Job Backend Integration Tests (BR-WE-014)
//
// These tests validate the Kubernetes Job execution backend with the real
// controller running against EnvTest (real K8s API server + etcd).
//
// Coverage:
// - IT-WE-014-001: Job lifecycle: Pending → Running → Completed
// - IT-WE-014-002: Job lifecycle: Pending → Running → Failed
// - IT-WE-014-003: Job spec correctness (labels, env vars, service account)
// - IT-WE-014-004: Job cleanup via finalizer during WFE deletion
// - IT-WE-014-005: Resource locking applies to Jobs (deterministic naming)
// - IT-WE-014-006: Executor dispatch selects Job backend for executionEngine="job"

var _ = Describe("Job Backend Lifecycle (BR-WE-014)", func() {

	Context("Job Creation and Status Sync", func() {

		It("should create a Job and transition to Running (IT-WE-014-001)", func() {
			targetResource := fmt.Sprintf("default/deployment/job-lifecycle-success-%d", time.Now().UnixNano())
			wfe := createUniqueJobWFE("success", targetResource)

			defer func() {
				cleanupJobWFE(wfe)
			}()

			By("Creating a WFE with executionEngine=job")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for controller to create a Job and transition to Running")
			Eventually(func() string {
				updated, err := getWFE(wfe.Name, wfe.Namespace)
				if err != nil {
					return ""
				}
				return string(updated.Status.Phase)
			}, 15*time.Second, 200*time.Millisecond).Should(Equal(string(workflowexecutionv1alpha1.PhaseRunning)),
				"Controller should dispatch to JobExecutor and create a Job")

			By("Verifying a Job was created in the execution namespace")
			job, err := waitForJobCreation(wfe.Name, 5*time.Second)
			Expect(err).ToNot(HaveOccurred(), "Job should be created in execution namespace")
			Expect(job).ToNot(BeNil())

			By("Verifying ExecutionRef is set on WFE status")
			updated, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(updated.Status.ExecutionRef).ToNot(BeNil(), "ExecutionRef must be set after Job creation")
			Expect(updated.Status.ExecutionRef.Name).To(Equal(job.Name))

			By("Simulating Job completion")
			Expect(simulateJobCompletion(job, true)).To(Succeed())

			By("Waiting for WFE to transition to Completed")
			Eventually(func() string {
				updated, err := getWFE(wfe.Name, wfe.Namespace)
				if err != nil {
					return ""
				}
				return string(updated.Status.Phase)
			}, 15*time.Second, 200*time.Millisecond).Should(Equal(string(workflowexecutionv1alpha1.PhaseCompleted)),
				"WFE should transition to Completed when Job succeeds")

			GinkgoWriter.Printf("✅ IT-WE-014-001: Job lifecycle Pending→Running→Completed passed\n")
		})

		It("should handle Job failure correctly (IT-WE-014-002)", func() {
			targetResource := fmt.Sprintf("default/deployment/job-lifecycle-fail-%d", time.Now().UnixNano())
			wfe := createUniqueJobWFE("failure", targetResource)

			defer func() {
				cleanupJobWFE(wfe)
			}()

			By("Creating a WFE with executionEngine=job")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for Running phase")
			Eventually(func() string {
				updated, err := getWFE(wfe.Name, wfe.Namespace)
				if err != nil {
					return ""
				}
				return string(updated.Status.Phase)
			}, 15*time.Second, 200*time.Millisecond).Should(Equal(string(workflowexecutionv1alpha1.PhaseRunning)))

			By("Getting the created Job")
			job, err := waitForJobCreation(wfe.Name, 5*time.Second)
			Expect(err).ToNot(HaveOccurred())
			Expect(job).ToNot(BeNil())

			By("Simulating Job failure")
			Expect(simulateJobCompletion(job, false)).To(Succeed())

			By("Waiting for WFE to transition to Failed")
			Eventually(func() string {
				updated, err := getWFE(wfe.Name, wfe.Namespace)
				if err != nil {
					return ""
				}
				return string(updated.Status.Phase)
			}, 15*time.Second, 200*time.Millisecond).Should(Equal(string(workflowexecutionv1alpha1.PhaseFailed)),
				"WFE should transition to Failed when Job fails")

			GinkgoWriter.Printf("✅ IT-WE-014-002: Job lifecycle Pending→Running→Failed passed\n")
		})
	})

	Context("Job Spec Correctness", func() {

		It("should create Job with correct labels and env vars (IT-WE-014-003)", func() {
			targetResource := fmt.Sprintf("default/deployment/job-spec-%d", time.Now().UnixNano())
			wfe := createUniqueJobWFE("spec-check", targetResource)
			wfe.Spec.Parameters = map[string]string{
				"REMEDIATION_TYPE": "restart",
				"TIMEOUT":          "300",
			}

			defer func() {
				cleanupJobWFE(wfe)
			}()

			By("Creating a WFE with parameters")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for Job creation")
			job, err := waitForJobCreation(wfe.Name, 15*time.Second)
			Expect(err).ToNot(HaveOccurred())
			Expect(job).ToNot(BeNil())

			By("Verifying Job labels")
			Expect(job.Labels).To(HaveKeyWithValue("kubernaut.ai/workflow-execution", wfe.Name))
			Expect(job.Labels).To(HaveKeyWithValue("kubernaut.ai/workflow-id", "test-workflow"))
			Expect(job.Labels).To(HaveKeyWithValue("kubernaut.ai/execution-engine", "job"))

			By("Verifying Job uses deterministic naming (DD-WE-003)")
			expectedName := executor.ExecutionResourceName(targetResource)
			Expect(job.Name).To(Equal(expectedName), "Job name must be deterministic for resource locking")

			By("Verifying container spec")
			Expect(job.Spec.Template.Spec.Containers).To(HaveLen(1))
			container := job.Spec.Template.Spec.Containers[0]
			Expect(container.Image).To(Equal("ghcr.io/kubernaut/workflows/test@sha256:abc123"))
			Expect(container.Name).To(Equal("workflow"))

			By("Verifying environment variables include TARGET_RESOURCE and parameters")
			envNames := make(map[string]string)
			for _, env := range container.Env {
				envNames[env.Name] = env.Value
			}
			Expect(envNames).To(HaveKeyWithValue("TARGET_RESOURCE", targetResource))
			Expect(envNames).To(HaveKeyWithValue("REMEDIATION_TYPE", "restart"))
			Expect(envNames).To(HaveKeyWithValue("TIMEOUT", "300"))

			By("Verifying service account")
			Expect(job.Spec.Template.Spec.ServiceAccountName).To(Equal("kubernaut-workflow-runner"))

			By("Verifying backoff limit is 0 (no retries)")
			Expect(job.Spec.BackoffLimit).ToNot(BeNil())
			Expect(*job.Spec.BackoffLimit).To(Equal(int32(0)))

			GinkgoWriter.Printf("✅ IT-WE-014-003: Job spec correctness verified\n")
		})
	})

	Context("Job Cleanup via Finalizer", func() {

		It("should clean up Job when WFE is deleted (IT-WE-014-004)", func() {
			targetResource := fmt.Sprintf("default/deployment/job-cleanup-%d", time.Now().UnixNano())
			wfe := createUniqueJobWFE("cleanup", targetResource)

			By("Creating a WFE with executionEngine=job")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for Running phase (Job created + finalizer added)")
			Eventually(func() string {
				updated, err := getWFE(wfe.Name, wfe.Namespace)
				if err != nil {
					return ""
				}
				return string(updated.Status.Phase)
			}, 15*time.Second, 200*time.Millisecond).Should(Equal(string(workflowexecutionv1alpha1.PhaseRunning)))

			By("Verifying Job exists")
			job, err := waitForJobCreation(wfe.Name, 5*time.Second)
			Expect(err).ToNot(HaveOccurred())
			Expect(job).ToNot(BeNil())

			By("Verifying finalizer is present")
			updated, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(updated.Finalizers).ToNot(BeEmpty(), "Finalizer must be present to ensure Job cleanup")

			By("Deleting the WFE")
			Expect(k8sClient.Delete(ctx, wfe)).To(Succeed())

			By("Waiting for WFE to be fully deleted (finalizer cleanup)")
			Eventually(func() bool {
				_, err := getWFE(wfe.Name, wfe.Namespace)
				return err != nil // Should return NotFound
			}, 15*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"WFE should be fully deleted after finalizer cleanup")

			By("Verifying Job was cleaned up")
			jobList := &batchv1.JobList{}
			Expect(k8sClient.List(ctx, jobList, client.InNamespace(WorkflowExecutionNS), client.MatchingLabels{
				"kubernaut.ai/workflow-execution": wfe.Name,
			})).To(Succeed())
			Expect(jobList.Items).To(BeEmpty(), "Job should be deleted during WFE cleanup")

			GinkgoWriter.Printf("✅ IT-WE-014-004: Job cleanup via finalizer passed\n")
		})
	})

	Context("Resource Locking (DD-WE-003)", func() {

		It("should use deterministic Job naming for resource locking (IT-WE-014-005)", func() {
			// Two WFEs targeting the same resource should produce the same Job name
			// This is how resource locking works - the second WFE gets an "AlreadyExists"
			// error when trying to create a Job with the same name
			targetResource := fmt.Sprintf("default/deployment/job-lock-test-%d", time.Now().UnixNano())

			wfe1 := createUniqueJobWFE("lock1", targetResource)
			defer func() {
				cleanupJobWFE(wfe1)
			}()

			By("Creating first WFE targeting the resource")
			Expect(k8sClient.Create(ctx, wfe1)).To(Succeed())

			By("Waiting for first WFE to reach Running (Job created)")
			Eventually(func() string {
				updated, err := getWFE(wfe1.Name, wfe1.Namespace)
				if err != nil {
					return ""
				}
				return string(updated.Status.Phase)
			}, 15*time.Second, 200*time.Millisecond).Should(Equal(string(workflowexecutionv1alpha1.PhaseRunning)))

			By("Verifying the Job name is deterministic")
			job, err := waitForJobCreation(wfe1.Name, 5*time.Second)
			Expect(err).ToNot(HaveOccurred())
			expectedName := executor.ExecutionResourceName(targetResource)
			Expect(job.Name).To(Equal(expectedName),
				"Job name must be deterministic based on target resource for resource locking")

			GinkgoWriter.Printf("✅ IT-WE-014-005: Deterministic Job naming for resource locking verified\n")
		})
	})

	Context("Executor Dispatch", func() {

		It("should select Job executor for executionEngine=job (IT-WE-014-006)", func() {
			targetResource := fmt.Sprintf("default/deployment/job-dispatch-%d", time.Now().UnixNano())
			wfe := createUniqueJobWFE("dispatch", targetResource)

			defer func() {
				cleanupJobWFE(wfe)
			}()

			By("Creating WFE with executionEngine=job")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Verifying a Job (not PipelineRun) was created")
			Eventually(func() bool {
				updated, err := getWFE(wfe.Name, wfe.Namespace)
				if err != nil {
					return false
				}
				return updated.Status.ExecutionRef != nil && updated.Status.ExecutionRef.Name != ""
			}, 15*time.Second, 200*time.Millisecond).Should(BeTrue())

			// Verify it's a Job, not a PipelineRun
			jobList := &batchv1.JobList{}
			Expect(k8sClient.List(ctx, jobList, client.InNamespace(WorkflowExecutionNS), client.MatchingLabels{
				"kubernaut.ai/workflow-execution": wfe.Name,
			})).To(Succeed())
			Expect(jobList.Items).To(HaveLen(1), "Exactly one Job should be created")
			Expect(jobList.Items[0].Labels["kubernaut.ai/execution-engine"]).To(Equal("job"))

			// Verify NO PipelineRun was created
			prList := &corev1.PodList{} // Using generic list since Tekton types share labels
			Expect(k8sClient.List(ctx, prList, client.InNamespace(WorkflowExecutionNS), client.MatchingLabels{
				"kubernaut.ai/workflow-execution": wfe.Name,
				"kubernaut.ai/execution-engine":   "tekton",
			})).To(Succeed())
			Expect(prList.Items).To(BeEmpty(), "No Tekton resources should be created for Job engine")

			GinkgoWriter.Printf("✅ IT-WE-014-006: Executor dispatch correctly selected Job backend\n")
		})
	})
})
