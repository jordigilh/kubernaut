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

// E2E-FP-1853-002: AF A2A Full Interactive Remediation (mode 3 —
// "autonomous-interactive" per pkg/apifrontend/agent/prompt.txt). A single
// combined message ("investigate and fix") triggers kubernaut_investigate
// directly (creating RR+IS from namespace/kind/name, no separate remediate
// call), then auto-chains through kubernaut_discover_workflows ->
// kubernaut_select_workflow (highest-confidence workflow, no pause for
// manual selection) -> kubernaut_watch, all within the same conversation
// turn, to a completed WorkflowExecution.
//
// This is the "1 message → full pipeline, zero manual turns" case that
// distinguishes mode 3 from mode 2 (E2E-FP-1853-001, which stops at RCA and
// waits for the user) and from mode 1 / autonomous (E2E-FP-1189-002, which
// never streams RCA transparency to the user at all). It is also the first
// test anywhere in the suite to exercise a NextToolCall chain deeper than 2
// tool calls, proving the #1853 N-deep chaining fix end to end against the
// real AF binary.
//
// DD-AF-011 (#1899) happy-path coverage: fullInteractiveRemediationScenarioYAML
// (test/infrastructure/shared_e2e.go) declares interaction_mode:
// "full_remediation_autonomous" on the kubernaut_investigate call, which is
// what authorizes the harness-enforced phase-transition consent gate
// (checkpointToolFilter + phaseGuardBefore) to let this same-turn auto-chain
// reach kubernaut_select_workflow/kubernaut_watch at all -- without it, the
// gate's fail-safe "interactive" default would block discover_workflows and
// this test would fail the same way E2E-FP-1899-001/002 prove a fire-and-
// forget attempt is blocked. This test is therefore the E2E happy-path
// regression proof for full_remediation_autonomous under the consent gate,
// complementing the negative-path proofs in 18_af_a2a_consent_gate_test.go.
var _ = Describe("AF A2A Full Interactive Remediation Full Pipeline [E2E-FP-1853-002]", Label("fp", "af", "a2a", "interactive", "issue-1853"), func() {

	It("should auto-chain investigate -> discover_workflows -> select_workflow -> watch from a single combined message, with no manual pause", NodeTimeout(8*time.Minute), func(_ SpecContext) {
		targetNS := fpRemediateNS["full-interactive"]
		Expect(targetNS).NotTo(BeEmpty(), "full-interactive namespace must be set by SynchronizedBeforeSuite")

		By("Verifying AF is reachable")
		resp, err := afHTTPClient.Get(afBaseURL + "/healthz")
		if err != nil || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable {
			Skip("AF not reachable in FP cluster — skipping E2E-FP-1853-002")
		}
		_ = resp.Body.Close()

		By("Ensuring managed target namespace exists for the full-interactive RR")
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

		By("Turn 1 (single message): investigate and fix in one shot (4-deep NextToolCall chain, no manual selection)")
		body := fpA2ATasksSend("fp-full-1",
			"investigate and fix remediation for deployment memory-eater")
		resp, err = fpA2AInvokeWithTimeout(body, 6*time.Minute)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		rpc, parseErr := fpParseRPC(resp)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "full-interactive turn should not return a JSON-RPC error")
		task, taskErr := fpExtractTask(rpc.Result)
		Expect(taskErr).NotTo(HaveOccurred())
		Expect(task.ID).NotTo(BeEmpty(), "A2A task ID must not be empty")
		GinkgoWriter.Printf("  Full-interactive turn — task: %s (state: %s)\n", task.ID, task.Status.State)

		By("Verifying the RR was created via kubernaut_investigate directly, targeting the isolated namespace")
		rrName := fpWaitForRRWithTargetNS(targetNS, 60*time.Second)
		Expect(rrName).NotTo(BeEmpty())

		By("Verifying the full pipeline completed with zero manual turns (proves the 4-deep chain reached kubernaut_watch)")
		fpWaitForWEComplete(rrName, 5*time.Minute)
		GinkgoWriter.Printf("  Full interactive remediation completed for %s with a single message\n", rrName)
	})
})
