package gateway

// BR-GATEWAY-082, BR-GATEWAY-019: Configuration Integration Tests
// Authority: GW_INTEGRATION_TEST_PLAN_V1.0.md Phase 3
//
// **SCOPE**: Configuration loading, validation, and safe defaults
// **ARCHITECTURE**: Gateway uses static ConfigMap-based config (no hot reload)
// **PATTERN**: Config loaded at startup, pod restart required for changes
//
// These tests validate:
// - Safe default values for production deployments (BR-GATEWAY-019)
// - Configuration validation catches invalid values (BR-GATEWAY-082)
// - Structured error messages for config errors (GAP-8)
// - Auto-detection of fallback namespace from pod environment
//
// **NOT TESTED** (hot reload not applicable):
// - Config reload triggers (CFG-001) - Gateway is stateless
// - Config change audit (CFG-004) - No reload events
// - Config rollback (CFG-006) - No in-memory state
// - Hot reload (CFG-007) - Requires pod restart
//
// Test Pattern:
// 1. Create config with specific values
// 2. Load and validate config
// 3. Verify expected behavior (defaults, validation errors)
// 4. Verify structured error messages
//
// Coverage: Phase 3 - 2 config tests (GW-INT-CFG-002, 003)

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/config"
)

var _ = Describe("Gateway Configuration", Label("integration", "config"), func() {
	var (
		tempDir string
	)

	BeforeEach(func() {
		// Create temporary directory for config files
		var err error
		tempDir, err = os.MkdirTemp("", "gw-config-test-*")
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		// Cleanup temporary directory
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	Context("BR-GATEWAY-019: Safe Defaults Validation", func() {
		It("[GW-INT-CFG-002] should provide production-ready default values (BR-GATEWAY-019)", func() {
			By("1. Create minimal config file (relying on defaults)")
			minimalConfig := `
server:
  listen_addr: ":8080"
infrastructure:
  data_storage_url: "http://data-storage:8080"
`
			configPath := filepath.Join(tempDir, "minimal-config.yaml")
			err := os.WriteFile(configPath, []byte(minimalConfig), 0644)
			Expect(err).ToNot(HaveOccurred())

			By("2. Load config from file")
			cfg, err := config.LoadFromFile(configPath)
			Expect(err).ToNot(HaveOccurred(),
				"BR-GATEWAY-019: Minimal config must load successfully with defaults")

			By("3. Verify safe default values for server timeouts")
			// Note: Zero values mean "use defaults" - actual defaults applied by HTTP server
			// We're verifying the config struct loads successfully, not the HTTP server behavior
			Expect(cfg.Server.ListenAddr).To(Equal(":8080"),
				"BR-GATEWAY-019: Listen address must be preserved")

			By("4. Verify default retry settings (BR-GATEWAY-111)")
			Expect(cfg.Processing.Retry.MaxAttempts).To(Equal(3),
				"BR-GATEWAY-111: Default max retry attempts must be 3")
			Expect(cfg.Processing.Retry.InitialBackoff).To(Equal(100*time.Millisecond),
				"BR-GATEWAY-111: Default initial backoff must be 100ms")
			Expect(cfg.Processing.Retry.MaxBackoff).To(Equal(5*time.Second),
				"BR-GATEWAY-111: Default max backoff must be 5s")

			By("5. Verify fallback namespace auto-detection")
			Expect(cfg.Processing.CRD.FallbackNamespace).ToNot(BeEmpty(),
				"BR-GATEWAY-019: Fallback namespace must be auto-detected")
			// In test environment, will default to "kubernaut-system"
			Expect(cfg.Processing.CRD.FallbackNamespace).To(Equal("kubernaut-system"),
				"BR-GATEWAY-019: Fallback namespace defaults to kubernaut-system in non-K8s env")

			By("6. Verify DataStorage URL preserved")
			Expect(cfg.Infrastructure.DataStorageURL).To(Equal("http://data-storage:8080"),
				"BR-GATEWAY-019: DataStorage URL must be preserved")

			By("7. Validate config passes validation")
			err = cfg.Validate()
			Expect(err).ToNot(HaveOccurred(),
				"BR-GATEWAY-019: Default config must pass validation")

			GinkgoWriter.Printf("✅ Safe defaults validated: MaxAttempts=%d, InitialBackoff=%v, MaxBackoff=%v, FallbackNS=%s\n",
				cfg.Processing.Retry.MaxAttempts,
				cfg.Processing.Retry.InitialBackoff,
				cfg.Processing.Retry.MaxBackoff,
				cfg.Processing.CRD.FallbackNamespace)
		})
	})

	Context("BR-GATEWAY-082: Invalid Config Rejection", func() {
		It("[GW-INT-CFG-003] should reject invalid config with structured error messages (BR-GATEWAY-082)", func() {
			By("1. Test invalid retry settings (MaxAttempts too high)")
			// Note: MaxAttempts=0 triggers smart defaults, so test with out-of-range value instead
			invalidRetryConfig := `
server:
  listen_addr: ":8080"
infrastructure:
  data_storage_url: "http://data-storage:8080"
processing:
  retry:
    max_attempts: 15
    initial_backoff: 100ms
    max_backoff: 5s
`
			configPath := filepath.Join(tempDir, "invalid-retry.yaml")
			err := os.WriteFile(configPath, []byte(invalidRetryConfig), 0644)
			Expect(err).ToNot(HaveOccurred())

			By("2. Load config and verify validation fails")
			cfg, err := config.LoadFromFile(configPath)
			Expect(err).ToNot(HaveOccurred(), "Config should load (validation happens separately)")

			err = cfg.Validate()
			Expect(err).To(HaveOccurred(),
				"BR-GATEWAY-082: Excessive max_attempts=15 must be rejected")
			Expect(err.Error()).To(ContainSubstring("max_attempts"),
				"BR-GATEWAY-082: Error must mention the invalid field")
			Expect(err.Error()).To(ContainSubstring("exceeds recommended maximum"),
				"BR-GATEWAY-082: Error must provide validation constraint")

			GinkgoWriter.Printf("✅ Excessive MaxAttempts=15 rejected: %v\n", err)

			By("3. Test invalid backoff settings (negative initial backoff)")
			invalidBackoffConfig := `
server:
  listen_addr: ":8080"
infrastructure:
  data_storage_url: "http://data-storage:8080"
processing:
  retry:
    max_attempts: 3
    initial_backoff: -100ms
    max_backoff: 5s
`
			configPath = filepath.Join(tempDir, "invalid-backoff.yaml")
			err = os.WriteFile(configPath, []byte(invalidBackoffConfig), 0644)
			Expect(err).ToNot(HaveOccurred())

			cfg, err = config.LoadFromFile(configPath)
			Expect(err).ToNot(HaveOccurred())

			err = cfg.Validate()
			Expect(err).To(HaveOccurred(),
				"BR-GATEWAY-082: Negative initial_backoff must be rejected")
			Expect(err.Error()).To(ContainSubstring("must be >= 0"),
				"BR-GATEWAY-082: Error must explain non-negative constraint")

			GinkgoWriter.Printf("✅ Negative InitialBackoff rejected: %v\n", err)

			By("4. Test invalid backoff settings (max_backoff < initial_backoff)")
			invalidMaxBackoffConfig := `
server:
  listen_addr: ":8080"
infrastructure:
  data_storage_url: "http://data-storage:8080"
processing:
  retry:
    max_attempts: 3
    initial_backoff: 5s
    max_backoff: 1s
`
			configPath = filepath.Join(tempDir, "invalid-max-backoff.yaml")
			err = os.WriteFile(configPath, []byte(invalidMaxBackoffConfig), 0644)
			Expect(err).ToNot(HaveOccurred())

			cfg, err = config.LoadFromFile(configPath)
			Expect(err).ToNot(HaveOccurred())

			err = cfg.Validate()
			Expect(err).To(HaveOccurred(),
				"BR-GATEWAY-082: max_backoff < initial_backoff must be rejected")
			Expect(err.Error()).To(ContainSubstring("must be >= initial_backoff"),
				"BR-GATEWAY-082: Error must explain backoff relationship constraint")

			GinkgoWriter.Printf("✅ Invalid max_backoff < initial_backoff rejected: %v\n", err)

			By("5. Test invalid server timeout (read_timeout too low)")
			invalidTimeoutConfig := `
server:
  listen_addr: ":8080"
  read_timeout: 1s
infrastructure:
  data_storage_url: "http://data-storage:8080"
`
			configPath = filepath.Join(tempDir, "invalid-timeout.yaml")
			err = os.WriteFile(configPath, []byte(invalidTimeoutConfig), 0644)
			Expect(err).ToNot(HaveOccurred())

			cfg, err = config.LoadFromFile(configPath)
			Expect(err).ToNot(HaveOccurred())

			err = cfg.Validate()
			Expect(err).To(HaveOccurred(),
				"BR-GATEWAY-082: read_timeout < 5s must be rejected")
			Expect(err.Error()).To(ContainSubstring("is too low"),
				"BR-GATEWAY-082: Error must explain timeout constraint")
			Expect(err.Error()).To(ContainSubstring("30s (recommended)"),
				"BR-GATEWAY-082: Error must provide recommended value")

			GinkgoWriter.Printf("✅ Low read_timeout=1s rejected: %v\n", err)

			By("6. Test missing required field (listen_addr)")
			invalidMissingConfig := `
infrastructure:
  data_storage_url: "http://data-storage:8080"
`
			configPath = filepath.Join(tempDir, "invalid-missing.yaml")
			err = os.WriteFile(configPath, []byte(invalidMissingConfig), 0644)
			Expect(err).ToNot(HaveOccurred())

			cfg, err = config.LoadFromFile(configPath)
			Expect(err).ToNot(HaveOccurred())

			err = cfg.Validate()
			Expect(err).To(HaveOccurred(),
				"BR-GATEWAY-082: Missing listen_addr must be rejected")
			Expect(err.Error()).To(ContainSubstring("listen_addr"),
				"BR-GATEWAY-082: Error must identify missing field")
			Expect(err.Error()).To(ContainSubstring("is required"),
				"BR-GATEWAY-082: Error must explain requirement")

			GinkgoWriter.Printf("✅ Missing listen_addr rejected: %v\n", err)

			By("7. Verify structured error contains actionable guidance")
			// All ConfigError instances should provide:
			// - Field name (e.g., "processing.retry.max_attempts")
			// - Invalid value
			// - Constraint explanation
			// - Recommended fix
			Expect(err.Error()).To(Or(
				ContainSubstring("Use"),
				ContainSubstring("Set"),
				ContainSubstring("recommended"),
			), "BR-GATEWAY-082: Error must provide actionable guidance")

			GinkgoWriter.Printf("✅ Structured error validation complete\n")
		})
	})
})
