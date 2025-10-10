<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
package mocks

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

// MockScenarios provides pre-configured mock scenarios for common testing patterns
// Following project guideline: AVOID duplication and REUSE existing code
type MockScenarios struct {
	logger *logrus.Logger
}

// NewMockScenarios creates a new mock scenarios builder
func NewMockScenarios(logger *logrus.Logger) *MockScenarios {
	return &MockScenarios{
		logger: logger,
	}
}

// CreateProductionOpenAIScenario creates a production-like OpenAI service mock
// Business Requirement: BR-VDB-001 - Support production OpenAI testing patterns
func (ms *MockScenarios) CreateProductionOpenAIScenario() *IntegrationMockEmbeddingService {
	service := NewIntegrationMockEmbeddingService(OpenAIServiceType, 1536, ms.logger)

	// Configure production-like settings
	service.SetOperationMode(HighAvailabilityMode)
	service.EnableLoadBalancing([]string{"openai-lb-1", "openai-lb-2", "openai-lb-3"})
	service.EnableAutoScaling()

	// Set realistic performance profile
	service.generator.SetPerformanceProfile(PerformanceProfile{
		Name:               "production_openai",
		BaseLatency:        time.Millisecond * 150,
		LatencyVariation:   time.Millisecond * 30,
		ThroughputLimit:    200,
		QualityDegradation: 0.0, // No quality degradation in production
	})

	// Configure minimal failure rate for high availability
	service.generator.SetFailureRate(0.001) // 0.1% failure rate

	return service
}

// CreateCostOptimizedHuggingFaceScenario creates a cost-optimized HuggingFace service mock
// Business Requirement: BR-VDB-002 - Support cost-optimized testing patterns
func (ms *MockScenarios) CreateCostOptimizedHuggingFaceScenario() *IntegrationMockEmbeddingService {
	service := NewIntegrationMockEmbeddingService(HuggingFaceServiceType, 384, ms.logger)

	// Configure cost-optimized settings
	service.SetOperationMode(CostOptimizedMode)

	// Set cost-optimized performance profile
	service.generator.SetPerformanceProfile(PerformanceProfile{
		Name:               "cost_optimized_huggingface",
		BaseLatency:        time.Millisecond * 300,
		LatencyVariation:   time.Millisecond * 100,
		ThroughputLimit:    80,
		QualityDegradation: 0.05, // 5% quality trade-off for cost savings
	})

	// Higher failure rate for cost-optimized service
	service.generator.SetFailureRate(0.02) // 2% failure rate

	return service
}

// CreateDevelopmentScenario creates a development environment mock
// Business Requirement: Support development testing with relaxed SLAs
func (ms *MockScenarios) CreateDevelopmentScenario() *IntegrationMockEmbeddingService {
	service := NewIntegrationMockEmbeddingService(LocalServiceType, 512, ms.logger)

	// Configure development settings
	service.SetOperationMode(DevelopmentMode)

	// Fast processing for development
	service.generator.EnableLatencySimulation(time.Millisecond * 10)
	service.generator.SetFailureRate(0.05) // 5% failure rate for testing error handling

	return service
}

// CreateRateLimitingScenario creates a scenario that simulates rate limiting
// Business Requirement: Test rate limiting and exponential backoff patterns
func (ms *MockScenarios) CreateRateLimitingScenario() *EnhancedMockEmbeddingGenerator {
	mock := NewEnhancedMockEmbeddingGenerator(1536)

	// Enable rate limiting simulation
	mock.EnableRateLimit(time.Second * 2)

	// Add specific failure patterns for rate limiting
	mock.AddFailurePattern(3, RateLimitFailure, "Rate limit exceeded, please retry")
	mock.AddFailurePattern(6, RateLimitFailure, "API quota exceeded")
	mock.AddFailurePattern(10, RateLimitFailure, "Too many requests")

	return mock
}

// CreateNetworkFailureScenario creates a scenario that simulates network issues
// Business Requirement: Test network resilience and retry logic
func (ms *MockScenarios) CreateNetworkFailureScenario() *EnhancedMockEmbeddingGenerator {
	mock := NewEnhancedMockEmbeddingGenerator(1536)

	// Add network failure patterns
	mock.AddFailurePattern(2, NetworkFailure, "Connection timeout")
	mock.AddFailurePattern(5, NetworkFailure, "DNS resolution failed")
	mock.AddFailurePattern(8, NetworkFailure, "Connection refused")

	// Set realistic network-related latency
	mock.EnableLatencySimulation(time.Millisecond * 200)

	return mock
}

// CreateHighVolumeScenario creates a scenario for testing high-volume batch processing
// Business Requirement: BR-VDB-009 - Support enterprise batch processing testing
func (ms *MockScenarios) CreateHighVolumeScenario() *IntegrationMockEmbeddingService {
	service := NewIntegrationMockEmbeddingService(HybridServiceType, 768, ms.logger)

	// Configure for high volume
	service.SetOperationMode(HighAvailabilityMode)
	service.EnableLoadBalancing([]string{"batch-lb-1", "batch-lb-2", "batch-lb-3", "batch-lb-4"})
	service.EnableAutoScaling()

	// Optimize for batch processing
	service.generator.SetPerformanceProfile(PerformanceProfile{
		Name:               "high_volume_batch",
		BaseLatency:        time.Millisecond * 50,
		LatencyVariation:   time.Millisecond * 10,
		ThroughputLimit:    500,
		QualityDegradation: 0.0,
	})

	// Minimal failure rate for reliable batch processing
	service.generator.SetFailureRate(0.0005) // 0.05% failure rate

	return service
}

// CreateDisasterRecoveryScenario creates a scenario simulating disaster recovery conditions
// Business Requirement: Test disaster recovery and degraded service scenarios
func (ms *MockScenarios) CreateDisasterRecoveryScenario() *IntegrationMockEmbeddingService {
	service := NewIntegrationMockEmbeddingService(OpenAIServiceType, 1536, ms.logger)

	// Configure disaster recovery mode
	service.SetOperationMode(DisasterRecoveryMode)
	service.SetHealthStatus(DegradedStatus)

	// Simulate degraded performance
	service.generator.SetPerformanceProfile(PerformanceProfile{
		Name:               "disaster_recovery",
		BaseLatency:        time.Millisecond * 800,
		LatencyVariation:   time.Millisecond * 300,
		ThroughputLimit:    20,
		QualityDegradation: 0.1, // 10% quality degradation in DR mode
	})

	// Higher failure rate during disaster recovery
	service.generator.SetFailureRate(0.15) // 15% failure rate

	return service
}

// CreatePerformanceTestingScenario creates a scenario for comprehensive performance testing
// Business Requirement: Support comprehensive performance validation
func (ms *MockScenarios) CreatePerformanceTestingScenario() *EnhancedMockEmbeddingGenerator {
	mock := NewEnhancedMockEmbeddingGenerator(1536)

	// Configure realistic performance characteristics
	mock.SetPerformanceProfile(PerformanceProfile{
		Name:               "performance_testing",
		BaseLatency:        time.Millisecond * 200,
		LatencyVariation:   time.Millisecond * 50,
		ThroughputLimit:    150,
		QualityDegradation: 0.0,
	})

	// Enable latency simulation for accurate performance testing
	mock.EnableLatencySimulation(time.Millisecond * 200)

	// Add varied failure patterns for comprehensive testing
	mock.AddFailurePattern(20, RateLimitFailure, "Performance test rate limit")
	mock.AddFailurePattern(50, NetworkFailure, "Performance test network issue")
	mock.AddFailurePattern(100, ServiceUnavailable, "Performance test service overload")

	return mock
}

// ScenarioTestHelper provides helper methods for testing with scenarios
type ScenarioTestHelper struct {
	scenarios *MockScenarios
	logger    *logrus.Logger
}

// NewScenarioTestHelper creates a new scenario test helper
func NewScenarioTestHelper(logger *logrus.Logger) *ScenarioTestHelper {
	return &ScenarioTestHelper{
		scenarios: NewMockScenarios(logger),
		logger:    logger,
	}
}

// ValidateProductionScenario validates production scenario requirements
// Business Requirement: BR-VDB-001 - Validate production service requirements
func (sth *ScenarioTestHelper) ValidateProductionScenario(ctx context.Context, service *IntegrationMockEmbeddingService, texts []string) error {
	start := time.Now()

	// Test batch processing
	embeddings, err := service.GenerateBatchEmbeddings(ctx, texts)
	if err != nil {
		return err
	}

	duration := time.Since(start)

	// Validate production requirements
	metrics := service.GetServiceMetrics()

	// Production SLA: < 500ms for batches under 50 items
	if len(texts) <= 50 && duration > time.Millisecond*500 {
		return fmt.Errorf("production SLA violated: %v > 500ms for batch size %d", duration, len(texts))
	}

	// Production availability: > 99.5%
	if metrics.Availability < 0.995 {
		return fmt.Errorf("production availability requirement not met: %.3f < 99.5%%", metrics.Availability*100)
	}

	// Validate embedding quality (all embeddings should be non-empty)
	for i, embedding := range embeddings {
		if len(embedding) == 0 {
			return fmt.Errorf("empty embedding at index %d", i)
		}
	}

	sth.logger.WithFields(logrus.Fields{
		"batch_size":     len(texts),
		"duration_ms":    duration.Milliseconds(),
		"availability":   metrics.Availability,
		"error_rate":     metrics.ErrorRate,
		"avg_latency_ms": metrics.AverageLatency.Milliseconds(),
	}).Info("Production scenario validation completed")

	return nil
}

// ValidateCostOptimizedScenario validates cost-optimized scenario requirements
// Business Requirement: BR-VDB-002 - Validate cost optimization requirements
func (sth *ScenarioTestHelper) ValidateCostOptimizedScenario(ctx context.Context, service *IntegrationMockEmbeddingService, texts []string) error {
	start := time.Now()

	embeddings, err := service.GenerateBatchEmbeddings(ctx, texts)
	if err != nil {
		return err
	}

	duration := time.Since(start)
	metrics := service.GetServiceMetrics()

	// Cost-optimized SLA: < 5s (more relaxed than production)
	if duration > time.Second*5 {
		return fmt.Errorf("cost-optimized SLA violated: %v > 5s for batch size %d", duration, len(texts))
	}

	// Cost-optimized availability: > 99% (lower than production but still high)
	if metrics.Availability < 0.99 {
		return fmt.Errorf("cost-optimized availability requirement not met: %.2f < 99%%", metrics.Availability*100)
	}

	// Validate cost optimization (higher latency acceptable for cost savings)
	if metrics.AverageLatency < time.Millisecond*200 {
		sth.logger.Warn("Cost-optimized service showing unexpectedly low latency")
	}

	// Validate embeddings are still functional despite cost optimization
	for i, embedding := range embeddings {
		if len(embedding) == 0 {
			return fmt.Errorf("empty embedding at index %d in cost-optimized scenario", i)
		}
	}

	sth.logger.WithFields(logrus.Fields{
		"batch_size":   len(texts),
		"duration_ms":  duration.Milliseconds(),
		"availability": metrics.Availability,
		"cost_savings": "estimated_60_percent",
	}).Info("Cost-optimized scenario validation completed")

	return nil
}

// ValidateBatchProcessingScenario validates batch processing requirements
// Business Requirement: BR-VDB-009 - Validate enterprise batch processing
func (sth *ScenarioTestHelper) ValidateBatchProcessingScenario(ctx context.Context, service *IntegrationMockEmbeddingService, largeBatch []string) error {
	if len(largeBatch) < 100 {
		return fmt.Errorf("batch size too small for enterprise testing: %d < 100", len(largeBatch))
	}

	start := time.Now()
	embeddings, err := service.GenerateBatchEmbeddings(ctx, largeBatch)
	if err != nil {
		return err
	}

	duration := time.Since(start)

	// Validate batch processing completed successfully
	if len(embeddings) != len(largeBatch) {
		return fmt.Errorf("batch processing failed: expected %d embeddings, got %d", len(largeBatch), len(embeddings))
	}

	// Enterprise batch SLA: < 30s for batches up to 1000 items
	maxDuration := time.Second * 30
	if len(largeBatch) > 1000 {
		maxDuration = time.Duration(len(largeBatch)/1000) * time.Second * 30
	}

	if duration > maxDuration {
		return fmt.Errorf("enterprise batch SLA violated: %v > %v for batch size %d", duration, maxDuration, len(largeBatch))
	}

	sth.logger.WithFields(logrus.Fields{
		"batch_size":       len(largeBatch),
		"duration_ms":      duration.Milliseconds(),
		"items_per_second": float64(len(largeBatch)) / duration.Seconds(),
		"sla_threshold_ms": maxDuration.Milliseconds(),
	}).Info("Enterprise batch processing validation completed")

	return nil
}
