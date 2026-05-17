package e2e_test

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// persona holds credentials for a DEX E2E user with a specific RBAC role.
type persona struct {
	Email    string
	Password string
	Role     string
}

// e2ePersonas defines all 6 RBAC personas available in E2E.
var e2ePersonas = map[string]persona{
	"sre":                  {Email: "sre@kubernaut.ai", Password: "password", Role: "sre"},
	"ai-orchestrator":      {Email: "orchestrator@kubernaut.ai", Password: "password", Role: "ai-orchestrator"},
	"cicd":                 {Email: "cicd@kubernaut.ai", Password: "password", Role: "cicd"},
	"observability":        {Email: "observability@kubernaut.ai", Password: "password", Role: "observability"},
	"l3-audit":             {Email: "auditor@kubernaut.ai", Password: "password", Role: "l3-audit"},
	"remediation-approver": {Email: "approver@kubernaut.ai", Password: "password", Role: "remediation-approver"},
}

// fetchDEXTokenForPersona obtains a JWT for the given persona role.
func fetchDEXTokenForPersona(role string) (string, error) {
	p, ok := e2ePersonas[role]
	if !ok {
		return "", fmt.Errorf("unknown persona role: %s", role)
	}
	return fetchDEXToken(dexURL, clientID, clientSecret, p.Email, p.Password)
}

// a2aInvoke sends a JSON-RPC request to POST /a2a/invoke with the given auth token.
func a2aInvoke(client *http.Client, base, token, body string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, base+"/a2a/invoke", strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	return client.Do(req)
}

// buildJSONRPC builds a JSON-RPC 2.0 request string.
func buildJSONRPC(id, method string, params map[string]interface{}) string {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"id":      id,
		"params":  params,
	}
	b, _ := json.Marshal(payload)
	return string(b)
}

// a2aTasksSend builds a message/send JSON-RPC payload with a user text message.
func a2aTasksSend(id, text string) string {
	return buildJSONRPC(id, "message/send", map[string]interface{}{
		"message": map[string]interface{}{
			"messageId": "msg-" + id,
			"role":      "user",
			"parts": []map[string]interface{}{
				{"kind": "text", "text": text},
			},
		},
	})
}

// a2aMessageStream builds a message/stream JSON-RPC payload (SSE variant).
func a2aMessageStream(id, text string) string {
	return buildJSONRPC(id, "message/stream", map[string]interface{}{
		"message": map[string]interface{}{
			"messageId": "msg-" + id,
			"role":      "user",
			"parts": []map[string]interface{}{
				{"kind": "text", "text": text},
			},
		},
	})
}

// rpcResponse represents a JSON-RPC 2.0 response.
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      string          `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// a2aTaskResult represents the task object in an A2A response.
type a2aTaskResult struct {
	ID     string `json:"id"`
	Status struct {
		State   string          `json:"state"`
		Message json.RawMessage `json:"message,omitempty"`
	} `json:"status"`
}

// parseRPCResponse reads and parses a JSON-RPC response from an http.Response.
func parseRPCResponse(resp *http.Response) (rpcResponse, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return rpcResponse{}, err
	}
	var r rpcResponse
	err = json.Unmarshal(body, &r)
	return r, err
}

// extractTaskFromResult unmarshals the result field into an a2aTaskResult.
func extractTaskFromResult(raw json.RawMessage) (a2aTaskResult, error) {
	var task a2aTaskResult
	err := json.Unmarshal(raw, &task)
	return task, err
}

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func newTLSClient(caCertPath string) *http.Client {
	base := newTLSTransport(caCertPath)
	return &http.Client{
		Transport: &retryOn429Transport{base: base, maxRetries: 5, baseDelay: 500 * time.Millisecond},
	}
}

func newTLSTransport(caCertPath string) *http.Transport {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if caCertPath != "" {
		caCert, err := os.ReadFile(caCertPath)
		if err != nil {
			panic(fmt.Sprintf("read CA cert: %v", err))
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caCert) {
			panic("failed to add CA cert to pool")
		}
		tlsCfg.RootCAs = pool
	}
	return &http.Transport{TLSClientConfig: tlsCfg}
}

// retryOn429Transport automatically retries on HTTP 429 with exponential
// backoff. Mirrors kubernaut/test/shared/auth.RetryOn429Transport so that
// parallel E2E procs absorb transient rate-limiter rejections without
// requiring inflated limits.
type retryOn429Transport struct {
	base       http.RoundTripper
	maxRetries int
	baseDelay  time.Duration
}

func (t *retryOn429Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	delay := t.baseDelay
	const maxDelay = 4 * time.Second

	for attempt := 0; ; attempt++ {
		resp, err := t.base.RoundTrip(req)
		if err != nil {
			return resp, err
		}
		if resp.StatusCode != http.StatusTooManyRequests || attempt >= t.maxRetries {
			return resp, nil
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()

		wait := delay
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, parseErr := strconv.Atoi(ra); parseErr == nil && secs > 0 {
				wait = time.Duration(secs) * time.Second
			}
		}
		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(wait):
		}
		delay *= 2
		if delay > maxDelay {
			delay = maxDelay
		}
	}
}

// unwrapSSEDataLine extracts the JSON payload from an SSE "data:" line.
// MCP Streamable HTTP responses may be SSE-wrapped.
func unwrapSSEDataLine(raw []byte) string {
	s := string(raw)
	if !strings.Contains(s, "data:") {
		return strings.TrimSpace(s)
	}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "data:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		}
	}
	return strings.TrimSpace(s)
}

// initMCPSession performs the MCP initialize handshake and returns the session ID.
func initMCPSession(token string) (string, error) {
	body := buildJSONRPC("init-1", "initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "e2e",
			"version": "1.0",
		},
	})
	req, err := http.NewRequest(http.MethodPost, baseURL+"/mcp", strings.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient.Do(req)
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

// mcpPOST sends a JSON-RPC request to the MCP endpoint with auth + session headers.
func mcpPOST(token, sessionID, jsonBody string) (body []byte, statusCode int, err error) {
	req, err := http.NewRequest(http.MethodPost, baseURL+"/mcp", strings.NewReader(jsonBody))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err = io.ReadAll(resp.Body)
	statusCode = resp.StatusCode
	return body, statusCode, err
}

// parseMCPToolPayload extracts tool result text from a JSON-RPC MCP response.
func parseMCPToolPayload(payload string) (text string, toolIsError bool, err error) {
	var root map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &root); err != nil {
		return "", false, fmt.Errorf("parse MCP JSON: %w", err)
	}
	if e, ok := root["error"]; ok && e != nil {
		return "", false, fmt.Errorf("json-rpc error: %v", e)
	}
	res, ok := root["result"].(map[string]interface{})
	if !ok {
		return payload, false, nil
	}
	toolIsError, _ = res["isError"].(bool)
	text = extractMCPResultText(root)
	return text, toolIsError, nil
}

// extractMCPResultText walks result.content[0].text in a CallToolResult.
func extractMCPResultText(root map[string]interface{}) string {
	res, _ := root["result"].(map[string]interface{})
	if res == nil {
		return ""
	}
	content, _ := res["content"].([]interface{})
	if len(content) == 0 {
		return ""
	}
	first, _ := content[0].(map[string]interface{})
	if first == nil {
		return ""
	}
	t, _ := first["text"].(string)
	return t
}

// parseJSONStringField extracts a string field from a JSON object string.
func parseJSONStringField(jsonStr, field string) string {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
		return ""
	}
	v, _ := m[field].(string)
	return v
}

// fetchDEXToken performs an OAuth2 Resource Owner Password Credentials grant
// against DEX to obtain a valid ID token for E2E testing.
func fetchDEXToken(dexURL, clientID, clientSecret, username, password string) (string, error) {
	tokenURL := dexURL + "/token"
	data := url.Values{
		"grant_type":    {"password"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"username":      {username},
		"password":      {password},
		"scope":         {"openid email profile groups"},
	}

	resp, err := http.PostForm(tokenURL, data)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request returned %d: %s", resp.StatusCode, body)
	}

	var tokenResp struct {
		IDToken string `json:"id_token"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("unmarshal token response: %w", err)
	}
	if tokenResp.IDToken == "" {
		return "", fmt.Errorf("id_token not found in response: %s", body)
	}
	return tokenResp.IDToken, nil
}
