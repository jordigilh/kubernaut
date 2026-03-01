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
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"

	"github.com/google/uuid"
)

// ========================================
// DD-WE-006: Schema-Declared Dependency Injection E2E Tests
// ========================================
// Authority: DD-WE-006, BR-WE-014, BR-WORKFLOW-004
// Test Plan: docs/testing/DD-WE-006/TEST_PLAN.md
//
// These tests validate the full DS → WFE dependency injection pipeline:
// 1. Workflow registered in DS with dependencies in schema
// 2. WFE controller fetches dependencies from DS via WorkflowQuerier
// 3. DependencyValidator confirms resources exist in execution namespace
// 4. Executor creates Job/PipelineRun with correct volume mounts/workspace bindings
//
// Prerequisites (created in infrastructure setup):
// - Secret "e2e-dep-secret" in kubernaut-workflows namespace
// - Workflow "test-dep-secret-job" registered in DS (schema declares the secret)
// - Workflow "test-dep-secret-tekton" registered in DS (schema declares the secret)
// ========================================

var _ = Describe("DD-WE-006: Schema-Declared Dependency Injection E2E", Serial, func() {

	Context("E2E-WE-006-001/002: Job dependency injection success and post-registration drift failure", func() {
		It("should mount declared secret in Job, then fail with ConfigurationError when secret is deleted", func() {
			depSecretJobUUID := infrastructure.RegisteredWorkflowUUIDs["test-dep-secret-job"]
			Expect(depSecretJobUUID).ToNot(BeEmpty(),
				"test-dep-secret-job UUID should have been captured during workflow registration")

			// --- Phase 1: E2E-WE-006-001 (success path) ---
			testName1 := fmt.Sprintf("e2e-dep-inj-001-%s", uuid.New().String()[:8])
			targetResource1 := fmt.Sprintf("default/deployment/dep-inj-test-%s", uuid.New().String()[:8])

			wfe1 := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName1,
					Namespace: controllerNamespace,
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					ExecutionEngine: "job",
					RemediationRequestRef: corev1.ObjectReference{
						APIVersion: "remediationorchestrator.kubernaut.ai/v1alpha1",
						Kind:       "RemediationRequest",
						Name:       "test-rr-" + testName1,
						Namespace:  controllerNamespace,
					},
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID: depSecretJobUUID,
						Version:    "v1.0.0",
						ExecutionBundle: fmt.Sprintf("%s/placeholder-execution:%s",
							infrastructure.TestWorkflowBundleRegistry, infrastructure.TestWorkflowBundleVersion),
					},
					TargetResource: targetResource1,
					Parameters: map[string]string{
						"MESSAGE": "DD-WE-006 E2E dependency injection test",
					},
				},
			}

			defer func() { _ = deleteWFE(wfe1) }()

			By("E2E-WE-006-001: Creating a WFE referencing a workflow with declared secret dependency")
			Expect(k8sClient.Create(ctx, wfe1)).To(Succeed())

			By("E2E-WE-006-001: Waiting for Running phase (Job created)")
			Eventually(func() string {
				updated, _ := getWFEDirect(wfe1.Name, wfe1.Namespace)
				if updated != nil {
					return updated.Status.Phase
				}
				return ""
			}, 60*time.Second, 2*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseRunning))

			By("E2E-WE-006-001: Fetching the created Job from execution namespace")
			var jobList1 batchv1.JobList
			Eventually(func() int {
				err := k8sClient.List(ctx, &jobList1,
					client.InNamespace(infrastructure.ExecutionNamespace),
					client.MatchingLabels{"kubernaut.ai/workflow-execution": wfe1.Name})
				if err != nil {
					return 0
				}
				return len(jobList1.Items)
			}, 30*time.Second, 2*time.Second).Should(Equal(1))

			job := jobList1.Items[0]

			By("E2E-WE-006-001: Verifying Job has a volume for the declared secret")
			Expect(job.Spec.Template.Spec.Volumes).To(ContainElement(
				HaveField("Name", "secret-e2e-dep-secret"),
			), "Job should have a volume named secret-e2e-dep-secret")

			By("E2E-WE-006-001: Verifying container mounts the secret at the DD-WE-006 convention path")
			Expect(job.Spec.Template.Spec.Containers).To(HaveLen(1))
			container := job.Spec.Template.Spec.Containers[0]
			Expect(container.VolumeMounts).To(ContainElement(And(
				HaveField("Name", "secret-e2e-dep-secret"),
				HaveField("MountPath", "/run/kubernaut/secrets/e2e-dep-secret"),
				HaveField("ReadOnly", true),
			)), "Secret should be mounted read-only at /run/kubernaut/secrets/e2e-dep-secret")

			GinkgoWriter.Printf("E2E-WE-006-001: Dependency injection validated\n")
			GinkgoWriter.Printf("   Workflow UUID: %s\n", depSecretJobUUID)
			GinkgoWriter.Printf("   Job name: %s\n", job.Name)

			// --- Phase 2: E2E-WE-006-002 (post-registration drift) ---
			// Delete the secret to simulate infrastructure drift after registration.
			// Since this runs within a single It block, no other test can observe the missing secret.
			By("E2E-WE-006-002: Deleting e2e-dep-secret to simulate post-registration removal")
			depSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-dep-secret",
					Namespace: infrastructure.ExecutionNamespace,
				},
			}
			Expect(k8sClient.Delete(ctx, depSecret)).To(Succeed())

			defer func() {
				By("Recreating e2e-dep-secret after test")
				recreated := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "e2e-dep-secret",
						Namespace: infrastructure.ExecutionNamespace,
					},
					Data: map[string][]byte{"token": []byte("e2e-test-value")},
				}
				_ = k8sClient.Create(ctx, recreated)
			}()

			testName2 := fmt.Sprintf("e2e-dep-inj-002-%s", uuid.New().String()[:8])
			targetResource2 := fmt.Sprintf("default/deployment/dep-inj-fail-%s", uuid.New().String()[:8])

			wfe2 := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName2,
					Namespace: controllerNamespace,
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					ExecutionEngine: "job",
					RemediationRequestRef: corev1.ObjectReference{
						APIVersion: "remediationorchestrator.kubernaut.ai/v1alpha1",
						Kind:       "RemediationRequest",
						Name:       "test-rr-" + testName2,
						Namespace:  controllerNamespace,
					},
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID: depSecretJobUUID,
						Version:    "v1.0.0",
						ExecutionBundle: fmt.Sprintf("%s/placeholder-execution:%s",
							infrastructure.TestWorkflowBundleRegistry, infrastructure.TestWorkflowBundleVersion),
					},
					TargetResource: targetResource2,
				},
			}

			defer func() { _ = deleteWFE(wfe2) }()

			By("E2E-WE-006-002: Creating a WFE whose declared secret no longer exists")
			Expect(k8sClient.Create(ctx, wfe2)).To(Succeed())

			By("E2E-WE-006-002: Waiting for WFE to transition to Failed (dependency validation error)")
			Eventually(func() string {
				updated, _ := getWFEDirect(wfe2.Name, wfe2.Namespace)
				if updated != nil {
					return updated.Status.Phase
				}
				return ""
			}, 60*time.Second, 2*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseFailed))

			By("E2E-WE-006-002: Verifying failure reason is ConfigurationError")
			failed, err := getWFE(wfe2.Name, wfe2.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(failed.Status.FailureDetails).ToNot(BeNil(), "FailureDetails should be populated")
			Expect(failed.Status.FailureDetails.Reason).To(Equal(workflowexecutionv1alpha1.FailureReasonConfigurationError),
				"Failure reason should be ConfigurationError for missing dependency")

			GinkgoWriter.Printf("E2E-WE-006-002: Missing dependency correctly detected\n")
			GinkgoWriter.Printf("   Failure reason: %s\n", failed.Status.FailureDetails.Reason)
			GinkgoWriter.Printf("   Failure message: %s\n", failed.Status.FailureDetails.Message)
		})
	})

	Context("E2E-WE-006-003: No dependency volumes for workflows without dependencies", func() {
		It("should create Job without dependency volumes when workflow schema has no dependencies", func() {
			helloWorldUUID := infrastructure.RegisteredWorkflowUUIDs["test-hello-world"]
			Expect(helloWorldUUID).ToNot(BeEmpty(),
				"test-hello-world UUID should have been captured during workflow registration")

			testName := fmt.Sprintf("e2e-dep-inj-003-%s", uuid.New().String()[:8])
			targetResource := fmt.Sprintf("default/deployment/dep-inj-nodeps-%s", uuid.New().String()[:8])

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: controllerNamespace,
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					ExecutionEngine: "job",
					RemediationRequestRef: corev1.ObjectReference{
						APIVersion: "remediationorchestrator.kubernaut.ai/v1alpha1",
						Kind:       "RemediationRequest",
						Name:       "test-rr-" + testName,
						Namespace:  controllerNamespace,
					},
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID: helloWorldUUID,
						Version:    "v1.0.0",
						ExecutionBundle: fmt.Sprintf("%s/job-hello-world:%s",
							infrastructure.TestWorkflowBundleRegistry, infrastructure.TestWorkflowBundleVersion),
					},
					TargetResource: targetResource,
					Parameters: map[string]string{
						"MESSAGE": "E2E no-deps test",
					},
				},
			}
			defer func() { _ = deleteWFE(wfe) }()

			By("Creating a WFE for a workflow without dependencies")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for Running phase (Job created)")
			Eventually(func() string {
				updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
				if updated != nil {
					return updated.Status.Phase
				}
				return ""
			}, 60*time.Second, 2*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseRunning))

			By("Fetching the created Job")
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

			By("Verifying no dependency volumes are present")
			for _, v := range job.Spec.Template.Spec.Volumes {
				Expect(v.Name).ToNot(HavePrefix("secret-"),
					"no secret dependency volumes should be present")
				Expect(v.Name).ToNot(HavePrefix("configmap-"),
					"no configMap dependency volumes should be present")
			}

			GinkgoWriter.Printf("E2E-WE-006-003: No-dependency workflow confirmed\n")
			GinkgoWriter.Printf("   Workflow UUID: %s\n", helloWorldUUID)
			GinkgoWriter.Printf("   Job name: %s\n", job.Name)
			GinkgoWriter.Printf("   Volume count: %d (none should be dependency volumes)\n", len(job.Spec.Template.Spec.Volumes))
		})
	})

	Context("E2E-WE-006-004: Tekton PipelineRun with secret workspace binding", func() {
		It("should create PipelineRun with workspace binding for declared secret dependency", func() {
			depSecretTektonUUID := infrastructure.RegisteredWorkflowUUIDs["test-dep-secret-tekton"]
			Expect(depSecretTektonUUID).ToNot(BeEmpty(),
				"test-dep-secret-tekton UUID should have been captured during workflow registration")

			testName := fmt.Sprintf("e2e-dep-inj-004-%s", uuid.New().String()[:8])
			targetResource := fmt.Sprintf("default/deployment/dep-inj-tekton-%s", uuid.New().String()[:8])

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: controllerNamespace,
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					ExecutionEngine: "tekton",
					RemediationRequestRef: corev1.ObjectReference{
						APIVersion: "remediationorchestrator.kubernaut.ai/v1alpha1",
						Kind:       "RemediationRequest",
						Name:       "test-rr-" + testName,
						Namespace:  controllerNamespace,
					},
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID:      depSecretTektonUUID,
						Version:         "v1.0.0",
						ExecutionBundle: "quay.io/kubernaut-cicd/tekton-bundles/hello-world:v1.0.0",
					},
					TargetResource: targetResource,
					Parameters: map[string]string{
						"MESSAGE": "DD-WE-006 Tekton dependency injection test",
					},
				},
			}

			defer func() { _ = deleteWFE(wfe) }()

			By("Creating a Tekton WFE referencing a workflow with declared secret dependency")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for Running phase (PipelineRun created)")
			Eventually(func() string {
				updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
				if updated != nil {
					return updated.Status.Phase
				}
				return ""
			}, 60*time.Second, 2*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseRunning))

			By("Fetching the created PipelineRun from execution namespace")
			var prList tektonv1.PipelineRunList
			Eventually(func() int {
				err := k8sClient.List(ctx, &prList,
					client.InNamespace(infrastructure.ExecutionNamespace),
					client.MatchingLabels{"kubernaut.ai/workflow-execution": wfe.Name})
				if err != nil {
					return 0
				}
				return len(prList.Items)
			}, 30*time.Second, 2*time.Second).Should(Equal(1))

			pr := prList.Items[0]

			By("Verifying PipelineRun has a workspace binding for the declared secret")
			Expect(pr.Spec.Workspaces).To(ContainElement(And(
				HaveField("Name", "secret-e2e-dep-secret"),
				HaveField("Secret", Not(BeNil())),
			)), "PipelineRun should have workspace secret-e2e-dep-secret backed by Secret")

			secretWs := findWorkspace(pr.Spec.Workspaces, "secret-e2e-dep-secret")
			Expect(secretWs.Name).To(Equal("secret-e2e-dep-secret"), "workspace should exist")
			Expect(secretWs.Secret.SecretName).To(Equal("e2e-dep-secret"),
				"Workspace should reference Secret e2e-dep-secret")

			GinkgoWriter.Printf("E2E-WE-006-004: Tekton dependency injection validated\n")
			GinkgoWriter.Printf("   Workflow UUID: %s\n", depSecretTektonUUID)
			GinkgoWriter.Printf("   PipelineRun name: %s\n", pr.Name)
			GinkgoWriter.Printf("   Workspace count: %d\n", len(pr.Spec.Workspaces))
		})
	})
})

func findWorkspace(workspaces []tektonv1.WorkspaceBinding, name string) *tektonv1.WorkspaceBinding {
	for i := range workspaces {
		if workspaces[i].Name == name {
			return &workspaces[i]
		}
	}
	return nil
}
