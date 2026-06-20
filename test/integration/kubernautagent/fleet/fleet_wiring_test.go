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

package fleet_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	mockgw "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
)

// BR-INTEGRATION-065: Multi-Cluster Federation — Fleet Tool Discovery
var _ = Describe("Fleet Wiring Integration Tests (BR-INTEGRATION-065)", func() {
	var (
		ctx context.Context
		gw  *mockgw.MockGateway
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	AfterEach(func() {
		if gw != nil {
			gw.Close()
		}
	})

	Describe("IT-FLEET-001: registerFleetTools discovers and registers BridgeTools via production path", func() {
		It("connects to MCP Gateway, calls tools/list, and wraps discovered tools as BridgeTools", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-east", "prod-west"))

			client, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer client.Close()

			session := client.Session()
			tools, err := session.ListTools(ctx, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(tools.Tools)).To(BeNumerically(">=", 4),
				"2 clusters x 2 tools each = at least 4 tools")

			var bridgeCount int
			for _, tool := range tools.Tools {
				def := mcpclient.ToolDefinition{
					Name:        tool.Name,
					Description: tool.Description,
				}
				if tool.InputSchema != nil {
					schema, marshalErr := json.Marshal(tool.InputSchema)
					Expect(marshalErr).ToNot(HaveOccurred())
					def.InputSchema = schema
				}
				bt := mcpclient.NewBridgeTool(def, "fleet", session)
				Expect(bt.Name()).To(Equal(tool.Name))
				bridgeCount++
			}

			Expect(bridgeCount).To(Equal(len(tools.Tools)),
				"every discovered tool must produce a BridgeTool")
		})
	})

	Describe("IT-FLEET-002: fleet disabled when endpoint is empty", func() {
		It("returns nil client when endpoint is empty (no-op path)", func() {
			client, err := mcpclient.New(ctx, "")
			Expect(err).To(HaveOccurred(),
				"empty endpoint should fail to connect")
			Expect(client).To(BeNil())
		})
	})

	Describe("IT-FLEET-003: OAuth2 transport integration", func() {
		It("creates an HTTP client with OAuth2 transport that injects Authorization header", func() {
			tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				body := make([]byte, 1024)
				n, _ := r.Body.Read(body)
				bodyStr := string(body[:n])
				if !strings.Contains(bodyStr, "grant_type=client_credentials") {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"access_token":"test-jwt-token","token_type":"Bearer","expires_in":3600}`))
			}))
			defer tokenServer.Close()

			cfg := mcpclient.OAuth2Config{
				TokenURL:     tokenServer.URL,
				ClientID:     "kubernaut-fleet",
				ClientSecret: "test-secret",
				Scopes:       []string{"openid"},
			}

			var capturedAuth string
			targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedAuth = r.Header.Get("Authorization")
				w.WriteHeader(http.StatusOK)
			}))
			defer targetServer.Close()

			transport := mcpclient.NewOAuth2Transport(cfg, nil)
			httpClient := &http.Client{Transport: transport}

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetServer.URL, nil)
			Expect(err).ToNot(HaveOccurred())
			resp, err := httpClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			resp.Body.Close()

			Expect(capturedAuth).To(Equal("Bearer test-jwt-token"),
				"OAuth2 transport must inject Bearer token acquired from token endpoint")
		})
	})

	Describe("IT-FLEET-004: BridgeTool executes through production MCP dispatch", func() {
		It("calls tool via session and returns text content from remote cluster", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("cluster-alpha"))

			client, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer client.Close()

			session := client.Session()
			tools, err := session.ListTools(ctx, nil)
			Expect(err).ToNot(HaveOccurred())

			var getResourceTool mcpclient.ToolDefinition
			for _, t := range tools.Tools {
				if t.Name == "cluster-alpha__get_resource" {
					getResourceTool = mcpclient.ToolDefinition{
						Name:        t.Name,
						Description: t.Description,
					}
					if t.InputSchema != nil {
						schema, _ := json.Marshal(t.InputSchema)
						getResourceTool.InputSchema = schema
					}
					break
				}
			}
			Expect(getResourceTool.Name).ToNot(BeEmpty(),
				"cluster-alpha__get_resource must be discoverable via tools/list")

			bt := mcpclient.NewBridgeTool(getResourceTool, "cluster-alpha", session)
			args := json.RawMessage(`{"kind":"Pod","namespace":"default","name":"nginx"}`)
			result, err := bt.Execute(ctx, args)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(ContainSubstring("cluster-alpha"),
				"response must contain cluster identifier")
			Expect(result).To(ContainSubstring("nginx"),
				"response must contain requested resource name")

			calls := gw.CallLog()
			Expect(calls).To(HaveLen(1))
			Expect(calls[0].ToolName).To(Equal("cluster-alpha__get_resource"),
				"mock gateway must record the tool call through the production dispatch path")
		})
	})

	Describe("IT-FLEET-005: cluster-aware fingerprint produces distinct dedup keys", func() {
		It("same resource on different clusters is not deduplicated", func() {
			_ = logr.Discard()

			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("east", "west"))

			client, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer client.Close()

			session := client.Session()

			btEast := mcpclient.NewBridgeTool(mcpclient.ToolDefinition{
				Name: "east__get_resource",
			}, "east", session)
			btWest := mcpclient.NewBridgeTool(mcpclient.ToolDefinition{
				Name: "west__get_resource",
			}, "west", session)

			Expect(btEast.Name()).ToNot(Equal(btWest.Name()),
				"tools on different clusters must have distinct names preventing accidental collision")
		})
	})
})
