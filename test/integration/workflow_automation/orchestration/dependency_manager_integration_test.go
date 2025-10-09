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

//go:build integration
// +build integration

package orchestration

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"

	"github.com/jordigilh/kubernaut/pkg/orchestration/dependency"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	testshared "github.com/jordigilh/kubernaut/test/integration/shared"
)

// Mock implementations for testing

// MockVectorDB implements engine.VectorDatabase for testing
type MockVectorDB struct {
	healthy bool
	data    map[string]*engine.VectorSearchResult
}

func NewMockVectorDB(healthy bool) *MockVectorDB {
	return &MockVectorDB{
		healthy: healthy,
		data:    make(map[string]*engine.VectorSearchResult),
	}
}

func (m *MockVectorDB) Store(ctx context.Context, id string, vector []float64, metadata map[string]interface{}) error {
	if !m.healthy {
		return fmt.Errorf("mock vector database failure")
	}
	m.data[id] = &engine.VectorSearchResult{
		ID:       id,
		Vector:   vector,
		Metadata: metadata,
		Score:    1.0,
	}
	return nil
}

func (m *MockVectorDB) Search(ctx context.Context, vector []float64, limit int) ([]*engine.VectorSearchResult, error) {
	if !m.healthy {
		return nil, fmt.Errorf("mock vector database failure")
	}

	results := make([]*engine.VectorSearchResult, 0)
	for _, result := range m.data {
		results = append(results, result)
		if len(results) >= limit {
			break
		}
	}
	return results, nil
}

func (m *MockVectorDB) Delete(ctx context.Context, id string) error {
	if !m.healthy {
		return fmt.Errorf("mock vector database failure")
	}
	delete(m.data, id)
	return nil
}

func (m *MockVectorDB) SetHealthy(healthy bool) {
	m.healthy = healthy
}

// MockPatternStore implements engine.PatternStore for testing
type MockPatternStore struct {
	healthy  bool
	patterns map[string]*types.DiscoveredPattern
}

func NewMockPatternStore(healthy bool) *MockPatternStore {
	return &MockPatternStore{
		healthy:  healthy,
		patterns: make(map[string]*types.DiscoveredPattern),
	}
}

func (m *MockPatternStore) StorePattern(ctx context.Context, pattern *types.DiscoveredPattern) error {
	if !m.healthy {
		return fmt.Errorf("mock pattern store failure")
	}
	m.patterns[pattern.ID] = pattern
	return nil
}

func (m *MockPatternStore) GetPattern(ctx context.Context, patternID string) (*types.DiscoveredPattern, error) {
	if !m.healthy {
		return nil, fmt.Errorf("mock pattern store failure")
	}
	pattern, exists := m.patterns[patternID]
	if !exists {
		return nil, fmt.Errorf("pattern not found: %s", patternID)
	}
	return pattern, nil
}

func (m *MockPatternStore) ListPatterns(ctx context.Context, patternType string) ([]*types.DiscoveredPattern, error) {
	if !m.healthy {
		return nil, fmt.Errorf("mock pattern store failure")
	}

	results := make([]*types.DiscoveredPattern, 0)
	for _, pattern := range m.patterns {
		if pattern.Type == patternType {
			results = append(results, pattern)
		}
	}
	return results, nil
}

func (m *MockPatternStore) DeletePattern(ctx context.Context, patternID string) error {
	if !m.healthy {
		return fmt.Errorf("mock pattern store failure")
	}
	delete(m.patterns, patternID)
	return nil
}

func (m *MockPatternStore) SetHealthy(healthy bool) {
	m.healthy = healthy
}

// Dependency Manager Integration Test Suite
// Business Requirements: BR-REL-009, BR-ERR-007, BR-RELIABILITY-006
// Following project guidelines: Real component integration testing with controlled mocks
var _ = Describe("Dependency Manager Integration", Ordered, func() {
	var (
		ctx        context.Context
		mockLogger *mocks.MockLogger
		hooks      *testshared.TestLifecycleHooks
		depManager *dependency.DependencyManager
	)

	BeforeAll(func() {
		mockLogger = mocks.NewMockLogger()
		// mockLogger level set automatically
		ctx = context.Background()

		GinkgoWriter.Printf("🧪 Starting Dependency Manager Integration Tests\n")
	})

	AfterAll(func() {
		if hooks != nil {
			hooks.Cleanup()
		}
		GinkgoWriter.Printf("✅ Dependency Manager Integration Tests Complete\n")
	})

	// Business Requirement: BR-REL-009 - Circuit breaker integration with vector database
	Context("INTEGRATION-DEPEND-001: Vector Database Circuit Breaker Integration", func() {
		BeforeEach(func() {
			// Setup integration test environment
			hooks = testshared.SetupAIIntegrationTest("Dependency Manager Integration",
				testshared.WithRealVectorDB(),
				testshared.WithDatabaseIsolation(testshared.TransactionIsolation),
			)
			hooks.Setup()

			// Create dependency manager with real components
			depManager = dependency.NewDependencyManager(
				&dependency.DependencyConfig{
					EnableFallbacks:         true,
					CircuitBreakerThreshold: 0.5,
					HealthCheckInterval:     1 * time.Second,
					ConnectionTimeout:       5 * time.Second,
					MaxRetries:              3,
					FallbackTimeout:         2 * time.Second,
				},
				mockLogger.Logger,
			)
		})

		AfterEach(func() {
			if hooks != nil {
				hooks.Cleanup()
			}
		})

		It("should demonstrate circuit breaker behavior with mock vector database", func() {
			// Business Requirement: BR-REL-009 - Circuit breaker integration

			// Create a healthy mock vector database
			mockVectorDB := NewMockVectorDB(true)
			vectorDBDep := dependency.NewVectorDatabaseDependency("vector_db", mockVectorDB, mockLogger.Logger)
			err := depManager.RegisterDependency(vectorDBDep)
			Expect(err).ToNot(HaveOccurred())

			// Initialize fallbacks and start health monitoring
			depManager.InitializeFallbacks()
			err = depManager.StartHealthMonitoring(ctx)
			Expect(err).ToNot(HaveOccurred())
			defer depManager.StopHealthMonitoring()

			managedVDB := depManager.GetVectorDB()

			// Phase 1: Store pattern in healthy database
			testVector := []float64{0.1, 0.2, 0.3, 0.4}
			testMetadata := map[string]interface{}{
				"pattern_type": "cpu_spike",
				"namespace":    "production",
				"severity":     "critical",
			}

			err = managedVDB.Store(ctx, "test_pattern_1", testVector, testMetadata)
			Expect(err).ToNot(HaveOccurred())

			// Verify storage in healthy database
			results, err := managedVDB.Search(ctx, testVector, 1)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(results)).To(BeNumerically(">=", 1))

			// Phase 2: Simulate database failure
			mockVectorDB.SetHealthy(false)

			// Force circuit breaker to open by making failing calls
			for i := 0; i < 15; i++ { // Increased to ensure circuit breaker opens
				managedVDB.Store(ctx, fmt.Sprintf("failing_test_%d", i), testVector, testMetadata)
			}

			// Wait for circuit breaker to detect failure
			Eventually(func() bool {
				report := depManager.GetHealthReport()
				GinkgoWriter.Printf("Health report: Overall=%v, Healthy=%d, Total=%d\n",
					report.OverallHealthy, report.HealthyDependencies, report.TotalDependencies)
				return !report.OverallHealthy
			}, 15*time.Second, 1*time.Second).Should(BeTrue())

			// Phase 3: Operations should continue using fallback
			err = managedVDB.Store(ctx, "test_pattern_2", testVector, testMetadata)
			Expect(err).ToNot(HaveOccurred()) // Should succeed with fallback

			// Search should also work with fallback
			fallbackResults, err := managedVDB.Search(ctx, testVector, 5)
			Expect(err).ToNot(HaveOccurred())
			// Fallback should return stored pattern
			Expect(len(fallbackResults)).To(BeNumerically(">=", 1))

			// Verify fallback metrics
			report := depManager.GetHealthReport()
			Expect(report.FallbacksActive).To(ContainElement("vector_fallback"))
			Expect(report.FallbacksAvailable).To(ContainElement("vector_fallback"))

			// Business Validation: System maintains functionality despite database failure
			GinkgoWriter.Printf("✅ Vector database circuit breaker integration successful\n")
		})

		It("should demonstrate circuit breaker recovery behavior", func() {
			// Business Requirement: BR-REL-009 - Recovery behavior validation

			// Create a circuit breaker with short timeout for testing
			cb := dependency.NewCircuitBreaker("test-recovery", 0.5, 100*time.Millisecond)

			// Force circuit breaker to open
			for i := 0; i < 10; i++ {
				cb.Call(func() error { return fmt.Errorf("failure") })
			}
			Expect(cb.GetState()).To(Equal(dependency.CircuitStateOpen))

			// Wait for reset timeout
			time.Sleep(150 * time.Millisecond)

			// Circuit should transition to half-open on next successful call
			err := cb.Call(func() error { return nil })
			Expect(err).ToNot(HaveOccurred())

			// After successful call, circuit should be closed
			Expect(cb.GetState()).To(Equal(dependency.CircuitStateClosed))

			// Business Validation: Circuit breaker recovery mechanism works
			GinkgoWriter.Printf("✅ Circuit breaker recovery behavior validated\n")
		})
	})

	// Business Requirement: BR-ERR-007 - Pattern store circuit breaker integration
	Context("INTEGRATION-DEPEND-002: Pattern Store Circuit Breaker Integration", func() {
		BeforeEach(func() {
			hooks = testshared.SetupAIIntegrationTest("Pattern Store Integration",
				testshared.WithRealVectorDB(),
				testshared.WithDatabaseIsolation(testshared.TransactionIsolation),
			)
			hooks.Setup()

			depManager = dependency.NewDependencyManager(
				&dependency.DependencyConfig{
					EnableFallbacks:         true,
					CircuitBreakerThreshold: 0.6, // Higher threshold for pattern store
					HealthCheckInterval:     2 * time.Second,
					ConnectionTimeout:       3 * time.Second,
				},
				mockLogger.Logger,
			)
		})

		AfterEach(func() {
			if hooks != nil {
				hooks.Cleanup()
			}
		})

		It("should handle pattern store failures with circuit breaker protection", func() {
			// Business Requirement: BR-ERR-007 - Pattern store resilience

			// Create a healthy mock pattern store
			mockPatternStore := NewMockPatternStore(true)
			patternStoreDep := dependency.NewPatternStoreDependency("pattern_store", mockPatternStore, mockLogger.Logger)
			err := depManager.RegisterDependency(patternStoreDep)
			Expect(err).ToNot(HaveOccurred())
			depManager.InitializeFallbacks()

			managedPatternStore := depManager.GetPatternStore()

			// Store test patterns
			testPattern := &types.DiscoveredPattern{
				ID:          "test_pattern_001",
				Type:        "cpu_optimization",
				Description: "CPU usage optimization pattern",
				Confidence:  0.85,
				Support:     0.9,
				Metadata: map[string]interface{}{
					"actions":     []string{"scale_up"},
					"replicas":    3,
					"usage_count": 15,
					"created_at":  time.Now(),
				},
			}

			err = managedPatternStore.StorePattern(ctx, testPattern)
			Expect(err).ToNot(HaveOccurred())

			// Verify pattern retrieval
			patterns, err := managedPatternStore.ListPatterns(ctx, "cpu_optimization")
			Expect(err).ToNot(HaveOccurred())
			Expect(len(patterns)).To(BeNumerically(">=", 1))

			// Simulate pattern store failure
			mockPatternStore.SetHealthy(false)

			// Force circuit breaker to activate by making failing calls
			for i := 0; i < 10; i++ {
				managedPatternStore.StorePattern(ctx, &types.DiscoveredPattern{
					ID:   fmt.Sprintf("failing_pattern_%d", i),
					Type: "test_failure",
				})
			}

			// Circuit breaker should activate after threshold failures
			Eventually(func() bool {
				report := depManager.GetHealthReport()
				for _, status := range report.DependencyStatus {
					if status.Name == "pattern_store" && !status.IsHealthy {
						return true
					}
				}
				return false
			}, 15*time.Second, 2*time.Second).Should(BeTrue())

			// Operations should use fallback
			fallbackPattern := &types.DiscoveredPattern{
				ID:          "fallback_pattern_001",
				Type:        "memory_optimization",
				Description: "Memory optimization fallback pattern",
				Confidence:  0.7,
				Support:     0.6,
				Metadata: map[string]interface{}{
					"fallback": true,
				},
			}

			err = managedPatternStore.StorePattern(ctx, fallbackPattern)
			Expect(err).ToNot(HaveOccurred()) // Should succeed with fallback

			// Business Validation: Pattern store operations continue with fallback
			GinkgoWriter.Printf("✅ Pattern store circuit breaker protection active\n")
		})
	})

	// Business Requirement: BR-RELIABILITY-006 - Multi-dependency health monitoring
	Context("INTEGRATION-DEPEND-003: Multi-Dependency Health Monitoring", func() {
		BeforeEach(func() {
			hooks = testshared.SetupAIIntegrationTest("Multi-Dependency Health",
				testshared.WithRealVectorDB(),
				testshared.WithDatabaseIsolation(testshared.TransactionIsolation),
			)
			hooks.Setup()

			depManager = dependency.NewDependencyManager(
				&dependency.DependencyConfig{
					EnableFallbacks:     true,
					HealthCheckInterval: 1 * time.Second,
				},
				mockLogger.Logger,
			)
		})

		AfterEach(func() {
			if hooks != nil {
				hooks.Cleanup()
			}
		})

		It("should provide comprehensive health reporting across multiple dependencies", func() {
			// Business Requirement: BR-RELIABILITY-006 - Comprehensive health monitoring

			// Register multiple mock dependencies
			mockVectorDB := NewMockVectorDB(true)
			mockPatternStore := NewMockPatternStore(true)

			vectorDBDep := dependency.NewVectorDatabaseDependency("vector_db", mockVectorDB, mockLogger.Logger)
			patternStoreDep := dependency.NewPatternStoreDependency("pattern_store", mockPatternStore, mockLogger.Logger)

			err := depManager.RegisterDependency(vectorDBDep)
			Expect(err).ToNot(HaveOccurred())
			err = depManager.RegisterDependency(patternStoreDep)
			Expect(err).ToNot(HaveOccurred())

			depManager.InitializeFallbacks()

			// Start health monitoring
			err = depManager.StartHealthMonitoring(ctx)
			Expect(err).ToNot(HaveOccurred())
			defer depManager.StopHealthMonitoring()

			// Wait for initial health checks
			time.Sleep(3 * time.Second)

			// Get comprehensive health report
			report := depManager.GetHealthReport()

			// Business Validation: All dependencies should be healthy initially
			Expect(report.TotalDependencies).To(Equal(2))
			Expect(report.HealthyDependencies).To(Equal(2))
			Expect(report.OverallHealthy).To(BeTrue())

			// Verify individual dependency status
			Expect(report.DependencyStatus).To(HaveKey("vector_db"))
			Expect(report.DependencyStatus).To(HaveKey("pattern_store"))

			vectorDBStatus := report.DependencyStatus["vector_db"]
			patternStoreStatus := report.DependencyStatus["pattern_store"]

			Expect(vectorDBStatus.IsHealthy).To(BeTrue())
			Expect(vectorDBStatus.Type).To(Equal(dependency.DependencyTypeVectorDB))
			Expect(patternStoreStatus.IsHealthy).To(BeTrue())
			Expect(patternStoreStatus.Type).To(Equal(dependency.DependencyTypePatternStore))

			// Simulate partial failure (vector DB only)
			mockVectorDB.SetHealthy(false)

			// Wait for health monitoring to detect failure
			Eventually(func() bool {
				report := depManager.GetHealthReport()
				return report.HealthyDependencies == 1 && !report.OverallHealthy
			}, 10*time.Second, 1*time.Second).Should(BeTrue())

			// Verify mixed health state
			updatedReport := depManager.GetHealthReport()
			Expect(updatedReport.TotalDependencies).To(Equal(2))
			Expect(updatedReport.HealthyDependencies).To(Equal(1))
			Expect(updatedReport.OverallHealthy).To(BeFalse())

			// Vector DB should be unhealthy, pattern store should remain healthy
			Expect(updatedReport.DependencyStatus["vector_db"].IsHealthy).To(BeFalse())
			Expect(updatedReport.DependencyStatus["pattern_store"].IsHealthy).To(BeTrue())

			// Business Validation: Health monitoring accurately reflects system state
			GinkgoWriter.Printf("✅ Multi-dependency health monitoring working correctly\n")
		})

		It("should track fallback usage metrics accurately across dependencies", func() {
			// Business Requirement: BR-RELIABILITY-006 - Fallback metrics tracking

			// Register dependencies
			mockVectorDB := NewMockVectorDB(true)
			vectorDBDep := dependency.NewVectorDatabaseDependency("vector_db", mockVectorDB, mockLogger.Logger)
			err := depManager.RegisterDependency(vectorDBDep)
			Expect(err).ToNot(HaveOccurred())
			depManager.InitializeFallbacks()

			managedVDB := depManager.GetVectorDB()

			// Perform operations with healthy system
			testVector := []float64{0.1, 0.2, 0.3}
			err = managedVDB.Store(ctx, "metrics_test_1", testVector, map[string]interface{}{})
			Expect(err).ToNot(HaveOccurred())

			// Initial report should show no fallback usage
			report := depManager.GetHealthReport()
			Expect(len(report.FallbacksActive)).To(Equal(0))
			Expect(len(report.FallbacksAvailable)).To(BeNumerically(">=", 1))

			// Simulate failure to trigger fallback usage
			mockVectorDB.SetHealthy(false)

			// Perform operations that will use fallback
			for i := 0; i < 5; i++ {
				err = managedVDB.Store(ctx, fmt.Sprintf("fallback_test_%d", i), testVector, map[string]interface{}{
					"test_id": i,
				})
				Expect(err).ToNot(HaveOccurred()) // Should succeed with fallback
			}

			// Wait for circuit breaker activation
			Eventually(func() bool {
				report := depManager.GetHealthReport()
				return !report.OverallHealthy
			}, 10*time.Second, 1*time.Second).Should(BeTrue())

			// Verify fallback metrics
			finalReport := depManager.GetHealthReport()
			Expect(len(finalReport.FallbacksActive)).To(BeNumerically(">=", 1))
			Expect(finalReport.FallbacksActive).To(ContainElement("vector_fallback"))

			// Business Validation: Fallback metrics accurately tracked
			GinkgoWriter.Printf("✅ Fallback usage metrics tracked accurately\n")
		})
	})

	// Business Requirement: BR-REL-009 - Orchestration system integration
	Context("INTEGRATION-DEPEND-004: Orchestration System Integration", func() {
		BeforeEach(func() {
			hooks = testshared.SetupAIIntegrationTest("Orchestration Integration",
				testshared.WithRealVectorDB(),
				testshared.WithDatabaseIsolation(testshared.TransactionIsolation),
			)
			hooks.Setup()

			depManager = dependency.NewDependencyManager(
				&dependency.DependencyConfig{
					EnableFallbacks:     true,
					HealthCheckInterval: 2 * time.Second,
				},
				mockLogger.Logger,
			)
		})

		AfterEach(func() {
			if hooks != nil {
				hooks.Cleanup()
			}
		})

		It("should integrate seamlessly with orchestration components", func() {
			// Business Requirement: BR-REL-009 - Orchestration integration

			// Register mock dependencies
			mockVectorDB := NewMockVectorDB(true)
			mockPatternStore := NewMockPatternStore(true)

			vectorDBDep := dependency.NewVectorDatabaseDependency("vector_db", mockVectorDB, mockLogger.Logger)
			patternStoreDep := dependency.NewPatternStoreDependency("pattern_store", mockPatternStore, mockLogger.Logger)

			err := depManager.RegisterDependency(vectorDBDep)
			Expect(err).ToNot(HaveOccurred())
			err = depManager.RegisterDependency(patternStoreDep)
			Expect(err).ToNot(HaveOccurred())

			depManager.InitializeFallbacks()

			// Get managed services for orchestrator
			managedVDB := depManager.GetVectorDB()
			managedPatternStore := depManager.GetPatternStore()

			// Simulate workflow execution scenario
			// 1. Store workflow patterns
			workflowPattern := &types.DiscoveredPattern{
				ID:          "workflow_pattern_001",
				Type:        "alert_response",
				Description: "Standard alert response workflow",
				Confidence:  0.9,
				Support:     0.85,
				Metadata: map[string]interface{}{
					"actions": []map[string]interface{}{
						{"type": "analyze", "depth": "full"},
						{"type": "respond", "urgency": "high"},
					},
					"created_at": time.Now(),
				},
			}

			err = managedPatternStore.StorePattern(ctx, workflowPattern)
			Expect(err).ToNot(HaveOccurred())

			// 2. Store vector embeddings for pattern matching
			patternVector := []float64{0.8, 0.6, 0.4, 0.2}
			err = managedVDB.Store(ctx, "workflow_vector_001", patternVector, map[string]interface{}{
				"pattern_id":    "workflow_pattern_001",
				"workflow_type": "alert_response",
				"confidence":    0.9,
			})
			Expect(err).ToNot(HaveOccurred())

			// 3. Simulate pattern matching for new alert
			queryVector := []float64{0.75, 0.65, 0.45, 0.25} // Similar to stored pattern
			searchResults, err := managedVDB.Search(ctx, queryVector, 3)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(searchResults)).To(BeNumerically(">=", 1))

			// 4. Retrieve matching patterns
			patterns, err := managedPatternStore.ListPatterns(ctx, "alert_response")
			Expect(err).ToNot(HaveOccurred())
			Expect(len(patterns)).To(BeNumerically(">=", 1))

			// Business Validation: Orchestration workflow completes successfully
			foundPattern := patterns[0]
			Expect(foundPattern.ID).To(Equal("workflow_pattern_001"))
			Expect(foundPattern.Confidence).To(BeNumerically(">=", 0.8))

			// Verify system health during orchestration
			report := depManager.GetHealthReport()
			Expect(report.OverallHealthy).To(BeTrue())
			Expect(report.HealthyDependencies).To(Equal(2))

			GinkgoWriter.Printf("✅ Orchestration system integration successful\n")
		})
	})
})
