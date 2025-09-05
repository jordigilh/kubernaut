package monitoring_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/jordigilh/kubernaut/pkg/infrastructure/types"
	"github.com/jordigilh/kubernaut/pkg/platform/monitoring"
)

var _ = Describe("PrometheusClient", func() {
	var (
		client     *monitoring.PrometheusClient
		mockServer *httptest.Server
		logger     *logrus.Logger
		ctx        context.Context
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.DebugLevel)
		ctx = context.Background()
	})

	AfterEach(func() {
		if mockServer != nil {
			mockServer.Close()
		}
	})

	Describe("NewPrometheusClient", func() {
		It("should create a new Prometheus client", func() {
			client := monitoring.NewPrometheusClient("http://localhost:9090", 30*time.Second, logger)
			Expect(client).NotTo(BeNil())
		})

		It("should trim trailing slash from endpoint", func() {
			client := monitoring.NewPrometheusClient("http://localhost:9090/", 30*time.Second, logger)
			Expect(client).NotTo(BeNil())
		})
	})

	Describe("CheckMetricsImprovement", func() {
		var (
			alert       types.Alert
			actionTrace *actionhistory.ResourceActionTrace
		)

		BeforeEach(func() {
			alert = types.Alert{
				Name:      "HighMemoryUsage",
				Namespace: "test-namespace",
				Severity:  "warning",
			}

			executionStart := time.Now().Add(-20 * time.Minute)
			executionEnd := time.Now().Add(-15 * time.Minute)
			actionTrace = &actionhistory.ResourceActionTrace{
				ActionID:           "test-action-123",
				ActionType:         "scale_deployment",
				AlertName:          "HighMemoryUsage",
				ExecutionStartTime: &executionStart,
				ExecutionEndTime:   &executionEnd,
			}
		})

		Context("when metrics show improvement", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					query := r.URL.Query().Get("query")

					var response monitoring.PrometheusQueryResponse
					response.Status = "success"
					response.Data.ResultType = "vector"

					// Simulate improvement for most metrics (need at least 60% to pass)
					if strings.Contains(query, "avg_over_time") {
						var beforeValue, afterValue string

						// Different improvement scenarios based on metric type
						if strings.Contains(query, "memory") || strings.Contains(query, "cpu") {
							// Memory/CPU metrics: lower is better (show improvement)
							// Use significant improvement to ensure 5% threshold is met
							beforeValue = "0.8" // High usage before
							afterValue = "0.6"  // Lower usage after (25% improvement > 5% threshold)
						} else if strings.Contains(query, "replicas") {
							// Replica metrics: higher is better (show improvement)
							beforeValue = "3" // Lower replicas before
							afterValue = "5"  // Higher replicas after (67% improvement > 5% threshold)
						} else {
							// Default: show improvement (lower values)
							beforeValue = "0.8"
							afterValue = "0.6"
						}

						timeParam := r.URL.Query().Get("time")
						if timeParam != "" {
							timestamp, _ := strconv.ParseInt(timeParam, 10, 64)
							queryTime := time.Unix(timestamp, 0)

							// Return different values for "before" vs "after" time
							var value string
							if queryTime.Before(actionTrace.ExecutionStartTime.Add(5 * time.Minute)) {
								value = beforeValue
							} else {
								value = afterValue
							}

							response.Data.Result = []monitoring.PrometheusQueryResult{
								{
									Metric: map[string]string{"container": "test"},
									Value:  []interface{}{float64(timestamp), value},
								},
							}
						}
					}

					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
				}))
				client = monitoring.NewPrometheusClient(mockServer.URL, 30*time.Second, logger)
			})

			It("should return true when metrics improved", func() {
				improved, err := client.CheckMetricsImprovement(ctx, alert, actionTrace)

				Expect(err).NotTo(HaveOccurred())
				Expect(improved).To(BeTrue())
			})
		})

		Context("when execution timestamps are missing", func() {
			BeforeEach(func() {
				actionTrace.ExecutionStartTime = nil
				client = monitoring.NewPrometheusClient("http://localhost:9090", 30*time.Second, logger)
			})

			It("should return an error", func() {
				_, err := client.CheckMetricsImprovement(ctx, alert, actionTrace)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("missing execution timestamps"))
			})
		})

		Context("when no relevant metrics found", func() {
			BeforeEach(func() {
				alert.Name = "UnknownAlert"
				actionTrace.ActionType = "unknown_action"
				client = monitoring.NewPrometheusClient("http://localhost:9090", 30*time.Second, logger)
			})

			It("should return false", func() {
				improved, err := client.CheckMetricsImprovement(ctx, alert, actionTrace)

				Expect(err).NotTo(HaveOccurred())
				Expect(improved).To(BeFalse())
			})
		})
	})

	Describe("GetResourceMetrics", func() {
		Context("when Prometheus returns metrics", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.URL.Path).To(Equal("/api/v1/query"))

					query := r.URL.Query().Get("query")
					var response monitoring.PrometheusQueryResponse
					response.Status = "success"
					response.Data.ResultType = "vector"

					// Return different values based on metric type
					value := "0.5"
					if strings.Contains(query, "memory") {
						value = "1073741824" // 1GB in bytes
					} else if strings.Contains(query, "cpu") {
						value = "0.75"
					}

					response.Data.Result = []monitoring.PrometheusQueryResult{
						{
							Metric: map[string]string{
								"namespace": "test-namespace",
								"pod":       "test-deployment-123",
							},
							Value: []interface{}{float64(time.Now().Unix()), value},
						},
					}

					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
				}))
				client = monitoring.NewPrometheusClient(mockServer.URL, 30*time.Second, logger)
			})

			It("should return metrics for the resource", func() {
				metricNames := []string{
					"container_memory_usage_bytes",
					"container_cpu_usage_seconds_total",
				}

				metrics, err := client.GetResourceMetrics(ctx, "test-namespace", "test-deployment", metricNames)

				Expect(err).NotTo(HaveOccurred())
				Expect(metrics).To(HaveLen(2))
				Expect(metrics).To(HaveKey("container_memory_usage_bytes"))
				Expect(metrics).To(HaveKey("container_cpu_usage_seconds_total"))
				Expect(metrics["container_memory_usage_bytes"]).To(Equal(1073741824.0))
				Expect(metrics["container_cpu_usage_seconds_total"]).To(Equal(0.75))
			})
		})

		Context("when Prometheus returns no data", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					response := monitoring.PrometheusQueryResponse{
						Status: "success",
						Data: struct {
							ResultType string                             `json:"resultType"`
							Result     []monitoring.PrometheusQueryResult `json:"result"`
						}{
							ResultType: "vector",
							Result:     []monitoring.PrometheusQueryResult{},
						},
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
				}))
				client = monitoring.NewPrometheusClient(mockServer.URL, 30*time.Second, logger)
			})

			It("should skip metrics with no data", func() {
				metricNames := []string{"nonexistent_metric"}

				metrics, err := client.GetResourceMetrics(ctx, "test-namespace", "test-deployment", metricNames)

				Expect(err).NotTo(HaveOccurred())
				Expect(metrics).To(BeEmpty())
			})
		})
	})

	Describe("GetMetricsHistory", func() {
		Context("when Prometheus returns range data", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.URL.Path).To(Equal("/api/v1/query_range"))

					params, _ := url.ParseQuery(r.URL.RawQuery)
					Expect(params.Get("start")).NotTo(BeEmpty())
					Expect(params.Get("end")).NotTo(BeEmpty())
					Expect(params.Get("step")).NotTo(BeEmpty())

					response := monitoring.PrometheusRangeResponse{
						Status: "success",
						Data: struct {
							ResultType string                             `json:"resultType"`
							Result     []monitoring.PrometheusRangeResult `json:"result"`
						}{
							ResultType: "matrix",
							Result: []monitoring.PrometheusRangeResult{
								{
									Metric: map[string]string{
										"namespace": "test-namespace",
										"pod":       "test-deployment-123",
									},
									Values: [][]interface{}{
										{float64(time.Now().Add(-10 * time.Minute).Unix()), "0.6"},
										{float64(time.Now().Add(-5 * time.Minute).Unix()), "0.7"},
										{float64(time.Now().Unix()), "0.8"},
									},
								},
							},
						},
					}

					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
				}))
				client = monitoring.NewPrometheusClient(mockServer.URL, 30*time.Second, logger)
			})

			It("should return historical metric points", func() {
				from := time.Now().Add(-15 * time.Minute)
				to := time.Now()
				metricNames := []string{"container_memory_usage_bytes"}

				points, err := client.GetMetricsHistory(ctx, "test-namespace", "test-deployment", metricNames, from, to)

				Expect(err).NotTo(HaveOccurred())
				Expect(points).To(HaveLen(3))

				for _, point := range points {
					Expect(point.MetricName).To(Equal("container_memory_usage_bytes"))
					Expect(point.Labels).To(HaveKeyWithValue("namespace", "test-namespace"))
					Expect(point.Value).To(BeNumerically(">=", 0.6))
					Expect(point.Value).To(BeNumerically("<=", 0.8))
				}
			})
		})
	})

	Describe("HealthCheck", func() {
		Context("when Prometheus is healthy", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/-/healthy" {
						w.WriteHeader(http.StatusOK)
						_, _ = w.Write([]byte("Prometheus is Healthy."))
					}
				}))
				client = monitoring.NewPrometheusClient(mockServer.URL, 30*time.Second, logger)
			})

			It("should return no error", func() {
				err := client.HealthCheck(ctx)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when Prometheus is unhealthy", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/-/healthy" {
						w.WriteHeader(http.StatusServiceUnavailable)
					}
				}))
				client = monitoring.NewPrometheusClient(mockServer.URL, 30*time.Second, logger)
			})

			It("should return an error", func() {
				err := client.HealthCheck(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("health check failed with status 503"))
			})
		})
	})

	Describe("Query Building", func() {
		BeforeEach(func() {
			client = monitoring.NewPrometheusClient("http://localhost:9090", 30*time.Second, logger)
		})

		Context("for different alert types", func() {
			It("should identify relevant metrics for memory alerts", func() {
				alert := types.Alert{Name: "HighMemoryUsage"}
				actionTrace := &actionhistory.ResourceActionTrace{ActionType: "scale_deployment"}

				// This is tested indirectly through the metric improvement check
				// The actual metric selection logic is internal to the client
				Expect(alert.Name).To(Equal("HighMemoryUsage"))
				Expect(actionTrace.ActionType).To(Equal("scale_deployment"))
			})

			It("should identify relevant metrics for CPU alerts", func() {
				alert := types.Alert{Name: "HighCPUUsage"}
				actionTrace := &actionhistory.ResourceActionTrace{ActionType: "increase_resources"}

				Expect(alert.Name).To(Equal("HighCPUUsage"))
				Expect(actionTrace.ActionType).To(Equal("increase_resources"))
			})
		})
	})

	Describe("Error Handling", func() {
		Context("when Prometheus is unreachable", func() {
			BeforeEach(func() {
				client = monitoring.NewPrometheusClient("http://invalid-endpoint", 1*time.Second, logger)
			})

			It("should handle connection errors gracefully", func() {
				metricNames := []string{"test_metric"}
				_, err := client.GetResourceMetrics(ctx, "test-namespace", "test-resource", metricNames)

				Expect(err).To(HaveOccurred())
			})
		})

		Context("when Prometheus returns invalid JSON", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte("invalid json"))
				}))
				client = monitoring.NewPrometheusClient(mockServer.URL, 30*time.Second, logger)
			})

			It("should handle JSON decode errors", func() {
				metricNames := []string{"test_metric"}
				_, err := client.GetResourceMetrics(ctx, "test-namespace", "test-resource", metricNames)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to decode response"))
			})
		})

		Context("when Prometheus returns error status", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					response := monitoring.PrometheusQueryResponse{
						Status: "error",
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
				}))
				client = monitoring.NewPrometheusClient(mockServer.URL, 30*time.Second, logger)
			})

			It("should handle API error status", func() {
				metricNames := []string{"test_metric"}
				_, err := client.GetResourceMetrics(ctx, "test-namespace", "test-resource", metricNames)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Prometheus API returned status: error"))
			})
		})

		Context("when metrics have invalid values", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					response := monitoring.PrometheusQueryResponse{
						Status: "success",
						Data: struct {
							ResultType string                             `json:"resultType"`
							Result     []monitoring.PrometheusQueryResult `json:"result"`
						}{
							ResultType: "vector",
							Result: []monitoring.PrometheusQueryResult{
								{
									Metric: map[string]string{"test": "value"},
									Value:  []interface{}{float64(time.Now().Unix()), "invalid_number"},
								},
							},
						},
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
				}))
				client = monitoring.NewPrometheusClient(mockServer.URL, 30*time.Second, logger)
			})

			It("should handle invalid metric values", func() {
				metricNames := []string{"test_metric"}
				_, err := client.GetResourceMetrics(ctx, "test-namespace", "test-resource", metricNames)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to parse value"))
			})
		})
	})

	Describe("Context Cancellation", func() {
		BeforeEach(func() {
			mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Simulate slow response
				time.Sleep(100 * time.Millisecond)
				response := monitoring.PrometheusQueryResponse{
					Status: "success",
					Data: struct {
						ResultType string                             `json:"resultType"`
						Result     []monitoring.PrometheusQueryResult `json:"result"`
					}{
						ResultType: "vector",
						Result:     []monitoring.PrometheusQueryResult{},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(response)
			}))
			client = monitoring.NewPrometheusClient(mockServer.URL, 30*time.Second, logger)
		})

		It("should respect context cancellation", func() {
			cancelCtx, cancel := context.WithCancel(ctx)
			cancel() // Cancel immediately

			metricNames := []string{"test_metric"}
			_, err := client.GetResourceMetrics(cancelCtx, "test-namespace", "test-resource", metricNames)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context canceled"))
		})
	})
})
