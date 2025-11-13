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

package contextapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/contextapi/datastorage"
	dsmodels "github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"go.uber.org/zap"
)

// ========================================
// BR-INTEGRATION-008, BR-INTEGRATION-009, BR-INTEGRATION-010
// TDD RED Phase: Data Storage HTTP Client Unit Tests
// ========================================
//
// BEHAVIOR: Data Storage HTTP Client makes REST API calls to Data Storage Service
// CORRECTNESS: HTTP requests are properly constructed, responses are correctly parsed
//
// ADR-033: Context API aggregates from Data Storage Service REST API
// ADR-032: No direct PostgreSQL access from Context API
// ========================================

var _ = Describe("DataStorageClient", func() {
	var (
		client     *datastorage.Client
		mockServer *httptest.Server
		logger     *zap.Logger
		ctx        context.Context
	)

	BeforeEach(func() {
		logger, _ = zap.NewDevelopment()
		ctx = context.Background()

		// Create mock HTTP server for Data Storage Service
		mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Default 404 response (tests will override with specific mocks)
			w.WriteHeader(http.StatusNotFound)
		}))

		// Create client pointing to mock server
		client = datastorage.NewClient(mockServer.URL, 5*time.Second, logger)
	})

	AfterEach(func() {
		mockServer.Close()
	})

	// ========================================
	// HTTP Request Construction Tests
	// ========================================

	Describe("GetSuccessRateByIncidentType", func() {
		Context("when Data Storage returns success rate data", func() {
			BeforeEach(func() {
				// Mock successful response
				mockServer.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// CORRECTNESS: Verify HTTP method
					Expect(r.Method).To(Equal(http.MethodGet))

					// CORRECTNESS: Verify endpoint path
					Expect(r.URL.Path).To(Equal("/api/v1/success-rate/incident-type"))

					// CORRECTNESS: Verify query parameters
					Expect(r.URL.Query().Get("incident_type")).To(Equal("pod-oom-killer"))
					Expect(r.URL.Query().Get("time_range")).To(Equal("7d"))
					Expect(r.URL.Query().Get("min_samples")).To(Equal("5"))

					// Return mock response
					response := &dsmodels.IncidentTypeSuccessRateResponse{
						IncidentType:         "pod-oom-killer",
						TimeRange:            "7d",
						TotalExecutions:      100,
						SuccessfulExecutions: 90,
						FailedExecutions:     10,
						SuccessRate:          90.0,
						Confidence:           "high",
						MinSamplesMet:        true,
					}

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(response)
				})
			})

			It("should construct correct HTTP GET request", func() {
				// ACT: Call client method
				result, err := client.GetSuccessRateByIncidentType(ctx, "pod-oom-killer", "7d", 5)

				// ASSERT: No error
				Expect(err).ToNot(HaveOccurred())

				// CORRECTNESS: Verify response data
				Expect(result).ToNot(BeNil())
				Expect(result.IncidentType).To(Equal("pod-oom-killer"))
				Expect(result.TotalExecutions).To(Equal(100))
				Expect(result.SuccessRate).To(Equal(90.0))
				Expect(result.Confidence).To(Equal("high"))
			})
		})

		Context("when incident_type is empty", func() {
			It("should return validation error", func() {
				// ACT: Call with empty incident_type
				_, err := client.GetSuccessRateByIncidentType(ctx, "", "7d", 5)

				// ASSERT: Validation error
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("incident_type cannot be empty"))
			})
		})
	})

	Describe("GetSuccessRateByPlaybook", func() {
		Context("when Data Storage returns playbook success rate", func() {
			BeforeEach(func() {
				mockServer.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// CORRECTNESS: Verify endpoint and parameters
					Expect(r.Method).To(Equal(http.MethodGet))
					Expect(r.URL.Path).To(Equal("/api/v1/success-rate/playbook"))
					Expect(r.URL.Query().Get("playbook_id")).To(Equal("pod-oom-recovery"))
					Expect(r.URL.Query().Get("playbook_version")).To(Equal("v1.2"))

					response := &dsmodels.PlaybookSuccessRateResponse{
						PlaybookID:           "pod-oom-recovery",
						PlaybookVersion:      "v1.2",
						TimeRange:            "30d",
						TotalExecutions:      200,
						SuccessfulExecutions: 180,
						FailedExecutions:     20,
						SuccessRate:          90.0,
						Confidence:           "high",
						MinSamplesMet:        true,
					}

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(response)
				})
			})

			It("should construct correct HTTP request with playbook parameters", func() {
				// ACT
				result, err := client.GetSuccessRateByPlaybook(ctx, "pod-oom-recovery", "v1.2", "30d", 5)

				// ASSERT
				Expect(err).ToNot(HaveOccurred())
				Expect(result.PlaybookID).To(Equal("pod-oom-recovery"))
				Expect(result.PlaybookVersion).To(Equal("v1.2"))
				Expect(result.SuccessRate).To(Equal(90.0))
			})
		})

		Context("when playbook_id is empty", func() {
			It("should return validation error", func() {
				_, err := client.GetSuccessRateByPlaybook(ctx, "", "v1.2", "7d", 5)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("playbook_id cannot be empty"))
			})
		})
	})

	Describe("GetSuccessRateMultiDimensional", func() {
		Context("when Data Storage returns multi-dimensional data", func() {
			BeforeEach(func() {
				mockServer.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// CORRECTNESS: Verify endpoint
					Expect(r.Method).To(Equal(http.MethodGet))
					Expect(r.URL.Path).To(Equal("/api/v1/success-rate/multi-dimensional"))

					// CORRECTNESS: Verify all dimension parameters
					query := r.URL.Query()
					Expect(query.Get("incident_type")).To(Equal("pod-oom-killer"))
					Expect(query.Get("playbook_id")).To(Equal("pod-oom-recovery"))
					Expect(query.Get("action_type")).To(Equal("increase_memory"))

					response := &dsmodels.MultiDimensionalSuccessRateResponse{
						Dimensions: dsmodels.QueryDimensions{
							IncidentType:    "pod-oom-killer",
							PlaybookID:      "pod-oom-recovery",
							PlaybookVersion: "v1.2",
							ActionType:      "increase_memory",
						},
						TimeRange:            "7d",
						TotalExecutions:      50,
						SuccessfulExecutions: 45,
						FailedExecutions:     5,
						SuccessRate:          90.0,
						Confidence:           "medium",
						MinSamplesMet:        true,
					}

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(response)
				})
			})

			It("should construct request with all dimensions", func() {
				// ACT
				result, err := client.GetSuccessRateMultiDimensional(ctx, &datastorage.MultiDimensionalQuery{
					IncidentType:    "pod-oom-killer",
					PlaybookID:      "pod-oom-recovery",
					PlaybookVersion: "v1.2",
					ActionType:      "increase_memory",
					TimeRange:       "7d",
					MinSamples:      5,
				})

				// ASSERT
				Expect(err).ToNot(HaveOccurred())
				Expect(result.Dimensions.IncidentType).To(Equal("pod-oom-killer"))
				Expect(result.Dimensions.PlaybookID).To(Equal("pod-oom-recovery"))
				Expect(result.SuccessRate).To(Equal(90.0))
			})
		})

		Context("when no dimensions are specified", func() {
			It("should return validation error", func() {
				_, err := client.GetSuccessRateMultiDimensional(ctx, &datastorage.MultiDimensionalQuery{
					TimeRange:  "7d",
					MinSamples: 5,
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("at least one dimension"))
			})
		})
	})

	// ========================================
	// Response Parsing Tests
	// ========================================

	Describe("Response Parsing", func() {
		Context("when Data Storage returns 400 Bad Request", func() {
			BeforeEach(func() {
				mockServer.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/problem+json")
					w.WriteHeader(http.StatusBadRequest)
					fmt.Fprint(w, `{"type":"validation-error","title":"Invalid Parameter","detail":"invalid time_range"}`)
				})
			})

			It("should parse RFC 7807 error response", func() {
				_, err := client.GetSuccessRateByIncidentType(ctx, "test", "invalid", 5)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("400"))
				Expect(err.Error()).To(ContainSubstring("validation-error"))
			})
		})

		Context("when Data Storage returns 500 Internal Server Error", func() {
			BeforeEach(func() {
				mockServer.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
					fmt.Fprint(w, `{"type":"internal-error","detail":"database connection failed"}`)
				})
			})

			It("should return error with status code", func() {
				_, err := client.GetSuccessRateByIncidentType(ctx, "test", "7d", 5)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("500"))
			})
		})

		Context("when response body is invalid JSON", func() {
			BeforeEach(func() {
				mockServer.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					fmt.Fprint(w, `{invalid json}`)
				})
			})

			It("should return JSON parsing error", func() {
				_, err := client.GetSuccessRateByIncidentType(ctx, "test", "7d", 5)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("parse"))
			})
		})
	})

	// ========================================
	// Error Handling Tests
	// ========================================

	Describe("Error Handling", func() {
		Context("when Data Storage Service is unreachable", func() {
			BeforeEach(func() {
				// Close mock server to simulate unreachable service
				mockServer.Close()
			})

			It("should return connection error", func() {
				_, err := client.GetSuccessRateByIncidentType(ctx, "test", "7d", 5)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("connection"))
			})
		})

		Context("when context is cancelled", func() {
			It("should return context cancellation error", func() {
				// Create cancelled context
				cancelledCtx, cancel := context.WithCancel(context.Background())
				cancel()

				_, err := client.GetSuccessRateByIncidentType(cancelledCtx, "test", "7d", 5)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("context"))
			})
		})
	})

	// ========================================
	// Timeout/Retry Logic Tests
	// ========================================

	Describe("Timeout Handling", func() {
		Context("when Data Storage Service is slow", func() {
			BeforeEach(func() {
				mockServer.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Simulate slow response (longer than client timeout)
					time.Sleep(500 * time.Millisecond)
					w.WriteHeader(http.StatusOK)
				})

				// Create client with short timeout
				client = datastorage.NewClient(mockServer.URL, 100*time.Millisecond, logger)
			})

			It("should timeout and return error", func() {
				_, err := client.GetSuccessRateByIncidentType(ctx, "test", "7d", 5)

				Expect(err).To(HaveOccurred())
				// Check for context deadline exceeded or timeout-related error
				Expect(err.Error()).To(Or(
					ContainSubstring("timeout"),
					ContainSubstring("deadline exceeded"),
					ContainSubstring("context"),
				))
			})
		})
	})

	Describe("Retry Logic", func() {
		var callCount int

		BeforeEach(func() {
			callCount = 0
			mockServer.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				callCount++
				if callCount < 3 {
					// Fail first 2 attempts
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				// Succeed on 3rd attempt
				response := &dsmodels.IncidentTypeSuccessRateResponse{
					IncidentType:         "test",
					TimeRange:            "7d",
					TotalExecutions:      10,
					SuccessfulExecutions: 9,
					FailedExecutions:     1,
					SuccessRate:          90.0,
					Confidence:           "medium",
					MinSamplesMet:        true,
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(response)
			})
		})

		It("should retry on 503 Service Unavailable and succeed", func() {
			result, err := client.GetSuccessRateByIncidentType(ctx, "test", "7d", 5)

			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			Expect(callCount).To(Equal(3), "Should have retried 2 times before succeeding")
		})
	})
})
