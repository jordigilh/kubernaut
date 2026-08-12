/*
Copyright 2025 Jordi Gil.

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

package gateway

// GAP-14 / Issue #1519: unlike test/integration/shared/telemetry's IT-1519-001/002
// (which prove pkg/shared/telemetry.NewTracerProvider's TLS behavior against a
// hand-wired otelhttp.NewMiddleware call), THIS test proves the actual
// production dispatch path: the real gateway.Server constructed the same way
// cmd/gateway/main.go constructs it, its real setupRoutes()/Handler() (chi
// router, security headers, CORS, otelhttp middleware, adapter parsing, real
// envtest K8s CRD creation, real DataStorage audit emission -- the whole
// chain), driven by a real webhook HTTP request, exporting a real span over
// TLS to a mock collector. No construct in this test re-implements Gateway's
// wiring; StartTestGateway/createGatewayServer are the same helpers every
// other Gateway IT test in this package uses.

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
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
	"go.opentelemetry.io/otel"
	oteltrace "go.opentelemetry.io/otel/trace"

	internalconfig "github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	sharedtelemetry "github.com/jordigilh/kubernaut/pkg/shared/telemetry"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

var _ = Describe("Gateway production wiring: OTel export over TLS (GAP-14 / Issue #1519)", func() {
	var (
		certDir      string
		caCertPath   string
		caKey        *ecdsa.PrivateKey
		caCert       *x509.Certificate
		prevProvider oteltrace.TracerProvider
	)

	BeforeEach(func() {
		var err error
		certDir, err = os.MkdirTemp("", "gw-otel-tls-it-*")
		Expect(err).ToNot(HaveOccurred())

		caKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		Expect(err).ToNot(HaveOccurred())
		caTemplate := &x509.Certificate{
			SerialNumber:          big.NewInt(1),
			Subject:               pkix.Name{CommonName: "Test Collector CA"},
			NotBefore:             time.Now().Add(-1 * time.Hour),
			NotAfter:              time.Now().Add(24 * time.Hour),
			IsCA:                  true,
			BasicConstraintsValid: true,
			KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		}
		caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
		Expect(err).ToNot(HaveOccurred())
		caCert, err = x509.ParseCertificate(caCertDER)
		Expect(err).ToNot(HaveOccurred())

		caCertPath = filepath.Join(certDir, "ca.crt")
		Expect(os.WriteFile(caCertPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertDER}), 0644)).To(Succeed())

		// This suite runs 25+ spec files in one process; otel's global
		// TracerProvider is process-wide state. Save/restore it so this
		// test's registration can't leak into unrelated specs.
		prevProvider = otel.GetTracerProvider()
	})

	AfterEach(func() {
		otel.SetTracerProvider(prevProvider)
		_ = os.RemoveAll(certDir)
	})

	// startMockCollectorSignedBy starts a real TLS listener presenting a
	// cert signed by signingCA/signingKey, standing in for an OTLP/HTTP
	// collector. Parameterized (rather than always signing with the
	// suite's caKey/caCert) so IT-1519-009 can reuse this same helper with
	// a deliberately untrusted CA.
	startMockCollectorSignedBy := func(signingKey *ecdsa.PrivateKey, signingCA *x509.Certificate) (server *http.Server, addr string, received *atomic.Int64) {
		received = &atomic.Int64{}

		srvKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		Expect(err).ToNot(HaveOccurred())
		srvTemplate := &x509.Certificate{
			SerialNumber: big.NewInt(2),
			Subject:      pkix.Name{CommonName: "localhost"},
			NotBefore:    time.Now().Add(-1 * time.Hour),
			NotAfter:     time.Now().Add(24 * time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
			DNSNames:     []string{"localhost"},
		}
		srvCertDER, err := x509.CreateCertificate(rand.Reader, srvTemplate, signingCA, &srvKey.PublicKey, signingKey)
		Expect(err).ToNot(HaveOccurred())
		srvKeyDER, err := x509.MarshalECPrivateKey(srvKey)
		Expect(err).ToNot(HaveOccurred())

		tlsCert, err := tls.X509KeyPair(
			pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srvCertDER}),
			pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: srvKeyDER}),
		)
		Expect(err).ToNot(HaveOccurred())

		mux := http.NewServeMux()
		mux.HandleFunc("/v1/traces", func(w http.ResponseWriter, r *http.Request) {
			received.Add(1)
			w.WriteHeader(http.StatusOK)
		})

		listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
			Certificates: []tls.Certificate{tlsCert},
			MinVersion:   tls.VersionTLS12,
		})
		Expect(err).ToNot(HaveOccurred())

		server = &http.Server{Handler: mux}
		go func() {
			defer GinkgoRecover()
			_ = server.Serve(listener)
		}()

		return server, listener.Addr().String(), received
	}

	// IT-1519-003: the real production dispatch path -- Gateway's actual
	// setupRoutes()/Handler(), constructed with the same config shape
	// cmd/gateway/main.go uses (ServerConfig.Telemetry -> telemetry.Config,
	// verbatim), driven by a real webhook POST that traverses the real
	// adapter/dedup/CRD-creation/audit pipeline against real envtest K8s and
	// real DataStorage -- produces a span that is exported over a real TLS
	// connection to the collector.
	It("IT-1519-003: exports a span over TLS from a real webhook request through Gateway's production Handler()", func() {
		collector, collectorAddr, received := startMockCollectorSignedBy(caKey, caCert)
		defer func() { _ = collector.Close() }()

		dataStorageURL := fmt.Sprintf("http://127.0.0.1:%d", infrastructure.GatewayIntegrationDataStoragePort)
		cfg := createGatewayConfig(dataStorageURL)
		// Same shape as an operator's YAML: ServerConfig.Telemetry, the
		// exact field cmd/gateway/main.go reads from.
		cfg.Telemetry = internalconfig.TelemetryConfig{
			Endpoint: collectorAddr,
			TLS: internalconfig.TelemetryTLSConfig{
				Enabled: true,
				CAFile:  caCertPath,
			},
		}

		// Verbatim copy of cmd/gateway/main.go's bootstrap call.
		shutdown, err := sharedtelemetry.NewTracerProvider(context.Background(), sharedtelemetry.Config{
			ServiceName: "gateway",
			Endpoint:    cfg.Telemetry.Endpoint,
			TLS:         cfg.Telemetry.TLS,
			LogSink:     cfg.Telemetry.LogSink,
		})
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = shutdown(context.Background()) }()

		testNamespace := fmt.Sprintf("test-otel-wiring-%d", time.Now().UnixNano())
		EnsureTestNamespace(ctx, &K8sTestClient{Client: k8sClient}, testNamespace)

		gatewayServer, err := createGatewayServer(cfg, logger, k8sClient, sharedAuditStore)
		Expect(err).ToNot(HaveOccurred())

		// Routes are registered dynamically via RegisterAdapter (see
		// server.go's setupRoutes() doc comment) -- mirrors the same call
		// StartTestGatewayWithOptions makes for every other Gateway IT test.
		prometheusAdapter := adapters.NewPrometheusAdapter(nil, adapters.NewTestAPIResourceRegistry())
		Expect(gatewayServer.RegisterAdapter(prometheusAdapter)).To(Succeed())

		// Real production router -- setupRoutes() + wrapWithMiddleware(),
		// identical to what http.Server.Handler serves in production.
		testServer := httptest.NewServer(gatewayServer.Handler())
		defer testServer.Close()

		alertPayload := createPrometheusAlert(testNamespace, "OTelWiringTestAlert", "critical", "", "")
		webhookResp := SendWebhookWithAuth(testServer.URL+"/api/v1/signals/prometheus", alertPayload, suiteAuthToken)
		Expect(webhookResp.StatusCode).To(Equal(http.StatusCreated),
			"an authorized webhook for a managed namespace must be accepted and create a RemediationRequest (proves the real business pipeline ran, not just an auth-rejected request)")

		Expect(shutdown(context.Background())).To(Succeed())

		Eventually(func() int64 { return received.Load() }, "10s", "100ms").Should(
			BeNumerically(">=", 1),
			"BR-OTEL-1519: a real webhook request through Gateway's production Handler() must produce a span delivered to the OTLP collector over TLS",
		)
	})

	// IT-1519-009: the fail-closed control (test/integration/shared/telemetry's
	// IT-1519-002) proven through Gateway's REAL production Handler(), not
	// just the hand-wired otelhttp.NewMiddleware call. Two things must both
	// hold: (1) the business pipeline (webhook -> RemediationRequest) must
	// succeed regardless of the collector's TLS trust status -- OTel is a
	// side channel, never a dependency of the request path -- and (2) an
	// untrusted collector must never receive a span, even when the span
	// originates from the full real dispatch chain (auth, adapters, CRD
	// creation, audit) rather than a synthetic handler.
	It("IT-1519-009: a real webhook request through Gateway's production Handler() still succeeds, and never delivers a span, when the collector's certificate is untrusted", func() {
		wrongKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		Expect(err).ToNot(HaveOccurred())
		wrongCATemplate := &x509.Certificate{
			SerialNumber:          big.NewInt(99),
			Subject:               pkix.Name{CommonName: "Untrusted CA"},
			NotBefore:             time.Now().Add(-1 * time.Hour),
			NotAfter:              time.Now().Add(24 * time.Hour),
			IsCA:                  true,
			BasicConstraintsValid: true,
			KeyUsage:              x509.KeyUsageCertSign,
		}
		wrongCADER, err := x509.CreateCertificate(rand.Reader, wrongCATemplate, wrongCATemplate, &wrongKey.PublicKey, wrongKey)
		Expect(err).ToNot(HaveOccurred())
		wrongCA, err := x509.ParseCertificate(wrongCADER)
		Expect(err).ToNot(HaveOccurred())

		// Collector's cert is signed by the UNTRUSTED CA; Gateway is
		// configured to trust caCertPath (the OTHER CA) -- simulates an
		// attacker-controlled or misconfigured endpoint.
		collector, collectorAddr, received := startMockCollectorSignedBy(wrongKey, wrongCA)
		defer func() { _ = collector.Close() }()

		dataStorageURL := fmt.Sprintf("http://127.0.0.1:%d", infrastructure.GatewayIntegrationDataStoragePort)
		cfg := createGatewayConfig(dataStorageURL)
		cfg.Telemetry = internalconfig.TelemetryConfig{
			Endpoint: collectorAddr,
			TLS: internalconfig.TelemetryTLSConfig{
				Enabled: true,
				CAFile:  caCertPath,
			},
		}

		shutdown, err := sharedtelemetry.NewTracerProvider(context.Background(), sharedtelemetry.Config{
			ServiceName: "gateway",
			Endpoint:    cfg.Telemetry.Endpoint,
			TLS:         cfg.Telemetry.TLS,
			LogSink:     cfg.Telemetry.LogSink,
		})
		Expect(err).ToNot(HaveOccurred())
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = shutdown(shutdownCtx)
		}()

		testNamespace := fmt.Sprintf("test-otel-wiring-%d", time.Now().UnixNano())
		EnsureTestNamespace(ctx, &K8sTestClient{Client: k8sClient}, testNamespace)

		gatewayServer, err := createGatewayServer(cfg, logger, k8sClient, sharedAuditStore)
		Expect(err).ToNot(HaveOccurred())
		prometheusAdapter := adapters.NewPrometheusAdapter(nil, adapters.NewTestAPIResourceRegistry())
		Expect(gatewayServer.RegisterAdapter(prometheusAdapter)).To(Succeed())

		testServer := httptest.NewServer(gatewayServer.Handler())
		defer testServer.Close()

		alertPayload := createPrometheusAlert(testNamespace, "OTelFailClosedTestAlert", "critical", "", "")
		webhookResp := SendWebhookWithAuth(testServer.URL+"/api/v1/signals/prometheus", alertPayload, suiteAuthToken)
		Expect(webhookResp.StatusCode).To(Equal(http.StatusCreated),
			"BR-OTEL-1519: an untrusted OTel collector must never block or fail the business pipeline -- tracing is a side channel")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = shutdown(shutdownCtx) // export/handshake failure is expected and swallowed by the SDK; asserted via the collector below

		Consistently(func() int64 { return received.Load() }, "1s", "100ms").Should(
			Equal(int64(0)),
			"BR-OTEL-1519: an untrusted collector certificate must never receive an exported span, even from Gateway's real production Handler()",
		)
	})

	// IT-1519-010: resilience control validation. If the configured OTLP
	// collector is unreachable or unresponsive, the request path must never
	// be blocked or materially delayed -- otherwise enabling tracing would
	// itself become an availability risk. The batch span processor exports
	// asynchronously, decoupled from the HTTP response; this test proves
	// that architectural property holds through the real production
	// Handler(), not just in principle.
	It("IT-1519-010: a real webhook request through Gateway's production Handler() completes promptly even when the OTLP collector never responds", func() {
		// A raw listener that accepts TCP connections but never completes
		// a TLS handshake or writes anything back -- simulates a hung/
		// unresponsive collector (as opposed to IT-1519-009's collector,
		// which responds immediately but is untrusted).
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = listener.Close() }()
		go func() {
			for {
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				_ = conn // deliberately never read/write/close
			}
		}()

		dataStorageURL := fmt.Sprintf("http://127.0.0.1:%d", infrastructure.GatewayIntegrationDataStoragePort)
		cfg := createGatewayConfig(dataStorageURL)
		cfg.Telemetry = internalconfig.TelemetryConfig{
			Endpoint: listener.Addr().String(),
			TLS: internalconfig.TelemetryTLSConfig{
				Enabled: true,
				CAFile:  caCertPath,
			},
		}

		shutdown, err := sharedtelemetry.NewTracerProvider(context.Background(), sharedtelemetry.Config{
			ServiceName: "gateway",
			Endpoint:    cfg.Telemetry.Endpoint,
			TLS:         cfg.Telemetry.TLS,
			LogSink:     cfg.Telemetry.LogSink,
		})
		Expect(err).ToNot(HaveOccurred())
		defer func() {
			// Bounded on its own -- a hung collector must not be allowed
			// to hang this test's cleanup either.
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = shutdown(shutdownCtx)
		}()

		testNamespace := fmt.Sprintf("test-otel-wiring-%d", time.Now().UnixNano())
		EnsureTestNamespace(ctx, &K8sTestClient{Client: k8sClient}, testNamespace)

		gatewayServer, err := createGatewayServer(cfg, logger, k8sClient, sharedAuditStore)
		Expect(err).ToNot(HaveOccurred())
		prometheusAdapter := adapters.NewPrometheusAdapter(nil, adapters.NewTestAPIResourceRegistry())
		Expect(gatewayServer.RegisterAdapter(prometheusAdapter)).To(Succeed())

		testServer := httptest.NewServer(gatewayServer.Handler())
		defer testServer.Close()

		alertPayload := createPrometheusAlert(testNamespace, "OTelResilienceTestAlert", "critical", "", "")

		start := time.Now()
		webhookResp := SendWebhookWithAuth(testServer.URL+"/api/v1/signals/prometheus", alertPayload, suiteAuthToken)
		elapsed := time.Since(start)

		Expect(webhookResp.StatusCode).To(Equal(http.StatusCreated),
			"the business pipeline must succeed even though the configured OTel collector is completely unresponsive")
		Expect(elapsed).To(BeNumerically("<", 5*time.Second),
			"BR-OTEL-1519: an unresponsive OTLP collector must never block or materially delay the request path -- span export is asynchronous and decoupled from the response")
	})
})
