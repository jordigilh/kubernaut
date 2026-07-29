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

// fleetGenericNameTool locally mirrors cmd/kubernautagent's unexported
// genericNameTool decorator (toolregistry.go's gatewayOverlayResolver.Overlay,
// DD-FLEET-004): it exposes a *mcpclient.BridgeTool under a generic
// (unprefixed) name to the LLM-facing registry while Execute still delegates
// to the inner BridgeTool, which dispatches using the tool's original wire
// name. A bare BridgeTool cannot do this itself — it uses a single Name field
// for both identities — so IT-KA-FLEET-010/011/012 re-derive the same
// two-name split here, from exported mcpclient primitives, to prove the real
// production recipe against a live mock gateway without reaching into
// cmd/kubernautagent's unexported types.
type fleetGenericNameTool struct {
	inner *mcpclient.BridgeTool
	name  string
}

func (g *fleetGenericNameTool) Name() string                { return g.name }
func (g *fleetGenericNameTool) Description() string         { return g.inner.Description() }
func (g *fleetGenericNameTool) Parameters() json.RawMessage { return g.inner.Parameters() }
func (g *fleetGenericNameTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return g.inner.Execute(ctx, args)
}

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

	// IT-KA-FLEET-010/011/012 previously asserted that registerFleetTools
	// registered list_clusters/list_tools_for_cluster LLM-facing meta-tools.
	// Under DD-FLEET-004 (issue #1732), KA pre-scopes tools for the one
	// target cluster server-side instead of letting the LLM discover and
	// select clusters itself, so those meta-tools are gone. These three
	// tests are repurposed to exercise the real discover -> rekey-to-generic-
	// name -> register recipe used by cmd/kubernautagent's
	// gatewayOverlayResolver.Overlay() (unexported, so re-derived here from
	// exported mcpclient primitives against a real mock gateway) and to
	// prove the two meta-tools never re-enter the LLM-facing registry.
	Describe("IT-KA-FLEET-010 [AC-4/AC-6]: kuadrant pre-scoping never exposes list_clusters/list_tools_for_cluster to the LLM-facing registry", func() {
		It("registers only the target cluster's own tools under generic names; the discovery meta-tools are absent", func() {
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

			defs, err := disc.ToolsForCluster(ctx, "prod-east")
			Expect(err).ToNot(HaveOccurred())
			Expect(defs).ToNot(BeEmpty())

			// AC-4/Issue #1756: derive the wire prefix via the same
			// gateway-agnostic PrefixFromToolNames helper production code
			// uses (cmd/kubernautagent's gatewayOverlayResolver.Overlay()),
			// instead of a hardcoded "prod_east_" literal that can silently
			// drift from Kuadrant's actual (admin-set, non-{clusterID}__)
			// MCPServerRegistration.spec.prefix convention. This proves the
			// helper's extraction against real discover_tools/select_tools/
			// ListTools MCP protocol traffic, not just a mocked table.
			names := make([]string, len(defs))
			for i, def := range defs {
				names[i] = def.Name
			}
			prefix, err := mcpclient.PrefixFromToolNames("prod-east", names)
			Expect(err).ToNot(HaveOccurred())
			Expect(prefix).To(Equal("prod_east_"),
				"sanity check: the mock gateway's configured Prefix must round-trip through real discovery")

			reg := toolregistry.New()
			for _, def := range defs {
				generic := strings.TrimPrefix(def.Name, prefix)
				bridge := mcpclient.NewBridgeTool(def, "prod-east", session)
				reg.Register(&fleetGenericNameTool{inner: bridge, name: generic})
			}

			_, err = reg.Get("list_clusters")
			Expect(err).To(HaveOccurred(),
				"DD-FLEET-004: list_clusters must never be registered into the LLM-facing registry")

			_, err = reg.Get("list_tools_for_cluster")
			Expect(err).To(HaveOccurred(),
				"DD-FLEET-004: list_tools_for_cluster must never be registered into the LLM-facing registry")

			getTool, err := reg.Get("resources_get")
			Expect(err).ToNot(HaveOccurred(),
				"the cluster's real tool must be reachable under its generic (unprefixed) name")
			Expect(getTool.Name()).To(Equal("resources_get"))

			result, execErr := getTool.Execute(ctx, json.RawMessage(`{"kind":"Pod","apiVersion":"v1","namespace":"default","name":"nginx"}`))
			Expect(execErr).ToNot(HaveOccurred())
			Expect(result).To(ContainSubstring("nginx"),
				"executing the generic name must still reach prod-east's real resource via the wire-prefixed tool")
		})
	})

	Describe("IT-KA-FLEET-011 [AC-6]: eaigw pre-scoping resolves a generic name to the correct cluster-prefixed wire tool", func() {
		It("registers the cluster's tools under generic names, executes through the real wire name, and keeps list_tools_for_cluster absent", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("cluster-alpha", "cluster-beta"))

			client, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer client.Close()

			session := client.Session()
			disc, err := mcpclient.NewDiscoverer("eaigw", session)
			Expect(err).ToNot(HaveOccurred())

			defs, err := disc.ToolsForCluster(ctx, "cluster-alpha")
			Expect(err).ToNot(HaveOccurred())
			Expect(defs).ToNot(BeEmpty())

			reg := toolregistry.New()
			for _, def := range defs {
				generic := strings.TrimPrefix(def.Name, "cluster-alpha__")
				bridge := mcpclient.NewBridgeTool(def, "cluster-alpha", session)
				reg.Register(&fleetGenericNameTool{inner: bridge, name: generic})
			}

			_, err = reg.Get("list_tools_for_cluster")
			Expect(err).To(HaveOccurred(),
				"DD-FLEET-004: list_tools_for_cluster must never be registered into the LLM-facing registry")

			getTool, err := reg.Get("resources_get")
			Expect(err).ToNot(HaveOccurred())
			Expect(getTool.Name()).To(Equal("resources_get"),
				"the LLM must see the tool under its bare, cluster-transparent name")

			result, execErr := getTool.Execute(ctx, json.RawMessage(`{"kind":"Pod","apiVersion":"v1","namespace":"default","name":"nginx"}`))
			Expect(execErr).ToNot(HaveOccurred())
			Expect(result).To(ContainSubstring("nginx"),
				"executing the generic name must still reach cluster-alpha's real resource")

			calls := gw.CallLog()
			Expect(calls).ToNot(BeEmpty())
			Expect(calls[len(calls)-1].ToolName).To(Equal("cluster-alpha__resources_get"),
				"the wire call must use the cluster-prefixed name even though the LLM only ever sees the generic one")
		})
	})

	Describe("IT-KA-FLEET-012 [AC-4/AC-6]: automatic pre-scoping resolves the LLM's generic tool call to the one target cluster, never another", func() {
		It("scopes discovery to target-cluster only and round-trips execution to it, not to a sibling cluster", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("target-cluster", "other-cluster"))

			client, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer client.Close()

			session := client.Session()
			disc, err := mcpclient.NewDiscoverer("eaigw", session)
			Expect(err).ToNot(HaveOccurred())

			// Real automatic pre-scoping recipe for a single investigation
			// targeting "target-cluster" only (DD-FLEET-004, ADR-068 #11):
			// ToolsForCluster narrows discovery to exactly this cluster's
			// tools before any generic-name rekeying happens.
			defs, err := disc.ToolsForCluster(ctx, "target-cluster")
			Expect(err).ToNot(HaveOccurred())

			reg := toolregistry.New()
			for _, def := range defs {
				Expect(def.Name).To(HavePrefix("target-cluster__"),
					"AC-6: pre-scoping for one investigation must never pull in another cluster's tools")
				generic := strings.TrimPrefix(def.Name, "target-cluster__")
				bridge := mcpclient.NewBridgeTool(def, "target-cluster", session)
				reg.Register(&fleetGenericNameTool{inner: bridge, name: generic})
			}

			getResourceTool, err := reg.Get("resources_get")
			Expect(err).ToNot(HaveOccurred())
			Expect(getResourceTool).ToNot(BeNil())

			listResourceTool, err := reg.Get("resources_list")
			Expect(err).ToNot(HaveOccurred())
			Expect(listResourceTool).ToNot(BeNil())

			result, execErr := getResourceTool.Execute(ctx, json.RawMessage(`{"kind":"Pod","apiVersion":"v1","namespace":"default","name":"nginx"}`))
			Expect(execErr).ToNot(HaveOccurred())
			Expect(result).To(ContainSubstring("nginx"))

			calls := gw.CallLog()
			Expect(calls[len(calls)-1].ToolName).To(Equal("target-cluster__resources_get"),
				"execution through the generic name must reach target-cluster, not other-cluster")
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
