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
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// ========================================
// E2E-WE-1661-SNAPSHOT: WorkflowRef snapshot isolation (Issue #1661 Phase 56, Fourth Finding)
// ========================================
//
// Authority: DD-WORKFLOW-018 Change 11e (Issue #1661) -- WorkflowExecution's
// spec.workflowRef is a fully self-sufficient snapshot taken at creation time
// (WorkflowID, Version, ExecutionBundle, ExecutionEngine, ServiceAccountName,
// Dependencies, Resources, DeclaredParameterNames all copied verbatim from
// AIAnalysis.Status.SelectedWorkflow -- pkg/remediationorchestrator/creator/
// workflowexecution.go:121-134). WE's reconciler builds its CreateOptions
// directly from this embedded snapshot and makes zero WorkflowQuerier/DataStorage
// calls at execution time.
//
// This proves that claim against a REAL cluster, not just IT-level mocks: a
// WorkflowExecution whose source RemediationWorkflow CRD is deleted immediately
// after creation must still run to completion purely from its own embedded
// snapshot, because it never looks the source CRD up again.
var _ = Describe("E2E-WE-1661-SNAPSHOT: WorkflowRef survives source RemediationWorkflow deletion", Label("e2e", "workflowexecution", "workflowref-snapshot"), func() {
	var rwClient client.Client

	BeforeEach(func() {
		var err error
		rwClient, err = infrastructure.NewKubeconfigWorkflowClient(kubeconfigPath)
		Expect(err).ToNot(HaveOccurred(), "failed to build a RemediationWorkflow-scheme client")
	})

	It("E2E-WE-1661-SNAPSHOT-001: WFE completes from its embedded snapshot after the source RemediationWorkflow is deleted", func() {
		suffix := uuid.New().String()[:8]
		rwName := fmt.Sprintf("e2e-snapshot-source-%s", suffix)
		wfeName := fmt.Sprintf("e2e-snapshot-wfe-%s", suffix)
		targetResource := fmt.Sprintf("default/deployment/snapshot-test-%s", suffix)

		By("Creating a dedicated RemediationWorkflow CRD and waiting for AuthWebhook to admit it")
		rw := &rwv1alpha1.RemediationWorkflow{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rwName,
				Namespace: controllerNamespace,
			},
			Spec: rwv1alpha1.RemediationWorkflowSpec{
				Version: "1.0.0",
				Description: rwv1alpha1.RemediationWorkflowDescription{
					What:      "E2E snapshot-isolation source workflow",
					WhenToUse: "Issue #1661 Phase 56 Fourth-Finding proof",
				},
				// RestartPod is already proven registerable in this cluster (the
				// suite's own test-hello-world fixture uses it), so this avoids
				// any risk of admission failing on an unseeded ActionType CRD.
				ActionType: "RestartPod",
				Labels: rwv1alpha1.RemediationWorkflowLabels{
					Severity:    []string{"info"},
					Environment: []string{"test"},
					Component:   []string{"apps/v1/Deployment"},
					Priority:    "P3",
				},
				Execution: rwv1alpha1.RemediationWorkflowExecution{
					Engine: "tekton",
					Bundle: "quay.io/kubernaut-cicd/tekton-bundles/hello-world:v1.0.0@sha256:a663ba9ddf8a074025723a4fbbef5542f520deb4e5eaf9814e07775456ecd7e0",
				},
				Parameters: []rwv1alpha1.RemediationWorkflowParameter{
					{
						Name:        "TARGET_RESOURCE",
						Type:        "string",
						Required:    true,
						Description: "Target resource identifier",
					},
				},
			},
		}
		Expect(rwClient.Create(ctx, rw)).To(Succeed(), "AuthWebhook should admit this RemediationWorkflow")

		var admitted *rwv1alpha1.RemediationWorkflow
		Eventually(func() string {
			admitted = &rwv1alpha1.RemediationWorkflow{}
			if err := rwClient.Get(ctx, types.NamespacedName{Name: rwName, Namespace: controllerNamespace}, admitted); err != nil {
				return ""
			}
			return admitted.Status.WorkflowID
		}, 30*time.Second, 1*time.Second).ShouldNot(BeEmpty(), "AuthWebhook should populate status.workflowId")

		workflowID := admitted.Status.WorkflowID
		GinkgoWriter.Printf("✅ RemediationWorkflow %s admitted with workflowId=%s\n", rwName, workflowID)

		By("Creating a WorkflowExecution referencing that workflow's embedded snapshot")
		wfe := &workflowexecutionv1alpha1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{
				Name:      wfeName,
				Namespace: controllerNamespace,
			},
			Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
				RemediationRequestRef: corev1.ObjectReference{
					APIVersion: "remediationorchestrator.kubernaut.ai/v1alpha1",
					Kind:       "RemediationRequest",
					Name:       "test-rr-" + wfeName,
					Namespace:  controllerNamespace,
				},
				WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
					WorkflowID: workflowID,
					Version:    rw.Spec.Version,
					// Same already-proven hello-world Tekton bundle used by
					// createTestWFE elsewhere in this suite.
					ExecutionBundle: "quay.io/kubernaut-cicd/tekton-bundles/hello-world:v1.0.0",
					ExecutionEngine: "tekton",
				},
				TargetResource: targetResource,
				Parameters: map[string]string{
					"MESSAGE": "E2E snapshot-isolation test message",
				},
			},
		}
		Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
		defer func() {
			_ = deleteWFE(wfe)
		}()

		By("Immediately deleting the source RemediationWorkflow CRD, before WE can complete")
		Expect(rwClient.Delete(ctx, rw)).To(Succeed())
		Eventually(func() bool {
			check := &rwv1alpha1.RemediationWorkflow{}
			err := rwClient.Get(ctx, types.NamespacedName{Name: rwName, Namespace: controllerNamespace}, check)
			return err != nil
		}, 30*time.Second, 1*time.Second).Should(BeTrue(), "source RemediationWorkflow should be fully deleted")
		GinkgoWriter.Printf("✅ Source RemediationWorkflow %s deleted\n", rwName)

		By("Verifying the WorkflowExecution still reaches PhaseCompleted purely from its embedded WorkflowRef snapshot")
		Eventually(func() string {
			updated, _ := getWFEDirect(wfe.Name, wfe.Namespace)
			if updated != nil {
				return updated.Status.Phase
			}
			return ""
		}, 120*time.Second, 2*time.Second).Should(Equal(workflowexecutionv1alpha1.PhaseCompleted),
			"WFE must complete from its own snapshot -- it never re-reads the (now-deleted) source RemediationWorkflow")

		GinkgoWriter.Println("✅ E2E-WE-1661-SNAPSHOT-001: WorkflowExecution completed with zero dependency on the deleted source RemediationWorkflow")
	})
})
