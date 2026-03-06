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

package datastorage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Moved from test/e2e/datastorage/11_connection_pool_exhaustion_test.go (#194-E).
// The <1s recovery threshold cannot be met in the Kind/Podman E2E environment.
var _ = Describe("Connection Pool Recovery (BR-DS-006)", func() {
	var httpClient *http.Client

	BeforeEach(func() {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	})

	It("should recover gracefully after burst subsides", func() {
		var wg sync.WaitGroup
		testID := fmt.Sprintf("test-pool-%s", uuid.New().String()[:8])

		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				defer GinkgoRecover()

				auditEvent := map[string]interface{}{
					"version":         "1.0",
					"event_type":      "workflow.completed",
					"event_timestamp": time.Now().UTC().Format(time.RFC3339Nano),
					"event_category":  "workflow",
					"event_action":    "completed",
					"event_outcome":   "success",
					"actor_type":      "service",
					"actor_id":        "workflow-service",
					"resource_type":   "Workflow",
					"resource_id":     fmt.Sprintf("wf-recovery-%s-%d", testID, index),
					"correlation_id":  fmt.Sprintf("remediation-recovery-%s-%d", testID, index),
					"event_data": map[string]interface{}{
						"recovery_test": true,
					},
				}

				payloadBytes, _ := json.Marshal(auditEvent)
				req, _ := http.NewRequest("POST", datastorageURL+"/api/v1/audit/events", bytes.NewBuffer(payloadBytes))
				req.Header.Set("Content-Type", "application/json")

				resp, err := httpClient.Do(req)
				if err == nil {
					_ = resp.Body.Close()
				}
			}(i)
		}

		wg.Wait()

		var normalDuration time.Duration
		var normalResp *http.Response
		Eventually(func() bool {
			normalEvent := map[string]interface{}{
				"version":         "1.0",
				"event_type":      "workflow.completed",
				"event_timestamp": time.Now().UTC().Format(time.RFC3339Nano),
				"event_category":  "workflow",
				"event_action":    "completed",
				"event_outcome":   "success",
				"actor_type":      "service",
				"actor_id":        "workflow-service",
				"resource_type":   "Workflow",
				"resource_id":     fmt.Sprintf("wf-normal-%s", testID),
				"correlation_id":  fmt.Sprintf("remediation-normal-%s", testID),
				"event_data": map[string]interface{}{
					"normal_after_burst": true,
				},
			}

			payloadBytes, err := json.Marshal(normalEvent)
			if err != nil {
				return false
			}

			normalStart := time.Now()
			req, _ := http.NewRequest("POST", datastorageURL+"/api/v1/audit/events", bytes.NewBuffer(payloadBytes))
			req.Header.Set("Content-Type", "application/json")

			resp, err := httpClient.Do(req)
			normalDuration = time.Since(normalStart)

			if err != nil || resp == nil {
				return false
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
				return false
			}
			if normalDuration >= 1*time.Second {
				return false
			}

			normalResp = resp
			return true
		}, 90*time.Second, 500*time.Millisecond).Should(BeTrue(),
			"Connection pool MUST recover after burst - normal request should succeed quickly (<1s)")

		Expect(normalResp.StatusCode).To(SatisfyAny(
			Equal(http.StatusCreated),
			Equal(http.StatusAccepted),
		))
		Expect(normalDuration).To(BeNumerically("<", 1*time.Second),
			"Connection pool should recover - normal request should be fast")
	})
})
