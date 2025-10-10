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
	"errors"
	"fmt"
	"sync"
	"time"
)

// EnhancedMockEmbeddingGenerator extends MockEmbeddingGenerator with batch processing
// and advanced testing scenarios.
// Business Requirement: BR-VDB-009 - Support batch embedding generation for enterprise scale
type EnhancedMockEmbeddingGenerator struct {
	// Embedded original mock for backward compatibility
	*MockEmbeddingGenerator

	// Enhanced features
	batchSize          int
	simulateLatency    bool
	latencyPerItem     time.Duration
	rateLimitEnabled   bool
	rateLimitDelay     time.Duration
	failureRate        float64
	failurePatterns    []FailurePattern
	performanceProfile PerformanceProfile

	// Batch-specific tracking
	batchCalls      []BatchEmbeddingCall
	batchEmbeddings map[string][][]float64
	batchMutex      sync.RWMutex

	// Statistics for performance testing
	totalRequests      int64
	totalBatchRequests int64
	totalLatency       time.Duration
	maxLatency         time.Duration
	minLatency         time.Duration
}

// BatchEmbeddingCall tracks batch embedding requests
type BatchEmbeddingCall struct {
	Texts     []string
	BatchSize int
	Timestamp time.Time
	Duration  time.Duration
}

// FailurePattern defines different failure scenarios for testing
type FailurePattern struct {
	RequestNumber int // Fail on specific request number
	ErrorMessage  string
	ErrorType     FailureType
}

// FailureType categorizes different types of failures for testing
type FailureType int

const (
	RateLimitFailure FailureType = iota
	NetworkFailure
	AuthFailure
	ValidationFailure
	ServiceUnavailable
)

// PerformanceProfile defines performance characteristics for realistic testing
// Business Requirement: Support realistic performance testing aligned with SLAs
type PerformanceProfile struct {
	Name               string
	BaseLatency        time.Duration
	LatencyVariation   time.Duration
	ThroughputLimit    int     // requests per second
	QualityDegradation float64 // embedding quality reduction factor
}

// NewEnhancedMockEmbeddingGenerator creates an enhanced mock with batch processing support
// Following project guideline: All code must be backed up by at least ONE business requirement
func NewEnhancedMockEmbeddingGenerator(dimension int) *EnhancedMockEmbeddingGenerator {
	return &EnhancedMockEmbeddingGenerator{
		MockEmbeddingGenerator: NewMockEmbeddingGenerator(dimension),
		batchSize:              10,                    // Default batch size for testing
		latencyPerItem:         time.Millisecond * 10, // Realistic per-item processing time
		batchEmbeddings:        make(map[string][][]float64),
		performanceProfile: PerformanceProfile{
			Name:               "standard",
			BaseLatency:        time.Millisecond * 100,
			LatencyVariation:   time.Millisecond * 20,
			ThroughputLimit:    100,
			QualityDegradation: 0.0,
		},
		minLatency: time.Duration(^int64(0) >> 1), // Max int64 converted to duration
	}
}

// GenerateBatchEmbeddings implements ExternalEmbeddingGenerator interface
// Business Requirement: BR-VDB-009 - Must support batch embedding generation
func (e *EnhancedMockEmbeddingGenerator) GenerateBatchEmbeddings(ctx context.Context, texts []string) ([][]float64, error) {
	e.batchMutex.Lock()
	defer e.batchMutex.Unlock()

	start := time.Now()
	e.totalBatchRequests++

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Apply failure patterns for testing resilience
	if err := e.applyFailurePatterns(int(e.totalBatchRequests)); err != nil {
		return nil, err
	}

	// Simulate rate limiting for testing
	if e.rateLimitEnabled {
		time.Sleep(e.rateLimitDelay)
	}

	// Check for preset batch embeddings
	batchKey := fmt.Sprintf("batch_%v", texts)
	if preset, exists := e.batchEmbeddings[batchKey]; exists {
		duration := time.Since(start)
		e.recordPerformanceMetrics(duration)
		e.recordBatchCall(texts, duration)
		return preset, nil
	}

	// Generate embeddings with realistic processing time
	var results [][]float64
	batchSize := e.batchSize
	if batchSize > len(texts) {
		batchSize = len(texts)
	}

	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}

		batch := texts[i:end]
		batchResults, err := e.processBatch(ctx, batch)
		if err != nil {
			return nil, fmt.Errorf("failed to process batch %d-%d: %w", i, end-1, err)
		}

		results = append(results, batchResults...)

		// Simulate realistic processing latency per batch
		if e.simulateLatency {
			processingTime := time.Duration(len(batch)) * e.latencyPerItem
			time.Sleep(processingTime)
		}
	}

	duration := time.Since(start)
	e.recordPerformanceMetrics(duration)
	e.recordBatchCall(texts, duration)

	return results, nil
}

// processBatch processes a single batch of texts
// Business Requirement: Support realistic batch processing patterns
func (e *EnhancedMockEmbeddingGenerator) processBatch(ctx context.Context, texts []string) ([][]float64, error) {
	var results [][]float64

	for i, text := range texts {
		// Check for context cancellation during batch processing
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Apply failure rate for realistic error testing
		if e.shouldSimulateFailure() {
			return nil, fmt.Errorf("simulated batch processing failure on item %d: %s", i, text)
		}

		// Generate embedding using base functionality
		embedding, err := e.MockEmbeddingGenerator.GenerateEmbedding(ctx, text, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to generate embedding for item %d: %w", i, err)
		}

		// Apply quality degradation if configured
		if e.performanceProfile.QualityDegradation > 0 {
			embedding = e.applyQualityDegradation(embedding)
		}

		results = append(results, embedding)
	}

	return results, nil
}

// GetModel implements ExternalEmbeddingGenerator interface
// Business Requirement: Support model identification for testing
func (e *EnhancedMockEmbeddingGenerator) GetModel() string {
	return fmt.Sprintf("mock-embedding-model-d%d", e.GetDimension())
}

// SetBatchEmbedding sets a preset result for specific batch inputs
// Following project guideline: Provide strong testing capabilities
func (e *EnhancedMockEmbeddingGenerator) SetBatchEmbedding(texts []string, embeddings [][]float64) {
	e.batchMutex.Lock()
	defer e.batchMutex.Unlock()

	batchKey := fmt.Sprintf("batch_%v", texts)
	e.batchEmbeddings[batchKey] = embeddings
}

// EnableLatencySimulation enables realistic latency simulation
// Business Requirement: Support performance testing with realistic conditions
func (e *EnhancedMockEmbeddingGenerator) EnableLatencySimulation(latencyPerItem time.Duration) {
	e.simulateLatency = true
	e.latencyPerItem = latencyPerItem
}

// EnableRateLimit simulates rate limiting for resilience testing
// Business Requirement: Test rate limiting and exponential backoff scenarios
func (e *EnhancedMockEmbeddingGenerator) EnableRateLimit(delay time.Duration) {
	e.rateLimitEnabled = true
	e.rateLimitDelay = delay
}

// SetFailureRate configures random failure simulation for testing resilience
// Business Requirement: Test error handling and retry logic
func (e *EnhancedMockEmbeddingGenerator) SetFailureRate(rate float64) {
	if rate < 0.0 {
		rate = 0.0
	}
	if rate > 1.0 {
		rate = 1.0
	}
	e.failureRate = rate
}

// AddFailurePattern adds specific failure patterns for deterministic testing
// Business Requirement: Support predictable failure scenarios for robust testing
func (e *EnhancedMockEmbeddingGenerator) AddFailurePattern(requestNumber int, errorType FailureType, message string) {
	e.failurePatterns = append(e.failurePatterns, FailurePattern{
		RequestNumber: requestNumber,
		ErrorMessage:  message,
		ErrorType:     errorType,
	})
}

// SetPerformanceProfile configures realistic performance characteristics
// Business Requirement: Support SLA validation and performance testing
func (e *EnhancedMockEmbeddingGenerator) SetPerformanceProfile(profile PerformanceProfile) {
	e.performanceProfile = profile
	e.latencyPerItem = profile.BaseLatency
}

// GetBatchCalls returns all batch embedding calls for testing validation
// Following project guideline: Provide comprehensive testing capabilities
func (e *EnhancedMockEmbeddingGenerator) GetBatchCalls() []BatchEmbeddingCall {
	e.batchMutex.RLock()
	defer e.batchMutex.RUnlock()

	result := make([]BatchEmbeddingCall, len(e.batchCalls))
	copy(result, e.batchCalls)
	return result
}

// GetPerformanceStats returns performance statistics for validation
// Business Requirement: Support performance requirement validation
func (e *EnhancedMockEmbeddingGenerator) GetPerformanceStats() PerformanceStats {
	e.batchMutex.RLock()
	defer e.batchMutex.RUnlock()

	avgLatency := time.Duration(0)
	if e.totalRequests > 0 {
		avgLatency = time.Duration(int64(e.totalLatency) / e.totalRequests)
	}

	return PerformanceStats{
		TotalRequests:       e.totalRequests,
		TotalBatchRequests:  e.totalBatchRequests,
		AverageLatency:      avgLatency,
		MinLatency:          e.minLatency,
		MaxLatency:          e.maxLatency,
		TotalProcessingTime: e.totalLatency,
	}
}

// PerformanceStats provides performance metrics for testing validation
// Business Requirement: Support quantifiable performance testing
type PerformanceStats struct {
	TotalRequests       int64
	TotalBatchRequests  int64
	AverageLatency      time.Duration
	MinLatency          time.Duration
	MaxLatency          time.Duration
	TotalProcessingTime time.Duration
}

// ResetStats clears all performance statistics and call history
// Following project guideline: Provide clean test state management
func (e *EnhancedMockEmbeddingGenerator) ResetStats() {
	e.batchMutex.Lock()
	defer e.batchMutex.Unlock()

	e.totalRequests = 0
	e.totalBatchRequests = 0
	e.totalLatency = 0
	e.maxLatency = 0
	e.minLatency = time.Duration(^int64(0) >> 1)
	e.batchCalls = []BatchEmbeddingCall{}
	e.batchEmbeddings = make(map[string][][]float64)

	// Reset base mock state
	e.MockEmbeddingGenerator.ClearHistory()
}

// Helper methods

func (e *EnhancedMockEmbeddingGenerator) applyFailurePatterns(requestNumber int) error {
	for _, pattern := range e.failurePatterns {
		if pattern.RequestNumber == requestNumber {
			switch pattern.ErrorType {
			case RateLimitFailure:
				return errors.New("rate limit exceeded: " + pattern.ErrorMessage)
			case NetworkFailure:
				return errors.New("network error: " + pattern.ErrorMessage)
			case AuthFailure:
				return errors.New("authentication failed: " + pattern.ErrorMessage)
			case ValidationFailure:
				return errors.New("validation error: " + pattern.ErrorMessage)
			case ServiceUnavailable:
				return errors.New("service unavailable: " + pattern.ErrorMessage)
			default:
				return errors.New("unknown error: " + pattern.ErrorMessage)
			}
		}
	}
	return nil
}

func (e *EnhancedMockEmbeddingGenerator) shouldSimulateFailure() bool {
	if e.failureRate <= 0.0 {
		return false
	}
	// Simple pseudo-random failure simulation based on request count
	return float64((e.totalRequests*13)%100)/100.0 < e.failureRate
}

func (e *EnhancedMockEmbeddingGenerator) applyQualityDegradation(embedding []float64) []float64 {
	if e.performanceProfile.QualityDegradation <= 0.0 {
		return embedding
	}

	// Apply quality degradation by reducing embedding values
	degraded := make([]float64, len(embedding))
	degradationFactor := 1.0 - e.performanceProfile.QualityDegradation

	for i, val := range embedding {
		degraded[i] = val * degradationFactor
	}

	return degraded
}

func (e *EnhancedMockEmbeddingGenerator) recordBatchCall(texts []string, duration time.Duration) {
	e.batchCalls = append(e.batchCalls, BatchEmbeddingCall{
		Texts:     append([]string{}, texts...), // Create copy
		BatchSize: len(texts),
		Timestamp: time.Now(),
		Duration:  duration,
	})
}

func (e *EnhancedMockEmbeddingGenerator) recordPerformanceMetrics(duration time.Duration) {
	e.totalRequests++
	e.totalLatency += duration

	if duration > e.maxLatency {
		e.maxLatency = duration
	}
	if duration < e.minLatency {
		e.minLatency = duration
	}
}
