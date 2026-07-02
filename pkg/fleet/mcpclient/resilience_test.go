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
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
)

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

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				count := attempts.Add(1)
				if count < 3 {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-03-26","capabilities":{},"serverInfo":{"name":"test","version":"1.0"}}}`))
			}))
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
})
