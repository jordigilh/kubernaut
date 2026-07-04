package ka_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
)

var _ = Describe("SDKMCPClient", func() {
	var (
		ts     *httptest.Server
		client *ka.SDKMCPClient
	)

	AfterEach(func() {
		if ts != nil {
			ts.Close()
		}
	})

	type toolDef struct {
		name    string
		handler func(ctx context.Context, req *mcp.CallToolRequest, extra any) (*mcp.CallToolResult, any, error)
	}

	buildTestServer := func(tools ...toolDef) *httptest.Server {
		server := mcp.NewServer(&mcp.Implementation{
			Name:    "ka-mock",
			Version: "test",
		}, nil)

		for _, td := range tools {
			mcp.AddTool(server, &mcp.Tool{
				Name:        td.name,
				Description: td.name,
			}, td.handler)
		}

		handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
			return server
		}, nil)

		mux := http.NewServeMux()
		mux.Handle("/mcp", fakeAuthMiddleware(handler))
		mux.Handle("/mcp/", fakeAuthMiddleware(handler))
		return httptest.NewServer(mux)
	}

	Describe("SelectWorkflow", func() {
		It("returns workflow result on success", func() {
			ts = buildTestServer(toolDef{
				name: "kubernaut_select_workflow",
				handler: func(_ context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
					resp := map[string]string{
						"status":  "accepted",
						"message": "workflow wf-001 selected",
					}
					data, _ := json.Marshal(resp)
					return &mcp.CallToolResult{
						Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
					}, nil, nil
				},
			})

			httpClient := &http.Client{Transport: &authedRoundTripper{user: "alice@example.com"}}
			client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, nil, logr.Discard())

			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "alice@example.com",
				RawToken: "token-for-alice@example.com",
			})

			result, err := client.SelectWorkflow(ctx, ka.SelectWorkflowArgs{
				RRID:       "rr-test-001",
				WorkflowID: "wf-001",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Status).To(Equal("accepted"))
			Expect(result.Message).To(ContainSubstring("wf-001"))
		})

		It("forwards the user JWT in the Authorization header (QE-6)", func() {
			var capturedAuth string
			server := mcp.NewServer(&mcp.Implementation{
				Name:    "ka-mock",
				Version: "test",
			}, nil)
			mcp.AddTool(server, &mcp.Tool{
				Name:        "kubernaut_select_workflow",
				Description: "Select a workflow",
			}, func(_ context.Context, _ *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: `{"status":"ok","message":"done"}`}},
				}, nil, nil
			})

			handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
				return server
			}, nil)
			mux := http.NewServeMux()
			mux.Handle("/mcp", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedAuth = r.Header.Get("Authorization")
				if capturedAuth == "" || !strings.HasPrefix(capturedAuth, "Bearer ") {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}
				handler.ServeHTTP(w, r)
			}))
			mux.Handle("/mcp/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedAuth = r.Header.Get("Authorization")
				if capturedAuth == "" || !strings.HasPrefix(capturedAuth, "Bearer ") {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}
				handler.ServeHTTP(w, r)
			}))
			ts = httptest.NewServer(mux)

			httpClient := &http.Client{Transport: &auth.ContextJWTDelegationTransport{}}
			client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, nil, logr.Discard())

			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "bob@example.com",
				RawToken: "my-secret-jwt-for-bob",
			})

			_, err := client.SelectWorkflow(ctx, ka.SelectWorkflowArgs{
				RRID:       "rr-test-003",
				WorkflowID: "wf-003",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(capturedAuth).To(Equal("Bearer my-secret-jwt-for-bob"))
		})

		It("returns error when auth fails", func() {
			ts = buildTestServer(toolDef{
				name: "kubernaut_select_workflow",
				handler: func(_ context.Context, _ *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
					return &mcp.CallToolResult{
						Content: []mcp.Content{&mcp.TextContent{Text: "{}"}},
					}, nil, nil
				},
			})

			httpClient := &http.Client{Transport: &authedRoundTripper{user: ""}}
			client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, nil, logr.Discard())

			_, err := client.SelectWorkflow(context.Background(), ka.SelectWorkflowArgs{
				RRID:       "rr-test-002",
				WorkflowID: "wf-002",
			})
			Expect(err).To(HaveOccurred())
		})
	})
	Describe("DiscoverWorkflows", func() {
		It("UT-AF-WP-019: calls kubernaut_investigate with discover_workflows action", func() {
			var calledTool string
			var receivedArgs map[string]any
			ts = buildTestServer(toolDef{
				name: "kubernaut_investigate",
				handler: func(_ context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
					calledTool = req.Params.Name
					_ = json.Unmarshal(req.Params.Arguments, &receivedArgs)
					resp := `{"workflows":[]}`
					return &mcp.CallToolResult{
						Content: []mcp.Content{&mcp.TextContent{Text: resp}},
					}, nil, nil
				},
			})

			httpClient := &http.Client{Transport: &authedRoundTripper{user: "alice@example.com"}}
			client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, nil, logr.Discard())

			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "alice@example.com",
				RawToken: "token-for-alice@example.com",
			})

			_, err := client.DiscoverWorkflows(ctx, ka.DiscoverWorkflowsArgs{RRID: "rr-test"})
			Expect(err).NotTo(HaveOccurred())
			Expect(calledTool).To(Equal("kubernaut_investigate"))
			Expect(receivedArgs).To(HaveKeyWithValue("action", "discover_workflows"))
			Expect(receivedArgs).To(HaveKeyWithValue("rr_id", "rr-test"))
		})

		It("UT-AF-WP-020: passes rr_id in args", func() {
			var receivedArgs map[string]any
			ts = buildTestServer(toolDef{
				name: "kubernaut_investigate",
				handler: func(_ context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
					_ = json.Unmarshal(req.Params.Arguments, &receivedArgs)
					resp := `{"workflows":[{"workflow_id":"wf-scale","name":"Scale"}]}`
					return &mcp.CallToolResult{
						Content: []mcp.Content{&mcp.TextContent{Text: resp}},
					}, nil, nil
				},
			})

			httpClient := &http.Client{Transport: &authedRoundTripper{user: "alice@example.com"}}
			client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, nil, logr.Discard())

			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "alice@example.com",
				RawToken: "token-for-alice@example.com",
			})

			_, err := client.DiscoverWorkflows(ctx, ka.DiscoverWorkflowsArgs{RRID: "rr-123", WorkflowID: "wf-scale"})
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedArgs).To(HaveKeyWithValue("rr_id", "rr-123"))
			Expect(receivedArgs).To(HaveKeyWithValue("action", "discover_workflows"))
		})

		It("UT-AF-WP-021: unmarshals KA JSON response", func() {
			ts = buildTestServer(toolDef{
				name: "kubernaut_investigate",
				handler: func(_ context.Context, _ *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
					resp := `{"workflows":[{"workflow_id":"wf-1","name":"Restart","description":"Restart pod","parameters":[{"name":"ns","type":"string","required":true}]}]}`
					return &mcp.CallToolResult{
						Content: []mcp.Content{&mcp.TextContent{Text: resp}},
					}, nil, nil
				},
			})

			httpClient := &http.Client{Transport: &authedRoundTripper{user: "alice@example.com"}}
			client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, nil, logr.Discard())

			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "alice@example.com",
				RawToken: "token-for-alice@example.com",
			})

			result, err := client.DiscoverWorkflows(ctx, ka.DiscoverWorkflowsArgs{RRID: "rr-test"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Workflows).To(HaveLen(1))
			Expect(result.Workflows[0].Parameters).To(HaveLen(1))
			Expect(result.Workflows[0].Parameters[0].Name).To(Equal("ns"))
		})

		It("UT-AF-WP-022: handles IsError response", func() {
			ts = buildTestServer(toolDef{
				name: "kubernaut_investigate",
				handler: func(_ context.Context, _ *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
					return &mcp.CallToolResult{
						IsError: true,
						Content: []mcp.Content{&mcp.TextContent{Text: "internal server error: db connection lost"}},
					}, nil, nil
				},
			})

			httpClient := &http.Client{Transport: &authedRoundTripper{user: "alice@example.com"}}
			client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, nil, logr.Discard())

			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "alice@example.com",
				RawToken: "token-for-alice@example.com",
			})

			_, err := client.DiscoverWorkflows(ctx, ka.DiscoverWorkflowsArgs{RRID: "rr-test"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("kubernaut agent"))
		})

		It("UT-AF-WP-023: handles empty content", func() {
			ts = buildTestServer(toolDef{
				name: "kubernaut_investigate",
				handler: func(_ context.Context, _ *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
					return &mcp.CallToolResult{
						Content: []mcp.Content{},
					}, nil, nil
				},
			})

			httpClient := &http.Client{Transport: &authedRoundTripper{user: "alice@example.com"}}
			client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, nil, logr.Discard())

			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "alice@example.com",
				RawToken: "token-for-alice@example.com",
			})

			result, err := client.DiscoverWorkflows(ctx, ka.DiscoverWorkflowsArgs{RRID: "rr-test"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
		})

		It("UT-AF-WP-024: SelectWorkflow includes parameters when non-nil", func() {
			var receivedArgs map[string]any
			ts = buildTestServer(toolDef{
				name: "kubernaut_select_workflow",
				handler: func(_ context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
					_ = json.Unmarshal(req.Params.Arguments, &receivedArgs)
					resp := `{"status":"accepted","message":"ok"}`
					return &mcp.CallToolResult{
						Content: []mcp.Content{&mcp.TextContent{Text: resp}},
					}, nil, nil
				},
			})

			httpClient := &http.Client{Transport: &authedRoundTripper{user: "alice@example.com"}}
			client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, nil, logr.Discard())

			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "alice@example.com",
				RawToken: "token-for-alice@example.com",
			})

			_, err := client.SelectWorkflow(ctx, ka.SelectWorkflowArgs{
				RRID:       "rr-1",
				WorkflowID: "wf-1",
				Parameters: map[string]any{"namespace": "prod", "replicas": 3},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedArgs).To(HaveKey("parameters"))
		})

		It("UT-AF-WP-025: SelectWorkflow omits parameters when nil (backward compat)", func() {
			var receivedArgs map[string]any
			ts = buildTestServer(toolDef{
				name: "kubernaut_select_workflow",
				handler: func(_ context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
					_ = json.Unmarshal(req.Params.Arguments, &receivedArgs)
					resp := `{"status":"accepted","message":"ok"}`
					return &mcp.CallToolResult{
						Content: []mcp.Content{&mcp.TextContent{Text: resp}},
					}, nil, nil
				},
			})

			httpClient := &http.Client{Transport: &authedRoundTripper{user: "alice@example.com"}}
			client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, nil, logr.Discard())

			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "alice@example.com",
				RawToken: "token-for-alice@example.com",
			})

			_, err := client.SelectWorkflow(ctx, ka.SelectWorkflowArgs{
				RRID:       "rr-1",
				WorkflowID: "wf-1",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedArgs).NotTo(HaveKey("parameters"))
		})
	})

	Describe("Investigate", func() {
		It("returns investigation result on success (QE-11)", func() {
			ts = buildTestServer(toolDef{
				name: "kubernaut_investigate",
				handler: func(_ context.Context, _ *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
					resp := map[string]string{
						"status":  "complete",
						"summary": "pod crashlooping due to OOM",
					}
					data, _ := json.Marshal(resp)
					return &mcp.CallToolResult{
						Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
					}, nil, nil
				},
			})

			httpClient := &http.Client{Transport: &authedRoundTripper{user: "alice@example.com"}}
			client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, nil, logr.Discard())

			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "alice@example.com",
				RawToken: "token-for-alice@example.com",
			})

			result, err := client.Investigate(ctx, ka.InvestigateArgs{
				Namespace: "prod",
				Kind:      "Deployment",
				Name:      "web-api",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Status).To(Equal("complete"))
			Expect(result.Summary).To(ContainSubstring("OOM"))
		})

		It("returns error on tool failure", func() {
			ts = buildTestServer(toolDef{
				name: "kubernaut_investigate",
				handler: func(_ context.Context, _ *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
					return &mcp.CallToolResult{
						IsError: true,
						Content: []mcp.Content{&mcp.TextContent{Text: "investigation failed: resource not found"}},
					}, nil, nil
				},
			})

			httpClient := &http.Client{Transport: &authedRoundTripper{user: "bob@example.com"}}
			client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, nil, logr.Discard())

			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "bob@example.com",
				RawToken: "token-for-bob@example.com",
			})

			_, err := client.Investigate(ctx, ka.InvestigateArgs{
				Namespace: "default",
				Kind:      "Pod",
				Name:      "missing",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("kubernaut agent"))
		})
	})

	Describe("Trusted Intermediary Identity Injection (#1287)", func() {
		It("UT-AF-1287-009: SelectWorkflow includes acting_user in args (AU-3)", func() {
			var capturedArgs map[string]any
			ts = buildTestServer(toolDef{
				name: "kubernaut_select_workflow",
				handler: func(_ context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
					_ = json.Unmarshal(req.Params.Arguments, &capturedArgs)
					resp := map[string]string{"status": "accepted", "message": "ok"}
					data, _ := json.Marshal(resp)
					return &mcp.CallToolResult{
						Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
					}, nil, nil
				},
			})

			httpClient := &http.Client{Transport: &authedRoundTripper{user: "alice@example.com"}}
			client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, nil, logr.Discard())

			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "alice@example.com",
				Groups:   []string{"sre", "admin"},
				RawToken: "token-for-alice@example.com",
			})

			_, err := client.SelectWorkflow(ctx, ka.SelectWorkflowArgs{
				RRID:       "rr-1287-sw",
				WorkflowID: "wf-001",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(capturedArgs).To(HaveKeyWithValue("acting_user", "alice@example.com"))
			Expect(capturedArgs).To(HaveKey("acting_user_groups"))
		})

		It("UT-AF-1287-010: DiscoverWorkflows includes acting_user in args (AU-3)", func() {
			var capturedArgs map[string]any
			ts = buildTestServer(toolDef{
				name: "kubernaut_investigate",
				handler: func(_ context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
					_ = json.Unmarshal(req.Params.Arguments, &capturedArgs)
					resp := map[string]any{"workflows": []any{}, "count": 0}
					data, _ := json.Marshal(resp)
					return &mcp.CallToolResult{
						Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
					}, nil, nil
				},
			})

			httpClient := &http.Client{Transport: &authedRoundTripper{user: "bob@example.com"}}
			client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, nil, logr.Discard())

			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "bob@example.com",
				Groups:   []string{"engineering"},
				RawToken: "token-for-bob@example.com",
			})

			_, err := client.DiscoverWorkflows(ctx, ka.DiscoverWorkflowsArgs{
				RRID: "rr-1287-dw",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(capturedArgs).To(HaveKeyWithValue("acting_user", "bob@example.com"))
			Expect(capturedArgs).To(HaveKey("acting_user_groups"))
		})
	})
})

// =============================================================================
// CHAR-AF-1532: Characterization tests for SDKMCPClient.StartInvestigation,
// pinning current behavior before complexity-driven refactor (#1532 Wave A).
// =============================================================================
var _ = Describe("SDKMCPClient.StartInvestigation (CHAR-AF-1532)", func() {
	var (
		ts     *httptest.Server
		client *ka.SDKMCPClient
	)

	AfterEach(func() {
		if ts != nil {
			ts.Close()
		}
	})

	buildInvestigateServer := func(handler func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, any, error)) *httptest.Server {
		server := mcp.NewServer(&mcp.Implementation{Name: "ka-mock", Version: "test"}, nil)
		mcp.AddTool(server, &mcp.Tool{
			Name:        "kubernaut_investigate",
			Description: "kubernaut_investigate",
		}, handler)
		httpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
			return server
		}, nil)
		mux := http.NewServeMux()
		mux.Handle("/mcp", fakeAuthMiddleware(httpHandler))
		mux.Handle("/mcp/", fakeAuthMiddleware(httpHandler))
		return httptest.NewServer(mux)
	}

	It("CHAR-AF-1532-010: returns error when no user identity in context", func() {
		ts = buildInvestigateServer(func(_ context.Context, _ *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: `{}`}}}, nil, nil
		})
		httpClient := &http.Client{Transport: &authedRoundTripper{user: "alice@example.com"}}
		client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, nil, logr.Discard())

		_, err := client.StartInvestigation(context.Background(), ka.StartInvestigationArgs{RRID: "rr-noident"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("user identity required"))
	})

	It("CHAR-AF-1532-011: happy path returns session_id/status, Events channel and Closer", func() {
		var capturedArgs map[string]any
		ts = buildInvestigateServer(func(_ context.Context, req *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
			_ = json.Unmarshal(req.Params.Arguments, &capturedArgs)
			resp := `{"session_id":"sess-011","status":"started"}`
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: resp}}}, nil, nil
		})
		httpClient := &http.Client{Transport: &authedRoundTripper{user: "alice@example.com"}}
		client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, nil, logr.Discard())

		ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
			Username: "alice@example.com",
			Groups:   []string{"sre"},
			RawToken: "token-for-alice@example.com",
		})

		result, err := client.StartInvestigation(ctx, ka.StartInvestigationArgs{RRID: "rr-011"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		Expect(result.SessionID).To(Equal("sess-011"))
		Expect(result.Status).To(Equal("started"))
		Expect(result.Events).NotTo(BeNil())
		Expect(result.Closer).NotTo(BeNil())
		Expect(result.Session).NotTo(BeNil())

		Expect(capturedArgs).To(HaveKeyWithValue("rr_id", "rr-011"))
		Expect(capturedArgs).To(HaveKeyWithValue("action", "start"))
		Expect(capturedArgs).To(HaveKeyWithValue("acting_user", "alice@example.com"))
		Expect(capturedArgs).NotTo(HaveKey("session_id"), "session_id omitted from args when not provided")

		result.Closer()
	})

	It("CHAR-AF-1532-012: passes session_id through to args when provided (#1452)", func() {
		var capturedArgs map[string]any
		ts = buildInvestigateServer(func(_ context.Context, req *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
			_ = json.Unmarshal(req.Params.Arguments, &capturedArgs)
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: `{"session_id":"sess-012","status":"reconnected"}`}}}, nil, nil
		})
		httpClient := &http.Client{Transport: &authedRoundTripper{user: "alice@example.com"}}
		client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, nil, logr.Discard())

		ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{Username: "alice@example.com", RawToken: "token-for-alice@example.com"})
		result, err := client.StartInvestigation(ctx, ka.StartInvestigationArgs{RRID: "rr-012", SessionID: "existing-session"})
		Expect(err).NotTo(HaveOccurred())
		Expect(capturedArgs).To(HaveKeyWithValue("session_id", "existing-session"))
		result.Closer()
	})

	It("CHAR-AF-1532-013: returns error when the MCP tool call reports IsError", func() {
		ts = buildInvestigateServer(func(_ context.Context, _ *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "lease already held by another session"}},
			}, nil, nil
		})
		httpClient := &http.Client{Transport: &authedRoundTripper{user: "alice@example.com"}}
		client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, nil, logr.Discard())

		ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{Username: "alice@example.com", RawToken: "token-for-alice@example.com"})
		_, err := client.StartInvestigation(ctx, ka.StartInvestigationArgs{RRID: "rr-013"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("lease already held"))
	})

	It("CHAR-AF-1532-014: streams a structured LoggingMessage event from KA to the Events channel", func() {
		ts = buildInvestigateServer(func(ctx context.Context, req *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
			_ = req.Session.Log(ctx, &mcp.LoggingMessageParams{
				Level: "info",
				Data:  json.RawMessage(`{"type":"rca_progress","turn":1,"phase":"investigating"}`),
			})
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: `{"session_id":"sess-014","status":"started"}`}}}, nil, nil
		})
		httpClient := &http.Client{Transport: &authedRoundTripper{user: "alice@example.com"}}
		client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, nil, logr.Discard())

		ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{Username: "alice@example.com", RawToken: "token-for-alice@example.com"})
		result, err := client.StartInvestigation(ctx, ka.StartInvestigationArgs{RRID: "rr-014"})
		Expect(err).NotTo(HaveOccurred())
		defer result.Closer()

		Eventually(result.Events).Should(Receive(Equal(ka.InvestigationEvent{Type: "rca_progress", Turn: 1, Phase: "investigating"})))
	})

	It("CHAR-AF-1532-015: Closer is idempotent (safe to call multiple times)", func() {
		ts = buildInvestigateServer(func(_ context.Context, _ *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: `{"session_id":"sess-015","status":"started"}`}}}, nil, nil
		})
		httpClient := &http.Client{Transport: &authedRoundTripper{user: "alice@example.com"}}
		client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, nil, logr.Discard())

		ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{Username: "alice@example.com", RawToken: "token-for-alice@example.com"})
		result, err := client.StartInvestigation(ctx, ka.StartInvestigationArgs{RRID: "rr-015"})
		Expect(err).NotTo(HaveOccurred())

		Expect(func() {
			result.Closer()
			result.Closer()
		}).NotTo(Panic())
	})
})

func fakeAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer token-for-") {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type authedRoundTripper struct {
	user string
	base http.RoundTripper
}

func (t *authedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.user != "" {
		req = req.Clone(req.Context())
		req.Header.Set("Authorization", "Bearer token-for-"+t.user)
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

// logEntry captures a single log call for assertion.
type logEntry struct {
	prefix string
	args   string
}

// capturingLogger returns a funcr.Logger that stores every log line at any
// verbosity in the returned slice (thread-safe). Error-level calls are
// stored with prefix "ERROR".
func capturingLogger() (logr.Logger, *[]logEntry) {
	var mu sync.Mutex
	entries := &[]logEntry{}
	logger := funcr.New(func(prefix, args string) {
		mu.Lock()
		defer mu.Unlock()
		*entries = append(*entries, logEntry{prefix: prefix, args: args})
	}, funcr.Options{Verbosity: 10})
	return logger, entries
}

var _ = Describe("ParseLoggingEvent [AU-6]", func() {

	Describe("UT-AF-LOG-001: Non-structured messages log at V(2), not Error", func() {
		It("should return false and log at debug level for plain-text KA messages", func() {
			logger, entries := capturingLogger()
			raw := json.RawMessage([]byte(`"this is a plain text log from KA session"`))

			evt, ok := ka.ParseLoggingEvent(logger, raw)
			Expect(ok).To(BeFalse(), "non-structured messages must return false")
			Expect(evt).To(Equal(ka.InvestigationEvent{}))

			Expect(*entries).To(HaveLen(1))
			Expect((*entries)[0].args).To(ContainSubstring("non-structured logging message"))
			Expect((*entries)[0].args).NotTo(ContainSubstring("ERROR"),
				"AU-6: non-structured messages must not emit Error-level logs")
		})
	})

	Describe("UT-AF-LOG-002: Valid structured events are parsed successfully", func() {
		It("should return the event and true for well-formed InvestigationEvent JSON", func() {
			logger, entries := capturingLogger()
			raw := json.RawMessage([]byte(`{"type":"rca_progress","turn":3,"phase":"investigating"}`))

			evt, ok := ka.ParseLoggingEvent(logger, raw)
			Expect(ok).To(BeTrue())
			Expect(evt.Type).To(Equal("rca_progress"))
			Expect(evt.Turn).To(Equal(3))
			Expect(evt.Phase).To(Equal("investigating"))

			Expect(*entries).To(BeEmpty(),
				"AU-6: valid events must not produce any log output")
		})
	})

	Describe("UT-AF-LOG-003: Malformed JSON logs at V(2), not Error", func() {
		It("should return false and log at debug level for invalid JSON", func() {
			logger, entries := capturingLogger()
			raw := json.RawMessage([]byte(`{not valid json`))

			evt, ok := ka.ParseLoggingEvent(logger, raw)
			Expect(ok).To(BeFalse())
			Expect(evt).To(Equal(ka.InvestigationEvent{}))

			Expect(*entries).To(HaveLen(1))
			Expect((*entries)[0].args).To(ContainSubstring("parse_error"))
			Expect((*entries)[0].args).NotTo(ContainSubstring("ERROR"),
				"AU-6: malformed JSON must not emit Error-level logs")
		})
	})
})
