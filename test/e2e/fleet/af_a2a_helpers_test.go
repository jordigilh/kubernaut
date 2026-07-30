/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package fleet

// A2A helpers for driving the real AF binary via its /a2a/invoke endpoint
// (E2E-FLEET-016, issue #1768 Gaps A+C). Adapted from
// test/e2e/fullpipeline/af_helpers_test.go's fp* helpers -- duplicated
// rather than shared because both are internal _test.go files in different
// packages. Kept intentionally minimal: only what E2E-FLEET-016 needs, not
// the full fullpipeline helper surface (RR polling, artifact lookup, etc.).
//
// Auth: this suite's AF is deployed by the SAME SetupFullPipelineInfrastructure
// base (fleet_e2e.go's SetupFleetE2EInfrastructure calls it first) that backs
// the fullpipeline suite, so AF's own JWT provider config is untouched Dex
// (patchAPIFrontendConfigForFleet only appends a "fleet:" block for the
// FleetReaderFactory wiring -- it never touches "auth:"). Keycloak is added
// alongside Dex for the fleet-specific MCP-gateway/kube-mcp-server OAuth2
// path (fleetAuthenticatedHTTPClient in suite_test.go); it does not replace
// Dex as AF's own incoming-request validator. So the fullpipeline suite's
// Dex client_credentials-via-password-grant flow applies unchanged here.

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// afA2AAuthToken caches the Dex token across calls within a process (mirrors
// fullpipeline's afAuthToken caching in suite_test.go).
var afA2AAuthToken string

// getAFA2AToken fetches (and caches) a Dex password-grant token for AF's A2A
// endpoint. Same IdP, client, and user as test/e2e/fullpipeline's getAFToken --
// Dex is deployed once by SetupFullPipelineInfrastructure and inherited
// unchanged by this suite (see file-level doc comment).
func getAFA2AToken() string {
	if afA2AAuthToken != "" {
		return afA2AAuthToken
	}
	tlsClient, tlsErr := infrastructure.NewTLSAwareClient(kubeconfigPath, 10*time.Second)
	Expect(tlsErr).NotTo(HaveOccurred(), "TLS client for Dex token endpoint")

	resp, err := tlsClient.PostForm("https://localhost:30556/dex/token", url.Values{
		"grant_type":    {"password"},
		"client_id":     {"kubernaut-apifrontend"},
		"client_secret": {"e2e-client-secret"},
		"username":      {"sre@kubernaut.ai"},
		"password":      {"password"},
		"scope":         {"openid email profile groups"},
	})
	Expect(err).NotTo(HaveOccurred())
	defer func() { _ = resp.Body.Close() }()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	Expect(json.NewDecoder(resp.Body).Decode(&tokenResp)).To(Succeed())
	afA2AAuthToken = tokenResp.AccessToken
	return afA2AAuthToken
}

func afBuildJSONRPC(id, method string, params map[string]interface{}) string {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"id":      id,
		"params":  params,
	}
	b, _ := json.Marshal(payload)
	return string(b)
}

// afA2ATasksSend builds a message/send JSON-RPC payload with a user text
// message. Each call gets a unique contextId so the SessionInterceptor
// doesn't route independent tests to a shared session.
func afA2ATasksSend(id, text string) string {
	return afBuildJSONRPC(id, "message/send", map[string]interface{}{
		"message": map[string]interface{}{
			"messageId": "msg-" + id,
			"contextId": "ctx-" + id,
			"role":      "user",
			"parts": []map[string]interface{}{
				{"kind": "text", "text": text},
			},
		},
	})
}

type afRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type afRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      string          `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *afRPCError     `json:"error"`
}

type afA2ATaskResult struct {
	ID     string `json:"id"`
	Status struct {
		State   string          `json:"state"`
		Message json.RawMessage `json:"message,omitempty"`
	} `json:"status"`
	// Artifacts carries the accumulated text/data content for a synchronous
	// message/send call under OutputArtifactPerEvent mode. The terminal
	// TaskStatusUpdateEvent's own Message field is nil for a genuinely
	// successful completion -- ADK only populates it for the
	// failed/input-required cases (see eventProcessor.makeFinalStatusUpdate
	// in google.golang.org/adk/server/adka2a/v2/processor.go) -- so a
	// completed task's final answer must be read from here, mirroring the
	// established fpA2ATaskResult/fpA2AArtifact pattern in
	// test/e2e/fullpipeline/af_helpers_test.go.
	Artifacts []afA2AArtifact `json:"artifacts,omitempty"`
}

type afA2AArtifact struct {
	Parts []afA2AArtifactPart `json:"parts"`
}

type afA2AArtifactPart struct {
	Kind string `json:"kind"`
	Text string `json:"text,omitempty"`
}

// afArtifactText concatenates the text of every part in every artifact on
// task, i.e. the full accumulated final-answer text for a completed task
// under OutputArtifactPerEvent mode (see afA2ATaskResult.Artifacts doc).
func afArtifactText(task afA2ATaskResult) string {
	var text string
	for _, art := range task.Artifacts {
		for _, p := range art.Parts {
			text += p.Text
		}
	}
	return text
}

// afA2AInvokeWithTimeout sends a JSON-RPC request to POST /a2a/invoke with a
// custom timeout. A zero timeout uses the default afHTTPClient (30s).
func afA2AInvokeWithTimeout(body string, timeout time.Duration) (*http.Response, error) {
	token := getAFA2AToken()
	req, err := http.NewRequest(http.MethodPost, afBaseURL+"/a2a/invoke", strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	if timeout > 0 {
		client := &http.Client{
			Transport: afHTTPClient.Transport,
			Timeout:   timeout,
		}
		return client.Do(req)
	}
	return afHTTPClient.Do(req)
}

func afParseRPC(resp *http.Response) (afRPCResponse, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return afRPCResponse{}, err
	}
	var r afRPCResponse
	err = json.Unmarshal(body, &r)
	return r, err
}

func afExtractTask(raw json.RawMessage) (afA2ATaskResult, error) {
	var task afA2ATaskResult
	err := json.Unmarshal(raw, &task)
	return task, err
}
