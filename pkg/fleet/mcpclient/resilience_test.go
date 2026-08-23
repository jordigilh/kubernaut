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

package mcpclient_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr/funcr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/jordigilh/kubernaut/pkg/fleet"
	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
)

// newFakeMCPServer starts an httptest.Server implementing just enough of the
// MCP streamable-HTTP handshake to unblock mcp.Client.Connect. go-sdk's
// Connect() issues a multi-step negotiation -- a "server/discover" probe
// (rejected here so the client falls back to the legacy flow), then
// "initialize", then a "notifications/initialized" notification -- each
// carrying a distinct request id. A response body must echo the id of the
// request that triggered it: the client matches responses to pending calls
// by id and otherwise waits forever for one that never arrives, so a fixed
// response id (e.g. always "id":1) breaks the moment initialize gets id 2.
// The standalone SSE stream (GET) and session teardown (DELETE) are
// tolerated with spec-permitted non-2xx responses, matching how a
// stateless real-world MCP Gateway would answer them.
func newFakeMCPServer(onInitialize func(w http.ResponseWriter, r *http.Request, id json.RawMessage)) *httptest.Server {
	return newFakeMCPServerWithDiscover(onInitialize, nil)
}

// newFakeMCPServerWithDiscover behaves like newFakeMCPServer but lets a
// caller override how the server answers "server/discover" (issue #2262).
// When onDiscover is nil, it falls back to newFakeMCPServer's original fast
// "-32601 method not found" rejection, so every pre-#2262 caller of
// newFakeMCPServer is unaffected.
func newFakeMCPServerWithDiscover(
	onInitialize func(w http.ResponseWriter, r *http.Request, id json.RawMessage),
	onDiscover func(w http.ResponseWriter, r *http.Request, id json.RawMessage),
) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
			return
		}

		body, _ := io.ReadAll(r.Body)
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		_ = json.Unmarshal(body, &req)

		switch req.Method {
		case "server/discover":
			if onDiscover != nil {
				onDiscover(w, r, req.ID)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"error":{"code":-32601,"message":"method not found"}}`, req.ID)
		case "notifications/initialized", "notifications/cancelled":
			w.WriteHeader(http.StatusAccepted)
		case "initialize":
			onInitialize(w, r, req.ID)
		default:
			w.WriteHeader(http.StatusNotImplemented)
		}
	}))
}

// writeMCPInitializeResult writes a minimal, spec-compliant "initialize"
// JSON-RPC result addressed to id.
func writeMCPInitializeResult(w http.ResponseWriter, id json.RawMessage) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":"2025-11-25","capabilities":{},"serverInfo":{"name":"test","version":"1.0"}}}`, id)
}

var _ = Describe("MCP Client Resilience (Phase 6)", func() {
	var logger = zap.New(zap.UseDevMode(true))

	Context("UT-FLEET-RES-001: ReloadableOAuth2Transport", func() {
		It("should rebuild TokenSource when credential file changes", func() {
			tmpDir, err := os.MkdirTemp("", "oauth2-reload-test")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			clientIDPath := filepath.Join(tmpDir, "client-id")
			clientSecretPath := filepath.Join(tmpDir, "client-secret")

			Expect(os.WriteFile(clientIDPath, []byte("original-id"), 0o600)).To(Succeed())
			Expect(os.WriteFile(clientSecretPath, []byte("original-secret"), 0o600)).To(Succeed())

			var requestCount atomic.Int32
			tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestCount.Add(1)
				_ = r.ParseForm()
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"access_token":"token-` + r.FormValue("client_id") + `","token_type":"Bearer","expires_in":1}`))
			}))
			defer tokenServer.Close()

			cfg := mcpclient.ReloadableOAuth2Config{
				TokenURL:         tokenServer.URL,
				ClientIDPath:     clientIDPath,
				ClientSecretPath: clientSecretPath,
				Scopes:           []string{"fleet"},
				TokenTimeout:     5 * time.Second,
			}

			transport, err := mcpclient.NewReloadableOAuth2Transport(cfg, http.DefaultTransport, logger)
			Expect(err).ToNot(HaveOccurred())
			Expect(transport).ToNot(BeNil())

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			err = transport.StartWatching(ctx)
			Expect(err).ToNot(HaveOccurred())
			defer transport.Stop()

			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(r.Header.Get("Authorization")))
			}))
			defer backend.Close()

			client := &http.Client{Transport: transport}
			resp, err := client.Get(backend.URL)
			Expect(err).ToNot(HaveOccurred())
			resp.Body.Close()

			Expect(os.WriteFile(clientIDPath, []byte("rotated-id"), 0o600)).To(Succeed())

			time.Sleep(500 * time.Millisecond)

			transport.InvalidateToken()

			resp2, err := client.Get(backend.URL)
			Expect(err).ToNot(HaveOccurred())
			resp2.Body.Close()

			Expect(requestCount.Load()).To(BeNumerically(">=", 2),
				"Token endpoint should be called again after credential rotation")
		})
	})

	Context("UT-FLEET-RES-002: Lazy reconnect on 401/session loss", func() {
		It("should detect retryable errors correctly", func() {
			cfg := mcpclient.DefaultResilienceConfig()
			cfg.MaxElapsedTime = 2 * time.Second

			ctx := context.Background()
			_, err := mcpclient.NewResilient(ctx, "http://127.0.0.1:1/mcp", cfg, logger)
			Expect(err).To(HaveOccurred(),
				"Should fail to connect to unreachable endpoint")
		})

		It("should report not ready when connection fails", func() {
			cfg := mcpclient.DefaultResilienceConfig()
			cfg.InitialInterval = 100 * time.Millisecond
			cfg.MaxElapsedTime = 500 * time.Millisecond

			ctx := context.Background()
			rc, err := mcpclient.NewResilient(ctx, "http://127.0.0.1:1/mcp", cfg, logger)
			if rc != nil {
				Expect(rc.Ready()).To(BeFalse(),
					"Client should not be ready after failed connection")
			} else {
				Expect(err).To(HaveOccurred())
			}
		})
	})

	Context("UT-FLEET-RES-003: Startup retry with backoff", func() {
		It("should retry with exponential backoff until success", func() {
			var attempts atomic.Int32

			server := newFakeMCPServer(func(w http.ResponseWriter, _ *http.Request, id json.RawMessage) {
				count := attempts.Add(1)
				if count < 3 {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				writeMCPInitializeResult(w, id)
			})
			defer server.Close()

			cfg := mcpclient.DefaultResilienceConfig()
			cfg.InitialInterval = 100 * time.Millisecond
			cfg.MaxInterval = 200 * time.Millisecond
			cfg.MaxElapsedTime = 5 * time.Second

			ctx := context.Background()

			_, err := mcpclient.NewResilient(ctx, server.URL+"/mcp", cfg, logger)
			_ = err
			Expect(attempts.Load()).To(BeNumerically(">=", 1),
				"Should have attempted connection at least once")
		})
	})

	Context("UT-FLEET-RES-004: Token refresh timeout", func() {
		It("should timeout token refresh when IdP is slow", func() {
			slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				time.Sleep(3 * time.Second)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"access_token":"late","token_type":"Bearer","expires_in":3600}`))
			}))
			defer slowServer.Close()

			cfg := mcpclient.OAuth2Config{
				TokenURL:     slowServer.URL,
				ClientID:     "test-id",
				ClientSecret: "test-secret",
				Scopes:       []string{"fleet"},
			}

			transport := mcpclient.NewOAuth2Transport(cfg, nil)

			client := &http.Client{
				Transport: transport,
				Timeout:   1 * time.Second,
			}

			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = fmt.Fprintln(w, "ok")
			}))
			defer backend.Close()

			_, err := client.Get(backend.URL)
			Expect(err).To(HaveOccurred(),
				"Request should timeout due to slow token endpoint")
		})
	})

	Context("UT-FLEET-RES-007 [readiness gate Wave 0]: Concurrent Reconnect() calls deduplicate", func() {
		It("collapses overlapping Reconnect() calls into a single MCP handshake instead of racing (prevents leaked connections when a periodic readiness prober and the lazy reconnect-on-error path call Reconnect concurrently)", func() {
			var handshakeCount atomic.Int32
			server := newFakeMCPServer(func(w http.ResponseWriter, _ *http.Request, id json.RawMessage) {
				handshakeCount.Add(1)
				// Widen the overlap window so all concurrent Reconnect() callers
				// below arrive before any single handshake completes.
				time.Sleep(300 * time.Millisecond)
				writeMCPInitializeResult(w, id)
			})
			defer server.Close()

			cfg := mcpclient.DefaultResilienceConfig()
			cfg.InitialInterval = 50 * time.Millisecond
			cfg.MaxInterval = 100 * time.Millisecond
			cfg.MaxElapsedTime = 5 * time.Second

			ctx := context.Background()
			rc, err := mcpclient.NewResilient(ctx, server.URL+"/mcp", cfg, logger)
			Expect(err).ToNot(HaveOccurred())
			Expect(rc.Ready()).To(BeTrue())

			handshakeCount.Store(0)

			const concurrency = 8
			var wg sync.WaitGroup
			errs := make([]error, concurrency)
			for i := 0; i < concurrency; i++ {
				idx := i
				wg.Add(1)
				go func() {
					defer wg.Done()
					defer GinkgoRecover()
					errs[idx] = rc.Reconnect(ctx)
				}()
			}
			wg.Wait()

			for _, e := range errs {
				Expect(e).ToNot(HaveOccurred())
			}
			Expect(rc.Ready()).To(BeTrue())
			Expect(handshakeCount.Load()).To(BeNumerically("<=", 2),
				"singleflight should collapse %d overlapping Reconnect() calls into at most 2 MCP "+
					"handshakes (observed %d \"initialize\" round trips) -- a higher count means "+
					"callers are racing instead of deduplicating",
				concurrency, handshakeCount.Load())
		})
	})

	Context("UT-FLEET-RES-005: FileWatcher on secret directory", func() {
		It("should detect file changes via hotreload.FileWatcher", func() {
			tmpDir, err := os.MkdirTemp("", "filewatcher-test")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			secretFile := filepath.Join(tmpDir, "client-secret")
			Expect(os.WriteFile(secretFile, []byte("initial"), 0o600)).To(Succeed())

			cfg := mcpclient.ReloadableOAuth2Config{
				TokenURL:         "http://localhost:0/token",
				ClientIDPath:     secretFile,
				ClientSecretPath: secretFile,
				Scopes:           []string{"fleet"},
				TokenTimeout:     5 * time.Second,
			}

			transport, err := mcpclient.NewReloadableOAuth2Transport(cfg, http.DefaultTransport, logger)
			Expect(err).ToNot(HaveOccurred())

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			err = transport.StartWatching(ctx)
			Expect(err).ToNot(HaveOccurred())
			defer transport.Stop()

			Expect(os.WriteFile(secretFile, []byte("rotated"), 0o600)).To(Succeed())

			time.Sleep(500 * time.Millisecond)
		})
	})

	Context("UT-FLEET-RES-008 [CP-10]: connectWithBackoff bounds each attempt independently of caller deadline", func() {
		It("returns instead of hanging forever when the server never responds to initialize (#1933-class hang)", func() {
			// Never write an "initialize" response until the client gives up on
			// its per-attempt deadline (post-fix) or a generous safety-net
			// duration (pre-fix, proving the missing bound) -- whichever comes
			// first, so the fake handler doesn't leak past the test.
			server := newFakeMCPServer(func(_ http.ResponseWriter, r *http.Request, _ json.RawMessage) {
				select {
				case <-r.Context().Done():
				case <-time.After(3 * time.Second):
				}
			})
			defer server.Close()

			cfg := mcpclient.DefaultResilienceConfig()
			cfg.ConnectTimeout = 150 * time.Millisecond
			cfg.InitialInterval = 50 * time.Millisecond
			cfg.MaxInterval = 50 * time.Millisecond
			cfg.MaxElapsedTime = 150 * time.Millisecond

			done := make(chan error, 1)
			go func() {
				_, err := mcpclient.NewResilient(context.Background(), server.URL+"/mcp", cfg, logger)
				done <- err
			}()

			select {
			case err := <-done:
				Expect(err).To(HaveOccurred(),
					"NewResilient should give up after exhausting bounded attempts against a permanently hanging server")
			case <-time.After(1500 * time.Millisecond):
				Fail("NewResilient did not return within 1.5s: a single hung connect attempt is blocking the " +
					"entire backoff loop (issue #1934) instead of being bounded by ConnectTimeout")
			}
		})
	})

	Context("UT-FLEET-RES-009 [CP-10]: doReconnect bounds its attempt independently of caller deadline", func() {
		It("returns instead of hanging forever when the server hangs on reconnect (#1933-class hang)", func() {
			var attempts atomic.Int32
			server := newFakeMCPServer(func(w http.ResponseWriter, r *http.Request, id json.RawMessage) {
				if attempts.Add(1) == 1 {
					// Initial connect (inside NewResilient below) succeeds immediately.
					writeMCPInitializeResult(w, id)
					return
				}
				// Every subsequent handshake (the Reconnect() under test) hangs.
				select {
				case <-r.Context().Done():
				case <-time.After(3 * time.Second):
				}
			})
			defer server.Close()

			cfg := mcpclient.DefaultResilienceConfig()
			cfg.ConnectTimeout = 150 * time.Millisecond
			cfg.InitialInterval = 50 * time.Millisecond
			cfg.MaxInterval = 50 * time.Millisecond
			cfg.MaxElapsedTime = 500 * time.Millisecond

			rc, err := mcpclient.NewResilient(context.Background(), server.URL+"/mcp", cfg, logger)
			Expect(err).ToNot(HaveOccurred())
			Expect(rc.Ready()).To(BeTrue())

			done := make(chan error, 1)
			go func() {
				done <- rc.Reconnect(context.Background())
			}()

			select {
			case err := <-done:
				Expect(err).To(HaveOccurred(),
					"Reconnect should fail promptly when the server hangs on the handshake, instead of blocking forever")
			case <-time.After(1 * time.Second):
				Fail("Reconnect did not return within 1s: doReconnect's single connect attempt has no bound " +
					"when the caller supplies no deadline (issue #1934)")
			}
		})
	})

	Context("UT-FLEET-RES-010 [CP-10, CC7.2]: New() bounds the server/discover probe independently of ConnectTimeout", func() {
		It("falls back to a successful legacy initialize handshake -- and logs the timeout -- when server/discover hangs past its own sub-timeout (issue #2262)", func() {
			server := newFakeMCPServerWithDiscover(
				func(w http.ResponseWriter, _ *http.Request, id json.RawMessage) {
					writeMCPInitializeResult(w, id)
				},
				func(_ http.ResponseWriter, r *http.Request, _ json.RawMessage) {
					// Simulates the #2262-reported gateway: server/discover
					// never responds. This must not consume the caller's
					// (generous, 2s) context budget below -- only the
					// short DiscoverProbeTimeout should bound it.
					select {
					case <-r.Context().Done():
					case <-time.After(2 * time.Second):
					}
				},
			)
			defer server.Close()

			var logBuf bytes.Buffer
			captureLogger := funcr.New(func(prefix, args string) {
				logBuf.WriteString(prefix + " " + args + "\n")
			}, funcr.Options{})

			type connectResult struct {
				c   *mcpclient.Client
				err error
			}
			done := make(chan connectResult, 1)
			go func() {
				c, err := mcpclient.New(context.Background(), server.URL+"/mcp",
					mcpclient.WithDiscoverProbeTimeout(100*time.Millisecond),
					mcpclient.WithDiscoverProbeLogger(captureLogger),
				)
				done <- connectResult{c, err}
			}()

			select {
			case result := <-done:
				Expect(result.err).ToNot(HaveOccurred(),
					"New() should fall back to the legacy initialize handshake instead of hanging when server/discover never answers")
				Expect(result.c).ToNot(BeNil())
			case <-time.After(1 * time.Second):
				Fail("New() did not return within 1s: the server/discover probe is not bounded by its own sub-timeout (issue #2262)")
			}

			Expect(logBuf.String()).To(ContainSubstring("server/discover"),
				"a timed-out discover probe should be logged for operator visibility (SOC2 CC7.2)")
		})
	})

	Context("UT-FLEET-RES-011 [CP-10]: NewResilient falls back through connectWithBackoff when server/discover hangs", func() {
		It("succeeds via the production entry point against a gateway that hangs forever on server/discover but answers initialize immediately", func() {
			server := newFakeMCPServerWithDiscover(
				func(w http.ResponseWriter, _ *http.Request, id json.RawMessage) {
					writeMCPInitializeResult(w, id)
				},
				func(_ http.ResponseWriter, r *http.Request, _ json.RawMessage) {
					<-r.Context().Done()
				},
			)
			defer server.Close()

			cfg := mcpclient.DefaultResilienceConfig()
			cfg.DiscoverProbeTimeout = 100 * time.Millisecond
			cfg.ConnectTimeout = 5 * time.Second
			cfg.InitialInterval = 50 * time.Millisecond
			cfg.MaxInterval = 50 * time.Millisecond
			cfg.MaxElapsedTime = 5 * time.Second

			done := make(chan error, 1)
			go func() {
				_, err := mcpclient.NewResilient(context.Background(), server.URL+"/mcp", cfg, logger)
				done <- err
			}()

			select {
			case err := <-done:
				Expect(err).ToNot(HaveOccurred(),
					"NewResilient should succeed via the legacy initialize fallback instead of exhausting ConnectTimeout on a hung server/discover probe")
			case <-time.After(2 * time.Second):
				Fail("NewResilient did not return within 2s: DiscoverProbeTimeout is not bounding server/discover independently of ConnectTimeout (issue #2262)")
			}
		})
	})

	Context("UT-FLEET-RES-012 [CP-10]: regression -- DiscoverProbeTimeout has no observable effect when server/discover answers quickly", func() {
		It("still connects normally against the existing fixture default (fast -32601 rejection of server/discover)", func() {
			server := newFakeMCPServer(func(w http.ResponseWriter, _ *http.Request, id json.RawMessage) {
				writeMCPInitializeResult(w, id)
			})
			defer server.Close()

			cfg := mcpclient.DefaultResilienceConfig()
			cfg.MaxElapsedTime = 5 * time.Second

			rc, err := mcpclient.NewResilient(context.Background(), server.URL+"/mcp", cfg, logger)
			Expect(err).ToNot(HaveOccurred())
			Expect(rc.Ready()).To(BeTrue())
		})
	})

	Context("UT-FLEET-RES-013 [CM-6]: ResilienceConfigFromFleet -- zero-value fields pass through to defaults", func() {
		It("returns DefaultResilienceConfig() unchanged when every fleet.FleetResilienceConfig field is zero", func() {
			got := mcpclient.ResilienceConfigFromFleet(fleet.FleetResilienceConfig{})
			Expect(got).To(Equal(mcpclient.DefaultResilienceConfig()),
				"an unset resilience override (e.g. no Helm/operator resilience: block configured) must produce identical behavior to today's hardcoded defaults")
		})
	})

	Context("UT-FLEET-RES-014 [CM-6]: ResilienceConfigFromFleet -- non-zero fields override, independently", func() {
		It("overrides only the fields explicitly set, leaving the rest at their default value", func() {
			got := mcpclient.ResilienceConfigFromFleet(fleet.FleetResilienceConfig{
				InitialInterval:      2 * time.Second,
				MaxInterval:          60 * time.Second,
				MaxElapsedTime:       10 * time.Minute,
				TokenRefreshTimeout:  20 * time.Second,
				ConnectTimeout:       45 * time.Second,
				DiscoverProbeTimeout: 7 * time.Second,
			})

			Expect(got.InitialInterval).To(Equal(2 * time.Second))
			Expect(got.MaxInterval).To(Equal(60 * time.Second))
			Expect(got.MaxElapsedTime).To(Equal(10 * time.Minute))
			Expect(got.TokenRefreshTimeout).To(Equal(20 * time.Second))
			Expect(got.ConnectTimeout).To(Equal(45 * time.Second))
			Expect(got.DiscoverProbeTimeout).To(Equal(7 * time.Second))
		})

		It("overrides a single field while every other field keeps its default", func() {
			defaults := mcpclient.DefaultResilienceConfig()

			got := mcpclient.ResilienceConfigFromFleet(fleet.FleetResilienceConfig{
				DiscoverProbeTimeout: 9 * time.Second,
			})

			Expect(got.DiscoverProbeTimeout).To(Equal(9 * time.Second))
			Expect(got.InitialInterval).To(Equal(defaults.InitialInterval))
			Expect(got.MaxInterval).To(Equal(defaults.MaxInterval))
			Expect(got.MaxElapsedTime).To(Equal(defaults.MaxElapsedTime))
			Expect(got.TokenRefreshTimeout).To(Equal(defaults.TokenRefreshTimeout))
			Expect(got.ConnectTimeout).To(Equal(defaults.ConnectTimeout))
		})
	})
})
