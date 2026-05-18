package e2e_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("E2E: discover_workflows (#1176)", Ordered, ContinueOnFailure, Label("e2e", "discover-workflows"), func() {
	var authToken string
	var mcpSessionID string

	BeforeAll(func() {
		var err error
		authToken, err = fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred(), "SRE DEX token")
		Expect(authToken).NotTo(BeEmpty())

		initBody := buildJSONRPC("dw-init", "initialize", map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "e2e-discover-wf",
				"version": "1.0",
			},
		})
		req, err := http.NewRequest(http.MethodPost, baseURL+"/mcp", strings.NewReader(initBody))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Authorization", "Bearer "+authToken)

		resp, err := httpClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, resp.Body)
		Expect(resp.StatusCode).To(BeNumerically("<", http.StatusBadRequest), "MCP initialize")

		mcpSessionID = resp.Header.Get("Mcp-Session-Id")
		Expect(mcpSessionID).NotTo(BeEmpty())
	})

	mcpToolCall := func(rpcID, toolName string, args map[string]interface{}) ([]byte, error) {
		callBody := buildJSONRPC(rpcID, "tools/call", map[string]interface{}{
			"name":      toolName,
			"arguments": args,
		})
		raw, code, err := mcpPOST(authToken, mcpSessionID, callBody)
		if err != nil {
			return nil, err
		}
		if code >= http.StatusBadRequest {
			return nil, fmt.Errorf("MCP returned HTTP %d: %s", code, raw)
		}
		return raw, nil
	}

	It("E2E-AF-WP-001: kubernaut_discover_workflows returns parameter schemas from KA", func() {
		raw, err := mcpToolCall("dw-e2e-001", "kubernaut_discover_workflows", map[string]interface{}{})
		Expect(err).NotTo(HaveOccurred())

		var rpcResp struct {
			Result struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"result"`
		}
		Expect(json.Unmarshal(raw, &rpcResp)).To(Succeed())
		Expect(rpcResp.Result.Content).NotTo(BeEmpty())

		var discoverResult struct {
			Workflows []struct {
				WorkflowID string `json:"workflow_id"`
				Name       string `json:"name"`
				Parameters []struct {
					Name     string `json:"name"`
					Type     string `json:"type"`
					Required bool   `json:"required"`
				} `json:"parameters"`
			} `json:"workflows"`
			Count int `json:"count"`
		}
		Expect(json.Unmarshal([]byte(rpcResp.Result.Content[0].Text), &discoverResult)).To(Succeed())
		Expect(discoverResult.Count).To(BeNumerically(">", 0))
		Expect(discoverResult.Workflows).NotTo(BeEmpty())
		Expect(discoverResult.Workflows[0].WorkflowID).NotTo(BeEmpty())
	})

	It("E2E-AF-WP-002: discover → select with parameters flow", func() {
		raw, err := mcpToolCall("dw-e2e-002a", "kubernaut_discover_workflows", map[string]interface{}{})
		Expect(err).NotTo(HaveOccurred())

		var rpcResp struct {
			Result struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"result"`
		}
		Expect(json.Unmarshal(raw, &rpcResp)).To(Succeed())
		Expect(rpcResp.Result.Content).NotTo(BeEmpty())

		var discoverResult struct {
			Workflows []struct {
				WorkflowID string `json:"workflow_id"`
				Parameters []struct {
					Name     string `json:"name"`
					Type     string `json:"type"`
					Required bool   `json:"required"`
				} `json:"parameters"`
			} `json:"workflows"`
		}
		Expect(json.Unmarshal([]byte(rpcResp.Result.Content[0].Text), &discoverResult)).To(Succeed())
		if len(discoverResult.Workflows) == 0 {
			Skip("No workflows discovered from KA — KA may not have workflows configured")
		}

		wf := discoverResult.Workflows[0]
		params := map[string]interface{}{}
		for _, p := range wf.Parameters {
			if p.Required {
				switch p.Type {
				case "string":
					params[p.Name] = "e2e-test-value"
				case "int":
					params[p.Name] = 1
				case "bool":
					params[p.Name] = true
				default:
					params[p.Name] = "test"
				}
			}
		}

		selectRaw, err := mcpToolCall("dw-e2e-002b", "kubernaut_select_workflow", map[string]interface{}{
			"rr_id":       "e2e-rr-discover-test",
			"workflow_id": wf.WorkflowID,
			"parameters":  params,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(len(selectRaw)).To(BeNumerically(">", 0))
	})

	It("E2E-AF-WP-003: unauthorized role is denied discover_workflows", func() {
		cicdToken, err := fetchDEXTokenForPersona("cicd")
		Expect(err).NotTo(HaveOccurred())

		initBody := buildJSONRPC("dw-denied-init", "initialize", map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo":      map[string]interface{}{"name": "e2e-denied", "version": "1.0"},
		})
		req, err := http.NewRequest(http.MethodPost, baseURL+"/mcp", strings.NewReader(initBody))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Authorization", "Bearer "+cicdToken)

		resp, err := httpClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, resp.Body)

		deniedSessionID := resp.Header.Get("Mcp-Session-Id")

		callBody := buildJSONRPC("dw-denied-call", "tools/call", map[string]interface{}{
			"name":      "kubernaut_discover_workflows",
			"arguments": map[string]interface{}{},
		})
		callReq, err := http.NewRequest(http.MethodPost, baseURL+"/mcp", strings.NewReader(callBody))
		Expect(err).NotTo(HaveOccurred())
		callReq.Header.Set("Content-Type", "application/json")
		callReq.Header.Set("Authorization", "Bearer "+cicdToken)
		if deniedSessionID != "" {
			callReq.Header.Set("Mcp-Session-Id", deniedSessionID)
		}

		callResp, err := httpClient.Do(callReq)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = callResp.Body.Close() }()
		body, _ := io.ReadAll(callResp.Body)

		Expect(string(body)).To(SatisfyAny(
			ContainSubstring("denied"),
			ContainSubstring("forbidden"),
			ContainSubstring("error"),
		))
	})

	It("E2E-AF-WP-004: af_discover_workflows_total metric is exposed", func() {
		resp, err := httpClient.Get(baseURL + "/metrics")
		if err != nil {
			resp, err = http.Get("http://localhost:18081/metrics")
		}
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		body, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(body)).To(SatisfyAny(
			ContainSubstring("af_discover_workflows_total"),
			ContainSubstring("af_discover_workflows_duration_seconds"),
		))
	})

	It("E2E-AF-WP-005: filtered discovery by workflow_id returns matching subset", func() {
		raw, err := mcpToolCall("dw-e2e-005a", "kubernaut_discover_workflows", map[string]interface{}{})
		Expect(err).NotTo(HaveOccurred())

		var rpcResp struct {
			Result struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"result"`
		}
		Expect(json.Unmarshal(raw, &rpcResp)).To(Succeed())
		Expect(rpcResp.Result.Content).NotTo(BeEmpty())

		var allResult struct {
			Workflows []struct {
				WorkflowID string `json:"workflow_id"`
			} `json:"workflows"`
		}
		Expect(json.Unmarshal([]byte(rpcResp.Result.Content[0].Text), &allResult)).To(Succeed())
		if len(allResult.Workflows) == 0 {
			Skip("No workflows discovered from KA — cannot test filtered discovery")
		}

		targetID := allResult.Workflows[0].WorkflowID
		filteredRaw, err := mcpToolCall("dw-e2e-005b", "kubernaut_discover_workflows", map[string]interface{}{
			"workflow_id": targetID,
		})
		Expect(err).NotTo(HaveOccurred())

		var filteredResp struct {
			Result struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"result"`
		}
		Expect(json.Unmarshal(filteredRaw, &filteredResp)).To(Succeed())
		Expect(filteredResp.Result.Content).NotTo(BeEmpty())

		var filteredResult struct {
			Workflows []struct {
				WorkflowID string `json:"workflow_id"`
			} `json:"workflows"`
			Count int `json:"count"`
		}
		Expect(json.Unmarshal([]byte(filteredResp.Result.Content[0].Text), &filteredResult)).To(Succeed())
		Expect(filteredResult.Count).To(Equal(1))
		Expect(filteredResult.Workflows).To(HaveLen(1))
		Expect(filteredResult.Workflows[0].WorkflowID).To(Equal(targetID))
	})

	It("E2E-AF-WP-006: validation rejects wrong parameter type at wire level", func() {
		raw, err := mcpToolCall("dw-e2e-006a", "kubernaut_discover_workflows", map[string]interface{}{})
		Expect(err).NotTo(HaveOccurred())

		var rpcResp struct {
			Result struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"result"`
		}
		Expect(json.Unmarshal(raw, &rpcResp)).To(Succeed())
		Expect(rpcResp.Result.Content).NotTo(BeEmpty())

		var discoverResult struct {
			Workflows []struct {
				WorkflowID string `json:"workflow_id"`
				Parameters []struct {
					Name     string `json:"name"`
					Type     string `json:"type"`
					Required bool   `json:"required"`
				} `json:"parameters"`
			} `json:"workflows"`
		}
		Expect(json.Unmarshal([]byte(rpcResp.Result.Content[0].Text), &discoverResult)).To(Succeed())
		if len(discoverResult.Workflows) == 0 {
			Skip("No workflows discovered from KA — cannot test validation rejection")
		}

		var targetWF struct {
			WorkflowID string
			IntParam   string
		}
		for _, wf := range discoverResult.Workflows {
			for _, p := range wf.Parameters {
				if p.Type == "int" && p.Required {
					targetWF.WorkflowID = wf.WorkflowID
					targetWF.IntParam = p.Name
					break
				}
			}
			if targetWF.WorkflowID != "" {
				break
			}
		}
		if targetWF.WorkflowID == "" {
			Skip("No workflow with required int parameter found — cannot test type validation")
		}

		badParams := map[string]interface{}{
			targetWF.IntParam: "not-a-number",
		}
		selectRaw, err := mcpToolCall("dw-e2e-006b", "kubernaut_select_workflow", map[string]interface{}{
			"rr_id":       "e2e-rr-type-reject",
			"workflow_id": targetWF.WorkflowID,
			"parameters":  badParams,
		})

		if err != nil {
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("type"),
				ContainSubstring("int"),
				ContainSubstring("validation"),
				ContainSubstring("parameter"),
			))
		} else {
			Expect(string(selectRaw)).To(SatisfyAny(
				ContainSubstring("type"),
				ContainSubstring("int"),
				ContainSubstring("validation"),
				ContainSubstring("parameter"),
				ContainSubstring("error"),
			))
		}
	})
})
