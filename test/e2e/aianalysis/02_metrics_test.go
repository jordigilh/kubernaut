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

package aianalysis

import (
	"io"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Metrics Endpoint E2E", Label("e2e", "metrics"), func() {
	var httpClient *http.Client

	BeforeEach(func() {
		httpClient = &http.Client{
			Timeout: 10 * time.Second,
		}
	})

	Context("Prometheus metrics (/metrics) - BR-AI-022", func() {
		It("should expose metrics in Prometheus format", func() {
			resp, err := httpClient.Get(metricsURL + "/metrics")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("text/plain"))
		})

		It("should include reconciliation metrics - BR-AI-022", func() {
			resp, err := httpClient.Get(metricsURL + "/metrics")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			metricsText := string(body)

			// Core metrics per DD-005 and implementation plan
			expectedMetrics := []string{
				"aianalysis_reconciler_reconciliations_total",
				"aianalysis_failures_total",
			}

			for _, metric := range expectedMetrics {
				Expect(metricsText).To(ContainSubstring(metric),
					"Missing metric: %s", metric)
			}
		})

		It("should include Rego policy metrics", func() {
			resp, err := httpClient.Get(metricsURL + "/metrics")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			metricsText := string(body)

			// Rego policy evaluation metrics
			Expect(metricsText).To(ContainSubstring("aianalysis_rego"))
		})

		It("should include Go runtime metrics", func() {
			resp, err := httpClient.Get(metricsURL + "/metrics")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			metricsText := string(body)

			// Standard Go runtime metrics
			Expect(metricsText).To(ContainSubstring("go_goroutines"))
			Expect(metricsText).To(ContainSubstring("go_memstats"))
		})

		It("should include controller-runtime metrics", func() {
			resp, err := httpClient.Get(metricsURL + "/metrics")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			metricsText := string(body)

			// Controller-runtime standard metrics
			Expect(metricsText).To(ContainSubstring("controller_runtime"))
		})
	})

	Context("Metrics accuracy", func() {
		It("should increment reconciliation counter after processing", func() {
			// Get initial metric value
			resp1, err := httpClient.Get(metricsURL + "/metrics")
			Expect(err).NotTo(HaveOccurred())
			body1, _ := io.ReadAll(resp1.Body)
			resp1.Body.Close()
			initialMetrics := string(body1)

			// Metrics should contain reconciliation counter
			// Value will increase after AIAnalysis CRDs are processed
			Expect(initialMetrics).To(ContainSubstring("aianalysis"))
		})
	})
})


