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

// Package telemetry contains integration tests for GAP-14 / Issue #1519's
// OTel TLS wiring: pkg/shared/telemetry.NewTracerProvider is the exact
// production bootstrap function called by cmd/gateway/main.go,
// cmd/datastorage/main.go, and cmd/kubernautagent/main.go. DD-OTEL-001's
// Wiring Manifest marked that bootstrap call "Manual/build verification
// (bootstrap-only, no branching logic to unit test)" -- true when it was
// written, but TLS support added real branching (Enabled/CAFile/CertFile/
// KeyFile) after that. pkg/shared/telemetry/tls_internal_test.go covers the
// branching logic at the unit level (buildTLSConfig); this suite proves the
// wiring: a real span, produced by the same otelhttp.NewMiddleware call
// production servers use, actually crosses a real TLS-encrypted network
// connection to a collector and is rejected when the collector's
// certificate isn't trusted (SC-8-style transmission-confidentiality
// fail-closed behavior, not just a config flag with no runtime effect).
package telemetry

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
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
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"

	internalconfig "github.com/jordigilh/kubernaut/internal/config"
	sharedtelemetry "github.com/jordigilh/kubernaut/pkg/shared/telemetry"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
)

var _ = Describe("OTel OTLP/HTTPS wiring (GAP-14 / Issue #1519)", func() {
	var (
		certDir    string
		caCertPath string
		caKey      *ecdsa.PrivateKey
		caCert     *x509.Certificate
	)

	BeforeEach(func() {
		var err error
		certDir, err = os.MkdirTemp("", "otel-tls-it-*")
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
	})

	AfterEach(func() {
		_ = os.RemoveAll(certDir)
	})

	// startMockCollector starts a real TLS listener presenting a cert signed
	// by signingCA/signingKey, standing in for an OTLP/HTTP collector. It
	// returns the collector's host:port (no scheme, matching
	// otlptracehttp.WithEndpoint's expected format) and an atomic counter of
	// POSTs received on the OTLP traces path.
	startMockCollector := func(signingKey *ecdsa.PrivateKey, signingCA *x509.Certificate) (server *http.Server, addr string, received *atomic.Int64) {
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
		// otlptracehttp's default OTLP/HTTP path -- any POST here is a real
		// span-export attempt, not a health check or unrelated traffic.
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

	// productionAppHandler wires the SAME otelhttp.NewMiddleware call used in
	// pkg/gateway/server.go's setupRoutes() and
	// pkg/datastorage/server/server.go's Handler() -- proving the span this
	// test exercises is created the same way production inbound spans are,
	// not a hand-rolled tracer.Start() call.
	productionAppHandler := func() http.Handler {
		inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		return otelhttp.NewMiddleware("gateway.http",
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				return r.Method + " " + r.URL.Path
			}),
		)(inner)
	}

	// IT-1519-001: proves the production bootstrap wiring end-to-end -- a
	// span created by the real otelhttp inbound middleware, exported through
	// the real telemetry.NewTracerProvider (identical call to all 3
	// services' main.go), actually crosses an encrypted TLS connection and
	// is received by the collector as a real OTLP export POST.
	It("IT-1519-001: delivers a real span over TLS to the collector when the CA is trusted", func() {
		collector, collectorAddr, received := startMockCollector(caKey, caCert)
		defer func() { _ = collector.Close() }()

		ctx := context.Background()
		shutdown, err := sharedtelemetry.NewTracerProvider(ctx, sharedtelemetry.Config{
			ServiceName: "gateway-it-1519",
			Endpoint:    collectorAddr,
			TLS: internalconfig.TelemetryTLSConfig{
				Enabled: true,
				CAFile:  caCertPath,
			},
		})
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = shutdown(context.Background()) }()

		appServer := httptest.NewServer(productionAppHandler())
		defer appServer.Close()

		resp, err := http.Get(appServer.URL + "/api/v1/signals/prometheus")
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.Body.Close()).To(Succeed())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		// Shutdown ForceFlushes the batch span processor -- the export
		// should already be in flight/complete by the time it returns.
		Expect(shutdown(context.Background())).To(Succeed())

		Eventually(func() int64 { return received.Load() }, "5s", "50ms").Should(
			BeNumerically(">=", 1),
			"BR-OTEL-1519: a span from the production otelhttp middleware must reach the OTLP collector over TLS",
		)
	})

	// IT-1519-002: fail-closed control validation. If the collector's
	// certificate isn't signed by the configured CA, the exporter's TLS
	// handshake must fail closed -- no span (and no other traffic) may
	// reach an unverified endpoint. This is the runtime behavior a
	// transmission-confidentiality control objective (e.g. SC-8) depends
	// on: enabling TLS is only meaningful if untrusted endpoints are
	// actually rejected, not merely accepted with a flag set.
	It("IT-1519-002: never delivers a span when the collector's certificate is untrusted (fail-closed)", func() {
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

		// Collector's cert is signed by the UNTRUSTED CA, not caCert --
		// simulates an attacker-controlled or misconfigured endpoint.
		collector, collectorAddr, received := startMockCollector(wrongKey, wrongCA)
		defer func() { _ = collector.Close() }()

		ctx := context.Background()
		shutdown, err := sharedtelemetry.NewTracerProvider(ctx, sharedtelemetry.Config{
			ServiceName: "gateway-it-1519",
			Endpoint:    collectorAddr,
			TLS: internalconfig.TelemetryTLSConfig{
				Enabled: true,
				CAFile:  caCertPath, // trusts the OTHER CA, not the one that signed the collector's cert
			},
		})
		Expect(err).ToNot(HaveOccurred())
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = shutdown(shutdownCtx)
		}()

		appServer := httptest.NewServer(productionAppHandler())
		defer appServer.Close()

		resp, err := http.Get(appServer.URL + "/api/v1/signals/prometheus")
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.Body.Close()).To(Succeed())

		// Bound the flush attempt -- the failed handshake must not hang
		// the shutdown path indefinitely.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = shutdown(shutdownCtx) // export error is expected and swallowed by the SDK; asserted via the collector below

		Consistently(func() int64 { return received.Load() }, "1s", "100ms").Should(
			Equal(int64(0)),
			"BR-OTEL-1519: an untrusted collector certificate must never receive an exported span",
		)
	})

	// issueClientCert signs a client-auth certificate (ExtKeyUsageClientAuth)
	// with signingCA/signingKey and writes cert+key PEM files under certDir.
	// Reused only by the mTLS test below.
	issueClientCert := func(signingKey *ecdsa.PrivateKey, signingCA *x509.Certificate) (certPath, keyPath string) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		Expect(err).ToNot(HaveOccurred())
		template := &x509.Certificate{
			SerialNumber: big.NewInt(3),
			Subject:      pkix.Name{CommonName: "otel-client-it-1519"},
			NotBefore:    time.Now().Add(-1 * time.Hour),
			NotAfter:     time.Now().Add(24 * time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		}
		certDER, err := x509.CreateCertificate(rand.Reader, template, signingCA, &key.PublicKey, signingKey)
		Expect(err).ToNot(HaveOccurred())
		keyDER, err := x509.MarshalECPrivateKey(key)
		Expect(err).ToNot(HaveOccurred())

		certPath = filepath.Join(certDir, "client.crt")
		keyPath = filepath.Join(certDir, "client.key")
		Expect(os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}), 0644)).To(Succeed())
		Expect(os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0600)).To(Succeed())
		return certPath, keyPath
	}

	// startMockCollectorRequiringClientCert is startMockCollector plus
	// tls.RequireAndVerifyClientCert against signingCA -- standing in for a
	// collector deployment that mandates mTLS, not just server-auth TLS.
	startMockCollectorRequiringClientCert := func(signingKey *ecdsa.PrivateKey, signingCA *x509.Certificate) (server *http.Server, addr string, received *atomic.Int64) {
		received = &atomic.Int64{}

		srvKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		Expect(err).ToNot(HaveOccurred())
		srvTemplate := &x509.Certificate{
			SerialNumber: big.NewInt(4),
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

		clientCAPool := x509.NewCertPool()
		clientCAPool.AddCert(signingCA)

		mux := http.NewServeMux()
		mux.HandleFunc("/v1/traces", func(w http.ResponseWriter, r *http.Request) {
			received.Add(1)
			w.WriteHeader(http.StatusOK)
		})

		listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
			Certificates: []tls.Certificate{tlsCert},
			MinVersion:   tls.VersionTLS12,
			ClientAuth:   tls.RequireAndVerifyClientCert,
			ClientCAs:    clientCAPool,
		})
		Expect(err).ToNot(HaveOccurred())

		server = &http.Server{Handler: mux}
		go func() {
			defer GinkgoRecover()
			_ = server.Serve(listener)
		}()

		return server, listener.Addr().String(), received
	}

	// IT-1519-006: mTLS control validation. internal/config.TelemetryTLSConfig
	// exposes CertFile/KeyFile specifically so the OTLP exporter can present a
	// client certificate when a collector enforces mutual TLS -- a stronger
	// SC-8 posture than server-auth-only TLS. UT-1519-009/010 (tls_internal_test.go)
	// already prove the *tls.Config is built correctly; this proves that config
	// actually completes a real mTLS handshake with a collector that demands
	// one, and that omitting the client cert against the same collector fails
	// closed (proving the collector's requirement -- and therefore the
	// client's cert -- is doing real work, not decorative config).
	It("IT-1519-006: completes a real mTLS handshake and delivers a span when the collector requires a client certificate", func() {
		collector, collectorAddr, received := startMockCollectorRequiringClientCert(caKey, caCert)
		defer func() { _ = collector.Close() }()

		clientCertPath, clientKeyPath := issueClientCert(caKey, caCert)

		ctx := context.Background()
		shutdown, err := sharedtelemetry.NewTracerProvider(ctx, sharedtelemetry.Config{
			ServiceName: "gateway-it-1519",
			Endpoint:    collectorAddr,
			TLS: internalconfig.TelemetryTLSConfig{
				Enabled:  true,
				CAFile:   caCertPath,
				CertFile: clientCertPath,
				KeyFile:  clientKeyPath,
			},
		})
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = shutdown(context.Background()) }()

		appServer := httptest.NewServer(productionAppHandler())
		defer appServer.Close()

		resp, err := http.Get(appServer.URL + "/api/v1/signals/prometheus")
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.Body.Close()).To(Succeed())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		Expect(shutdown(context.Background())).To(Succeed())

		Eventually(func() int64 { return received.Load() }, "5s", "50ms").Should(
			BeNumerically(">=", 1),
			"BR-OTEL-1519: a client certificate configured via CertFile/KeyFile must complete a real mTLS handshake and deliver a span",
		)
	})

	// IT-1519-006b: the negative control for IT-1519-006 -- against the SAME
	// mTLS-requiring collector, omitting CertFile/KeyFile must fail closed
	// (no span delivered), proving the collector's client-cert requirement is
	// actually enforced rather than the mock accepting any TLS connection.
	It("IT-1519-006b: never delivers a span to an mTLS-requiring collector when no client certificate is configured", func() {
		collector, collectorAddr, received := startMockCollectorRequiringClientCert(caKey, caCert)
		defer func() { _ = collector.Close() }()

		ctx := context.Background()
		shutdown, err := sharedtelemetry.NewTracerProvider(ctx, sharedtelemetry.Config{
			ServiceName: "gateway-it-1519",
			Endpoint:    collectorAddr,
			TLS: internalconfig.TelemetryTLSConfig{
				Enabled: true,
				CAFile:  caCertPath, // server-auth only -- no CertFile/KeyFile
			},
		})
		Expect(err).ToNot(HaveOccurred())
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = shutdown(shutdownCtx)
		}()

		appServer := httptest.NewServer(productionAppHandler())
		defer appServer.Close()

		resp, err := http.Get(appServer.URL + "/api/v1/signals/prometheus")
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.Body.Close()).To(Succeed())

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = shutdown(shutdownCtx) // handshake failure is expected and swallowed by the SDK; asserted via the collector below

		Consistently(func() int64 { return received.Load() }, "1s", "100ms").Should(
			Equal(int64(0)),
			"BR-OTEL-1519: a collector that requires a client certificate must never receive a span from a client presenting none",
		)
	})

	// startMockCollectorCapturingTLSState is startMockCollector plus
	// recording the negotiated tls.ConnectionState of each accepted
	// connection, so the test can assert what the CLIENT actually
	// negotiated on the wire -- not just what buildTLSConfig constructed
	// in memory (already covered by UT-1519-011/012).
	startMockCollectorCapturingTLSState := func(signingKey *ecdsa.PrivateKey, signingCA *x509.Certificate) (server *http.Server, addr string, received *atomic.Int64, negotiated *atomic.Value) {
		received = &atomic.Int64{}
		negotiated = &atomic.Value{}

		srvKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		Expect(err).ToNot(HaveOccurred())
		srvTemplate := &x509.Certificate{
			SerialNumber: big.NewInt(5),
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
			if r.TLS != nil {
				negotiated.Store(*r.TLS)
			}
			w.WriteHeader(http.StatusOK)
		})

		// Deliberately permissive listener-side minimum (TLS 1.0) -- the
		// point of this test is that the CLIENT's configured
		// SecurityProfile, not the server, is what enforces the floor.
		listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
			Certificates: []tls.Certificate{tlsCert},
			MinVersion:   tls.VersionTLS10,
		})
		Expect(err).ToNot(HaveOccurred())

		server = &http.Server{Handler: mux}
		go func() {
			defer GinkgoRecover()
			_ = server.Serve(listener)
		}()

		return server, listener.Addr().String(), received, negotiated
	}

	// IT-1519-007: the process-wide SecurityProfile (Issue #748; e.g. the
	// OpenShift TLSSecurityProfile the operator resolves onto every
	// service) must constrain the ACTUAL negotiated connection to the
	// collector, not just the in-memory *tls.Config UT-1519-011/012
	// already prove was built correctly. A Modern profile (TLS 1.3-only)
	// against a collector willing to negotiate as low as TLS 1.0 must
	// still result in a TLS 1.3 connection -- proving the client enforces
	// its own floor rather than accepting whatever the server offers.
	It("IT-1519-007: enforces the process-wide SecurityProfile's minimum TLS version on the wire, not just in the constructed config", func() {
		sharedtls.SetDefaultSecurityProfile(sharedtls.ModernProfile())
		defer sharedtls.ResetDefaultSecurityProfileForTesting()

		collector, collectorAddr, received, negotiated := startMockCollectorCapturingTLSState(caKey, caCert)
		defer func() { _ = collector.Close() }()

		ctx := context.Background()
		shutdown, err := sharedtelemetry.NewTracerProvider(ctx, sharedtelemetry.Config{
			ServiceName: "gateway-it-1519",
			Endpoint:    collectorAddr,
			TLS: internalconfig.TelemetryTLSConfig{
				Enabled: true,
				CAFile:  caCertPath,
			},
		})
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = shutdown(context.Background()) }()

		appServer := httptest.NewServer(productionAppHandler())
		defer appServer.Close()

		resp, err := http.Get(appServer.URL + "/api/v1/signals/prometheus")
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.Body.Close()).To(Succeed())

		Expect(shutdown(context.Background())).To(Succeed())

		Eventually(func() int64 { return received.Load() }, "5s", "50ms").Should(BeNumerically(">=", 1))

		state, ok := negotiated.Load().(tls.ConnectionState)
		Expect(ok).To(BeTrue(), "collector must have observed a TLS connection state")
		Expect(state.Version).To(Equal(uint16(tls.VersionTLS13)),
			"BR-OTEL-1519: a Modern SecurityProfile must force the OTLP exporter's actual negotiated TLS version to 1.3, "+
				"even though the collector would accept as low as TLS 1.0")
	})

	// startMockCollectorCapturingBody is startMockCollector plus capturing
	// the raw request body of every /v1/traces POST onto a buffered
	// channel, so the test can inspect exactly what bytes crossed the wire
	// to the collector.
	startMockCollectorCapturingBody := func(signingKey *ecdsa.PrivateKey, signingCA *x509.Certificate) (server *http.Server, addr string, bodies chan []byte) {
		bodies = make(chan []byte, 10)

		srvKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		Expect(err).ToNot(HaveOccurred())
		srvTemplate := &x509.Certificate{
			SerialNumber: big.NewInt(6),
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
			body, err := io.ReadAll(r.Body)
			if err == nil {
				bodies <- body
			}
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

		return server, listener.Addr().String(), bodies
	}

	// IT-1519-008: data-minimization control validation. otelhttp's default
	// inbound instrumentation records method/route/status attributes only --
	// it does not capture request headers unless a caller explicitly adds
	// otelhttp.WithMessageEvents or a custom attribute extractor (neither is
	// configured anywhere in this codebase; grep confirms no SetAttributes
	// call ever forwards headers into a span). This test turns that
	// currently-true-by-construction property into a regression-guarded one:
	// a real Authorization bearer token sent on the inbound request must
	// never appear in the bytes actually delivered to the collector. This is
	// the AU-3-adjacent "audit/telemetry content must not itself become a
	// secret-disclosure channel" control, distinct from SC-8 (which only
	// covers the transport, not the payload).
	It("IT-1519-008: never includes the inbound Authorization header/token in the exported span payload", func() {
		collector, collectorAddr, bodies := startMockCollectorCapturingBody(caKey, caCert)
		defer func() { _ = collector.Close() }()

		ctx := context.Background()
		shutdown, err := sharedtelemetry.NewTracerProvider(ctx, sharedtelemetry.Config{
			ServiceName: "gateway-it-1519",
			Endpoint:    collectorAddr,
			TLS: internalconfig.TelemetryTLSConfig{
				Enabled: true,
				CAFile:  caCertPath,
			},
		})
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = shutdown(context.Background()) }()

		appServer := httptest.NewServer(productionAppHandler())
		defer appServer.Close()

		const secretToken = "super-secret-it-1519-token-must-never-leak-into-a-span"
		req, err := http.NewRequest(http.MethodGet, appServer.URL+"/api/v1/signals/prometheus", nil)
		Expect(err).ToNot(HaveOccurred())
		req.Header.Set("Authorization", "Bearer "+secretToken)

		resp, err := http.DefaultClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.Body.Close()).To(Succeed())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		Expect(shutdown(context.Background())).To(Succeed())

		var body []byte
		Eventually(bodies, "5s", "50ms").Should(Receive(&body),
			"BR-OTEL-1519: the OTLP export POST must actually reach the collector")

		// Sanity check first: prove the body isn't empty/vacuous by
		// decoding it as a real OTLP export request and confirming it
		// carries at least one span with attributes -- otherwise the
		// absence checks below would trivially pass on garbage.
		var exportReq coltracepb.ExportTraceServiceRequest
		Expect(proto.Unmarshal(body, &exportReq)).To(Succeed())
		Expect(exportReq.ResourceSpans).ToNot(BeEmpty())
		Expect(exportReq.ResourceSpans[0].ScopeSpans).ToNot(BeEmpty())
		Expect(exportReq.ResourceSpans[0].ScopeSpans[0].Spans).ToNot(BeEmpty(),
			"decoded export must contain a real span, not an empty payload")

		Expect(body).ToNot(ContainSubstring(secretToken),
			"BR-OTEL-1519: the exported span payload must never contain the inbound request's bearer token")
		Expect(body).ToNot(ContainSubstring("Authorization"),
			"BR-OTEL-1519: the exported span payload must never echo the Authorization header name/value")
	})
})
