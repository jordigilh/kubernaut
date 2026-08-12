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

package datastorage

// GAP-14 / Issue #1519: proves Data Storage's real production dispatch path
// -- server.NewServer's real Handler() (chi router, otelhttp middleware,
// real DD-AUTH-014 auth, real adapter/repository code) -- exports a span
// over a real TLS connection to a collector, driven by a real authenticated
// HTTP GET that reads real seeded data from Postgres. Mirrors
// test/integration/gateway/otel_tls_wiring_integration_test.go's approach:
// no construct here re-implements Data Storage's wiring; newXxxTestServer-
// style helpers are the same pattern every other IT test in this package
// uses, and the request is copied from context_propagation_test.go's
// existing IT-DS-042-003 pattern.

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

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.opentelemetry.io/otel"
	oteltrace "go.opentelemetry.io/otel/trace"

	internalconfig "github.com/jordigilh/kubernaut/internal/config"
	dsconfig "github.com/jordigilh/kubernaut/pkg/datastorage/config"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
	sharedtelemetry "github.com/jordigilh/kubernaut/pkg/shared/telemetry"
)

var _ = Describe("Data Storage production wiring: OTel export over TLS (GAP-14 / Issue #1519)", func() {
	var (
		certDir      string
		caCertPath   string
		caKey        *ecdsa.PrivateKey
		caCert       *x509.Certificate
		prevProvider oteltrace.TracerProvider
		testID       string
	)

	BeforeEach(func() {
		testID = generateTestID()

		var err error
		certDir, err = os.MkdirTemp("", "ds-otel-tls-it-*")
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

		// This suite runs many spec files in one process; otel's global
		// TracerProvider is process-wide state. Save/restore it so this
		// test's registration can't leak into unrelated specs.
		prevProvider = otel.GetTracerProvider()
	})

	AfterEach(func() {
		otel.SetTracerProvider(prevProvider)
		_ = os.RemoveAll(certDir)
		_, _ = db.ExecContext(context.Background(),
			"DELETE FROM audit_events WHERE correlation_id LIKE $1",
			fmt.Sprintf("%%otel-wiring-%s%%", testID))
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

	// newOTelWiringTestServer mirrors newContextPropagationTestServer /
	// newBatchLimitTestServer exactly, except appCfg.Telemetry is populated
	// (the field cmd/datastorage/main.go reads its bootstrap call from) so
	// the config-to-bootstrap shape matches production, and the bootstrap
	// call itself is a verbatim copy of that main.go call.
	newOTelWiringTestServer := func(collectorAddr string) *httptest.Server {
		pgHost := os.Getenv("POSTGRES_HOST")
		if pgHost == "" {
			pgHost = "localhost"
		}
		pgPort := os.Getenv("POSTGRES_PORT")
		if pgPort == "" {
			pgPort = "15433"
		}
		redisHost := os.Getenv("REDIS_HOST")
		if redisHost == "" {
			redisHost = "localhost"
		}
		redisPort := os.Getenv("REDIS_PORT")
		if redisPort == "" {
			redisPort = "16379"
		}

		dbConnStr := fmt.Sprintf(
			"host=%s port=%s user=slm_user password=test_password dbname=action_history sslmode=disable options='-c search_path=public'",
			pgHost, pgPort,
		)
		redisAddr := fmt.Sprintf("%s:%s", redisHost, redisPort)

		appCfg := &dsconfig.Config{
			Server: dsconfig.ServerConfig{
				SignerCertDir: datastorageIntegrationSigningCertDirOrDie(),
			},
			Database: dsconfig.DatabaseConfig{
				MaxOpenConns:    5,
				MaxIdleConns:    2,
				ConnMaxLifetime: "1m",
				ConnMaxIdleTime: "1m",
			},
			// Same shape as an operator's YAML: Config.Telemetry, the exact
			// field cmd/datastorage/main.go reads from.
			Telemetry: internalconfig.TelemetryConfig{
				Endpoint: collectorAddr,
				TLS: internalconfig.TelemetryTLSConfig{
					Enabled: true,
					CAFile:  caCertPath,
				},
			},
		}

		const testToken = "otel-wiring-test-token"
		const testUser = "system:serviceaccount:test:otel-wiring"

		srv, err := server.NewServer(server.ServerDeps{
			DBConnStr:     dbConnStr,
			RedisAddr:     redisAddr,
			RedisPassword: "",
			Logger:        logger,
			AppConfig:     appCfg,
			ServerConfig: &server.Config{
				Port:         18090,
				ReadTimeout:  30 * time.Second,
				WriteTimeout: 30 * time.Second,
			},
			DLQMaxLen: 100,
			Authenticator: &auth.MockAuthenticator{
				ValidUsers: map[string]string{
					testToken: testUser,
				},
			},
			Authorizer: &auth.MockAuthorizer{
				AllowedUsers: map[string]bool{
					testUser: true,
				},
			},
			AuthNamespace: "test",
		})
		Expect(err).ToNot(HaveOccurred())

		return httptest.NewServer(srv.Handler())
	}

	// IT-1519-004: the real production dispatch path -- Data Storage's
	// actual Handler() (chi router, otelhttp middleware, real DD-AUTH-014
	// auth, real repository/Postgres read), driven by a real authenticated
	// GET against seeded data -- produces a span exported over a real TLS
	// connection to the collector.
	It("IT-1519-004: exports a span over TLS from a real authenticated request through Data Storage's production Handler()", func() {
		collector, collectorAddr, received := startMockCollector()
		defer func() { _ = collector.Close() }()

		// Verbatim copy of cmd/datastorage/main.go's bootstrap call.
		shutdown, err := sharedtelemetry.NewTracerProvider(context.Background(), sharedtelemetry.Config{
			ServiceName: "datastorage",
			Endpoint:    collectorAddr,
			TLS: internalconfig.TelemetryTLSConfig{
				Enabled: true,
				CAFile:  caCertPath,
			},
		})
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = shutdown(context.Background()) }()

		corrID := fmt.Sprintf("otel-wiring-%s", testID)
		auditRepo := repository.NewAuditEventsRepository(db.DB, logger)
		evt := &repository.AuditEvent{
			EventID:       uuid.New(),
			EventType:     "effectiveness.health_assessed",
			Version:       "1.0",
			EventCategory: "effectiveness",
			EventAction:   "assess",
			EventOutcome:  "success",
			CorrelationID: corrID,
			ResourceType:  "deployment",
			ResourceID:    "otel-wiring-test-deploy",
			ActorID:       "system",
			ActorType:     "service",
			RetentionDays: 30,
			EventData:     map[string]interface{}{"score": 0.9, "event_type": "effectiveness.health_assessed"},
		}
		_, err = auditRepo.Create(context.Background(), evt)
		Expect(err).ToNot(HaveOccurred(), "seed event must be created")

		ts := newOTelWiringTestServer(collectorAddr)
		defer ts.Close()

		reqCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet,
			fmt.Sprintf("%s/api/v1/effectiveness/%s", ts.URL, corrID), nil)
		Expect(err).ToNot(HaveOccurred())
		req.Header.Set("Authorization", "Bearer otel-wiring-test-token")

		resp, err := http.DefaultClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK),
			"authenticated effectiveness read for seeded data must succeed (proves the real business pipeline ran, not just an auth-rejected request)")

		Expect(shutdown(context.Background())).To(Succeed())

		Eventually(func() int64 { return received.Load() }, "10s", "100ms").Should(
			BeNumerically(">=", 1),
			"BR-OTEL-1519: a real authenticated request through Data Storage's production Handler() must produce a span delivered to the OTLP collector over TLS",
		)
	})
})
