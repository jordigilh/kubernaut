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
	mockgw "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
)

var _ = Describe("SP Fleet OAuth2 Integration (BR-INTEGRATION-054)", func() {
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

	Describe("IT-SP-054-OAuth2 [SC-7]: SP authenticates to the MCP Gateway before accessing remote cluster data, proving boundary protection is enforced for every cross-cluster call", func() {
		It("acquires OAuth2 token via client_credentials and creates MCP client with auth transport", func() {
			tmpDir, err := os.MkdirTemp("", "sp-fleet-auth-*")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			Expect(os.WriteFile(filepath.Join(tmpDir, "client-id"), []byte("kubernaut-fleet-read"), 0o600)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(tmpDir, "client-secret"), []byte("e2e-fleet-secret"), 0o600)).To(Succeed())

			var tokenRequests int64
			var capturedGrantType string
			var capturedScope string
			tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt64(&tokenRequests, 1)
				_ = r.ParseForm()
				capturedGrantType = r.PostFormValue("grant_type")
				capturedScope = r.PostFormValue("scope")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"access_token":"sp-fleet-jwt","token_type":"Bearer","expires_in":3600}`))
			}))
			defer tokenServer.Close()

			var capturedAuth string
			authCheckProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if auth := r.Header.Get("Authorization"); auth != "" {
					capturedAuth = auth
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"name":"proxy","version":"0.1"},"capabilities":{}}}`)))
			}))
			defer authCheckProxy.Close()

			logger := zap.New(zap.UseDevMode(true))
			reloadCfg := mcpclient.ReloadableOAuth2Config{
				TokenURL:         tokenServer.URL,
				ClientIDPath:     filepath.Join(tmpDir, "client-id"),
				ClientSecretPath: filepath.Join(tmpDir, "client-secret"),
				Scopes:           mcpclient.DefaultFleetScopes(nil),
				TokenTimeout:     5 * time.Second,
			}

			transport, err := mcpclient.NewReloadableOAuth2Transport(
				reloadCfg, http.DefaultTransport, logger)
			Expect(err).ToNot(HaveOccurred())

			httpClient := &http.Client{Transport: transport}
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, authCheckProxy.URL, nil)
			Expect(err).ToNot(HaveOccurred())
			resp, err := httpClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			resp.Body.Close()

			Expect(atomic.LoadInt64(&tokenRequests)).To(BeNumerically(">=", 1),
				"token server must be called to acquire credentials")
			Expect(capturedGrantType).To(Equal("client_credentials"),
				"SP must use client_credentials grant for service-to-service auth")
			Expect(capturedScope).To(Equal("openid groups"),
				"default scopes must be 'openid groups' for minimal privilege")
			Expect(capturedAuth).To(Equal("Bearer sp-fleet-jwt"),
				"Bearer token from token server must be injected into MCP requests")

			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("loopback-cluster"))
			client, err := mcpclient.New(ctx, gw.URL(),
				mcpclient.WithHTTPClient(&http.Client{Transport: transport}),
			)
			Expect(err).ToNot(HaveOccurred())
			defer client.Close()

			tools, err := client.Session().ListTools(ctx, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(tools.Tools).ToNot(BeEmpty(),
				"OAuth2-authenticated client must discover tools through MCP Gateway")
		})
	})
})
