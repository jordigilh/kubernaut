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

package server_test

// GAP-14 / Issue #1519: proves Kubernaut Agent's real production dispatch
// path -- the actual /api/v1 route group main.go builds (real
// otelhttp.NewMiddleware, real rate limiter, real auth.Middleware/
// JWTAuthenticator, real kaserver.NewHandler) -- exports a span over a real
// TLS connection to a collector, driven by a real JWT-authenticated
// request. The router assembly here mirrors cmd/kubernautagent/main.go's
// r.Route("/api/v1", ...) block line-for-line for the pieces relevant to
// this proof; auth uses the SAME pattern as the package's existing
// jwt_middleware_test.go (real auth.Middleware + real auth.JWTAuthenticator
// verified against a real mock JWKS server -- only the K8s SAR/TokenReview
// leg is test-doubled, consistent with this codebase's established
// convention of mocking the Kubernetes API as the external dependency,
// while keeping the auth business logic real).

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
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	oteltrace "go.opentelemetry.io/otel/trace"

	internalconfig "github.com/jordigilh/kubernaut/internal/config"
	kaaudit "github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	kaserver "github.com/jordigilh/kubernaut/internal/kubernautagent/server"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	"github.com/jordigilh/kubernaut/pkg/agentclient"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
	sharedtelemetry "github.com/jordigilh/kubernaut/pkg/shared/telemetry"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
)

var _ = Describe("Kubernaut Agent production wiring: OTel export over TLS (GAP-14 / Issue #1519)", func() {
	var (
		certDir      string
		caCertPath   string
		caKey        *ecdsa.PrivateKey
		caCert       *x509.Certificate
		prevProvider oteltrace.TracerProvider
		mockJWKS     *testauth.MockJWKSServer
	)

	BeforeEach(func() {
		var err error
		certDir, err = os.MkdirTemp("", "ka-otel-tls-it-*")
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

		mockJWKS, err = testauth.NewMockJWKSServer("https://keycloak.example.com/realms/kubernaut")
		Expect(err).ToNot(HaveOccurred())

		// This suite runs multiple spec files in one process; otel's global
		// TracerProvider is process-wide state. Save/restore it so this
		// test's registration can't leak into unrelated specs.
		prevProvider = otel.GetTracerProvider()
	})

	AfterEach(func() {
		otel.SetTracerProvider(prevProvider)
		if mockJWKS != nil {
			mockJWKS.Close()
		}
		_ = os.RemoveAll(certDir)
	})

	startMockCollector := func() (srv *http.Server, addr string, received *atomic.Int64) {
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
		srvCertDER, err := x509.CreateCertificate(rand.Reader, srvTemplate, caCert, &srvKey.PublicKey, caKey)
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

		srv = &http.Server{Handler: mux}
		go func() {
			defer GinkgoRecover()
			_ = srv.Serve(listener)
		}()

		return srv, listener.Addr().String(), received
	}

	// newProductionShapedRouter mirrors cmd/kubernautagent/main.go's
	// r.Route("/api/v1", ...) block: real otelhttp.NewMiddleware, real
	// HTTPMetricsMiddleware, real rate limiter, real auth.Middleware
	// (wrapped in AuditAuthMiddleware exactly as production does), then the
	// real kaserver.NewHandler/agentclient.NewServer business handler --
	// same construction newTestAPIServer/newTestHarness use elsewhere in
	// this package, just assembled with the production middleware chain
	// instead of the test-only shortcuts those helpers use.
	newProductionShapedRouter := func(authMw *auth.Middleware) *chi.Mux {
		store := session.NewStore(24 * time.Hour)
		mgr := session.NewManager(store, logr.Discard(), nil, nil)
		inv := &stubInvestigator{}
		handler := kaserver.NewHandler(mgr, inv, logr.Discard(), nil)
		ogenSrv, err := agentclient.NewServer(handler)
		Expect(err).ToNot(HaveOccurred())

		r := chi.NewRouter()
		r.Route("/api/v1", func(r chi.Router) {
			r.Use(otelhttp.NewMiddleware("kubernautagent.http",
				otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
					return r.Method + " " + r.URL.Path
				}),
			))
			r.Use(kaserver.HTTPMetricsMiddleware(nil))
			rl := kaserver.NewRateLimiter(kaserver.DefaultRateLimitConfig(), nil)
			r.Use(rl.Middleware)
			r.Use(func(next http.Handler) http.Handler {
				return kaserver.AuditAuthMiddleware(authMw.Handler(next), kaaudit.NopAuditStore{}, logr.Discard())
			})
			r.Handle("/*", kaserver.SSEHeadersMiddleware(ogenSrv))
		})
		return r
	}

	// IT-1519-005: the real production dispatch path -- KA's actual
	// /api/v1 route group (real otelhttp middleware, real rate limiter,
	// real auth.Middleware verifying a real signed JWT via a real mock JWKS
	// server, real kaserver.NewHandler), driven by a real authenticated
	// POST -- produces a span exported over a real TLS connection to the
	// collector.
	It("IT-1519-005: exports a span over TLS from a real JWT-authenticated request through KA's production route group", func() {
		collector, collectorAddr, received := startMockCollector()
		defer func() { _ = collector.Close() }()

		// Verbatim copy of cmd/kubernautagent/main.go's bootstrap call.
		shutdown, err := sharedtelemetry.NewTracerProvider(context.Background(), sharedtelemetry.Config{
			ServiceName: "kubernaut-agent",
			Endpoint:    collectorAddr,
			TLS: internalconfig.TelemetryTLSConfig{
				Enabled: true,
				CAFile:  caCertPath,
			},
		})
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = shutdown(context.Background()) }()

		// Same shape as cmd/kubernautagent/main.go's newAuthMiddleware:
		// CompositeAuthenticator(JWT, K8s) -- only the K8s leg is a test
		// double, matching this package's existing jwt_middleware_test.go
		// convention.
		jwtAuth, err := auth.NewJWTAuthenticator([]auth.JWTProviderEntry{{
			Issuer:        mockJWKS.Issuer,
			JWKSURL:       mockJWKS.JWKSURL(),
			Audience:      "kubernaut-agent",
			UsernameClaim: "preferred_username",
			GroupsClaim:   "groups",
		}}, logr.Discard())
		Expect(err).ToNot(HaveOccurred())
		defer jwtAuth.Close()

		mockK8sAuth := &auth.MockAuthenticator{}
		composite := auth.NewCompositeAuthenticator(jwtAuth, mockK8sAuth)
		mockAuthz := &auth.MockAuthorizer{
			AllowedUsers: map[string]bool{"otel-wiring-user": true},
		}
		authMw := auth.NewMiddleware(composite, mockAuthz, auth.MiddlewareConfig{
			Namespace:    "kubernaut-system",
			Resource:     "services",
			ResourceName: "kubernaut-agent",
			Verb:         "create",
		}, logr.Discard())

		r := newProductionShapedRouter(authMw)
		ts := httptest.NewServer(r)
		defer ts.Close()

		token, err := mockJWKS.IssueJWT("otel-wiring-user", []string{"kubernaut-users"}, "kubernaut-agent", time.Hour)
		Expect(err).ToNot(HaveOccurred())

		req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/incident/analyze", strings.NewReader(validIncidentJSON()))
		Expect(err).ToNot(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := http.DefaultClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusAccepted),
			"a real JWT-authenticated incident analysis request must be accepted (proves the real auth + business pipeline ran, not just an auth-rejected request)")

		Expect(shutdown(context.Background())).To(Succeed())

		Eventually(func() int64 { return received.Load() }, "10s", "100ms").Should(
			BeNumerically(">=", 1),
			"BR-OTEL-1519: a real JWT-authenticated request through KA's production route group must produce a span delivered to the OTLP collector over TLS",
		)
	})
})
