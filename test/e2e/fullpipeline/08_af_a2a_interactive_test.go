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

package fullpipeline

import (
	"context"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// E2E-FP-2390-001 (extends E2E-FP-1189-003): A2A Interactive four-turn
// transcript - simulates a multi-turn conversation where the user starts a
// fresh interactive investigation, discovers workflows, selects one, and
// watches the pipeline to completion (DD-TEST-016).
//
// Namespace isolation: the RR targets a dedicated fp-a2a-interactive namespace
// with a zero-replica deployment, keeping the fingerprint distinct from the
// shared kubernaut-system memory-eater used by other FP tests.
//
// Turn 1: "investigate GitOps remediation" → kubernaut_investigate (creates RR+IS)
// Turn 2: "discover available workflows"   → kubernaut_discover_workflows  (rr_id)
// Turn 3: "select workflow"                → kubernaut_select_workflow  (rr_id, workflow_id)
// Turn 4: "watch remediation progress"     → kubernaut_watch  (namespace, rr name)
var _ = Describe("AF A2A Interactive Transcript Full Pipeline [E2E-FP-2390-001]", Label("fp", "af", "a2a", "interactive", "issue-1189", "issue-2390"), func() {

	It("should complete 4-turn interactive conversation and trigger full pipeline", NodeTimeout(8*time.Minute), func(_ SpecContext) {
		targetNS := fpRemediateNS["interactive"]
		Expect(targetNS).NotTo(BeEmpty(), "interactive namespace must be set by SynchronizedBeforeSuite")
		gitOpsWorkflowUUID, ok := workflowUUIDs["gitops-drift-2390-v1:production"]
		Expect(ok).To(BeTrue(), "E2E-FP-2390-001: GitOps workflow must be seeded")
		Expect(gitOpsWorkflowUUID).NotTo(BeEmpty(), "E2E-FP-2390-001: GitOps workflow UUID must be populated")
		By("Verifying AF is reachable")
		resp, err := afHTTPClient.Get(afBaseURL + "/healthz")
		if err != nil || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable {
			Skip("AF not reachable in FP cluster — skipping E2E-FP-1189-003")
		}
		_ = resp.Body.Close()

		By("Ensuring managed target namespace exists for the interactive RR")
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: targetNS,
				Labels: map[string]string{
					"kubernaut.ai/managed":     "true",
					"kubernaut.ai/environment": "staging",
				},
			},
		}
		if err := k8sClient.Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred(), "Failed to create namespace %s", targetNS)
		}
		DeferCleanup(func() {
			_ = k8sClient.Delete(context.Background(), ns, &client.DeleteOptions{})
		})

		By("Deploying zero-replica target Deployment in isolated namespace")
		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "memory-eater",
				Namespace: targetNS,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To[int32](0),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "memory-eater"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": "memory-eater"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name:  "app",
							Image: "busybox:1.36",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("64Mi"),
								},
							},
						}},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, dep)).To(Succeed())

		By("Grounding the interactive investigation with a synthetic GitOps signal event")
		Expect(k8sClient.Create(ctx, &corev1.Event{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "memory-eater-e2efp2390-gitops-",
				Namespace:    targetNS,
			},
			InvolvedObject: corev1.ObjectReference{
				Kind:       "Deployment",
				Namespace:  targetNS,
				Name:       "memory-eater",
				APIVersion: "apps/v1",
			},
			Reason:         "GitOpsDrift2390",
			Message:        "E2E-FP-2390-001: synthetic signal for the interactive GitOps workflow snapshot journey",
			Type:           corev1.EventTypeWarning,
			FirstTimestamp: metav1.Now(),
			LastTimestamp:  metav1.Now(),
			Count:          1,
			Source:         corev1.EventSource{Component: "e2e-fp-2390-test"},
		})).To(Succeed())
		Eventually(func() bool {
			events := &corev1.EventList{}
			if err := apiReader.List(ctx, events, client.InNamespace(targetNS)); err != nil {
				return false
			}
			for _, event := range events.Items {
				if event.Reason == "GitOpsDrift2390" &&
					event.Type == corev1.EventTypeWarning &&
					event.InvolvedObject.Kind == "Deployment" &&
					event.InvolvedObject.Name == "memory-eater" {
					return true
				}
			}
			return false
		}, 30*time.Second, 2*time.Second).Should(BeTrue(),
			"E2E-FP-2390-001: synthetic GitOps signal event must be observable before RR creation")

		By("Creating the GitOps repository credential dependency")
		gitOpsSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "gitea-repo-creds", Namespace: namespace},
			StringData: map[string]string{"username": "kubernaut", "password": "test-password"},
		}
		if err := k8sClient.Create(ctx, gitOpsSecret); err != nil && !apierrors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred())
		}
		DeferCleanup(func() { _ = k8sClient.Delete(context.Background(), gitOpsSecret) })

		By("Creating the GitOps repository configuration dependency")
		gitOpsConfig := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "gitea-repo-config", Namespace: namespace},
			Data:       map[string]string{"repository": "http://gitea.test/gitops/remediation.git"},
		}
		if err := k8sClient.Create(ctx, gitOpsConfig); err != nil && !apierrors.IsAlreadyExists(err) {
			Expect(err).NotTo(HaveOccurred())
		}
		DeferCleanup(func() { _ = k8sClient.Delete(context.Background(), gitOpsConfig) })

		By("Turn 1: investigate the remediation (kubernaut_investigate — creates an interactive RR)")
		turn1ContextID := "ctx-fp-int-1"
		body := fpA2ATasksSend("fp-int-1",
			"investigate GitOps remediation for deployment memory-eater")
		resp, err = fpA2AInvokeWithTimeout(body, 60*time.Second)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		rpc, parseErr := fpParseRPC(resp)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "Turn 1 should not return JSON-RPC error")
		task, taskErr := fpExtractTask(rpc.Result)
		Expect(taskErr).NotTo(HaveOccurred())
		taskID := task.ID
		Expect(taskID).NotTo(BeEmpty())
		GinkgoWriter.Printf("  Turn 1 — task: %s (state: %s)\n", taskID, task.Status.State)

		By("Turn 2: discover available workflows")
		body = fpA2ATasksSendWithContext("fp-int-2", turn1ContextID, taskID,
			"discover available workflows")
		resp2, err := fpA2AInvokeWithTimeout(body, 90*time.Second)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp2.Body.Close() }()
		Expect(resp2.StatusCode).To(Equal(http.StatusOK))
		rpc, parseErr = fpParseRPC(resp2)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "Turn 2 should not return JSON-RPC error")
		GinkgoWriter.Printf("  Turn 2 — discover workflows OK\n")

		By("Turn 3: select workflow")
		body = fpA2ATasksSendWithContext("fp-int-3", turn1ContextID, taskID,
			"select the discovered GitOps workflow")
		resp3, err := fpA2AInvokeWithTimeout(body, 90*time.Second)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp3.Body.Close() }()
		Expect(resp3.StatusCode).To(Equal(http.StatusOK))
		rpc, parseErr = fpParseRPC(resp3)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "Turn 3 should not return JSON-RPC error")
		GinkgoWriter.Printf("  Turn 3 — select workflow OK\n")

		By("Turn 4: watch remediation progress (blocks until terminal phase)")
		body = fpA2ATasksSendWithContext("fp-int-4", turn1ContextID, taskID,
			"watch remediation progress")
		resp4, err := fpA2AInvokeWithTimeout(body, 300*time.Second)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp4.Body.Close() }()
		Expect(resp4.StatusCode).To(Equal(http.StatusOK))
		rpc, parseErr = fpParseRPC(resp4)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "Turn 4 should not return JSON-RPC error")
		GinkgoWriter.Printf("  Turn 4 — watch OK\n")

		By("Verifying full pipeline completed")
		rrName := fpWaitForRRWithTargetNS(targetNS, 30*time.Second)
		Expect(rrName).NotTo(BeEmpty())
		fpWaitForWEComplete(rrName, 60*time.Second)
		GinkgoWriter.Printf("  Full pipeline completed for %s\n", rrName)

		By("[E2E-FP-1189-004] Verifying interactive WFE has TARGET_RESOURCE_* parameters")
		weList := &workflowexecutionv1.WorkflowExecutionList{}
		Expect(apiReader.List(ctx, weList, client.InNamespace(namespace))).To(Succeed())
		var we *workflowexecutionv1.WorkflowExecution
		for i := range weList.Items {
			if weList.Items[i].Spec.RemediationRequestRef.Name == rrName {
				we = &weList.Items[i]
				break
			}
		}
		Expect(we).NotTo(BeNil(), "WorkflowExecution for RR %s must exist", rrName)
		Expect(we.Spec.WorkflowRef.WorkflowID).To(Equal(gitOpsWorkflowUUID),
			"E2E-FP-2390-001: GitOps workflow must be selected")
		Expect(we.Spec.WorkflowRef.Dependencies).NotTo(BeNil(),
			"E2E-FP-2390-001: selected GitOps workflow must declare dependencies")
		// DD-WE-006 / FedRAMP AC-6 and AU-3: both declared dependency kinds must
		// survive catalog selection as attributable, least-privilege inputs.
		Expect(we.Spec.WorkflowRef.Dependencies.Secrets).To(ContainElement(HaveField("Name", "gitea-repo-creds")),
			"E2E-FP-2390-001: gitea-repo-creds must survive interactive selection")
		Expect(we.Spec.WorkflowRef.Dependencies.ConfigMaps).To(ContainElement(HaveField("Name", "gitea-repo-config")),
			"E2E-FP-2390-002: gitea-repo-config must survive interactive selection")
		Expect(we.Spec.WorkflowRef.Resources).NotTo(BeNil(),
			"E2E-FP-2390-001: selected GitOps workflow must declare resources")
		Expect(we.Spec.WorkflowRef.Resources.Requests[corev1.ResourceCPU]).To(Equal(resource.MustParse("10m")),
			"E2E-FP-2390-001: catalog resources must survive interactive selection")
		jobs := &batchv1.JobList{}
		Eventually(func() int {
			if err := apiReader.List(ctx, jobs, client.InNamespace(namespace), client.MatchingLabels{"kubernaut.ai/workflow-execution": we.Name}); err != nil {
				return 0
			}
			return len(jobs.Items)
		}, 30*time.Second, 2*time.Second).Should(Equal(1),
			"E2E-FP-2390-001: selected GitOps workflow must create a Job")
		Expect(jobs.Items[0].Spec.Template.Spec.Volumes).To(ContainElement(
			HaveField("Name", "secret-gitea-repo-creds")),
			"E2E-FP-2390-001: Job must mount gitea-repo-creds")
		Expect(jobs.Items[0].Spec.Template.Spec.Volumes).To(ContainElement(And(
			HaveField("Name", "configmap-gitea-repo-config"),
			HaveField("VolumeSource.ConfigMap.Name", "gitea-repo-config"),
		)), "E2E-FP-2390-002: Job must mount gitea-repo-config")
		Expect(jobs.Items[0].Spec.Template.Spec.Containers[0].VolumeMounts).To(ContainElement(And(
			HaveField("Name", "secret-gitea-repo-creds"),
			HaveField("MountPath", "/run/kubernaut/secrets/gitea-repo-creds"),
			HaveField("ReadOnly", BeTrue()),
		)), "E2E-FP-2390-003: Secret dependency must be mounted read-only")
		Expect(jobs.Items[0].Spec.Template.Spec.Containers[0].VolumeMounts).To(ContainElement(And(
			HaveField("Name", "configmap-gitea-repo-config"),
			HaveField("MountPath", "/run/kubernaut/configmaps/gitea-repo-config"),
			HaveField("ReadOnly", BeTrue()),
		)), "E2E-FP-2390-003: ConfigMap dependency must be mounted read-only")
		params := we.Spec.Parameters
		Expect(params).ToNot(BeNil(), "interactive WFE must have parameters")
		Expect(params).To(HaveKeyWithValue("TARGET_RESOURCE_NAME", "memory-eater"),
			"TARGET_RESOURCE_NAME must be injected into interactive WFE parameters")
		Expect(params).To(HaveKeyWithValue("TARGET_RESOURCE_KIND", "Deployment"),
			"TARGET_RESOURCE_KIND must be injected into interactive WFE parameters")
		Expect(params).To(HaveKeyWithValue("TARGET_RESOURCE_NAMESPACE", targetNS),
			"TARGET_RESOURCE_NAMESPACE must be injected into interactive WFE parameters")
		GinkgoWriter.Printf("  [E2E-FP-1189-004] WFE params: TARGET_RESOURCE_NAME=%s, KIND=%s, NAMESPACE=%s\n",
			params["TARGET_RESOURCE_NAME"], params["TARGET_RESOURCE_KIND"], params["TARGET_RESOURCE_NAMESPACE"])

	})
})
