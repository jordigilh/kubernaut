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
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/jordigilh/kubernaut/pkg/fleet"
	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	mockgw "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
)

var _ = Describe("Connect (issue #2315 self-healing fix)", func() {
	var logger = zap.New(zap.UseDevMode(true))

	It("UT-FLEET-CONN-001: always returns a non-nil client even when the initial connect fails", func() {
		cfg := mcpclient.ConnectConfig{
			Endpoint: "http://127.0.0.1:1/mcp",
			Resilience: fleet.FleetResilienceConfig{
				InitialInterval: 50 * time.Millisecond,
				MaxInterval:     100 * time.Millisecond,
				MaxElapsedTime:  300 * time.Millisecond,
			},
		}

		rc, err := mcpclient.Connect(context.Background(), cfg, logger)
		Expect(rc).ToNot(BeNil(),
			"Connect must always return a non-nil *ResilientClient when Endpoint is set, "+
				"regardless of whether the initial connect attempt succeeded -- callers must "+
				"not gate dependent resolver/factory construction on the returned error")
		Expect(err).To(HaveOccurred())
		Expect(rc.Ready()).To(BeFalse())
	})

	It("UT-FLEET-CONN-002: succeeds against a reachable gateway with no OAuth2 configured", func() {
		gw := mockgw.NewMockGateway(mockgw.WithMultiCluster("c1"))
		defer gw.Close()

		cfg := mcpclient.ConnectConfig{
			Endpoint: gw.URL(),
		}

		rc, err := mcpclient.Connect(context.Background(), cfg, logger)
		Expect(err).ToNot(HaveOccurred())
		Expect(rc).ToNot(BeNil())
		Expect(rc.Ready()).To(BeTrue())
		defer func() { _ = rc.Close() }()
	})

	It("UT-FLEET-CONN-003: wires a reloadable OAuth2 transport when OAuth2.Enabled is true", func() {
		tmpDir, err := os.MkdirTemp("", "connect-oauth2-test")
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = os.RemoveAll(tmpDir) }()

		Expect(os.WriteFile(filepath.Join(tmpDir, "client-id"), []byte("id"), 0o600)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(tmpDir, "client-secret"), []byte("secret"), 0o600)).To(Succeed())

		gw := mockgw.NewMockGateway(mockgw.WithMultiCluster("c1"))
		defer gw.Close()

		cfg := mcpclient.ConnectConfig{
			Endpoint: gw.URL(),
			OAuth2: fleet.FleetOAuth2Config{
				Enabled:  true,
				TokenURL: "http://127.0.0.1:1/token",
			},
			CredentialsBasePath: tmpDir,
			Resilience: fleet.FleetResilienceConfig{
				InitialInterval: 50 * time.Millisecond,
				MaxInterval:     100 * time.Millisecond,
				MaxElapsedTime:  300 * time.Millisecond,
			},
		}

		// The OAuth2 transport is lazy (no token fetched at construction
		// time), but every connect attempt against the reachable gateway
		// still fails because the unreachable token endpoint can't supply a
		// bearer token -- proving the transport was actually wired into the
		// connect path, not silently skipped.
		rc, err := mcpclient.Connect(context.Background(), cfg, logger)
		Expect(rc).ToNot(BeNil())
		Expect(err).To(HaveOccurred())
		Expect(rc.Ready()).To(BeFalse())
	})

	It("UT-FLEET-CONN-004 [SC-8]: uses the configured MCP Gateway CA without changing OAuth2 trust", func() {
		gateway, gatewayCA := newTLSTestGateway()
		defer func() {
			gateway.CloseClientConnections()
			gateway.Close()
		}()

		tmpDir, err := os.MkdirTemp("", "connect-gateway-tls-test")
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = os.RemoveAll(tmpDir) }()

		gatewayCAPath := filepath.Join(tmpDir, "gateway-ca.pem")
		invalidCAPath := filepath.Join(tmpDir, "invalid-ca.pem")
		Expect(os.WriteFile(gatewayCAPath, gatewayCA, 0o600)).To(Succeed())
		Expect(os.WriteFile(invalidCAPath, []byte("not a certificate"), 0o600)).To(Succeed())

		cfg := mcpclient.ConnectConfig{
			Endpoint:            gateway.URL,
			MCPGatewayTLSCAFile: gatewayCAPath,
			Resilience: fleet.FleetResilienceConfig{
				InitialInterval: 50 * time.Millisecond,
				MaxInterval:     100 * time.Millisecond,
				MaxElapsedTime:  300 * time.Millisecond,
			},
		}
		rc, err := mcpclient.Connect(context.Background(), cfg, logger)
		Expect(err).NotTo(HaveOccurred())
		Expect(rc.Ready()).To(BeTrue())
		Expect(rc.Close()).To(Succeed())

		cfg.Endpoint = gateway.URL
		cfg.MCPGatewayTLSCAFile = invalidCAPath
		rc, err = mcpclient.Connect(context.Background(), cfg, logger)
		Expect(rc).NotTo(BeNil())
		Expect(err).To(HaveOccurred())
		Expect(rc.Ready()).To(BeFalse())
		Expect(rc.Close()).To(Succeed())
	})
})

func newTLSTestGateway() (*httptest.Server, []byte) {
	server := mcp.NewServer(
		&mcp.Implementation{Name: "tls-test-mcp-gateway", Version: "v0.1.0"},
		nil,
	)
	handler := mcp.NewStreamableHTTPHandler(
		func(_ *http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{DisableLocalhostProtection: true},
	)
	httpServer := httptest.NewTLSServer(handler)
	return httpServer, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: httpServer.Certificate().Raw})
}
