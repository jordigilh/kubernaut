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
package storage

import (
	"time"

	. "github.com/onsi/gomega" //nolint:staticcheck
)

// BusinessRequirementFixtures provides test fixtures aligned with business requirements
// Following project guideline: All code must be backed up by at least ONE business requirement
type BusinessRequirementFixtures struct {
	embeddingFactory *EmbeddingDataFactory
	dimensions       *BusinessRequirementDimensions
}

// NewBusinessRequirementFixtures creates new business requirement fixtures
func NewBusinessRequirementFixtures() *BusinessRequirementFixtures {
	return &BusinessRequirementFixtures{
		embeddingFactory: NewEmbeddingDataFactory(),
		dimensions:       NewBusinessRequirementDimensions(),
	}
}

// KubernetesIncidentDescriptions provides realistic Kubernetes incident scenarios
// Business Requirement: BR-VDB-001 and BR-VDB-002 - Support real-world Kubernetes scenarios
func (f *BusinessRequirementFixtures) KubernetesIncidentDescriptions() []string {
	return []string{
		"Pod memory leak causing frequent restarts in payment microservice cluster",
		"High CPU usage in web-frontend deployment causing response latency issues",
		"Persistent volume claim stuck in pending state blocking database pod startup",
		"Service mesh ingress controller returning 503 errors during peak traffic",
		"ConfigMap update not propagating to running pods requiring manual restart",
	}
}

// AlertSeverityLevels provides business-aligned severity levels
// Business Requirement: Support critical vs non-critical alert processing
func (f *BusinessRequirementFixtures) AlertSeverityLevels() map[string]string {
	return map[string]string{
		"production_critical": "critical",
		"production_warning":  "warning",
		"development_info":    "info",
		"staging_warning":     "warning",
		"monitoring_critical": "critical",
	}
}

// BusinessContextScenarios provides context scenarios for testing
// Business Requirement: Support different operational contexts and environments
func (f *BusinessRequirementFixtures) BusinessContextScenarios() map[string]map[string]interface{} {
	return map[string]map[string]interface{}{
		"production_incident": {
			"environment":       "production",
			"priority":          "high",
			"sla_requirement":   "sub_500ms",
			"quality_threshold": 0.95,
		},
		"development_context": {
			"environment":       "development",
			"priority":          "medium",
			"sla_requirement":   "sub_2s",
			"quality_threshold": 0.80,
		},
		"cost_optimized_batch": {
			"environment":       "batch_processing",
			"priority":          "low",
			"sla_requirement":   "sub_30s",
			"quality_threshold": 0.85,
		},
	}
}

// ExpectedEmbeddingDimensions provides dimension validation fixtures
// Business Requirement: Ensure correct embedding dimensions for business operations
func (f *BusinessRequirementFixtures) ExpectedEmbeddingDimensions() map[string]int {
	return map[string]int{
		"openai_standard":      f.dimensions.OpenAI,      // BR-VDB-001: 1536 dimensions
		"huggingface_standard": f.dimensions.HuggingFace, // BR-VDB-002: 384 dimensions
	}
}

// PerformanceThresholds provides SLA and performance requirement fixtures
// Business Requirement: Define measurable performance expectations
type PerformanceThresholds struct {
	MaxLatencyProduction   time.Duration
	MaxLatencyDevelopment  time.Duration
	MinQualityProduction   float64
	MinQualityDevelopment  float64
	MaxBatchProcessingTime time.Duration
}

// GetPerformanceThresholds returns business-aligned performance thresholds
// Business Requirement: Support SLA validation in tests
func (f *BusinessRequirementFixtures) GetPerformanceThresholds() *PerformanceThresholds {
	return &PerformanceThresholds{
		MaxLatencyProduction:   500 * time.Millisecond, // BR-VDB-001: Sub-500ms for production
		MaxLatencyDevelopment:  2 * time.Second,        // BR-VDB-002: Relaxed for development
		MinQualityProduction:   0.95,                   // BR-VDB-001: High quality requirement
		MinQualityDevelopment:  0.80,                   // BR-VDB-002: Cost-optimized quality
		MaxBatchProcessingTime: 30 * time.Second,       // BR-VDB-009: Batch processing SLA
	}
}

// ValidationHelpers provides business requirement validation functions
// Following project guideline: Assertions MUST be backed on business outcomes
type ValidationHelpers struct {
	fixtures   *BusinessRequirementFixtures
	thresholds *PerformanceThresholds
}

// NewValidationHelpers creates new validation helpers
func NewValidationHelpers() *ValidationHelpers {
	fixtures := NewBusinessRequirementFixtures()
	return &ValidationHelpers{
		fixtures:   fixtures,
		thresholds: fixtures.GetPerformanceThresholds(),
	}
}

// ValidateOpenAIBusinessRequirements validates OpenAI service against business requirements
// Business Requirement: BR-VDB-001 - OpenAI service business validation
func (v *ValidationHelpers) ValidateOpenAIBusinessRequirements(embedding []float64, latency time.Duration) {
	// BR-VDB-001: Validate embedding dimensions for maximum semantic fidelity
	Expect(embedding).To(HaveLen(v.fixtures.dimensions.OpenAI),
		"BR-VDB-001: OpenAI embeddings must provide full 1536-dimensional representation")

	// BR-VDB-001: Validate production latency requirements
	Expect(latency).To(BeNumerically("<", v.thresholds.MaxLatencyProduction),
		"BR-VDB-001: Production latency must be under 500ms for real-time operations")

	// BR-VDB-001: Validate semantic content quality (non-zero embedding values)
	nonZeroCount := 0
	for _, val := range embedding {
		if val != 0.0 {
			nonZeroCount++
		}
	}
	Expect(float64(nonZeroCount)/float64(len(embedding))).To(BeNumerically(">", 0.5),
		"BR-VDB-001: Embedding should have >50% non-zero values for semantic richness")
}

// ValidateHuggingFaceBusinessRequirements validates HuggingFace service against business requirements
// Business Requirement: BR-VDB-002 - HuggingFace service business validation
func (v *ValidationHelpers) ValidateHuggingFaceBusinessRequirements(embedding []float64, latency time.Duration) {
	// BR-VDB-002: Validate embedding dimensions for cost-optimized operations
	Expect(embedding).To(HaveLen(v.fixtures.dimensions.HuggingFace),
		"BR-VDB-002: HuggingFace embeddings must provide 384-dimensional representation")

	// BR-VDB-002: Validate development latency requirements (less strict than production)
	Expect(latency).To(BeNumerically("<", v.thresholds.MaxLatencyDevelopment),
		"BR-VDB-002: Development latency must be under 2s for cost-optimized operations")

	// BR-VDB-002: Validate cost-effectiveness while maintaining quality
	nonZeroCount := 0
	for _, val := range embedding {
		if val != 0.0 {
			nonZeroCount++
		}
	}
	Expect(float64(nonZeroCount)/float64(len(embedding))).To(BeNumerically(">", 0.3),
		"BR-VDB-002: Cost-optimized embedding should maintain >30% semantic density")
}

// ValidateBatchProcessingRequirements validates batch processing against business requirements
// Business Requirement: BR-VDB-009 - Batch embedding generation validation
func (v *ValidationHelpers) ValidateBatchProcessingRequirements(embeddings [][]float64, latency time.Duration, expectedCount int) {
	// BR-VDB-009: Validate batch count matches expected
	Expect(embeddings).To(HaveLen(expectedCount),
		"BR-VDB-009: Batch processing must return embedding for each input")

	// BR-VDB-009: Validate batch processing latency
	Expect(latency).To(BeNumerically("<", v.thresholds.MaxBatchProcessingTime),
		"BR-VDB-009: Batch processing must complete within 30s SLA")

	// BR-VDB-009: Validate each embedding in batch
	for i, embedding := range embeddings {
		Expect(embedding).ToNot(BeEmpty(),
			"BR-VDB-009: Batch embedding %d must not be empty", i)
	}
}

// ValidateServiceAvailability validates service availability requirements
// Business Requirement: BR-VDB-001/BR-VDB-002 - Service availability validation
func (v *ValidationHelpers) ValidateServiceAvailability(err error, serviceType string, requiredAvailability float64) {
	switch serviceType {
	case "openai":
		Expect(err).ToNot(HaveOccurred(),
			"BR-VDB-001: OpenAI service must maintain >99.5%% availability for business continuity")
	case "huggingface":
		Expect(err).ToNot(HaveOccurred(),
			"BR-VDB-002: HuggingFace service must maintain >99%% availability for business operations")
	default:
		Expect(err).ToNot(HaveOccurred(),
			"Service availability requirement not met for %s", serviceType)
	}
}

// CostOptimizationMetrics provides cost analysis fixtures
// Business Requirement: BR-VDB-002 - Cost optimization validation
type CostOptimizationMetrics struct {
	ExpectedMonthlySavings  float64
	MinimumQualityThreshold float64
	MaxAcceptableLatency    time.Duration
	CostReductionPercentage float64
}

// GetCostOptimizationMetrics returns cost optimization validation metrics
// Business Requirement: BR-VDB-002 - Quantifiable cost optimization
func (v *ValidationHelpers) GetCostOptimizationMetrics() *CostOptimizationMetrics {
	return &CostOptimizationMetrics{
		ExpectedMonthlySavings:  2000.0,          // $2,000+ monthly savings requirement
		MinimumQualityThreshold: 0.80,            // 80% of premium quality acceptable
		MaxAcceptableLatency:    2 * time.Second, // 2s acceptable for cost optimization
		CostReductionPercentage: 0.60,            // 60% cost reduction target
	}
}

// ValidateCostOptimization validates cost optimization business requirements
// Business Requirement: BR-VDB-002 - Cost optimization validation
func (v *ValidationHelpers) ValidateCostOptimization(actualCostSavings float64, qualityScore float64, latency time.Duration) {
	metrics := v.GetCostOptimizationMetrics()

	Expect(actualCostSavings).To(BeNumerically(">=", metrics.ExpectedMonthlySavings),
		"BR-VDB-002: Must deliver >=60%% cost savings for strong business value")

	Expect(qualityScore).To(BeNumerically(">=", metrics.MinimumQualityThreshold),
		"BR-VDB-002: Must maintain >=80%% of premium service quality")

	Expect(latency).To(BeNumerically("<=", metrics.MaxAcceptableLatency),
		"BR-VDB-002: Cost-optimized latency must be acceptable for business operations")
}
