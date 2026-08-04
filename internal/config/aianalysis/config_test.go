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
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	config "github.com/jordigilh/kubernaut/internal/config/aianalysis"
)

func TestAIAnalysisConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AIAnalysis Config Test Suite")
}

var _ = Describe("Config.Validate", func() {
	It("accepts the default config unchanged", func() {
		cfg := config.DefaultConfig()
		Expect(cfg.Validate()).ToNot(HaveOccurred())
	})

	Describe("rego.confidenceThreshold validation (#225)", func() {
		It("accepts a nil confidenceThreshold (Rego policy's built-in default applies)", func() {
			cfg := config.DefaultConfig()
			cfg.Rego.ConfidenceThreshold = nil
			Expect(cfg.Validate()).ToNot(HaveOccurred())
		})

		It("rejects a confidenceThreshold of 0", func() {
			cfg := config.DefaultConfig()
			zero := 0.0
			cfg.Rego.ConfidenceThreshold = &zero
			err := cfg.Validate()
			Expect(err).To(MatchError(ContainSubstring("rego.confidenceThreshold must be in range (0.0, 1.0]")))
		})

		It("rejects a confidenceThreshold above 1.0", func() {
			cfg := config.DefaultConfig()
			tooHigh := 1.5
			cfg.Rego.ConfidenceThreshold = &tooHigh
			err := cfg.Validate()
			Expect(err).To(MatchError(ContainSubstring("rego.confidenceThreshold must be in range (0.0, 1.0]")))
		})
	})

	// UT-AA-1828-004: BR-AI-088.4 / Issue #1828 — lowConfidenceFloor validation.
	// Mirrors confidenceThreshold's validation shape exactly (same range, same
	// nil-means-default semantics) since both are operator-configurable
	// confidence knobs on the same RegoConfig struct — see LowConfidenceFloor's
	// doc comment for how the two gates differ.
	Describe("rego.lowConfidenceFloor validation (BR-AI-088.4, #1828)", func() {
		It("accepts a nil lowConfidenceFloor (built-in 70% floor applies)", func() {
			cfg := config.DefaultConfig()
			cfg.Rego.LowConfidenceFloor = nil
			Expect(cfg.Validate()).ToNot(HaveOccurred())
		})

		It("accepts a valid lowConfidenceFloor within (0.0, 1.0]", func() {
			cfg := config.DefaultConfig()
			floor := 0.5
			cfg.Rego.LowConfidenceFloor = &floor
			Expect(cfg.Validate()).ToNot(HaveOccurred())
		})

		It("accepts the upper boundary value 1.0", func() {
			cfg := config.DefaultConfig()
			floor := 1.0
			cfg.Rego.LowConfidenceFloor = &floor
			Expect(cfg.Validate()).ToNot(HaveOccurred())
		})

		It("rejects a lowConfidenceFloor of 0", func() {
			cfg := config.DefaultConfig()
			zero := 0.0
			cfg.Rego.LowConfidenceFloor = &zero
			err := cfg.Validate()
			Expect(err).To(MatchError(ContainSubstring("rego.lowConfidenceFloor must be in range (0.0, 1.0]")))
		})

		It("rejects a negative lowConfidenceFloor", func() {
			cfg := config.DefaultConfig()
			negative := -0.1
			cfg.Rego.LowConfidenceFloor = &negative
			err := cfg.Validate()
			Expect(err).To(MatchError(ContainSubstring("rego.lowConfidenceFloor must be in range (0.0, 1.0]")))
		})

		It("rejects a lowConfidenceFloor above 1.0", func() {
			cfg := config.DefaultConfig()
			tooHigh := 1.01
			cfg.Rego.LowConfidenceFloor = &tooHigh
			err := cfg.Validate()
			Expect(err).To(MatchError(ContainSubstring("rego.lowConfidenceFloor must be in range (0.0, 1.0]")))
		})
	})
})
