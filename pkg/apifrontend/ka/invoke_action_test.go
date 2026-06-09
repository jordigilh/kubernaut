package ka_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
)

var _ = Describe("InvokeAction (G3: Args + Identity)", func() {
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

	withIdentityCtx := func(username string, groups []string) context.Context {
		return auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
			Username: username,
			Groups:   groups,
			RawToken: "token-for-" + username,
		})
	}

	Describe("Wire format", func() {
		It("UT-AF-1234-001: InvokeAction sends {rr_id, action: start} for start action", func() {
			var receivedArgs map[string]any
			ts = buildTestServer(toolDef{
				name: "kubernaut_investigate",
				handler: func(_ context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
					_ = json.Unmarshal(req.Params.Arguments, &receivedArgs)
					return &mcp.CallToolResult{
						Content: []mcp.Content{&mcp.TextContent{Text: `{"status":"active","session_id":"s-001"}`}},
					}, nil, nil
				},
			})

			httpClient := &http.Client{Transport: &authedRoundTripper{user: "alice@example.com"}}
			client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, nil, logr.Discard())

			result, err := client.InvokeAction(withIdentityCtx("alice@example.com", []string{"sre"}), ka.InvokeActionArgs{
				RRID:   "prod/rr-pod-crash-001",
				Action: "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(receivedArgs).To(HaveKeyWithValue("rr_id", "prod/rr-pod-crash-001"))
			Expect(receivedArgs).To(HaveKeyWithValue("action", "start"))
		})

		It("UT-AF-1234-002: InvokeAction sends {rr_id, action: takeover} with message", func() {
			var receivedArgs map[string]any
			ts = buildTestServer(toolDef{
				name: "kubernaut_investigate",
				handler: func(_ context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
					_ = json.Unmarshal(req.Params.Arguments, &receivedArgs)
					return &mcp.CallToolResult{
						Content: []mcp.Content{&mcp.TextContent{Text: `{"status":"active"}`}},
					}, nil, nil
				},
			})

			httpClient := &http.Client{Transport: &authedRoundTripper{user: "alice@example.com"}}
			client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, nil, logr.Discard())

			_, err := client.InvokeAction(withIdentityCtx("alice@example.com", []string{"sre"}), ka.InvokeActionArgs{
				RRID:    "prod/rr-pod-crash-001",
				Action:  "takeover",
				Message: "I want to take over this investigation",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedArgs).To(HaveKeyWithValue("rr_id", "prod/rr-pod-crash-001"))
			Expect(receivedArgs).To(HaveKeyWithValue("action", "takeover"))
			Expect(receivedArgs).To(HaveKeyWithValue("message", "I want to take over this investigation"))
		})

		It("UT-AF-1234-003: InvokeAction sends {rr_id, action: message, message: ...}", func() {
			var receivedArgs map[string]any
			ts = buildTestServer(toolDef{
				name: "kubernaut_investigate",
				handler: func(_ context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
					_ = json.Unmarshal(req.Params.Arguments, &receivedArgs)
					return &mcp.CallToolResult{
						Content: []mcp.Content{&mcp.TextContent{Text: `{"status":"active"}`}},
					}, nil, nil
				},
			})

			httpClient := &http.Client{Transport: &authedRoundTripper{user: "alice@example.com"}}
			client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, nil, logr.Discard())

			_, err := client.InvokeAction(withIdentityCtx("alice@example.com", []string{"sre"}), ka.InvokeActionArgs{
				RRID:    "prod/rr-001",
				Action:  "message",
				Message: "Check the pod logs for OOM signals",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedArgs).To(HaveKeyWithValue("action", "message"))
			Expect(receivedArgs).To(HaveKeyWithValue("message", "Check the pod logs for OOM signals"))
		})

		It("UT-AF-1234-004: InvokeAction sends {rr_id, action: discover_workflows}", func() {
			var receivedArgs map[string]any
			ts = buildTestServer(toolDef{
				name: "kubernaut_investigate",
				handler: func(_ context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
					_ = json.Unmarshal(req.Params.Arguments, &receivedArgs)
					return &mcp.CallToolResult{
						Content: []mcp.Content{&mcp.TextContent{Text: `{"status":"active"}`}},
					}, nil, nil
				},
			})

			httpClient := &http.Client{Transport: &authedRoundTripper{user: "alice@example.com"}}
			client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, nil, logr.Discard())

			_, err := client.InvokeAction(withIdentityCtx("alice@example.com", []string{"sre"}), ka.InvokeActionArgs{
				RRID:   "prod/rr-001",
				Action: "discover_workflows",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedArgs).To(HaveKeyWithValue("action", "discover_workflows"))
			Expect(receivedArgs).To(HaveKeyWithValue("rr_id", "prod/rr-001"))
		})

		It("UT-AF-1234-005: InvokeAction sends {rr_id, action: complete}", func() {
			var receivedArgs map[string]any
			ts = buildTestServer(toolDef{
				name: "kubernaut_investigate",
				handler: func(_ context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
					_ = json.Unmarshal(req.Params.Arguments, &receivedArgs)
					return &mcp.CallToolResult{
						Content: []mcp.Content{&mcp.TextContent{Text: `{"status":"completed"}`}},
					}, nil, nil
				},
			})

			httpClient := &http.Client{Transport: &authedRoundTripper{user: "alice@example.com"}}
			client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, nil, logr.Discard())

			_, err := client.InvokeAction(withIdentityCtx("alice@example.com", []string{"sre"}), ka.InvokeActionArgs{
				RRID:   "prod/rr-001",
				Action: "complete",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedArgs).To(HaveKeyWithValue("action", "complete"))
		})
	})

	Describe("Identity injection", func() {
		It("UT-AF-1234-006: acting_user injected from UserIdentityFromContext", func() {
			var receivedArgs map[string]any
			ts = buildTestServer(toolDef{
				name: "kubernaut_investigate",
				handler: func(_ context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
					_ = json.Unmarshal(req.Params.Arguments, &receivedArgs)
					return &mcp.CallToolResult{
						Content: []mcp.Content{&mcp.TextContent{Text: `{"status":"active"}`}},
					}, nil, nil
				},
			})

			httpClient := &http.Client{Transport: &authedRoundTripper{user: "alice@example.com"}}
			client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, nil, logr.Discard())

			_, err := client.InvokeAction(withIdentityCtx("alice@example.com", []string{"sre"}), ka.InvokeActionArgs{
				RRID:   "prod/rr-001",
				Action: "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedArgs).To(HaveKeyWithValue("acting_user", "alice@example.com"))
		})

		It("UT-AF-1234-007: acting_user_groups injected as string slice", func() {
			var receivedArgs map[string]any
			ts = buildTestServer(toolDef{
				name: "kubernaut_investigate",
				handler: func(_ context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
					_ = json.Unmarshal(req.Params.Arguments, &receivedArgs)
					return &mcp.CallToolResult{
						Content: []mcp.Content{&mcp.TextContent{Text: `{"status":"active"}`}},
					}, nil, nil
				},
			})

			httpClient := &http.Client{Transport: &authedRoundTripper{user: "bob@example.com"}}
			client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, nil, logr.Discard())

			_, err := client.InvokeAction(withIdentityCtx("bob@example.com", []string{"sre", "admin"}), ka.InvokeActionArgs{
				RRID:   "prod/rr-001",
				Action: "start",
			})
			Expect(err).NotTo(HaveOccurred())

			groups, ok := receivedArgs["acting_user_groups"]
			Expect(ok).To(BeTrue(), "acting_user_groups should be present in args")
			groupSlice, ok := groups.([]any)
			Expect(ok).To(BeTrue(), "acting_user_groups should be a slice")
			Expect(groupSlice).To(ConsistOf("sre", "admin"))
		})

		It("UT-AF-1234-008: nil UserIdentity returns error (fail-closed)", func() {
			ts = buildTestServer(toolDef{
				name: "kubernaut_investigate",
				handler: func(_ context.Context, _ *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
					return &mcp.CallToolResult{
						Content: []mcp.Content{&mcp.TextContent{Text: `{"status":"active"}`}},
					}, nil, nil
				},
			})

			httpClient := &http.Client{Transport: &authedRoundTripper{user: "anonymous"}}
			client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, nil, logr.Discard())

			_, err := client.InvokeAction(context.Background(), ka.InvokeActionArgs{
				RRID:   "prod/rr-001",
				Action: "start",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("identity"))
		})
	})

	Describe("Error handling", func() {
		It("UT-AF-1234-009: KA unavailable returns user-friendly error", func() {
			httpClient := &http.Client{Transport: &authedRoundTripper{user: "alice@example.com"}}
			client = ka.NewSDKMCPClient("http://localhost:1/mcp", httpClient, nil, logr.Discard())

			_, err := client.InvokeAction(withIdentityCtx("alice@example.com", []string{"sre"}), ka.InvokeActionArgs{
				RRID:   "prod/rr-001",
				Action: "start",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("unavailable"),
				ContainSubstring("retry"),
			))
		})

		It("UT-AF-1234-010: KA IsError result wrapped as kubernaut agent error", func() {
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

			_, err := client.InvokeAction(withIdentityCtx("alice@example.com", []string{"sre"}), ka.InvokeActionArgs{
				RRID:   "prod/rr-001",
				Action: "start",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("kubernaut agent"))
		})
	})

	Describe("httptest Unit Tests", func() {
		It("UT-AF-1234-201: SDKMCPClient.InvokeAction wire format against httptest MCP", func() {
			var receivedToolName string
			var receivedArgs map[string]any
			ts = buildTestServer(toolDef{
				name: "kubernaut_investigate",
				handler: func(_ context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
					receivedToolName = req.Params.Name
					_ = json.Unmarshal(req.Params.Arguments, &receivedArgs)
					return &mcp.CallToolResult{
						Content: []mcp.Content{&mcp.TextContent{Text: `{"status":"active","session_id":"s-it-001"}`}},
					}, nil, nil
				},
			})

			httpClient := &http.Client{Transport: &authedRoundTripper{user: "sre@kubernaut.ai"}}
			client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, nil, logr.Discard())

			result, err := client.InvokeAction(withIdentityCtx("sre@kubernaut.ai", []string{"sre"}), ka.InvokeActionArgs{
				RRID:   "monitoring/rr-oom-456",
				Action: "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedToolName).To(Equal("kubernaut_investigate"))
			Expect(receivedArgs).To(HaveKeyWithValue("rr_id", "monitoring/rr-oom-456"))
			Expect(receivedArgs).To(HaveKeyWithValue("action", "start"))
			Expect(receivedArgs).To(HaveKeyWithValue("acting_user", "sre@kubernaut.ai"))
			Expect(result.Status).To(Equal("active"))
			Expect(result.SessionID).To(Equal("s-it-001"))
		})

		It("UT-AF-1234-202: InvokeAction forwards acting_user in args to KA", func() {
			var receivedArgs map[string]any
			ts = buildTestServer(toolDef{
				name: "kubernaut_investigate",
				handler: func(_ context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
					_ = json.Unmarshal(req.Params.Arguments, &receivedArgs)
					return &mcp.CallToolResult{
						Content: []mcp.Content{&mcp.TextContent{Text: `{"status":"active"}`}},
					}, nil, nil
				},
			})

			httpClient := &http.Client{Transport: &authedRoundTripper{user: "dev@kubernaut.ai"}}
			client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, nil, logr.Discard())

			_, err := client.InvokeAction(withIdentityCtx("dev@kubernaut.ai", []string{"dev-team", "viewers"}), ka.InvokeActionArgs{
				RRID:   "prod/rr-crash-789",
				Action: "message",
				Message: "Can you check the recent deployment?",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedArgs).To(HaveKeyWithValue("acting_user", "dev@kubernaut.ai"))
			groups := receivedArgs["acting_user_groups"]
			Expect(groups).NotTo(BeNil())
		})

		It("UT-AF-1234-203: InvokeAction error mapping from httptest 500", func() {
			ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}))

			httpClient := &http.Client{Transport: &authedRoundTripper{user: "alice@example.com"}}
			client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, nil, logr.Discard())

			_, err := client.InvokeAction(withIdentityCtx("alice@example.com", []string{"sre"}), ka.InvokeActionArgs{
				RRID:   "prod/rr-001",
				Action: "start",
			})
			Expect(err).To(HaveOccurred())
		})
	})
})
