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

// Package signalprocessing contains unit tests for Signal Processing controller.
// Unit tests validate implementation correctness, not business value delivery.
// See docs/development/business-requirements/TESTING_GUIDELINES.md
package signalprocessing

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	sharedconfig "github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/config"
)

// Unit Test: Config.Validate implementation correctness
var _ = Describe("Config.Validate", func() {
	// Test 1: Function behavior for valid input
	It("should return nil for valid configuration with all required fields", func() {
		cfg := &config.Config{
			Enrichment: config.EnrichmentConfig{
				CacheTTL: 5 * time.Minute,
				Timeout:  2 * time.Second,
			},
			Classifier: config.ClassifierConfig{
				RegoConfigMapName: "signalprocessing-rego-policies",
				RegoConfigMapKey:  "policy.rego",
				HotReloadInterval: 30 * time.Second,
			},
			DataStorage: sharedconfig.DefaultDataStorageConfig(),
		}
		err := cfg.Validate()
		Expect(err).NotTo(HaveOccurred())
	})

	// Test 2: Error handling for zero enrichment timeout
	Context("when enrichment timeout is invalid", func() {
		It("should return error for zero timeout", func() {
			cfg := &config.Config{
				Enrichment: config.EnrichmentConfig{
					CacheTTL: 5 * time.Minute,
					Timeout:  0, // Invalid
				},
				Classifier: config.ClassifierConfig{
					RegoConfigMapName: "signalprocessing-rego-policies",
					RegoConfigMapKey:  "policy.rego",
					HotReloadInterval: 30 * time.Second,
				},
				DataStorage: sharedconfig.DefaultDataStorageConfig(),
			}
			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("enrichment timeout"))
		})
	})

	// Test 3: Error handling for empty Rego ConfigMap name
	Context("when Rego ConfigMap name is missing", func() {
		It("should return error for empty ConfigMap name", func() {
			cfg := &config.Config{
				Enrichment: config.EnrichmentConfig{
					CacheTTL: 5 * time.Minute,
					Timeout:  2 * time.Second,
				},
				Classifier: config.ClassifierConfig{
					RegoConfigMapName: "", // Invalid
					RegoConfigMapKey:  "policy.rego",
					HotReloadInterval: 30 * time.Second,
				},
				DataStorage: sharedconfig.DefaultDataStorageConfig(),
			}
			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Rego ConfigMap name"))
		})
	})

	// Test 4: Error handling for zero hot-reload interval
	Context("when hot-reload interval is invalid", func() {
		It("should return error for zero hot-reload interval", func() {
			cfg := &config.Config{
				Enrichment: config.EnrichmentConfig{
					CacheTTL: 5 * time.Minute,
					Timeout:  2 * time.Second,
				},
				Classifier: config.ClassifierConfig{
					RegoConfigMapName: "signalprocessing-rego-policies",
					RegoConfigMapKey:  "policy.rego",
					HotReloadInterval: 0, // Invalid
				},
				DataStorage: sharedconfig.DefaultDataStorageConfig(),
			}
			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("hot-reload interval"))
		})
	})

	// Test 5: Error handling for empty DataStorage URL
	Context("when DataStorage URL is empty", func() {
		It("should return error for empty DataStorage URL", func() {
			cfg := &config.Config{
				Enrichment: config.EnrichmentConfig{
					CacheTTL: 5 * time.Minute,
					Timeout:  2 * time.Second,
				},
				Classifier: config.ClassifierConfig{
					RegoConfigMapName: "signalprocessing-rego-policies",
					RegoConfigMapKey:  "policy.rego",
					HotReloadInterval: 30 * time.Second,
				},
				DataStorage: sharedconfig.DataStorageConfig{
					URL:     "", // Invalid
					Timeout: 10 * time.Second,
					Buffer:  sharedconfig.DefaultDataStorageConfig().Buffer,
				},
			}
			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("datastorage.url"))
		})
	})

	// Test 6: Error handling for zero buffer size
	Context("when DataStorage buffer size is invalid", func() {
		It("should return error for zero buffer size", func() {
			cfg := &config.Config{
				Enrichment: config.EnrichmentConfig{
					CacheTTL: 5 * time.Minute,
					Timeout:  2 * time.Second,
				},
				Classifier: config.ClassifierConfig{
					RegoConfigMapName: "signalprocessing-rego-policies",
					RegoConfigMapKey:  "policy.rego",
					HotReloadInterval: 30 * time.Second,
				},
				DataStorage: sharedconfig.DataStorageConfig{
					URL:     "http://data-storage:8080",
					Timeout: 10 * time.Second,
					Buffer: sharedconfig.BufferConfig{
						BufferSize:    0, // Invalid
						BatchSize:     100,
						FlushInterval: 1 * time.Second,
						MaxRetries:    3,
					},
				},
			}
			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("bufferSize"))
		})
	})
})

// Unit Test: DefaultControllerConfig implementation correctness
var _ = Describe("DefaultControllerConfig", func() {
	It("CFG-01: should return config with default values", func() {
		cfg := config.DefaultControllerConfig()

		Expect(cfg).ToNot(BeNil())
		Expect(cfg.MetricsAddr).To(Equal(":8080"))
		Expect(cfg.HealthProbeAddr).To(Equal(":8081"))
		Expect(cfg.LeaderElection).To(BeFalse())
		Expect(cfg.LeaderElectionID).To(Equal("signalprocessing.kubernaut.ai"))
	})
})

// Unit Test: LoadFromFile implementation correctness
var _ = Describe("LoadFromFile", func() {
	var tempDir string

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "sp-config-test")
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	It("CFG-02: should load config from valid YAML file", func() {
		configYAML := `
enrichment:
  cacheTtl: 5m
  timeout: 2s
classifier:
  regoConfigMapName: signalprocessing-rego-policies
  regoConfigMapKey: policy.rego
  hotReloadInterval: 30s
datastorage:
  url: http://data-storage:8080
  timeout: 10s
  buffer:
    bufferSize: 1000
    batchSize: 100
    flushInterval: 1s
    maxRetries: 3
`
		configPath := filepath.Join(tempDir, "config.yaml")
		err := os.WriteFile(configPath, []byte(configYAML), 0644)
		Expect(err).ToNot(HaveOccurred())

		cfg, err := config.LoadFromFile(configPath)
		Expect(err).ToNot(HaveOccurred())
		Expect(cfg).ToNot(BeNil())
		Expect(cfg.Classifier.RegoConfigMapName).To(Equal("signalprocessing-rego-policies"))
		Expect(cfg.DataStorage.Buffer.BufferSize).To(Equal(1000))
	})

	It("CFG-03: should return error for missing file", func() {
		cfg, err := config.LoadFromFile("/nonexistent/config.yaml")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to read config file"))
		Expect(cfg).To(BeNil())
	})

	It("CFG-04: should return error for invalid YAML", func() {
		invalidYAML := `
enrichment:
  cachettl: [invalid yaml structure
`
		configPath := filepath.Join(tempDir, "invalid.yaml")
		err := os.WriteFile(configPath, []byte(invalidYAML), 0644)
		Expect(err).ToNot(HaveOccurred())

		cfg, err := config.LoadFromFile(configPath)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to parse config file"))
		Expect(cfg).To(BeNil())
	})
})
