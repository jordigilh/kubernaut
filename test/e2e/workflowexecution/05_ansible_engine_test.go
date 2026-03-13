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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	weconditions "github.com/jordigilh/kubernaut/pkg/workflowexecution"
	"github.com/jordigilh/kubernaut/test/infrastructure"

	"github.com/google/uuid"
)

// Ansible Engine E2E Tests (BR-WE-015)
//
// Validates the ansible execution backend with a real AWX instance deployed
// in the Kind cluster. AWX shares PostgreSQL and Redis with Kubernaut services.
//
// Test Plan: docs/testing/45/TEST_PLAN.md
// Infrastructure: test/infrastructure/awx_e2e.go
//
// Coverage:
//   E2E-WE-015-001: Ansible happy path — WFE with engine=ansible completes via AWX
//   E2E-WE-015-002: Ansible failure path — AWX playbook fails, WFE transitions to Failed
//   E2E-WE-015-003: Ansible status sync — ExecutionRef, StartTime, CompletionTime, Duration
//   E2E-WE-015-004: Ansible external cancellation — AWX job canceled via API, WFE Failed

var _ = Describe("Ansible Engine E2E [BR-WE-015]", func() {

	Context("Happy Path", func() {
		It("E2E-WE-015-001: should execute ansible workflow to completion via AWX", func() {
			testName := fmt.Sprintf("e2e-ansible-%s", uuid.New().String()[:8])
			targetResource := "default/deployment/ansible-target"
			wfe := createAnsibleWFE(testName, targetResource, "test-ansible-success",
				"playbooks/test-success.yml", "kubernaut-test-success")

			defer func() {
				_ = deleteWFE(wfe)
			}()

			By("Creating WorkflowExecution with engine=ansible")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Verifying WFE transitions to Running (AWX job launched)")
			Eventually(func() string {
				updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
				if updated != nil {
					return updated.Status.Phase
				}
				return ""
			}, 60*time.Second, 2*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseRunning),
				"WFE should transition to Running when AWX launches the job")

			GinkgoWriter.Println("WFE transitioned to Running (AWX job launched)")

			By("Verifying WFE completes successfully (AWX job finishes)")
			Eventually(func() string {
				updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
				if updated != nil {
					return updated.Status.Phase
				}
				return ""
			}, 180*time.Second, 5*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseCompleted),
				"WFE should transition to Completed after AWX job succeeds")

			By("Verifying completion metadata")
			completed, err := getWFEDirect(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(completed.Status.CompletionTime).NotTo(BeNil(), "CompletionTime should be set")
			Expect(completed.Status.Phase).To(Equal(workflowexecutionv1alpha1.PhaseCompleted))

			By("Verifying ExecutionCreated condition is set")
			Expect(weconditions.IsConditionTrue(completed, weconditions.ConditionExecutionCreated)).To(BeTrue(),
				"ExecutionCreated condition should be True (AWX job was created)")

			By("Verifying ExecutionComplete condition is set")
			Expect(weconditions.GetCondition(completed, weconditions.ConditionExecutionComplete)).NotTo(BeNil(),
				"ExecutionComplete condition should exist")

			GinkgoWriter.Printf("E2E-WE-015-001 passed: ansible WFE completed in %s\n",
				completed.Status.CompletionTime.Time.Sub(completed.CreationTimestamp.Time).Round(time.Second))
		})
	})

	Context("Failure Path", func() {
		It("E2E-WE-015-002: should populate failure details when AWX playbook fails", func() {
			testName := fmt.Sprintf("e2e-ansible-fail-%s", uuid.New().String()[:8])
			targetResource := fmt.Sprintf("default/deployment/ansible-fail-%s", uuid.New().String()[:8])
			wfe := createAnsibleWFE(testName, targetResource, "test-ansible-failure",
				"playbooks/test-failure.yml", "kubernaut-test-failure")

			defer func() {
				_ = deleteWFE(wfe)
			}()

			By("Creating WFE with intentionally-failing ansible playbook")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for WFE to transition to Failed via AWX")
			Eventually(func() string {
				updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
				if updated != nil {
					return updated.Status.Phase
				}
				return ""
			}, 180*time.Second, 2*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseFailed),
				"WFE should transition to Failed when AWX playbook fails")

			By("Verifying failure details are populated (BR-WE-004)")
			failed, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(failed.Status.FailureDetails).ToNot(BeNil(), "FailureDetails should be populated")
			Expect(failed.Status.FailureDetails.Message).ToNot(BeEmpty(), "Failure message should be set")

			By("Verifying ExecutionComplete condition reflects failure")
			completeCond := weconditions.GetCondition(failed, weconditions.ConditionExecutionComplete)
			Expect(completeCond).ToNot(BeNil(), "ExecutionComplete condition should exist")
			Expect(completeCond.Status).To(Equal(metav1.ConditionFalse),
				"ExecutionComplete should be False on AWX playbook failure")

			GinkgoWriter.Printf("E2E-WE-015-002 passed: AWX failure handled correctly\n")
			GinkgoWriter.Printf("   Failure reason: %s\n", failed.Status.FailureDetails.Reason)
			GinkgoWriter.Printf("   Failure message: %.200s\n", failed.Status.FailureDetails.Message)
		})
	})

	Context("Status Sync", func() {
		It("E2E-WE-015-003: should sync WFE status with AWX job status accurately", func() {
			testName := fmt.Sprintf("e2e-ansible-sync-%s", uuid.New().String()[:8])
			targetResource := fmt.Sprintf("default/deployment/ansible-sync-%s", uuid.New().String()[:8])
			wfe := createAnsibleWFE(testName, targetResource, "test-ansible-success",
				"playbooks/test-success.yml", "kubernaut-test-success")

			defer func() {
				_ = deleteWFE(wfe)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Verifying WFE tracks AWX execution reference after Running")
			Eventually(func() bool {
				updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
				if updated != nil && updated.Status.Phase == workflowexecutionv1alpha1.PhaseRunning {
					return updated.Status.ExecutionRef != nil
				}
				return false
			}, 60*time.Second, 2*time.Second).Should(BeTrue(), "WFE should track AWX job reference")

			runningWFE, _ := getWFE(wfe.Name, wfe.Namespace)
			Expect(runningWFE.Status.ExecutionRef).NotTo(BeNil(), "ExecutionRef should be set while running")
			Expect(runningWFE.Status.ExecutionRef.Name).To(HavePrefix("awx-job-"),
				"ExecutionRef should reference an AWX job")
			GinkgoWriter.Printf("WFE tracks AWX job: %s\n", runningWFE.Status.ExecutionRef.Name)

			By("Waiting for completion")
			Eventually(func() bool {
				updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
				if updated != nil {
					phase := updated.Status.Phase
					return phase == workflowexecutionv1alpha1.PhaseCompleted ||
						phase == workflowexecutionv1alpha1.PhaseFailed
				}
				return false
			}, 180*time.Second, 2*time.Second).Should(BeTrue())

			By("Verifying timing fields for SLA tracking")
			completedWFE, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(completedWFE.Status.StartTime).ToNot(BeNil(),
				"StartTime should be set for SLA calculation")
			Expect(completedWFE.Status.CompletionTime).ToNot(BeNil(),
				"CompletionTime should be set for SLA calculation")
			Expect(completedWFE.Status.Duration).ToNot(BeEmpty(),
				"Duration should be set for metrics")

			GinkgoWriter.Printf("E2E-WE-015-003 passed: AWX status sync verified\n")
			GinkgoWriter.Printf("   ExecutionRef: %s\n", completedWFE.Status.ExecutionRef.Name)
			GinkgoWriter.Printf("   StartTime: %v\n", completedWFE.Status.StartTime.Time)
			GinkgoWriter.Printf("   CompletionTime: %v\n", completedWFE.Status.CompletionTime.Time)
			GinkgoWriter.Printf("   Duration: %s\n", completedWFE.Status.Duration)
		})
	})

	Context("External Cancellation", func() {
		It("E2E-WE-015-004: should mark WFE as Failed when AWX job is canceled externally", func() {
			testName := fmt.Sprintf("e2e-ansible-cancel-%s", uuid.New().String()[:8])
			targetResource := fmt.Sprintf("default/deployment/ansible-cancel-%s", uuid.New().String()[:8])
			wfe := createAnsibleWFE(testName, targetResource, "test-ansible-success",
				"playbooks/test-success.yml", "kubernaut-test-success")

			defer func() {
				_ = deleteWFE(wfe)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("Waiting for WFE to reach Running (AWX job launched)")
			Eventually(func() bool {
				updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
				return updated != nil &&
					updated.Status.Phase == workflowexecutionv1alpha1.PhaseRunning &&
					updated.Status.ExecutionRef != nil
			}, 60*time.Second, 2*time.Second).Should(BeTrue(),
				"WFE should be Running with ExecutionRef set")

			runningWFE, err := getWFEDirect(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			GinkgoWriter.Printf("WFE Running, AWX ref: %s\n", runningWFE.Status.ExecutionRef.Name)

			By("Extracting AWX job ID from ExecutionRef")
			awxJobID := extractAWXJobID(runningWFE.Status.ExecutionRef.Name)
			GinkgoWriter.Printf("AWX job ID to cancel: %d\n", awxJobID)

			By("Canceling AWX job via AWX REST API (external cancellation)")
			cancelAWXJob(awxJobID)

			By("Verifying WFE transitions to Failed after external cancellation")
			Eventually(func() string {
				updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
				if updated != nil {
					return updated.Status.Phase
				}
				return ""
			}, 60*time.Second, 2*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseFailed),
				"WFE should transition to Failed when AWX job is canceled externally")

			By("Verifying failure details reference cancellation")
			failed, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(failed.Status.FailureDetails).ToNot(BeNil(), "FailureDetails should be populated")
			Expect(failed.Status.FailureDetails.Reason).To(
				Equal("TaskFailed"),
				"Failure reason should indicate AWX cancellation (mapped to CRD enum TaskFailed)")

			GinkgoWriter.Printf("E2E-WE-015-004 passed: external AWX cancellation handled\n")
			GinkgoWriter.Printf("   Failure reason: %s\n", failed.Status.FailureDetails.Reason)
			GinkgoWriter.Printf("   Failure message: %.200s\n", failed.Status.FailureDetails.Message)
		})
	})

	Context("Dependency Injection", func() {
		It("E2E-WE-015-005: should inject Secret as ephemeral AWX credential and complete", func() {
			depSecretAnsibleUUID := infrastructure.RegisteredWorkflowUUIDs["test-dep-secret-ansible"]
			Expect(depSecretAnsibleUUID).ToNot(BeEmpty(),
				"test-dep-secret-ansible UUID should have been captured during workflow registration")

			testName := fmt.Sprintf("e2e-ansible-dep-secret-%s", uuid.New().String()[:8])
			targetResource := fmt.Sprintf("default/deployment/ansible-dep-secret-%s", uuid.New().String()[:8])

			engineCfgJSON, err := json.Marshal(map[string]string{
				"playbookPath":    "playbooks/test-dep-secret.yml",
				"jobTemplateName": "kubernaut-test-dep-secret",
			})
			Expect(err).ToNot(HaveOccurred())

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: controllerNamespace,
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					ExecutionEngine: "ansible",
					RemediationRequestRef: corev1.ObjectReference{
						APIVersion: "remediationorchestrator.kubernaut.ai/v1alpha1",
						Kind:       "RemediationRequest",
						Name:       "test-rr-" + testName,
						Namespace:  controllerNamespace,
					},
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID:      depSecretAnsibleUUID,
						Version:         "v1.0.0",
						ExecutionBundle: "https://github.com/jordigilh/kubernaut-test-playbooks.git",
						EngineConfig:    &apiextensionsv1.JSON{Raw: engineCfgJSON},
					},
					TargetResource: targetResource,
					Parameters: map[string]string{
						"target_kind":      "Deployment",
						"target_name":      "dep-secret-test",
						"target_namespace": "default",
					},
				},
			}

			defer func() { _ = deleteWFE(wfe) }()

			By("E2E-WE-015-005: Creating WFE with ansible engine and secret dependency")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("E2E-WE-015-005: Verifying WFE transitions to Running")
			Eventually(func() string {
				updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
				if updated != nil {
					return updated.Status.Phase
				}
				return ""
			}, 60*time.Second, 2*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseRunning))

			By("E2E-WE-015-005: Verifying ephemeral credential annotation was set")
			runningWFE, err := getWFEDirect(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			credAnnotation, hasAnnotation := runningWFE.Annotations["kubernaut.ai/awx-ephemeral-credentials"]
			Expect(hasAnnotation).To(BeTrue(),
				"WFE should have kubernaut.ai/awx-ephemeral-credentials annotation")
			Expect(credAnnotation).ToNot(BeEmpty(),
				"Ephemeral credential annotation should contain credential IDs")
			GinkgoWriter.Printf("Ephemeral credential IDs: %s\n", credAnnotation)

			By("E2E-WE-015-005: Waiting for WFE to complete (playbook validates env var)")
			Eventually(func() string {
				updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
				if updated != nil {
					return updated.Status.Phase
				}
				return ""
			}, 180*time.Second, 5*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseCompleted),
				"WFE should complete after playbook validates KUBERNAUT_SECRET_* env var")

			GinkgoWriter.Printf("E2E-WE-015-005 passed: Ansible secret dependency injection via AWX credential verified\n")
		})

		It("E2E-WE-015-006: should inject ConfigMap as extra_vars and complete", func() {
			depConfigMapAnsibleUUID := infrastructure.RegisteredWorkflowUUIDs["test-dep-configmap-ansible"]
			Expect(depConfigMapAnsibleUUID).ToNot(BeEmpty(),
				"test-dep-configmap-ansible UUID should have been captured during workflow registration")

			testName := fmt.Sprintf("e2e-ansible-dep-cm-%s", uuid.New().String()[:8])
			targetResource := fmt.Sprintf("default/deployment/ansible-dep-cm-%s", uuid.New().String()[:8])

			engineCfgJSON, err := json.Marshal(map[string]string{
				"playbookPath":    "playbooks/test-dep-configmap.yml",
				"jobTemplateName": "kubernaut-test-dep-configmap",
			})
			Expect(err).ToNot(HaveOccurred())

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: controllerNamespace,
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					ExecutionEngine: "ansible",
					RemediationRequestRef: corev1.ObjectReference{
						APIVersion: "remediationorchestrator.kubernaut.ai/v1alpha1",
						Kind:       "RemediationRequest",
						Name:       "test-rr-" + testName,
						Namespace:  controllerNamespace,
					},
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID:      depConfigMapAnsibleUUID,
						Version:         "v1.0.0",
						ExecutionBundle: "https://github.com/jordigilh/kubernaut-test-playbooks.git",
						EngineConfig:    &apiextensionsv1.JSON{Raw: engineCfgJSON},
					},
					TargetResource: targetResource,
					Parameters: map[string]string{
						"target_kind":      "Deployment",
						"target_name":      "dep-configmap-test",
						"target_namespace": "default",
					},
				},
			}

			defer func() { _ = deleteWFE(wfe) }()

			By("E2E-WE-015-006: Creating WFE with ansible engine and ConfigMap dependency")
			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			By("E2E-WE-015-006: Verifying WFE transitions to Running")
			Eventually(func() string {
				updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
				if updated != nil {
					return updated.Status.Phase
				}
				return ""
			}, 60*time.Second, 2*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseRunning))

			By("E2E-WE-015-006: Verifying NO ephemeral credential annotation (ConfigMaps use extra_vars)")
			runningWFE, err := getWFEDirect(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			_, hasCredAnnotation := runningWFE.Annotations["kubernaut.ai/awx-ephemeral-credentials"]
			Expect(hasCredAnnotation).To(BeFalse(),
				"ConfigMap-only deps should NOT create ephemeral credentials")

			By("E2E-WE-015-006: Waiting for WFE to complete (playbook validates extra_var)")
			Eventually(func() string {
				updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
				if updated != nil {
					return updated.Status.Phase
				}
				return ""
			}, 180*time.Second, 5*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseCompleted),
				"WFE should complete after playbook validates KUBERNAUT_CONFIGMAP_* extra_var")

			GinkgoWriter.Printf("E2E-WE-015-006 passed: Ansible ConfigMap dependency injection via extra_vars verified\n")
		})
	})
})

// createAnsibleWFE builds a WorkflowExecution CRD targeting the ansible engine.
func createAnsibleWFE(name, targetResource, workflowID, playbookPath, templateName string) *workflowexecutionv1alpha1.WorkflowExecution {
	engineCfgJSON, err := json.Marshal(map[string]string{
		"playbookPath":    playbookPath,
		"jobTemplateName": templateName,
	})
	Expect(err).ToNot(HaveOccurred())

	return &workflowexecutionv1alpha1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: controllerNamespace,
		},
		Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
			ExecutionEngine: "ansible",
			RemediationRequestRef: corev1.ObjectReference{
				APIVersion: "remediationorchestrator.kubernaut.ai/v1alpha1",
				Kind:       "RemediationRequest",
				Name:       "test-rr-" + name,
				Namespace:  controllerNamespace,
			},
			WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
				WorkflowID:      workflowID,
				Version:         "v1.0.0",
				ExecutionBundle: "https://github.com/jordigilh/kubernaut-test-playbooks.git",
				EngineConfig:    &apiextensionsv1.JSON{Raw: engineCfgJSON},
			},
			TargetResource: targetResource,
			Parameters: map[string]string{
				"target_kind":      "Deployment",
				"target_name":      "ansible-target",
				"target_namespace": "default",
			},
		},
	}
}

// extractAWXJobID parses the numeric AWX job ID from an ExecutionRef name (format: awx-job-{id}).
func extractAWXJobID(executionRefName string) int {
	const prefix = "awx-job-"
	Expect(executionRefName).To(HavePrefix(prefix), "ExecutionRef must have awx-job- prefix")
	id, err := strconv.Atoi(executionRefName[len(prefix):])
	Expect(err).ToNot(HaveOccurred(), "AWX job ID must be a valid integer")
	return id
}

// cancelAWXJob sends POST /api/v2/jobs/{id}/cancel/ to the AWX API via its NodePort.
// The AWX token is read from the K8s Secret created during E2E infrastructure setup.
func cancelAWXJob(jobID int) {
	token := readAWXToken()
	url := fmt.Sprintf("http://localhost:%d/api/v2/jobs/%d/cancel/", infrastructure.AWXNodePort, jobID)

	httpClient := &http.Client{
		Timeout: 15 * time.Second,
	}

	req, err := http.NewRequest("POST", url, strings.NewReader(""))
	Expect(err).ToNot(HaveOccurred())
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/json")

	var resp *http.Response
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		resp, lastErr = httpClient.Do(req)
		if lastErr == nil {
			break
		}
		GinkgoWriter.Printf("AWX cancel attempt %d failed: %v\n", attempt+1, lastErr)
		time.Sleep(1 * time.Second)
	}
	Expect(lastErr).ToNot(HaveOccurred(), "AWX cancel API call should succeed after retries")
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)

	// AWX returns 202 Accepted for cancel, or 405 if the job already finished
	Expect(resp.StatusCode).To(Or(Equal(http.StatusAccepted), Equal(http.StatusMethodNotAllowed)),
		fmt.Sprintf("AWX cancel should return 202 or 405, got %d: %s", resp.StatusCode, string(body)))

	GinkgoWriter.Printf("AWX cancel response: %d\n", resp.StatusCode)
}

// readAWXToken reads the AWX API token from the K8s Secret deployed during E2E setup.
func readAWXToken() string {
	secret := &corev1.Secret{}
	err := k8sClient.Get(ctx, client.ObjectKey{
		Name:      infrastructure.AWXTokenSecretName,
		Namespace: controllerNamespace,
	}, secret)
	Expect(err).ToNot(HaveOccurred(), "AWX token Secret should exist")

	token, ok := secret.Data["token"]
	Expect(ok).To(BeTrue(), "AWX token Secret should have 'token' key")
	return strings.TrimSpace(string(token))
}
