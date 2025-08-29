package database

import (
	"os"
	"time"

	"github.com/sirupsen/logrus"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Database Configuration", func() {
	Describe("DefaultConfig", func() {
		It("should return correct default values", func() {
			config := DefaultConfig()
			
			Expect(config.Host).To(Equal("localhost"))
			Expect(config.Port).To(Equal(5432))
			Expect(config.User).To(Equal("slm_user"))
			Expect(config.Database).To(Equal("action_history"))
			Expect(config.SSLMode).To(Equal("disable"))
			Expect(config.MaxOpenConns).To(Equal(25))
			Expect(config.MaxIdleConns).To(Equal(5))
			Expect(config.ConnMaxLifetime).To(Equal(5 * time.Minute))
			Expect(config.ConnMaxIdleTime).To(Equal(5 * time.Minute))
		})
	})

	Describe("LoadFromEnv", func() {
		var config *Config
		var originalEnvVars map[string]string

		BeforeEach(func() {
			config = DefaultConfig()
			
			// Save original environment variables
			originalEnvVars = map[string]string{
				"DB_HOST":     os.Getenv("DB_HOST"),
				"DB_PORT":     os.Getenv("DB_PORT"),
				"DB_USER":     os.Getenv("DB_USER"),
				"DB_PASSWORD": os.Getenv("DB_PASSWORD"),
				"DB_NAME":     os.Getenv("DB_NAME"),
				"DB_SSL_MODE": os.Getenv("DB_SSL_MODE"),
			}
		})

		AfterEach(func() {
			// Restore original environment variables
			for key, value := range originalEnvVars {
				if value == "" {
					os.Unsetenv(key)
				} else {
					os.Setenv(key, value)
				}
			}
		})

		Context("when all environment variables are set", func() {
			BeforeEach(func() {
				os.Setenv("DB_HOST", "testhost")
				os.Setenv("DB_PORT", "3306")
				os.Setenv("DB_USER", "testuser")
				os.Setenv("DB_PASSWORD", "testpass")
				os.Setenv("DB_NAME", "testdb")
				os.Setenv("DB_SSL_MODE", "require")
			})

			It("should load values from environment", func() {
				config.LoadFromEnv()
				
				Expect(config.Host).To(Equal("testhost"))
				Expect(config.Port).To(Equal(3306))
				Expect(config.User).To(Equal("testuser"))
				Expect(config.Password).To(Equal("testpass"))
				Expect(config.Database).To(Equal("testdb"))
				Expect(config.SSLMode).To(Equal("require"))
			})
		})

		Context("when DB_PORT has invalid value", func() {
			BeforeEach(func() {
				os.Setenv("DB_PORT", "invalid_port")
			})

			It("should keep default port value", func() {
				originalPort := config.Port
				config.LoadFromEnv()
				
				Expect(config.Port).To(Equal(originalPort))
			})
		})

		Context("when environment variables are not set", func() {
			It("should keep default values", func() {
				originalConfig := *config
				config.LoadFromEnv()
				
				Expect(*config).To(Equal(originalConfig))
			})
		})
	})

	Describe("Validate", func() {
		var config *Config

		BeforeEach(func() {
			config = DefaultConfig()
		})

		Context("when config is valid", func() {
			It("should pass validation", func() {
				err := config.Validate()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when host is empty", func() {
			BeforeEach(func() {
				config.Host = ""
			})

			It("should return validation error", func() {
				err := config.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("database host is required"))
			})
		})

		Context("when port is invalid", func() {
			Context("when port is zero", func() {
				BeforeEach(func() {
					config.Port = 0
				})

				It("should return validation error", func() {
					err := config.Validate()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("database port must be between 1 and 65535"))
				})
			})

			Context("when port is too high", func() {
				BeforeEach(func() {
					config.Port = 70000
				})

				It("should return validation error", func() {
					err := config.Validate()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("database port must be between 1 and 65535"))
				})
			})
		})

		Context("when user is empty", func() {
			BeforeEach(func() {
				config.User = ""
			})

			It("should return validation error", func() {
				err := config.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("database user is required"))
			})
		})

		Context("when database name is empty", func() {
			BeforeEach(func() {
				config.Database = ""
			})

			It("should return validation error", func() {
				err := config.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("database name is required"))
			})
		})

		Context("when max open connections is invalid", func() {
			BeforeEach(func() {
				config.MaxOpenConns = 0
			})

			It("should return validation error", func() {
				err := config.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("max open connections must be greater than 0"))
			})
		})

		Context("when max idle connections is negative", func() {
			BeforeEach(func() {
				config.MaxIdleConns = -1
			})

			It("should return validation error", func() {
				err := config.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("max idle connections must be non-negative"))
			})
		})
	})

	Describe("ConnectionString", func() {
		var config *Config

		BeforeEach(func() {
			config = &Config{
				Host:     "localhost",
				Port:     5432,
				User:     "testuser",
				Database: "testdb",
				SSLMode:  "disable",
			}
		})

		Context("when password is provided", func() {
			BeforeEach(func() {
				config.Password = "testpass"
			})

			It("should include password in connection string", func() {
				result := config.ConnectionString()
				expected := "host=localhost port=5432 user=testuser dbname=testdb sslmode=disable password=testpass"
				Expect(result).To(Equal(expected))
			})
		})

		Context("when password is empty", func() {
			It("should exclude password from connection string", func() {
				result := config.ConnectionString()
				expected := "host=localhost port=5432 user=testuser dbname=testdb sslmode=disable"
				Expect(result).To(Equal(expected))
				Expect(result).NotTo(ContainSubstring("password="))
			})
		})

		Context("with production-like configuration", func() {
			BeforeEach(func() {
				config.Host = "prod-db.example.com"
				config.Password = "secure_password"
				config.Database = "action_history_prod"
				config.SSLMode = "verify-full"
			})

			It("should generate correct connection string", func() {
				result := config.ConnectionString()
				expected := "host=prod-db.example.com port=5432 user=testuser dbname=action_history_prod sslmode=verify-full password=secure_password"
				Expect(result).To(Equal(expected))
			})
		})
	})

	Describe("Connect", func() {
		var logger *logrus.Logger

		BeforeEach(func() {
			logger = logrus.New()
			logger.SetLevel(logrus.FatalLevel) // Suppress logs during tests
		})

		Context("with invalid configuration", func() {
			It("should return error for invalid config", func() {
				config := &Config{
					Host: "", // Invalid: empty host
					Port: 5432,
					User: "testuser",
				}

				_, err := Connect(config, logger)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid database configuration"))
			})
		})

		// Note: We don't test actual database connection here as it would require
		// a real database instance. Integration tests should cover that scenario.
	})
})