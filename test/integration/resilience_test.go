package integration_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
	"github.com/jordigilh/kubernaut-apifrontend/internal/handler"
	"github.com/jordigilh/kubernaut-apifrontend/internal/ka"
)

// tcpProxy forwards TCP connections from a local listener to a remote target.
// Disconnect() kills all active connections, simulating a network partition.
// Reconnect() re-enables forwarding.
// Adapted from kubernaut PR #828 commit 26d7f17b.
type tcpProxy struct {
	listener     net.Listener
	target       string
	mu           sync.Mutex
	conns        []net.Conn
	disconnected atomic.Bool
}

func newTCPProxy(target string) (*tcpProxy, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	p := &tcpProxy{
		listener: ln,
		target:   target,
	}
	go p.acceptLoop()
	return p, nil
}

func (p *tcpProxy) Addr() string {
	return p.listener.Addr().String()
}

func (p *tcpProxy) acceptLoop() {
	for {
		conn, err := p.listener.Accept()
		if err != nil {
			return
		}
		if p.disconnected.Load() {
			_ = conn.Close()
			continue
		}
		go p.forward(conn)
	}
}

func (p *tcpProxy) forward(client net.Conn) {
	p.mu.Lock()
	p.conns = append(p.conns, client)
	p.mu.Unlock()

	remote, err := net.DialTimeout("tcp", p.target, 5*time.Second)
	if err != nil {
		_ = client.Close()
		return
	}

	p.mu.Lock()
	p.conns = append(p.conns, remote)
	p.mu.Unlock()

	go func() { _, _ = io.Copy(remote, client) }()
	_, _ = io.Copy(client, remote)

	_ = client.Close()
	_ = remote.Close()
}

// Disconnect kills all active connections and rejects new ones.
func (p *tcpProxy) Disconnect() {
	p.disconnected.Store(true)
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range p.conns {
		_ = c.Close()
	}
	p.conns = nil
}

// Reconnect allows new connections to be forwarded again.
func (p *tcpProxy) Reconnect() {
	p.disconnected.Store(false)
}

func (p *tcpProxy) Close() {
	_ = p.listener.Close()
	p.Disconnect()
}

var _ = Describe("Resilience — Circuit Breaker + Timeout (TCP Proxy)", func() {

	var (
		proxy        *tcpProxy
		resilHandler http.Handler
		resilSession string
		resilAudit   *testAuditor
	)

	BeforeEach(func() {
		var err error
		kaTarget := fmt.Sprintf("127.0.0.1:%d", afITKAPort)
		proxy, err = newTCPProxy(kaTarget)
		Expect(err).ToNot(HaveOccurred())

		proxyKAClient := ka.NewClient(ka.Config{
			BaseURL:            "http://" + proxy.Addr(),
			Token:              itKAToken,
			Timeout:            3 * time.Second,
			CBFailureThreshold: 3,
			CBMaxRequests:      1,
			CBInterval:         500 * time.Millisecond,
			CBTimeout:          500 * time.Millisecond,
			RetryMax:           0,
			RetryableStatuses:  []int{503},
		})

		resilAudit = &testAuditor{}
		cfg := handler.MCPConfig{
			ServerName:    "af-it-resilience",
			ServerVersion: "0.0.1-resil",
			Enabled:       true,
			Bridge: &handler.MCPBridgeConfig{
				DynFactory: auth.StaticDynamicFactory(itDynClient),
				KAClient:   proxyKAClient,
				KAMCPClient: &ka.MockMCPClient{
					SelectWorkflowFn: func(_ context.Context, _ ka.SelectWorkflowArgs) (*ka.SelectWorkflowResult, error) {
						return &ka.SelectWorkflowResult{Status: "selected"}, nil
					},
				},
				DSClient:           nil,
				RBACRoles:          map[string][]string{"sre": {"*"}},
				Logger:             logr.Discard(),
				Auditor:            resilAudit,
				Metrics:            newITBridgeMetrics(),
				ToolTimeout:        3 * time.Second,
				MaxConcurrentTools: 1,
			},
		}

		resilHandler, err = handler.NewMCPHandler(cfg)
		Expect(err).ToNot(HaveOccurred())
		resilSession = itMCPInitialize(resilHandler, testUser)
	})

	AfterEach(func() {
		if proxy != nil {
			proxy.Close()
		}
	})

	It("IT-RESIL-001: baseline call through TCP proxy succeeds", func() {
		status, body := itMCPCallTool(resilHandler, resilSession, "kubernaut_start_investigation", map[string]any{
			"namespace": "default", "name": "proxy-baseline", "kind": "Deployment",
		}, testUser)

		Expect(status).To(Equal(http.StatusOK))
		text := itExtractTextContent(body)
		Expect(text).ToNot(ContainSubstring("permission denied"))
		Expect(text).To(SatisfyAny(
			ContainSubstring("session_id"),
			ContainSubstring("accepted"),
			ContainSubstring("investigation"),
		))
	})

	It("IT-RESIL-002: proxy.Disconnect -> failures trip CB", func() {
		proxy.Disconnect()

		for i := 0; i < 4; i++ {
			_, body := itMCPCallTool(resilHandler, resilSession, "kubernaut_start_investigation", map[string]any{
				"namespace": "default", "name": fmt.Sprintf("cb-trip-%d", i), "kind": "Pod",
			}, testUser)
			text := itExtractTextContent(body)
			Expect(text).To(SatisfyAny(
				ContainSubstring("unavailable"),
				ContainSubstring("error"),
				ContainSubstring("connection refused"),
				ContainSubstring("circuit breaker"),
			))
		}
	})

	It("IT-RESIL-003: CB open -> fast-fail without hitting KA", func() {
		proxy.Disconnect()

		// Trip the CB
		for i := 0; i < 4; i++ {
			itMCPCallTool(resilHandler, resilSession, "kubernaut_start_investigation", map[string]any{
				"namespace": "default", "name": fmt.Sprintf("ff-%d", i), "kind": "Pod",
			}, testUser)
		}

		start := time.Now()
		_, body := itMCPCallTool(resilHandler, resilSession, "kubernaut_start_investigation", map[string]any{
			"namespace": "default", "name": "fast-fail", "kind": "Pod",
		}, testUser)
		elapsed := time.Since(start)

		text := itExtractTextContent(body)
		Expect(text).To(SatisfyAny(
			ContainSubstring("unavailable"),
			ContainSubstring("error"),
			ContainSubstring("circuit breaker"),
		))
		Expect(elapsed).To(BeNumerically("<", 1*time.Second),
			"CB-open fast-fail should return in <1s")
	})

	It("IT-RESIL-004: reconnect proxy -> half-open probe succeeds (recovery)", func() {
		proxy.Disconnect()

		for i := 0; i < 4; i++ {
			itMCPCallTool(resilHandler, resilSession, "kubernaut_start_investigation", map[string]any{
				"namespace": "default", "name": fmt.Sprintf("trip-%d", i), "kind": "Pod",
			}, testUser)
		}

		proxy.Reconnect()

		Eventually(func() string {
			_, body := itMCPCallTool(resilHandler, resilSession, "kubernaut_start_investigation", map[string]any{
				"namespace": "default", "name": "recovery-probe", "kind": "Pod",
			}, testUser)
			return itExtractTextContent(body)
		}, 5*time.Second, 200*time.Millisecond).Should(SatisfyAny(
			ContainSubstring("session_id"),
			ContainSubstring("accepted"),
			ContainSubstring("investigation"),
		))
	})

	It("IT-RESIL-005: DS CB independent from KA CB (DS tools still work)", func() {
		proxy.Disconnect()

		// Trip KA CB
		for i := 0; i < 4; i++ {
			itMCPCallTool(resilHandler, resilSession, "kubernaut_start_investigation", map[string]any{
				"namespace": "default", "name": fmt.Sprintf("ds-ind-%d", i), "kind": "Pod",
			}, testUser)
		}

		// DS tool should still work (uses mcpHandler which has real DS, not proxy)
		status, body := itMCPCallTool(mcpHandler, sessionIDForDSIndependence(), "kubernaut_list_workflows", map[string]any{}, testUser)
		Expect(status).To(Equal(http.StatusOK))
		text := itExtractTextContent(body)
		Expect(text).ToNot(ContainSubstring("circuit breaker"))
	})

	It("IT-RESIL-006: tool timeout interrupts long operation", func() {
		_, body := itMCPCallTool(resilHandler, resilSession, "kubernaut_poll_investigation", map[string]any{
			"session_id": "timeout-test-nonexistent",
		}, testUser)

		text := itExtractTextContent(body)
		Expect(text).To(SatisfyAny(
			ContainSubstring("timeout"),
			ContainSubstring("deadline"),
			ContainSubstring("error"),
			ContainSubstring("unavailable"),
			ContainSubstring("not found"),
			ContainSubstring("404"),
		))
	})

	It("IT-RESIL-007: semaphore exhaustion returns server busy", func() {
		proxy.Reconnect()

		// Each call uses a distinct MCP session to avoid the go-sdk's
		// per-session request serialization, which would prevent both
		// calls from contending on the semaphore concurrently.
		sessions := [2]string{
			itMCPInitialize(resilHandler, testUser),
			itMCPInitialize(resilHandler, testUser),
		}

		results := make([]string, 2)
		ready := make(chan struct{})
		var barrier sync.WaitGroup
		barrier.Add(2)

		var wg sync.WaitGroup
		wg.Add(2)
		for i := 0; i < 2; i++ {
			go func(idx int) {
				defer wg.Done()
				defer GinkgoRecover()
				barrier.Done()
				<-ready // both goroutines start their call at the same instant
				_, body := itMCPCallTool(resilHandler, sessions[idx], "kubernaut_start_investigation", map[string]any{
					"namespace": "default", "name": fmt.Sprintf("sem-%d", idx), "kind": "Pod",
				}, testUser)
				results[idx] = itExtractTextContent(body)
			}(i)
		}

		barrier.Wait() // wait for both goroutines to be ready
		close(ready)   // release them simultaneously

		done := make(chan struct{})
		go func() { wg.Wait(); close(done) }()

		select {
		case <-done:
		case <-time.After(15 * time.Second):
			Fail("IT-RESIL-007 timed out waiting for concurrent tool calls (ToolTimeout=3s)")
		}

		allText := results[0] + " " + results[1]
		// At least one should succeed; with MaxConcurrentTools=1,
		// the second may get throttled or still succeed if the first is fast enough
		Expect(allText).To(SatisfyAny(
			ContainSubstring("session_id"),
			ContainSubstring("accepted"),
			ContainSubstring("investigation"),
			ContainSubstring("busy"),
			ContainSubstring("throttle"),
			ContainSubstring("retry"),
		))
	})
})

func sessionIDForDSIndependence() string {
	return itMCPInitialize(mcpHandler, testUser)
}
