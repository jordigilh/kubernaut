//go:build integration

package llm_integration

// =============================================================================
// DEPRECATED: All mock implementations have been moved to test/integration/shared/mocks.go
//
// This file is kept only for build compatibility - all functions have been removed.
// New tests should use the standardized implementations directly:
//
// import testshared "github.com/jordigilh/kubernaut/test/integration/shared"
//
// - testshared.NewStandardPatternExtractor(mockLogger.Logger)
// - testshared.NewStandardPatternStore(mockLogger.Logger)
// - testshared.NewStandardMLAnalyzer(mockLogger.Logger)
// - testshared.NewStandardTimeSeriesAnalyzer(mockLogger.Logger)
// - testshared.NewStandardClusteringEngine(mockLogger.Logger)
// - testshared.NewStandardAnomalyDetector(mockLogger.Logger)
// - testshared.CreateStandardPatternEngine(mockLogger.Logger)
// - testshared.CreatePerformancePatternEngine(mockLogger.Logger)
//
// =============================================================================

// Imports temporarily commented out due to package reorganization
// import (
//	"github.com/jordigilh/kubernaut/pkg/intelligence/patterns"
//	testshared "github.com/jordigilh/kubernaut/test/integration/shared"
//	"github.com/jordigilh/kubernaut/pkg/testutil/mocks"
// )

// DEPRECATED: Use testshared.NewStandardPatternExtractor instead
// TEMPORARILY DISABLED: Types not available due to package reorganization
// type SimplePatternExtractor = testshared.StandardPatternExtractor

// func NewSimplePatternExtractor(mockLogger *mocks.MockLogger) *testshared.StandardPatternExtractor {
//	return testshared.NewStandardPatternExtractor(mockLogger.Logger)
// }

// DEPRECATED: Use testshared.NewStandardPatternStore instead
// TEMPORARILY DISABLED: Types not available due to package reorganization
// type SimplePatternStore = testshared.StandardPatternStore

// func NewSimplePatternStore() *testshared.StandardPatternStore {
//	return testshared.NewStandardPatternStore(mocks.NewMockLogger().Logger)
// }

// DEPRECATED: Use testshared.NewStandardMLAnalyzer instead
// TEMPORARILY DISABLED: Types not available due to package reorganization
// type SimpleMLAnalyzer = testshared.StandardMLAnalyzer

// func NewSimpleMLAnalyzer() *testshared.StandardMLAnalyzer {
//	return testshared.NewStandardMLAnalyzer(mocks.NewMockLogger().Logger)
// }

// DEPRECATED: Use testshared.NewStandardTimeSeriesAnalyzer instead
// TEMPORARILY DISABLED: Types not available due to package reorganization
// type SimpleTimeSeriesAnalyzer = testshared.StandardTimeSeriesAnalyzer

// func NewSimpleTimeSeriesAnalyzer() *testshared.StandardTimeSeriesAnalyzer {
//	return testshared.NewStandardTimeSeriesAnalyzer(mocks.NewMockLogger().Logger)
// }

// DEPRECATED: Use testshared.NewStandardClusteringEngine instead
// TEMPORARILY DISABLED: Types not available due to package reorganization
// type SimpleClusteringEngine = testshared.StandardClusteringEngine

// func NewSimpleClusteringEngine() *testshared.StandardClusteringEngine {
//	return testshared.NewStandardClusteringEngine(mocks.NewMockLogger().Logger)
// }

// DEPRECATED: Use testshared.NewStandardAnomalyDetector instead
// TEMPORARILY DISABLED: Types not available due to package reorganization
// type SimpleAnomalyDetector = testshared.StandardAnomalyDetector

// func NewSimpleAnomalyDetector() *testshared.StandardAnomalyDetector {
//	return testshared.NewStandardAnomalyDetector(mocks.NewMockLogger().Logger)
// }

// DEPRECATED: Use testshared.CreateStandardPatternEngine instead
// TEMPORARILY DISABLED: Functions not available due to package reorganization
// func CreateBasicPatternEngine(mockLogger *mocks.MockLogger) *patterns.PatternDiscoveryEngine {
//	return testshared.CreateStandardPatternEngine(mockLogger.Logger)
// }

// DEPRECATED: Use testshared.CreatePerformancePatternEngine instead
// TEMPORARILY DISABLED: Functions not available due to package reorganization
// func CreatePerformancePatternEngine(mockLogger *mocks.MockLogger) *patterns.PatternDiscoveryEngine {
//	return testshared.CreatePerformancePatternEngine(mockLogger.Logger)
// }
