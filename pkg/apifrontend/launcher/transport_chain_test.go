package launcher_test

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/config"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
)

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
