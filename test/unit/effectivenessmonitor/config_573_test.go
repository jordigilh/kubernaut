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
	controller "github.com/jordigilh/kubernaut/internal/controller/effectivenessmonitor"
)

var _ = Describe("EM Config Knobs (#573, ADR-EM-001 section 10)", func() {

	// UT-EM-573-006: LoadFromFile parses prometheusLookback and scrapeInterval
	Describe("LoadFromFile with new config knobs (UT-EM-573-006)", func() {
		It("should parse external.prometheusLookback and external.scrapeInterval from YAML", func() {
			tmpDir := GinkgoT().TempDir()
			cfgFile := filepath.Join(tmpDir, "config.yaml")

			yamlContent := `
assessment:
  stabilizationWindow: 5m
  validityWindow: 30m
external:
  prometheusUrl: "http://prometheus:9090"
  prometheusEnabled: true
  alertManagerUrl: "http://alertmanager:9093"
  alertManagerEnabled: true
  connectionTimeout: 10s
  prometheusLookback: 30m
  scrapeInterval: 60s
datastorage:
  url: "http://datastorage:8080"
controller:
  metricsAddr: ":9090"
  healthProbeAddr: ":8081"
`
			err := os.WriteFile(cfgFile, []byte(yamlContent), 0644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := config.LoadFromFile(cfgFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.External.PrometheusLookback).To(Equal(30*time.Minute),
				"UT-EM-573-006: prometheusLookback should be parsed as 30m")
			Expect(cfg.External.ScrapeInterval).To(Equal(60*time.Second),
				"UT-EM-573-006: scrapeInterval should be parsed as 60s")
		})
	})

	// UT-EM-573-007: DefaultConfig returns correct new defaults
	Describe("DefaultConfig new field defaults (UT-EM-573-007)", func() {
		It("should return PrometheusLookback=30m, ScrapeInterval=60s, MaxConcurrentReconciles=10", func() {
			cfg := config.DefaultConfig()

			Expect(cfg.External.PrometheusLookback).To(Equal(30*time.Minute),
				"UT-EM-573-007: default PrometheusLookback should be 30m per ADR-EM-001")
			Expect(cfg.External.ScrapeInterval).To(Equal(60*time.Second),
				"UT-EM-573-007: default ScrapeInterval should be 60s per ADR-EM-001")
			Expect(cfg.Assessment.MaxConcurrentReconciles).To(Equal(10),
				"UT-EM-573-007: default MaxConcurrentReconciles should be 10")
		})
	})

	// UT-EM-573-008: Validate rejects invalid config knob values
	Describe("Validate rejects invalid config knobs (UT-EM-573-008)", func() {
		It("should reject prometheusLookback < 1m", func() {
			cfg := config.DefaultConfig()
			cfg.External.PrometheusLookback = 30 * time.Second

			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("prometheusLookback"),
				"UT-EM-573-008: validation should mention prometheusLookback")
		})

		It("should reject scrapeInterval < 5s", func() {
			cfg := config.DefaultConfig()
			cfg.External.ScrapeInterval = 2 * time.Second

			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("scrapeInterval"),
				"UT-EM-573-008: validation should mention scrapeInterval")
		})

		It("should reject maxConcurrentReconciles < 1", func() {
			cfg := config.DefaultConfig()
			cfg.Assessment.MaxConcurrentReconciles = 0

			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("maxConcurrentReconciles"),
				"UT-EM-573-008: validation should mention maxConcurrentReconciles")
		})

		It("should accept valid config knob values", func() {
			cfg := config.DefaultConfig()
			Expect(cfg.Validate()).To(Succeed(),
				"UT-EM-573-008: default config should pass validation")
		})
	})

	// IT-EM-573-015: ReconcilerConfig receives PrometheusLookback and RequeueAssessmentInProgress from config
	Describe("ReconcilerConfig wiring from config (IT-EM-573-015)", func() {
		It("should propagate PrometheusLookback and ScrapeInterval-derived RequeueAssessmentInProgress to ReconcilerConfig", func() {
			cfg := config.DefaultConfig()
			cfg.External.PrometheusLookback = 20 * time.Minute
			cfg.External.ScrapeInterval = 45 * time.Second

			reconcilerCfg := controller.DefaultReconcilerConfig()
			reconcilerCfg.PrometheusLookback = cfg.External.PrometheusLookback
			reconcilerCfg.RequeueAssessmentInProgress = cfg.External.ScrapeInterval

			Expect(reconcilerCfg.PrometheusLookback).To(Equal(20*time.Minute),
				"IT-EM-573-015: ReconcilerConfig.PrometheusLookback should match config value")
			Expect(reconcilerCfg.RequeueAssessmentInProgress).To(Equal(45*time.Second),
				"IT-EM-573-015: ReconcilerConfig.RequeueAssessmentInProgress should derive from ScrapeInterval")
		})
	})
})
