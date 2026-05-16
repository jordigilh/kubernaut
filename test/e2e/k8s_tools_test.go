package e2e_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("K8s Read Tools (G6)", Ordered, Label("e2e", "phase2", "g6"), func() {
	var authToken string
	var mcpSessionID string

	BeforeAll(func() {
		var err error
		authToken, err = fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred(), "SRE DEX token")
		Expect(authToken).NotTo(BeEmpty())

		initBody := buildJSONRPC("k8s-init", "initialize", map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "e2e-k8s",
				"version": "1.0",
			},
		})
		req, err := http.NewRequest(http.MethodPost, baseURL+"/mcp", strings.NewReader(initBody))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+authToken)

		resp, err := httpClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, resp.Body)
		Expect(resp.StatusCode).To(BeNumerically("<", http.StatusBadRequest), "MCP initialize")

		mcpSessionID = resp.Header.Get("Mcp-Session-Id")
		Expect(mcpSessionID).NotTo(BeEmpty())
	})

	mcpToolCall := func(rpcID, toolName string, args map[string]interface{}) (string, error) {
		callBody := buildJSONRPC(rpcID, "tools/call", map[string]interface{}{
			"name":      toolName,
			"arguments": args,
		})
		req, err := http.NewRequest(http.MethodPost, baseURL+"/mcp", strings.NewReader(callBody))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+authToken)
		req.Header.Set("Mcp-Session-Id", mcpSessionID)

		resp, err := httpClient.Do(req)
		if err != nil {
			return "", err
		}
		defer func() { _ = resp.Body.Close() }()
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		if resp.StatusCode >= http.StatusBadRequest {
			return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(raw))
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

	It("TC-E2E-K8S-01: af_get_pods returns apifrontend, mock-llm, or dex pods in kubernaut-system", func() {
		text, err := mcpToolCall("k8s-01", "af_get_pods", map[string]interface{}{
			"namespace": e2eNamespace,
		})
		Expect(err).NotTo(HaveOccurred(), "af_get_pods: %s", text)

		lower := strings.ToLower(text)
		Expect(lower).To(Or(
			ContainSubstring("apifrontend"),
			ContainSubstring("mock-llm"),
			ContainSubstring("dex"),
		), "expected at least one AF stack pod name in result: %s", text)
	})

	It("TC-E2E-K8S-02: af_list_events returns recent events for kubernaut-system", func() {
		Eventually(func(g Gomega) {
			text, err := mcpToolCall("k8s-02", "af_list_events", map[string]interface{}{
				"namespace": e2eNamespace,
			})
			g.Expect(err).NotTo(HaveOccurred(), text)

			var out map[string]interface{}
			g.Expect(json.Unmarshal([]byte(text), &out)).To(Succeed())
			events, ok := out["events"].([]interface{})
			g.Expect(ok).To(BeTrue(), "result should include events array: %s", text)
			g.Expect(len(events)).To(BeNumerically(">", 0), "expected non-empty events list: %s", text)
		}, 2*time.Minute, 3*time.Second).Should(Succeed())
	})

	It("TC-E2E-K8S-03: af_get_workloads lists apifrontend and kubernaut-agent deployments", func() {
		text, err := mcpToolCall("k8s-03", "af_get_workloads", map[string]interface{}{
			"namespace": e2eNamespace,
		})
		Expect(err).NotTo(HaveOccurred(), text)

		var out map[string]interface{}
		Expect(json.Unmarshal([]byte(text), &out)).To(Succeed())
		workloads, ok := out["workloads"].([]interface{})
		Expect(ok).To(BeTrue(), "result should include workloads: %s", text)

		namesLower := make([]string, 0, len(workloads))
		for _, w := range workloads {
			m, isMap := w.(map[string]interface{})
			if !isMap {
				continue
			}
			if n, ok := m["name"].(string); ok {
				namesLower = append(namesLower, strings.ToLower(n))
			}
		}
		j := strings.Join(namesLower, " ")
		Expect(j).To(And(
			ContainSubstring("apifrontend"),
			ContainSubstring("kubernaut-agent"),
		), "expected deployment names in workloads: %s", text)
	})

	It("TC-E2E-K8S-04: af_resolve_owner walks pod owner to ReplicaSet or Deployment", func() {
		podsText, err := mcpToolCall("k8s-04a", "af_get_pods", map[string]interface{}{
			"namespace": e2eNamespace,
		})
		Expect(err).NotTo(HaveOccurred(), podsText)

		var podsOut map[string]interface{}
		Expect(json.Unmarshal([]byte(podsText), &podsOut)).To(Succeed())
		items, ok := podsOut["pods"].([]interface{})
		Expect(ok).To(BeTrue(), "pods: %s", podsText)
		Expect(len(items)).To(BeNumerically(">", 0))

		var sawWorkloadOwner bool
		for i, elem := range items {
			pm, isMap := elem.(map[string]interface{})
			if !isMap {
				continue
			}
			podName, ok := pm["name"].(string)
			if !ok || podName == "" {
				continue
			}
			id := fmt.Sprintf("k8s-04b-%d", i)
			resText, err := mcpToolCall(id, "af_resolve_owner", map[string]interface{}{
				"namespace": e2eNamespace,
				"kind":      "Pod",
				"name":      podName,
			})
			if err != nil {
				continue
			}
			joined := strings.ToLower(resText)
			if strings.Contains(joined, "replicaset") || strings.Contains(joined, "deployment") {
				sawWorkloadOwner = true
				break
			}
		}
		Expect(sawWorkloadOwner).To(BeTrue(),
			"expected af_resolve_owner for some pod to reach ReplicaSet or Deployment across: %s", podsText)
	})

	It("TC-E2E-K8S-05: af_get_pods with kube-system shows system pods (e.g. CoreDNS)", func() {
		text, err := mcpToolCall("k8s-05", "af_get_pods", map[string]interface{}{
			"namespace": "kube-system",
		})
		Expect(err).NotTo(HaveOccurred(), text)

		lower := strings.ToLower(text)
		Expect(lower).To(Or(
			ContainSubstring("coredns"),
			ContainSubstring("kube-proxy"),
			ContainSubstring("kindnet"),
			ContainSubstring("etcd"),
		), "expected a recognizable kube-system pod in result: %s", text)
	})

	It("TC-E2E-K8S-06: af_get_pods in non-existent namespace returns empty list without error", func() {
		text, err := mcpToolCall("k8s-06", "af_get_pods", map[string]interface{}{
			"namespace": "nonexistent-ns-12345",
		})
		Expect(err).NotTo(HaveOccurred(), text)

		var out map[string]interface{}
		Expect(json.Unmarshal([]byte(text), &out)).To(Succeed())
		pods, ok := out["pods"].([]interface{})
		Expect(ok).To(BeTrue(), "pods array: %s", text)
		Expect(pods).To(BeEmpty())

		if c, has := out["count"].(float64); has {
			Expect(int(c)).To(Equal(0))
		}
	})

	It("TC-E2E-K8S-07: af_get_workloads denied for observability on restricted namespace (K8s RBAC)", func() {
		obsToken, err := fetchDEXTokenForPersona("observability")
		Expect(err).NotTo(HaveOccurred())

		obsInit := buildJSONRPC("k8s-07-init", "initialize", map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "e2e-k8s-obs",
				"version": "1.0",
			},
		})
		initReq, err := http.NewRequest(http.MethodPost, baseURL+"/mcp", strings.NewReader(obsInit))
		Expect(err).NotTo(HaveOccurred())
		initReq.Header.Set("Content-Type", "application/json")
		initReq.Header.Set("Authorization", "Bearer "+obsToken)
		initResp, err := httpClient.Do(initReq)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = initResp.Body.Close() }()
		_, _ = io.Copy(io.Discard, initResp.Body)
		Expect(initResp.StatusCode).To(BeNumerically("<", http.StatusBadRequest))
		obsSession := initResp.Header.Get("Mcp-Session-Id")
		Expect(obsSession).NotTo(BeEmpty())

		callBody := buildJSONRPC("k8s-07", "tools/call", map[string]interface{}{
			"name": "af_get_workloads",
			"arguments": map[string]interface{}{
				"namespace": "kube-system",
			},
		})
		req, err := http.NewRequest(http.MethodPost, baseURL+"/mcp", strings.NewReader(callBody))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+obsToken)
		req.Header.Set("Mcp-Session-Id", obsSession)

		resp, err := httpClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		raw, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(BeNumerically("<", http.StatusBadRequest))

		payload := unwrapSSEDataLine(raw)
		text, toolErr, perr := parseMCPToolPayload(payload)
		Expect(perr).NotTo(HaveOccurred())
		Expect(toolErr).To(BeTrue(), "expected tool error for RBAC denial, got: %s", text)
		lower := strings.ToLower(text)
		Expect(lower).To(Or(
			ContainSubstring("forbidden"),
			ContainSubstring("denied"),
			ContainSubstring("access denied"),
			ContainSubstring("rbac"),
			ContainSubstring("cannot list"),
			ContainSubstring("not allowed"),
		), "expected RBAC or forbidden wording: %s", text)
	})
})
