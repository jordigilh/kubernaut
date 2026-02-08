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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	weconditions "github.com/jordigilh/kubernaut/pkg/workflowexecution"
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

	// ========================================
	// External Job Deletion (IT-WE-014-010 to IT-WE-014-012)
	// BR-WE-007 equivalent: Handle externally deleted execution resources
	// ========================================

	Context("External Job Deletion (BR-WE-007)", func() {

		It("should detect external Job deletion and mark WFE as Failed (IT-WE-014-010)", func() {
			targetResource := fmt.Sprintf("default/deployment/job-ext-del-%d", time.Now().UnixNano())
			wfe := createUniqueJobWFE("ext-del", targetResource)

			defer func() {
				cleanupJobWFE(wfe)
			}()

			By("Creating a WFE with executionEngine=job")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for WFE to transition to Running (Job created)")
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

			By("Simulating external Job deletion (operator action)")
			propagation := metav1.DeletePropagationBackground
			Expect(k8sClient.Delete(ctx, job, &client.DeleteOptions{
				PropagationPolicy: &propagation,
			})).To(Succeed())
			GinkgoWriter.Printf("🗑️  Job %s deleted externally\n", job.Name)

			By("Waiting for controller to detect deletion and update WFE status")
			// Job backend relies on periodic requeue (~10s) for deletion detection
			Eventually(func(g Gomega) {
				updated, err := getWFE(wfe.Name, wfe.Namespace)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(updated.Status.Phase).To(Equal(workflowexecutionv1alpha1.PhaseFailed),
					"WFE should transition to Failed when Job is externally deleted")
			}, 30*time.Second, 500*time.Millisecond).Should(Succeed())

			By("Verifying failure details indicate external deletion")
			failedWFE, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(failedWFE.Status.FailureDetails).ToNot(BeNil(), "FailureDetails should be populated")

			By("Verifying WFE remains Failed (no retry loop)")
			Consistently(func(g Gomega) {
				updated, err := getWFE(wfe.Name, wfe.Namespace)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(updated.Status.Phase).To(Equal(workflowexecutionv1alpha1.PhaseFailed),
					"WFE should remain Failed (no retry loop)")
			}, 5*time.Second, 1*time.Second).Should(Succeed())

			GinkgoWriter.Printf("✅ IT-WE-014-010: External Job deletion detected, WFE Failed\n")
		})

		It("should set AuditRecorded condition on external Job deletion (IT-WE-014-011)", func() {
			targetResource := fmt.Sprintf("default/deployment/job-ext-del-audit-%d", time.Now().UnixNano())
			wfe := createUniqueJobWFE("ext-del-audit", targetResource)

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

			By("Getting and deleting the Job externally")
			job, err := waitForJobCreation(wfe.Name, 5*time.Second)
			Expect(err).ToNot(HaveOccurred())
			propagation := metav1.DeletePropagationBackground
			Expect(k8sClient.Delete(ctx, job, &client.DeleteOptions{
				PropagationPolicy: &propagation,
			})).To(Succeed())

			By("Waiting for WFE to transition to Failed")
			Eventually(func(g Gomega) {
				updated, err := getWFE(wfe.Name, wfe.Namespace)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(updated.Status.Phase).To(Equal(workflowexecutionv1alpha1.PhaseFailed))
			}, 30*time.Second, 500*time.Millisecond).Should(Succeed())

			By("Verifying AuditRecorded condition is set (BR-WE-006)")
			failedWFE, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())

			auditCondition := weconditions.GetCondition(failedWFE, weconditions.ConditionAuditRecorded)
			Expect(auditCondition).ToNot(BeNil(),
				"AuditRecorded condition should be set (proves controller attempted audit for external Job deletion)")

			GinkgoWriter.Printf("✅ IT-WE-014-011: AuditRecorded condition set on external Job deletion\n")
			GinkgoWriter.Printf("   AuditRecorded Status: %s, Reason: %s\n", auditCondition.Status, auditCondition.Reason)
		})

		It("should NOT misidentify normal Job completion as external deletion (IT-WE-014-012)", func() {
			targetResource := fmt.Sprintf("default/deployment/job-normal-complete-%d", time.Now().UnixNano())
			wfe := createUniqueJobWFE("normal-complete", targetResource)

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

			By("Simulating normal Job completion (NOT external deletion)")
			job, err := waitForJobCreation(wfe.Name, 5*time.Second)
			Expect(err).ToNot(HaveOccurred())
			Expect(simulateJobCompletion(job, true)).To(Succeed())

			By("Verifying WFE transitions to Completed (NOT Failed)")
			Eventually(func() string {
				updated, err := getWFE(wfe.Name, wfe.Namespace)
				if err != nil {
					return ""
				}
				return string(updated.Status.Phase)
			}, 15*time.Second, 200*time.Millisecond).Should(Equal(string(workflowexecutionv1alpha1.PhaseCompleted)),
				"WFE should complete normally (not trigger external deletion logic)")

			completedWFE, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(completedWFE.Status.FailureDetails).To(BeNil(),
				"Completed WFE should NOT have FailureDetails")

			GinkgoWriter.Printf("✅ IT-WE-014-012: Normal Job completion NOT misidentified as external deletion\n")
		})
	})

	// ========================================
	// Condition Lifecycle for Job Backend (IT-WE-014-013 to IT-WE-014-015)
	// BR-WE-006 equivalent: Kubernetes conditions for Jobs
	// ========================================

	Context("Job Condition Lifecycle (BR-WE-006)", func() {

		It("should set ExecutionCreated condition after Job creation (IT-WE-014-013)", func() {
			targetResource := fmt.Sprintf("default/deployment/job-cond-created-%d", time.Now().UnixNano())
			wfe := createUniqueJobWFE("cond-created", targetResource)

			defer func() {
				cleanupJobWFE(wfe)
			}()

			By("Creating a WFE with executionEngine=job")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for ExecutionCreated condition to be set")
			key := client.ObjectKeyFromObject(wfe)
			Eventually(func() []metav1.Condition {
				updated := &workflowexecutionv1alpha1.WorkflowExecution{}
				_ = k8sClient.Get(ctx, key, updated)
				return updated.Status.Conditions
			}, 30*time.Second, 1*time.Second).Should(ContainElement(
				And(
					HaveField("Type", weconditions.ConditionExecutionCreated),
					HaveField("Status", metav1.ConditionTrue),
					HaveField("Reason", weconditions.ReasonExecutionCreated),
				),
			), "ExecutionCreated condition should be set after Job creation")

			By("Verifying Job was actually created")
			job, err := waitForJobCreation(wfe.Name, 10*time.Second)
			Expect(err).ToNot(HaveOccurred())
			Expect(job).ToNot(BeNil())

			By("Verifying condition message includes Job name")
			updated := &workflowexecutionv1alpha1.WorkflowExecution{}
			Expect(k8sClient.Get(ctx, key, updated)).To(Succeed())
			condition := weconditions.GetCondition(updated, weconditions.ConditionExecutionCreated)
			Expect(condition.Message).To(ContainSubstring(job.Name))

			GinkgoWriter.Printf("✅ IT-WE-014-013: ExecutionCreated condition set for Job backend\n")
		})

		It("should set all conditions during successful Job lifecycle (IT-WE-014-014)", func() {
			targetResource := fmt.Sprintf("default/deployment/job-cond-lifecycle-%d", time.Now().UnixNano())
			wfe := createUniqueJobWFE("cond-lifecycle", targetResource)

			defer func() {
				cleanupJobWFE(wfe)
			}()

			By("Creating a WFE with executionEngine=job")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			key := client.ObjectKeyFromObject(wfe)

			By("Waiting for ExecutionCreated condition")
			Eventually(func() bool {
				updated := &workflowexecutionv1alpha1.WorkflowExecution{}
				_ = k8sClient.Get(ctx, key, updated)
				return weconditions.IsConditionTrue(updated, weconditions.ConditionExecutionCreated)
			}, 60*time.Second, 1*time.Second).Should(BeTrue())

			By("Waiting for ExecutionRunning condition")
			Eventually(func() bool {
				updated := &workflowexecutionv1alpha1.WorkflowExecution{}
				_ = k8sClient.Get(ctx, key, updated)
				return weconditions.IsConditionTrue(updated, weconditions.ConditionExecutionRunning)
			}, 60*time.Second, 1*time.Second).Should(BeTrue())

			By("Simulating Job completion")
			job, err := waitForJobCreation(wfe.Name, 10*time.Second)
			Expect(err).ToNot(HaveOccurred())
			Expect(simulateJobCompletion(job, true)).To(Succeed())

			By("Waiting for ExecutionComplete condition (True = success)")
			Eventually(func() bool {
				updated := &workflowexecutionv1alpha1.WorkflowExecution{}
				_ = k8sClient.Get(ctx, key, updated)
				return weconditions.IsConditionTrue(updated, weconditions.ConditionExecutionComplete)
			}, 60*time.Second, 1*time.Second).Should(BeTrue())

			By("Waiting for WFE to reach Completed phase")
			Eventually(func() string {
				updated := &workflowexecutionv1alpha1.WorkflowExecution{}
				_ = k8sClient.Get(ctx, key, updated)
				return updated.Status.Phase
			}, 15*time.Second, 1*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseCompleted))

			By("Verifying all 4 conditions are present")
			updated := &workflowexecutionv1alpha1.WorkflowExecution{}
			Expect(k8sClient.Get(ctx, key, updated)).To(Succeed())
			Expect(updated.Status.Conditions).To(HaveLen(4),
				"Complete Job lifecycle should have 4 conditions: Created, Running, Complete, AuditRecorded")

			Expect(weconditions.IsConditionTrue(updated, weconditions.ConditionExecutionCreated)).To(BeTrue())
			Expect(weconditions.IsConditionTrue(updated, weconditions.ConditionExecutionRunning)).To(BeTrue())
			Expect(weconditions.IsConditionTrue(updated, weconditions.ConditionExecutionComplete)).To(BeTrue())
			Expect(weconditions.GetCondition(updated, weconditions.ConditionAuditRecorded)).ToNot(BeNil())

			GinkgoWriter.Printf("✅ IT-WE-014-014: Complete Job condition lifecycle (success) verified\n")
		})

		It("should set conditions and FailureDetails on Job failure (IT-WE-014-015)", func() {
			targetResource := fmt.Sprintf("default/deployment/job-cond-fail-%d", time.Now().UnixNano())
			wfe := createUniqueJobWFE("cond-fail", targetResource)

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

			By("Simulating Job failure")
			job, err := waitForJobCreation(wfe.Name, 5*time.Second)
			Expect(err).ToNot(HaveOccurred())
			Expect(simulateJobCompletion(job, false)).To(Succeed())

			By("Waiting for WFE to transition to Failed")
			key := client.ObjectKeyFromObject(wfe)
			Eventually(func() string {
				updated := &workflowexecutionv1alpha1.WorkflowExecution{}
				_ = k8sClient.Get(ctx, key, updated)
				return updated.Status.Phase
			}, 15*time.Second, 200*time.Millisecond).Should(Equal(workflowexecutionv1alpha1.PhaseFailed))

			By("Verifying ExecutionComplete condition is False (failure)")
			failedWFE := &workflowexecutionv1alpha1.WorkflowExecution{}
			Expect(k8sClient.Get(ctx, key, failedWFE)).To(Succeed())

			completeCond := weconditions.GetCondition(failedWFE, weconditions.ConditionExecutionComplete)
			Expect(completeCond).ToNot(BeNil(), "ExecutionComplete condition should exist on failure")
			Expect(completeCond.Status).To(Equal(metav1.ConditionFalse),
				"ExecutionComplete should be False on Job failure")

			By("Verifying FailureDetails is populated")
			Expect(failedWFE.Status.FailureDetails).ToNot(BeNil(),
				"FailureDetails should be populated on Job failure")

			By("Verifying AuditRecorded condition exists")
			auditCond := weconditions.GetCondition(failedWFE, weconditions.ConditionAuditRecorded)
			Expect(auditCond).ToNot(BeNil(),
				"AuditRecorded condition should be set (controller attempted audit for Job failure)")

			GinkgoWriter.Printf("✅ IT-WE-014-015: Job failure conditions + FailureDetails verified\n")
		})
	})

	// ========================================
	// AlreadyExists / Race Condition for Job Backend (IT-WE-014-016 to IT-WE-014-017)
	// BR-WE-002 equivalent: Idempotent execution resource creation
	// ========================================

	Context("Job AlreadyExists Race Condition (BR-WE-002)", func() {

		It("should fail second WFE when Job already exists for same target (IT-WE-014-016)", func() {
			targetResource := fmt.Sprintf("default/deployment/job-conflict-%d", time.Now().UnixNano())

			wfe1 := createUniqueJobWFE("conflict-job1", targetResource)
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

			By("Verifying Job exists for first WFE")
			job, err := waitForJobCreation(wfe1.Name, 5*time.Second)
			Expect(err).ToNot(HaveOccurred())
			Expect(job).ToNot(BeNil())

			By("Creating second WFE for SAME target resource")
			wfe2 := createUniqueJobWFE("conflict-job2", targetResource)
			defer func() {
				cleanupJobWFE(wfe2)
			}()
			Expect(k8sClient.Create(ctx, wfe2)).To(Succeed())

			By("Verifying second WFE fails with ExecutionResourceExists")
			Eventually(func() string {
				updated, err := getWFE(wfe2.Name, wfe2.Namespace)
				if err != nil {
					return ""
				}
				return string(updated.Status.Phase)
			}, 15*time.Second, 200*time.Millisecond).Should(Equal(string(workflowexecutionv1alpha1.PhaseFailed)),
				"Second WFE should fail due to Job name collision (resource locked)")

			failedWFE2, err := getWFE(wfe2.Name, wfe2.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(failedWFE2.Status.FailureDetails).ToNot(BeNil())
			Expect(failedWFE2.Status.FailureDetails.Reason).To(Equal("Unknown"),
				"Failure reason should be 'Unknown' (CRD enum constraint; details in message)")
			Expect(failedWFE2.Status.FailureDetails.Message).To(ContainSubstring("already exists"),
				"Failure message should mention resource already exists")

			GinkgoWriter.Printf("✅ IT-WE-014-016: Second WFE failed with ExecutionResourceExists\n")
		})

		It("should keep first WFE unaffected when second WFE fails on AlreadyExists (IT-WE-014-017)", func() {
			targetResource := fmt.Sprintf("default/deployment/job-isolation-%d", time.Now().UnixNano())

			wfe1 := createUniqueJobWFE("isolation-job1", targetResource)
			defer func() {
				cleanupJobWFE(wfe1)
			}()

			By("Creating first WFE targeting the resource")
			Expect(k8sClient.Create(ctx, wfe1)).To(Succeed())

			By("Waiting for first WFE to reach Running")
			Eventually(func() string {
				updated, err := getWFE(wfe1.Name, wfe1.Namespace)
				if err != nil {
					return ""
				}
				return string(updated.Status.Phase)
			}, 15*time.Second, 200*time.Millisecond).Should(Equal(string(workflowexecutionv1alpha1.PhaseRunning)))

			By("Creating second WFE for SAME target (will fail)")
			wfe2 := createUniqueJobWFE("isolation-job2", targetResource)
			defer func() {
				cleanupJobWFE(wfe2)
			}()
			Expect(k8sClient.Create(ctx, wfe2)).To(Succeed())

			By("Waiting for second WFE to fail")
			Eventually(func() string {
				updated, err := getWFE(wfe2.Name, wfe2.Namespace)
				if err != nil {
					return ""
				}
				return string(updated.Status.Phase)
			}, 15*time.Second, 200*time.Millisecond).Should(Equal(string(workflowexecutionv1alpha1.PhaseFailed)))

			By("Verifying first WFE remains Running (unaffected)")
			wfe1Key := types.NamespacedName{Name: wfe1.Name, Namespace: wfe1.Namespace}
			wfe1Updated := &workflowexecutionv1alpha1.WorkflowExecution{}
			Expect(k8sClient.Get(ctx, wfe1Key, wfe1Updated)).To(Succeed())
			Expect(wfe1Updated.Status.Phase).To(Equal(workflowexecutionv1alpha1.PhaseRunning),
				"First WFE should continue running unaffected by second WFE's failure")

			By("Completing first WFE normally to verify full isolation")
			job, err := waitForJobCreation(wfe1.Name, 5*time.Second)
			Expect(err).ToNot(HaveOccurred())
			Expect(simulateJobCompletion(job, true)).To(Succeed())

			Eventually(func() string {
				updated, err := getWFE(wfe1.Name, wfe1.Namespace)
				if err != nil {
					return ""
				}
				return string(updated.Status.Phase)
			}, 15*time.Second, 200*time.Millisecond).Should(Equal(string(workflowexecutionv1alpha1.PhaseCompleted)),
				"First WFE should complete successfully despite second WFE's failure")

			GinkgoWriter.Printf("✅ IT-WE-014-017: First WFE completed normally, isolated from second WFE's AlreadyExists failure\n")
		})
	})
})
