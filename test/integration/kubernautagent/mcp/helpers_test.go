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

package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/adapters"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/langchaingo"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	openaitypes "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
	sharedauth "github.com/jordigilh/kubernaut/pkg/shared/auth"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------------------------------------------------------------------------
// Mock LLM httptest.Server (OpenAI-compatible /v1/chat/completions)
// ---------------------------------------------------------------------------

// mockLLMServer wraps an httptest.Server that serves OpenAI chat completions.
// Tests enqueue responses; the server dequeues them in FIFO order.
type mockLLMServer struct {
	Server    *httptest.Server
	mu        sync.Mutex
	responses []openaitypes.ChatCompletionResponse
	requests  []openaitypes.ChatCompletionRequest
}

func newMockLLMServer() *mockLLMServer {
	m := &mockLLMServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", m.handleChatCompletions)
	m.Server = httptest.NewServer(mux)
	return m
}

func (m *mockLLMServer) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}

	var req openaitypes.ChatCompletionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "decode json: "+err.Error(), http.StatusBadRequest)
		return
	}

	m.mu.Lock()
	m.requests = append(m.requests, req)
	var resp openaitypes.ChatCompletionResponse
	if len(m.responses) > 0 {
		resp = m.responses[0]
		m.responses = m.responses[1:]
	} else {
		text := "Default mock LLM response"
		resp = openaitypes.ChatCompletionResponse{
			ID:      "chatcmpl-test",
			Object:  "chat.completion",
			Model:   "test-model",
			Choices: []openaitypes.Choice{{Index: 0, Message: openaitypes.Message{Role: "assistant", Content: &text}, FinishReason: "stop"}},
			Usage:   openaitypes.Usage{PromptTokens: 10, CompletionTokens: 10, TotalTokens: 20},
		}
	}
	m.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// Enqueue adds a response to the FIFO queue.
func (m *mockLLMServer) Enqueue(resp openaitypes.ChatCompletionResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses = append(m.responses, resp)
}

// EnqueueText adds a plain text assistant response.
func (m *mockLLMServer) EnqueueText(text string) {
	m.Enqueue(openaitypes.ChatCompletionResponse{
		ID:      "chatcmpl-test",
		Object:  "chat.completion",
		Model:   "test-model",
		Choices: []openaitypes.Choice{{Index: 0, Message: openaitypes.Message{Role: "assistant", Content: &text}, FinishReason: "stop"}},
		Usage:   openaitypes.Usage{PromptTokens: 10, CompletionTokens: 10, TotalTokens: 20},
	})
}

// GetRequests returns a copy of all captured requests.
func (m *mockLLMServer) GetRequests() []openaitypes.ChatCompletionRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]openaitypes.ChatCompletionRequest, len(m.requests))
	copy(cp, m.requests)
	return cp
}

func (m *mockLLMServer) Close() {
	m.Server.Close()
}

// ---------------------------------------------------------------------------
// Mock DataStorage httptest.Server (GET /api/v1/audit/events)
// ---------------------------------------------------------------------------

type mockDSServer struct {
	Server *httptest.Server
	mu     sync.Mutex
	events []json.RawMessage
}

func newMockDSServer() *mockDSServer {
	d := &mockDSServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/audit/events", d.handleQueryAuditEvents)
	d.Server = httptest.NewServer(mux)
	return d
}

func (d *mockDSServer) handleQueryAuditEvents(w http.ResponseWriter, _ *http.Request) {
	d.mu.Lock()
	events := d.events
	d.mu.Unlock()

	resp := map[string]interface{}{
		"data":       events,
		"pagination": map[string]interface{}{},
	}
	if events == nil {
		resp["data"] = []interface{}{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (d *mockDSServer) Close() {
	d.Server.Close()
}

// ---------------------------------------------------------------------------
// Real MCP Test Stack (ZERO MOCKS policy compliant)
// ---------------------------------------------------------------------------

// realMCPTestStack holds all real production components for integration tests.
type realMCPTestStack struct {
	Server      *httptest.Server
	MCPServer   *mcpinternal.MCPServer
	SessionMgr  *mcpinternal.LeaseSessionManager
	RateLimiter *mcpinternal.SessionRateLimiter
	TimeoutMgr  *mcpinternal.TimeoutManager
	Notifier    *mcpinternal.SessionNotifier
	EventStore  *mcpinternal.DelegatingEventStore
	MockLLM     *mockLLMServer
	MockDS      *mockDSServer
	LLMClient   llm.Client
	K8sClient   client.Client
	Namespace   string

	expiredMu       sync.Mutex
	expiredSessions []string
}

func (s *realMCPTestStack) GetExpiredSessions() []string {
	s.expiredMu.Lock()
	defer s.expiredMu.Unlock()
	cp := make([]string, len(s.expiredSessions))
	copy(cp, s.expiredSessions)
	return cp
}

func (s *realMCPTestStack) addExpired(sessionID string) {
	s.expiredMu.Lock()
	defer s.expiredMu.Unlock()
	s.expiredSessions = append(s.expiredSessions, sessionID)
}

// realStackOpts configures the real test stack.
type realStackOpts struct {
	maxPerMinute      int
	maxMessageSize    int
	inactivityTimeout time.Duration
	warningIntervals  []time.Duration
	maxSessions       int
	sessionTTL        time.Duration
}

func defaultRealStackOpts() realStackOpts {
	return realStackOpts{
		maxPerMinute:      10,
		maxMessageSize:    64 * 1024,
		inactivityTimeout: 5 * time.Second,
		maxSessions:       0,
		sessionTTL:        30 * time.Minute,
	}
}

// newRealMCPTestStack builds a fully production-wired MCP test stack using:
// - envtest K8s client for real LeaseSessionManager
// - httptest mock LLM for real investigator.Investigator via langchaingo
// - httptest mock DS for real DSContextReconstructor via ogenclient
// - real TimeoutManager, SessionRateLimiter, SessionNotifier, DelegatingEventStore
func newRealMCPTestStack(k8sClient client.Client, namespace string, opts realStackOpts) *realMCPTestStack {
	stack := &realMCPTestStack{
		K8sClient: k8sClient,
		Namespace: namespace,
	}

	logger := slog.New(slog.NewTextHandler(GinkgoWriter, &slog.HandlerOptions{Level: slog.LevelError}))

	// Mock LLM httptest.Server
	stack.MockLLM = newMockLLMServer()

	// Mock DS httptest.Server
	stack.MockDS = newMockDSServer()

	// Real LLM client via langchaingo -> mock LLM
	llmAdapter, err := langchaingo.New("openai", stack.MockLLM.Server.URL, "test-model", "test-key")
	Expect(err).ToNot(HaveOccurred(), "langchaingo adapter should build")
	stack.LLMClient = llmAdapter

	// Real investigator.Investigator (minimal config for RunInteractiveTurn)
	promptBuilder, err := prompt.NewBuilder()
	Expect(err).ToNot(HaveOccurred(), "prompt builder should build")

	inv := investigator.New(investigator.Config{
		Client:     llmAdapter,
		Builder:    promptBuilder,
		ResultParser: parser.NewResultParser(),
		AuditStore: audit.NopAuditStore{},
		Logger:     logger,
		MaxTurns:   15,
		ModelName:  "test-model",
	})
	runner := adapters.NewInvestigatorRunnerAdapter(inv)

	// Real DSContextReconstructor via ogenclient -> mock DS
	dsClient, err := ogenclient.NewClient(stack.MockDS.Server.URL)
	Expect(err).ToNot(HaveOccurred(), "ogen DS client should build")
	recon := mcpinternal.NewDSContextReconstructor(dsClient, 5*time.Second, logger)

	// Real LeaseSessionManager via envtest K8s client
	leaseOpts := []mcpinternal.LeaseOption{
		mcpinternal.WithSessionTTL(opts.sessionTTL),
	}
	if opts.maxSessions > 0 {
		leaseOpts = append(leaseOpts, mcpinternal.WithMaxConcurrentSessions(opts.maxSessions))
	}
	stack.SessionMgr = mcpinternal.NewLeaseSessionManagerConcrete(k8sClient, namespace, logger, leaseOpts...)

	// Real rate limiter, notifier, timeout manager
	stack.RateLimiter = mcpinternal.NewSessionRateLimiter(opts.maxPerMinute, opts.maxMessageSize)
	stack.Notifier = mcpinternal.NewSessionNotifier()
	warningIntervals := opts.warningIntervals
	if warningIntervals == nil {
		warningIntervals = []time.Duration{opts.inactivityTimeout - 1*time.Second}
	}
	stack.TimeoutMgr = mcpinternal.NewTimeoutManager(
		opts.inactivityTimeout,
		warningIntervals,
		func(sessionID string) {
			stack.addExpired(sessionID)
			_ = stack.SessionMgr.Release(sessionID, "inactivity_timeout")
		},
	)

	// Real DelegatingEventStore
	stack.EventStore = mcpinternal.NewDelegatingEventStore()

	// Wire InvestigateTool with all real components
	investigateOpts := []tools.InvestigateOption{
		tools.WithRateLimiter(stack.RateLimiter),
		tools.WithTimeoutTracker(stack.TimeoutMgr),
		tools.WithNotifyFunc(stack.Notifier.Notify),
	}
	investigateTool := tools.NewInvestigateTool(stack.SessionMgr, runner, recon, investigateOpts...)

	toolDeps := mcpinternal.ToolDeps{
		Investigate: tools.InvestigateRegistration(investigateTool, stack.EventStore, stack.Notifier),
	}

	handler, srv := mcpinternal.BootstrapMCP(mcpinternal.MCPDeps{
		AuthMiddleware: fakeAuthMiddlewareWithUserInfo,
		Tools:          toolDeps,
		EventStore:     stack.EventStore,
	})
	stack.MCPServer = srv

	r := chi.NewRouter()
	r.Handle("/mcp", handler)
	r.Handle("/mcp/*", handler)
	stack.Server = httptest.NewServer(r)

	return stack
}

func (s *realMCPTestStack) Close() {
	if s.TimeoutMgr != nil {
		s.TimeoutMgr.StopAll()
	}
	if s.Server != nil {
		s.Server.Close()
	}
	if s.LLMClient != nil {
		_ = s.LLMClient.Close()
	}
	if s.MockLLM != nil {
		s.MockLLM.Close()
	}
	if s.MockDS != nil {
		s.MockDS.Close()
	}
}

// ---------------------------------------------------------------------------
// Namespace helpers for test isolation
// ---------------------------------------------------------------------------

var nsCounter uint64
var nsCounterMu sync.Mutex

func uniqueNamespace(prefix string) string {
	nsCounterMu.Lock()
	nsCounter++
	n := nsCounter
	nsCounterMu.Unlock()
	return fmt.Sprintf("mcp-it-%s-%d", prefix, n)
}

func createNamespace(ctx context.Context, k8sClient client.Client, name string) {
	GinkgoHelper()
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
	Expect(k8sClient.Create(ctx, ns)).To(Succeed(), "namespace %s should be created", name)
}

// ---------------------------------------------------------------------------
// Auth helpers (shared across all test files)
// ---------------------------------------------------------------------------

// fakeAuthMiddlewareWithUserInfo injects both UserContextKey and UserInfoContextKey
// from the Authorization header, simulating the production middleware with groups.
func fakeAuthMiddlewareWithUserInfo(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		username := "unknown"
		if len(authHeader) > len("Bearer token-for-") {
			username = authHeader[len("Bearer token-for-"):]
		}
		groups := []string{"sre", "platform"}
		ctx := context.WithValue(r.Context(), sharedauth.UserContextKey, username)
		ctx = context.WithValue(ctx, sharedauth.UserInfoContextKey, sharedauth.UserInfo{
			Username: username,
			Groups:   groups,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// authedHTTPClient returns an *http.Client that always includes the Authorization header.
func authedHTTPClient(user string) *http.Client {
	return &http.Client{
		Transport: &authedTransport{user: user, base: http.DefaultTransport},
	}
}

type authedTransport struct {
	user string
	base http.RoundTripper
}

func (t *authedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer token-for-"+t.user)
	return t.base.RoundTrip(req)
}

// ---------------------------------------------------------------------------
// MCP SDK helpers (shared across all test files)
// ---------------------------------------------------------------------------

// connectMCP creates an MCP SDK client session connected to the test server.
func connectMCP(ts *httptest.Server, username string) (*mcpsdk.ClientSession, error) {
	mcpClient := mcpsdk.NewClient(&mcpsdk.Implementation{
		Name:    "integration-test-client",
		Version: "0.0.1",
	}, nil)

	transport := &mcpsdk.StreamableClientTransport{
		Endpoint:   ts.URL + "/mcp",
		HTTPClient: authedHTTPClient(username),
	}
	return mcpClient.Connect(context.Background(), transport, nil)
}

// callInvestigate is a helper that calls kubernaut_investigate with the given args.
func callInvestigate(session *mcpsdk.ClientSession, args map[string]any) (*mcpsdk.CallToolResult, error) {
	return session.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "kubernaut_investigate",
		Arguments: args,
	})
}

// decodeOutput extracts a map from the structured tool result.
func decodeOutput(result *mcpsdk.CallToolResult) (map[string]interface{}, error) {
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		return nil, err
	}
	var output map[string]interface{}
	err = json.Unmarshal(raw, &output)
	return output, err
}

