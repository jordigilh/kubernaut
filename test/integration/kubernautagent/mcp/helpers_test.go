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
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"

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
	sharedauth "github.com/jordigilh/kubernaut/pkg/shared/auth"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------------------------------------------------------------------------
// Real MCP Test Stack (ZERO MOCKS policy compliant)
// Uses Podman Mock LLM (mode=interactive) and real DataStorage from suite.
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
	customRunner      tools.InvestigatorRunner // if set, bypasses the real investigator
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
// - Podman Mock LLM (mode=interactive) for real investigator.Investigator via langchaingo
// - Podman DataStorage for real DSContextReconstructor via ogenclient
// - real TimeoutManager, SessionRateLimiter, SessionNotifier, DelegatingEventStore
func newRealMCPTestStack(k8sClient client.Client, namespace string, opts realStackOpts) *realMCPTestStack {
	stack := &realMCPTestStack{
		K8sClient: k8sClient,
		Namespace: namespace,
	}

	logrLogger := logr.Discard()

	var runner tools.InvestigatorRunner
	if opts.customRunner != nil {
		runner = opts.customRunner
	} else {
		// Real LLM client via langchaingo -> Podman Mock LLM (mode=interactive)
		llmAdapter, err := langchaingo.New("openai", sharedMockLLMEndpoint, "test-model", "test-key")
		Expect(err).ToNot(HaveOccurred(), "langchaingo adapter should build against Mock LLM at %s", sharedMockLLMEndpoint)
		stack.LLMClient = llmAdapter

		// Real investigator.Investigator
		promptBuilder, err := prompt.NewBuilder()
		Expect(err).ToNot(HaveOccurred(), "prompt builder should build")

		inv := investigator.New(investigator.Config{
			Client:       llmAdapter,
			Builder:      promptBuilder,
			ResultParser: parser.NewResultParser(),
			AuditStore:   audit.NopAuditStore{},
			Logger:       logrLogger,
			MaxTurns:     15,
			ModelName:    "test-model",
		})
		runner = adapters.NewInvestigatorRunnerAdapter(inv)
	}

	// Real DSContextReconstructor via ogenclient -> Podman DataStorage
	Expect(sharedDSClient).ToNot(BeNil(), "shared DS client must be initialized by suite")
	recon := mcpinternal.NewDSContextReconstructor(sharedDSClient, 5*time.Second, logrLogger)

	// Real LeaseSessionManager via envtest K8s client
	leaseOpts := []mcpinternal.LeaseOption{
		mcpinternal.WithSessionTTL(opts.sessionTTL),
	}
	if opts.maxSessions > 0 {
		leaseOpts = append(leaseOpts, mcpinternal.WithMaxConcurrentSessions(opts.maxSessions))
	}
	stack.SessionMgr = mcpinternal.NewLeaseSessionManagerConcrete(k8sClient, namespace, logrLogger, leaseOpts...)

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
	investigateTool := tools.NewInvestigateTool(stack.SessionMgr, runner, recon, tools.NopAutonomousManager{}, investigateOpts...)

	toolDeps := mcpinternal.ToolDeps{
		Investigate: tools.InvestigateRegistration(investigateTool, stack.EventStore, stack.Notifier, logr.Discard()),
	}

	handler, srv := mcpinternal.BootstrapMCP(mcpinternal.MCPDeps{
		AuthMiddleware: fakeAuthMiddlewareWithUserInfo,
		Tools:          toolDeps,
		EventStore:     stack.EventStore,
	})
	stack.MCPServer = srv

	r := chi.NewRouter()
	r.Use(fakeAuthMiddlewareWithUserInfo)
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
}

// ---------------------------------------------------------------------------
// Mock LLM verification helpers (Podman container API)
// ---------------------------------------------------------------------------

// getMockLLMRequestCount queries the Mock LLM's verification API for total request count.
func getMockLLMRequestCount() int {
	GinkgoHelper()
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(sharedMockLLMEndpoint + "/api/test/request-count")
	Expect(err).NotTo(HaveOccurred(), "Mock LLM request-count should be reachable")
	defer resp.Body.Close()
	Expect(resp.StatusCode).To(Equal(http.StatusOK))

	var data struct {
		Count int `json:"count"`
	}
	Expect(json.NewDecoder(resp.Body).Decode(&data)).To(Succeed())
	return data.Count
}

// ---------------------------------------------------------------------------
// Namespace helpers for test isolation
// ---------------------------------------------------------------------------

func uniqueNamespace(prefix string) string {
	return fmt.Sprintf("mcp-it-%s-%d-%s",
		prefix,
		GinkgoParallelProcess(),
		uuid.New().String()[:8])
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
// Uses a 10s deadline as defense-in-depth against SSE handshake hangs.
func connectMCP(ts *httptest.Server, username string) (*mcpsdk.ClientSession, error) {
	mcpClient := mcpsdk.NewClient(&mcpsdk.Implementation{
		Name:    "integration-test-client",
		Version: "0.0.1",
	}, nil)

	transport := &mcpsdk.StreamableClientTransport{
		Endpoint:   ts.URL + "/mcp",
		HTTPClient: authedHTTPClient(username),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return mcpClient.Connect(ctx, transport, nil)
}

// callInvestigate is a helper that calls kubernaut_investigate with the given args.
// Uses a 30s deadline to bound the investigator -> Mock LLM round-trip.
func callInvestigate(session *mcpsdk.ClientSession, args map[string]any) (*mcpsdk.CallToolResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return session.CallTool(ctx, &mcpsdk.CallToolParams{
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
