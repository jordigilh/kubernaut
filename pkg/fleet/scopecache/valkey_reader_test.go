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

package scopecache_test

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
	"os"
	"path/filepath"
	"time"

	"github.com/alicebob/miniredis/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
)

// generateSelfSignedCertPair writes a self-signed cert/key pair usable as
// both server certificate and its own trust root (mirrors
// pkg/shared/tls/tls_test.go and cmd/apifrontend/replay_cache_wiring_test.go).
func generateSelfSignedCertPair(certFile, keyFile string) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).ToNot(HaveOccurred())

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
		DNSNames:     []string{"localhost"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	Expect(err).ToNot(HaveOccurred())
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	Expect(os.WriteFile(certFile, certPEM, 0o644)).To(Succeed()) //nolint:gosec // test fixture

	keyDER, err := x509.MarshalECPrivateKey(key)
	Expect(err).ToNot(HaveOccurred())
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	Expect(os.WriteFile(keyFile, keyPEM, 0o600)).To(Succeed())
}

// UT-FLEET-VALKEY-READ: ValkeyCacheReader I/O tests
// Authority: BR-FLEET-002 (Fleet Metadata Caching)
// FedRAMP: SC-8 (Transmission Confidentiality and Integrity) -- cache layer I/O
var _ = Describe("UT-FLEET-VALKEY-READ: ValkeyCacheReader", func() {
	var (
		ctx    context.Context
		mr     *miniredis.Miniredis
		reader *scopecache.ValkeyCacheReader
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		mr, err = miniredis.Run()
		Expect(err).ToNot(HaveOccurred())
		reader = scopecache.NewValkeyCacheReader(mr.Addr())
	})

	AfterEach(func() {
		reader.Close()
		mr.Close()
	})

	Describe("Exists", func() {
		It("UT-FLEET-VALKEY-READ-001: should return true for existing key", func() {
			_ = mr.Set("fleet:cluster-a:v1:Pod:default:nginx", "1")

			exists, err := reader.Exists(ctx, "fleet:cluster-a:v1:Pod:default:nginx")
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeTrue())
		})

		It("UT-FLEET-VALKEY-READ-002: should return false for non-existing key", func() {
			exists, err := reader.Exists(ctx, "fleet:nonexistent:v1:Pod:default:ghost")
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeFalse())
		})
	})

	Describe("Ping", func() {
		It("UT-FLEET-VALKEY-READ-003: should succeed when server is reachable", func() {
			err := reader.Ping(ctx)
			Expect(err).ToNot(HaveOccurred())
		})

		It("UT-FLEET-VALKEY-READ-004: should fail when server is unreachable", func() {
			mr.Close()
			err := reader.Ping(ctx)
			Expect(err).To(HaveOccurred())
		})
	})
})

// UT-FLEET-VALKEY-READ-TLS: ValkeyCacheReader TLS wiring tests
// DD-PLATFORM-006 DA9 follow-up (round-16 RCA, PR #1790): DA8 makes the
// chart's Valkey TLS-only, but ValkeyCacheReader had zero TLS support --
// these tests prove WithTLSConfig is actually wired into a real TLS
// handshake against a TLS-only Valkey, not just accepted and dropped.
// Authority: BR-FLEET-002 (Fleet Metadata Caching)
// FedRAMP: SC-8 (Transmission Confidentiality and Integrity)
var _ = Describe("UT-FLEET-VALKEY-READ-TLS: ValkeyCacheReader TLS", func() {
	var (
		ctx     context.Context
		certDir string
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		certDir, err = os.MkdirTemp("", "fmc-valkey-tls-*")
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		_ = os.RemoveAll(certDir)
	})

	It("UT-FLEET-VALKEY-READ-TLS-001: connects over TLS with a trusted CA", func() {
		certFile := filepath.Join(certDir, "tls.crt")
		keyFile := filepath.Join(certDir, "tls.key")
		generateSelfSignedCertPair(certFile, keyFile)

		serverCert, err := tls.LoadX509KeyPair(certFile, keyFile)
		Expect(err).ToNot(HaveOccurred())
		mr, err := miniredis.RunTLS(&tls.Config{Certificates: []tls.Certificate{serverCert}}) //nolint:gosec // test-only min version default is fine
		Expect(err).ToNot(HaveOccurred())
		defer mr.Close()

		caPool := x509.NewCertPool()
		caPEM, err := os.ReadFile(certFile)
		Expect(err).ToNot(HaveOccurred())
		Expect(caPool.AppendCertsFromPEM(caPEM)).To(BeTrue())

		reader := scopecache.NewValkeyCacheReader(mr.Addr(), scopecache.WithTLSConfig(&tls.Config{RootCAs: caPool, MinVersion: tls.VersionTLS12}))
		defer reader.Close()

		Expect(reader.Ping(ctx)).To(Succeed())
	})

	It("UT-FLEET-VALKEY-READ-TLS-002: fails closed against a TLS-only server when no TLS config is supplied", func() {
		certFile := filepath.Join(certDir, "tls.crt")
		keyFile := filepath.Join(certDir, "tls.key")
		generateSelfSignedCertPair(certFile, keyFile)

		serverCert, err := tls.LoadX509KeyPair(certFile, keyFile)
		Expect(err).ToNot(HaveOccurred())
		mr, err := miniredis.RunTLS(&tls.Config{Certificates: []tls.Certificate{serverCert}}) //nolint:gosec // test-only min version default is fine
		Expect(err).ToNot(HaveOccurred())
		defer mr.Close()

		reader := scopecache.NewValkeyCacheReader(mr.Addr()) // no WithTLSConfig -- plaintext client
		defer reader.Close()

		err = reader.Ping(ctx)
		Expect(err).To(HaveOccurred(), "a plaintext client must not silently succeed against a TLS-only Valkey")
	})

	It("UT-FLEET-VALKEY-READ-TLS-003: a nil TLS config is a no-op (plaintext) for backward compatibility", func() {
		mr, err := miniredis.Run()
		Expect(err).ToNot(HaveOccurred())
		defer mr.Close()

		reader := scopecache.NewValkeyCacheReader(mr.Addr(), scopecache.WithTLSConfig(nil))
		defer reader.Close()

		Expect(reader.Ping(ctx)).To(Succeed())
	})
})
