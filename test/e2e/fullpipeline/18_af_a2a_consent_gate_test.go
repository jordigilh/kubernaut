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
)

// E2E-FP-1899-001: AF A2A Phase-Transition Consent Gate — Phase 1->2
// (DD-AF-011, issue #1899). A single combined message declares
// interaction_mode=interactive on kubernaut_investigate, then scripts a
// same-turn fire-and-forget attempt at kubernaut_discover_workflows with no
// intervening genuine user message -- the literal #1899 repro (the console
// team observed AF silently discarding session-scoped parameters and
// auto-proceeding past a phase boundary the user never confirmed).
//
// This proves the structural (harness-enforced) gate end to end: even
// though the scripted mock-LLM "misbehaves" and emits the discover_workflows
// call anyway (regardless of what tools are advertised), checkpointToolFilter
// (BeforeModelCallback) and phaseGuardBefore's hard-reject backstop
// (BeforeToolCallback) block it, so no WorkflowExecution is ever created
// from the single turn. It then proves the gate is not a permanent stall:
// once the user genuinely confirms each subsequent phase (interactive mode
// gates every transition, mirroring E2E-FP-1189-003's turn-per-phase
// design), the journey completes to a WorkflowExecution.
var _ = Describe("AF A2A Phase-Transition Consent Gate — Phase 1->2 [E2E-FP-1899-001]", Label("fp", "af", "a2a", "interactive", "issue-1899"), func() {

	It("should block a same-turn fire-and-forget discover_workflows attempt, then complete once the user genuinely confirms each phase", NodeTimeout(8*time.Minute), func(_ SpecContext) {
		targetNS := fpRemediateNS["consent-phase2"]
		Expect(targetNS).NotTo(BeEmpty(), "consent-phase2 namespace must be set by SynchronizedBeforeSuite")

		By("Verifying AF is reachable")
		resp, err := afHTTPClient.Get(afBaseURL + "/healthz")
		if err != nil || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable {
			Skip("AF not reachable in FP cluster — skipping E2E-FP-1899-001")
		}
		_ = resp.Body.Close()

		By("Ensuring managed target namespace exists for the consent-gate phase2 RR")
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

		By("Turn 1 (single message): declare interactive mode, then fire-and-forget attempt discover_workflows in the same turn")
		body := fpA2ATasksSend("fp-cg2-1",
			"create and investigate then sneak workflow discovery for deployment memory-eater")
		resp, err = fpA2AInvokeWithTimeout(body, 180*time.Second)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		rpc, parseErr := fpParseRPC(resp)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "blocked turn should still complete gracefully, not return a JSON-RPC error")
		task, taskErr := fpExtractTask(rpc.Result)
		Expect(taskErr).NotTo(HaveOccurred())
		taskID := task.ID
		Expect(taskID).NotTo(BeEmpty(), "A2A task ID must not be empty")
		GinkgoWriter.Printf("  Turn 1 (blocked attempt) — task: %s (state: %s)\n", taskID, task.Status.State)

		By("Verifying investigate ran (RR exists) but the fire-and-forget discover_workflows attempt never reached KA")
		rrName := fpWaitForRRWithTargetNS(targetNS, 60*time.Second)
		Expect(rrName).NotTo(BeEmpty())
		fpAssertNoWEForRR(rrName)
		GinkgoWriter.Printf("  Confirmed: no WorkflowExecution exists for %s after the blocked same-turn attempt\n", rrName)

		By("Turn 2 (genuine): user explicitly asks to discover workflows — phase 1->2 gate must now lift")
		body = fpA2ATasksSendWithTask("fp-cg2-2", taskID, "discover available workflows")
		resp2, err := fpA2AInvokeWithTimeout(body, 90*time.Second)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp2.Body.Close() }()
		Expect(resp2.StatusCode).To(Equal(http.StatusOK))
		rpc, parseErr = fpParseRPC(resp2)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "Turn 2 (genuine discover_workflows) should not return a JSON-RPC error")
		GinkgoWriter.Printf("  Turn 2 — discover workflows OK (gate lifted on genuine user turn)\n")

		By("Turn 3 (genuine): user explicitly selects a workflow — phase 2->3 gate must also lift on its own genuine turn")
		body = fpA2ATasksSendWithTask("fp-cg2-3", taskID, "select workflow oomkill-increase-memory-v1")
		resp3, err := fpA2AInvokeWithTimeout(body, 90*time.Second)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp3.Body.Close() }()
		Expect(resp3.StatusCode).To(Equal(http.StatusOK))
		rpc, parseErr = fpParseRPC(resp3)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "Turn 3 (genuine select_workflow) should not return a JSON-RPC error")
		GinkgoWriter.Printf("  Turn 3 — select workflow OK\n")

		By("Turn 4 (genuine): watch remediation progress to completion")
		body = fpA2ATasksSendWithTask("fp-cg2-4", taskID, "watch remediation progress")
		resp4, err := fpA2AInvokeWithTimeout(body, 300*time.Second)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp4.Body.Close() }()
		Expect(resp4.StatusCode).To(Equal(http.StatusOK))
		rpc, parseErr = fpParseRPC(resp4)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "Turn 4 (genuine watch) should not return a JSON-RPC error")
		GinkgoWriter.Printf("  Turn 4 — watch OK\n")

		By("Verifying the journey completed once every phase was genuinely confirmed")
		fpWaitForWEComplete(rrName, 60*time.Second)
		GinkgoWriter.Printf("  Full pipeline completed for %s after the consent gate lifted on genuine turns\n", rrName)
	})
})

// E2E-FP-1899-002: AF A2A Phase-Transition Consent Gate — Phase 2->3
// (DD-AF-011, issue #1899, the more severe newly-discovered risk). A single
// combined message declares interaction_mode=full_remediation on
// kubernaut_investigate (legitimately authorizing the auto-chain into
// kubernaut_discover_workflows), then scripts a same-turn fire-and-forget
// attempt at kubernaut_select_workflow with a guessed workflow -- no user
// confirmation. Selecting and executing a workflow is the more consequential
// action (it creates a WorkflowExecution and begins remediating a live
// resource), so this is the sharper edge of the #1899 risk: the harness must
// let the model auto-proceed exactly as far as the declared mode authorizes
// (through discovery) and no further.
var _ = Describe("AF A2A Phase-Transition Consent Gate — Phase 2->3 [E2E-FP-1899-002]", Label("fp", "af", "a2a", "interactive", "issue-1899"), func() {

	It("should auto-chain through discover_workflows but block a same-turn fire-and-forget select_workflow attempt, then complete once the user genuinely confirms", NodeTimeout(8*time.Minute), func(_ SpecContext) {
		targetNS := fpRemediateNS["consent-phase3"]
		Expect(targetNS).NotTo(BeEmpty(), "consent-phase3 namespace must be set by SynchronizedBeforeSuite")

		By("Verifying AF is reachable")
		resp, err := afHTTPClient.Get(afBaseURL + "/healthz")
		if err != nil || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable {
			Skip("AF not reachable in FP cluster — skipping E2E-FP-1899-002")
		}
		_ = resp.Body.Close()

		By("Ensuring managed target namespace exists for the consent-gate phase3 RR")
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

		By("Turn 1 (single message): declare full_remediation mode (authorizes discovery), then fire-and-forget attempt select_workflow in the same turn")
		body := fpA2ATasksSend("fp-cg3-1",
			"create and investigate then sneak workflow selection for deployment memory-eater")
		resp, err = fpA2AInvokeWithTimeout(body, 180*time.Second)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		rpc, parseErr := fpParseRPC(resp)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "blocked turn should still complete gracefully, not return a JSON-RPC error")
		task, taskErr := fpExtractTask(rpc.Result)
		Expect(taskErr).NotTo(HaveOccurred())
		taskID := task.ID
		Expect(taskID).NotTo(BeEmpty(), "A2A task ID must not be empty")
		GinkgoWriter.Printf("  Turn 1 (blocked attempt) — task: %s (state: %s)\n", taskID, task.Status.State)

		By("Verifying investigate+discover_workflows ran (mode-authorized) but the fire-and-forget select_workflow attempt never created a WorkflowExecution")
		rrName := fpWaitForRRWithTargetNS(targetNS, 60*time.Second)
		Expect(rrName).NotTo(BeEmpty())
		fpAssertNoWEForRR(rrName)
		GinkgoWriter.Printf("  Confirmed: no WorkflowExecution exists for %s after the blocked same-turn attempt\n", rrName)

		By("Turn 2 (genuine): user explicitly selects a workflow — phase 2->3 gate must now lift")
		body = fpA2ATasksSendWithTask("fp-cg3-2", taskID, "select workflow oomkill-increase-memory-v1")
		resp2, err := fpA2AInvokeWithTimeout(body, 90*time.Second)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp2.Body.Close() }()
		Expect(resp2.StatusCode).To(Equal(http.StatusOK))
		rpc, parseErr = fpParseRPC(resp2)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "Turn 2 (genuine select_workflow) should not return a JSON-RPC error")
		GinkgoWriter.Printf("  Turn 2 — select workflow OK (gate lifted on genuine user turn)\n")

		By("Turn 3 (genuine): watch remediation progress to completion")
		body = fpA2ATasksSendWithTask("fp-cg3-3", taskID, "watch remediation progress")
		resp3, err := fpA2AInvokeWithTimeout(body, 300*time.Second)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp3.Body.Close() }()
		Expect(resp3.StatusCode).To(Equal(http.StatusOK))
		rpc, parseErr = fpParseRPC(resp3)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "Turn 3 (genuine watch) should not return a JSON-RPC error")
		GinkgoWriter.Printf("  Turn 3 — watch OK\n")

		By("Verifying the journey completed once the user genuinely confirmed workflow selection")
		fpWaitForWEComplete(rrName, 60*time.Second)
		GinkgoWriter.Printf("  Full pipeline completed for %s after the consent gate lifted on the genuine turn\n", rrName)
	})
})
