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

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ========================================
// IT-WE-015-001: WFE Controller Ansible Engine Handling
// ========================================
// Authority: BR-WE-015 (Ansible Execution Engine)
// Test Plan: docs/testing/45/TEST_PLAN.md
// Pattern: envtest with controller (no AWX infra — validates graceful error handling)
//
// NOTE: The integration suite registers only "tekton" and "job" executors.
// Ansible executor registration requires AWX infrastructure (E2E scope).
// This test validates the controller correctly fails with UnsupportedEngine
// when no ansible executor is registered, proving the dispatch path is reached.
// Full ansible dispatch is tested in E2E-WE-015-001 with a real AWX instance.
// ========================================

var _ = Describe("Ansible Executor Integration (BR-WE-015)", func() {
	Context("controller dispatch without ansible executor registered", func() {
		It("IT-WE-015-001: should fail WFE with UnsupportedEngine when ansible executor is not registered", func() {
			targetResource := fmt.Sprintf("default/deployment/ansible-dispatch-%d", time.Now().UnixNano())

			engineConfig, err := json.Marshal(map[string]interface{}{
				"playbookPath":    "playbooks/restart.yml",
				"jobTemplateName": "restart-pod",
				"inventoryName":   "production",
			})
			Expect(err).ToNot(HaveOccurred())

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("ansible-it-%d", time.Now().UnixNano()),
					Namespace: DefaultNamespace,
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					ExecutionEngine: "ansible",
					TargetResource:  targetResource,
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID:      "ansible-restart",
						Version:         "1.0.0",
						ExecutionBundle: "https://github.com/kubernaut/playbooks.git",
						EngineConfig: &apiextensionsv1.JSON{
							Raw: engineConfig,
						},
					},
					Parameters: map[string]string{
						"NAMESPACE": "default",
					},
				},
			}

			defer func() {
				cleanupWFE(wfe)
			}()

			Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

			// Controller should transition to Failed because no ansible executor is registered.
			// This proves the dispatch path is reached and the engine is correctly identified.
			Eventually(func(g Gomega) {
				fetched, err := getWFE(wfe.Name, wfe.Namespace)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(fetched.Status.Phase).To(Equal("Failed"),
					"WFE with unregistered ansible engine should transition to Failed")
			}, 10*time.Second, 200*time.Millisecond).Should(Succeed())

			fetched, err := getWFE(wfe.Name, wfe.Namespace)
			Expect(err).ToNot(HaveOccurred())
			Expect(fetched.Status.FailureDetails).ToNot(BeNil(), "FailureDetails should be populated")
			Expect(fetched.Status.FailureDetails.Reason).To(Equal("UnsupportedEngine"),
				"Failure reason should indicate unsupported engine")
			Expect(fetched.Status.FailureDetails.WasExecutionFailure).To(BeFalse(),
				"Should be a pre-execution failure, not an execution failure")

			GinkgoWriter.Printf("✅ IT-WE-015-001: Ansible WFE correctly failed with UnsupportedEngine\n")
		})
	})
})
