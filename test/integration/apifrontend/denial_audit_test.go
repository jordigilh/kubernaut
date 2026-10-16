package apifrontend_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-logr/zapr"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	adksession "google.golang.org/adk/v2/session"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	agentpkg "github.com/jordigilh/kubernaut/pkg/apifrontend/agent"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/handler"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	sharedaudit "github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// Tracks only external Kubernetes CRD calls. Internal tool/guard/store logic is real.
type denialReadTransport struct {
	base  http.RoundTripper
	reads *atomic.Int32
}

func (t *denialReadTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.Contains(req.URL.Path, "/remediationrequests") {
		t.reads.Add(1)
	}
	return t.base.RoundTrip(req)
}

var _ = Describe("BR-AUDIT-001 / BR-AUDIT-005: schema-valid denied dispatch (AU-2, AU-3, SI-10, AC-6)", func() {
	DescribeTable("real MCP HTTP denial remains fail-closed and observable",
		func(consoleGrant, sarFailure, storeFailure bool) {
			actor := "denial-2500-" + uuid.NewString()
			if consoleGrant {
				grantDenialConsoleAccess(actor)
			}
			core, logs := observer.New(zapcore.DebugLevel)
			logger := zapr.NewLogger(zap.New(core))
			transport, err := sharedtls.NewTLSTransport(infrastructure.BootstrapTLSCAFile("https://localhost:18096"))
			Expect(err).ToNot(HaveOccurred())
			dsURL := "https://localhost:18096"
			if storeFailure {
				backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					http.Error(w, "test-owned audit backend unavailable", http.StatusServiceUnavailable)
				}))
				DeferCleanup(backend.Close)
				dsURL = backend.URL
			}
			auditClient, err := sharedaudit.NewOpenAPIClientAdapterWithTransport(dsURL, time.Second,
				&bearerTransport{token: serviceAccountToken, base: transport})
			Expect(err).ToNot(HaveOccurred())
			store, err := sharedaudit.NewBufferedStore(auditClient, sharedaudit.Config{
				BufferSize: 100, BatchSize: 10, FlushInterval: 50 * time.Millisecond, MaxRetries: 1,
			}, "apifrontend", logger)
			Expect(err).ToNot(HaveOccurred())
			emitter := audit.NewStoreAdapter(store, logger)
			DeferCleanup(func() {
				err := emitter.Close(context.Background())
				if storeFailure {
					Expect(err).To(HaveOccurred())
				} else {
					Expect(err).ToNot(HaveOccurred())
				}
			})
			var reads atomic.Int32
			clientCfg := rest.CopyConfig(restCfg)
			clientCfg.WrapTransport = func(base http.RoundTripper) http.RoundTripper {
				return &denialReadTransport{base: base, reads: &reads}
			}
			typedClient, err := crclient.NewWithWatch(clientCfg, crclient.Options{Scheme: scheme})
			Expect(err).ToNot(HaveOccurred())
			sarClient := k8sClientset
			if sarFailure {
				sarBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					http.Error(w, "test-owned SAR failure", http.StatusServiceUnavailable)
				}))
				DeferCleanup(sarBackend.Close)
				sarClient, err = kubernetes.NewForConfig(&rest.Config{Host: sarBackend.URL, Timeout: time.Second})
				Expect(err).ToNot(HaveOccurred())
			}
			mcpHandler, err := handler.NewMCPHandler(handler.MCPConfig{
				Enabled: true, ServerName: "audit-denial-it", ServerVersion: "1.0",
				Bridge: &handler.MCPBridgeConfig{
					K8sClient: dynamicClient, TypedClient: typedClient, Namespace: defaultFixture,
					Authorizer: auth.NewSARChecker(sarClient, time.Second, logger), Auditor: emitter,
					Logger: logger, ToolTimeout: 2 * time.Second, MaxConcurrentTools: 2,
				},
			})
			Expect(err).ToNot(HaveOccurred())
			// Exercise the production MCP/auth dispatch directly. Unused A2A,
			// agent-card and readiness dependencies are covered by router specs.
			server := httptest.NewServer(auth.MiddlewareWithConfig(auth.MiddlewareConfig{
				Validator: jwtValidator, Logger: logger,
			})(mcpHandler))
			DeferCleanup(server.Close)
			token := signValidToken(actor)
			_, sessionID := denialRPC(server.URL+"/mcp", token, "", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"audit-it","version":"1"}}}`)
			Expect(sessionID).ToNot(BeEmpty())
			denialRPC(server.URL+"/mcp", token, sessionID, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
			body, _ := denialRPC(server.URL+"/mcp", token, sessionID, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"kubernaut_list_remediations","arguments":{}}}`)
			Expect(body).To(ContainSubstring(`"isError":true`))
			Expect(reads.Load()).To(BeZero(), "denied tool must not reach the external Kubernetes API")
			if storeFailure {
				Expect(store.Flush(context.Background())).To(HaveOccurred(),
					"mandatory audit flush must surface the unavailable backend")
				Eventually(func() int { return logs.FilterLevelExact(zapcore.ErrorLevel).Len() }, 5*time.Second, 20*time.Millisecond).Should(BeNumerically(">", 0))
			} else {
				Expect(store.Flush(context.Background())).To(Succeed())
				assertPersistedDenial(actor, "mcp")
			}
			Expect(fmt.Sprint(logs.All())).ToNot(ContainSubstring(token), "audit errors must not leak JWTs")
		},
		Entry("IT-AF-2500-001: tool permission denied after console grant", true, false, false),
		Entry("IT-AF-2500-002: console permission denied", false, false, false),
		Entry("IT-AF-2500-004: SAR API failure stays fail-closed", true, true, false),
		Entry("IT-AF-2500-005: audit backend failure is observable without tool execution", true, false, true),
	)

	It("IT-AF-2500-003: actual A2A agent dispatch preserves a2a attribution", func() {
		actor := "a2a-denial-2500-" + uuid.NewString()
		transport, err := sharedtls.NewTLSTransport(infrastructure.BootstrapTLSCAFile("https://localhost:18096"))
		Expect(err).ToNot(HaveOccurred())
		auditClient, err := sharedaudit.NewOpenAPIClientAdapterWithTransport("https://localhost:18096", time.Second,
			&bearerTransport{token: serviceAccountToken, base: transport})
		Expect(err).ToNot(HaveOccurred())
		store, err := sharedaudit.NewBufferedStore(auditClient, sharedaudit.Config{
			BufferSize: 100, BatchSize: 10, FlushInterval: 50 * time.Millisecond, MaxRetries: 1,
		}, "apifrontend", zapr.NewLogger(zap.NewNop()))
		Expect(err).ToNot(HaveOccurred())
		emitter := audit.NewStoreAdapter(store, zapr.NewLogger(zap.NewNop()))
		DeferCleanup(func() { Expect(emitter.Close(context.Background())).To(Succeed()) })
		var llmCalls, reads atomic.Int32
		llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if llmCalls.Add(1) == 1 {
				_, err := io.WriteString(w, `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"kubernaut_list_remediations","args":{}}}]},"finishReason":"STOP"}],"modelVersion":"mock-model"}`)
				Expect(err).ToNot(HaveOccurred())
				return
			}
			_, err := io.WriteString(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"Permission denied."}]},"finishReason":"STOP"}],"modelVersion":"mock-model"}`)
			Expect(err).ToNot(HaveOccurred())
		}))
		DeferCleanup(llm.Close)
		model, err := launcher.NewModelFromConfig(context.Background(), types.LLMConfig{
			Provider: types.LLMProviderGemini, Model: "mock-model", Endpoint: llm.URL, APIKey: "test-key",
		})
		Expect(err).ToNot(HaveOccurred())
		clientCfg := rest.CopyConfig(restCfg)
		clientCfg.WrapTransport = func(base http.RoundTripper) http.RoundTripper {
			return &denialReadTransport{base: base, reads: &reads}
		}
		typedClient, err := crclient.NewWithWatch(clientCfg, crclient.Options{Scheme: scheme})
		Expect(err).ToNot(HaveOccurred())
		root, _, err := agentpkg.NewRootAgent(agentpkg.AgentConfig{
			Instruction: "Call kubernaut_list_remediations once and report its result.", LLMModel: model,
			TypedClient: typedClient, K8sClient: dynamicClient, Namespace: defaultFixture,
			Authorizer: auth.NewSARChecker(k8sClientset, time.Second, zapr.NewLogger(zap.NewNop())), Auditor: emitter,
		})
		Expect(err).ToNot(HaveOccurred())
		a2a, err := launcher.NewA2AHandler(launcher.A2AConfig{Agent: root,
			SessionService: adksession.InMemoryService(), AppName: "denial-audit-it", Auditor: emitter})
		Expect(err).ToNot(HaveOccurred())
		server := httptest.NewServer(auth.MiddlewareWithConfig(auth.MiddlewareConfig{Validator: jwtValidator})(a2a))
		DeferCleanup(server.Close)
		denialRPC(server.URL, signValidToken(actor), "", `{"jsonrpc":"2.0","id":"2500","method":"message/send","params":{"message":{"messageId":"msg-2500","role":"user","parts":[{"kind":"text","text":"list remediations"}]}}}`)
		Expect(llmCalls.Load()).To(BeNumerically(">=", 1))
		Expect(reads.Load()).To(BeZero())
		Expect(store.Flush(context.Background())).To(Succeed())
		assertPersistedDenial(actor, "a2a")
	})
})

func grantDenialConsoleAccess(actor string) {
	role := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: actor}, Rules: []rbacv1.PolicyRule{{
		APIGroups: []string{"kubernaut.ai"}, Resources: []string{"console"}, Verbs: []string{"use"},
	}}}
	Expect(k8sClient.Create(context.Background(), role)).To(Succeed())
	DeferCleanup(func() { Expect(k8sClient.Delete(context.Background(), role)).To(Succeed()) })
	binding := &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: actor},
		RoleRef:  rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: actor},
		Subjects: []rbacv1.Subject{{Kind: "User", APIGroup: "rbac.authorization.k8s.io", Name: actor}}}
	Expect(k8sClient.Create(context.Background(), binding)).To(Succeed())
	DeferCleanup(func() { Expect(k8sClient.Delete(context.Background(), binding)).To(Succeed()) })
}

func denialRPC(url, token, sessionID, body string) (string, string) {
	reqCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewBufferString(body))
	Expect(err).ToNot(HaveOccurred())
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	resp, err := http.DefaultClient.Do(req)
	Expect(err).ToNot(HaveOccurred())
	defer func() { Expect(resp.Body.Close()).To(Succeed()) }()
	raw, err := io.ReadAll(resp.Body)
	Expect(err).ToNot(HaveOccurred())
	Expect(resp.StatusCode).To(BeNumerically("<", 400), "dispatch response: %s", raw)
	return string(raw), resp.Header.Get("Mcp-Session-Id")
}

func assertPersistedDenial(actor, endpoint string) {
	transport, err := sharedtls.NewTLSTransport(infrastructure.BootstrapTLSCAFile("https://localhost:18096"))
	Expect(err).ToNot(HaveOccurred())
	query, err := ogenclient.NewClient("https://localhost:18096", ogenclient.WithClient(&http.Client{
		Transport: &bearerTransport{token: serviceAccountToken, base: transport}, Timeout: 2 * time.Second,
	}))
	Expect(err).ToNot(HaveOccurred())
	var found ogenclient.AuditEvent
	Eventually(func(g Gomega) {
		response, err := query.QueryAuditEvents(context.Background(), ogenclient.QueryAuditEventsParams{
			EventType: ogenclient.NewOptString("apifrontend.auth.access_denied"),
			DetailKey: ogenclient.NewOptString("tool_name"), DetailValue: ogenclient.NewOptString("kubernaut_list_remediations"),
			Since: ogenclient.NewOptString("1m"), Limit: ogenclient.NewOptInt(1000),
		})
		g.Expect(err).ToNot(HaveOccurred())
		for _, event := range response.Data {
			if event.ActorID.Value == actor {
				found = event
				break
			}
		}
		g.Expect(found.EventID.IsSet()).To(BeTrue(), "missing persisted denial for unique actor %s", actor)
	}, 10*time.Second, 50*time.Millisecond).Should(Succeed())
	Expect(found.EventData.Validate()).To(Succeed())
	payload, ok := found.EventData.GetApifrontendAuthAccessDeniedPayload()
	Expect(ok).To(BeTrue())
	Expect(string(payload.Endpoint)).To(Equal(endpoint))
	Expect(payload.RequiredRoles).ToNot(BeNil())
	Expect(payload.RequiredRoles).To(BeEmpty(), "SAR does not reveal policy roles; never invent them")
	Expect(found.EventAction).To(Equal("denied"))
	Expect(string(found.EventOutcome)).To(Equal("failure"))
	Expect(found.ActorType.Value).To(Equal("user"))
	Expect(found.CorrelationID).ToNot(BeEmpty())
	response, err := query.QueryAuditEvents(context.Background(), ogenclient.QueryAuditEventsParams{CorrelationID: ogenclient.NewOptString(found.CorrelationID)})
	Expect(err).ToNot(HaveOccurred())
	Expect(response).ToNot(BeNil())
	Expect(response.Data).To(ContainElement(HaveField("EventID", Equal(found.EventID))), "round-trip the actual persisted correlation")
}
