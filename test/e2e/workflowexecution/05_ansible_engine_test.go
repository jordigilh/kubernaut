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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	weconditions "github.com/jordigilh/kubernaut/pkg/workflowexecution"

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

var _ = Describe("Ansible Engine E2E [BR-WE-015]", func() {
	Context("Happy Path", func() {
		It("E2E-WE-015-001: should execute ansible workflow to completion via AWX", func() {
			testName := fmt.Sprintf("e2e-ansible-%s", uuid.New().String()[:8])
			targetResource := "default/deployment/ansible-target"

			engineCfgJSON, err := json.Marshal(map[string]string{
				"playbookPath":    "playbooks/test-success.yml",
				"jobTemplateName": "kubernaut-test-success",
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
						WorkflowID:        "test-ansible-success",
						Version:           "v1.0.0",
						ExecutionBundle:   "https://github.com/jordigilh/kubernaut-test-playbooks.git",
						EngineConfig:      &apiextensionsv1.JSON{Raw: engineCfgJSON},
					},
					TargetResource: targetResource,
					Parameters: map[string]string{
						"target_kind":      "Deployment",
						"target_name":      "ansible-target",
						"target_namespace": "default",
					},
				},
			}

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

			GinkgoWriter.Println("✅ WFE transitioned to Running (AWX job launched)")

			By("Verifying WFE completes successfully (AWX job finishes)")
			Eventually(func() string {
				updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
				if updated != nil {
					return updated.Status.Phase
				}
				return ""
			}, 180*time.Second, 5*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseCompleted),
				"WFE should transition to Completed after AWX job succeeds")

			GinkgoWriter.Println("✅ WFE completed via AWX")

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

			GinkgoWriter.Printf("✅ E2E-WE-015-001 passed: ansible WFE completed in %s\n",
				completed.Status.CompletionTime.Time.Sub(completed.CreationTimestamp.Time).Round(time.Second))
		})
	})
})
