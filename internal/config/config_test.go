package config

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Config", func() {
	var (
		tempDir    string
		configFile string
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "config-test")
		Expect(err).NotTo(HaveOccurred())
		configFile = filepath.Join(tempDir, "config.yaml")
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
	})

	Describe("Load", func() {
		Context("when config file exists with valid content", func() {
			BeforeEach(func() {
				validConfig := `
server:
  webhook_port: "8080"
  metrics_port: "9090"

slm:
  endpoint: "http://localhost:11434"
  model: "llama2"
  timeout: "30s"
  retry_count: 3
  provider: "localai"
  temperature: 0.3
  max_tokens: 500

kubernetes:
  context: "test-context"
  namespace: "default"

actions:
  dry_run: false
  max_concurrent: 5
  cooldown_period: "5m"

filters:
  - name: "production-filter"
    conditions:
      namespace:
        - "production"
        - "staging"
      severity:
        - "critical"
        - "warning"

logging:
  level: "info"
  format: "json"

webhook:
  port: "8080"
  path: "/webhook"
`
				err := os.WriteFile(configFile, []byte(validConfig), 0644)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should load configuration successfully", func() {
				config, err := Load(configFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(config).NotTo(BeNil())

				// Verify server config
				Expect(config.Server.WebhookPort).To(Equal("8080"))
				Expect(config.Server.MetricsPort).To(Equal("9090"))

				// Verify SLM config
				Expect(config.SLM.Endpoint).To(Equal("http://localhost:11434"))
				Expect(config.SLM.Model).To(Equal("llama2"))
				Expect(config.SLM.Timeout).To(Equal(30 * time.Second))
				Expect(config.SLM.RetryCount).To(Equal(3))
				Expect(config.SLM.Provider).To(Equal("localai"))
				Expect(config.SLM.Temperature).To(Equal(float32(0.3)))
				Expect(config.SLM.MaxTokens).To(Equal(500))

				// Verify Kubernetes config
				Expect(config.Kubernetes.Context).To(Equal("test-context"))
				Expect(config.Kubernetes.Namespace).To(Equal("default"))

				// Verify Actions config
				Expect(config.Actions.DryRun).To(BeFalse())
				Expect(config.Actions.MaxConcurrent).To(Equal(5))
				Expect(config.Actions.CooldownPeriod).To(Equal(5 * time.Minute))

				// Verify Filters
				Expect(config.Filters).To(HaveLen(1))
				Expect(config.Filters[0].Name).To(Equal("production-filter"))
				Expect(config.Filters[0].Conditions["namespace"]).To(ContainElements("production", "staging"))
				Expect(config.Filters[0].Conditions["severity"]).To(ContainElements("critical", "warning"))

				// Verify Logging config
				Expect(config.Logging.Level).To(Equal("info"))
				Expect(config.Logging.Format).To(Equal("json"))

				// Verify Webhook config
				Expect(config.Webhook.Port).To(Equal("8080"))
				Expect(config.Webhook.Path).To(Equal("/webhook"))
			})
		})

		Context("when config file has minimal content", func() {
			BeforeEach(func() {
				minimalConfig := `
server:
  webhook_port: "3000"

slm:
  endpoint: "http://localhost:8080"
  model: "test-model"
  provider: "localai"
`
				err := os.WriteFile(configFile, []byte(minimalConfig), 0644)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should load with defaults for missing values", func() {
				config, err := Load(configFile)
				Expect(err).NotTo(HaveOccurred())

				// Required fields should be set
				Expect(config.Server.WebhookPort).To(Equal("3000"))
				Expect(config.SLM.Endpoint).To(Equal("http://localhost:8080"))
				Expect(config.SLM.Model).To(Equal("test-model"))

				// Check that defaults are applied where needed
				Expect(config.Kubernetes.Namespace).To(Equal("default"))
				Expect(config.Actions.MaxConcurrent).To(Equal(5))
				Expect(config.SLM.Provider).To(Equal("localai"))
			})
		})

		Context("when config file does not exist", func() {
			It("should return an error", func() {
				_, err := Load("/nonexistent/config.yaml")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to read config file"))
			})
		})

		Context("when config file has invalid YAML", func() {
			BeforeEach(func() {
				invalidConfig := `
server:
  webhook_port: "8080"
  invalid_yaml: [
slm:
  endpoint: "test"
`
				err := os.WriteFile(configFile, []byte(invalidConfig), 0644)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return an error", func() {
				_, err := Load(configFile)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to parse config file"))
			})
		})

		Context("when config has invalid duration formats", func() {
			BeforeEach(func() {
				invalidDurationConfig := `
server:
  webhook_port: "8080"

slm:
  endpoint: "http://localhost:11434"
  model: "test"
  timeout: "invalid-duration"
  provider: "localai"

actions:
  cooldown_period: "not-a-duration"
`
				err := os.WriteFile(configFile, []byte(invalidDurationConfig), 0644)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return an error", func() {
				_, err := Load(configFile)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to parse config file"))
			})
		})
	})

	Describe("validate", func() {
		var config *Config

		BeforeEach(func() {
			config = &Config{
				Server: ServerConfig{
					WebhookPort: "8080",
					MetricsPort: "9090",
				},
				SLM: SLMConfig{
					Endpoint:    "http://localhost:11434",
					Model:       "llama2",
					Timeout:     30 * time.Second,
					RetryCount:  3,
					Provider:    "localai",
					Temperature: 0.3,
					MaxTokens:   500,
				},
				Kubernetes: KubernetesConfig{
					Context:   "test-context",
					Namespace: "default",
				},
				Actions: ActionsConfig{
					DryRun:         false,
					MaxConcurrent:  5,
					CooldownPeriod: 5 * time.Minute,
				},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "json",
				},
			}
		})

		Context("when config is valid", func() {
			It("should pass validation", func() {
				err := validate(config)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when SLM provider is invalid", func() {
			BeforeEach(func() {
				config.SLM.Provider = "invalid"
			})

			It("should return validation error", func() {
				err := validate(config)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unsupported SLM provider"))
			})
		})

		Context("when SLM endpoint is missing", func() {
			BeforeEach(func() {
				config.SLM.Endpoint = ""
			})

			It("should set default endpoint", func() {
				err := validate(config)
				// SLM endpoint gets default value in validation, so this won't fail
				Expect(err).NotTo(HaveOccurred())
				Expect(config.SLM.Endpoint).To(Equal("http://localhost:8080"))
			})
		})

		Context("when SLM model is missing", func() {
			BeforeEach(func() {
				config.SLM.Model = ""
			})

			It("should return validation error", func() {
				err := validate(config)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("SLM model is required for LocalAI provider"))
			})
		})

		Context("when SLM temperature is out of range", func() {
			BeforeEach(func() {
				config.SLM.Temperature = 1.5
			})

			It("should return validation error", func() {
				err := validate(config)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("SLM temperature must be between 0.0 and 1.0"))
			})
		})

		Context("when SLM max tokens is invalid", func() {
			BeforeEach(func() {
				config.SLM.MaxTokens = 0
			})

			It("should return validation error", func() {
				err := validate(config)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("SLM max tokens must be greater than 0"))
			})
		})

		Context("when Kubernetes namespace is empty", func() {
			BeforeEach(func() {
				config.Kubernetes.Namespace = ""
			})

			It("should return validation error", func() {
				err := validate(config)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Kubernetes namespace is required"))
			})
		})

		Context("when max concurrent actions is invalid", func() {
			BeforeEach(func() {
				config.Actions.MaxConcurrent = 0
			})

			It("should return validation error", func() {
				err := validate(config)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("max concurrent actions must be greater than 0"))
			})
		})

		Context("when max concurrent actions is negative", func() {
			BeforeEach(func() {
				config.Actions.MaxConcurrent = -1
			})

			It("should return validation error", func() {
				err := validate(config)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("max concurrent actions must be greater than 0"))
			})
		})

		Context("when SLM retry count is negative", func() {
			BeforeEach(func() {
				config.SLM.RetryCount = -1
			})

			It("should pass validation", func() {
				// The current validation doesn't check for negative retry count
				err := validate(config)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when cooldown period is negative", func() {
			BeforeEach(func() {
				config.Actions.CooldownPeriod = -1 * time.Minute
			})

			It("should pass validation", func() {
				// The current validation doesn't check for negative cooldown period
				err := validate(config)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when SLM timeout is negative", func() {
			BeforeEach(func() {
				config.SLM.Timeout = -1 * time.Second
			})

			It("should pass validation", func() {
				// The current validation doesn't check for negative timeout
				err := validate(config)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Describe("loadFromEnv", func() {
		var config *Config

		BeforeEach(func() {
			config = &Config{}
			// Clear any existing environment variables
			os.Clearenv()
		})

		Context("when environment variables are set", func() {
			BeforeEach(func() {
				os.Setenv("SLM_ENDPOINT", "http://test:8080")
				os.Setenv("SLM_MODEL", "test-model")
				os.Setenv("SLM_PROVIDER", "localai")
				os.Setenv("WEBHOOK_PORT", "3000")
				os.Setenv("METRICS_PORT", "9999")
				os.Setenv("LOG_LEVEL", "debug")
				os.Setenv("DRY_RUN", "true")
			})

			AfterEach(func() {
				os.Clearenv()
			})

			It("should load values from environment", func() {
				err := loadFromEnv(config)
				Expect(err).NotTo(HaveOccurred())

				Expect(config.SLM.Endpoint).To(Equal("http://test:8080"))
				Expect(config.SLM.Model).To(Equal("test-model"))
				Expect(config.SLM.Provider).To(Equal("localai"))
				Expect(config.Server.WebhookPort).To(Equal("3000"))
				Expect(config.Server.MetricsPort).To(Equal("9999"))
				Expect(config.Logging.Level).To(Equal("debug"))
				Expect(config.Actions.DryRun).To(BeTrue())
			})
		})

		Context("when no environment variables are set", func() {
			It("should not modify config", func() {
				originalConfig := *config
				err := loadFromEnv(config)
				Expect(err).NotTo(HaveOccurred())
				Expect(*config).To(Equal(originalConfig))
			})
		})
	})
})