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

import (
	"github.com/jordigilh/kubernaut/pkg/intelligence/patterns"
	testshared "github.com/jordigilh/kubernaut/test/integration/shared"
	"github.com/sirupsen/logrus"
)

// DEPRECATED: Use testshared.NewStandardPatternExtractor instead
type SimplePatternExtractor = testshared.StandardPatternExtractor

func NewSimplePatternExtractor(logger *logrus.Logger) *testshared.StandardPatternExtractor {
	return testshared.NewStandardPatternExtractor(logger)
}

// DEPRECATED: Use testshared.NewStandardPatternStore instead
type SimplePatternStore = testshared.StandardPatternStore

func NewSimplePatternStore() *testshared.StandardPatternStore {
	return testshared.NewStandardPatternStore(logrus.New())
}

// DEPRECATED: Use testshared.NewStandardMLAnalyzer instead
type SimpleMLAnalyzer = testshared.StandardMLAnalyzer

func NewSimpleMLAnalyzer() *testshared.StandardMLAnalyzer {
	return testshared.NewStandardMLAnalyzer(logrus.New())
}

// DEPRECATED: Use testshared.NewStandardTimeSeriesAnalyzer instead
type SimpleTimeSeriesAnalyzer = testshared.StandardTimeSeriesAnalyzer

func NewSimpleTimeSeriesAnalyzer() *testshared.StandardTimeSeriesAnalyzer {
	return testshared.NewStandardTimeSeriesAnalyzer(logrus.New())
}

// DEPRECATED: Use testshared.NewStandardClusteringEngine instead
type SimpleClusteringEngine = testshared.StandardClusteringEngine

func NewSimpleClusteringEngine() *testshared.StandardClusteringEngine {
	return testshared.NewStandardClusteringEngine(logrus.New())
}

// DEPRECATED: Use testshared.NewStandardAnomalyDetector instead
type SimpleAnomalyDetector = testshared.StandardAnomalyDetector

func NewSimpleAnomalyDetector() *testshared.StandardAnomalyDetector {
	return testshared.NewStandardAnomalyDetector(logrus.New())
}

// DEPRECATED: Use testshared.CreateStandardPatternEngine instead
func CreateBasicPatternEngine(logger *logrus.Logger) *patterns.PatternDiscoveryEngine {
	return testshared.CreateStandardPatternEngine(logger)
}

// DEPRECATED: Use testshared.CreatePerformancePatternEngine instead
func CreatePerformancePatternEngine(logger *logrus.Logger) *patterns.PatternDiscoveryEngine {
	return testshared.CreatePerformancePatternEngine(logger)
}
