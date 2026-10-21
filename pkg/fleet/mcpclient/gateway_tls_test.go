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
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/jordigilh/kubernaut/pkg/fleet"
	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
)

func fastGatewayResilience() fleet.FleetResilienceConfig {
	return fleet.FleetResilienceConfig{
		InitialInterval: 20 * time.Millisecond,
		MaxInterval:     40 * time.Millisecond,
		MaxElapsedTime:  300 * time.Millisecond,
		ConnectTimeout:  200 * time.Millisecond,
	}
}

func mcpGatewayHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		body, err := io.ReadAll(r.Body)
		Expect(err).ToNot(HaveOccurred())
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		Expect(json.Unmarshal(body, &request)).To(Succeed())
		switch request.Method {
		case "server/discover":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"error":{"code":-32601,"message":"method not found"}}`, request.ID)
		case "notifications/initialized", "notifications/cancelled":
			w.WriteHeader(http.StatusAccepted)
		case "initialize":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":"2025-11-25","capabilities":{},"serverInfo":{"name":"test","version":"1.0"}}}`, request.ID)
		case "tools/call":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"content":[{"type":"text","text":"{\"apiVersion\":\"v1\",\"kind\":\"Pod\",\"metadata\":{\"name\":\"nginx\"}}"}]}}`, request.ID)
		default:
			w.WriteHeader(http.StatusNotImplemented)
		}
	})
}

func newMCPGatewayServer(tlsEnabled bool) (*httptest.Server, string) {
	handler := mcpGatewayHandler()

	var httpServer *httptest.Server
	if tlsEnabled {
		httpServer = httptest.NewUnstartedServer(handler)
		httpServer.StartTLS()
	} else {
		httpServer = httptest.NewServer(handler)
	}

	var caPath string
	if tlsEnabled {
		cert := httpServer.Certificate()
		Expect(cert).ToNot(BeNil())
		caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
		tmpDir, err := os.MkdirTemp("", "mcp-gateway-tls-2540-")
		Expect(err).ToNot(HaveOccurred())
		caPath = filepath.Join(tmpDir, "gateway-ca.crt")
		Expect(os.WriteFile(caPath, caPEM, 0o600)).To(Succeed())
		DeferCleanup(func() {
			httpServer.Close()
			Expect(os.RemoveAll(tmpDir)).To(Succeed())
		})
	} else {
		DeferCleanup(httpServer.Close)
	}

	return httpServer, caPath
}

func generateMCPGatewayCertificate() (tls.Certificate, []byte) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).ToNot(HaveOccurred())
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	Expect(err).ToNot(HaveOccurred())
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	Expect(err).ToNot(HaveOccurred())
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	Expect(err).ToNot(HaveOccurred())
	return pair, certPEM
}

func newRotatingMCPGatewayServer() (*httptest.Server, string, func()) {
	firstCert, firstCAPEM := generateMCPGatewayCertificate()
	secondCert, secondCAPEM := generateMCPGatewayCertificate()
	var current atomic.Pointer[tls.Certificate]
	current.Store(&firstCert)

	server := httptest.NewUnstartedServer(mcpGatewayHandler())
	server.TLS = &tls.Config{
		MinVersion: tls.VersionTLS12,
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			return current.Load(), nil
		},
	}
	server.StartTLS()
	// StartTLS installs a generated static certificate when Certificates is
	// empty. Clear it so the listener uses GetCertificate for the rotation
	// assertion below.
	server.TLS.Certificates = nil

	tmpDir, err := os.MkdirTemp("", "mcp-gateway-rotate-2540-")
	Expect(err).ToNot(HaveOccurred())
	caPath := filepath.Join(tmpDir, "gateway-ca.crt")
	Expect(os.WriteFile(caPath, firstCAPEM, 0o600)).To(Succeed())
	DeferCleanup(func() {
		server.Close()
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
	})

	rotate := func() {
		current.Store(&secondCert)
		Expect(os.WriteFile(caPath, secondCAPEM, 0o600)).To(Succeed())
	}
	return server, caPath, rotate
}

func newBogusCAPath() string {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).ToNot(HaveOccurred())
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(2540),
		Subject:               pkix.Name{CommonName: "issue-2540-wrong-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	Expect(err).ToNot(HaveOccurred())

	tmpDir, err := os.MkdirTemp("", "mcp-gateway-wrong-ca-2540-")
	Expect(err).ToNot(HaveOccurred())
	caPath := filepath.Join(tmpDir, "wrong-ca.crt")
	Expect(os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600)).To(Succeed())
	DeferCleanup(func() { Expect(os.RemoveAll(tmpDir)).To(Succeed()) })
	return caPath
}

func newTLSTokenServer(requests *atomic.Int32) (*httptest.Server, string) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"issue-2540-token","token_type":"Bearer","expires_in":3600}`))
	})
	server := httptest.NewUnstartedServer(handler)
	server.StartTLS()
	cert := server.Certificate()
	tmpDir, err := os.MkdirTemp("", "mcp-token-tls-2540-")
	Expect(err).ToNot(HaveOccurred())
	caPath := filepath.Join(tmpDir, "token-ca.crt")
	Expect(os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), 0o600)).To(Succeed())
	DeferCleanup(func() {
		server.Close()
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
	})
	return server, caPath
}

var _ = Describe("MCP Gateway scoped TLS trust (Issue #2540)", func() {
	logger := zap.New(zap.UseDevMode(true))

	It("UT-FLEET-2540-001 [BR-INTEGRATION-054, SC-8, ASVS-12.3.1]: preserves existing transport behavior when the gateway CA path is empty", func() {
		gateway, caPath := newMCPGatewayServer(false)
		Expect(caPath).To(BeEmpty())

		client, err := mcpclient.Connect(context.Background(), mcpclient.ConnectConfig{
			Endpoint:   gateway.URL,
			Resilience: fastGatewayResilience(),
		}, logger)
		Expect(err).ToNot(HaveOccurred())
		Expect(client).ToNot(BeNil())
		Expect(client.Ready()).To(BeTrue())
		DeferCleanup(func() { Expect(client.Close()).To(Succeed()) })
	})

	It("UT-FLEET-2540-002 [BR-INTEGRATION-054, SC-8, SC-12, SC-13, ASVS-12.3.2/12.3.4]: completes an HTTPS MCP handshake with the configured private CA", func() {
		gateway, caPath := newMCPGatewayServer(true)

		client, err := mcpclient.Connect(context.Background(), mcpclient.ConnectConfig{
			Endpoint:            gateway.URL,
			MCPGatewayTLSCAFile: caPath,
			Resilience:          fastGatewayResilience(),
		}, logger)
		Expect(err).ToNot(HaveOccurred())
		Expect(client).ToNot(BeNil())
		Expect(client.Ready()).To(BeTrue())
		DeferCleanup(func() { Expect(client.Close()).To(Succeed()) })
	})

	It("UT-FLEET-2540-003 [BR-INTEGRATION-054, SI-10, ASVS-12.3.2]: rejects a gateway signed by the wrong CA", func() {
		gateway, _ := newMCPGatewayServer(true)
		wrongCAPath := newBogusCAPath()

		client, err := mcpclient.Connect(context.Background(), mcpclient.ConnectConfig{
			Endpoint:            gateway.URL,
			MCPGatewayTLSCAFile: wrongCAPath,
			Resilience:          fastGatewayResilience(),
		}, logger)
		Expect(client).ToNot(BeNil())
		Expect(err).To(HaveOccurred())
		Expect(client.Ready()).To(BeFalse())
		DeferCleanup(func() { Expect(client.Close()).To(Succeed()) })
	})

	It("UT-FLEET-2540-004 [BR-INTEGRATION-054, SI-10, CM-6, ASVS-16.5.3]: fails closed for an unreadable gateway CA instead of falling back", func() {
		gateway, _ := newMCPGatewayServer(false)

		client, err := mcpclient.Connect(context.Background(), mcpclient.ConnectConfig{
			Endpoint:            gateway.URL,
			MCPGatewayTLSCAFile: "/nonexistent/mcp-gateway-ca.crt",
			Resilience:          fastGatewayResilience(),
		}, logger)
		Expect(client).ToNot(BeNil())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("mcp gateway tls ca"))
		Expect(client.Ready()).To(BeFalse())
		DeferCleanup(func() { Expect(client.Close()).To(Succeed()) })
	})

	It("UT-FLEET-2540-004b [BR-INTEGRATION-054, SI-10, CM-6, ASVS-16.5.3]: fails closed for malformed gateway CA PEM", func() {
		gateway, _ := newMCPGatewayServer(true)
		tmpDir, err := os.MkdirTemp("", "mcp-gateway-malformed-ca-2540-")
		Expect(err).ToNot(HaveOccurred())
		malformedCAPath := filepath.Join(tmpDir, "malformed-ca.crt")
		Expect(os.WriteFile(malformedCAPath, []byte("not a PEM certificate\n"), 0o600)).To(Succeed())
		DeferCleanup(func() { Expect(os.RemoveAll(tmpDir)).To(Succeed()) })

		client, err := mcpclient.Connect(context.Background(), mcpclient.ConnectConfig{
			Endpoint:            gateway.URL,
			MCPGatewayTLSCAFile: malformedCAPath,
			Resilience:          fastGatewayResilience(),
		}, logger)
		Expect(client).ToNot(BeNil())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("mcp gateway tls ca"))
		Expect(client.Ready()).To(BeFalse())
		DeferCleanup(func() { Expect(client.Close()).To(Succeed()) })
	})

	It("UT-FLEET-2540-005 [BR-INTEGRATION-054, BR-INTEGRATION-065, SC-8, ASVS-12.3.2/12.3.4]: keeps gateway and OAuth2 token CA trust independent", func() {
		gateway, gatewayCAPath := newMCPGatewayServer(true)
		var tokenRequests atomic.Int32
		tokenServer, tokenCAPath := newTLSTokenServer(&tokenRequests)

		credentialsDir, err := os.MkdirTemp("", "mcp-oauth2-2540-")
		Expect(err).ToNot(HaveOccurred())
		Expect(os.WriteFile(filepath.Join(credentialsDir, "client-id"), []byte("issue-2540-client"), 0o600)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(credentialsDir, "client-secret"), []byte("issue-2540-secret"), 0o600)).To(Succeed())
		DeferCleanup(func() { Expect(os.RemoveAll(credentialsDir)).To(Succeed()) })

		client, err := mcpclient.Connect(context.Background(), mcpclient.ConnectConfig{
			Endpoint:            gateway.URL,
			MCPGatewayTLSCAFile: gatewayCAPath,
			OAuth2: fleet.FleetOAuth2Config{
				Enabled:   true,
				TokenURL:  tokenServer.URL,
				TLSCAFile: tokenCAPath,
			},
			CredentialsBasePath: credentialsDir,
			Resilience:          fastGatewayResilience(),
		}, logger)
		Expect(err).ToNot(HaveOccurred())
		Expect(client.Ready()).To(BeTrue())
		Expect(tokenRequests.Load()).To(BeNumerically(">=", 1))
		DeferCleanup(func() { Expect(client.Close()).To(Succeed()) })
	})

	It("UT-FLEET-2540-006 [BR-INTEGRATION-054, SC-12, SC-13, ASVS-12.3.4]: reloads the configured CA before reconnecting", func() {
		gateway, caPath, rotate := newRotatingMCPGatewayServer()
		client, err := mcpclient.Connect(context.Background(), mcpclient.ConnectConfig{
			Endpoint:            gateway.URL,
			MCPGatewayTLSCAFile: caPath,
			Resilience:          fastGatewayResilience(),
		}, logger)
		Expect(err).ToNot(HaveOccurred())
		Expect(client.Ready()).To(BeTrue())
		DeferCleanup(func() { Expect(client.Close()).To(Succeed()) })

		readPod := func() error {
			pod := &unstructured.Unstructured{}
			pod.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"})
			if err := client.Get(context.Background(), k8sclient.ObjectKey{Name: "nginx"}, pod); err != nil {
				return err
			}
			if pod.GetName() != "nginx" {
				return fmt.Errorf("unexpected pod name %q", pod.GetName())
			}
			return nil
		}
		Expect(readPod()).To(Succeed())

		rotate()
		Eventually(readPod, 5*time.Second, 100*time.Millisecond).Should(Succeed())
		Eventually(func() error {
			return client.Reconnect(context.Background())
		}, 5*time.Second, 100*time.Millisecond).Should(Succeed())
		Expect(client.Ready()).To(BeTrue())
	})
})
