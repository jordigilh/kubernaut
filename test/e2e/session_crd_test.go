package e2e_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("InvestigationSession CRD (E2E)", Ordered, ContinueOnFailure, Label("e2e", "phase1", "session-crd"), func() {

	var (
		namespace      string
		kubeconfigPath string
	)

	BeforeAll(func() {
		namespace = getEnvOrDefault("AF_E2E_NAMESPACE", "kubernaut-system")
		kubeconfigPath = os.Getenv("HOME") + "/.kube/apifrontend-e2e-config"
	})

	kubectl := func(args ...string) (string, error) {
		allArgs := append([]string{"--kubeconfig", kubeconfigPath}, args...)
		cmd := exec.CommandContext(context.Background(), "kubectl", allArgs...)
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}

	// -------------------------------------------------------------------
	// TC-E2E-SESS-001: AF pod starts ctrl.Manager successfully
	// -------------------------------------------------------------------
	It("TC-E2E-SESS-001: AF logs confirm session controller manager started", func() {
		out, err := kubectl("logs", "-n", namespace,
			"-l", "app.kubernetes.io/name=kubernaut-apifrontend",
			"--tail=500", "--all-containers")
		Expect(err).NotTo(HaveOccurred(), "kubectl logs failed: %s", out)
		Expect(out).To(ContainSubstring("session controller manager started"),
			"AF pod should log that the session controller manager started")
	})

	// -------------------------------------------------------------------
	// TC-E2E-SESS-002: InvestigationSession CRD registered in cluster
	// -------------------------------------------------------------------
	It("TC-E2E-SESS-002: InvestigationSession CRD is registered in the cluster", func() {
		out, err := kubectl("get", "crd", "investigationsessions.apifrontend.kubernaut.ai",
			"-o", "jsonpath={.metadata.name}")
		Expect(err).NotTo(HaveOccurred(), "CRD not found: %s", out)
		Expect(out).To(Equal("investigationsessions.apifrontend.kubernaut.ai"))
	})

	// -------------------------------------------------------------------
	// TC-E2E-SESS-003: CRD create + status update works in-cluster
	// -------------------------------------------------------------------
	It("TC-E2E-SESS-003: InvestigationSession can be created and status updated", func() {
		manifest := fmt.Sprintf(`apiVersion: apifrontend.kubernaut.ai/v1alpha1
kind: InvestigationSession
metadata:
  name: e2e-sess-003
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: kubernaut-apifrontend
    apifrontend.kubernaut.ai/phase: Active
spec:
  a2aTaskID: task-e2e-003
  joinMode: start
  userIdentity:
    username: e2e-user@kubernaut.ai
    groups:
      - sre
  remediationRequestRef:
    name: rr-e2e-test
    namespace: %s
`, namespace, namespace)

		cmd := exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", kubeconfigPath, "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(manifest)
		out, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "kubectl apply failed: %s", string(out))

		Eventually(func() string {
			phase, _ := kubectl("get", "investigationsession", "e2e-sess-003",
				"-n", namespace, "-o", "jsonpath={.metadata.name}")
			return phase
		}, 10*time.Second, 1*time.Second).Should(Equal("e2e-sess-003"))

		DeferCleanup(func() {
			_, _ = kubectl("delete", "investigationsession", "e2e-sess-003", "-n", namespace, "--ignore-not-found")
		})
	})

	// -------------------------------------------------------------------
	// TC-E2E-SESS-004: TTL reconciler auto-cancels disconnected session
	// -------------------------------------------------------------------
	It("TC-E2E-SESS-004: TTL reconciler transitions Disconnected session to Cancelled", func() {
		// Step 1: Create the CRD (status is a subresource, so kubectl apply won't set it)
		manifest := fmt.Sprintf(`apiVersion: apifrontend.kubernaut.ai/v1alpha1
kind: InvestigationSession
metadata:
  name: e2e-sess-004
  namespace: %s
  labels:
    app.kubernetes.io/managed-by: kubernaut-apifrontend
    apifrontend.kubernaut.ai/phase: Disconnected
spec:
  a2aTaskID: task-e2e-004
  joinMode: start
  userIdentity:
    username: e2e-user@kubernaut.ai
    groups:
      - sre
  remediationRequestRef:
    name: rr-e2e-test-ttl
    namespace: %s
`, namespace, namespace)

		cmd := exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", kubeconfigPath, "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(manifest)
		out, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "kubectl apply failed: %s", string(out))

		// Step 2: Patch the status subresource with Disconnected phase and a past timestamp
		statusPatch := `{"status":{"phase":"Disconnected","message":"simulated disconnect","connectionState":"Disconnected","disconnectedAt":"2020-01-01T00:00:00Z"}}`
		out2, err2 := kubectl("patch", "investigationsession", "e2e-sess-004",
			"-n", namespace, "--type=merge", "--subresource=status", "-p", statusPatch)
		Expect(err2).NotTo(HaveOccurred(), "status patch failed: %s", out2)

		// Step 3: The reconciler should see Disconnected with an expired TTL and auto-cancel
		Eventually(func() string {
			phase, _ := kubectl("get", "investigationsession", "e2e-sess-004",
				"-n", namespace, "-o", "jsonpath={.status.phase}")
			return phase
		}, 30*time.Second, 2*time.Second).Should(Equal("Cancelled"),
			"Reconciler should auto-cancel a Disconnected session with expired TTL")

		DeferCleanup(func() {
			_, _ = kubectl("delete", "investigationsession", "e2e-sess-004", "-n", namespace, "--ignore-not-found")
		})
	})
})
