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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/config"
)

var _ = Describe("Configuration (BR-EM-006, BR-EM-007, BR-EM-008)", func() {

	// ========================================
	// UT-EM-CF-001: Valid config parsed successfully
	// ========================================
	Describe("DefaultAssessmentConfig (UT-EM-CF-001, UT-EM-CF-003)", func() {

		It("UT-EM-CF-001: should return a valid config with sensible defaults", func() {
			// Given: Request for default assessment config
			// When: We call DefaultAssessmentConfig
			cfg := config.DefaultAssessmentConfig()

			// Then: Config should pass validation
			Expect(cfg.Validate()).To(Succeed())
		})

		It("UT-EM-CF-003: should apply default values for all optional fields", func() {
			// Given: Default assessment config
			cfg := config.DefaultAssessmentConfig()

			// Then: All fields should have non-zero defaults
			Expect(cfg.StabilizationWindow).To(Equal(5 * time.Minute), "BR-EM-006: default stabilization 5m")
			Expect(cfg.ValidityWindow).To(Equal(30 * time.Minute), "BR-EM-007: default validity 30m")
			Expect(cfg.ScoringThreshold).To(Equal(0.5), "BR-EM-008: default scoring threshold 0.5")
			Expect(cfg.PrometheusEnabled).To(BeTrue(), "Prometheus enabled by default")
			Expect(cfg.AlertManagerEnabled).To(BeTrue(), "AlertManager enabled by default")
		})

		It("UT-EM-CF-004: should have validityWindow default of 30m", func() {
			// Given/When: Default config
			cfg := config.DefaultAssessmentConfig()

			// Then: Validity window is exactly 30 minutes
			Expect(cfg.ValidityWindow).To(Equal(30 * time.Minute))
		})
	})

	// ========================================
	// UT-EM-CF-002: Missing/invalid required fields -> error
	// ========================================
	Describe("Validate (UT-EM-CF-002)", func() {

		Context("stabilization window validation", func() {

			It("should reject stabilization window below 30s", func() {
				cfg := config.DefaultAssessmentConfig()
				cfg.StabilizationWindow = 10 * time.Second

				err := cfg.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("stabilizationWindow must be at least 30s"))
			})

			It("should reject stabilization window above 1h", func() {
				cfg := config.DefaultAssessmentConfig()
				cfg.StabilizationWindow = 2 * time.Hour

				err := cfg.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("stabilizationWindow must not exceed 1h"))
			})

			It("should accept stabilization window at lower bound (30s)", func() {
				cfg := config.DefaultAssessmentConfig()
				cfg.StabilizationWindow = 30 * time.Second

				Expect(cfg.Validate()).To(Succeed())
			})

			It("should accept stabilization window at upper bound (1h minus 1ns)", func() {
				cfg := config.DefaultAssessmentConfig()
				cfg.StabilizationWindow = 1*time.Hour - 1*time.Nanosecond
				cfg.ValidityWindow = 24 * time.Hour // Ensure validity > stabilization

				Expect(cfg.Validate()).To(Succeed())
			})
		})

		Context("validity window validation", func() {

			It("should reject validity window below 5m", func() {
				cfg := config.DefaultAssessmentConfig()
				cfg.ValidityWindow = 1 * time.Minute

				err := cfg.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("validityWindow must be at least 5m"))
			})

			It("should reject validity window above 24h", func() {
				cfg := config.DefaultAssessmentConfig()
				cfg.ValidityWindow = 48 * time.Hour

				err := cfg.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("validityWindow must not exceed 24h"))
			})
		})

		Context("stabilization vs validity constraint", func() {

			It("should reject stabilization >= validity", func() {
				cfg := config.DefaultAssessmentConfig()
				cfg.StabilizationWindow = 30 * time.Minute
				cfg.ValidityWindow = 30 * time.Minute

				err := cfg.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("stabilizationWindow"))
				Expect(err.Error()).To(ContainSubstring("must be shorter than"))
			})

			It("should reject stabilization > validity", func() {
				cfg := config.DefaultAssessmentConfig()
				cfg.StabilizationWindow = 45 * time.Minute
				cfg.ValidityWindow = 30 * time.Minute

				err := cfg.Validate()
				Expect(err).To(HaveOccurred())
			})
		})

		// UT-EM-CF-008: scoringThreshold parsed correctly
		Context("scoring threshold validation (UT-EM-CF-008)", func() {

			It("should reject scoring threshold below 0.0", func() {
				cfg := config.DefaultAssessmentConfig()
				cfg.ScoringThreshold = -0.1

				err := cfg.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("scoringThreshold must be between 0.0 and 1.0"))
			})

			It("should reject scoring threshold above 1.0", func() {
				cfg := config.DefaultAssessmentConfig()
				cfg.ScoringThreshold = 1.1

				err := cfg.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("scoringThreshold must be between 0.0 and 1.0"))
			})

			It("should accept scoring threshold at lower bound (0.0)", func() {
				cfg := config.DefaultAssessmentConfig()
				cfg.ScoringThreshold = 0.0

				Expect(cfg.Validate()).To(Succeed())
			})

			It("should accept scoring threshold at upper bound (1.0)", func() {
				cfg := config.DefaultAssessmentConfig()
				cfg.ScoringThreshold = 1.0

				Expect(cfg.Validate()).To(Succeed())
			})

			It("should accept scoring threshold at default (0.5)", func() {
				cfg := config.DefaultAssessmentConfig()
				// Default is 0.5
				Expect(cfg.Validate()).To(Succeed())
			})
		})

		// UT-EM-CF-006 / UT-EM-CF-007: Feature flag validation
		Context("feature flag configuration (UT-EM-CF-006, UT-EM-CF-007)", func() {

			It("UT-EM-CF-006: should allow prometheus disabled", func() {
				cfg := config.DefaultAssessmentConfig()
				cfg.PrometheusEnabled = false

				Expect(cfg.Validate()).To(Succeed())
				Expect(cfg.PrometheusEnabled).To(BeFalse())
			})

			It("UT-EM-CF-007: should allow alertmanager disabled", func() {
				cfg := config.DefaultAssessmentConfig()
				cfg.AlertManagerEnabled = false

				Expect(cfg.Validate()).To(Succeed())
				Expect(cfg.AlertManagerEnabled).To(BeFalse())
			})

			It("should allow both prometheus and alertmanager disabled", func() {
				cfg := config.DefaultAssessmentConfig()
				cfg.PrometheusEnabled = false
				cfg.AlertManagerEnabled = false

				Expect(cfg.Validate()).To(Succeed())
			})
		})
	})
})
