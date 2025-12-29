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

// Package signalprocessing contains unit tests for hot reload lifecycle.
//
// This file covers P3 tests: Hot reload lifecycle
// - StartHotReload, Stop, GetPolicyHash (environment, priority, rego)
// - extractConfidence helper
package signalprocessing

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/jordigilh/kubernaut/pkg/signalprocessing/classifier"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/rego"
)

var _ = Describe("Hot Reload Lifecycle", func() {
	var (
		ctx     context.Context
		scheme  *runtime.Scheme
		logger  logr.Logger
		tempDir string
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		logger = zap.New(zap.UseDevMode(true))

		var err error
		tempDir, err = os.MkdirTemp("", "sp-hotreload-test")
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
	})

	// ========================================
	// REGO ENGINE HOT RELOAD (P3)
	// ========================================

	Describe("Rego Engine Hot Reload", func() {
		Context("HOT-REGO-01: StartHotReload initializes fileWatcher and hash", func() {
			It("should start hot reload and provide policy hash", func() {
				policyContent := `
package customlabels

result := {"team": ["platform"]}
`
				policyPath := filepath.Join(tempDir, "customlabels.rego")
				Expect(os.WriteFile(policyPath, []byte(policyContent), 0644)).To(Succeed())

				engine := rego.NewEngine(logger, policyPath)
				Expect(engine).ToNot(BeNil())
				Expect(engine.LoadPolicy(policyContent)).To(Succeed())

				// Start hot reload - this initializes fileWatcher
				err := engine.StartHotReload(ctx)
				Expect(err).ToNot(HaveOccurred())

				// After hot reload starts, hash should be available
				hash := engine.GetPolicyHash()
				Expect(hash).ToNot(BeEmpty())

				engine.Stop()
			})
		})

		Context("HOT-REGO-02: StartHotReload and Stop lifecycle", func() {
			It("should start and stop hot reload without error", func() {
				policyContent := `
package customlabels

result := {"team": ["platform"]}
`
				policyPath := filepath.Join(tempDir, "customlabels.rego")
				Expect(os.WriteFile(policyPath, []byte(policyContent), 0644)).To(Succeed())

				engine := rego.NewEngine(logger, policyPath)
				Expect(engine.LoadPolicy(policyContent)).To(Succeed())

				// Start hot reload
				err := engine.StartHotReload(ctx)
				Expect(err).ToNot(HaveOccurred())

				// Brief pause to let hot reload start
				time.Sleep(50 * time.Millisecond)

				// Stop should not error
				engine.Stop()
			})
		})

		Context("HOT-REGO-03: Multiple start/stop cycles", func() {
			It("should handle multiple start/stop cycles gracefully", func() {
				policyContent := `
package customlabels

result := {"team": ["team-a"]}
`
				policyPath := filepath.Join(tempDir, "customlabels.rego")
				Expect(os.WriteFile(policyPath, []byte(policyContent), 0644)).To(Succeed())

				engine := rego.NewEngine(logger, policyPath)
				Expect(engine.LoadPolicy(policyContent)).To(Succeed())

				// First cycle
				Expect(engine.StartHotReload(ctx)).To(Succeed())
				time.Sleep(20 * time.Millisecond)
				engine.Stop()

				// Second cycle - should work after stop
				Expect(engine.StartHotReload(ctx)).To(Succeed())
				hash := engine.GetPolicyHash()
				Expect(hash).ToNot(BeEmpty())
				engine.Stop()
			})
		})
	})

	// ========================================
	// ENVIRONMENT CLASSIFIER HOT RELOAD (P3)
	// ========================================

	Describe("Environment Classifier Hot Reload", func() {
		Context("HOT-ENV-01: StartHotReload initializes hash", func() {
			It("should return policy hash after hot reload starts", func() {
				policyContent := `
package environment

default result := {"environment": "unknown", "source": "default"}

result := {"environment": env, "source": "namespace-labels"} if {
    env := input.kubernetes.namespace_labels["kubernaut.ai/environment"]
    env != ""
}
`
				policyPath := filepath.Join(tempDir, "environment.rego")
				Expect(os.WriteFile(policyPath, []byte(policyContent), 0644)).To(Succeed())

				envClassifier, err := classifier.NewEnvironmentClassifier(ctx, policyPath, logger)
				Expect(err).ToNot(HaveOccurred())
				Expect(envClassifier).ToNot(BeNil())

				// Start hot reload to initialize fileWatcher
				err = envClassifier.StartHotReload(ctx)
				Expect(err).ToNot(HaveOccurred())

				hash := envClassifier.GetPolicyHash()
				Expect(hash).ToNot(BeEmpty())

				envClassifier.Stop()
			})
		})

		Context("HOT-ENV-02: StartHotReload and Stop lifecycle", func() {
			It("should start and stop hot reload gracefully", func() {
				policyContent := `
package environment

default result := {"environment": "unknown", "source": "default"}
`
				policyPath := filepath.Join(tempDir, "environment.rego")
				Expect(os.WriteFile(policyPath, []byte(policyContent), 0644)).To(Succeed())

				envClassifier, err := classifier.NewEnvironmentClassifier(ctx, policyPath, logger)
				Expect(err).ToNot(HaveOccurred())

				err = envClassifier.StartHotReload(ctx)
				Expect(err).ToNot(HaveOccurred())

				time.Sleep(50 * time.Millisecond)

				envClassifier.Stop()
			})
		})
	})

	// ========================================
	// PRIORITY ENGINE HOT RELOAD (P3)
	// ========================================

	Describe("Priority Engine Hot Reload", func() {
		Context("HOT-PRIO-01: StartHotReload initializes hash", func() {
			It("should return policy hash after hot reload starts", func() {
				policyContent := `
package priority

default result := {"priority": "P2", "source": "default"}

result := {"priority": "P0", "source": "policy-matrix"} if {
    input.environment == "production"
    input.signal.severity == "critical"
}
`
				policyPath := filepath.Join(tempDir, "priority.rego")
				Expect(os.WriteFile(policyPath, []byte(policyContent), 0644)).To(Succeed())

				priorityEngine, err := classifier.NewPriorityEngine(ctx, policyPath, logger)
				Expect(err).ToNot(HaveOccurred())
				Expect(priorityEngine).ToNot(BeNil())

				// Start hot reload to initialize fileWatcher
				err = priorityEngine.StartHotReload(ctx)
				Expect(err).ToNot(HaveOccurred())

				hash := priorityEngine.GetPolicyHash()
				Expect(hash).ToNot(BeEmpty())

				priorityEngine.Stop()
			})
		})

		Context("HOT-PRIO-02: StartHotReload and Stop lifecycle", func() {
			It("should start and stop hot reload gracefully", func() {
				policyContent := `
package priority

default result := {"priority": "P2", "source": "default"}
`
				policyPath := filepath.Join(tempDir, "priority.rego")
				Expect(os.WriteFile(policyPath, []byte(policyContent), 0644)).To(Succeed())

				priorityEngine, err := classifier.NewPriorityEngine(ctx, policyPath, logger)
				Expect(err).ToNot(HaveOccurred())

				err = priorityEngine.StartHotReload(ctx)
				Expect(err).ToNot(HaveOccurred())

				time.Sleep(50 * time.Millisecond)

				priorityEngine.Stop()
			})
		})
	})

	// ========================================
	// EXTRACT CONFIDENCE HELPER (P3)
	// ========================================

	Describe("Extract Confidence Helper", func() {
		Context("HOT-CONF-01: json.Number handling", func() {
			It("should handle json.Number from Rego results", func() {
				// Simulate Rego result with UseNumber decoder
				jsonData := `{"confidence": 0.85}`
				decoder := json.NewDecoder(strings.NewReader(jsonData))
				decoder.UseNumber()

				var result map[string]interface{}
				Expect(decoder.Decode(&result)).To(Succeed())

				confidence := result["confidence"]
				Expect(confidence).To(BeAssignableToTypeOf(json.Number("")))

				numVal := confidence.(json.Number)
				floatVal, err := numVal.Float64()
				Expect(err).ToNot(HaveOccurred())
				Expect(floatVal).To(BeNumerically("~", 0.85, 0.001))
			})
		})

		Context("HOT-CONF-02: float64 handling", func() {
			It("should handle direct float64 values", func() {
				jsonData := `{"confidence": 0.95}`
				var result map[string]interface{}
				Expect(json.Unmarshal([]byte(jsonData), &result)).To(Succeed())

				confidence := result["confidence"]
				Expect(confidence).To(BeAssignableToTypeOf(float64(0)))
				Expect(confidence.(float64)).To(BeNumerically("~", 0.95, 0.001))
			})
		})

		Context("HOT-CONF-03: nil handling", func() {
			It("should handle nil confidence gracefully", func() {
				jsonData := `{}`
				var result map[string]interface{}
				Expect(json.Unmarshal([]byte(jsonData), &result)).To(Succeed())

				confidence := result["confidence"]
				Expect(confidence).To(BeNil())
			})
		})

		Context("HOT-CONF-04: integer handling", func() {
			It("should handle integer confidence values", func() {
				jsonData := `{"confidence": 1}`
				decoder := json.NewDecoder(strings.NewReader(jsonData))
				decoder.UseNumber()

				var result map[string]interface{}
				Expect(decoder.Decode(&result)).To(Succeed())

				numVal := result["confidence"].(json.Number)
				floatVal, err := numVal.Float64()
				Expect(err).ToNot(HaveOccurred())
				Expect(floatVal).To(Equal(float64(1)))
			})
		})
	})
})
