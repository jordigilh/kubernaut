package fullpipeline

import (
	"context"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// E2E-FP-1189-003: A2A Interactive 5-Phase — Simulates a multi-turn conversation
// where the user creates an RR, investigates, discovers workflows, selects one,
// and watches the pipeline to completion.
//
// Namespace isolation: the RR targets a dedicated fp-a2a-interactive namespace
// with a zero-replica deployment, keeping the fingerprint distinct from the
// shared kubernaut-system memory-eater used by other FP tests.
//
// Issue #1332: Turn 1 now uses kubernaut_remediate
// for autonomous RR creation. Turn 2 upgrades to interactive via kubernaut_investigate.
//
// Turn 1: "create a remediation request"   → kubernaut_remediate  (creates RR, no IS)
// Turn 2: "investigate the remediation"    → kubernaut_investigate  (rr_id, blocks until complete)
// Turn 3: "discover available workflows"   → kubernaut_discover_workflows  (rr_id)
// Turn 4: "select workflow"                → kubernaut_select_workflow  (rr_id, workflow_id)
// Turn 5: "watch remediation progress"     → kubernaut_watch  (namespace, rr name)
var _ = Describe("AF A2A Interactive 5-Phase Full Pipeline [E2E-FP-1189-003]", Label("fp", "af", "a2a", "interactive", "issue-1189", "issue-1332"), func() {

	It("should complete 5-turn interactive conversation and trigger full pipeline", NodeTimeout(8*time.Minute), func(_ SpecContext) {
		targetNS := fpRemediateNS["interactive"]
		Expect(targetNS).NotTo(BeEmpty(), "interactive namespace must be set by SynchronizedBeforeSuite")
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

		By("Turn 1: create a remediation request (kubernaut_remediate — interactive RR)")
		body := fpA2ATasksSend("fp-int-1",
			"create interactive remediation for deployment memory-eater")
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

		By("Turn 2: investigate the remediation (blocks until KA investigation completes)")
		body = fpA2ATasksSendWithTask("fp-int-2", taskID,
			"investigate the remediation")
		resp2, err := fpA2AInvokeWithTimeout(body, 180*time.Second)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp2.Body.Close() }()
		Expect(resp2.StatusCode).To(Equal(http.StatusOK))
		rpc, parseErr = fpParseRPC(resp2)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "Turn 2 should not return JSON-RPC error")
		GinkgoWriter.Printf("  Turn 2 — investigate OK\n")

		By("Turn 3: discover available workflows")
		body = fpA2ATasksSendWithTask("fp-int-3", taskID,
			"discover available workflows")
		resp3, err := fpA2AInvokeWithTimeout(body, 90*time.Second)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp3.Body.Close() }()
		Expect(resp3.StatusCode).To(Equal(http.StatusOK))
		rpc, parseErr = fpParseRPC(resp3)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "Turn 3 should not return JSON-RPC error")
		GinkgoWriter.Printf("  Turn 3 — discover workflows OK\n")

		By("Turn 4: select workflow")
		body = fpA2ATasksSendWithTask("fp-int-4", taskID,
			"select workflow oomkill-increase-memory-v1")
		resp4, err := fpA2AInvokeWithTimeout(body, 90*time.Second)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp4.Body.Close() }()
		Expect(resp4.StatusCode).To(Equal(http.StatusOK))
		rpc, parseErr = fpParseRPC(resp4)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "Turn 4 should not return JSON-RPC error")
		GinkgoWriter.Printf("  Turn 4 — select workflow OK\n")

		By("Turn 5: watch remediation progress (blocks until terminal phase)")
		body = fpA2ATasksSendWithTask("fp-int-5", taskID,
			"watch remediation progress")
		resp5, err := fpA2AInvokeWithTimeout(body, 300*time.Second)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp5.Body.Close() }()
		Expect(resp5.StatusCode).To(Equal(http.StatusOK))
		rpc, parseErr = fpParseRPC(resp5)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "Turn 5 should not return JSON-RPC error")
		GinkgoWriter.Printf("  Turn 5 — watch OK\n")

		By("Verifying full pipeline completed")
		rrName := fpWaitForRRWithTargetNS("memory-eater", targetNS, 30*time.Second)
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
