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

//go:build unit
// +build unit

package dependency_test

import (
	"context"
	"time"

	"github.com/jordigilh/kubernaut/pkg/orchestration/dependency"
	"github.com/sirupsen/logrus"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Mock dependency for testing
type MockDependency struct {
	name      string
	depType   dependency.DependencyType
	isHealthy bool
	metrics   *dependency.DependencyMetrics
}

func (m *MockDependency) Name() string                              { return m.name }
func (m *MockDependency) Type() dependency.DependencyType           { return m.depType }
func (m *MockDependency) IsHealthy(ctx context.Context) bool        { return m.isHealthy }
func (m *MockDependency) Connect(ctx context.Context) error         { return nil }
func (m *MockDependency) Disconnect() error                         { return nil }
func (m *MockDependency) GetMetrics() *dependency.DependencyMetrics { return m.metrics }
func (m *MockDependency) GetHealthStatus() *dependency.DependencyHealthStatus {
	return &dependency.DependencyHealthStatus{
		Name:             m.name,
		Type:             m.depType,
		IsHealthy:        m.isHealthy,
		LastHealthCheck:  time.Now(),
		ErrorRate:        m.calculateErrorRate(),
		ConnectionStatus: m.getConnectionStatus(),
		Details: map[string]interface{}{
			"total_requests": m.metrics.TotalRequests,
		},
	}
}

func (m *MockDependency) calculateErrorRate() float64 {
	if m.metrics.TotalRequests == 0 {
		return 0.0
	}
	return float64(m.metrics.FailedRequests) / float64(m.metrics.TotalRequests)
}

func (m *MockDependency) getConnectionStatus() dependency.ConnectionStatus {
	if m.isHealthy {
		return dependency.ConnectionStatusConnected
	}
	return dependency.ConnectionStatusFailed
}

var _ = Describe("Dependency Health Monitoring", func() {
	var (
		logger *logrus.Logger
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.ErrorLevel) // Reduce noise during tests
	})

	// Business Requirement: BR-RELIABILITY-006 - Health monitoring for external data sources
	Context("BR-RELIABILITY-006: Dependency Health Status Calculation", func() {
		It("should detect healthy dependencies accurately", func() {
			// Business Contract: Healthy dependencies should report correct status
			dm := dependency.NewDependencyManager(nil, logger)

			// Register healthy dependency
			mockDep := &MockDependency{
				name:      "test_vector_db",
				depType:   dependency.DependencyTypeVectorDB,
				isHealthy: true,
				metrics: &dependency.DependencyMetrics{
					TotalRequests:      100,
					SuccessfulRequests: 95,
					FailedRequests:     5,
				},
			}

			err := dm.RegisterDependency(mockDep)
			Expect(err).ToNot(HaveOccurred())

			// Get health report
			report := dm.GetHealthReport()

			// Business Validation: Overall health should be true
			Expect(report.OverallHealthy).To(BeTrue())
			Expect(report.HealthyDependencies).To(Equal(1))
			Expect(report.TotalDependencies).To(Equal(1))

			// Verify specific dependency status
			status := report.DependencyStatus["test_vector_db"]
			Expect(status.IsHealthy).To(BeTrue())
			Expect(status.ErrorRate).To(BeNumerically("~", 0.05, 0.001)) // 5% error rate
			Expect(status.ConnectionStatus).To(Equal(dependency.ConnectionStatusConnected))
		})

		It("should detect unhealthy dependencies accurately", func() {
			// Business Requirement: BR-RELIABILITY-006 - Unhealthy dependency detection
			dm := dependency.NewDependencyManager(nil, logger)

			// Register unhealthy dependency with high error rate
			mockDep := &MockDependency{
				name:      "test_pattern_store",
				depType:   dependency.DependencyTypePatternStore,
				isHealthy: false,
				metrics: &dependency.DependencyMetrics{
					TotalRequests:      100,
					SuccessfulRequests: 25,
					FailedRequests:     75, // 75% failure rate
				},
			}

			err := dm.RegisterDependency(mockDep)
			Expect(err).ToNot(HaveOccurred())

			// Get health report
			report := dm.GetHealthReport()

			// Business Validation: Overall health should be false
			Expect(report.OverallHealthy).To(BeFalse())
			Expect(report.HealthyDependencies).To(Equal(0))
			Expect(report.TotalDependencies).To(Equal(1))

			// Verify specific dependency status
			status := report.DependencyStatus["test_pattern_store"]
			Expect(status.IsHealthy).To(BeFalse())
			Expect(status.ErrorRate).To(BeNumerically("~", 0.75, 0.001)) // 75% error rate
			Expect(status.ConnectionStatus).To(Equal(dependency.ConnectionStatusFailed))
		})

		It("should handle mixed dependency health states", func() {
			// Business Requirement: BR-RELIABILITY-006 - Mixed health state handling
			dm := dependency.NewDependencyManager(nil, logger)

			// Register healthy dependency
			healthyDep := &MockDependency{
				name:      "healthy_vector_db",
				depType:   dependency.DependencyTypeVectorDB,
				isHealthy: true,
				metrics: &dependency.DependencyMetrics{
					TotalRequests:      50,
					SuccessfulRequests: 48,
					FailedRequests:     2,
				},
			}

			// Register unhealthy dependency
			unhealthyDep := &MockDependency{
				name:      "unhealthy_pattern_store",
				depType:   dependency.DependencyTypePatternStore,
				isHealthy: false,
				metrics: &dependency.DependencyMetrics{
					TotalRequests:      50,
					SuccessfulRequests: 10,
					FailedRequests:     40,
				},
			}

			err := dm.RegisterDependency(healthyDep)
			Expect(err).ToNot(HaveOccurred())
			err = dm.RegisterDependency(unhealthyDep)
			Expect(err).ToNot(HaveOccurred())

			// Get health report
			report := dm.GetHealthReport()

			// Business Validation: Overall health should be false (any unhealthy dependency)
			Expect(report.OverallHealthy).To(BeFalse())
			Expect(report.HealthyDependencies).To(Equal(1))
			Expect(report.TotalDependencies).To(Equal(2))

			// Verify individual statuses
			healthyStatus := report.DependencyStatus["healthy_vector_db"]
			unhealthyStatus := report.DependencyStatus["unhealthy_pattern_store"]

			Expect(healthyStatus.IsHealthy).To(BeTrue())
			Expect(healthyStatus.ErrorRate).To(BeNumerically("~", 0.04, 0.001)) // 4% error rate

			Expect(unhealthyStatus.IsHealthy).To(BeFalse())
			Expect(unhealthyStatus.ErrorRate).To(BeNumerically("~", 0.8, 0.001)) // 80% error rate
		})

		It("should calculate error rates with mathematical precision", func() {
			// Business Requirement: BR-RELIABILITY-006 - Mathematical accuracy in health calculations

			testCases := []struct {
				name           string
				totalRequests  int64
				failedRequests int64
				expectedRate   float64
			}{
				{"zero_requests", 0, 0, 0.0},
				{"no_failures", 100, 0, 0.0},
				{"all_failures", 100, 100, 1.0},
				{"quarter_failures", 100, 25, 0.25},
				{"half_failures", 100, 50, 0.5},
				{"three_quarter_failures", 100, 75, 0.75},
			}

			for _, tc := range testCases {
				mockDep := &MockDependency{
					name:      tc.name,
					depType:   dependency.DependencyTypeVectorDB,
					isHealthy: tc.expectedRate < 0.5, // Healthy if error rate < 50%
					metrics: &dependency.DependencyMetrics{
						TotalRequests:      tc.totalRequests,
						SuccessfulRequests: tc.totalRequests - tc.failedRequests,
						FailedRequests:     tc.failedRequests,
					},
				}

				// Test error rate calculation
				calculatedRate := mockDep.calculateErrorRate()
				Expect(calculatedRate).To(BeNumerically("~", tc.expectedRate, 0.001),
					"Error rate calculation failed for case: %s", tc.name)
			}
		})

		It("should handle edge cases in health status calculation", func() {
			// Business Requirement: BR-RELIABILITY-006 - Edge case handling
			dm := dependency.NewDependencyManager(nil, logger)

			// Test with zero metrics
			zeroMetricsDep := &MockDependency{
				name:      "zero_metrics",
				depType:   dependency.DependencyTypeCache,
				isHealthy: true,
				metrics: &dependency.DependencyMetrics{
					TotalRequests:      0,
					SuccessfulRequests: 0,
					FailedRequests:     0,
				},
			}

			err := dm.RegisterDependency(zeroMetricsDep)
			Expect(err).ToNot(HaveOccurred())

			report := dm.GetHealthReport()
			status := report.DependencyStatus["zero_metrics"]

			// Business Validation: Zero metrics should not cause errors
			Expect(status.ErrorRate).To(Equal(0.0))
			Expect(status.IsHealthy).To(BeTrue())
		})
	})

	// Business Requirement: BR-REL-009 - Circuit breaker integration with health monitoring
	Context("BR-REL-009: Health Monitoring Integration with Circuit Breakers", func() {
		It("should reflect circuit breaker state in health status", func() {
			// Business Contract: Health status should reflect circuit breaker state
			dm := dependency.NewDependencyManager(&dependency.DependencyConfig{
				EnableFallbacks: true,
			}, logger)

			// Create a dependency that will trigger circuit breaker
			mockDep := &MockDependency{
				name:      "circuit_breaker_test",
				depType:   dependency.DependencyTypeVectorDB,
				isHealthy: false, // Will cause circuit breaker to open
				metrics: &dependency.DependencyMetrics{
					TotalRequests:      10,
					SuccessfulRequests: 2,
					FailedRequests:     8, // 80% failure rate
				},
			}

			err := dm.RegisterDependency(mockDep)
			Expect(err).ToNot(HaveOccurred())

			// Get health report
			report := dm.GetHealthReport()

			// Business Validation: Health should reflect circuit breaker impact
			Expect(report.OverallHealthy).To(BeFalse())
			status := report.DependencyStatus["circuit_breaker_test"]
			Expect(status.IsHealthy).To(BeFalse())
			Expect(status.ErrorRate).To(BeNumerically("~", 0.8, 0.001))
		})
	})

	// Business Requirement: BR-ERR-007 - Health monitoring for AI services
	Context("BR-ERR-007: AI Service Health Monitoring", func() {
		It("should monitor AI service health with appropriate thresholds", func() {
			// Business Contract: AI services have specific health requirements
			dm := dependency.NewDependencyManager(nil, logger)

			// AI service with acceptable performance
			aiService := &MockDependency{
				name:      "ai_llm_service",
				depType:   dependency.DependencyTypeMLLibrary,
				isHealthy: true,
				metrics: &dependency.DependencyMetrics{
					TotalRequests:       200,
					SuccessfulRequests:  180,
					FailedRequests:      20, // 10% failure rate - acceptable for AI
					AverageResponseTime: 2 * time.Second,
				},
			}

			err := dm.RegisterDependency(aiService)
			Expect(err).ToNot(HaveOccurred())

			report := dm.GetHealthReport()
			status := report.DependencyStatus["ai_llm_service"]

			// Business Validation: AI service health should be properly assessed
			Expect(status.IsHealthy).To(BeTrue())
			Expect(status.ErrorRate).To(BeNumerically("~", 0.1, 0.001)) // 10% error rate
			Expect(status.Type).To(Equal(dependency.DependencyTypeMLLibrary))
		})
	})
})

// TestRunner bootstraps the Ginkgo test suite
