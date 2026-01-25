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

var _ = Describe("Health Endpoints E2E", Label("e2e", "health"), func() {
	var httpClient *http.Client

	BeforeEach(func() {
		httpClient = &http.Client{
			Timeout: 10 * time.Second,
		}
	})

	Context("Liveness probe (/healthz) - BR-AI-025", func() {
		It("should return 200 OK when controller is alive", func() {
			resp, err := httpClient.Get(healthURL + "/healthz")
			Expect(err).NotTo(HaveOccurred())
			defer func() {
				if err := resp.Body.Close(); err != nil {
					GinkgoLogr.Error(err, "Failed to close response body")
				}
			}()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(Or(
				ContainSubstring("ok"),
				ContainSubstring("healthy"),
				ContainSubstring("alive"),
			))
		})

		It("should respond quickly (< 1s)", func() {
			start := time.Now()
			resp, err := httpClient.Get(healthURL + "/healthz")
			duration := time.Since(start)

			Expect(err).NotTo(HaveOccurred())
			defer func() {
				if err := resp.Body.Close(); err != nil {
					GinkgoLogr.Error(err, "Failed to close response body")
				}
			}()

			Expect(duration).To(BeNumerically("<", time.Second))
		})
	})

	Context("Readiness probe (/readyz) - BR-AI-025", func() {
		It("should return 200 OK when controller is ready", func() {
			resp, err := httpClient.Get(healthURL + "/readyz")
			Expect(err).NotTo(HaveOccurred())
			defer func() {
				if err := resp.Body.Close(); err != nil {
					GinkgoLogr.Error(err, "Failed to close response body")
				}
			}()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})

		It("should indicate HolmesGPT-API dependency status", func() {
			resp, err := httpClient.Get(healthURL + "/readyz?verbose=true")
			Expect(err).NotTo(HaveOccurred())
			defer func() {
				if err := resp.Body.Close(); err != nil {
					GinkgoLogr.Error(err, "Failed to close response body")
				}
			}()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())

			// Should show dependency health status
			// The exact format depends on implementation
			Expect(len(body)).To(BeNumerically(">", 0))
		})
	})

	Context("Dependency health checks", func() {
		It("should verify HolmesGPT-API is reachable", func() {
			// HolmesGPT-API health endpoint (NodePort 30088 -> host port 8088)
			// Use Eventually to wait for service to be ready
			var resp *http.Response
			var err error
			Eventually(func() error {
				resp, err = httpClient.Get("http://localhost:8088/health")
				return err
			}, 30*time.Second, 500*time.Millisecond).Should(Succeed())
			defer func() {
				if err := resp.Body.Close(); err != nil {
					GinkgoLogr.Error(err, "Failed to close response body")
				}
			}()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})

		It("should verify Data Storage is reachable", func() {
			// Data Storage health endpoint (NodePort 30081 -> host port 8091)
			// Note: Using 8091 to avoid conflicts (8081=AIAnalysis container, 8085=gvproxy)
			// Use Eventually to wait for service to be ready
			var resp *http.Response
			var err error
			Eventually(func() error {
				resp, err = httpClient.Get("http://localhost:8091/health")
				return err
			}, 30*time.Second, 500*time.Millisecond).Should(Succeed())
			defer func() {
				if err := resp.Body.Close(); err != nil {
					GinkgoLogr.Error(err, "Failed to close response body")
				}
			}()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})
	})
})
