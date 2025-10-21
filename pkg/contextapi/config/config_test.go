package config_test

import (
	"os"
	"testing"

	"github.com/jordigilh/kubernaut/pkg/contextapi/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Config Suite")
}

var _ = Describe("Config Loading", func() {
	Context("when loading from YAML file", func() {
		It("should load configuration from valid YAML file", func() {
			cfg, err := config.LoadConfig("testdata/valid-config.yaml")
			Expect(err).ToNot(HaveOccurred())
			Expect(cfg).ToNot(BeNil())
			Expect(cfg.Server.Port).To(Equal(8091))
			Expect(cfg.Server.Host).To(Equal("0.0.0.0"))
			Expect(cfg.Database.Host).To(Equal("localhost"))
			Expect(cfg.Database.Port).To(Equal(5432))
			Expect(cfg.Database.Name).To(Equal("action_history"))
			Expect(cfg.Cache.RedisAddr).To(Equal("localhost:6379"))
			Expect(cfg.Cache.RedisDB).To(Equal(0))
		})

		It("should return error for non-existent file", func() {
			cfg, err := config.LoadConfig("testdata/non-existent.yaml")
			Expect(err).To(HaveOccurred())
			Expect(cfg).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("failed to read config file"))
		})

		It("should return error for malformed YAML", func() {
			cfg, err := config.LoadConfig("testdata/malformed-config.yaml")
			Expect(err).To(HaveOccurred())
			Expect(cfg).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("failed to parse config"))
		})
	})

	Context("when loading from environment variables", func() {
		AfterEach(func() {
			// Cleanup environment variables
			_ = os.Unsetenv("DB_HOST")
			_ = os.Unsetenv("DB_PORT")
			_ = os.Unsetenv("DB_PASSWORD")
			_ = os.Unsetenv("REDIS_ADDR")
			_ = os.Unsetenv("REDIS_DB")
		})

		It("should override YAML with environment variables", func() {
			cfg, err := config.LoadConfig("testdata/valid-config.yaml")
			Expect(err).ToNot(HaveOccurred())

			// Set environment variables
			Expect(os.Setenv("DB_HOST", "env-host")).To(Succeed())
			Expect(os.Setenv("DB_PORT", "5433")).To(Succeed())
			Expect(os.Setenv("DB_PASSWORD", "env-password")).To(Succeed())
			Expect(os.Setenv("REDIS_ADDR", "env-redis:6379")).To(Succeed())
			Expect(os.Setenv("REDIS_DB", "5")).To(Succeed())

			cfg.LoadFromEnv()

			Expect(cfg.Database.Host).To(Equal("env-host"))
			Expect(cfg.Database.Port).To(Equal(5433))
			Expect(cfg.Database.Password).To(Equal("env-password"))
			Expect(cfg.Cache.RedisAddr).To(Equal("env-redis:6379"))
			Expect(cfg.Cache.RedisDB).To(Equal(5))
		})

		It("should not override YAML when env vars are empty", func() {
			cfg, err := config.LoadConfig("testdata/valid-config.yaml")
			Expect(err).ToNot(HaveOccurred())

			originalHost := cfg.Database.Host
			originalPort := cfg.Database.Port

			cfg.LoadFromEnv()

			Expect(cfg.Database.Host).To(Equal(originalHost))
			Expect(cfg.Database.Port).To(Equal(originalPort))
		})
	})

	Context("when validating configuration", func() {
		It("should pass validation for valid config", func() {
			cfg, err := config.LoadConfig("testdata/valid-config.yaml")
			Expect(err).ToNot(HaveOccurred())

			err = cfg.Validate()
			Expect(err).ToNot(HaveOccurred())
		})

		It("should fail validation when database host is missing", func() {
			cfg := &config.Config{
				Database: config.DatabaseConfig{
					Port: 5432,
					Name: "test",
				},
				Server: config.ServerConfig{
					Port: 8091,
				},
			}

			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("database host required"))
		})

		It("should fail validation when database port is missing", func() {
			cfg := &config.Config{
				Database: config.DatabaseConfig{
					Host: "localhost",
					Name: "test",
				},
				Server: config.ServerConfig{
					Port: 8091,
				},
			}

			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("database port required"))
		})

		It("should fail validation when database name is missing", func() {
			cfg := &config.Config{
				Database: config.DatabaseConfig{
					Host: "localhost",
					Port: 5432,
				},
				Server: config.ServerConfig{
					Port: 8091,
				},
			}

			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("database name required"))
		})

		It("should fail validation when server port is missing", func() {
			cfg := &config.Config{
				Database: config.DatabaseConfig{
					Host: "localhost",
					Port: 5432,
					Name: "test",
					User: "testuser",
				},
				Server: config.ServerConfig{
					Port: 0,
				},
				Cache: config.CacheConfig{
					RedisAddr: "localhost:6379",
				},
			}

			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("server port required"))
		})
	})
})
