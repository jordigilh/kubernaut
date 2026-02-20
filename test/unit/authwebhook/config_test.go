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

package authwebhook

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	awconfig "github.com/jordigilh/kubernaut/pkg/authwebhook/config"
)

// ========================================
// CONFIG VALIDATION UNIT TESTS
// ========================================
//
// ADR-030: Service Configuration Management
//
// Purpose: Validate that AuthWebhook Config.Validate() correctly rejects
// invalid configuration and that LoadFromFile handles graceful degradation.

var _ = Describe("AuthWebhook Config - Unit Tests", Label("config", "validation", "ADR-030"), func() {

	// ========================================
	// DefaultConfig Characterization
	// ========================================
	Context("DefaultConfig", func() {
		It("should return a valid config with sensible defaults", func() {
			cfg := awconfig.DefaultConfig()
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.Validate()).To(Succeed())
		})

		It("should set expected default values", func() {
			cfg := awconfig.DefaultConfig()
			Expect(cfg.Webhook.Port).To(Equal(9443))
			Expect(cfg.Webhook.CertDir).To(Equal("/tmp/k8s-webhook-server/serving-certs"))
			Expect(cfg.Webhook.HealthProbeAddr).To(Equal(":8081"))
			Expect(cfg.DataStorage.URL).To(Equal("http://data-storage-service:8080"))
		})
	})

	// ========================================
	// LoadFromFile
	// ========================================
	Context("LoadFromFile", func() {
		It("should load valid configuration from YAML file", func() {
			path := filepath.Join("config", "testdata", "valid-config.yaml")
			cfg, err := awconfig.LoadFromFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.Validate()).To(Succeed())
			Expect(cfg.Webhook.Port).To(Equal(9443))
		})

		It("should return defaults gracefully when file does not exist", func() {
			// ADR-030: Graceful degradation on file-not-found
			// RED: This test will FAIL with current AuthWebhook LoadFromFile
			// which returns (nil, error) instead of (defaults, nil)
			cfg, err := awconfig.LoadFromFile("/nonexistent/path/config.yaml")
			Expect(err).NotTo(HaveOccurred(), "LoadFromFile should gracefully fall back to defaults")
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.Webhook.Port).To(Equal(9443))
		})

		It("should return defaults gracefully when YAML is malformed", func() {
			tmpDir := GinkgoT().TempDir()
			malformedPath := filepath.Join(tmpDir, "malformed.yaml")
			Expect(os.WriteFile(malformedPath, []byte("{{invalid yaml:::"), 0644)).To(Succeed())

			// RED: Current LoadFromFile returns (nil, error) for malformed YAML
			cfg, err := awconfig.LoadFromFile(malformedPath)
			Expect(err).NotTo(HaveOccurred(), "LoadFromFile should gracefully fall back to defaults")
			Expect(cfg).NotTo(BeNil())
		})
	})

	// ========================================
	// Validate - Invalid Configurations
	// ========================================
	Context("Validate rejects invalid configs", func() {
		It("should reject invalid port", func() {
			cfg := awconfig.DefaultConfig()
			cfg.Webhook.Port = -1
			Expect(cfg.Validate()).To(MatchError(ContainSubstring("webhook.port")))
		})

		It("should reject empty certDir", func() {
			cfg := awconfig.DefaultConfig()
			cfg.Webhook.CertDir = ""
			Expect(cfg.Validate()).To(MatchError(ContainSubstring("certDir")))
		})

		It("should reject empty healthProbeAddr", func() {
			cfg := awconfig.DefaultConfig()
			cfg.Webhook.HealthProbeAddr = ""
			Expect(cfg.Validate()).To(MatchError(ContainSubstring("healthProbeAddr")))
		})

		It("ADR-030: should reject empty DataStorage URL", func() {
			cfg := awconfig.DefaultConfig()
			cfg.DataStorage.URL = ""
			Expect(cfg.Validate()).To(MatchError(ContainSubstring("datastorage.url")))
		})

		It("should reject config loaded from invalid YAML testdata", func() {
			path := filepath.Join("config", "testdata", "invalid-config.yaml")
			cfg, err := awconfig.LoadFromFile(path)
			if err != nil {
				// LoadFromFile may return error for invalid YAML
				return
			}
			// If loaded, manual Validate should fail
			Expect(cfg.Validate()).To(HaveOccurred())
		})
	})
})
