//go:build integration

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
