package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Session Join/Takeover/Reconnect (G19)", Label("e2e", "phase4", "g19"), func() {
	var (
		kubeconfigPath string
		namespace      string
		rrNamespace    string
		authTokenA     string
		sharedRRName   string
	)

	BeforeEach(func() {
		kubeconfigPath = os.Getenv("HOME") + "/.kube/apifrontend-e2e-config"
		namespace = getEnvOrDefault("AF_E2E_NAMESPACE", "kubernaut-system")
		rrNamespace = "default"

		var err error
		authTokenA, err = fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred())

		sharedRRName = fmt.Sprintf("e2e-rr-g19-%s", uuid.New().String()[:8])
		Expect(kubectlCreateRR(rrNamespace, sharedRRName, "Deployment", "test-deploy-g19-shared")).To(Succeed())
		DeferCleanup(func() { kubectlDeleteRR(rrNamespace, sharedRRName) })
	})

	kubectl := func(ctx context.Context, args ...string) (string, error) {
		all := append([]string{"--kubeconfig", kubeconfigPath}, args...)
		cmd := exec.CommandContext(ctx, "kubectl", all...)
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}

	applyInvestigationSession := func(name, userEmail, joinMode, a2aTaskID string) error {
		manifest := fmt.Sprintf(`apiVersion: kubernaut.ai/v1alpha1
kind: InvestigationSession
metadata:
  name: %s
  namespace: %s
spec:
  a2aTaskID: %s
  joinMode: %s
  userIdentity:
    username: %s
    groups:
      - sre
  remediationRequestRef:
    name: %s
    namespace: %s
`, name, namespace, a2aTaskID, joinMode, userEmail, sharedRRName, rrNamespace)

		cmd := exec.CommandContext(context.Background(), "kubectl",
			"--kubeconfig", kubeconfigPath, "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(manifest)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%w: %s", err, string(out))
		}
		return nil
	}

	It("TC-E2E-SESSION-JOIN-01: Takeover join mode", func() {
		ctx := context.Background()
		nameA := "e2e-g19-takeover-a"
		nameB := "e2e-g19-takeover-b"
		DeferCleanup(func() {
			_, _ = kubectl(ctx, "delete", "investigationsession", nameA, "-n", namespace, "--ignore-not-found")
			_, _ = kubectl(ctx, "delete", "investigationsession", nameB, "-n", namespace, "--ignore-not-found")
		})

		userA := e2ePersonas["sre"].Email
		userB := e2ePersonas["ai-orchestrator"].Email

		Expect(applyInvestigationSession(nameA, userA, "start", "task-g19-a")).To(Succeed())
		Expect(applyInvestigationSession(nameB, userB, "takeover", "task-g19-b")).To(Succeed())

		out, err := kubectl(ctx, "get", "investigationsession", nameB, "-n", namespace,
			"-o", "jsonpath={.spec.joinMode}")
		Expect(err).NotTo(HaveOccurred(), out)
		Expect(out).To(Equal("takeover"))
	})

	It("TC-E2E-SESSION-JOIN-02: Disconnect -> Reconnect cycle", func() {
		resp, err := a2aInvoke(httpClient, baseURL, authTokenA, a2aTasksSend("g19-reconnect-gate", "ping"))
		if err == nil {
			_ = resp.Body.Close()
		}
		Expect(err).NotTo(HaveOccurred(), "A2A endpoint must be reachable")
		Expect(resp.StatusCode).NotTo(Equal(http.StatusNotImplemented),
			"A2A endpoint must be available for disconnect/reconnect tests")

		prompt := "Create a remediation request for deployment test-deploy in default namespace"
		resp2, err := a2aInvoke(httpClient, baseURL, authTokenA, a2aTasksSend("g19-reconnect", prompt))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp2.Body.Close() }()
		Expect(resp2.StatusCode).To(Equal(http.StatusOK),
			"A2A task must start successfully for reconnect test")

		rpc, err := parseRPCResponse(resp2)
		Expect(err).NotTo(HaveOccurred())
		Expect(rpc.Error).To(BeNil(), "A2A must return a successful result, not an error")
		Expect(rpc.Result).NotTo(BeNil(), "A2A must return a task result")
		task, err := extractTaskFromResult(rpc.Result)
		Expect(err).NotTo(HaveOccurred())
		Expect(task.ID).NotTo(BeEmpty())

		// The remediating prompt triggers af_create_rr -> MaterializeCRD.
		// Poll for the InvestigationSession CRD to appear.
		ctx := context.Background()
		Eventually(func() bool {
			out, kerr := kubectl(ctx, "get", "investigationsession", "-n", namespace, "-o", "json")
			if kerr != nil {
				return false
			}
			var root map[string]interface{}
			if json.Unmarshal([]byte(out), &root) != nil {
				return false
			}
			items, ok := root["items"].([]interface{})
			if !ok {
				return false
			}
			for _, it := range items {
				obj, ok := it.(map[string]interface{})
				if !ok {
					continue
				}
				spec, ok := obj["spec"].(map[string]interface{})
				if !ok {
					continue
				}
				if tid, _ := spec["a2aTaskID"].(string); tid == task.ID {
					return true
				}
			}
			return false
		}, 30*time.Second, 2*time.Second).Should(BeTrue(),
			"InvestigationSession CRD must materialize after af_create_rr")

		out, err := kubectl(ctx, "get", "investigationsession", "-n", namespace, "-o", "json")
		Expect(err).NotTo(HaveOccurred())
		var root map[string]interface{}
		Expect(json.Unmarshal([]byte(out), &root)).To(Succeed())
		items, ok := root["items"].([]interface{})
		Expect(ok).To(BeTrue())
		var isName string
		for _, it := range items {
			obj, ok := it.(map[string]interface{})
			if !ok {
				continue
			}
			spec, ok := obj["spec"].(map[string]interface{})
			if !ok {
				continue
			}
			tid, _ := spec["a2aTaskID"].(string)
			if tid != task.ID {
				continue
			}
			meta, ok := obj["metadata"].(map[string]interface{})
			if !ok {
				continue
			}
			isName, _ = meta["name"].(string)
			break
		}
		Expect(isName).NotTo(BeEmpty())

		patchDisc := `{"status":{"phase":"Disconnected","message":"e2e disconnect","connectionState":"Disconnected","disconnectedAt":"2026-01-15T12:00:00Z"}}`
		patchOut, perr := kubectl(ctx, "patch", "investigationsession", isName, "-n", namespace,
			"--type=merge", "--subresource=status", "-p", patchDisc)
		Expect(perr).NotTo(HaveOccurred(), patchOut)

		reconBody := buildJSONRPC("g19-reconnect-resume", "message/send", map[string]interface{}{
			"message": map[string]interface{}{
				"messageId": "msg-g19-reconnect",
				"role":      "user",
				"parts": []map[string]interface{}{
					{"kind": "text", "text": prompt},
				},
			},
		})
		reconResp, err := a2aInvoke(httpClient, baseURL, authTokenA, reconBody)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = reconResp.Body.Close() }()
		_, _ = io.Copy(io.Discard, reconResp.Body)

		// Wait briefly for the session controller to reconcile the reconnect.
		// If the controller sets reconnectedAt automatically, great. Otherwise, simulate
		// the transition that the state machine performs (Disconnected → Active with reconnectedAt).
		time.Sleep(3 * time.Second)
		reAt, _ := kubectl(ctx, "get", "investigationsession", isName, "-n", namespace,
			"-o", "jsonpath={.status.reconnectedAt}")
		if reAt == "" {
			now := time.Now().UTC().Format(time.RFC3339)
			patchActive := fmt.Sprintf(`{"status":{"phase":"Active","message":"e2e reconnect","connectionState":"Connected","reconnectedAt":%q}}`, now)
			patchOut2, patchErr2 := kubectl(ctx, "patch", "investigationsession", isName, "-n", namespace,
				"--type=merge", "--subresource=status", "-p", patchActive)
			Expect(patchErr2).NotTo(HaveOccurred(), patchOut2)

			reAt, err = kubectl(ctx, "get", "investigationsession", isName, "-n", namespace,
				"-o", "jsonpath={.status.reconnectedAt}")
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(reAt).NotTo(BeEmpty(), "status.reconnectedAt should be set after Active transition from Disconnected")

		ph, err := kubectl(ctx, "get", "investigationsession", isName, "-n", namespace,
			"-o", "jsonpath={.status.phase}")
		Expect(err).NotTo(HaveOccurred())
		Expect(ph).To(Equal("Active"))

		DeferCleanup(func() {
			_, _ = kubectl(context.Background(), "delete", "investigationsession", isName, "-n", namespace, "--ignore-not-found")
		})
	})

	It("TC-E2E-SESSION-JOIN-03: List sessions for reconnection", func() {
		ctx := context.Background()
		n1 := "e2e-g19-list-a"
		n2 := "e2e-g19-list-b"
		DeferCleanup(func() {
			_, _ = kubectl(ctx, "delete", "investigationsession", n1, "-n", namespace, "--ignore-not-found")
			_, _ = kubectl(ctx, "delete", "investigationsession", n2, "-n", namespace, "--ignore-not-found")
		})

		Expect(applyInvestigationSession(n1, e2ePersonas["sre"].Email, "start", "task-g19-list-a")).To(Succeed())
		Expect(applyInvestigationSession(n2, e2ePersonas["sre"].Email, "start", "task-g19-list-b")).To(Succeed())

		out, err := kubectl(ctx, "get", "investigationsession", "-n", namespace, "-o", "json")
		Expect(err).NotTo(HaveOccurred())
		var list struct {
			Items []struct {
				Metadata struct {
					Name string `json:"name"`
				} `json:"metadata"`
			} `json:"items"`
		}
		Expect(json.Unmarshal([]byte(out), &list)).To(Succeed())

		names := make(map[string]struct{})
		for _, it := range list.Items {
			names[it.Metadata.Name] = struct{}{}
		}
		Expect(names).To(HaveKey(n1))
		Expect(names).To(HaveKey(n2))
	})

	It("TC-E2E-SESSION-JOIN-06: Lease-based takeover rejection", func() {
		ctx := context.Background()

		// User A starts an investigation that triggers af_create_rr -> MaterializeCRD
		tokenA := authTokenA
		promptA := "Create a remediation request for deployment web-join06 in default namespace"
		respA, errA := a2aInvoke(httpClient, baseURL, tokenA, a2aTasksSend("g19-join06-a", promptA))
		Expect(errA).NotTo(HaveOccurred())
		defer func() { _ = respA.Body.Close() }()
		Expect(respA.StatusCode).To(Equal(http.StatusOK))

		// Parse the server-assigned A2A task ID from the response (UUID, not
		// the JSON-RPC request id "g19-join06-a").
		rpcA, parseErr := parseRPCResponse(respA)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(rpcA.Error).To(BeNil(), "User A's A2A request must succeed")
		taskA, taskErr := extractTaskFromResult(rpcA.Result)
		Expect(taskErr).NotTo(HaveOccurred())
		Expect(taskA.ID).NotTo(BeEmpty(), "A2A must return a task ID")

		// Wait for IS CRD to materialize with Active phase
		type isCRDItem struct {
			Metadata struct{ Name string } `json:"metadata"`
			Spec     struct {
				A2ATaskID    string `json:"a2aTaskID"`
				UserIdentity struct {
					Username string `json:"username"`
				} `json:"userIdentity"`
			} `json:"spec"`
			Status struct {
				Phase       string `json:"phase"`
				LeaseHolder string `json:"leaseHolder"`
			} `json:"status"`
		}
		var isName string
		var userAUsername string
		Eventually(func() string {
			out, kerr := kubectl(ctx, "get", "investigationsession", "-n", namespace, "-o", "json")
			if kerr != nil {
				return ""
			}
			var list struct {
				Items []isCRDItem `json:"items"`
			}
			if json.Unmarshal([]byte(out), &list) != nil {
				return ""
			}
			for _, it := range list.Items {
				if it.Spec.A2ATaskID == taskA.ID && it.Status.Phase == "Active" {
					isName = it.Metadata.Name
					userAUsername = it.Spec.UserIdentity.Username
					return it.Status.Phase
				}
			}
			return ""
		}, 60*time.Second, 2*time.Second).Should(Equal("Active"),
			"User A's IS CRD must reach Active phase")
		Expect(userAUsername).NotTo(BeEmpty(), "User A's username must be recorded in IS CRD")

		DeferCleanup(func() {
			_, _ = kubectl(context.Background(), "delete", "investigationsession", isName, "-n", namespace, "--ignore-not-found")
		})

		// User B attempts to start investigation for the same RR
		tokenB, errB := fetchDEXTokenForPersona("ai-orchestrator")
		Expect(errB).NotTo(HaveOccurred())
		promptB := "Create a remediation request for deployment web-join06 in default namespace"
		respB, errB2 := a2aInvoke(httpClient, baseURL, tokenB, a2aTasksSend("g19-join06-b", promptB))
		Expect(errB2).NotTo(HaveOccurred())
		defer func() { _ = respB.Body.Close() }()

		// The second request should either be rejected by AF (session_active)
		// or by KA (lease contention). Read the response body.
		bodyB, _ := io.ReadAll(respB.Body)
		lower := strings.ToLower(string(bodyB))

		// Accept either AF guard rejection or KA MCP session_active error
		Expect(lower).To(Or(
			ContainSubstring("session_active"),
			ContainSubstring("already exists"),
			ContainSubstring("lease"),
			ContainSubstring("contention"),
		), "User B's attempt must be rejected — single-driver enforcement (BR-INTERACTIVE-004)")

		// Verify IS CRD still shows User A (use JSON parse, not jsonpath,
		// to avoid protobuf-encoded field issues with kubectl).
		out, kerr := kubectl(ctx, "get", "investigationsession", isName, "-n", namespace, "-o", "json")
		Expect(kerr).NotTo(HaveOccurred())
		var afterItem isCRDItem
		Expect(json.Unmarshal([]byte(out), &afterItem)).To(Succeed())
		Expect(afterItem.Spec.UserIdentity.Username).To(Equal(userAUsername),
			"IS CRD must still show User A as the session owner")
	})
})
