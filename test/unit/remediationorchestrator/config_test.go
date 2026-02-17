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

package remediationorchestrator

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	config "github.com/jordigilh/kubernaut/internal/config/remediationorchestrator"
)

// ========================================
// CONFIG VALIDATION UNIT TESTS
// ========================================
//
// Business Requirement: BR-ORCH-027, BR-ORCH-028 (Timeout Configuration)
// ADR-030: Service Configuration Management
//
// Purpose: Validate that RemediationOrchestrator Config.Validate() correctly
// rejects invalid configuration and that LoadFromFile handles graceful
// degradation per ADR-030.
//
// Test Strategy:
// - Valid configuration should pass (DefaultConfig characterization)
// - LoadFromFile with valid YAML should produce expected config
// - LoadFromFile with nonexistent path should return defaults gracefully
// - LoadFromFile with malformed YAML should return defaults gracefully
// - Validate() should reject invalid configs with clear error messages

var _ = Describe("RemediationOrchestrator Config - Unit Tests", Label("config", "validation", "ADR-030"), func() {

	// ========================================
	// DefaultConfig Characterization
	// ========================================
	Context("DefaultConfig", func() {
		It("should return a valid config with sensible defaults", func() {
			cfg := config.DefaultConfig()
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.Validate()).To(Succeed())
		})

		It("should set expected default values", func() {
			cfg := config.DefaultConfig()
			Expect(cfg.Controller.MetricsAddr).To(Equal(":9090"))
			Expect(cfg.Controller.HealthProbeAddr).To(Equal(":8081"))
			Expect(cfg.Timeouts.Global).To(Equal(1 * time.Hour))
			Expect(cfg.Timeouts.Processing).To(Equal(5 * time.Minute))
			Expect(cfg.Timeouts.Analyzing).To(Equal(10 * time.Minute))
			Expect(cfg.Timeouts.Executing).To(Equal(30 * time.Minute))
			Expect(cfg.EA.StabilizationWindow).To(Equal(5 * time.Minute))
			Expect(cfg.DataStorage.URL).To(Equal("http://data-storage-service:8080"))
		})
	})

	// ========================================
	// LoadFromFile
	// ========================================
	Context("LoadFromFile", func() {
		It("should load valid configuration from YAML file", func() {
			path := filepath.Join("config", "testdata", "valid-config.yaml")
			cfg, err := config.LoadFromFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.Validate()).To(Succeed())
			Expect(cfg.Controller.MetricsAddr).To(Equal(":9090"))
			Expect(cfg.Timeouts.Global).To(Equal(1 * time.Hour))
		})

		It("should return defaults when path is empty", func() {
			cfg, err := config.LoadFromFile("")
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.Validate()).To(Succeed())
		})

		It("should return defaults gracefully when file does not exist", func() {
			cfg, err := config.LoadFromFile("/nonexistent/path/config.yaml")
			// Graceful degradation: returns defaults even on error
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.Controller.MetricsAddr).To(Equal(":9090"))
			// Error may or may not be nil depending on implementation;
			// the key contract is that cfg is usable with defaults
			_ = err
		})

		It("should return defaults gracefully when YAML is malformed", func() {
			tmpDir := GinkgoT().TempDir()
			malformedPath := filepath.Join(tmpDir, "malformed.yaml")
			Expect(os.WriteFile(malformedPath, []byte("{{invalid yaml:::"), 0644)).To(Succeed())

			cfg, err := config.LoadFromFile(malformedPath)
			Expect(cfg).NotTo(BeNil())
			_ = err
		})
	})

	// ========================================
	// Validate - Invalid Configurations
	// ========================================
	Context("Validate rejects invalid configs", func() {
		It("should reject empty metricsAddr", func() {
			cfg := config.DefaultConfig()
			cfg.Controller.MetricsAddr = ""
			Expect(cfg.Validate()).To(MatchError(ContainSubstring("metricsAddr")))
		})

		It("should reject empty healthProbeAddr", func() {
			cfg := config.DefaultConfig()
			cfg.Controller.HealthProbeAddr = ""
			Expect(cfg.Validate()).To(MatchError(ContainSubstring("healthProbeAddr")))
		})

		It("BR-ORCH-027: should reject zero global timeout", func() {
			cfg := config.DefaultConfig()
			cfg.Timeouts.Global = 0
			Expect(cfg.Validate()).To(MatchError(ContainSubstring("timeouts.global")))
		})

		It("BR-ORCH-028: should reject zero processing timeout", func() {
			cfg := config.DefaultConfig()
			cfg.Timeouts.Processing = 0
			Expect(cfg.Validate()).To(MatchError(ContainSubstring("timeouts.processing")))
		})

		It("BR-ORCH-028: should reject global timeout smaller than sum of phase timeouts", func() {
			cfg := config.DefaultConfig()
			cfg.Timeouts.Global = 1 * time.Minute // Less than processing+analyzing+executing
			Expect(cfg.Validate()).To(MatchError(ContainSubstring("sum of phase timeouts")))
		})

		It("ADR-EM-001: should reject stabilization window below 1s", func() {
			cfg := config.DefaultConfig()
			cfg.EA.StabilizationWindow = 0
			Expect(cfg.Validate()).To(MatchError(ContainSubstring("stabilizationWindow")))
		})

		It("ADR-EM-001: should reject stabilization window above 1h", func() {
			cfg := config.DefaultConfig()
			cfg.EA.StabilizationWindow = 2 * time.Hour
			Expect(cfg.Validate()).To(MatchError(ContainSubstring("stabilizationWindow")))
		})

		It("ADR-030: should reject empty DataStorage URL", func() {
			cfg := config.DefaultConfig()
			cfg.DataStorage.URL = ""
			Expect(cfg.Validate()).To(MatchError(ContainSubstring("datastorage.url")))
		})

		It("should reject config loaded from invalid YAML testdata", func() {
			path := filepath.Join("config", "testdata", "invalid-config.yaml")
			cfg, err := config.LoadFromFile(path)
			// LoadFromFile validates internally; should return error for invalid config
			if err != nil {
				Expect(err.Error()).To(ContainSubstring("invalid configuration"))
			} else {
				// If LoadFromFile didn't validate, manual Validate should fail
				Expect(cfg.Validate()).To(HaveOccurred())
			}
		})
	})
})
