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
package testutil

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/intelligence/patterns"
)

// IntelligenceTestSuiteComponents contains common test setup components for intelligence tests
type IntelligenceTestSuiteComponents struct {
	Context                context.Context
	Logger                 *logrus.Logger
	PatternDiscoveryConfig *patterns.PatternDiscoveryConfig
	LearningMetrics        *patterns.LearningMetrics
}

// IntelligenceTestSuite creates a standardized test suite setup for intelligence tests
func IntelligenceTestSuite(testName string) *IntelligenceTestSuiteComponents {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel) // Suppress logs during tests

	// Create default pattern discovery configuration
	patternConfig := &patterns.PatternDiscoveryConfig{
		MinExecutionsForPattern: 10,
		MaxHistoryDays:          90,
		SimilarityThreshold:     0.85,
		PredictionConfidence:    0.7,
		ClusteringEpsilon:       0.3,
		MinClusterSize:          5,
		PatternCacheSize:        1000,
		EnableRealTimeDetection: true,
	}

	return &IntelligenceTestSuiteComponents{
		Context:                context.Background(),
		Logger:                 logger,
		PatternDiscoveryConfig: patternConfig,
		LearningMetrics:        patterns.NewLearningMetrics(),
	}
}

// PatternDiscoveryTestSuite creates a specialized setup for pattern discovery tests
func PatternDiscoveryTestSuite(testName string) *IntelligenceTestSuiteComponents {
	return IntelligenceTestSuite(testName)
}

// MLValidationTestSuite creates a specialized setup for ML validation tests
func MLValidationTestSuite(testName string) *IntelligenceTestSuiteComponents {
	return IntelligenceTestSuite(testName)
}

// PatternEngineTestSuite creates a specialized setup for pattern engine tests
func PatternEngineTestSuite(testName string) *IntelligenceTestSuiteComponents {
	return IntelligenceTestSuite(testName)
}

// StatisticalValidationTestSuite creates a specialized setup for statistical validation tests
func StatisticalValidationTestSuite(testName string) *IntelligenceTestSuiteComponents {
	components := IntelligenceTestSuite(testName)

	// Initialize statistical validation with enhanced config
	enhancedConfig := &patterns.PatternDiscoveryConfig{
		MinExecutionsForPattern: 20,
		MaxHistoryDays:          180,
		SimilarityThreshold:     0.90,
		PredictionConfidence:    0.8,
		ClusteringEpsilon:       0.2,
		MinClusterSize:          10,
		PatternCacheSize:        2000,
		EnableRealTimeDetection: true,
	}

	components.PatternDiscoveryConfig = enhancedConfig

	return components
}

// UpdatePatternDiscoveryConfig updates the pattern discovery configuration
func (c *IntelligenceTestSuiteComponents) UpdatePatternDiscoveryConfig(config *patterns.PatternDiscoveryConfig) {
	c.PatternDiscoveryConfig = config
}

// GetDefaultPatternDiscoveryConfig returns the default pattern discovery configuration
func (c *IntelligenceTestSuiteComponents) GetDefaultPatternDiscoveryConfig() *patterns.PatternDiscoveryConfig {
	return c.PatternDiscoveryConfig
}

// GetEnhancedPatternDiscoveryConfig returns an enhanced pattern discovery configuration
func (c *IntelligenceTestSuiteComponents) GetEnhancedPatternDiscoveryConfig() *patterns.PatternDiscoveryConfig {
	return &patterns.PatternDiscoveryConfig{
		MinExecutionsForPattern: 50,
		MaxHistoryDays:          365,
		SimilarityThreshold:     0.95,
		PredictionConfidence:    0.9,
		ClusteringEpsilon:       0.1,
		MinClusterSize:          20,
		PatternCacheSize:        5000,
		EnableRealTimeDetection: true,
	}
}
