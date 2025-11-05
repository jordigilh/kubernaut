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
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
)

// ========================================
// TDD RED PHASE: ADR-033 Aggregation Handlers Unit Tests
// 📋 Authority: IMPLEMENTATION_PLAN_V5.0.md Day 14.1
// 📋 Business Requirements:
//    - BR-STORAGE-031-01: Incident-Type Success Rate API
//    - BR-STORAGE-031-02: Playbook Success Rate API
// 📋 Testing Principle: Behavior + Correctness
// ========================================
//
// TESTING STRATEGY:
// - Mock ActionTraceRepository (external dependency)
// - Test HTTP request/response behavior
// - Validate query parameter parsing
// - Validate RFC 7807 error responses
// - Validate success response structure
//
// ========================================

var _ = Describe("ADR-033 Aggregation Handlers", func() {
	var (
		handler *server.Handler
		req     *http.Request
		rec     *httptest.ResponseRecorder
	)

	BeforeEach(func() {
		// Create handler with minimal setup for TDD GREEN phase
		// TODO: Add ActionTraceRepository when repository integration is needed
		handler = server.NewHandler(nil) // nil DB is fine for now (handlers don't use it yet)
		rec = httptest.NewRecorder()
	})

	// ========================================
	// BR-STORAGE-031-01: Incident-Type Success Rate Handler
	// BEHAVIOR: Parse query params, call repository, return JSON
	// CORRECTNESS: Exact HTTP status codes and response structure
	// ========================================
	Describe("GET /api/v1/success-rate/incident-type", func() {
		Context("with valid query parameters", func() {
			It("should return 200 OK with incident-type success rate data", func() {
				// ARRANGE: Create HTTP request with valid params
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/incident-type?incident_type=HighCPUUsage&time_range=7d&min_samples=5",
					nil,
				)

				// ACT: Call handler
				handler.HandleGetSuccessRateByIncidentType(rec, req)

				// ASSERT: HTTP 200 OK
				Expect(rec.Code).To(Equal(http.StatusOK),
					"Handler should return 200 OK for valid request")

				// ASSERT: Response is valid JSON
				var response models.IncidentTypeSuccessRateResponse
				err := json.NewDecoder(rec.Body).Decode(&response)
				Expect(err).ToNot(HaveOccurred(),
					"Response should be valid JSON")

				// CORRECTNESS: Validate response structure
				Expect(response.IncidentType).To(Equal("HighCPUUsage"),
					"Response should contain requested incident type")
				Expect(response.TimeRange).To(Equal("7d"),
					"Response should contain requested time range")
				Expect(response.SuccessRate).To(BeNumerically(">=", 0.0),
					"Success rate should be non-negative")
				Expect(response.SuccessRate).To(BeNumerically("<=", 100.0),
					"Success rate should be <= 100%")
				Expect(response.TotalExecutions).To(BeNumerically(">=", 0),
					"Total executions should be non-negative")
				Expect(response.SuccessfulExecutions).To(BeNumerically(">=", 0),
					"Successful executions should be non-negative")
				Expect(response.Confidence).To(BeElementOf("high", "medium", "low", "insufficient_data"),
					"Confidence should be one of the valid values")
			})

			It("should use default time_range=7d when not specified", func() {
				// ARRANGE: Request without time_range param
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/incident-type?incident_type=HighCPUUsage",
					nil,
				)

				// ACT: Call handler
				handler.HandleGetSuccessRateByIncidentType(rec, req)

				// ASSERT: HTTP 200 OK
				Expect(rec.Code).To(Equal(http.StatusOK))

				// CORRECTNESS: Default time range applied
				var response models.IncidentTypeSuccessRateResponse
				err := json.NewDecoder(rec.Body).Decode(&response)
				Expect(err).ToNot(HaveOccurred())
				Expect(response.TimeRange).To(Equal("7d"),
					"Handler should default to 7d time range")
			})

			It("should use default min_samples=5 when not specified", func() {
				// ARRANGE: Request without min_samples param
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/incident-type?incident_type=HighCPUUsage&time_range=7d",
					nil,
				)

				// ACT: Call handler
				handler.HandleGetSuccessRateByIncidentType(rec, req)

				// ASSERT: HTTP 200 OK
				Expect(rec.Code).To(Equal(http.StatusOK))

				// CORRECTNESS: Response should be valid (min_samples=5 used internally)
				var response models.IncidentTypeSuccessRateResponse
				err := json.NewDecoder(rec.Body).Decode(&response)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("with missing required parameters", func() {
			It("should return 400 Bad Request when incident_type is missing", func() {
				// ARRANGE: Request without incident_type param
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/incident-type?time_range=7d",
					nil,
				)

				// ACT: Call handler
				handler.HandleGetSuccessRateByIncidentType(rec, req)

				// ASSERT: HTTP 400 Bad Request
				Expect(rec.Code).To(Equal(http.StatusBadRequest),
					"Handler should return 400 when incident_type is missing")

				// CORRECTNESS: RFC 7807 error response
				Expect(rec.Header().Get("Content-Type")).To(ContainSubstring("application/problem+json"),
					"Error response should use RFC 7807 format")
			})
		})

		Context("with invalid query parameters", func() {
			It("should return 400 Bad Request when time_range format is invalid", func() {
				// ARRANGE: Request with invalid time_range
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/incident-type?incident_type=HighCPUUsage&time_range=invalid",
					nil,
				)

				// ACT: Call handler
				handler.HandleGetSuccessRateByIncidentType(rec, req)

				// ASSERT: HTTP 400 Bad Request
				Expect(rec.Code).To(Equal(http.StatusBadRequest),
					"Handler should return 400 for invalid time_range format")

				// CORRECTNESS: RFC 7807 error response
				Expect(rec.Header().Get("Content-Type")).To(ContainSubstring("application/problem+json"))
			})

			It("should return 400 Bad Request when min_samples is not a number", func() {
				// ARRANGE: Request with non-numeric min_samples
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/incident-type?incident_type=HighCPUUsage&min_samples=abc",
					nil,
				)

				// ACT: Call handler
				handler.HandleGetSuccessRateByIncidentType(rec, req)

				// ASSERT: HTTP 400 Bad Request
				Expect(rec.Code).To(Equal(http.StatusBadRequest),
					"Handler should return 400 for non-numeric min_samples")

				// CORRECTNESS: RFC 7807 error response
				Expect(rec.Header().Get("Content-Type")).To(ContainSubstring("application/problem+json"))
			})
		})
	})

	// ========================================
	// BR-STORAGE-031-02: Playbook Success Rate Handler
	// BEHAVIOR: Parse query params, call repository, return JSON
	// CORRECTNESS: Exact HTTP status codes and response structure
	// ========================================
	Describe("GET /api/v1/success-rate/playbook", func() {
		Context("with valid query parameters", func() {
			It("should return 200 OK with playbook success rate data", func() {
				// ARRANGE: Create HTTP request with valid params
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/playbook?playbook_id=restart-pod-v1&time_range=7d&min_samples=5",
					nil,
				)

				// ACT: Call handler
				handler.HandleGetSuccessRateByPlaybook(rec, req)

				// ASSERT: HTTP 200 OK
				Expect(rec.Code).To(Equal(http.StatusOK),
					"Handler should return 200 OK for valid request")

				// ASSERT: Response is valid JSON
				var response models.PlaybookSuccessRateResponse
				err := json.NewDecoder(rec.Body).Decode(&response)
				Expect(err).ToNot(HaveOccurred(),
					"Response should be valid JSON")

				// CORRECTNESS: Validate response structure
				Expect(response.PlaybookID).To(Equal("restart-pod-v1"),
					"Response should contain requested playbook ID")
				Expect(response.TimeRange).To(Equal("7d"),
					"Response should contain requested time range")
				Expect(response.SuccessRate).To(BeNumerically(">=", 0.0),
					"Success rate should be non-negative")
				Expect(response.SuccessRate).To(BeNumerically("<=", 100.0),
					"Success rate should be <= 100%")
				Expect(response.TotalExecutions).To(BeNumerically(">=", 0),
					"Total executions should be non-negative")
				Expect(response.SuccessfulExecutions).To(BeNumerically(">=", 0),
					"Successful executions should be non-negative")
				Expect(response.Confidence).To(BeElementOf("high", "medium", "low", "insufficient_data"),
					"Confidence should be one of the valid values")
			})

			It("should accept optional playbook_version parameter", func() {
				// ARRANGE: Request with playbook_version
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/playbook?playbook_id=restart-pod-v1&playbook_version=1.2.3&time_range=7d",
					nil,
				)

				// ACT: Call handler
				handler.HandleGetSuccessRateByPlaybook(rec, req)

				// ASSERT: HTTP 200 OK
				Expect(rec.Code).To(Equal(http.StatusOK))

				// CORRECTNESS: Version included in response
				var response models.PlaybookSuccessRateResponse
				err := json.NewDecoder(rec.Body).Decode(&response)
				Expect(err).ToNot(HaveOccurred())
				Expect(response.PlaybookVersion).To(Equal("1.2.3"),
					"Response should include playbook version when specified")
			})
		})

		Context("with missing required parameters", func() {
			It("should return 400 Bad Request when playbook_id is missing", func() {
				// ARRANGE: Request without playbook_id param
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/playbook?time_range=7d",
					nil,
				)

				// ACT: Call handler
				handler.HandleGetSuccessRateByPlaybook(rec, req)

				// ASSERT: HTTP 400 Bad Request
				Expect(rec.Code).To(Equal(http.StatusBadRequest),
					"Handler should return 400 when playbook_id is missing")

				// CORRECTNESS: RFC 7807 error response
				Expect(rec.Header().Get("Content-Type")).To(ContainSubstring("application/problem+json"))
			})
		})

		Context("with invalid query parameters", func() {
			It("should return 400 Bad Request when time_range format is invalid", func() {
				// ARRANGE: Request with invalid time_range
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/playbook?playbook_id=restart-pod-v1&time_range=invalid",
					nil,
				)

				// ACT: Call handler
				handler.HandleGetSuccessRateByPlaybook(rec, req)

				// ASSERT: HTTP 400 Bad Request
				Expect(rec.Code).To(Equal(http.StatusBadRequest),
					"Handler should return 400 for invalid time_range format")

				// CORRECTNESS: RFC 7807 error response
				Expect(rec.Header().Get("Content-Type")).To(ContainSubstring("application/problem+json"))
			})
		})
	})

	// ========================================
	// Edge Cases and Error Scenarios
	// ========================================
	Describe("Edge Cases", func() {
		Context("time range parsing", func() {
			It("should accept valid time range formats: 1h, 24h, 7d, 30d", func() {
				validRanges := []string{"1h", "24h", "7d", "30d"}

				for _, timeRange := range validRanges {
					req = httptest.NewRequest(
						http.MethodGet,
						"/api/v1/success-rate/incident-type?incident_type=HighCPUUsage&time_range="+timeRange,
						nil,
					)
					rec = httptest.NewRecorder()

					handler.HandleGetSuccessRateByIncidentType(rec, req)

					Expect(rec.Code).To(Equal(http.StatusOK),
						"Handler should accept time_range=%s", timeRange)
				}
			})

			It("should reject invalid time range formats", func() {
				invalidRanges := []string{"1x", "abc", "7days", "-1d", "0d"}

				for _, timeRange := range invalidRanges {
					req = httptest.NewRequest(
						http.MethodGet,
						"/api/v1/success-rate/incident-type?incident_type=HighCPUUsage&time_range="+timeRange,
						nil,
					)
					rec = httptest.NewRecorder()

					handler.HandleGetSuccessRateByIncidentType(rec, req)

					Expect(rec.Code).To(Equal(http.StatusBadRequest),
						"Handler should reject time_range=%s", timeRange)
				}
			})
		})

		Context("min_samples validation", func() {
			It("should accept positive integers for min_samples", func() {
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/incident-type?incident_type=HighCPUUsage&min_samples=10",
					nil,
				)

				handler.HandleGetSuccessRateByIncidentType(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))
			})

			It("should reject negative min_samples", func() {
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/incident-type?incident_type=HighCPUUsage&min_samples=-5",
					nil,
				)

				handler.HandleGetSuccessRateByIncidentType(rec, req)

				Expect(rec.Code).To(Equal(http.StatusBadRequest),
					"Handler should reject negative min_samples")
			})

			It("should reject zero min_samples", func() {
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/incident-type?incident_type=HighCPUUsage&min_samples=0",
					nil,
				)

				handler.HandleGetSuccessRateByIncidentType(rec, req)

				Expect(rec.Code).To(Equal(http.StatusBadRequest),
					"Handler should reject zero min_samples")
			})
		})

		// ========================================
		// EDGE CASES: Security, Boundaries, Special Characters
		// Testing edge cases that integration tests might reveal
		// ========================================
		Context("edge cases and security", func() {
			It("should handle incident_type with special characters (Kubernetes naming)", func() {
				// BEHAVIOR: Kubernetes-valid special characters should be accepted
				// (hyphens, underscores, dots are valid in Kubernetes labels)
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/incident-type?incident_type=pod-oom-killer_v2.0&time_range=7d",
					nil,
				)

				handler.HandleGetSuccessRateByIncidentType(rec, req)

				// CORRECTNESS: Returns 200 OK
				Expect(rec.Code).To(Equal(http.StatusOK),
					"Special characters (hyphen, underscore, dot) should be accepted")

				var response models.IncidentTypeSuccessRateResponse
				json.NewDecoder(rec.Body).Decode(&response)
				Expect(response.IncidentType).To(Equal("pod-oom-killer_v2.0"),
					"Incident type with special characters should be preserved")
			})

			It("should handle incident_type with URL-encoded spaces", func() {
				// BEHAVIOR: URL-encoded spaces should be decoded correctly
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/incident-type?incident_type=High%20CPU%20Usage&time_range=7d",
					nil,
				)

				handler.HandleGetSuccessRateByIncidentType(rec, req)

				// CORRECTNESS: Returns 200 OK, incident_type decoded to "High CPU Usage"
				Expect(rec.Code).To(Equal(http.StatusOK),
					"URL-encoded incident_type should be decoded")

				var response models.IncidentTypeSuccessRateResponse
				json.NewDecoder(rec.Body).Decode(&response)
				Expect(response.IncidentType).To(Equal("High CPU Usage"),
					"Incident type should be URL-decoded")
			})

			It("should handle very large min_samples value", func() {
				// BEHAVIOR: Very large min_samples should be accepted (no upper limit in spec)
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/incident-type?incident_type=HighCPUUsage&min_samples=1000000",
					nil,
				)

				handler.HandleGetSuccessRateByIncidentType(rec, req)

				// CORRECTNESS: Returns 200 OK (valid integer)
				Expect(rec.Code).To(Equal(http.StatusOK),
					"Very large min_samples should be accepted")
			})

			It("should handle multiple query parameters in different order", func() {
				// BEHAVIOR: Query parameter order should not matter
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/incident-type?time_range=7d&min_samples=10&incident_type=HighCPUUsage",
					nil,
				)

				handler.HandleGetSuccessRateByIncidentType(rec, req)

				// CORRECTNESS: Returns 200 OK
				Expect(rec.Code).To(Equal(http.StatusOK),
					"Query parameter order should not matter")
			})

			It("should handle case-sensitive incident_type", func() {
				// BEHAVIOR: incident_type should be case-sensitive
				// (Kubernetes labels are case-sensitive)
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/incident-type?incident_type=HighCPUUsage&time_range=7d",
					nil,
				)

				handler.HandleGetSuccessRateByIncidentType(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))

				var response models.IncidentTypeSuccessRateResponse
				json.NewDecoder(rec.Body).Decode(&response)
				Expect(response.IncidentType).To(Equal("HighCPUUsage"),
					"Incident type case should be preserved (case-sensitive)")
			})
		})

		// ========================================
		// EDGE CASES: Playbook Endpoint
		// ========================================
		Context("playbook endpoint edge cases", func() {
			It("should handle playbook_id with special characters", func() {
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/playbook?playbook_id=pod-oom-recovery_v2.0&playbook_version=v1.2&time_range=7d",
					nil,
				)

				handler.HandleGetSuccessRateByPlaybook(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK),
					"Special characters in playbook_id should be accepted")

				var response models.PlaybookSuccessRateResponse
				json.NewDecoder(rec.Body).Decode(&response)
				Expect(response.PlaybookID).To(Equal("pod-oom-recovery_v2.0"),
					"Playbook ID with special characters should be preserved")
			})

			It("should handle semantic version formats for playbook_version", func() {
				// BEHAVIOR: Various semantic version formats should be accepted
				validVersions := []string{"v1.0", "v1.2.3", "v2.0.0-alpha", "v1.0.0+build123"}

				for _, version := range validVersions {
					rec = httptest.NewRecorder() // Reset recorder for each test
					req = httptest.NewRequest(
						http.MethodGet,
						"/api/v1/success-rate/playbook?playbook_id=test-playbook&playbook_version="+version+"&time_range=7d",
						nil,
					)

					handler.HandleGetSuccessRateByPlaybook(rec, req)

					Expect(rec.Code).To(Equal(http.StatusOK),
						"Semantic version format %s should be accepted", version)
				}
			})

			It("should handle playbook_version with URL-encoded plus sign", func() {
				// BEHAVIOR: URL-encoded + in version (e.g., v1.0.0+build) should be decoded
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/playbook?playbook_id=test-playbook&playbook_version=v1.0.0%2Bbuild123&time_range=7d",
					nil,
				)

				handler.HandleGetSuccessRateByPlaybook(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK),
					"URL-encoded + in version should be decoded")

				var response models.PlaybookSuccessRateResponse
				json.NewDecoder(rec.Body).Decode(&response)
				Expect(response.PlaybookVersion).To(Equal("v1.0.0+build123"),
					"Playbook version with + should be URL-decoded")
			})
		})
	})

	// ========================================
	// BR-STORAGE-031-05: Multi-Dimensional Success Rate Handler
	// TDD RED Phase: Write failing tests for HandleGetSuccessRateMultiDimensional
	// ========================================
	Describe("HandleGetSuccessRateMultiDimensional", func() {
		Context("with all three dimensions specified", func() {
			It("should return 200 OK with multi-dimensional success rate data", func() {
				// ARRANGE: Create HTTP request with all dimensions
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/multi-dimensional?incident_type=pod-oom-killer&playbook_id=pod-oom-recovery&playbook_version=v1.2&action_type=increase_memory&time_range=7d&min_samples=5",
					nil,
				)

				// ACT: Call handler
				handler.HandleGetSuccessRateMultiDimensional(rec, req)

				// ASSERT: HTTP 200 OK
				Expect(rec.Code).To(Equal(http.StatusOK),
					"Handler should return 200 OK for valid request")

				// ASSERT: Response is valid JSON
				var response models.MultiDimensionalSuccessRateResponse
				err := json.NewDecoder(rec.Body).Decode(&response)
				Expect(err).ToNot(HaveOccurred(),
					"Response should be valid JSON")

				// CORRECTNESS: Validate response structure
				Expect(response.Dimensions.IncidentType).To(Equal("pod-oom-killer"))
				Expect(response.Dimensions.PlaybookID).To(Equal("pod-oom-recovery"))
				Expect(response.Dimensions.PlaybookVersion).To(Equal("v1.2"))
				Expect(response.Dimensions.ActionType).To(Equal("increase_memory"))
				Expect(response.TimeRange).To(Equal("7d"))
			})
		})

		Context("with partial dimensions (incident_type + playbook only)", func() {
			It("should return 200 OK with aggregated data across all action_types", func() {
				// ARRANGE: Create HTTP request with partial dimensions
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/multi-dimensional?incident_type=pod-oom-killer&playbook_id=pod-oom-recovery&playbook_version=v1.2&time_range=7d",
					nil,
				)

				// ACT: Call handler
				handler.HandleGetSuccessRateMultiDimensional(rec, req)

				// ASSERT: HTTP 200 OK
				Expect(rec.Code).To(Equal(http.StatusOK))

				// ASSERT: Response has incident_type + playbook, no action_type
				var response models.MultiDimensionalSuccessRateResponse
				json.NewDecoder(rec.Body).Decode(&response)
				Expect(response.Dimensions.IncidentType).To(Equal("pod-oom-killer"))
				Expect(response.Dimensions.PlaybookID).To(Equal("pod-oom-recovery"))
				Expect(response.Dimensions.ActionType).To(BeEmpty())
			})
		})

		Context("validation errors", func() {
			It("should return 400 Bad Request when playbook_version is specified without playbook_id", func() {
				// ARRANGE: Create HTTP request with invalid params
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/multi-dimensional?incident_type=pod-oom-killer&playbook_version=v1.2",
					nil,
				)

				// ACT: Call handler
				handler.HandleGetSuccessRateMultiDimensional(rec, req)

				// ASSERT: HTTP 400 Bad Request
				Expect(rec.Code).To(Equal(http.StatusBadRequest),
					"Handler should return 400 for invalid params")

				// CORRECTNESS: RFC 7807 error format
				var problem map[string]interface{}
				json.Unmarshal(rec.Body.Bytes(), &problem)
				Expect(problem["type"]).To(Equal("https://api.kubernaut.io/problems/validation-error"))
				Expect(problem["detail"]).To(ContainSubstring("playbook_version requires playbook_id"))
			})

			It("should return 400 Bad Request for invalid time_range", func() {
				// ARRANGE: Create HTTP request with invalid time_range
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/multi-dimensional?incident_type=pod-oom-killer&time_range=invalid",
					nil,
				)

				// ACT: Call handler
				handler.HandleGetSuccessRateMultiDimensional(rec, req)

				// ASSERT: HTTP 400 Bad Request
				Expect(rec.Code).To(Equal(http.StatusBadRequest))

				// CORRECTNESS: Error message mentions time_range
				var problem map[string]interface{}
				json.Unmarshal(rec.Body.Bytes(), &problem)
				Expect(problem["detail"]).To(ContainSubstring("time_range"))
			})

			It("should return 400 Bad Request for invalid min_samples", func() {
				// ARRANGE: Create HTTP request with invalid min_samples
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/multi-dimensional?incident_type=pod-oom-killer&min_samples=invalid",
					nil,
				)

				// ACT: Call handler
				handler.HandleGetSuccessRateMultiDimensional(rec, req)

				// ASSERT: HTTP 400 Bad Request
				Expect(rec.Code).To(Equal(http.StatusBadRequest))

				// CORRECTNESS: Error message mentions min_samples
				var problem map[string]interface{}
				json.Unmarshal(rec.Body.Bytes(), &problem)
				Expect(problem["detail"]).To(ContainSubstring("min_samples"))
			})

			It("should return 400 Bad Request for negative min_samples", func() {
				// ARRANGE: Create HTTP request with negative min_samples
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/multi-dimensional?incident_type=pod-oom-killer&min_samples=-5",
					nil,
				)

				// ACT: Call handler
				handler.HandleGetSuccessRateMultiDimensional(rec, req)

				// ASSERT: HTTP 400 Bad Request
				Expect(rec.Code).To(Equal(http.StatusBadRequest))

				// CORRECTNESS: Error message mentions positive integer
				var problem map[string]interface{}
				json.Unmarshal(rec.Body.Bytes(), &problem)
				Expect(problem["detail"]).To(ContainSubstring("positive"))
			})
		})

		Context("defaults", func() {
			It("should default to 7d time_range when not specified", func() {
				// ARRANGE: Create HTTP request without time_range
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/multi-dimensional?incident_type=pod-oom-killer",
					nil,
				)

				// ACT: Call handler
				handler.HandleGetSuccessRateMultiDimensional(rec, req)

				// ASSERT: HTTP 200 OK
				Expect(rec.Code).To(Equal(http.StatusOK))

				// CORRECTNESS: time_range defaults to "7d"
				var result models.MultiDimensionalSuccessRateResponse
				json.Unmarshal(rec.Body.Bytes(), &result)
				Expect(result.TimeRange).To(Equal("7d"))
			})

			It("should default to 5 min_samples when not specified", func() {
				// ARRANGE: Create HTTP request without min_samples
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/multi-dimensional?incident_type=pod-oom-killer",
					nil,
				)

				// ACT: Call handler
				handler.HandleGetSuccessRateMultiDimensional(rec, req)

				// ASSERT: HTTP 200 OK
				Expect(rec.Code).To(Equal(http.StatusOK))

				// BEHAVIOR: min_samples defaults to 5 (used in confidence calculation)
				var result models.MultiDimensionalSuccessRateResponse
				json.Unmarshal(rec.Body.Bytes(), &result)
				// Response will reflect default min_samples behavior
			})
		})

		Context("edge cases", func() {
			It("should handle special characters in incident_type", func() {
				// BEHAVIOR: Special characters should be URL-encoded and decoded
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/multi-dimensional?incident_type=pod-oom-killer%2Fhigh-memory&time_range=7d",
					nil,
				)

				handler.HandleGetSuccessRateMultiDimensional(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK),
					"Special characters should be URL-decoded")

				var response models.MultiDimensionalSuccessRateResponse
				json.NewDecoder(rec.Body).Decode(&response)
				Expect(response.Dimensions.IncidentType).To(Equal("pod-oom-killer/high-memory"))
			})

			It("should handle large min_samples value", func() {
				// BEHAVIOR: Large min_samples values should be accepted
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/multi-dimensional?incident_type=pod-oom-killer&min_samples=10000&time_range=7d",
					nil,
				)

				handler.HandleGetSuccessRateMultiDimensional(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK),
					"Large min_samples values should be accepted")
			})

			It("should handle query parameter order independence", func() {
				// BEHAVIOR: Parameter order should not affect response
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/multi-dimensional?time_range=7d&action_type=increase_memory&playbook_version=v1.2&incident_type=pod-oom-killer&playbook_id=pod-oom-recovery",
					nil,
				)

				handler.HandleGetSuccessRateMultiDimensional(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK),
					"Parameter order should not affect response")

				var response models.MultiDimensionalSuccessRateResponse
				json.NewDecoder(rec.Body).Decode(&response)
				Expect(response.Dimensions.IncidentType).To(Equal("pod-oom-killer"))
				Expect(response.Dimensions.PlaybookID).To(Equal("pod-oom-recovery"))
				Expect(response.Dimensions.PlaybookVersion).To(Equal("v1.2"))
				Expect(response.Dimensions.ActionType).To(Equal("increase_memory"))
			})

			It("should handle case-sensitive incident_type", func() {
				// BEHAVIOR: Incident type should be case-sensitive
				req = httptest.NewRequest(
					http.MethodGet,
					"/api/v1/success-rate/multi-dimensional?incident_type=Pod-OOM-Killer&time_range=7d",
					nil,
				)

				handler.HandleGetSuccessRateMultiDimensional(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK),
					"Case should be preserved")

				var response models.MultiDimensionalSuccessRateResponse
				json.NewDecoder(rec.Body).Decode(&response)
				Expect(response.Dimensions.IncidentType).To(Equal("Pod-OOM-Killer"))
			})
		})
	})
})
