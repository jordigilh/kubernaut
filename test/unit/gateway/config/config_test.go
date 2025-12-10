package config

import (
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	config "github.com/jordigilh/kubernaut/pkg/gateway/config"
)

func TestGatewayConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gateway Config Test Suite")
}

var _ = Describe("Gateway Configuration Loading", func() {
	Context("config.LoadFromFile", func() {
		It("should load valid configuration from YAML file", func() {
			cfg, err := config.LoadFromFile("testdata/valid-config.yaml")

			Expect(err).ToNot(HaveOccurred())
			Expect(cfg).ToNot(BeNil())

			// Validate server settings
			Expect(cfg.Server.ListenAddr).To(Equal(":8080"))
			Expect(cfg.Server.ReadTimeout).To(Equal(30 * time.Second))
			Expect(cfg.Server.WriteTimeout).To(Equal(30 * time.Second))
			Expect(cfg.Server.IdleTimeout).To(Equal(120 * time.Second))

			// Middleware: Rate limiting removed (ADR-048) - delegated to proxy
			// DD-GATEWAY-012: Redis REMOVED - Gateway is now Redis-free

			// DD-AUDIT-003: Validate Data Storage URL
			Expect(cfg.Infrastructure.DataStorageURL).To(Equal("http://data-storage-service:8080"))

			// Validate processing settings
			Expect(cfg.Processing.Deduplication.TTL).To(Equal(5 * time.Minute))
			Expect(cfg.Processing.Storm.RateThreshold).To(Equal(10))
			Expect(cfg.Processing.Storm.PatternThreshold).To(Equal(5))
			// Note: Environment.CacheTTL removed (2025-12-06) - classification moved to SP
		})

		It("should return error for non-existent file", func() {
			cfg, err := config.LoadFromFile("testdata/nonexistent.yaml")

			Expect(err).To(HaveOccurred())
			Expect(cfg).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("failed to read config file"))
		})

		It("should return error for malformed YAML", func() {
			cfg, err := config.LoadFromFile("testdata/malformed-config.yaml")

			Expect(err).To(HaveOccurred())
			Expect(cfg).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("failed to parse config"))
		})
	})

	Context("LoadFromEnv", func() {
		var cfg *config.ServerConfig

		BeforeEach(func() {
			var err error
			cfg, err = config.LoadFromFile("testdata/valid-config.yaml")
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			// Clean up environment variables
			os.Unsetenv("GATEWAY_LISTEN_ADDR")
			// DD-GATEWAY-012: Redis env vars REMOVED
			os.Unsetenv("GATEWAY_DATA_STORAGE_URL")
			os.Unsetenv("GATEWAY_DEDUP_TTL")
		})

		It("should override listen address from environment variable", func() {
			os.Setenv("GATEWAY_LISTEN_ADDR", ":9090")

			cfg.LoadFromEnv()

			Expect(cfg.Server.ListenAddr).To(Equal(":9090"))
		})

		// DD-GATEWAY-012: Redis env var tests REMOVED
		// Gateway is now Redis-free per DD-GATEWAY-012

		It("should override Data Storage URL from environment variable (DD-AUDIT-003)", func() {
			os.Setenv("GATEWAY_DATA_STORAGE_URL", "http://datastorage:8080")

			cfg.LoadFromEnv()

			Expect(cfg.Infrastructure.DataStorageURL).To(Equal("http://datastorage:8080"))
		})

		It("should override deduplication TTL from environment variable", func() {
			os.Setenv("GATEWAY_DEDUP_TTL", "10m")

			cfg.LoadFromEnv()

			Expect(cfg.Processing.Deduplication.TTL).To(Equal(10 * time.Minute))
		})

		It("should ignore invalid duration values", func() {
			originalTTL := cfg.Processing.Deduplication.TTL
			os.Setenv("GATEWAY_DEDUP_TTL", "invalid")

			cfg.LoadFromEnv()

			Expect(cfg.Processing.Deduplication.TTL).To(Equal(originalTTL))
		})
	})

	Context("Validate", func() {
		It("should validate valid configuration", func() {
			cfg, err := config.LoadFromFile("testdata/valid-config.yaml")
			Expect(err).ToNot(HaveOccurred())

			err = cfg.Validate()
			Expect(err).ToNot(HaveOccurred())
		})

		It("should reject empty listen address", func() {
			cfg := &config.ServerConfig{
				Server: config.ServerSettings{
					ListenAddr: "",
				},
			}

			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("listen_addr required"))
		})

		// DD-GATEWAY-012: Redis validation tests REMOVED
		// Gateway is now Redis-free per DD-GATEWAY-012

		// DD-AUDIT-003: Data Storage URL is OPTIONAL (graceful degradation)
		// No validation test needed - audit events are dropped if not configured

		// Rate limit validation tests removed (ADR-048) - delegated to proxy

		It("should reject negative storm threshold", func() {
			cfg := &config.ServerConfig{
				Server: config.ServerSettings{
					ListenAddr: ":8080",
				},
				// DD-GATEWAY-012: Redis REMOVED from InfrastructureSettings
				// Middleware: Rate limiting removed (ADR-048)
				Processing: config.ProcessingSettings{
					Storm: config.StormSettings{
						RateThreshold: -1,
					},
				},
			}

			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("rate_threshold must be positive"))
		})
	})
})
