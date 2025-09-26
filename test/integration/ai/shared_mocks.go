//go:build integration

package ai

// =============================================================================
// DEPRECATED: All mock implementations have been moved to test/integration/shared/mocks.go
//
// This file only provides backwards compatibility aliases for existing tests.
// New tests should use the standardized implementations directly:
//
// import testshared "github.com/jordigilh/kubernaut/test/integration/shared"
//
// - testshared.NewStandardPatternExtractor(logger)
// - testshared.NewStandardPatternStore(logger)
// - testshared.NewStandardMLAnalyzer(logger)
// - testshared.NewStandardTimeSeriesAnalyzer(logger)
// - testshared.NewStandardClusteringEngine(logger)
// - testshared.NewStandardAnomalyDetector(logger)
// - testshared.CreateStandardPatternEngine(logger)
// - testshared.CreatePerformancePatternEngine(logger)
//
// =============================================================================

// No imports needed - all functions have been removed
// Use direct imports from testshared package in your test files

// REMOVED: Deprecated type aliases and functions
// Following project guidelines: Use direct imports instead of type aliases
//
// Use testshared.NewStandardPatternExtractor(logger) directly
// Use testshared.NewStandardPatternStore(logger) directly
// Use testshared.NewStandardMLAnalyzer(logger) directly
// Use testshared.NewStandardTimeSeriesAnalyzer(logger) directly
// Use testshared.NewStandardClusteringEngine(logger) directly
// Use testshared.NewStandardAnomalyDetector(logger) directly

// REMOVED: Additional deprecated functions
// Use testshared.CreateStandardPatternEngine(logger) directly
// Use testshared.CreatePerformancePatternEngine(logger) directly
