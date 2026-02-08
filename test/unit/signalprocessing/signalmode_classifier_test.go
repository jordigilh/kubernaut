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

package signalprocessing

import (
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/signalprocessing/classifier"
)

// Unit Tests: Signal Mode Classifier (YAML-based)
// BR-SP-106: Predictive Signal Mode Classification
// ADR-054: Predictive Signal Mode Classification and Prompt Strategy
//
// Design Decision: YAML config (not Rego) because signal mode classification
// is a simple key-value lookup, unlike severity/environment/priority which
// evaluate complex multi-input policies.
var _ = Describe("Signal Mode Classifier (YAML)", func() {
	var (
		signalModeClassifier *classifier.SignalModeClassifier
		logger               logr.Logger
		configDir            string
	)

	BeforeEach(func() {
		logger = logr.Discard()

		// Create temp directory for test config
		var err error
		configDir, err = os.MkdirTemp("", "signalmode-test-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if configDir != "" {
			_ = os.RemoveAll(configDir)
		}
	})

	// Helper to create config file and load it
	createAndLoadConfig := func(content string) {
		configPath := filepath.Join(configDir, "predictive-signal-mappings.yaml")
		err := os.WriteFile(configPath, []byte(content), 0644)
		Expect(err).NotTo(HaveOccurred())

		signalModeClassifier = classifier.NewSignalModeClassifier(logger)
		err = signalModeClassifier.LoadConfig(configPath)
		Expect(err).NotTo(HaveOccurred())
	}

	// Standard config used in most tests
	standardConfig := `predictive_signal_mappings:
  PredictedOOMKill: OOMKilled
  PredictedCPUThrottling: CPUThrottling
  PredictedDiskPressure: DiskPressure
  PredictedNodeNotReady: NodeNotReady
`

	// UT-SP-106-001: Classify PredictedOOMKill as predictive + normalize to OOMKilled
	It("UT-SP-106-001: should classify PredictedOOMKill as predictive and normalize to OOMKilled", func() {
		createAndLoadConfig(standardConfig)

		result := signalModeClassifier.Classify("PredictedOOMKill")

		Expect(result.SignalMode).To(Equal("predictive"))
		Expect(result.NormalizedType).To(Equal("OOMKilled"))
		Expect(result.OriginalSignalType).To(Equal("PredictedOOMKill"))
	})

	// UT-SP-106-002: Classify OOMKilled as reactive (unchanged)
	It("UT-SP-106-002: should classify OOMKilled as reactive with no normalization", func() {
		createAndLoadConfig(standardConfig)

		result := signalModeClassifier.Classify("OOMKilled")

		Expect(result.SignalMode).To(Equal("reactive"))
		Expect(result.NormalizedType).To(Equal("OOMKilled"))
		Expect(result.OriginalSignalType).To(BeEmpty())
	})

	// UT-SP-106-003: Classify unmapped type as reactive (default)
	It("UT-SP-106-003: should classify unmapped signal type as reactive by default", func() {
		createAndLoadConfig(standardConfig)

		result := signalModeClassifier.Classify("CrashLoopBackOff")

		Expect(result.SignalMode).To(Equal("reactive"))
		Expect(result.NormalizedType).To(Equal("CrashLoopBackOff"))
		Expect(result.OriginalSignalType).To(BeEmpty())
	})

	// UT-SP-106-004: Preserve OriginalSignalType for predictive signals
	It("UT-SP-106-004: should preserve OriginalSignalType for all predictive mappings", func() {
		createAndLoadConfig(standardConfig)

		// Test all mapped predictive types
		entries := []struct {
			input          string
			expectedType   string
			expectedOriginal string
		}{
			{"PredictedOOMKill", "OOMKilled", "PredictedOOMKill"},
			{"PredictedCPUThrottling", "CPUThrottling", "PredictedCPUThrottling"},
			{"PredictedDiskPressure", "DiskPressure", "PredictedDiskPressure"},
			{"PredictedNodeNotReady", "NodeNotReady", "PredictedNodeNotReady"},
		}

		for _, e := range entries {
			result := signalModeClassifier.Classify(e.input)
			Expect(result.SignalMode).To(Equal("predictive"), "mode for %s", e.input)
			Expect(result.NormalizedType).To(Equal(e.expectedType), "normalized type for %s", e.input)
			Expect(result.OriginalSignalType).To(Equal(e.expectedOriginal), "original for %s", e.input)
		}
	})

	// UT-SP-106-005: Empty/nil signal type handling
	It("UT-SP-106-005: should classify empty signal type as reactive", func() {
		createAndLoadConfig(standardConfig)

		result := signalModeClassifier.Classify("")

		Expect(result.SignalMode).To(Equal("reactive"))
		Expect(result.NormalizedType).To(BeEmpty())
		Expect(result.OriginalSignalType).To(BeEmpty())
	})

	// UT-SP-106-006: Config loading from YAML file
	Describe("Config Loading", func() {
		It("UT-SP-106-006: should load config from YAML file", func() {
			configPath := filepath.Join(configDir, "predictive-signal-mappings.yaml")
			err := os.WriteFile(configPath, []byte(standardConfig), 0644)
			Expect(err).NotTo(HaveOccurred())

			c := classifier.NewSignalModeClassifier(logger)
			err = c.LoadConfig(configPath)
			Expect(err).NotTo(HaveOccurred())

			// Verify loaded config works
			result := c.Classify("PredictedOOMKill")
			Expect(result.SignalMode).To(Equal("predictive"))
			Expect(result.NormalizedType).To(Equal("OOMKilled"))
		})

		It("UT-SP-106-006b: should return error for missing config file", func() {
			c := classifier.NewSignalModeClassifier(logger)
			err := c.LoadConfig("/nonexistent/path.yaml")
			Expect(err).To(HaveOccurred())
		})

		It("UT-SP-106-006c: should return error for invalid YAML", func() {
			configPath := filepath.Join(configDir, "bad.yaml")
			err := os.WriteFile(configPath, []byte("not: valid: yaml: ["), 0644)
			Expect(err).NotTo(HaveOccurred())

			c := classifier.NewSignalModeClassifier(logger)
			err = c.LoadConfig(configPath)
			Expect(err).To(HaveOccurred())
		})

		It("UT-SP-106-006d: should handle empty mappings gracefully", func() {
			createAndLoadConfig("predictive_signal_mappings: {}\n")

			// All signals should default to reactive
			result := signalModeClassifier.Classify("PredictedOOMKill")
			Expect(result.SignalMode).To(Equal("reactive"))
			Expect(result.NormalizedType).To(Equal("PredictedOOMKill"))
		})
	})

	// UT-SP-106-007: Hot-reload config change
	Describe("Hot-reload", func() {
		It("UT-SP-106-007: should pick up new mappings after config reload", func() {
			// Initial config without PredictedMemoryLeak
			createAndLoadConfig(standardConfig)

			result := signalModeClassifier.Classify("PredictedMemoryLeak")
			Expect(result.SignalMode).To(Equal("reactive"), "before reload: unmapped type should be reactive")

			// Update config with new mapping
			updatedConfig := `predictive_signal_mappings:
  PredictedOOMKill: OOMKilled
  PredictedCPUThrottling: CPUThrottling
  PredictedDiskPressure: DiskPressure
  PredictedNodeNotReady: NodeNotReady
  PredictedMemoryLeak: MemoryLeak
`
			configPath := filepath.Join(configDir, "predictive-signal-mappings.yaml")
			err := os.WriteFile(configPath, []byte(updatedConfig), 0644)
			Expect(err).NotTo(HaveOccurred())

			// Reload
			err = signalModeClassifier.LoadConfig(configPath)
			Expect(err).NotTo(HaveOccurred())

			result = signalModeClassifier.Classify("PredictedMemoryLeak")
			Expect(result.SignalMode).To(Equal("predictive"), "after reload: new mapping should work")
			Expect(result.NormalizedType).To(Equal("MemoryLeak"))
			Expect(result.OriginalSignalType).To(Equal("PredictedMemoryLeak"))
		})
	})

	// Case sensitivity - signal types are case-sensitive (exact match)
	Describe("Case sensitivity", func() {
		It("should treat signal types as case-sensitive", func() {
			createAndLoadConfig(standardConfig)

			// Lowercase variant should NOT match (case-sensitive lookup)
			result := signalModeClassifier.Classify("predictedoomkill")
			Expect(result.SignalMode).To(Equal("reactive"))
			Expect(result.NormalizedType).To(Equal("predictedoomkill"))
		})
	})
})
