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

package aianalysis_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	config "github.com/jordigilh/kubernaut/internal/config/aianalysis"
)

// ========================================
// CONFIG VALIDATION UNIT TESTS
// ========================================
//
// Business Requirement: BR-AI-007, BR-AI-012, BR-AA-KA-064
// ADR-030: Service Configuration Management
//
// Purpose: Validate that AIAnalysis Config.Validate() correctly rejects
// invalid configuration and that LoadFromFile handles graceful degradation.

var _ = Describe("AIAnalysis Config - Unit Tests", Label("config", "validation", "ADR-030"), func() {

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
			Expect(cfg.DataStorage.URL).To(Equal("http://data-storage-service:8080"))
			Expect(cfg.Rego.PolicyPath).To(Equal("/etc/aianalysis/policies/approval.rego"))
		})

		It("DD-AUDIT-004: should use LOW tier buffer defaults", func() {
			cfg := config.DefaultConfig()
			Expect(cfg.DataStorage.Buffer.BufferSize).To(Equal(20000))
			Expect(cfg.DataStorage.Buffer.BatchSize).To(Equal(1000))
		})

		It("#2204: should default MaxConcurrentReconciles to 10 workers", func() {
			// Prior to this fix, cmd/aianalysis/main.go called
			// SetupWithManager(mgr) with no explicit worker count, so
			// controller-runtime's implicit default of 1 silently serialized
			// every AIAnalysis reconcile in production (and in the E2E Kind
			// deployment, which runs this same binary) -- the exact
			// bottleneck already fixed for the integration envtest suite
			// (test/integration/aianalysis/suite_test.go: SetupWithManager(k8sManager, 10))
			// but never wired into the production entry point itself.
			cfg := config.DefaultConfig()
			Expect(cfg.MaxConcurrentReconciles).To(Equal(10))
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
			Expect(cfg.DataStorage.URL).To(Equal("http://data-storage-service:8080"))
		})

		It("should return defaults when path is empty", func() {
			cfg, err := config.LoadFromFile("")
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.Validate()).To(Succeed())
		})

		It("should return defaults gracefully when file does not exist", func() {
			cfg, err := config.LoadFromFile("/nonexistent/path/config.yaml")
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.DataStorage.URL).To(Equal("http://data-storage-service:8080"))
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

		It("ADR-030: should reject empty DataStorage URL", func() {
			cfg := config.DefaultConfig()
			cfg.DataStorage.URL = ""
			Expect(cfg.Validate()).To(MatchError(ContainSubstring("datastorage.url")))
		})

		It("BR-AI-012: should reject empty Rego policy path", func() {
			cfg := config.DefaultConfig()
			cfg.Rego.PolicyPath = ""
			Expect(cfg.Validate()).To(MatchError(ContainSubstring("policyPath")))
		})

		It("#2204: should reject a maxConcurrentReconciles below 1", func() {
			cfg := config.DefaultConfig()
			cfg.MaxConcurrentReconciles = 0
			Expect(cfg.Validate()).To(MatchError(ContainSubstring("maxConcurrentReconciles must be at least 1")))
		})

		It("should reject config loaded from invalid YAML testdata", func() {
			path := filepath.Join("config", "testdata", "invalid-config.yaml")
			cfg, err := config.LoadFromFile(path)
			if err != nil {
				Expect(err.Error()).To(ContainSubstring("invalid configuration"))
			} else {
				Expect(cfg.Validate()).To(HaveOccurred())
			}
		})
	})
})
