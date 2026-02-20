/*
Copyright 2025 Jordi Gil.

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

package workflowexecution

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	sharedconfig "github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/workflowexecution/config"
)

// ========================================
// CONFIG VALIDATION UNIT TESTS
// ========================================
//
// **Business Requirement**: BR-WE-009 (Configuration Validation)
// **Migrated from**: test/e2e/workflowexecution/05_custom_config_test.go (Line 253)
//
// **Purpose**: Validate that Config.Validate() correctly rejects invalid
// configuration and provides clear error messages to prevent silent failures
// in production.
//
// **Why Unit Tests (not E2E)**:
// ✅ Config.Validate() is a pure function (no K8s API needed)
// ✅ Tests validation logic directly (no controller deployment needed)
// ✅ Much faster execution (milliseconds vs minutes)
// ✅ Can test all edge cases easily
// ✅ Same pattern as other services (signalprocessing, datastorage, gateway)
//
// **Priority**: HIGHEST - Validates fail-fast behavior to prevent silent failures
//
// **Test Strategy**:
// - Valid configuration should pass
// - Invalid cooldown periods should fail
// - Invalid namespace values should fail
// - Invalid backoff settings should fail
// - Invalid audit URLs should fail
// - Error messages should be clear and actionable

var _ = Describe("Config.Validate - Unit Tests", Label("config", "validation"), func() {

	// ========================================
	// TEST 1: Valid Configuration
	// ========================================
	Context("when configuration is valid", func() {
		It("should return nil for complete valid configuration", func() {
			cfg := config.DefaultConfig()
			err := cfg.Validate()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept valid custom cooldown period", func() {
			cfg := config.DefaultConfig()
			cfg.Execution.CooldownPeriod = 10 * time.Minute // Custom but valid
			err := cfg.Validate()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept valid custom execution namespace", func() {
			cfg := config.DefaultConfig()
			cfg.Execution.Namespace = "custom-workflows" // Custom but valid
			err := cfg.Validate()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// ========================================
	// TEST 2: Invalid Cooldown Period
	// **Priority**: HIGHEST (from E2E test #3)
	// ========================================
	Context("BR-WE-009: Invalid Cooldown Period", func() {
		It("should fail with clear error for negative cooldown period", func() {
			cfg := config.DefaultConfig()
			cfg.Execution.CooldownPeriod = -1 * time.Minute // Invalid!

			err := cfg.Validate()

			Expect(err).To(HaveOccurred(), "Negative cooldown should be rejected")
			Expect(err.Error()).To(Or(
				ContainSubstring("CooldownPeriod"),
				ContainSubstring("cooldown"),
				ContainSubstring("positive"),
			), "Error message should mention cooldown and be actionable")
		})

		It("should fail with clear error for zero cooldown period", func() {
			cfg := config.DefaultConfig()
			cfg.Execution.CooldownPeriod = 0 // Invalid!

			err := cfg.Validate()

			Expect(err).To(HaveOccurred(), "Zero cooldown should be rejected")
			Expect(err.Error()).To(Or(
				ContainSubstring("CooldownPeriod"),
				ContainSubstring("cooldown"),
			), "Error message should mention cooldown")
		})
	})

	// ========================================
	// TEST 3: Invalid Execution Namespace
	// ========================================
	Context("BR-WE-009: Invalid Execution Namespace", func() {
		It("should fail with clear error for empty execution namespace", func() {
			cfg := config.DefaultConfig()
			cfg.Execution.Namespace = "" // Invalid!

			err := cfg.Validate()

			Expect(err).To(HaveOccurred(), "Empty namespace should be rejected")
			Expect(err.Error()).To(Or(
				ContainSubstring("Namespace"),
				ContainSubstring("namespace"),
				ContainSubstring("required"),
			), "Error message should mention namespace and be actionable")
		})
	})

	// ========================================
	// TEST 4: Invalid Service Account
	// ========================================
	Context("BR-WE-007: Invalid Service Account", func() {
		It("should fail with clear error for empty service account", func() {
			cfg := config.DefaultConfig()
			cfg.Execution.ServiceAccount = "" // Invalid!

			err := cfg.Validate()

			Expect(err).To(HaveOccurred(), "Empty service account should be rejected")
			Expect(err.Error()).To(Or(
				ContainSubstring("ServiceAccount"),
				ContainSubstring("service"),
				ContainSubstring("required"),
			), "Error message should mention service account")
		})
	})

	// Issue #99: TEST 5 (DD-WE-004 Invalid Backoff Settings) removed per DD-RO-002 Phase 3

	// ========================================
	// TEST 6: Invalid DataStorage Configuration
	// ========================================
	Context("BR-WE-005, ADR-032: Invalid DataStorage Configuration", func() {
		It("should fail with clear error for empty DataStorage URL", func() {
			cfg := config.DefaultConfig()
			cfg.DataStorage.URL = "" // Invalid!

			err := cfg.Validate()

			Expect(err).To(HaveOccurred(), "Empty DataStorage URL should be rejected")
			Expect(err.Error()).To(Or(
				ContainSubstring("datastorage"),
				ContainSubstring("url"),
				ContainSubstring("required"),
			), "Error message should mention datastorage URL")
		})

		It("should fail with clear error for zero DataStorage timeout", func() {
			cfg := config.DefaultConfig()
			cfg.DataStorage.Timeout = 0 // Invalid!

			err := cfg.Validate()

			Expect(err).To(HaveOccurred(), "Zero DataStorage timeout should be rejected")
		})
	})

	// ========================================
	// TEST 7: Invalid Controller Configuration
	// ========================================
	Context("DD-005: Invalid Controller Settings", func() {
		It("should fail with clear error for empty metrics address", func() {
			cfg := config.DefaultConfig()
			cfg.Controller.MetricsAddr = "" // Invalid!

			err := cfg.Validate()

			Expect(err).To(HaveOccurred(), "Empty metrics address should be rejected")
		})

		It("should fail with clear error for empty health probe address", func() {
			cfg := config.DefaultConfig()
			cfg.Controller.HealthProbeAddr = "" // Invalid!

			err := cfg.Validate()

			Expect(err).To(HaveOccurred(), "Empty health probe address should be rejected")
		})

		It("should fail with clear error for empty leader election ID", func() {
			cfg := config.DefaultConfig()
			cfg.Controller.LeaderElectionID = "" // Invalid!

			err := cfg.Validate()

			Expect(err).To(HaveOccurred(), "Empty leader election ID should be rejected")
		})
	})

	// ========================================
	// TEST 8: Multiple Invalid Fields
	// ========================================
	Context("when multiple fields are invalid", func() {
		It("should fail and report validation errors", func() {
			cfg := &config.Config{
				Controller: config.ControllerConfig{
					MetricsAddr:      "", // Invalid!
					HealthProbeAddr:  "", // Invalid!
					LeaderElection:   false,
					LeaderElectionID: "", // Invalid!
				},
				Execution: config.ExecutionConfig{
					Namespace:      "",               // Invalid!
					CooldownPeriod: -1 * time.Minute, // Invalid!
					ServiceAccount: "",               // Invalid!
				},
			// Issue #99: BackoffConfig removed (DD-RO-002 Phase 3)
			DataStorage: sharedconfig.DataStorageConfig{
					URL:     "", // Invalid!
					Timeout: 0,  // Invalid!
				},
			}

			err := cfg.Validate()

			Expect(err).To(HaveOccurred(), "Multiple invalid fields should be rejected")
			// Validator reports all errors, so error message should be comprehensive
		})
	})
})
