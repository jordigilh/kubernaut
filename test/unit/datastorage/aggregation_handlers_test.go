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

	"github.com/jordigilh/kubernaut/pkg/datastorage/mocks"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-STORAGE-030: Aggregation API Endpoints
// TDD RED Phase: Write failing tests for aggregation endpoints
// Following Data Storage Implementation Plan V4.8 guidelines:
// - Behavior + Correctness Testing (GAP-05)
// - Table-driven tests for edge cases
// - Defense-in-depth: 70% unit test coverage target
var _ = Describe("Aggregation API Handlers - BR-STORAGE-030", func() {
	var (
		handler *server.Handler
		mockDB  *mocks.MockDB
		req     *http.Request
		rec     *httptest.ResponseRecorder
	)

	BeforeEach(func() {
		mockDB = mocks.NewMockDB()
		handler = server.NewHandler(mockDB)
		rec = httptest.NewRecorder()
	})

	// BR-STORAGE-031: Success Rate Aggregation
	Describe("AggregateSuccessRate", func() {
		Context("Behavior + Correctness Testing ✅ GAP-05", func() {
			It("should calculate success rate correctly with exact counts", func() {
				// Setup: MockDB with 4 incidents (3 completed, 1 failed)
				mockDB.SetAggregationData("success_rate", map[string]interface{}{
					"total_count":   4,
					"success_count": 3,
					"failure_count": 1,
					"success_rate":  0.75,
				})

				req = httptest.NewRequest("GET", "/api/v1/incidents/aggregate/success-rate?workflow_id=workflow-123", nil)
				handler.AggregateSuccessRate(rec, req)

				// ✅ BEHAVIOR TEST: API returns 200 OK
				Expect(rec.Code).To(Equal(http.StatusOK))

				var response map[string]interface{}
				err := json.Unmarshal(rec.Body.Bytes(), &response)
				Expect(err).ToNot(HaveOccurred())

				// ✅ BEHAVIOR TEST: Response has required structure
				Expect(response).To(HaveKey("workflow_id"))
				Expect(response).To(HaveKey("total_count"))
				Expect(response).To(HaveKey("success_count"))
				Expect(response).To(HaveKey("failure_count"))
				Expect(response).To(HaveKey("success_rate"))

				// ✅ CORRECTNESS TEST: Values match database aggregation exactly
				Expect(response["workflow_id"]).To(Equal("workflow-123"),
					"workflow_id must match request parameter exactly")
				Expect(response["total_count"]).To(Equal(float64(4)),
					"total_count must match database COUNT(*) exactly, not approximation")
				Expect(response["success_count"]).To(Equal(float64(3)),
					"success_count must match database WHERE status='completed' COUNT exactly")
				Expect(response["failure_count"]).To(Equal(float64(1)),
					"failure_count must match database WHERE status='failed' COUNT exactly")
				Expect(response["success_rate"]).To(BeNumerically("~", 0.75, 0.01),
					"success_rate must equal success_count/total_count (3/4 = 0.75) exactly")
			})

			It("should handle 100% success rate", func() {
				mockDB.SetAggregationData("success_rate", map[string]interface{}{
					"total_count":   5,
					"success_count": 5,
					"failure_count": 0,
					"success_rate":  1.0,
				})

				req = httptest.NewRequest("GET", "/api/v1/incidents/aggregate/success-rate?workflow_id=perfect-workflow", nil)
				handler.AggregateSuccessRate(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))

				var response map[string]interface{}
				err := json.Unmarshal(rec.Body.Bytes(), &response)
				Expect(err).ToNot(HaveOccurred())

				Expect(response["success_rate"]).To(Equal(float64(1.0)))
			})

			It("should handle 0% success rate", func() {
				mockDB.SetAggregationData("success_rate", map[string]interface{}{
					"total_count":   3,
					"success_count": 0,
					"failure_count": 3,
					"success_rate":  0.0,
				})

				req = httptest.NewRequest("GET", "/api/v1/incidents/aggregate/success-rate?workflow_id=failed-workflow", nil)
				handler.AggregateSuccessRate(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))

				var response map[string]interface{}
				err := json.Unmarshal(rec.Body.Bytes(), &response)
				Expect(err).ToNot(HaveOccurred())

				Expect(response["success_rate"]).To(Equal(float64(0.0)))
			})
		})

		Context("when no incidents exist for workflow", func() {
			It("should return zero counts with 0.0 success rate", func() {
				mockDB.SetAggregationData("success_rate", map[string]interface{}{
					"total_count":   0,
					"success_count": 0,
					"failure_count": 0,
					"success_rate":  0.0,
				})

				req = httptest.NewRequest("GET", "/api/v1/incidents/aggregate/success-rate?workflow_id=empty-workflow", nil)
				handler.AggregateSuccessRate(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))

				var response map[string]interface{}
				err := json.Unmarshal(rec.Body.Bytes(), &response)
				Expect(err).ToNot(HaveOccurred())

				Expect(response["total_count"]).To(Equal(float64(0)))
				Expect(response["success_rate"]).To(Equal(float64(0.0)))
			})
		})

		// ✅ Table-Driven Tests for Edge Cases (Implementation Plan: Use DescribeTable)
		DescribeTable("Edge cases with exact correctness validation",
			func(workflowID string, mockData map[string]interface{}, expectedRate float64) {
				mockDB.SetAggregationData("success_rate", mockData)

				req = httptest.NewRequest("GET", "/api/v1/incidents/aggregate/success-rate?workflow_id="+workflowID, nil)
				handler.AggregateSuccessRate(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))

				var response map[string]interface{}
				err := json.Unmarshal(rec.Body.Bytes(), &response)
				Expect(err).ToNot(HaveOccurred())

				// ✅ CORRECTNESS: Exact success rate match
				Expect(response["success_rate"]).To(BeNumerically("~", expectedRate, 0.001))

				// ✅ CORRECTNESS: Math verification
				totalCount := response["total_count"].(float64)
				successCount := response["success_count"].(float64)
				if totalCount > 0 {
					calculatedRate := successCount / totalCount
					Expect(response["success_rate"]).To(BeNumerically("~", calculatedRate, 0.001),
						"success_rate must equal success_count/total_count exactly")
				}
			},
			// Edge Case 1: 100% success rate
			Entry("100% success rate (all incidents completed)",
				"perfect-workflow",
				map[string]interface{}{
					"total_count":   5,
					"success_count": 5,
					"failure_count": 0,
					"success_rate":  1.0,
				},
				1.0),

			// Edge Case 2: 0% success rate
			Entry("0% success rate (all incidents failed)",
				"failed-workflow",
				map[string]interface{}{
					"total_count":   3,
					"success_count": 0,
					"failure_count": 3,
					"success_rate":  0.0,
				},
				0.0),

			// Edge Case 3: Single incident success
			Entry("Single incident (100% success)",
				"single-success",
				map[string]interface{}{
					"total_count":   1,
					"success_count": 1,
					"failure_count": 0,
					"success_rate":  1.0,
				},
				1.0),

			// Edge Case 4: Single incident failure
			Entry("Single incident (0% success)",
				"single-failure",
				map[string]interface{}{
					"total_count":   1,
					"success_count": 0,
					"failure_count": 1,
					"success_rate":  0.0,
				},
				0.0),

			// Edge Case 5: Large number of incidents
			Entry("Large workflow (1000 incidents, 95% success)",
				"large-workflow",
				map[string]interface{}{
					"total_count":   1000,
					"success_count": 950,
					"failure_count": 50,
					"success_rate":  0.95,
				},
				0.95),
		)

		Context("when workflow_id parameter is missing", func() {
			It("should return RFC 7807 error", func() {
				req = httptest.NewRequest("GET", "/api/v1/incidents/aggregate/success-rate", nil)
				handler.AggregateSuccessRate(rec, req)

				// ✅ BEHAVIOR TEST: Returns 400 Bad Request
				Expect(rec.Code).To(Equal(http.StatusBadRequest))

				var problem map[string]interface{}
				err := json.Unmarshal(rec.Body.Bytes(), &problem)
				Expect(err).ToNot(HaveOccurred())

				// ✅ CORRECTNESS TEST: RFC 7807 structure is complete
				Expect(problem).To(HaveKey("type"))
				Expect(problem["type"]).To(ContainSubstring("missing-parameter"))
				Expect(problem).To(HaveKey("title"))
				Expect(problem).To(HaveKey("status"))
				Expect(problem["status"]).To(Equal(float64(400)))

				// ✅ CORRECTNESS: Error message mentions the specific parameter
				Expect(problem["detail"]).To(ContainSubstring("workflow_id"))
			})
		})
	})

	// BR-STORAGE-032: Namespace Grouping Aggregation
	Describe("AggregateByNamespace", func() {
		Context("when incidents exist across namespaces", func() {
			It("should group incidents by namespace with counts", func() {
				mockDB.SetAggregationData("by_namespace", map[string]interface{}{
					"aggregations": []map[string]interface{}{
						{"namespace": "prod", "count": 50},
						{"namespace": "staging", "count": 30},
						{"namespace": "dev", "count": 20},
					},
				})

				req = httptest.NewRequest("GET", "/api/v1/incidents/aggregate/by-namespace", nil)
				handler.AggregateByNamespace(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))

				var response map[string]interface{}
				err := json.Unmarshal(rec.Body.Bytes(), &response)
				Expect(err).ToNot(HaveOccurred())

				Expect(response).To(HaveKey("aggregations"))
				aggregations := response["aggregations"].([]interface{})
				Expect(aggregations).To(HaveLen(3))

				// Verify first aggregation
				firstAgg := aggregations[0].(map[string]interface{})
				Expect(firstAgg["namespace"]).To(Equal("prod"))
				Expect(firstAgg["count"]).To(Equal(float64(50)))
			})

			It("should order namespaces by count descending", func() {
				mockDB.SetAggregationData("by_namespace", map[string]interface{}{
					"aggregations": []map[string]interface{}{
						{"namespace": "prod", "count": 100},
						{"namespace": "staging", "count": 50},
						{"namespace": "dev", "count": 10},
					},
				})

				req = httptest.NewRequest("GET", "/api/v1/incidents/aggregate/by-namespace", nil)
				handler.AggregateByNamespace(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))

				var response map[string]interface{}
				err := json.Unmarshal(rec.Body.Bytes(), &response)
				Expect(err).ToNot(HaveOccurred())

				aggregations := response["aggregations"].([]interface{})
				firstCount := aggregations[0].(map[string]interface{})["count"].(float64)
				lastCount := aggregations[len(aggregations)-1].(map[string]interface{})["count"].(float64)

				Expect(firstCount).To(BeNumerically(">=", lastCount))
			})
		})

		Context("when no incidents exist", func() {
			It("should return empty aggregations array", func() {
				mockDB.SetAggregationData("by_namespace", map[string]interface{}{
					"aggregations": []map[string]interface{}{},
				})

				req = httptest.NewRequest("GET", "/api/v1/incidents/aggregate/by-namespace", nil)
				handler.AggregateByNamespace(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))

				var response map[string]interface{}
				err := json.Unmarshal(rec.Body.Bytes(), &response)
				Expect(err).ToNot(HaveOccurred())

				aggregations := response["aggregations"].([]interface{})
				Expect(aggregations).To(HaveLen(0))
			})
		})
	})

	// BR-STORAGE-033: Severity Distribution Aggregation
	Describe("AggregateBySeverity", func() {
		Context("when incidents exist with different severities", func() {
			It("should group incidents by severity with counts", func() {
				mockDB.SetAggregationData("by_severity", map[string]interface{}{
					"aggregations": []map[string]interface{}{
						{"severity": "critical", "count": 10},
						{"severity": "high", "count": 25},
						{"severity": "medium", "count": 40},
						{"severity": "low", "count": 25},
					},
				})

				req = httptest.NewRequest("GET", "/api/v1/incidents/aggregate/by-severity", nil)
				handler.AggregateBySeverity(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))

				var response map[string]interface{}
				err := json.Unmarshal(rec.Body.Bytes(), &response)
				Expect(err).ToNot(HaveOccurred())

				Expect(response).To(HaveKey("aggregations"))
				aggregations := response["aggregations"].([]interface{})
				Expect(aggregations).To(HaveLen(4))

				// Verify critical severity aggregation
				criticalAgg := aggregations[0].(map[string]interface{})
				Expect(criticalAgg["severity"]).To(Equal("critical"))
				Expect(criticalAgg["count"]).To(Equal(float64(10)))
			})

			It("should order severities by severity level (critical first)", func() {
				mockDB.SetAggregationData("by_severity", map[string]interface{}{
					"aggregations": []map[string]interface{}{
						{"severity": "critical", "count": 5},
						{"severity": "high", "count": 10},
						{"severity": "medium", "count": 15},
						{"severity": "low", "count": 20},
					},
				})

				req = httptest.NewRequest("GET", "/api/v1/incidents/aggregate/by-severity", nil)
				handler.AggregateBySeverity(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))

				var response map[string]interface{}
				err := json.Unmarshal(rec.Body.Bytes(), &response)
				Expect(err).ToNot(HaveOccurred())

				aggregations := response["aggregations"].([]interface{})
				firstSeverity := aggregations[0].(map[string]interface{})["severity"].(string)

				Expect(firstSeverity).To(Equal("critical"))
			})
		})

		Context("when no incidents exist", func() {
			It("should return empty aggregations array", func() {
				mockDB.SetAggregationData("by_severity", map[string]interface{}{
					"aggregations": []map[string]interface{}{},
				})

				req = httptest.NewRequest("GET", "/api/v1/incidents/aggregate/by-severity", nil)
				handler.AggregateBySeverity(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))

				var response map[string]interface{}
				err := json.Unmarshal(rec.Body.Bytes(), &response)
				Expect(err).ToNot(HaveOccurred())

				aggregations := response["aggregations"].([]interface{})
				Expect(aggregations).To(HaveLen(0))
			})
		})
	})

	// BR-STORAGE-034: Incident Trend Aggregation
	Describe("AggregateIncidentTrend", func() {
		Context("when period parameter is valid", func() {
			It("should return daily incident counts for 7d period", func() {
				mockDB.SetAggregationData("incident_trend", map[string]interface{}{
					"period": "7d",
					"data_points": []map[string]interface{}{
						{"date": "2025-11-01", "count": 20},
						{"date": "2025-11-02", "count": 25},
						{"date": "2025-11-03", "count": 18},
						{"date": "2025-11-04", "count": 30},
						{"date": "2025-11-05", "count": 22},
						{"date": "2025-11-06", "count": 28},
						{"date": "2025-11-07", "count": 24},
					},
				})

				req = httptest.NewRequest("GET", "/api/v1/incidents/aggregate/trend?period=7d", nil)
				handler.AggregateIncidentTrend(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))

				var response map[string]interface{}
				err := json.Unmarshal(rec.Body.Bytes(), &response)
				Expect(err).ToNot(HaveOccurred())

				Expect(response).To(HaveKey("period"))
				Expect(response["period"]).To(Equal("7d"))
				Expect(response).To(HaveKey("data_points"))

				dataPoints := response["data_points"].([]interface{})
				Expect(dataPoints).To(HaveLen(7))

				// Verify first data point
				firstPoint := dataPoints[0].(map[string]interface{})
				Expect(firstPoint).To(HaveKey("date"))
				Expect(firstPoint).To(HaveKey("count"))
			})

			It("should return daily incident counts for 30d period", func() {
				mockDB.SetAggregationData("incident_trend", map[string]interface{}{
					"period":      "30d",
					"data_points": generateTrendData(30),
				})

				req = httptest.NewRequest("GET", "/api/v1/incidents/aggregate/trend?period=30d", nil)
				handler.AggregateIncidentTrend(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))

				var response map[string]interface{}
				err := json.Unmarshal(rec.Body.Bytes(), &response)
				Expect(err).ToNot(HaveOccurred())

				Expect(response["period"]).To(Equal("30d"))
				dataPoints := response["data_points"].([]interface{})
				Expect(dataPoints).To(HaveLen(30))
			})
		})

		Context("when period parameter is missing", func() {
			It("should default to 7d period", func() {
				mockDB.SetAggregationData("incident_trend", map[string]interface{}{
					"period":      "7d",
					"data_points": generateTrendData(7),
				})

				req = httptest.NewRequest("GET", "/api/v1/incidents/aggregate/trend", nil)
				handler.AggregateIncidentTrend(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))

				var response map[string]interface{}
				err := json.Unmarshal(rec.Body.Bytes(), &response)
				Expect(err).ToNot(HaveOccurred())

				Expect(response["period"]).To(Equal("7d"))
			})
		})

		Context("when period parameter is invalid", func() {
			It("should return RFC 7807 error", func() {
				req = httptest.NewRequest("GET", "/api/v1/incidents/aggregate/trend?period=invalid", nil)
				handler.AggregateIncidentTrend(rec, req)

				Expect(rec.Code).To(Equal(http.StatusBadRequest))

				var problem map[string]interface{}
				err := json.Unmarshal(rec.Body.Bytes(), &problem)
				Expect(err).ToNot(HaveOccurred())

				Expect(problem).To(HaveKey("type"))
				Expect(problem["type"]).To(ContainSubstring("invalid-parameter"))
				// ✅ CORRECTNESS: Error detail must mention the specific parameter and valid values
				Expect(problem["detail"]).To(ContainSubstring("period"))
				Expect(problem["detail"]).To(ContainSubstring("7d"))
			})
		})

		Context("when no incidents exist in period", func() {
			It("should return empty data points array", func() {
				mockDB.SetAggregationData("incident_trend", map[string]interface{}{
					"period":      "7d",
					"data_points": []map[string]interface{}{},
				})

				req = httptest.NewRequest("GET", "/api/v1/incidents/aggregate/trend?period=7d", nil)
				handler.AggregateIncidentTrend(rec, req)

				Expect(rec.Code).To(Equal(http.StatusOK))

				var response map[string]interface{}
				err := json.Unmarshal(rec.Body.Bytes(), &response)
				Expect(err).ToNot(HaveOccurred())

				dataPoints := response["data_points"].([]interface{})
				Expect(dataPoints).To(HaveLen(0))
			})
		})
	})
})

// Helper function to generate trend data for testing
func generateTrendData(days int) []map[string]interface{} {
	data := make([]map[string]interface{}, days)
	for i := 0; i < days; i++ {
		data[i] = map[string]interface{}{
			"date":  "2025-11-01", // Simplified for mock
			"count": 20 + i,
		}
	}
	return data
}

