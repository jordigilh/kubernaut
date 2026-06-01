package launcher_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/config"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
)

func generateTestCertPair(dir string) (certFile, keyFile string) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).ToNot(HaveOccurred())
	template := &x509.Certificate{
		SerialNumber: big.NewInt(99),
		Subject:      pkix.Name{CommonName: "test-client"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	Expect(err).ToNot(HaveOccurred())

	certFile = filepath.Join(dir, "client.crt")
	Expect(os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}), 0644)).To(Succeed())

	keyDER, err := x509.MarshalECPrivateKey(key)
	Expect(err).ToNot(HaveOccurred())
	keyFile = filepath.Join(dir, "client.key")
	Expect(os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0600)).To(Succeed())

	return certFile, keyFile
}

var _ = Describe("Transport Chain", func() {
	Describe("BuildTransportChain", func() {
		It("UT-AF-1252-TC-001: returns nil when no transport config is set", func() {
			cfg := config.LLMConfig{
				Provider: config.LLMProviderGemini,
				Model:    "gemini-2.0-flash",
				APIKey:   "test-key",
			}
			rt, err := launcher.BuildTransportChainForTest(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(rt).To(BeNil())
		})

		It("UT-AF-1252-TC-002: constructs TLS-only transport chain", func() {
			dir := GinkgoT().TempDir()
			caFile := filepath.Join(dir, "ca.crt")
			Expect(os.WriteFile(caFile, []byte(testCACert), 0o600)).To(Succeed())

			cfg := config.LLMConfig{
				Provider:  config.LLMProviderGemini,
				Model:     "gemini-2.0-flash",
				APIKey:    "test-key",
				TLSCaFile: caFile,
			}
			rt, err := launcher.BuildTransportChainForTest(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(rt).NotTo(BeNil())
		})

		It("UT-AF-1252-TC-003: returns error for invalid TLS CA file", func() {
			cfg := config.LLMConfig{
				Provider:  config.LLMProviderGemini,
				Model:     "gemini-2.0-flash",
				APIKey:    "test-key",
				TLSCaFile: "/nonexistent/ca.crt",
			}
			rt, err := launcher.BuildTransportChainForTest(cfg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("load TLS CA"))
			Expect(rt).To(BeNil())
		})

		It("UT-AF-1252-TC-004: constructs circuit-breaker-only transport chain", func() {
			cfg := config.LLMConfig{
				Provider: config.LLMProviderGemini,
				Model:    "gemini-2.0-flash",
				APIKey:   "test-key",
				CircuitBreaker: config.LLMCircuitBreaker{
					Enabled:          true,
					MaxRequests:      3,
					Interval:         10 * time.Second,
					Timeout:          30 * time.Second,
					FailureThreshold: 5,
				},
			}
			rt, err := launcher.BuildTransportChainForTest(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(rt).NotTo(BeNil())
		})

		It("UT-AF-1252-TC-005: returns error when OAuth2 credential files are missing", func() {
			cfg := config.LLMConfig{
				Provider: config.LLMProviderGemini,
				Model:    "gemini-2.0-flash",
				APIKey:   "test-key",
				OAuth2: config.LLMOAuth2Config{
					Enabled:        true,
					TokenURL:       "https://auth.example.com/token",
					CredentialsDir: "/nonexistent/creds",
				},
			}
			rt, err := launcher.BuildTransportChainForTest(cfg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("resolve OAuth2 secrets"))
			Expect(rt).To(BeNil())
		})

		// UT-AF-1342-020: mTLS transport chain construction
		It("UT-AF-1342-020: constructs mTLS transport chain with client cert", func() {
			dir := GinkgoT().TempDir()
			caFile := filepath.Join(dir, "ca.crt")
			Expect(os.WriteFile(caFile, []byte(testCACert), 0o600)).To(Succeed())

			certFile, keyFile := generateTestCertPair(dir)

			cfg := config.LLMConfig{
				Provider:    config.LLMProviderGemini,
				Model:       "gemini-2.0-flash",
				APIKey:      "test-key",
				TLSCaFile:   caFile,
				TLSCertFile: certFile,
				TLSKeyFile:  keyFile,
			}
			rt, err := launcher.BuildTransportChainForTest(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(rt).NotTo(BeNil())
		})

		// UT-AF-1342-021: mTLS with invalid cert returns error
		It("UT-AF-1342-021: returns error for invalid client cert file", func() {
			dir := GinkgoT().TempDir()
			caFile := filepath.Join(dir, "ca.crt")
			Expect(os.WriteFile(caFile, []byte(testCACert), 0o600)).To(Succeed())

			cfg := config.LLMConfig{
				Provider:    config.LLMProviderGemini,
				Model:       "gemini-2.0-flash",
				APIKey:      "test-key",
				TLSCaFile:   caFile,
				TLSCertFile: "/nonexistent/client.crt",
				TLSKeyFile:  "/nonexistent/client.key",
			}
			rt, err := launcher.BuildTransportChainForTest(cfg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("client certificate"))
			Expect(rt).To(BeNil())
		})
	})

	Describe("BuildLLMHTTPClient", func() {
		It("UT-AF-1252-TC-006: returns nil client when no transport config", func() {
			cfg := config.LLMConfig{
				Provider: config.LLMProviderGemini,
				Model:    "gemini-2.0-flash",
				APIKey:   "test-key",
			}
			client, err := launcher.BuildLLMHTTPClientForTest(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(client).To(BeNil())
		})

		It("UT-AF-1252-TC-007: applies configured timeout", func() {
			dir := GinkgoT().TempDir()
			caFile := filepath.Join(dir, "ca.crt")
			Expect(os.WriteFile(caFile, []byte(testCACert), 0o600)).To(Succeed())

			cfg := config.LLMConfig{
				Provider:       config.LLMProviderGemini,
				Model:          "gemini-2.0-flash",
				APIKey:         "test-key",
				TLSCaFile:      caFile,
				TimeoutSeconds: 60,
			}
			client, err := launcher.BuildLLMHTTPClientForTest(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil())
			Expect(client.Timeout).To(Equal(60 * time.Second))
		})

		It("UT-AF-1252-TC-008: uses default timeout when not configured", func() {
			dir := GinkgoT().TempDir()
			caFile := filepath.Join(dir, "ca.crt")
			Expect(os.WriteFile(caFile, []byte(testCACert), 0o600)).To(Succeed())

			cfg := config.LLMConfig{
				Provider:  config.LLMProviderGemini,
				Model:     "gemini-2.0-flash",
				APIKey:    "test-key",
				TLSCaFile: caFile,
			}
			client, err := launcher.BuildLLMHTTPClientForTest(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil())
			Expect(client.Timeout).To(Equal(time.Duration(config.DefaultLLMTimeoutSeconds) * time.Second))
		})
	})
})

// testCACert is a self-signed CA certificate for testing TLS transport construction.
const testCACert = `-----BEGIN CERTIFICATE-----
MIIDAzCCAeugAwIBAgIUe0eB61uYLyO7ZNkfNmH+zzSjqOYwDQYJKoZIhvcNAQEL
BQAwETEPMA0GA1UEAwwGVGVzdENBMB4XDTI2MDUyNDAwMDI1OFoXDTI3MDUyNDAw
MDI1OFowETEPMA0GA1UEAwwGVGVzdENBMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8A
MIIBCgKCAQEAu6gaNCj6skPlixo6BDyYTMYWMkm25skLYxqiC7/ocNLmkO6nMYAT
naKxfZE28/Ebx7XwGUHbA+yajFWWod5JH8HosXMBli2XCs0yJQpsinmztZUOOEWB
PYIyv8HeuARqH/RiO+aeVybkc54UDKh367zxPdda/YkcpkMLBDlX152HGkhjTGNO
dePMU9jdcDi7fgQty1K3kHY0zshKnolXDt7kqsWQptPtfdk+f619B+kByWQnyJmA
qi+vX9Ixa1lxCf68DdfS/l3PJ6PWbSLeD6vs7I6CAfX2hlcLjj6xNkrnqaI5/ZOO
tkF4H5gWoBJtVHYHqAMKQPOebJTwJ3Z+OwIDAQABo1MwUTAdBgNVHQ4EFgQUB6SU
CmZY7/HdcM0dwDB1poV6bmEwHwYDVR0jBBgwFoAUB6SUCmZY7/HdcM0dwDB1poV6
bmEwDwYDVR0TAQH/BAUwAwEB/zANBgkqhkiG9w0BAQsFAAOCAQEAHvL1JjCEIoY6
g8MytDNzZv2R4UYjHi4HkeinP8uu/uF2kyT1qsGRZW+eOsQ+D8Nr4ounmUODZz4k
Wge57pu9F4AzvxtC5z4jLcDAWGvAGTvOUqx/Kc50VJ2w6GBqunW4oPQ53OgrrqqJ
AEJwNJqC+xkWMNM6O9FL6v7NDGLqOookAw/bTY6hByRnmvGQIvZCpekeQbzq06iV
J7/w9nCZ4qAQZtyXnb5rKMZ9x4oe2L+CtSD+ZfmE4PRVm2LCbYwgxmoskUMLxPpj
HQTWeS9GcO9/pD8+DJtP8wIbitnT3oSsKZ1WaoegsEB5IEIrk8CD4lgkAEYAv7GZ
McX6RiIchA==
-----END CERTIFICATE-----`
