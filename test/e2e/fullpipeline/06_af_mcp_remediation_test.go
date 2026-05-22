package fullpipeline

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// E2E-FP-1189-001: RR CRD fixture triggers the full downstream pipeline
// (RO → SP → AA → KA → WE → Notification).
var _ = Describe("AF MCP Path Full Pipeline [E2E-FP-1189-001]", Label("fp", "af", "mcp", "issue-1189"), func() {

	It("should create RR via kubectl and trigger full pipeline execution", func() {
		By("Verifying AF is reachable")
		resp, err := afHTTPClient.Get(afBaseURL + "/healthz")
		if err != nil || resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable {
			Skip("AF not reachable in FP cluster — skipping E2E-FP-1189-001")
		}
		_ = resp.Body.Close()

		rrName := "e2e-fp-mcp-rr-001"
		By("Creating RemediationRequest via kubectl CRD fixture")
		manifest := fmt.Sprintf(`apiVersion: kubernaut.ai/v1alpha1
kind: RemediationRequest
metadata:
  name: %s
  namespace: %s
spec:
  signalName: "E2ETestAlert"
  signalFingerprint: "e2e-%s-Deployment-memory-eater"
  signalType: "prometheus"
  severity: "warning"
  firingTime: "2026-01-01T00:00:00Z"
  receivedTime: "2026-01-01T00:00:01Z"
  targetType: "kubernetes"
  targetResource:
    kind: Deployment
    name: memory-eater
`, rrName, namespace, namespace)
		cmd := exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", kubeconfigPath, "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(manifest)
		out, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "kubectl apply RR: %s", string(out))
		DeferCleanup(func() {
			_, _ = exec.CommandContext(context.Background(), "kubectl",
				"--kubeconfig", kubeconfigPath,
				"delete", "remediationrequest", rrName, "-n", namespace, "--ignore-not-found").CombinedOutput()
		})

		By("Waiting for full pipeline execution (RR → WE completion)")
		foundRR := fpWaitForRR("memory-eater", 120*time.Second)
		Expect(foundRR).NotTo(BeEmpty())
		GinkgoWriter.Printf("  RemediationRequest created: %s\n", foundRR)

		fpWaitForWEComplete(foundRR, 5*time.Minute)
		GinkgoWriter.Printf("  WorkflowExecution completed for %s\n", foundRR)
	})
})
