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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/aianalysis/client"
)

// BR-AI-006: HolmesGPT-API Client Unit Tests
var _ = Describe("HolmesGPTClient", func() {
	var (
		ctx        context.Context
		mockServer *httptest.Server
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	AfterEach(func() {
		if mockServer != nil {
			mockServer.Close()
		}
	})

	// BR-AI-006: API call construction
	Describe("Investigate", func() {
		Context("with successful response", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Verify request format
					Expect(r.URL.Path).To(Equal("/api/v1/incident/analyze"))
					Expect(r.Method).To(Equal(http.MethodPost))
					Expect(r.Header.Get("Content-Type")).To(Equal("application/json"))

					// Return success response
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)

					resp := map[string]interface{}{
						"incident_id":           "test-incident",
						"analysis":              "Root cause: OOMKilled due to memory leak",
						"root_cause_analysis":   map[string]interface{}{},
						"confidence":            0.85,
						"timestamp":             "2025-12-04T10:00:00Z",
						"target_in_owner_chain": true,
						"warnings":              []string{},
					}
					json.NewEncoder(w).Encode(resp)
				}))
			})

			It("should return valid response - BR-AI-006", func() {
				hgClient, err := client.NewClient(client.Config{
					BaseURL: mockServer.URL,
				})
				Expect(err).NotTo(HaveOccurred())

				resp, err := hgClient.Investigate(ctx, &client.IncidentRequest{
					IncidentID:    "test-incident",
					RemediationID: "rem-123",
					SignalType:    "OOMKilled",
					Severity:      "warning",
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(resp).NotTo(BeNil())
				Expect(resp.Analysis).To(ContainSubstring("OOMKilled"))
				Expect(resp.Confidence).To(BeNumerically("~", 0.85, 0.01))
				Expect(resp.TargetInOwnerChain).To(BeTrue())
			})
		})

		// BR-AI-008: Handle warnings in response
		Context("with warnings in response", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)

					resp := map[string]interface{}{
						"incident_id":           "test-incident",
						"analysis":              "Analysis with warnings",
						"root_cause_analysis":   map[string]interface{}{},
						"confidence":            0.65,
						"timestamp":             "2025-12-04T10:00:00Z",
						"target_in_owner_chain": false,
						"warnings":              []string{"Low confidence", "OwnerChain validation failed"},
					}
					json.NewEncoder(w).Encode(resp)
				}))
			})

			It("should capture warnings in response - BR-AI-008", func() {
				hgClient, err := client.NewClient(client.Config{
					BaseURL: mockServer.URL,
				})
				Expect(err).NotTo(HaveOccurred())

				resp, err := hgClient.Investigate(ctx, &client.IncidentRequest{
					IncidentID: "test-incident",
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.Warnings).To(HaveLen(2))
				Expect(resp.TargetInOwnerChain).To(BeFalse())
			})
		})

		// BR-AI-009: Transient error handling
		DescribeTable("error classification",
			func(statusCode int, expectedTransient bool) {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(statusCode)
				}))

				hgClient, err := client.NewClient(client.Config{
					BaseURL: mockServer.URL,
				})
				Expect(err).NotTo(HaveOccurred())

				_, err = hgClient.Investigate(ctx, &client.IncidentRequest{})

				Expect(err).To(HaveOccurred())
				var apiErr *client.APIError
				Expect(err).To(BeAssignableToTypeOf(&client.APIError{}))
				apiErr = err.(*client.APIError)
				Expect(apiErr.IsTransient()).To(Equal(expectedTransient))
			},
			Entry("429 Too Many Requests - transient", 429, true),
			Entry("502 Bad Gateway - transient", 502, true),
			Entry("503 Service Unavailable - transient", 503, true),
			Entry("504 Gateway Timeout - transient", 504, true),
			Entry("401 Unauthorized - permanent", 401, false),
			Entry("400 Bad Request - permanent", 400, false),
			Entry("404 Not Found - permanent", 404, false),
			Entry("500 Internal Server Error - permanent", 500, false),
		)
	})
})
