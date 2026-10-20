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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
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
		DeferCleanup(func() {
			gateway.CloseClientConnections()
			gateway.Close()
		})

		tmpDir, err := os.MkdirTemp("", "connect-gateway-tls-test")
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() { Expect(os.RemoveAll(tmpDir)).To(Succeed()) })

		gatewayCAPath := filepath.Join(tmpDir, "gateway-ca.pem")
		wrongCAPath := filepath.Join(tmpDir, "wrong-ca.pem")
		invalidCAPath := filepath.Join(tmpDir, "invalid-ca.pem")
		Expect(os.WriteFile(gatewayCAPath, gatewayCA, 0o600)).To(Succeed())
		Expect(os.WriteFile(wrongCAPath, unrelatedCAPEM(), 0o600)).To(Succeed())
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
		cfg.MCPGatewayTLSCAFile = wrongCAPath
		rc, err = mcpclient.Connect(context.Background(), cfg, logger)
		Expect(rc).NotTo(BeNil())
		Expect(err).To(HaveOccurred())
		Expect(rc.Ready()).To(BeFalse())
		Expect(rc.Close()).To(Succeed())

		cfg.MCPGatewayTLSCAFile = invalidCAPath
		rc, err = mcpclient.Connect(context.Background(), cfg, logger)
		Expect(rc).NotTo(BeNil())
		Expect(err).To(HaveOccurred())
		Expect(rc.Ready()).To(BeFalse())
		Expect(rc.Close()).To(Succeed())
	})

	It("UT-FLEET-CONN-005 [SC-8, IA-5]: keeps gateway and OAuth2 token CA trust independent", func() {
		gateway, gatewayCA := newTLSTestGateway()
		tokenServer, tokenCA := newTLSTestServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			response.Header().Set("Content-Type", "application/json")
			if _, err := response.Write([]byte(`{"access_token":"test-token","token_type":"Bearer","expires_in":3600}`)); err != nil {
				logger.Error(err, "failed to write OAuth2 token test response")
			}
		}))
		DeferCleanup(func() {
			gateway.CloseClientConnections()
			gateway.Close()
			tokenServer.CloseClientConnections()
			tokenServer.Close()
		})

		tmpDir, err := os.MkdirTemp("", "connect-gateway-oauth2-tls-test")
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() { Expect(os.RemoveAll(tmpDir)).To(Succeed()) })
		gatewayCAPath := filepath.Join(tmpDir, "gateway-ca.pem")
		tokenCAPath := filepath.Join(tmpDir, "token-ca.pem")
		Expect(os.WriteFile(gatewayCAPath, gatewayCA, 0o600)).To(Succeed())
		Expect(os.WriteFile(tokenCAPath, tokenCA, 0o600)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(tmpDir, "client-id"), []byte("client-id"), 0o600)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(tmpDir, "client-secret"), []byte("client-secret"), 0o600)).To(Succeed())

		cfg := mcpclient.ConnectConfig{
			Endpoint:            gateway.URL,
			MCPGatewayTLSCAFile: gatewayCAPath,
			OAuth2: fleet.FleetOAuth2Config{
				Enabled:   true,
				TokenURL:  tokenServer.URL,
				TLSCAFile: tokenCAPath,
			},
			CredentialsBasePath: tmpDir,
			Resilience: fleet.FleetResilienceConfig{
				InitialInterval: 50 * time.Millisecond,
				MaxInterval:     100 * time.Millisecond,
				MaxElapsedTime:  500 * time.Millisecond,
			},
		}
		rc, err := mcpclient.Connect(context.Background(), cfg, logger)
		Expect(err).NotTo(HaveOccurred())
		Expect(rc.Ready()).To(BeTrue())
		Expect(rc.Close()).To(Succeed())
	})
})

// newTLSTestGateway creates a stateful MCP server with a unique self-signed
// certificate so gateway and token-endpoint trust can be tested independently.
func newTLSTestGateway() (*httptest.Server, []byte) {
	server := mcp.NewServer(
		&mcp.Implementation{Name: "tls-test-mcp-gateway", Version: "v0.1.0"},
		nil,
	)
	handler := mcp.NewStreamableHTTPHandler(
		func(_ *http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{DisableLocalhostProtection: true},
	)
	return newTLSTestServer(handler)
}

// newTLSTestServer starts an HTTPS test server and returns its CA PEM.
func newTLSTestServer(handler http.Handler) (*httptest.Server, []byte) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "localhost"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"localhost"},
	}
	certificateDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	Expect(err).NotTo(HaveOccurred())
	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificateDER})
	privateKey, err := x509.MarshalECPrivateKey(key)
	Expect(err).NotTo(HaveOccurred())
	certificate, err := tls.X509KeyPair(certificatePEM, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privateKey}))
	Expect(err).NotTo(HaveOccurred())
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	return server, certificatePEM
}

// unrelatedCAPEM returns a valid CA that does not sign the gateway certificate.
func unrelatedCAPEM() []byte {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "unrelated-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	certificateDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	Expect(err).NotTo(HaveOccurred())
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificateDER})
}
