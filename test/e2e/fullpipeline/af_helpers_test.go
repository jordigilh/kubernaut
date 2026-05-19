package fullpipeline

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/gomega"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ────────────────────────────────────────────────────────────────────────────
// JSON-RPC helpers (adapted from test/e2e/apifrontend/helpers_test.go)
// ────────────────────────────────────────────────────────────────────────────

func fpBuildJSONRPC(id, method string, params map[string]interface{}) string {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"id":      id,
		"params":  params,
	}
	b, _ := json.Marshal(payload)
	return string(b)
}

// fpA2ATasksSend builds a message/send JSON-RPC payload with a user text message.
func fpA2ATasksSend(id, text string) string {
	return fpBuildJSONRPC(id, "message/send", map[string]interface{}{
		"message": map[string]interface{}{
			"messageId": "msg-" + id,
			"role":      "user",
			"parts": []map[string]interface{}{
				{"kind": "text", "text": text},
			},
		},
	})
}

// fpA2ATasksSendWithTask continues an existing A2A task by including taskId.
func fpA2ATasksSendWithTask(id, taskID, text string) string {
	return fpBuildJSONRPC(id, "message/send", map[string]interface{}{
		"id": taskID,
		"message": map[string]interface{}{
			"messageId": "msg-" + id,
			"role":      "user",
			"parts": []map[string]interface{}{
				{"kind": "text", "text": text},
			},
		},
	})
}

type fpRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      string          `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *fpRPCError     `json:"error"`
}

type fpRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type fpA2ATaskResult struct {
	ID     string `json:"id"`
	Status struct {
		State   string          `json:"state"`
		Message json.RawMessage `json:"message,omitempty"`
	} `json:"status"`
}

// fpA2AInvoke sends a JSON-RPC request to POST /a2a/invoke.
func fpA2AInvoke(body string) (*http.Response, error) {
	token := getAFToken()
	req, err := http.NewRequest(http.MethodPost, afBaseURL+"/a2a/invoke", strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	return afHTTPClient.Do(req)
}

func fpParseRPC(resp *http.Response) (fpRPCResponse, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fpRPCResponse{}, err
	}
	var r fpRPCResponse
	err = json.Unmarshal(body, &r)
	return r, err
}

func fpExtractTask(raw json.RawMessage) (fpA2ATaskResult, error) {
	var task fpA2ATaskResult
	err := json.Unmarshal(raw, &task)
	return task, err
}

// ────────────────────────────────────────────────────────────────────────────
// MCP helpers
// ────────────────────────────────────────────────────────────────────────────

func fpInitMCPSession() (string, error) {
	token := getAFToken()
	body := fpBuildJSONRPC("init-1", "initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "fp-e2e",
			"version": "1.0",
		},
	})
	req, err := http.NewRequest(http.MethodPost, afBaseURL+"/mcp", strings.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := afHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("MCP initialize: HTTP %d", resp.StatusCode)
	}
	return resp.Header.Get("Mcp-Session-Id"), nil
}

func fpMCPCall(sessionID, toolName string, args map[string]interface{}) ([]byte, int, error) {
	token := getAFToken()
	body := fpBuildJSONRPC("call-1", "tools/call", map[string]interface{}{
		"name":      toolName,
		"arguments": args,
	})
	req, err := http.NewRequest(http.MethodPost, afBaseURL+"/mcp", strings.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	resp, err := afHTTPClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	return respBody, resp.StatusCode, err
}

// ────────────────────────────────────────────────────────────────────────────
// Pipeline polling helpers
// ────────────────────────────────────────────────────────────────────────────

// fpWaitForRR polls for a RemediationRequest containing nameSubstring in its name.
// Returns the RR name or fails after timeout.
func fpWaitForRR(nameSubstring string, timeout time.Duration) string {
	var rrName string
	Eventually(func() bool {
		rrList := &remediationv1.RemediationRequestList{}
		if err := apiReader.List(ctx, rrList, client.InNamespace(namespace)); err != nil {
			return false
		}
		for _, rr := range rrList.Items {
			if strings.Contains(rr.Name, nameSubstring) {
				rrName = rr.Name
				return true
			}
		}
		return false
	}, timeout, 2*time.Second).Should(BeTrue(), "RemediationRequest with %q not found", nameSubstring)
	return rrName
}

// fpWaitForWEComplete waits until a WorkflowExecution for the given RR reaches Completed phase.
func fpWaitForWEComplete(rrName string, timeout time.Duration) {
	Eventually(func() bool {
		weList := &workflowexecutionv1.WorkflowExecutionList{}
		if err := apiReader.List(ctx, weList, client.InNamespace("kubernaut-workflows")); err != nil {
			return false
		}
		for _, we := range weList.Items {
			if strings.Contains(we.Name, rrName) || we.Spec.RemediationRequestRef.Name == rrName {
				return we.Status.Phase == "Completed"
			}
		}
		return false
	}, timeout, 3*time.Second).Should(BeTrue(), "WorkflowExecution for %q did not complete", rrName)
}
