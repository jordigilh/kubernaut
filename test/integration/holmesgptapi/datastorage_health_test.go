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

package holmesgptapi

import (
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/test/infrastructure"
)

var _ = Describe("HolmesGPT API Integration Infrastructure", func() {
	Context("Data Storage Health Check", func() {
		It("should have Data Storage service available", func() {
			// BR-HAPI-250: HAPI requires Data Storage for workflow catalog
			// DD-INTEGRATION-001 v2.0: Infrastructure set up programmatically via Go

			// Use 127.0.0.1 instead of localhost to force IPv4 (DD-TEST-001 v1.2)
			dataStorageURL := fmt.Sprintf("http://127.0.0.1:%d/health", infrastructure.HAPIIntegrationDataStoragePort)

			By(fmt.Sprintf("Checking Data Storage health at %s", dataStorageURL))
			client := &http.Client{Timeout: 5 * time.Second}

			Eventually(func() error {
				resp, err := client.Get(dataStorageURL)
				if err != nil {
					return err
				}
				defer func() { _ = resp.Body.Close() }()

				if resp.StatusCode != http.StatusOK {
					return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
				}
				return nil
			}, 10*time.Second, 1*time.Second).Should(Succeed(),
				"Data Storage should be healthy (infrastructure set up via Go programmatic setup)")

			GinkgoWriter.Println("✅ Data Storage is healthy and accessible")
		})

		It("should have PostgreSQL available via Data Storage connection", func() {
			// Verify Data Storage can connect to PostgreSQL
			// This is an indirect check via Data Storage health endpoint

			// Use 127.0.0.1 instead of localhost to force IPv4 (DD-TEST-001 v1.2)
			dataStorageURL := fmt.Sprintf("http://127.0.0.1:%d/health", infrastructure.HAPIIntegrationDataStoragePort)
			client := &http.Client{Timeout: 5 * time.Second}

			resp, err := client.Get(dataStorageURL)
			Expect(err).ToNot(HaveOccurred(), "Data Storage health check should not error")
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusOK),
				"Data Storage health check should return 200 (implies PostgreSQL connection)")

			GinkgoWriter.Println("✅ PostgreSQL connection verified via Data Storage")
		})
	})

	Context("Infrastructure Port Allocation", func() {
		It("should use correct DD-TEST-001 v1.8 port allocations", func() {
			// Verify HAPI integration uses correct ports per DD-TEST-001 v1.8

			Expect(infrastructure.HAPIIntegrationPostgresPort).To(Equal(15439),
				"PostgreSQL port should be 15439 (HAPI-specific, shared with Notification/WE)")

			Expect(infrastructure.HAPIIntegrationRedisPort).To(Equal(16387),
				"Redis port should be 16387 (HAPI-specific, shared with Notification/WE)")

			Expect(infrastructure.HAPIIntegrationDataStoragePort).To(Equal(18098),
				"Data Storage port should be 18098 (HAPI allocation per DD-TEST-001 v1.8)")

			GinkgoWriter.Println("✅ Port allocations verified (DD-TEST-001 v1.8 compliant)")
		})
	})
})
