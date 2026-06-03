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

// E2E-FP-1189-002: A2A Autonomous — A single message/send instructs the AF agent to
// create a remediation request. The mock-LLM returns a kubernaut_remediate tool call,
// the agent executes it, and the full downstream pipeline processes the RR.
// Issue #1332: Autonomous flow must NOT create an InvestigationSession.
var _ = Describe("AF A2A Autonomous Full Pipeline [E2E-FP-1189-002]", Label("fp", "af", "a2a", "issue-1189", "issue-1332"), func() {

	It("should create RR via A2A and trigger full pipeline execution without IS", FlakeAttempts(2), func() {
		autoNS := fpRemediateNS["autonomous"]
		Expect(autoNS).NotTo(BeEmpty(), "autonomous namespace must be set by SynchronizedBeforeSuite")

		By("Verifying AF is reachable")
		resp, err := afHTTPClient.Get(afBaseURL + "/healthz")
		if err != nil || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable {
			Skip("AF not reachable in FP cluster — skipping E2E-FP-1189-002")
		}
		_ = resp.Body.Close()

		By("Deploying zero-replica target Deployment in autonomous namespace")
		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "memory-eater",
				Namespace: autoNS,
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
		if createErr := k8sClient.Create(ctx, dep); createErr != nil && !apierrors.IsAlreadyExists(createErr) {
			Expect(createErr).NotTo(HaveOccurred(), "Failed to create memory-eater in %s", autoNS)
		}
		DeferCleanup(func() {
			_ = k8sClient.Delete(context.Background(), dep, &client.DeleteOptions{})
		})

		By("Waiting for memory-eater pod to crash in kubernaut-system (F-SIG-08: ensures Warning events exist for DominantEventReason)")
		fpWaitForPodCrash("memory-eater", 2*time.Minute)

		By("Creating RR via A2A message/send (kubernaut_remediate — autonomous, no IS)")
		body := fpA2ATasksSend("fp-auto-1",
			"create autonomous remediation for deployment memory-eater")
		resp, err = fpA2AInvoke(body)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK),
			"A2A message/send should return 200")
		rpc, parseErr := fpParseRPC(resp)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "A2A should not return JSON-RPC error")
		task, taskErr := fpExtractTask(rpc.Result)
		Expect(taskErr).NotTo(HaveOccurred())
		Expect(task.ID).NotTo(BeEmpty(), "A2A task ID must not be empty")
		GinkgoWriter.Printf("  A2A task: %s (state: %s)\n", task.ID, task.Status.State)

		By("Waiting for full pipeline execution (match by signal fingerprint)")
		fp := rrFingerprint(autoNS, "Deployment", "memory-eater")
		rrName := fpWaitForRRByFingerprint(fp, 120*time.Second)
		Expect(rrName).NotTo(BeEmpty())
		fpWaitForWEComplete(rrName, 5*time.Minute)
		GinkgoWriter.Printf("  Full pipeline completed for %s\n", rrName)

		By("Verifying no InvestigationSession was created (Issue #1332: autonomous flow)")
		fpAssertNoISForRR(rrName, namespace)
	})
})
