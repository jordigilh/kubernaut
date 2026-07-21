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
	"sigs.k8s.io/controller-runtime/pkg/client"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	weconditions "github.com/jordigilh/kubernaut/pkg/workflowexecution"
	"github.com/jordigilh/kubernaut/test/infrastructure"

	"github.com/google/uuid"
)

// WorkflowExecution Job Backend E2E Tests (BR-WE-014)
//
// Test Plan: docs/testing/BR-WE-014/E2E_TEST_PLAN_BR_WE_014.md
// Test ID Convention: E2E-WE-014-{SEQUENCE}
//
// These tests validate the Kubernetes Job execution backend with real:
// - Kubernetes API (Kind cluster)
// - WorkflowExecution Controller (deployed with executor registry)
// - batchv1.Job (native K8s, no Tekton required)
//
// Per ADR-043 v1.1: Job is a V1 execution engine alongside Tekton
// Per BR-WE-014: K8s Job backend for single-step remediations

var _ = Describe("WorkflowExecution Job Backend E2E (BR-WE-014)", func() {

	// ========================================
	// P0: Core Job Backend Functionality
	// ========================================

	Context("E2E-WE-014-001: Job Lifecycle Success Path", func() {
		It("should execute Job workflow to completion (BR-WE-014, BR-WE-001)", func() {
			// Business Outcome: Job-based remediations complete successfully within SLA
			testName := fmt.Sprintf("e2e-job-success-%s", uuid.New().String()[:8])
			targetResource := fmt.Sprintf("default/deployment/job-test-%s", uuid.New().String()[:8])
			wfe := createTestJobWFE(testName, targetResource)

			defer func() {
				_ = deleteWFE(wfe)
			}()

			By("Creating a WorkflowExecution with executionEngine=job")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for transition to Running (Job created)")
			Eventually(func() string {
				updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
				if updated != nil {
					return updated.Status.Phase
				}
				return ""
			}, 60*time.Second, 2*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseRunning))

			GinkgoWriter.Println("Job WFE transitioned to Running")

			By("Verifying a Job was created (not a PipelineRun)")
			var jobList batchv1.JobList
			Eventually(func() int {
				err := k8sClient.List(ctx, &jobList, client.InNamespace(infrastructure.ExecutionNamespace))
				if err != nil {
					return 0
				}
				count := 0
				for _, job := range jobList.Items {
					if job.Labels["kubernaut.ai/workflow-execution"] == wfe.Name {
						count++
					}
				}
				return count
			}, 30*time.Second, 2*time.Second).Should(Equal(1), "Expected exactly 1 Job for this WFE")

			By("Waiting for Job workflow to complete")
			Eventually(func() bool {
				updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
				GinkgoWriter.Printf("🔍 poll: %s\n", wfeDiagnosticSnapshot(updated))
				if updated != nil {
					phase := updated.Status.Phase
					return phase == workflowexecutionv1alpha1.PhaseCompleted ||
						phase == workflowexecutionv1alpha1.PhaseFailed
				}
				return false
			}, 120*time.Second, 2*time.Second).Should(BeTrue(), "Job WFE should complete within SLA")

			By("Verifying successful completion")
			completed, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(completed.Status.Phase).To(Equal(workflowexecutionv1alpha1.PhaseCompleted),
				"Job hello-world should complete successfully")
			Expect(completed.Status.CompletionTime).NotTo(BeNil(), "CompletionTime should be set")

			// E2E-WE-163-001: Job backend has exactly 1 task
			Expect(completed.Status.ExecutionStatus).NotTo(BeNil())
			Expect(completed.Status.ExecutionStatus.CompletedTasks).To(Equal(1))

			By("Verifying Kubernetes Conditions are set (BR-WE-006)")
			Eventually(func() bool {
				updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
				if updated == nil {
					return false
				}
				hasCreated := weconditions.IsConditionTrue(updated, weconditions.ConditionExecutionCreated)
				hasComplete := weconditions.GetCondition(updated, weconditions.ConditionExecutionComplete) != nil
				return hasCreated && hasComplete
			}, 30*time.Second, 5*time.Second).Should(BeTrue(),
				"ExecutionCreated and ExecutionComplete conditions should be set")

			GinkgoWriter.Printf("E2E-WE-014-001: Job backend completed successfully - phase: %s\n", completed.Status.Phase)
		})
	})

	Context("E2E-WE-014-002: Job Lifecycle Failure Path", func() {
		It("should populate failure details when Job fails (BR-WE-014, BR-WE-004)", func() {
			// Business Outcome: Job failures produce actionable failure details
			testName := fmt.Sprintf("e2e-job-failure-%s", uuid.New().String()[:8])
			targetResource := fmt.Sprintf("default/deployment/job-fail-test-%s", uuid.New().String()[:8])

			// Issue #518: WorkflowID must be a valid UUID (resolved at runtime via DS)
			jobFailureUUID := infrastructure.RegisteredWorkflowUUIDs["test-job-intentional-failure"]
			Expect(jobFailureUUID).ToNot(BeEmpty(),
				"test-job-intentional-failure UUID should have been captured during workflow registration")

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: controllerNamespace,
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					RemediationRequestRef: corev1.ObjectReference{
						APIVersion: "remediationorchestrator.kubernaut.ai/v1alpha1",
						Kind:       "RemediationRequest",
						Name:       "test-rr-" + testName,
						Namespace:  controllerNamespace,
					},
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID: jobFailureUUID,
						Version:    "v1.0.0",
						// Spec seed — resolveExecutionBundle overrides from DS catalog
						// (job-failing:v1.0.0-exec, which exits non-zero)
						ExecutionBundle: fmt.Sprintf("%s/placeholder-execution:%s",
							infrastructure.TestWorkflowBundleRegistry, infrastructure.TestWorkflowBundleVersion),
					},
					TargetResource: targetResource,
					Parameters: map[string]string{
						"FAILURE_MESSAGE": "E2E test simulated Job failure",
					},
				},
			}

			defer func() {
				_ = deleteWFE(wfe)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for Job WFE to fail")
			Eventually(func() string {
				updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
				if updated != nil {
					return updated.Status.Phase
				}
				return ""
			}, 120*time.Second, 2*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseFailed))

			By("Verifying failure details are populated (BR-WE-004)")
			failed, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(failed.Status.FailureDetails).ToNot(BeNil(), "FailureDetails should be populated")
			Expect(failed.Status.FailureDetails.Message).ToNot(BeEmpty(), "Failure message should be set")

			By("Verifying ExecutionComplete condition reflects failure")
			completeCond := weconditions.GetCondition(failed, weconditions.ConditionExecutionComplete)
			Expect(completeCond).ToNot(BeNil(), "ExecutionComplete condition should exist")
			Expect(completeCond.Status).To(Equal(metav1.ConditionFalse),
				"ExecutionComplete should be False on Job failure")

			GinkgoWriter.Printf("E2E-WE-014-002: Job failure handled correctly\n")
			GinkgoWriter.Printf("   Failure reason: %s\n", failed.Status.FailureDetails.Reason)
			GinkgoWriter.Printf("   Failure message: %s\n", failed.Status.FailureDetails.Message)
		})
	})

	// ========================================
	// DD-WE-008 / BR-WE-019: Real-Cluster Proof of PodFailurePolicy
	// (Wiring Point B's actual GREEN gate -- envtest cannot exercise the
	// kubelet/Job-controller pod-failure-policy evaluation loop; see
	// DD-WE-008 Section 8)
	// ========================================

	Context("E2E-WE-019-001: PodFailurePolicy Tolerates OOM-Kill (exit 137)", func() {
		It("should let the real Job controller Ignore exit-137 pod failures while the WFE stays Running (BR-WE-019)", func() {
			// Business Outcome: transient OOM-kills during a remediation Job do not
			// prematurely fail the whole remediation -- the Job controller keeps
			// creating replacement Pods instead of counting the failure against
			// backoffLimit.
			//
			// DD-WE-008 Section 8: job-oomkill unconditionally exits 137 on every
			// attempt (no stateful retry-then-succeed logic) -- this test proves
			// tolerance within a short observation window and deletes the WFE
			// before ActiveDeadlineSeconds (30m) would otherwise eventually fail
			// it, deliberately avoiding a 30-minute CI run.
			testName := fmt.Sprintf("e2e-job-oomkill-%s", uuid.New().String()[:8])
			targetResource := fmt.Sprintf("default/deployment/job-oomkill-test-%s", uuid.New().String()[:8])

			jobOomkillUUID := infrastructure.RegisteredWorkflowUUIDs["test-job-oomkill"]
			Expect(jobOomkillUUID).ToNot(BeEmpty(),
				"test-job-oomkill UUID should have been captured during workflow registration")

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: controllerNamespace,
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					RemediationRequestRef: corev1.ObjectReference{
						APIVersion: "remediationorchestrator.kubernaut.ai/v1alpha1",
						Kind:       "RemediationRequest",
						Name:       "test-rr-" + testName,
						Namespace:  controllerNamespace,
					},
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID: jobOomkillUUID,
						Version:    "v1.0.0",
						// Spec seed -- resolveExecutionBundle overrides from DS catalog
						// (job-oomkill:v1.0.0-exec, which unconditionally exits 137)
						ExecutionBundle: fmt.Sprintf("%s/placeholder-execution:%s",
							infrastructure.TestWorkflowBundleRegistry, infrastructure.TestWorkflowBundleVersion),
					},
					TargetResource: targetResource,
					Parameters: map[string]string{
						"FAILURE_MESSAGE": "E2E-WE-019-001 simulated OOM-kill",
					},
				},
			}

			defer func() {
				_ = deleteWFE(wfe)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for the WFE to transition to Running (Job created)")
			Eventually(func() string {
				updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
				if updated != nil {
					return updated.Status.Phase
				}
				return ""
			}, 60*time.Second, 2*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseRunning))

			By("Locating the created Job")
			var jobList batchv1.JobList
			Eventually(func() int {
				err := k8sClient.List(ctx, &jobList,
					client.InNamespace(infrastructure.ExecutionNamespace),
					client.MatchingLabels{"kubernaut.ai/workflow-execution": wfe.Name})
				if err != nil {
					return 0
				}
				return len(jobList.Items)
			}, 30*time.Second, 2*time.Second).Should(Equal(1), "Expected exactly 1 Job for this WFE")
			jobName := jobList.Items[0].Name

			By("Verifying the real Job controller tolerates >= 2 exit-137 pod failures (PodFailurePolicy Ignore in effect)")
			// NOT job.Status.Failed: k8s.io/api batch/v1's
			// PodFailurePolicyActionIgnore doc comment states the counter
			// towards .backoffLimit (job.Status.Failed itself) "is not
			// incremented" for Ignore-action failures -- confirmed
			// empirically via a real-cluster spike (DD-WE-008 Section 8),
			// which is what first caught this test's original incorrect
			// assertion. Instead, count "SuccessfulCreate" Events on the Job
			// (one per Pod creation, including every Ignore-tolerated
			// replacement) -- the same signal JobExecutor.GetStatus uses to
			// compute BR-WE-019 AC10's RetryCount.
			Eventually(func() int32 {
				var events corev1.EventList
				if err := k8sClient.List(ctx, &events, client.InNamespace(infrastructure.ExecutionNamespace)); err != nil {
					return 0
				}
				var totalPodCreations int32
				for _, evt := range events.Items {
					if evt.InvolvedObject.Kind != "Job" || evt.InvolvedObject.Name != jobName || evt.Reason != "SuccessfulCreate" {
						continue
					}
					count := evt.Count
					if count == 0 {
						count = 1
					}
					totalPodCreations += count
				}
				return totalPodCreations
			}, 90*time.Second, 2*time.Second).Should(BeNumerically(">=", 3),
				"BR-WE-019: PodFailurePolicy must Ignore exit-137 pod failures, allowing >= 3 pod-creation attempts (initial + >=2 tolerated replacements) within the observation window")

			By("Verifying job.Status.Failed stays 0 throughout (Ignore-action failures are never counted there)")
			var job batchv1.Job
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: jobName, Namespace: infrastructure.ExecutionNamespace}, &job)).To(Succeed())
			Expect(job.Status.Failed).To(Equal(int32(0)),
				"regression guard: confirms job.Status.Failed is NOT a viable retry-tolerance signal for Ignore-action failures")

			By("Verifying the WFE is still Running (not Failed) despite the tolerated pod failures")
			stillRunning, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(stillRunning.Status.Phase).To(Equal(workflowexecutionv1alpha1.PhaseRunning),
				"BR-WE-019: tolerated OOM-kill pod failures must not fail the WFE -- PodFailurePolicy Ignore must keep it Running")

			GinkgoWriter.Printf("E2E-WE-019-001: real Job controller tolerated exit-137 pod failures, WFE remained Running\n")
		})
	})

	Context("E2E-WE-019-002: PodFailurePolicy Does Not Weaken Fail-Fast for Genuine Failures", func() {
		It("should let the real Job controller count exactly 1 genuine (non-tolerated) failure and reach Failed (BR-WE-019 regression guard)", func() {
			// Business Outcome: Phase 4's unconditional PodFailurePolicy addition
			// must not loosen today's fail-fast behavior for failure causes other
			// than exit-137/DisruptionTarget -- backoffLimit: 0 still applies.
			// Reuses the existing test-job-intentional-failure fixture (exit 1,
			// no new image) per DD-WE-008 Section 8.
			testName := fmt.Sprintf("e2e-job-genuine-failure-%s", uuid.New().String()[:8])
			targetResource := fmt.Sprintf("default/deployment/job-genuine-fail-test-%s", uuid.New().String()[:8])

			jobFailureUUID := infrastructure.RegisteredWorkflowUUIDs["test-job-intentional-failure"]
			Expect(jobFailureUUID).ToNot(BeEmpty(),
				"test-job-intentional-failure UUID should have been captured during workflow registration")

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: controllerNamespace,
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					RemediationRequestRef: corev1.ObjectReference{
						APIVersion: "remediationorchestrator.kubernaut.ai/v1alpha1",
						Kind:       "RemediationRequest",
						Name:       "test-rr-" + testName,
						Namespace:  controllerNamespace,
					},
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID: jobFailureUUID,
						Version:    "v1.0.0",
						ExecutionBundle: fmt.Sprintf("%s/placeholder-execution:%s",
							infrastructure.TestWorkflowBundleRegistry, infrastructure.TestWorkflowBundleVersion),
					},
					TargetResource: targetResource,
					Parameters: map[string]string{
						"FAILURE_MESSAGE": "E2E-WE-019-002 genuine failure regression guard",
					},
				},
			}

			defer func() {
				_ = deleteWFE(wfe)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for the WFE to reach Failed")
			Eventually(func() string {
				updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
				if updated != nil {
					return updated.Status.Phase
				}
				return ""
			}, 120*time.Second, 2*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseFailed))

			By("Verifying the Job recorded exactly 1 failure (fail-fast unchanged, BackoffLimit: 0)")
			var jobList batchv1.JobList
			Expect(k8sClient.List(ctx, &jobList,
				client.InNamespace(infrastructure.ExecutionNamespace),
				client.MatchingLabels{"kubernaut.ai/workflow-execution": wfe.Name})).To(Succeed())
			Expect(jobList.Items).To(HaveLen(1), "Expected exactly 1 Job for this WFE")
			Expect(jobList.Items[0].Status.Failed).To(Equal(int32(1)),
				"BR-WE-019: PodFailurePolicy's unconditional addition must not weaken fail-fast for a genuine (non-tolerated) failure")

			GinkgoWriter.Printf("E2E-WE-019-002: genuine failure still fails fast with exactly 1 recorded pod failure\n")
		})
	})

	Context("E2E-WE-014-003: Job Status Sync", func() {
		It("should sync WFE status with Job status accurately (BR-WE-014, BR-WE-003)", func() {
			// Business Outcome: WFE status accurately reflects Job execution state
			testName := fmt.Sprintf("e2e-job-sync-%s", uuid.New().String()[:8])
			targetResource := fmt.Sprintf("default/deployment/job-sync-test-%s", uuid.New().String()[:8])
			wfe := createTestJobWFE(testName, targetResource)

			defer func() {
				_ = deleteWFE(wfe)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Verifying WFE tracks execution reference after Running")
			Eventually(func() bool {
				updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
				if updated != nil && updated.Status.Phase == workflowexecutionv1alpha1.PhaseRunning {
					return updated.Status.ExecutionRef != nil
				}
				return false
			}, 60*time.Second, 2*time.Second).Should(BeTrue(), "WFE should track Job reference")

			runningWFE, _ := getWFE(wfe.Name, wfe.Namespace)
			Expect(runningWFE.Status.ExecutionRef).NotTo(BeNil(), "ExecutionRef should be set while running")
			GinkgoWriter.Printf("WFE tracks Job: %s\n", runningWFE.Status.ExecutionRef.Name)

			By("Waiting for completion")
			Eventually(func() bool {
				updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
				GinkgoWriter.Printf("🔍 poll: %s\n", wfeDiagnosticSnapshot(updated))
				if updated != nil {
					phase := updated.Status.Phase
					return phase == workflowexecutionv1alpha1.PhaseCompleted ||
						phase == workflowexecutionv1alpha1.PhaseFailed
				}
				return false
			}, 120*time.Second, 2*time.Second).Should(BeTrue())

			By("Verifying timing fields for SLA tracking")
			completedWFE, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(completedWFE.Status.StartTime).ToNot(BeNil(),
				"StartTime should be set for SLA calculation")
			Expect(completedWFE.Status.CompletionTime).ToNot(BeNil(),
				"CompletionTime should be set for SLA calculation")
			Expect(completedWFE.Status.Duration).ToNot(BeNil(),
				"Duration should be set for metrics")

			GinkgoWriter.Printf("E2E-WE-014-003: Job status sync complete\n")
			GinkgoWriter.Printf("   StartTime: %v\n", completedWFE.Status.StartTime.Time)
			GinkgoWriter.Printf("   CompletionTime: %v\n", completedWFE.Status.CompletionTime.Time)
			GinkgoWriter.Printf("   Duration: %v\n", completedWFE.Status.Duration)
		})
	})

	Context("E2E-WE-014-004: Job Spec Correctness", func() {
		It("should create Job with correct labels, env vars, and image (BR-WE-014)", func() {
			// Business Outcome: Created Jobs match expected spec for observability and traceability
			testName := fmt.Sprintf("e2e-job-spec-%s", uuid.New().String()[:8])
			targetResource := fmt.Sprintf("default/deployment/job-spec-test-%s", uuid.New().String()[:8])
			wfe := createTestJobWFE(testName, targetResource)

			defer func() {
				_ = deleteWFE(wfe)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for Job creation (Running phase)")
			Eventually(func() string {
				updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
				if updated != nil {
					return updated.Status.Phase
				}
				return ""
			}, 60*time.Second, 2*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseRunning))

			By("Fetching the created Job from execution namespace")
			var jobList batchv1.JobList
			Eventually(func() int {
				err := k8sClient.List(ctx, &jobList,
					client.InNamespace(infrastructure.ExecutionNamespace),
					client.MatchingLabels{"kubernaut.ai/workflow-execution": wfe.Name})
				if err != nil {
					return 0
				}
				return len(jobList.Items)
			}, 30*time.Second, 2*time.Second).Should(Equal(1), "Expected exactly 1 Job for this WFE")

			job := jobList.Items[0]

			By("Verifying Job labels")
			Expect(job.Labels).To(HaveKeyWithValue("kubernaut.ai/workflow-execution", wfe.Name),
				"Job should have workflow-execution label")
			Expect(job.Labels).To(HaveKeyWithValue("kubernaut.ai/workflow-id", wfe.Spec.WorkflowRef.WorkflowID),
				"Job should have workflow-id label")
			Expect(job.Labels).To(HaveKeyWithValue("kubernaut.ai/execution-engine", "job"),
				"Job should have execution-engine=job label")

			By("Verifying Job container image")
			Expect(job.Spec.Template.Spec.Containers).To(HaveLen(1))
			container := job.Spec.Template.Spec.Containers[0]
			Expect(container.Image).To(HavePrefix(wfe.Spec.WorkflowRef.ExecutionBundle),
				"Job container image should start with WFE spec bundle (DS resolution may append @sha256: digest)")

			By("Verifying environment variables include parameters")
			envNames := make(map[string]string)
			for _, env := range container.Env {
				envNames[env.Name] = env.Value
			}
			Expect(envNames).To(HaveKeyWithValue("TARGET_RESOURCE", targetResource),
				"Job should have TARGET_RESOURCE env var")
			Expect(envNames).To(HaveKey("MESSAGE"),
				"Job should have MESSAGE parameter as env var")

			By("Verifying Job spec configuration")
			Expect(job.Spec.BackoffLimit).To(HaveValue(BeNumerically(">=", 0)))
			Expect(*job.Spec.BackoffLimit).To(Equal(int32(0)),
				"Job backoff limit should be 0 (no retries)")
			Expect(job.Spec.Template.Spec.RestartPolicy).To(Equal(corev1.RestartPolicyNever),
				"Pod restart policy should be Never")

			GinkgoWriter.Printf("E2E-WE-014-004: Job spec validated\n")
			GinkgoWriter.Printf("   Job name: %s\n", job.Name)
			GinkgoWriter.Printf("   Labels: %d\n", len(job.Labels))
			GinkgoWriter.Printf("   Env vars: %d\n", len(container.Env))
		})
	})

	// ========================================
	// P1: Edge Cases and Cross-Cutting Concerns
	// ========================================

	// E2E-WE-014-005 ("Invalid executionEngine rejected by API server") is intentionally
	// absent: ExecutionEngine was removed from WorkflowExecutionSpec, so the CRD-level
	// admission scenario this test ID originally covered no longer applies. The
	// replacement runtime behavior (WE controller rejects an unsupported engine resolved
	// from DataStorage/ExecutorRegistry) is already proven without a permanent Skip():
	//   - UT-WE-659-002 (pkg/workflowexecution/controller_events_test.go) proves the logic
	//   - IT-WE-015-001 (test/integration/workflowexecution/ansible_dispatch_integration_test.go)
	//     proves the controller wiring via envtest with a real reconciler
	// See docs/testing/BR-WE-014/E2E_TEST_PLAN_BR_WE_014.md for the updated scope note.

	Context("E2E-WE-014-006: Deterministic Job Naming for Resource Locking", func() {
		It("should use deterministic Job name based on targetResource (BR-WE-014, BR-WE-009)", func() {
			// Business Outcome: Deterministic Job naming enables resource locking
			// via AlreadyExists errors (DD-WE-003: Lock Persistence via Deterministic Name).
			//
			// Note: The full concurrent resource locking scenario (two WFEs competing for the
			// same Job name) is validated in integration tests (IT-WE-014-xxx) where timing
			// can be controlled. The E2E test validates the deterministic naming mechanism
			// that enables locking: same targetResource → same Job name.
			testName := fmt.Sprintf("e2e-job-naming-%s", uuid.New().String()[:8])
			targetResource := fmt.Sprintf("default/deployment/naming-test-%s", uuid.New().String()[:8])
			wfe := createTestJobWFE(testName, targetResource)

			defer func() {
				_ = deleteWFE(wfe)
			}()

			By("Creating a WFE with a known targetResource")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for Job creation (Running phase)")
			Eventually(func() string {
				updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
				if updated != nil {
					return updated.Status.Phase
				}
				return ""
			}, 60*time.Second, 2*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseRunning))

			By("Verifying the Job name follows the deterministic naming convention")
			var jobList batchv1.JobList
			Eventually(func() int {
				err := k8sClient.List(ctx, &jobList,
					client.InNamespace(infrastructure.ExecutionNamespace),
					client.MatchingLabels{"kubernaut.ai/workflow-execution": wfe.Name})
				if err != nil {
					return 0
				}
				return len(jobList.Items)
			}, 30*time.Second, 2*time.Second).Should(Equal(1))

			job := jobList.Items[0]

			// DD-WE-003: Job name format is wfe-<sha256(targetResource)[:16]>
			Expect(job.Name).To(HavePrefix("wfe-"),
				"Job name should follow deterministic naming convention (wfe- prefix)")
			Expect(len(job.Name)).To(Equal(20),
				"Job name should be wfe- + 16 hex chars = 20 chars total")

			By("Verifying ExecutionRef in WFE status matches the deterministic Job name")
			running, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(running.Status.ExecutionRef).NotTo(BeNil(), "ExecutionRef should be set while running")
			Expect(running.Status.ExecutionRef.Name).To(Equal(job.Name),
				"WFE ExecutionRef should reference the deterministic Job name")

			GinkgoWriter.Printf("E2E-WE-014-006: Deterministic Job naming validated\n")
			GinkgoWriter.Printf("   Target resource: %s\n", targetResource)
			GinkgoWriter.Printf("   Deterministic Job name: %s\n", job.Name)
			GinkgoWriter.Printf("   ExecutionRef matches: %t\n", running.Status.ExecutionRef.Name == job.Name)
		})
	})

	Context("E2E-WE-014-007: External Job Deletion Handling", func() {
		It("should mark WFE as Failed when Job is deleted externally (BR-WE-014, BR-WE-007)", func() {
			// Business Outcome: WFE detects externally deleted Jobs and fails gracefully
			testName := fmt.Sprintf("e2e-job-extdel-%s", uuid.New().String()[:8])
			targetResource := fmt.Sprintf("default/deployment/job-extdel-test-%s", uuid.New().String()[:8])
			wfe := createTestJobWFE(testName, targetResource)

			defer func() {
				_ = deleteWFE(wfe)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for Running phase (Job created)")
			Eventually(func() string {
				updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
				if updated != nil {
					return updated.Status.Phase
				}
				return ""
			}, 60*time.Second, 2*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseRunning))

			GinkgoWriter.Println("WFE is Running, Job exists")

			By("Finding the created Job")
			var jobList batchv1.JobList
			Eventually(func() int {
				err := k8sClient.List(ctx, &jobList,
					client.InNamespace(infrastructure.ExecutionNamespace),
					client.MatchingLabels{"kubernaut.ai/workflow-execution": wfe.Name})
				if err != nil {
					return 0
				}
				return len(jobList.Items)
			}, 30*time.Second, 2*time.Second).Should(Equal(1))

			targetJob := &jobList.Items[0]
			GinkgoWriter.Printf("Deleting Job %s externally...\n", targetJob.Name)

			By("Deleting the Job externally (simulating operator action)")
			propagation := metav1.DeletePropagationBackground
			Expect(k8sClient.Delete(ctx, targetJob, &client.DeleteOptions{
				PropagationPolicy: &propagation,
			})).To(Succeed())

			By("Waiting for WFE to detect deletion and mark as Failed")
			Eventually(func() string {
				updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
				if updated != nil {
					return updated.Status.Phase
				}
				return ""
			}, 60*time.Second, 2*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseFailed))

			By("Verifying failure details explain the external deletion")
			failed, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(failed.Status.FailureDetails).NotTo(BeNil(), "FailureDetails should be populated on failure")

			GinkgoWriter.Printf("E2E-WE-014-007: External Job deletion handled correctly\n")
			GinkgoWriter.Printf("   Phase: %s\n", failed.Status.Phase)
			if failed.Status.FailureDetails != nil {
				GinkgoWriter.Printf("   Failure reason: %s\n", failed.Status.FailureDetails.Reason)
				GinkgoWriter.Printf("   Failure message: %s\n", failed.Status.FailureDetails.Message)
			}
		})
	})
})

// ========================================
// Test Helpers
// ========================================

// createTestJobWFE creates a WorkflowExecution for job-backend E2E (engine resolved from DS at runtime).
// Issue #518: WorkflowID must be a valid UUID (resolved at runtime by the WE controller via DS).
// Uses the pre-built placeholder-execution image (echoes params and exits 0).
func createTestJobWFE(name, targetResource string) *workflowexecutionv1alpha1.WorkflowExecution {
	jobHelloWorldUUID := infrastructure.RegisteredWorkflowUUIDs["test-job-hello-world"]
	Expect(jobHelloWorldUUID).ToNot(BeEmpty(),
		"test-job-hello-world UUID should have been captured during workflow registration")
	return &workflowexecutionv1alpha1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: controllerNamespace,
		},
		Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
			// Required reference to parent RemediationRequest
			RemediationRequestRef: corev1.ObjectReference{
				APIVersion: "remediationorchestrator.kubernaut.ai/v1alpha1",
				Kind:       "RemediationRequest",
				Name:       "test-rr-" + name,
				Namespace:  controllerNamespace,
			},
			WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
				WorkflowID: jobHelloWorldUUID,
				Version:    "v1.0.0",
				ExecutionBundle: fmt.Sprintf("%s/placeholder-execution:%s",
					infrastructure.TestWorkflowBundleRegistry, infrastructure.TestWorkflowBundleVersion),
			},
			TargetResource: targetResource,
			Parameters: map[string]string{
				"MESSAGE": "E2E Job test message",
			},
		},
	}
}
