package config

import (
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	rediscache "github.com/jordigilh/kubernaut/pkg/cache/redis"
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

			// Validate infrastructure settings
			Expect(cfg.Infrastructure.Redis.Addr).To(Equal("redis-gateway:6379"))
			Expect(cfg.Infrastructure.Redis.DB).To(Equal(0))

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
			os.Unsetenv("GATEWAY_REDIS_ADDR")
			os.Unsetenv("GATEWAY_REDIS_DB")
			os.Unsetenv("GATEWAY_REDIS_PASSWORD")
			// GATEWAY_RATE_LIMIT_RPM removed (ADR-048)
			os.Unsetenv("GATEWAY_DEDUP_TTL")
		})

		It("should override listen address from environment variable", func() {
			os.Setenv("GATEWAY_LISTEN_ADDR", ":9090")

			cfg.LoadFromEnv()

			Expect(cfg.Server.ListenAddr).To(Equal(":9090"))
		})

		It("should override Redis address from environment variable", func() {
			os.Setenv("GATEWAY_REDIS_ADDR", "redis-prod:6379")

			cfg.LoadFromEnv()

			Expect(cfg.Infrastructure.Redis.Addr).To(Equal("redis-prod:6379"))
		})

		It("should override Redis password from environment variable", func() {
			os.Setenv("GATEWAY_REDIS_PASSWORD", "secret-password")

			cfg.LoadFromEnv()

			Expect(cfg.Infrastructure.Redis.Password).To(Equal("secret-password"))
		})

		It("should override Redis DB from environment variable", func() {
			os.Setenv("GATEWAY_REDIS_DB", "2")

			cfg.LoadFromEnv()

			Expect(cfg.Infrastructure.Redis.DB).To(Equal(2))
		})

		// Rate limit env override test removed (ADR-048) - delegated to proxy

		It("should override deduplication TTL from environment variable", func() {
			os.Setenv("GATEWAY_DEDUP_TTL", "10m")

			cfg.LoadFromEnv()

			Expect(cfg.Processing.Deduplication.TTL).To(Equal(10 * time.Minute))
		})

		It("should ignore invalid numeric values", func() {
			originalDB := cfg.Infrastructure.Redis.DB
			os.Setenv("GATEWAY_REDIS_DB", "invalid")

			cfg.LoadFromEnv()

			Expect(cfg.Infrastructure.Redis.DB).To(Equal(originalDB))
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

		It("should reject empty Redis address", func() {
			cfg := &config.ServerConfig{
				Server: config.ServerSettings{
					ListenAddr: ":8080",
				},
				Infrastructure: config.InfrastructureSettings{
					Redis: &rediscache.Options{
						Addr: "",
					},
				},
			}

			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("redis address required"))
		})

		// Rate limit validation tests removed (ADR-048) - delegated to proxy

		It("should reject negative storm threshold", func() {
			cfg := &config.ServerConfig{
				Server: config.ServerSettings{
					ListenAddr: ":8080",
				},
				Infrastructure: config.InfrastructureSettings{
					Redis: &rediscache.Options{
						Addr: "redis:6379",
					},
				},
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
