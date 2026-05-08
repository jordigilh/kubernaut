package ka_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
	"github.com/jordigilh/kubernaut-apifrontend/internal/ka"
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
			client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, logr.Discard())

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
			client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, logr.Discard())

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
			client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, logr.Discard())

			_, err := client.SelectWorkflow(context.Background(), ka.SelectWorkflowArgs{
				RRID:       "rr-test-002",
				WorkflowID: "wf-002",
			})
			Expect(err).To(HaveOccurred())
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
			client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, logr.Discard())

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
			client = ka.NewSDKMCPClient(ts.URL+"/mcp", httpClient, logr.Discard())

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
