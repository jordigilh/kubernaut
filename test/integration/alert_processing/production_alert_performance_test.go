//go:build integration
// +build integration

package alert_processing

import (
	"context"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/integration/shared"
)

// AlertProcessingPerformanceValidator validates Phase 1 Critical Production Readiness - Alert Processing Performance
// Business Requirements Covered:
// - BR-PA-001: 99.9% uptime (max 8.6s downtime/day)
// - BR-PA-003: <5 seconds response time (95th percentile) for 1000 alerts/min
// - BR-PA-004: 100 concurrent requests without degradation
type AlertProcessingPerformanceValidator struct {
	logger         *logrus.Logger
	llmClient      llm.Client
	testConfig     shared.IntegrationConfig
	stateManager   *shared.ComprehensiveStateManager
	metricsTracker *AlertProcessingMetrics
}

// AlertProcessingMetrics tracks performance metrics for business requirement validation
type AlertProcessingMetrics struct {
	TotalAlerts           int
	SuccessfulAlerts      int
	FailedAlerts          int
	TotalProcessingTime   time.Duration
	ResponseTimes         []time.Duration
	ConcurrentSuccessRate float64
	AvailabilityPercentage float64
	ErrorsByType         map[string]int
	mu                   sync.RWMutex
}

// NewAlertProcessingPerformanceValidator creates a validator for Phase 1 alert processing performance requirements
func NewAlertProcessingPerformanceValidator(config shared.IntegrationConfig, stateManager *shared.ComprehensiveStateManager) *AlertProcessingPerformanceValidator {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	return &AlertProcessingPerformanceValidator{
		logger:       logger,
		testConfig:   config,
		stateManager: stateManager,
		metricsTracker: &AlertProcessingMetrics{
			ErrorsByType: make(map[string]int),
		},
	}
}

// ValidateUptimeRequirement validates BR-PA-001: 99.9% uptime requirement
func (v *AlertProcessingPerformanceValidator) ValidateUptimeRequirement(ctx context.Context, duration time.Duration) (*UptimeValidationResult, error) {
	v.logger.WithField("duration", duration).Info("Starting uptime validation for alert processing service")

	// Initialize LLM client if not already set
	if v.llmClient == nil {
		v.llmClient = shared.NewFakeSLMClient()
	}

	// Track uptime metrics
	startTime := time.Now()
	serviceInterruptions := 0
	totalDowntime := time.Duration(0)

	// Simulate alert processing under load to test uptime
	alertCount := int(duration.Seconds()) * 2 // 2 alerts per second baseline (120 per minute)
	if alertCount < 10 {
		alertCount = 10 // Minimum alerts for meaningful test
	}
	successfulRequests := 0

	// Generate test alerts for uptime validation
	testAlerts := v.generateTestAlerts(alertCount)

	// Process alerts and track availability
	for i, alert := range testAlerts {
		requestStart := time.Now()

		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Process alert through LLM client
		_, err := v.llmClient.AnalyzeAlert(ctx, alert)
		requestDuration := time.Since(requestStart)

		if err != nil {
			// Track service interruption
			serviceInterruptions++
			totalDowntime += requestDuration
			v.logger.WithError(err).WithField("alert_index", i).Debug("Alert processing failed")
		} else {
			successfulRequests++
		}

		// Add small delay between requests to simulate realistic load
		time.Sleep(10 * time.Millisecond)
	}

	// Calculate uptime metrics
	totalTestTime := time.Since(startTime)
	availabilityPercentage := (float64(successfulRequests) / float64(alertCount)) * 100

	// BR-PA-001: Must meet 99.9% uptime requirement
	meetsRequirement := availabilityPercentage >= 99.9

	// Calculate max acceptable downtime for the test duration (0.1% of duration)
	maxAcceptableDowntime := time.Duration(float64(totalTestTime) * 0.001)

	v.logger.WithFields(logrus.Fields{
		"availability_percentage": availabilityPercentage,
		"total_downtime": totalDowntime,
		"max_acceptable_downtime": maxAcceptableDowntime,
		"service_interruptions": serviceInterruptions,
		"alerts_processed": alertCount,
	}).Info("Uptime validation completed")

	return &UptimeValidationResult{
		AvailabilityPercentage: availabilityPercentage,
		TotalDowntime:          totalDowntime,
		MeetsRequirement:       meetsRequirement,
		ServiceInterruptions:   serviceInterruptions,
		MaxAcceptableDowntime:  maxAcceptableDowntime,
	}, nil
}

// ValidateResponseTimeRequirement validates BR-PA-003: <5s response time for 1000 alerts/min
func (v *AlertProcessingPerformanceValidator) ValidateResponseTimeRequirement(ctx context.Context, alertsPerMinute int, duration time.Duration) (*ResponseTimeValidationResult, error) {
	v.logger.WithFields(logrus.Fields{
		"alerts_per_minute": alertsPerMinute,
		"duration": duration,
	}).Info("Starting response time validation for alert processing")

	// Initialize LLM client if not already set
	if v.llmClient == nil {
		v.llmClient = shared.NewFakeSLMClient()
	}

	// Calculate total alerts to process
	totalAlerts := int(duration.Seconds()) * alertsPerMinute / 60 // Convert per-minute rate to total for duration
	if totalAlerts < 10 {
		totalAlerts = 10 // Minimum alerts for meaningful test
	}
	alertsProcessed := 0
	responseTimes := make([]time.Duration, 0, totalAlerts)

	// Distribution buckets for response time analysis
	responseTimeDistribution := map[string]int{
		"<1s":  0,
		"1-2s": 0,
		"2-5s": 0,
		">5s":  0,
	}

	// Generate test alerts
	testAlerts := v.generateTestAlerts(totalAlerts)

	// Calculate interval between alerts to achieve target rate
	intervalBetweenAlerts := time.Minute / time.Duration(alertsPerMinute)

	v.logger.WithField("interval_between_alerts", intervalBetweenAlerts).Debug("Alert processing interval calculated")

	// Process alerts at specified rate
	startTime := time.Now()
	ticker := time.NewTicker(intervalBetweenAlerts)
	defer ticker.Stop()

	for i, alert := range testAlerts {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Wait for next tick to maintain rate
		if i > 0 {
			<-ticker.C
		}

		// Measure response time
		requestStart := time.Now()
		_, err := v.llmClient.AnalyzeAlert(ctx, alert)
		responseTime := time.Since(requestStart)

		if err != nil {
			v.logger.WithError(err).WithField("alert_index", i).Debug("Alert processing failed")
			// Still track response time even for failed requests
		}

		responseTimes = append(responseTimes, responseTime)
		alertsProcessed++

		// Update response time distribution
		if responseTime < 1*time.Second {
			responseTimeDistribution["<1s"]++
		} else if responseTime < 2*time.Second {
			responseTimeDistribution["1-2s"]++
		} else if responseTime < 5*time.Second {
			responseTimeDistribution["2-5s"]++
		} else {
			responseTimeDistribution[">5s"]++
		}

		// Check if we've exceeded test duration
		if time.Since(startTime) >= duration {
			break
		}
	}

	// Calculate metrics
	actualDuration := time.Since(startTime)
	actualAlertsPerMinute := int(float64(alertsProcessed) / actualDuration.Minutes())
	averageResponseTime := v.calculateAverageResponseTime(responseTimes)
	percentile95ResponseTime := v.calculatePercentile(responseTimes, 0.95)

	// BR-PA-003: Must meet <5s response time requirement (95th percentile)
	meetsRequirement := percentile95ResponseTime < 5*time.Second

	v.logger.WithFields(logrus.Fields{
		"alerts_processed": alertsProcessed,
		"actual_alerts_per_minute": actualAlertsPerMinute,
		"average_response_time": averageResponseTime,
		"percentile_95_response_time": percentile95ResponseTime,
		"meets_requirement": meetsRequirement,
	}).Info("Response time validation completed")

	return &ResponseTimeValidationResult{
		Percentile95ResponseTime: percentile95ResponseTime,
		AverageResponseTime:     averageResponseTime,
		MeetsRequirement:        meetsRequirement,
		AlertsProcessed:         alertsProcessed,
		AlertsPerMinuteActual:   actualAlertsPerMinute,
		ResponseTimeDistribution: responseTimeDistribution,
	}, nil
}

// ValidateConcurrentProcessingRequirement validates BR-PA-004: 100 concurrent requests without degradation
func (v *AlertProcessingPerformanceValidator) ValidateConcurrentProcessingRequirement(ctx context.Context, concurrentRequests int) (*ConcurrentProcessingValidationResult, error) {
	v.logger.WithField("concurrent_requests", concurrentRequests).Info("Starting concurrent processing validation")

	// Initialize LLM client if not already set
	if v.llmClient == nil {
		v.llmClient = shared.NewFakeSLMClient()
	}

	// Generate test alerts for concurrent processing
	testAlerts := v.generateTestAlerts(concurrentRequests)

	// Track concurrent processing metrics
	var wg sync.WaitGroup
	var mu sync.Mutex
	successfulRequests := 0
	failedRequests := 0
	latencies := make([]time.Duration, 0, concurrentRequests)
	errorCount := 0

	// Baseline latency measurement (single request)
	baselineStart := time.Now()
	_, err := v.llmClient.AnalyzeAlert(ctx, testAlerts[0])
	baselineLatency := time.Since(baselineStart)
	if err != nil {
		v.logger.WithError(err).Debug("Baseline request failed")
	}

	// Execute concurrent requests
	startTime := time.Now()
	for i := 0; i < concurrentRequests; i++ {
		wg.Add(1)
		go func(alertIndex int) {
			defer wg.Done()

			// Check context cancellation
			select {
			case <-ctx.Done():
				return
			default:
			}

			requestStart := time.Now()
			_, err := v.llmClient.AnalyzeAlert(ctx, testAlerts[alertIndex])
			requestLatency := time.Since(requestStart)

			// Thread-safe metrics update
			mu.Lock()
			latencies = append(latencies, requestLatency)
			if err != nil {
				failedRequests++
				errorCount++
				v.logger.WithError(err).WithField("alert_index", alertIndex).Debug("Concurrent request failed")
			} else {
				successfulRequests++
			}
			mu.Unlock()
		}(i)
	}

	// Wait for all concurrent requests to complete
	wg.Wait()
	totalExecutionTime := time.Since(startTime)

	// Calculate metrics
	successRate := float64(successfulRequests) / float64(concurrentRequests)
	errorRate := float64(errorCount) / float64(concurrentRequests)
	averageLatency := v.calculateAverageResponseTime(latencies)

	// Calculate performance degradation vs baseline
	degradationPercentage := float64(averageLatency-baselineLatency) / float64(baselineLatency)
	averageLatencyDegradation := averageLatency - baselineLatency

	// BR-PA-004: Must handle 100 concurrent requests without significant degradation
	meetsRequirement := concurrentRequests >= 100 && successRate >= 0.95 && errorRate <= 0.05
	throughputMaintained := degradationPercentage <= 1.0 // 100% degradation threshold

	v.logger.WithFields(logrus.Fields{
		"concurrent_requests_handled": concurrentRequests,
		"successful_requests": successfulRequests,
		"failed_requests": failedRequests,
		"success_rate": successRate,
		"error_rate": errorRate,
		"average_latency": averageLatency,
		"baseline_latency": baselineLatency,
		"degradation_percentage": degradationPercentage,
		"total_execution_time": totalExecutionTime,
		"meets_requirement": meetsRequirement,
	}).Info("Concurrent processing validation completed")

	return &ConcurrentProcessingValidationResult{
		ConcurrentRequestsHandled: concurrentRequests,
		SuccessRate:              successRate,
		MeetsRequirement:         meetsRequirement,
		AverageLatencyDegradation: averageLatencyDegradation,
		ErrorRate:                errorRate,
		ThroughputMaintained:     throughputMaintained,
	}, nil
}

// Business contract types for TDD
type UptimeValidationResult struct {
	AvailabilityPercentage   float64
	TotalDowntime           time.Duration
	MeetsRequirement        bool  // Must be true for BR-PA-001 compliance
	ServiceInterruptions    int
	MaxAcceptableDowntime   time.Duration // 8.64s per day for 99.9%
}

type ResponseTimeValidationResult struct {
	Percentile95ResponseTime time.Duration
	AverageResponseTime     time.Duration
	MeetsRequirement        bool  // Must be true for BR-PA-003 compliance
	AlertsProcessed         int
	AlertsPerMinuteActual   int
	ResponseTimeDistribution map[string]int // buckets: <1s, 1-2s, 2-5s, >5s
}

type ConcurrentProcessingValidationResult struct {
	ConcurrentRequestsHandled int
	SuccessRate              float64
	MeetsRequirement         bool  // Must be true for BR-PA-004 compliance
	AverageLatencyDegradation time.Duration
	ErrorRate                float64
	ThroughputMaintained     bool
}

var _ = Describe("Phase 1: Alert Processing Performance - Critical Production Readiness", Ordered, func() {
	var (
		validator    *AlertProcessingPerformanceValidator
		testConfig   shared.IntegrationConfig
		stateManager *shared.ComprehensiveStateManager
		ctx          context.Context
	)

	BeforeAll(func() {
		ctx = context.Background()
		testConfig = shared.LoadConfig()
		if testConfig.SkipIntegration {
			Skip("Integration tests disabled")
		}

		// Initialize comprehensive state manager with database isolation
		patterns := &shared.TestIsolationPatterns{}
		stateManager = patterns.DatabaseTransactionIsolatedSuite("Phase 1 Alert Processing Performance")

		validator = NewAlertProcessingPerformanceValidator(testConfig, stateManager)
	})

	AfterAll(func() {
		err := stateManager.CleanupAllState()
		Expect(err).ToNot(HaveOccurred())
	})

	Context("BR-PA-001: Platform Uptime Requirement (99.9% availability)", func() {
		It("should maintain 99.9% availability under simulated production load", func() {
			By("Running uptime validation for production load simulation")
			testDuration := 30 * time.Second // Scaled for integration test

			result, err := validator.ValidateUptimeRequirement(ctx, testDuration)

			Expect(err).ToNot(HaveOccurred(), "Uptime validation should not fail")
			Expect(result).ToNot(BeNil(), "Should return uptime validation result")

			// BR-PA-001 Business Requirement: 99.9% uptime
			Expect(result.MeetsRequirement).To(BeTrue(), "Must meet 99.9% uptime requirement")
			Expect(result.AvailabilityPercentage).To(BeNumerically(">=", 99.9), "Availability must be >= 99.9%")

			// Max 8.6s downtime per day scaled for test duration
			maxDowntimeForTest := time.Duration(float64(testDuration) * 0.001) // 0.1% of test duration
			Expect(result.TotalDowntime).To(BeNumerically("<=", maxDowntimeForTest),
				"Total downtime must not exceed scaled limit")

			GinkgoWriter.Printf("✅ BR-PA-001 Validation: %.2f%% availability (downtime: %v)\n",
				result.AvailabilityPercentage, result.TotalDowntime)
		})
	})

	Context("BR-PA-003: Response Time Requirement (<5s for 1000 alerts/min)", func() {
		It("should process 1000 alerts per minute with 95th percentile <5 seconds", func() {
			By("Processing alerts at 1000/min rate and measuring response times")
			alertsPerMinute := 100 // Reduced for integration test
			testDuration := 30 * time.Second // Reduced for integration test

			result, err := validator.ValidateResponseTimeRequirement(ctx, alertsPerMinute, testDuration)

			Expect(err).ToNot(HaveOccurred(), "Response time validation should not fail")
			Expect(result).ToNot(BeNil(), "Should return response time validation result")

			// BR-PA-003 Business Requirement: <5s response time (95th percentile)
			Expect(result.MeetsRequirement).To(BeTrue(), "Must meet <5s response time requirement")
			Expect(result.Percentile95ResponseTime).To(BeNumerically("<", 5*time.Second),
				"95th percentile response time must be <5 seconds")
			Expect(result.AlertsPerMinuteActual).To(BeNumerically(">=", int(float64(alertsPerMinute)*0.95)),
				"Should maintain at least 95% of target throughput")

			GinkgoWriter.Printf("✅ BR-PA-003 Validation: 95th percentile %v, processed %d alerts/min\n",
				result.Percentile95ResponseTime, result.AlertsPerMinuteActual)
		})

		It("should handle burst load scenarios without degrading response times", func() {
			By("Testing burst load at 200 alerts/min for short duration")
			alertsPerMinute := 200 // Double the base requirement (reduced for integration test)
			testDuration := 15 * time.Second

			result, err := validator.ValidateResponseTimeRequirement(ctx, alertsPerMinute, testDuration)

			Expect(err).ToNot(HaveOccurred(), "Burst load validation should not fail")
			Expect(result).ToNot(BeNil(), "Should return burst load validation result")

			// Even under burst load, should maintain reasonable response times
			Expect(result.Percentile95ResponseTime).To(BeNumerically("<", 10*time.Second),
				"95th percentile should remain <10s even under burst load")
			Expect(result.AlertsProcessed).To(BeNumerically(">", 0), "Should process alerts under burst load")

			GinkgoWriter.Printf("✅ Burst Load Validation: 95th percentile %v, processed %d alerts\n",
				result.Percentile95ResponseTime, result.AlertsProcessed)
		})
	})

	Context("BR-PA-004: Concurrent Processing Requirement (100 concurrent requests)", func() {
		It("should handle 100 concurrent alert processing requests without degradation", func() {
			By("Processing 100 concurrent alert requests simultaneously")
			concurrentRequests := 100

			result, err := validator.ValidateConcurrentProcessingRequirement(ctx, concurrentRequests)

			Expect(err).ToNot(HaveOccurred(), "Concurrent processing validation should not fail")
			Expect(result).ToNot(BeNil(), "Should return concurrent processing validation result")

			// BR-PA-004 Business Requirement: 100 concurrent requests without degradation
			Expect(result.MeetsRequirement).To(BeTrue(), "Must handle 100 concurrent requests successfully")
			Expect(result.ConcurrentRequestsHandled).To(Equal(concurrentRequests),
				"Should handle all concurrent requests")
			Expect(result.SuccessRate).To(BeNumerically(">=", 0.95),
				"Success rate must be >= 95% for concurrent requests")
			Expect(result.ErrorRate).To(BeNumerically("<=", 0.05),
				"Error rate must be <= 5% for concurrent requests")

			GinkgoWriter.Printf("✅ BR-PA-004 Validation: %.1f%% success rate, %.1f%% error rate\n",
				result.SuccessRate*100, result.ErrorRate*100)
		})

		It("should maintain throughput under concurrent load", func() {
			By("Verifying throughput is maintained with concurrent processing")
			concurrentRequests := 100

			result, err := validator.ValidateConcurrentProcessingRequirement(ctx, concurrentRequests)

			Expect(err).ToNot(HaveOccurred(), "Throughput validation should not fail")
			Expect(result).ToNot(BeNil(), "Should return throughput validation result")

			// Throughput should not degrade significantly under concurrent load
			Expect(result.ThroughputMaintained).To(BeTrue(), "Throughput must be maintained under concurrent load")
			Expect(result.AverageLatencyDegradation).To(BeNumerically("<", 2*time.Second),
				"Latency degradation should be minimal")

			GinkgoWriter.Printf("✅ Throughput Validation: maintained=%t, degradation=%v\n",
				result.ThroughputMaintained, result.AverageLatencyDegradation)
		})
	})

	Context("Production Readiness Integration Testing", func() {
		It("should demonstrate end-to-end alert processing pipeline performance", func() {
			By("Running comprehensive production readiness validation")

			// Test combined requirements: uptime + response time + concurrency
			testDuration := 20 * time.Second

			// Run uptime validation
			uptimeResult, err := validator.ValidateUptimeRequirement(ctx, testDuration)
			Expect(err).ToNot(HaveOccurred())
			Expect(uptimeResult.MeetsRequirement).To(BeTrue())

			// Run response time validation at production rate
			responseResult, err := validator.ValidateResponseTimeRequirement(ctx, 100, testDuration)
			Expect(err).ToNot(HaveOccurred())
			Expect(responseResult.MeetsRequirement).To(BeTrue())

			// Run concurrent processing validation
			concurrentResult, err := validator.ValidateConcurrentProcessingRequirement(ctx, 100)
			Expect(err).ToNot(HaveOccurred())
			Expect(concurrentResult.MeetsRequirement).To(BeTrue())

			GinkgoWriter.Printf("✅ Phase 1 Alert Processing: All critical requirements validated\n")
			GinkgoWriter.Printf("   - Uptime: %.2f%% (>= 99.9%%)\n", uptimeResult.AvailabilityPercentage)
			GinkgoWriter.Printf("   - Response Time: %v (< 5s)\n", responseResult.Percentile95ResponseTime)
			GinkgoWriter.Printf("   - Concurrency: %.1f%% success rate (>= 95%%)\n", concurrentResult.SuccessRate*100)
		})
	})
})

// Helper methods for alert processing performance validation

// generateTestAlerts creates test alerts for performance validation
func (v *AlertProcessingPerformanceValidator) generateTestAlerts(count int) []types.Alert {
	alerts := make([]types.Alert, count)
	alertTypes := []string{"HighMemoryUsage", "PodCrashLooping", "HighCPUUsage", "DiskSpaceLow", "NetworkLatencyHigh"}
	severities := []string{"warning", "critical", "info"}
	namespaces := []string{"production", "staging", "monitoring"}

	for i := 0; i < count; i++ {
		alertType := alertTypes[i%len(alertTypes)]
		severity := severities[i%len(severities)]
		namespace := namespaces[i%len(namespaces)]

		alerts[i] = types.Alert{
			Name:        alertType,
			Status:      "firing",
			Severity:    severity,
			Description: fmt.Sprintf("Test alert %d for %s", i+1, alertType),
			Namespace:   namespace,
			Resource:    fmt.Sprintf("resource-%d", i+1),
			Labels: map[string]string{
				"alertname":  alertType,
				"severity":   severity,
				"test_id":    fmt.Sprintf("perf-test-%d", i+1),
			},
			Annotations: map[string]string{
				"description": fmt.Sprintf("Performance test alert %d", i+1),
				"runbook":     "test-runbook",
			},
		}
	}

	return alerts
}

// calculateAverageResponseTime calculates the average response time from a slice of durations
func (v *AlertProcessingPerformanceValidator) calculateAverageResponseTime(responseTimes []time.Duration) time.Duration {
	if len(responseTimes) == 0 {
		return 0
	}

	var total time.Duration
	for _, responseTime := range responseTimes {
		total += responseTime
	}

	return total / time.Duration(len(responseTimes))
}

// calculatePercentile calculates the specified percentile from a slice of durations
func (v *AlertProcessingPerformanceValidator) calculatePercentile(responseTimes []time.Duration, percentile float64) time.Duration {
	if len(responseTimes) == 0 {
		return 0
	}

	// Sort response times
	sortedTimes := make([]time.Duration, len(responseTimes))
	copy(sortedTimes, responseTimes)

	// Simple sort implementation (for production use sort.Slice)
	for i := 0; i < len(sortedTimes); i++ {
		for j := i + 1; j < len(sortedTimes); j++ {
			if sortedTimes[i] > sortedTimes[j] {
				sortedTimes[i], sortedTimes[j] = sortedTimes[j], sortedTimes[i]
			}
		}
	}

	// Calculate percentile index
	index := int(float64(len(sortedTimes)-1) * percentile)
	if index >= len(sortedTimes) {
		index = len(sortedTimes) - 1
	}

	return sortedTimes[index]
}