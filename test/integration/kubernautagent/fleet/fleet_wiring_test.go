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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	gwtypes "github.com/jordigilh/kubernaut/pkg/gateway/types"
	toolregistry "github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
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
				if t.Name == "cluster-alpha__resources_get" {
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
				"cluster-alpha__resources_get must be discoverable via tools/list")

			bt := mcpclient.NewBridgeTool(getResourceTool, "cluster-alpha", session)
			args := json.RawMessage(`{"kind":"Pod","apiVersion":"v1","namespace":"default","name":"nginx"}`)
			result, err := bt.Execute(ctx, args)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(ContainSubstring("nginx"),
				"response must contain requested resource name")
			Expect(result).To(ContainSubstring(`"kind":"Pod"`),
				"response must contain resource kind from remote cluster")

			calls := gw.CallLog()
			Expect(calls).To(HaveLen(1))
			Expect(calls[0].ToolName).To(Equal("cluster-alpha__resources_get"),
				"mock gateway must record the tool call through the production dispatch path")
		})
	})

	Describe("IT-FLEET-FMC-001 [AC-4]: NewFromSession + WithClusterID creates working reader for FMC Writer", func() {
		It("creates a client.Reader from an existing session that can list resources through the MCP gateway", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("cluster-fmc"))

			parent, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer parent.Close()

			session := parent.Session()
			child := mcpclient.NewFromSession(session, "cluster-fmc")

			var reader client.Reader = child
			list := &unstructured.UnstructuredList{}
			list.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "PodList"})

			err = reader.List(ctx, list)
			Expect(err).ToNot(HaveOccurred())
			Expect(list.Items).ToNot(BeEmpty(),
				"NewFromSession client must return resources through the FMC Writer pipeline (AC-4: managed resources only)")

			calls := gw.CallLog()
			Expect(calls).ToNot(BeEmpty())
			Expect(calls[0].ToolName).To(Equal("cluster-fmc__resources_list"),
				"reader must route List calls through the correct cluster-prefixed MCP tool")
		})
	})

	Describe("UT-FLEET-BT-001 [SI-10]: BridgeTool auto-parses clusterID from tool name (Phase C)", func() {
		It("extracts clusterID from '{clusterID}__tool_name' convention without explicit parameter", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("auto-cluster"))

			parentClient, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer parentClient.Close()

			session := parentClient.Session()

			bt := mcpclient.NewBridgeToolFromSession(mcpclient.ToolDefinition{
				Name: "auto-cluster__resources_get",
			}, session)

			Expect(bt.ClusterID()).To(Equal("auto-cluster"),
				"BridgeTool must auto-parse clusterID from tool name prefix (SI-10: input validation)")
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
				Name: "east__resources_get",
			}, "east", session)
			btWest := mcpclient.NewBridgeTool(mcpclient.ToolDefinition{
				Name: "west__resources_get",
			}, "west", session)

			Expect(btEast.Name()).ToNot(Equal(btWest.Name()),
				"tools on different clusters must have distinct names preventing accidental collision")
		})
	})

	Describe("IT-FLEET-006 [AC-6]: Service scopes are operator-configurable, enabling least-privilege enforcement per deployment (BR-INTEGRATION-065)", func() {
		It("reads scopes from FleetOAuth2 config rather than hardcoding", func() {
			var capturedScope string
			tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = r.ParseForm()
				capturedScope = r.PostFormValue("scope")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"access_token":"fleet-token","token_type":"Bearer","expires_in":3600}`))
			}))
			defer tokenServer.Close()

			customScopes := []string{"openid", "groups", "fleet-admin"}
			scopes := mcpclient.DefaultFleetScopes(customScopes)
			Expect(scopes).To(Equal(customScopes), "explicit scopes must be passed through, not overridden")

			cfg := mcpclient.OAuth2Config{
				TokenURL:     tokenServer.URL,
				ClientID:     "kubernaut-ka",
				ClientSecret: "test-secret",
				Scopes:       customScopes,
			}

			transport := mcpclient.NewOAuth2Transport(cfg, nil)
			httpClient := &http.Client{Transport: transport}

			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer backend.Close()

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, backend.URL, nil)
			Expect(err).ToNot(HaveOccurred())
			resp, err := httpClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			resp.Body.Close()

			Expect(capturedScope).To(Equal("openid groups fleet-admin"),
				"token request must use operator-configured scopes, not hardcoded values")
		})
	})

	Describe("IT-FLEET-AUTH-REJECT-001 [AC-3]: Unauthorized callers are rejected at the gateway boundary and the client surfaces the denial (BR-INTEGRATION-065)", func() {
		It("surfaces 401 error with actionable message when token is invalid", func() {
			var tokenRequestCount int
			tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				tokenRequestCount++
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"access_token":"expired-token","token_type":"Bearer","expires_in":1}`))
			}))
			defer tokenServer.Close()

			rejectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				auth := r.Header.Get("Authorization")
				if auth == "Bearer expired-token" {
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = w.Write([]byte(`{"error":"invalid_token","error_description":"token is expired"}`))
					return
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer rejectServer.Close()

			cfg := mcpclient.OAuth2Config{
				TokenURL:     tokenServer.URL,
				ClientID:     "kubernaut-fleet-read",
				ClientSecret: "test-secret",
				Scopes:       []string{"openid", "groups"},
			}

			transport := mcpclient.NewOAuth2Transport(cfg, nil)
			httpClient := &http.Client{Transport: transport}

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, rejectServer.URL, nil)
			Expect(err).ToNot(HaveOccurred())
			resp, err := httpClient.Do(req)
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
				"gateway must reject calls with expired/invalid tokens (AC-3 enforcement)")
			resp.Body.Close()
		})
	})

	Describe("IT-KA-FLEET-010: registerFleetTools with gatewayType=kuadrant registers list_clusters tool", func() {
		It("creates GatewayDiscoverer and registers list_clusters + list_tools_for_cluster", func() {
			gw = mockgw.NewMockGateway(mockgw.WithDiscoverableTools(
				mockgw.DiscoverableClusterOption{
					Name:       "prod-east",
					Prefix:     "prod_east_",
					Categories: []string{"k8s"},
					Hint:       "Production cluster",
				},
			))

			client, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer client.Close()

			session := client.Session()
			disc, err := mcpclient.NewDiscoverer("kuadrant", session)
			Expect(err).ToNot(HaveOccurred())

			reg := toolregistry.New()
			listClustersTool := mcpclient.NewListClustersTool(disc)
			listToolsTool := mcpclient.NewListToolsForClusterTool(disc, reg, session)
			reg.Register(listClustersTool)
			reg.Register(listToolsTool)

			t, err := reg.Get("list_clusters")
			Expect(err).ToNot(HaveOccurred())
			Expect(t.Name()).To(Equal("list_clusters"))

			t2, err := reg.Get("list_tools_for_cluster")
			Expect(err).ToNot(HaveOccurred())
			Expect(t2.Name()).To(Equal("list_tools_for_cluster"))
		})
	})

	Describe("IT-KA-FLEET-011: registerFleetTools with gatewayType=eaigw registers list_tools_for_cluster tool", func() {
		It("creates EAIGWDiscoverer and registers discovery tools", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("cluster-alpha", "cluster-beta"))

			client, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer client.Close()

			session := client.Session()
			disc, err := mcpclient.NewDiscoverer("eaigw", session)
			Expect(err).ToNot(HaveOccurred())

			reg := toolregistry.New()
			listClustersTool := mcpclient.NewListClustersTool(disc)
			listToolsTool := mcpclient.NewListToolsForClusterTool(disc, reg, session)
			reg.Register(listClustersTool)
			reg.Register(listToolsTool)

			t, err := reg.Get("list_tools_for_cluster")
			Expect(err).ToNot(HaveOccurred())
			Expect(t.Name()).To(Equal("list_tools_for_cluster"))

			result, execErr := t.Execute(ctx, json.RawMessage(`{"cluster_id":"cluster-alpha"}`))
			Expect(execErr).ToNot(HaveOccurred())
			Expect(result).To(ContainSubstring("cluster-alpha__resources_get"))
		})
	})

	Describe("IT-KA-FLEET-012: registerFleetTools pre-scopes target cluster tools as BridgeTools", func() {
		It("discovered tools are registered in the registry as BridgeTools", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("target-cluster"))

			client, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer client.Close()

			session := client.Session()
			disc, err := mcpclient.NewDiscoverer("eaigw", session)
			Expect(err).ToNot(HaveOccurred())

			reg := toolregistry.New()
			listToolsTool := mcpclient.NewListToolsForClusterTool(disc, reg, session)
			reg.Register(listToolsTool)

			_, err = listToolsTool.Execute(ctx, json.RawMessage(`{"cluster_id":"target-cluster"}`))
			Expect(err).ToNot(HaveOccurred())

			getResourceTool, err := reg.Get("target-cluster__resources_get")
			Expect(err).ToNot(HaveOccurred())
			Expect(getResourceTool).ToNot(BeNil())

			listResourceTool, err := reg.Get("target-cluster__resources_list")
			Expect(err).ToNot(HaveOccurred())
			Expect(listResourceTool).ToNot(BeNil())
		})
	})

	Describe("UT-FLEET-FP-001 [CC4.2]: AF and GW fingerprints match for same resource (Phase D)", func() {
		It("produces identical dedup fingerprints using the shared helper, preventing audit trail inconsistency", func() {
			clusterID := "prod-east"
			resource := gwtypes.ResourceIdentifier{
				Namespace: "default",
				Kind:      "Deployment",
				Name:      "nginx",
			}

			gwFingerprint := gwtypes.CalculateClusterAwareFingerprint(clusterID, resource)

			afFingerprint := gwtypes.CalculateClusterAwareFingerprint(clusterID, resource)

			Expect(gwFingerprint).To(Equal(afFingerprint),
				"GW and AF must produce identical fingerprints for the same resource (CC4.2: change tracking consistency)")

			localGW := gwtypes.CalculateClusterAwareFingerprint("", resource)
			localAF := gwtypes.CalculateClusterAwareFingerprint("", resource)
			Expect(localGW).To(Equal(localAF),
				"local cluster fingerprints must also be identical")

			Expect(gwFingerprint).ToNot(Equal(localGW),
				"cluster-aware fingerprint must differ from local fingerprint")
		})
	})
})
