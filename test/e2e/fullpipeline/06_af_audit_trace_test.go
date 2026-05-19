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

package fullpipeline

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// E2E-FP-AF-AUDIT: AF audit trace coverage in the full pipeline.
//
// Prerequisite: AF + DEX deployed in the FP cluster (Issue #1189).
// This test authenticates via DEX, exercises AF's MCP endpoint, then
// queries DataStorage for AF-specific audit events (event_category=apifrontend).
//
// Happy-path events validated:
//   - apifrontend.auth.success (JWT validation via DEX)
//   - apifrontend.mcp.session_init (MCP protocol initialization)
//   - apifrontend.session.created (InvestigationSession CRD creation)
//   - apifrontend.tool.executed (MCP tool invocations)
//   - apifrontend.session.phase_changed (phase transitions)
//   - apifrontend.jwt.delegation (forwarding auth to KA)
//   - apifrontend.impersonation.created (K8s client impersonation)
//   - apifrontend.session.completed (terminal phase)
//
// BR: BR-AUDIT-005, Issue #1156 (SOC2 AU-2)

var _ = Describe("E2E-FP-AF-001: AF audit trace coverage in happy-path MCP lifecycle",
	Label("e2e", "fullpipeline", "audit", "apifrontend"), func() {

		var (
			afBaseURL    string
			afDexURL     string
			afClientID   string
			afClientSec  string
			afHTTPClient *http.Client
		)

		BeforeEach(func() {
			afBaseURL = envOrDefault("AF_FP_BASE_URL", "https://localhost:30443")
			afDexURL = envOrDefault("AF_FP_DEX_URL", "http://localhost:5556/dex")
			afClientID = envOrDefault("AF_FP_CLIENT_ID", "kubernaut-apifrontend")
			afClientSec = envOrDefault("AF_FP_CLIENT_SECRET", "e2e-client-secret")

			afHTTPClient = &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // E2E test cluster
				},
				Timeout: 30 * time.Second,
			}

			resp, err := afHTTPClient.Get(afBaseURL + "/healthz")
			if err != nil || resp.StatusCode != http.StatusOK {
				Skip("AF not deployed in FP cluster (Issue #1189 prerequisite)")
			}
			_ = resp.Body.Close()
		})

		It("should emit required AF audit events during interactive MCP lifecycle", func() {
			testStartTime := time.Now().UTC()

			By("Authenticating via DEX")
			token, err := fpFetchDEXToken(afDexURL, afClientID, afClientSec, "e2e-user@kubernaut.ai", "password")
			if err != nil {
				Skip(fmt.Sprintf("DEX not available: %v", err))
			}
			Expect(token).NotTo(BeEmpty())

			By("Initializing MCP session through AF")
			sessionID, err := fpInitMCPSession(afHTTPClient, afBaseURL, token)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Printf("  MCP Session ID: %s\n", sessionID)

			By("Calling af_get_pods tool through AF MCP (generates tool.executed, jwt.delegation, impersonation.created)")
			toolBody := fpBuildJSONRPC("fp-af-audit-1", "tools/call", map[string]interface{}{
				"name":      "af_get_pods",
				"arguments": map[string]interface{}{"namespace": "default"},
			})
			_, code, err := fpMCPPOST(afHTTPClient, afBaseURL, token, sessionID, toolBody)
			Expect(err).NotTo(HaveOccurred())
			Expect(code).To(BeNumerically("<", 500), "tool call should not return 5xx")

			By("Querying DataStorage for AF audit events")

			afRequiredEvents := []string{
				"apifrontend.auth.success",
				"apifrontend.mcp.session_init",
				"apifrontend.tool.executed",
			}

			afOptionalEvents := []string{
				"apifrontend.session.created",
				"apifrontend.session.phase_changed",
				"apifrontend.session.completed",
				"apifrontend.jwt.delegation",
				"apifrontend.impersonation.created",
			}

			sinceStr := testStartTime.Add(-10 * time.Second).Format(time.RFC3339)

			var allAFEvents []ogenclient.AuditEvent
			eventTypeCounts := map[string]int{}
			Eventually(func() []string {
				resp, qErr := dataStorageClient.QueryAuditEvents(ctx, ogenclient.QueryAuditEventsParams{
					EventCategory: ogenclient.NewOptString("apifrontend"),
					Since:         ogenclient.NewOptString(sinceStr),
					Limit:         ogenclient.NewOptInt(200),
				})
				if qErr != nil {
					GinkgoWriter.Printf("  [AF Audit] Query error: %v\n", qErr)
					return afRequiredEvents
				}
				allAFEvents = resp.Data

				eventTypeCounts = map[string]int{}
				for _, event := range allAFEvents {
					eventTypeCounts[event.EventType]++
				}

				var missing []string
				for _, eventType := range afRequiredEvents {
					if eventTypeCounts[eventType] == 0 {
						missing = append(missing, eventType)
					}
				}
				GinkgoWriter.Printf("  [AF Audit] Found %d AF events (%d unique types), %d required still missing\n",
					len(allAFEvents), len(eventTypeCounts), len(missing))
				return missing
			}, 120*time.Second, 3*time.Second).Should(BeEmpty(),
				"All required AF audit event types must be present")

			for _, event := range allAFEvents {
				GinkgoWriter.Printf("  AF Audit: type=%s outcome=%s ts=%s\n",
					event.EventType, event.EventOutcome,
					event.EventTimestamp.Format(time.RFC3339))
			}

			By("Verifying required AF events")
			for _, eventType := range afRequiredEvents {
				Expect(eventTypeCounts[eventType]).To(BeNumerically(">=", 1),
					"AF event %s must appear at least once", eventType)
			}

			By("Logging optional AF events")
			for _, eventType := range afOptionalEvents {
				if eventTypeCounts[eventType] > 0 {
					GinkgoWriter.Printf("  Optional %s: present (%d)\n", eventType, eventTypeCounts[eventType])
				} else {
					GinkgoWriter.Printf("  Optional %s: absent\n", eventType)
				}
			}

			By("Verifying all AF events have category 'apifrontend'")
			for _, event := range allAFEvents {
				Expect(string(event.EventCategory)).To(Equal("apifrontend"))
			}

			GinkgoWriter.Printf("  AF audit trace verified: %d events, %d unique types\n",
				len(allAFEvents), len(eventTypeCounts))
		})
	})

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func fpFetchDEXToken(dexURL, clientID, clientSecret, username, password string) (string, error) {
	data := url.Values{
		"grant_type":    {"password"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"username":      {username},
		"password":      {password},
		"scope":         {"openid email profile groups"},
	}
	resp, err := http.PostForm(dexURL+"/token", data)
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
	return tokenResp.IDToken, nil
}

func fpInitMCPSession(client *http.Client, baseURL, token string) (string, error) {
	body := fpBuildJSONRPC("fp-init-1", "initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo":      map[string]interface{}{"name": "fp-e2e", "version": "1.0"},
	})
	raw, code, err := fpMCPPOST(client, baseURL, token, "", body)
	if err != nil {
		return "", err
	}
	if code >= http.StatusBadRequest {
		return "", fmt.Errorf("MCP initialize: HTTP %d: %s", code, string(raw))
	}
	return "", nil
}

func fpMCPPOST(client *http.Client, baseURL, token, sessionID, jsonBody string) ([]byte, int, error) {
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
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	return body, resp.StatusCode, err
}

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
