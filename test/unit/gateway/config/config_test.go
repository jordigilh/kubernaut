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

// ============================================================================
// BUSINESS OUTCOME TESTS: Configuration Validation
// ============================================================================
//
// PURPOSE: Validate Gateway configuration enables business functionality:
// - Gateway can start and accept webhook requests (valid config)
// - Gateway fails fast with actionable errors (invalid config)
//
// BUSINESS VALUE:
// - Operators can deploy Gateway successfully with valid configuration
// - Operators get clear error messages when configuration is invalid
// - Gateway doesn't start in invalid state (fail-fast principle)
//
// REFACTORED: December 27, 2025 - Reduced from 24 tests to 4 tests
// Previous version tested config validation framework (TESTING_GUIDELINES.md violation)
// New version tests business outcomes only
// ============================================================================

var _ = Describe("BR-GATEWAY-100: Gateway Configuration Validation", func() {
	Context("Valid Configuration (Gateway Can Start)", func() {
		It("should load complete valid configuration enabling all business features", func() {
			// BUSINESS OUTCOME: Gateway can start with valid config and accept webhooks
			// This enables: Alert ingestion, deduplication, CRD creation, audit persistence
			cfg, err := config.LoadFromFile("testdata/valid-config.yaml")

			Expect(err).ToNot(HaveOccurred(), "Valid config must load successfully for Gateway to start")
			Expect(cfg).ToNot(BeNil())

			// Validate critical business settings are present
			// These enable Gateway to fulfill its business purpose:
			// - Accept webhook requests (Server.ListenAddr)
			// - Deduplicate alerts (Processing.Deduplication.TTL)
			// - Persist audit events (Infrastructure.DataStorageURL)
			Expect(cfg.Server.ListenAddr).ToNot(BeEmpty(), "Listen address required to accept webhook requests")
			Expect(cfg.Processing.Deduplication.TTL).To(BeNumerically(">", 0), "TTL required for alert deduplication")
			Expect(cfg.Infrastructure.DataStorageURL).ToNot(BeEmpty(), "Data Storage URL required for audit persistence")
		})

		It("should support environment variable overrides for deployment flexibility", func() {
			// BUSINESS OUTCOME: Operators can override config via env vars without rebuilding images
			// This enables: 12-factor app deployments, multi-environment configurations
			cfg, err := config.LoadFromFile("testdata/valid-config.yaml")
			Expect(err).ToNot(HaveOccurred())

			// Override critical settings via environment variables
			_ = os.Setenv("GATEWAY_LISTEN_ADDR", ":9090")
			_ = os.Setenv("GATEWAY_DATA_STORAGE_URL", "http://datastorage:8080")
			_ = os.Setenv("GATEWAY_DEDUP_TTL", "10m")
			defer func() {
				_ = os.Unsetenv("GATEWAY_LISTEN_ADDR")
				_ = os.Unsetenv("GATEWAY_DATA_STORAGE_URL")
				_ = os.Unsetenv("GATEWAY_DEDUP_TTL")
			}()

			cfg.LoadFromEnv()

			// Verify environment overrides work (enables multi-environment deployments)
			Expect(cfg.Server.ListenAddr).To(Equal(":9090"), "Env override enables port customization per environment")
			Expect(cfg.Infrastructure.DataStorageURL).To(Equal("http://datastorage:8080"), "Env override enables service URL customization")
			Expect(cfg.Processing.Deduplication.TTL).To(Equal(10*time.Minute), "Env override enables TTL tuning per environment")
		})
	})

	Context("Invalid Configuration (Gateway Fails Fast)", func() {
		It("should reject configuration missing critical required fields", func() {
			// BUSINESS OUTCOME: Gateway fails to start with clear error when misconfigured
			// This prevents: Silent failures, incorrect alert processing, data loss
			cfg := &config.ServerConfig{
				Server: config.ServerSettings{
					ListenAddr: "", // Missing required field
				},
				Processing: config.ProcessingSettings{
					CRD: config.CRDSettings{
						FallbackNamespace: "kubernaut-system",
					},
					Deduplication: config.DeduplicationSettings{
						TTL: 5 * time.Minute,
					},
					Retry: config.DefaultRetrySettings(),
				},
			}

			err := cfg.Validate()
			Expect(err).To(HaveOccurred(), "Invalid config must fail validation to prevent silent failures")
			Expect(err.Error()).To(MatchRegexp("listen.*addr|address"), "Error message must identify missing required field")
		})

		It("should reject configuration with invalid business-critical values", func() {
			// BUSINESS OUTCOME: Gateway rejects configs that would cause business logic failures
			// This prevents: Alert duplication (invalid TTL), retry storms (invalid retry config)
			cfg := &config.ServerConfig{
				Server: config.ServerSettings{
					ListenAddr:   ":8080",
					ReadTimeout:  30 * time.Second,
					WriteTimeout: 30 * time.Second,
					IdleTimeout:  120 * time.Second,
				},
				Processing: config.ProcessingSettings{
					CRD: config.CRDSettings{
						FallbackNamespace: "kubernaut-system",
					},
					Deduplication: config.DeduplicationSettings{
						TTL: 5 * time.Second, // Invalid: Below minimum threshold (10s)
					},
					Retry: config.RetrySettings{
						MaxAttempts:    0, // Invalid: Must be >= 1
						InitialBackoff: 100 * time.Millisecond,
						MaxBackoff:     5 * time.Second,
					},
				},
			}

			err := cfg.Validate()
			Expect(err).To(HaveOccurred(), "Invalid business-critical values must fail validation")
			// Error should identify at least one invalid field (TTL or retry settings)
			Expect(err.Error()).To(MatchRegexp("ttl|retry|attempts"), "Error message must identify business-critical validation failure")
		})
	})
})
