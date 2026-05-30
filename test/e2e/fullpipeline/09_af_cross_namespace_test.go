package fullpipeline

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// E2E-FP-1292-001: Cross-namespace RR creation (ADR-057, BR-PLATFORM-057, CM-6)
//
// Deploys memory-eater in a separate namespace from kubernaut-system, then sends
// an A2A message that triggers kubernaut_remediate with the workload namespace. Verifies:
//   - RR CRD lives in kubernaut-system (metadata.namespace = controllerNS)
//   - spec.targetResource.namespace = workload namespace (not controllerNS)
//   - No InvestigationSession is created (autonomous flow, Issue #1332)
//
// This completes the Pyramid Invariant E2E tier for Issue #1292.
var _ = Describe("AF A2A Cross-Namespace RR [E2E-FP-1292-001]", Label("fp", "af", "a2a", "issue-1292", "adr-057", "issue-1332"), func() {

	It("should create RR in kubernaut-system with targetResource in workload namespace", func() {
		By("Verifying AF is reachable")
		resp, err := afHTTPClient.Get(afBaseURL + "/healthz")
		if err != nil || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable {
			Skip("AF not reachable in FP cluster — skipping E2E-FP-1292-001")
		}
		_ = resp.Body.Close()

		workloadNS := fmt.Sprintf("fp-e2e-1292-%d", time.Now().Unix())
		GinkgoWriter.Printf("  workloadNS: %s\n", workloadNS)

		By("Creating managed workload namespace")
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   workloadNS,
				Labels: map[string]string{"kubernaut.ai/managed": "true"},
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(context.Background(), ns, &client.DeleteOptions{})
		})

		By("Deploying memory-eater in workload namespace")
		Expect(infrastructure.DeployMemoryEater(ctx, workloadNS, kubeconfigPath, GinkgoWriter)).To(Succeed())

		By("Waiting for memory-eater pod to crash in workload namespace")
		fpWaitForPodCrashInNS("memory-eater", workloadNS, 2*time.Minute)

		By("Sending A2A message with cross-namespace remediation keyword")
		prompt := fmt.Sprintf(
			"cross-namespace remediation for deployment memory-eater in %s namespace",
			workloadNS)
		body := fpA2ATasksSend("fp-1292-cross-ns", prompt)
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

		By("Waiting for RR in kubernaut-system (controllerNS per ADR-057)")
		rrName := fpWaitForRRWithTargetNS("memory-eater", workloadNS, 120*time.Second)
		Expect(rrName).NotTo(BeEmpty())
		GinkgoWriter.Printf("  RR created: %s/%s\n", namespace, rrName)

		DeferCleanup(func() {
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{Name: rrName, Namespace: namespace},
			}
			_ = k8sClient.Delete(context.Background(), rr, &client.DeleteOptions{})
		})

		By("Fetching RR and verifying ADR-057 namespace split")
		rr := &remediationv1.RemediationRequest{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Namespace: namespace,
			Name:      rrName,
		}, rr)).To(Succeed())

		Expect(rr.Namespace).To(Equal(namespace),
			"RR metadata.namespace must be controllerNS (kubernaut-system)")
		Expect(rr.Spec.TargetResource.Namespace).To(Equal(workloadNS),
			"spec.targetResource.namespace must be workloadNS, not controllerNS")
		Expect(rr.Spec.TargetResource.Namespace).NotTo(Equal(rr.Namespace),
			"ADR-057: targetResource.namespace must differ from metadata.namespace in cross-NS case")

		GinkgoWriter.Printf("  ADR-057 verified: metadata.ns=%s, targetResource.ns=%s\n",
			rr.Namespace, rr.Spec.TargetResource.Namespace)

		By("Verifying RR target resource fields")
		Expect(rr.Spec.TargetResource.Kind).To(Equal("Deployment"))
		Expect(strings.Contains(rr.Spec.TargetResource.Name, "memory-eater")).To(BeTrue(),
			"targetResource.name should reference memory-eater")

		By("Verifying no InvestigationSession was created (Issue #1332: autonomous flow)")
		fpAssertNoISForRR(rrName, namespace)
	})
})
