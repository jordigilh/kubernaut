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

package effectivenessmonitor

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	config "github.com/jordigilh/kubernaut/internal/config/effectivenessmonitor"
)

// ========================================
// EM SERVICE CONFIG VALIDATION UNIT TESTS
// ========================================
//
// Business Requirement: BR-EM-006, BR-EM-007, BR-EM-008
// ADR-030: Service Configuration Management
//
// Purpose: Validate that EffectivenessMonitor Config.Validate() correctly
// rejects invalid configuration and that LoadFromFile handles graceful
// degradation per ADR-030.
//
// Test Strategy:
// - Valid configuration should pass (DefaultConfig characterization)
// - LoadFromFile with valid YAML should produce expected config
// - LoadFromFile with nonexistent path should return defaults gracefully
// - LoadFromFile with malformed YAML should return defaults gracefully
// - Validate() should reject invalid configs with clear error messages

var _ = Describe("EffectivenessMonitor Service Config - Unit Tests", Label("config", "validation", "ADR-030"), func() {

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
			Expect(cfg.Controller.LeaderElection).To(BeFalse())
			Expect(cfg.Controller.LeaderElectionID).To(Equal("effectivenessmonitor.kubernaut.ai"))
			Expect(cfg.Assessment.StabilizationWindow).To(Equal(5 * time.Minute))
			Expect(cfg.Assessment.ValidityWindow).To(Equal(30 * time.Minute))
			Expect(cfg.External.PrometheusURL).To(Equal("http://prometheus:9090"))
			Expect(cfg.External.PrometheusEnabled).To(BeTrue())
			Expect(cfg.External.AlertManagerURL).To(Equal("http://alertmanager:9093"))
			Expect(cfg.External.AlertManagerEnabled).To(BeTrue())
			Expect(cfg.External.ConnectionTimeout).To(Equal(10 * time.Second))
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
			Expect(cfg.Assessment.StabilizationWindow).To(Equal(5 * time.Minute))
			Expect(cfg.External.PrometheusURL).To(Equal("http://prometheus:9090"))
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

		It("BR-EM-006: should reject stabilizationWindow below 30s", func() {
			cfg := config.DefaultConfig()
			cfg.Assessment.StabilizationWindow = 10 * time.Second
			Expect(cfg.Validate()).To(MatchError(ContainSubstring("stabilizationWindow")))
		})

		It("BR-EM-006: should reject stabilizationWindow above 1h", func() {
			cfg := config.DefaultConfig()
			cfg.Assessment.StabilizationWindow = 2 * time.Hour
			Expect(cfg.Validate()).To(MatchError(ContainSubstring("stabilizationWindow")))
		})

		It("BR-EM-007: should reject validityWindow below 30s", func() {
			cfg := config.DefaultConfig()
			cfg.Assessment.ValidityWindow = 10 * time.Second
			Expect(cfg.Validate()).To(MatchError(ContainSubstring("validityWindow")))
		})

		It("BR-EM-007: should reject validityWindow above 24h", func() {
			cfg := config.DefaultConfig()
			cfg.Assessment.ValidityWindow = 48 * time.Hour
			Expect(cfg.Validate()).To(MatchError(ContainSubstring("validityWindow")))
		})

		It("BR-EM-008: should reject stabilizationWindow >= validityWindow", func() {
			cfg := config.DefaultConfig()
			cfg.Assessment.StabilizationWindow = 30 * time.Minute
			cfg.Assessment.ValidityWindow = 30 * time.Minute
			Expect(cfg.Validate()).To(MatchError(ContainSubstring("stabilizationWindow")))
			Expect(cfg.Validate()).To(MatchError(ContainSubstring("must be shorter than")))
		})

		It("should reject prometheusEnabled with empty URL", func() {
			cfg := config.DefaultConfig()
			cfg.External.PrometheusEnabled = true
			cfg.External.PrometheusURL = ""
			Expect(cfg.Validate()).To(MatchError(ContainSubstring("prometheusUrl")))
		})

		It("should reject alertManagerEnabled with empty URL", func() {
			cfg := config.DefaultConfig()
			cfg.External.AlertManagerEnabled = true
			cfg.External.AlertManagerURL = ""
			Expect(cfg.Validate()).To(MatchError(ContainSubstring("alertManagerUrl")))
		})

		It("should reject zero connectionTimeout", func() {
			cfg := config.DefaultConfig()
			cfg.External.ConnectionTimeout = 0
			Expect(cfg.Validate()).To(MatchError(ContainSubstring("connectionTimeout")))
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
