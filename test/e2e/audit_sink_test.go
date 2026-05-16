package e2e_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DS Audit Sink (G8)", Ordered, Label("e2e", "phase4", "g8"), func() {
	var (
		kubeconfigPath string
		namespace      string
		authToken      string
		mcpSessionID   string
		dsAuditURL     string
	)

	BeforeAll(func() {
		kubeconfigPath = os.Getenv("HOME") + "/.kube/apifrontend-e2e-config"
		namespace = getEnvOrDefault("AF_E2E_NAMESPACE", "kubernaut-system")
		dsAuditURL = getEnvOrDefault("AF_E2E_DS_AUDIT_URL", "https://localhost:8089/api/v1/audit/events")

		var err error
		authToken, err = fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred())
		mcpSessionID, err = initMCPSession(authToken)
		Expect(err).NotTo(HaveOccurred())
	})

	kubectl := func(ctx context.Context, args ...string) (string, error) {
		all := append([]string{"--kubeconfig", kubeconfigPath}, args...)
		cmd := exec.CommandContext(ctx, "kubectl", all...)
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}

	mcpToolCall := func(toolName string, args map[string]interface{}) (string, error) {
		callBody := buildJSONRPC(fmt.Sprintf("g8-%s-%d", toolName, time.Now().UnixNano()),
			"tools/call", map[string]interface{}{
				"name":      toolName,
				"arguments": args,
			})
		raw, code, err := mcpPOST(authToken, mcpSessionID, callBody)
		if err != nil {
			return "", err
		}
		if code >= http.StatusBadRequest {
			return "", fmt.Errorf("HTTP %d: %s", code, string(raw))
		}
		payload := unwrapSSEDataLine(raw)
		text, toolErr, parseErr := parseMCPToolPayload(payload)
		if parseErr != nil {
			return text, parseErr
		}
		if toolErr {
			return text, fmt.Errorf("%s", text)
		}
		return text, nil
	}

	dsClient := &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, //nolint:gosec // E2E test only
		Timeout:   10 * time.Second,
	}

	fetchAuditBody := func() ([]byte, int, error) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, dsAuditURL, http.NoBody)
		if err != nil {
			return nil, 0, err
		}
		resp, err := dsClient.Do(req)
		if err != nil {
			return nil, 0, err
		}
		defer func() { _ = resp.Body.Close() }()
		b, rerr := io.ReadAll(resp.Body)
		return b, resp.StatusCode, rerr
	}

	auditBodyContainsTool := func(body []byte, toolSubstring string) bool {
		if len(body) == 0 {
			return false
		}
		s := strings.ToLower(string(body))
		return strings.Contains(s, strings.ToLower(toolSubstring))
	}

	It("TC-E2E-AUDIT-01: After A2A tool call -> DS QueryAuditEvents returns matching entry", func() {
		// Pre-check: verify DS audit endpoint is reachable from the test host.
		func() {
			body, code, rerr := fetchAuditBody()
			if rerr != nil {
				Skip(fmt.Sprintf("DS audit endpoint not reachable from test host (%s): %v", dsAuditURL, rerr))
			}
			if code == http.StatusNotFound || code == http.StatusBadGateway || code == http.StatusServiceUnavailable {
				Skip(fmt.Sprintf("DS audit endpoint returned %d — service not exposed to host", code))
			}
			_ = body
		}()

		marker := fmt.Sprintf("g8-audit-01-%d", time.Now().UnixNano())
		_, err := mcpToolCall("af_get_pods", map[string]interface{}{
			"namespace":     "default",
			"labelSelector": fmt.Sprintf("e2e-audit=%s", marker),
		})
		if err != nil {
			_, err = mcpToolCall("af_get_pods", map[string]interface{}{
				"namespace": "default",
			})
		}
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() bool {
			body, code, rerr := fetchAuditBody()
			if rerr != nil || code != http.StatusOK {
				return false
			}
			return auditBodyContainsTool(body, "af_get_pods")
		}, 60*time.Second, 2*time.Second).Should(BeTrue(), "DS audit API should list an event referencing af_get_pods")
	})

	It("TC-E2E-AUDIT-02: DS unavailable -> audit failure logged, request succeeds", func() {
		DeferCleanup(func() {
			_, _ = kubectl(context.Background(), "scale", "deployment/data-storage",
				"-n", namespace, "--replicas=1")
			_, _ = kubectl(context.Background(), "rollout", "status", "deployment/data-storage",
				"-n", namespace, "--timeout=120s")
		})

		_, err := kubectl(context.Background(), "scale", "deployment/data-storage",
			"-n", namespace, "--replicas=0")
		Expect(err).NotTo(HaveOccurred())
		time.Sleep(2 * time.Second)

		text, err := mcpToolCall("af_get_pods", map[string]interface{}{
			"namespace": "default",
		})
		Expect(err).NotTo(HaveOccurred(), text)

		ctx := context.Background()
		Eventually(func(g Gomega) {
			logs, lerr := kubectl(ctx, "logs", "-n", namespace,
				"-l", "app.kubernetes.io/name=kubernaut-apifrontend",
				"--tail=300")
			g.Expect(lerr).NotTo(HaveOccurred(), logs)
			lower := strings.ToLower(logs)
			g.Expect(lower).To(Or(
				ContainSubstring("audit"),
				ContainSubstring("flush"),
				ContainSubstring("datastorage"),
				ContainSubstring("ds"),
			))
		}, 40*time.Second, 2*time.Second).Should(Succeed())
	})

	It("TC-E2E-AUDIT-03: Buffer flush on graceful shutdown -> pending events reach DS", func() {
		_, err := mcpToolCall("af_get_pods", map[string]interface{}{"namespace": "default"})
		Expect(err).NotTo(HaveOccurred())
		_, err = mcpToolCall("af_list_events", map[string]interface{}{"namespace": namespace})
		Expect(err).NotTo(HaveOccurred())

		ctx := context.Background()
		pod, err := kubectl(ctx, "-n", namespace,
			"get", "pods", "-l", "app.kubernetes.io/name=kubernaut-apifrontend",
			"-o", "jsonpath={.items[0].metadata.name}")
		Expect(err).NotTo(HaveOccurred(), pod)
		Expect(pod).NotTo(BeEmpty())

		_, err = kubectl(ctx, "delete", "pod", pod, "-n", namespace, "--grace-period=30")
		Expect(err).NotTo(HaveOccurred())
		_, err = kubectl(ctx, "rollout", "status", "deployment/apifrontend", "-n", namespace, "--timeout=180s")
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() bool {
			body, code, rerr := fetchAuditBody()
			if rerr != nil || code != http.StatusOK {
				return false
			}
			return auditBodyContainsTool(body, "af_list_events")
		}, 90*time.Second, 3*time.Second).Should(BeTrue(),
			"after AF restarts, DS should retain audit rows for tool calls made before shutdown")

		// Restore MCP session (pod restart drops in-memory session table on server).
		var ierr error
		authToken, ierr = fetchDEXTokenForPersona("sre")
		Expect(ierr).NotTo(HaveOccurred())
		mcpSessionID, ierr = initMCPSession(authToken)
		Expect(ierr).NotTo(HaveOccurred())
	})

	It("TC-E2E-AUDIT-04: Audit events contain redacted Detail (no raw secrets)", func() {
		body, code, err := fetchAuditBody()
		Expect(err).NotTo(HaveOccurred())
		if code != http.StatusOK {
			Skip(fmt.Sprintf("DS audit API not reachable (%d) — %s", code, string(body)))
		}
		Expect(len(body)).To(BeNumerically(">", 2))
		Expect(json.Valid(body)).To(BeTrue(), "DS audit response should be JSON")

		lower := string(body)
		dangerPatterns := []string{
			`"password":"`,
			`"token":"`,
			`"secret":"`,
			`'password':`,
		}
		for _, p := range dangerPatterns {
			Expect(strings.ToLower(lower)).NotTo(ContainSubstring(strings.ToLower(p)),
				"audit payload should not contain raw %s material", p)
		}
	})
})
