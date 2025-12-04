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

// Package classifier provides environment and priority classification for signals.
// Design Decision: DD-CATEGORIZATION-001 - Signal Processing owns ALL categorization
// Design Decision: ADR-041 - K8s Enricher fetches data, Classifiers evaluate
// Business Requirements: BR-SP-070 (Environment), BR-SP-071 (Priority)
package classifier

import (
	"context"
	"strings"

	"github.com/go-logr/logr"
)

// ========================================
// ENVIRONMENT CLASSIFIER (BR-SP-070)
// ========================================

// EnvironmentClassification holds the classification result.
type EnvironmentClassification struct {
	// Environment name (e.g., "production", "staging", "development")
	Environment string
	// Confidence score (0.0 - 1.0)
	Confidence float64
	// Source of classification (e.g., "namespace-label", "signal-label", "default")
	Source string
}

// EnvironmentClassifier classifies signal environment.
// Uses pre-fetched data from K8s Enricher (ADR-041: no K8s calls in classifier).
type EnvironmentClassifier struct {
	logger logr.Logger
}

// NewEnvironmentClassifier creates a new environment classifier.
func NewEnvironmentClassifier(logger logr.Logger) *EnvironmentClassifier {
	return &EnvironmentClassifier{
		logger: logger.WithName("environment-classifier"),
	}
}

// Classify determines environment from namespace and signal labels.
// Priority order: namespace labels → signal labels → default ("unknown")
// This follows ADR-041: data is pre-fetched by K8s Enricher.
func (c *EnvironmentClassifier) Classify(ctx context.Context, namespaceLabels, signalLabels map[string]string) *EnvironmentClassification {
	// 1. Check namespace labels (highest confidence - explicit configuration)
	if env := c.extractEnvironment(namespaceLabels); env != "" {
		c.logger.V(1).Info("Environment classified from namespace label",
			"environment", env,
			"source", "namespace-label")
		return &EnvironmentClassification{
			Environment: env,
			Confidence:  0.95,
			Source:      "namespace-label",
		}
	}

	// 2. Check signal labels (medium confidence - inferred from alert)
	if env := c.extractEnvironment(signalLabels); env != "" {
		c.logger.V(1).Info("Environment classified from signal label",
			"environment", env,
			"source", "signal-label")
		return &EnvironmentClassification{
			Environment: env,
			Confidence:  0.80,
			Source:      "signal-label",
		}
	}

	// 3. Default fallback (no confidence)
	c.logger.Info("No environment label found, using default",
		"source", "default")
	return &EnvironmentClassification{
		Environment: "unknown",
		Confidence:  0.0,
		Source:      "default",
	}
}

// extractEnvironment looks for environment-related keys in labels.
func (c *EnvironmentClassifier) extractEnvironment(labels map[string]string) string {
	// Check common environment label keys
	envKeys := []string{"environment", "env", "deployment-environment", "app.kubernetes.io/environment"}
	for _, key := range envKeys {
		if env, ok := labels[key]; ok && env != "" {
			return env
		}
	}
	return ""
}

// ========================================
// PRIORITY CLASSIFIER (BR-SP-071)
// ========================================

// PriorityClassification holds the priority classification result.
type PriorityClassification struct {
	// Priority level (e.g., "P0", "P1", "P2", "P3")
	Priority string
	// Confidence score (0.0 - 1.0)
	Confidence float64
	// Reason for the classification
	Reason string
}

// PriorityClassifier assigns priority based on severity and environment.
// Uses matrix-based classification per DD-CATEGORIZATION-001.
type PriorityClassifier struct {
	logger logr.Logger
}

// NewPriorityClassifier creates a new priority classifier.
func NewPriorityClassifier(logger logr.Logger) *PriorityClassifier {
	return &PriorityClassifier{
		logger: logger.WithName("priority-classifier"),
	}
}

// Classify assigns priority based on severity and environment.
// Priority Matrix:
//
//	| Severity | Prod | Staging | Dev | Unknown |
//	|----------|------|---------|-----|---------|
//	| critical | P0   | P1      | P2  | P1      |
//	| warning  | P1   | P2      | P3  | P2      |
//	| info     | P3   | P3      | P3  | P3      |
func (c *PriorityClassifier) Classify(ctx context.Context, severity, environment string) *PriorityClassification {
	// Normalize inputs
	severity = strings.ToLower(severity)
	environment = strings.ToLower(environment)

	// Determine if environment is production-like
	isProd := c.isProductionEnvironment(environment)
	isStaging := c.isStagingEnvironment(environment)
	isDev := c.isDevelopmentEnvironment(environment)

	var priority string
	var confidence float64
	var reason string

	switch severity {
	case "critical":
		if isProd {
			priority = "P0"
			confidence = 0.95
			reason = "critical severity in production"
		} else if isStaging {
			priority = "P1"
			confidence = 0.90
			reason = "critical severity in staging"
		} else if isDev {
			priority = "P2"
			confidence = 0.85
			reason = "critical severity in development"
		} else {
			priority = "P1"
			confidence = 0.80
			reason = "critical severity in unknown environment"
		}

	case "warning":
		if isProd {
			priority = "P1"
			confidence = 0.90
			reason = "warning severity in production"
		} else if isStaging {
			priority = "P2"
			confidence = 0.85
			reason = "warning severity in staging"
		} else if isDev {
			priority = "P3"
			confidence = 0.80
			reason = "warning severity in development"
		} else {
			priority = "P2"
			confidence = 0.75
			reason = "warning severity in unknown environment"
		}

	case "info":
		priority = "P3"
		confidence = 0.90
		reason = "info severity (always low priority)"

	default:
		// Unknown severity defaults to P2
		priority = "P2"
		confidence = 0.50
		reason = "unknown severity"
	}

	c.logger.V(1).Info("Priority classified",
		"severity", severity,
		"environment", environment,
		"priority", priority,
		"reason", reason)

	return &PriorityClassification{
		Priority:   priority,
		Confidence: confidence,
		Reason:     reason,
	}
}

// isProductionEnvironment checks if environment is production-like.
func (c *PriorityClassifier) isProductionEnvironment(env string) bool {
	prodEnvs := []string{"production", "prod", "prd", "live"}
	for _, p := range prodEnvs {
		if env == p {
			return true
		}
	}
	return false
}

// isStagingEnvironment checks if environment is staging-like.
func (c *PriorityClassifier) isStagingEnvironment(env string) bool {
	stagingEnvs := []string{"staging", "stage", "stg", "pre-prod", "preprod", "uat"}
	for _, s := range stagingEnvs {
		if env == s {
			return true
		}
	}
	return false
}

// isDevelopmentEnvironment checks if environment is development-like.
func (c *PriorityClassifier) isDevelopmentEnvironment(env string) bool {
	devEnvs := []string{"development", "dev", "local", "test", "testing", "qa"}
	for _, d := range devEnvs {
		if env == d {
			return true
		}
	}
	return false
}

