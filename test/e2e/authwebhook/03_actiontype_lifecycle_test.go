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

package authwebhook

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	atv1alpha1 "github.com/jordigilh/kubernaut/api/actiontype/v1alpha1"
	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
)

// ========================================
// E2E: ActionType CRD Lifecycle (#300)
// ========================================
//
// Authority: BR-WORKFLOW-007, ADR-059, DD-ACTIONTYPE-001
// Test Plan: docs/testing/300/TEST_PLAN.md §5.3 (Tier 3: E2E)
//
// These tests exercise the full ActionType CRD lifecycle in a real Kind
// cluster with the AW + DS services deployed. No mocks.
//
// ========================================

var _ = Describe("E2E: ActionType CRD Lifecycle (#300)", Ordered, Label("e2e", "actiontype"), func() {
	var (
		testCtx       context.Context
		testNamespace string
	)

	BeforeAll(func() {
		testCtx = context.Background()
		// Use the shared namespace so the ValidatingWebhookConfiguration's
		// namespaceSelector (kubernetes.io/metadata.name: authwebhook-e2e) matches.
		testNamespace = sharedNamespace
	})

	AfterAll(func() {
		By("Cleaning up ActionType CRDs from shared namespace")
		_ = client.IgnoreNotFound(k8sClient.Delete(testCtx, &atv1alpha1.ActionType{
			ObjectMeta: metav1.ObjectMeta{Name: "e2e-restart-pod", Namespace: testNamespace},
		}))
	})

	// ========================================
	// E2E-AT-300-001: kubectl apply creates ActionType, status populated
	// BR-WORKFLOW-007.1
	// ========================================
	It("E2E-AT-300-001: kubectl apply creates ActionType and populates status", func() {
		at := &atv1alpha1.ActionType{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-restart-pod",
				Namespace: testNamespace,
			},
			Spec: atv1alpha1.ActionTypeSpec{
				Name: "E2ERestartPod",
				Description: atv1alpha1.ActionTypeDescription{
					What:          "E2E: Kill and recreate one or more pods.",
					WhenToUse:     "Root cause is a transient runtime state issue.",
					Preconditions: "Evidence that the issue is transient.",
				},
			},
		}

		By("Creating ActionType CRD")
		Expect(k8sClient.Create(testCtx, at)).To(Succeed())

		By("Waiting for status.registered to become true")
		Eventually(func() bool {
			updated := &atv1alpha1.ActionType{}
			if err := k8sClient.Get(testCtx, client.ObjectKeyFromObject(at), updated); err != nil {
				return false
			}
			return updated.Status.Registered
		}, 30*time.Second, 1*time.Second).Should(BeTrue(),
			"AW should register the ActionType in DS and populate status.registered=true")

		By("Verifying all status fields")
		updated := &atv1alpha1.ActionType{}
		Expect(k8sClient.Get(testCtx, client.ObjectKeyFromObject(at), updated)).To(Succeed())
		Expect(updated.Status.CatalogStatus).To(Equal("active"))
		Expect(updated.Status.RegisteredBy).ToNot(BeZero(),
			"registeredBy should be populated with the K8s user")
		Expect(updated.Status.RegisteredAt).ToNot(BeZero(),
			"registeredAt should be populated")
		Expect(updated.Status.PreviouslyExisted).To(BeFalse())

		GinkgoWriter.Printf("✅ ActionType created: %s, registeredBy=%s\n",
			updated.Spec.Name, updated.Status.RegisteredBy)
	})

	// ========================================
	// E2E-AT-300-002: kubectl edit updates description
	// BR-WORKFLOW-007.2
	// ========================================
	It("E2E-AT-300-002: updating description is allowed", func() {
		at := &atv1alpha1.ActionType{}
		Expect(k8sClient.Get(testCtx, client.ObjectKey{
			Namespace: testNamespace, Name: "e2e-restart-pod",
		}, at)).To(Succeed())

		By("Updating description.what field")
		at.Spec.Description.What = "E2E: Gracefully restart pods with rolling strategy."
		Expect(k8sClient.Update(testCtx, at)).To(Succeed(),
			"UPDATE with description change should be Allowed by webhook")

		By("Verifying updated description persisted")
		updated := &atv1alpha1.ActionType{}
		Expect(k8sClient.Get(testCtx, client.ObjectKey{
			Namespace: testNamespace, Name: "e2e-restart-pod",
		}, updated)).To(Succeed())
		Expect(updated.Spec.Description.What).To(Equal("E2E: Gracefully restart pods with rolling strategy."))

		GinkgoWriter.Println("✅ Description updated successfully")
	})

	// ========================================
	// E2E-AT-300-IMMUTABLE: spec.name change denied (bonus, not in plan)
	// BR-WORKFLOW-007.2
	// ========================================
	It("E2E-AT-300-IMMUTABLE: spec.name change is denied by webhook", func() {
		at := &atv1alpha1.ActionType{}
		Expect(k8sClient.Get(testCtx, client.ObjectKey{
			Namespace: testNamespace, Name: "e2e-restart-pod",
		}, at)).To(Succeed())

		By("Attempting to change spec.name (immutable field)")
		at.Spec.Name = "RenamedPod"
		err := k8sClient.Update(testCtx, at)
		Expect(err).To(HaveOccurred(),
			"UPDATE with spec.name change should be Denied by webhook")
		Expect(err.Error()).To(ContainSubstring("immutable"),
			"Error should mention immutability")

		GinkgoWriter.Println("✅ spec.name change correctly denied")
	})

	// ========================================
	// E2E-AT-300-004: Printer columns
	// BR-WORKFLOW-007
	// ========================================
	It("E2E-AT-300-004: printer columns show correct values", func() {
		By("Running kubectl get actiontypes")
		cmd := exec.Command("kubectl",
			"--kubeconfig", kubeconfigPath,
			"get", "actiontypes", "-n", testNamespace,
			"--no-headers",
		)
		output, err := cmd.CombinedOutput()
		Expect(err).ToNot(HaveOccurred(), "kubectl get actiontypes should succeed: %s", string(output))

		line := strings.TrimSpace(string(output))
		GinkgoWriter.Printf("kubectl output: %s\n", line)

		Expect(line).To(ContainSubstring("e2e-restart-pod"),
			"CRD name should appear in output")
		Expect(line).To(ContainSubstring("E2ERestartPod"),
			"ACTION TYPE column (spec.name) should appear")
		Expect(line).To(ContainSubstring("true"),
			"REGISTERED column should show true")
	})

	// ========================================
	// E2E-AT-300-005: Wide output shows DESCRIPTION column
	// BR-WORKFLOW-007
	// ========================================
	It("E2E-AT-300-005: wide output shows DESCRIPTION column", func() {
		By("Running kubectl get actiontypes -o wide")
		cmd := exec.Command("kubectl",
			"--kubeconfig", kubeconfigPath,
			"get", "actiontypes", "-n", testNamespace,
			"-o", "wide",
			"--no-headers",
		)
		output, err := cmd.CombinedOutput()
		Expect(err).ToNot(HaveOccurred(), "kubectl get actiontypes -o wide should succeed: %s", string(output))

		line := strings.TrimSpace(string(output))
		GinkgoWriter.Printf("kubectl -o wide output: %s\n", line)

		Expect(line).To(ContainSubstring("Gracefully restart pods"),
			"DESCRIPTION column should show the updated description.what")
	})

	// ========================================
	// E2E-AT-300-006: RW CREATE updates activeWorkflowCount
	// BR-WORKFLOW-007.5 (Phase 3c)
	// ========================================
	It("E2E-AT-300-006: RW CREATE updates ActionType activeWorkflowCount", func() {
		By("Creating a RemediationWorkflow referencing E2ERestartPod")
		rw := &rwv1alpha1.RemediationWorkflow{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-rw-for-at",
				Namespace: testNamespace,
			},
			Spec: rwv1alpha1.RemediationWorkflowSpec{
				Metadata: rwv1alpha1.RemediationWorkflowMetadata{
					WorkflowName: "e2e-rw-for-at",
					Version:      "1.0.0",
					Description: rwv1alpha1.RemediationWorkflowDescription{
						What:      "E2E workflow for ActionType cross-update",
						WhenToUse: "During E2E testing",
					},
				},
				ActionType: "E2ERestartPod",
				Labels: rwv1alpha1.RemediationWorkflowLabels{
					Severity:    []string{"critical"},
					Environment: []string{"production"},
					Component:   "pod",
					Priority:    "P1",
				},
				Execution: rwv1alpha1.RemediationWorkflowExecution{
					Engine: "job",
					Bundle: "quay.io/kubernaut-cicd/test-workflows/placeholder-execution:v1.0.0@sha256:adfc09ea45a5b627550c6a73fe75d50efe1c80fa43359fcc4908c9c5b0639ac3",
				},
				Parameters: []rwv1alpha1.RemediationWorkflowParameter{
					{Name: "TARGET_RESOURCE", Type: "string", Required: true, Description: "Target resource"},
				},
			},
		}
		Expect(k8sClient.Create(testCtx, rw)).To(Succeed())

		By("Waiting for ActionType activeWorkflowCount to be updated")
		Eventually(func() int {
			at := &atv1alpha1.ActionType{}
			if err := k8sClient.Get(testCtx, client.ObjectKey{
				Namespace: testNamespace, Name: "e2e-restart-pod",
			}, at); err != nil {
				return -1
			}
			return at.Status.ActiveWorkflowCount
		}, 30*time.Second, 1*time.Second).Should(BeNumerically(">=", 1),
			"activeWorkflowCount should be updated after RW CREATE")

		GinkgoWriter.Println("✅ activeWorkflowCount updated after RW CREATE")
	})

	// ========================================
	// E2E-AT-300-003: DELETE denied with dependent workflows, allowed after removal
	// BR-WORKFLOW-007.3
	// ========================================
	It("E2E-AT-300-003: DELETE denied with dependent workflows, allowed after removal", func() {
		By("Attempting to delete ActionType (should be denied)")
		at := &atv1alpha1.ActionType{}
		Expect(k8sClient.Get(testCtx, client.ObjectKey{
			Namespace: testNamespace, Name: "e2e-restart-pod",
		}, at)).To(Succeed())

		err := k8sClient.Delete(testCtx, at)
		Expect(err).To(HaveOccurred(),
			"DELETE should be Denied when dependent workflows exist")
		Expect(err.Error()).To(ContainSubstring("active workflow"),
			"Denial should mention dependent workflows")

		GinkgoWriter.Println("✅ DELETE correctly denied due to dependent workflows")

		By("Removing the dependent RemediationWorkflow")
		rw := &rwv1alpha1.RemediationWorkflow{}
		Expect(k8sClient.Get(testCtx, client.ObjectKey{
			Namespace: testNamespace, Name: "e2e-rw-for-at",
		}, rw)).To(Succeed())
		Expect(k8sClient.Delete(testCtx, rw)).To(Succeed())

		By("Waiting for RW to be fully deleted")
		Eventually(func() bool {
			err := k8sClient.Get(testCtx, client.ObjectKey{
				Namespace: testNamespace, Name: "e2e-rw-for-at",
			}, &rwv1alpha1.RemediationWorkflow{})
			return err != nil
		}, 30*time.Second, 1*time.Second).Should(BeTrue(),
			"RemediationWorkflow should be deleted")

		By("Deleting ActionType (should succeed now)")
		at = &atv1alpha1.ActionType{}
		Expect(k8sClient.Get(testCtx, client.ObjectKey{
			Namespace: testNamespace, Name: "e2e-restart-pod",
		}, at)).To(Succeed())

		Expect(k8sClient.Delete(testCtx, at)).To(Succeed(),
			"DELETE should succeed after removing dependent workflows")

		GinkgoWriter.Println("✅ DELETE succeeded after workflow removal")
	})

	// ========================================
	// E2E-AT-300-007: Re-applying deleted ActionType re-enables
	// BR-WORKFLOW-007.1
	// ========================================
	It("E2E-AT-300-007: re-applying deleted ActionType re-enables with previouslyExisted=true", func() {
		By("Re-creating the ActionType that was deleted")
		at := &atv1alpha1.ActionType{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-restart-pod",
				Namespace: testNamespace,
			},
			Spec: atv1alpha1.ActionTypeSpec{
				Name: "E2ERestartPod",
				Description: atv1alpha1.ActionTypeDescription{
					What:          "E2E: Re-enabled after deletion.",
					WhenToUse:     "Testing re-enable flow.",
					Preconditions: "Previous ActionType was deleted.",
				},
			},
		}
		Expect(k8sClient.Create(testCtx, at)).To(Succeed())

		By("Waiting for status.previouslyExisted to become true")
		Eventually(func() bool {
			updated := &atv1alpha1.ActionType{}
			if err := k8sClient.Get(testCtx, client.ObjectKeyFromObject(at), updated); err != nil {
				return false
			}
			return updated.Status.PreviouslyExisted
		}, 30*time.Second, 1*time.Second).Should(BeTrue(),
			"Re-applied ActionType should have status.previouslyExisted=true")

		By("Verifying status is active")
		updated := &atv1alpha1.ActionType{}
		Expect(k8sClient.Get(testCtx, client.ObjectKeyFromObject(at), updated)).To(Succeed())
		Expect(updated.Status.Registered).To(BeTrue())
		Expect(updated.Status.CatalogStatus).To(Equal("active"))

		GinkgoWriter.Println("✅ Re-enable: previouslyExisted=true, status=active")
	})

	// ========================================
	// E2E-AT-300-AUDIT: Verify audit events emitted
	// BR-WORKFLOW-007.4
	// ========================================
	It("E2E-AT-300-AUDIT: audit events emitted for ActionType lifecycle", func() {
		By("Querying DS audit API for ActionType events")

		// Poll for audit events: the authwebhook's buffered audit store flushes
		// every 5 seconds in E2E, so we need to retry until events appear.
		var eventTypes []string
		queryURL := fmt.Sprintf("%s/api/v1/audit/events?event_category=actiontype&limit=50", dataStorageURL)

		Eventually(func() []string {
			resp, err := authHTTPClient.Get(queryURL)
			if err != nil {
				GinkgoWriter.Printf("⚠️ Audit query failed: %v\n", err)
				return nil
			}
			defer resp.Body.Close()

			if resp.StatusCode != 200 {
				GinkgoWriter.Printf("⚠️ Audit query returned %d\n", resp.StatusCode)
				return nil
			}

			var auditResp struct {
				Data []struct {
					EventType string `json:"event_type"`
				} `json:"data"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&auditResp); err != nil {
				GinkgoWriter.Printf("⚠️ Failed to decode audit response: %v\n", err)
				return nil
			}

			eventTypes = make([]string, 0, len(auditResp.Data))
			for _, e := range auditResp.Data {
				eventTypes = append(eventTypes, e.EventType)
			}
			return eventTypes
		}, 15*time.Second, 2*time.Second).ShouldNot(BeEmpty(),
			"Audit events should appear within 15s (flush interval is 5s)")

		GinkgoWriter.Printf("Audit events found: %v\n", eventTypes)

		Expect(eventTypes).To(ContainElement("actiontype.admitted.create"),
			"At least one CREATE audit event should exist")
	})
})
