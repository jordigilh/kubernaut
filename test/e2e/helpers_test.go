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
	"strings"
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

// a2aTasksSend builds a tasks/send JSON-RPC payload with a user text message.
func a2aTasksSend(id, text string) string {
	return buildJSONRPC(id, "tasks/send", map[string]interface{}{
		"message": map[string]interface{}{
			"role": "user",
			"parts": []map[string]interface{}{
				{"type": "text", "text": text},
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
		State   string `json:"state"`
		Message string `json:"message,omitempty"`
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
	return &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
	}
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
