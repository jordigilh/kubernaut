/*
Copyright 2026 Jordi Gil.

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

// Package config provides configuration parsing and validation for the Effectiveness Monitor.
// This package contains unit-testable business logic for config defaults, validation, and merging.
//
// YAML file I/O lives in internal/config/effectivenessmonitor.go (integration-testable).
//
// Business Requirements:
// - BR-EM-006: Configurable stabilization window (default: 5m)
// - BR-EM-007: Configurable validity window (default: 30m)
// - BR-EM-008: Configurable scoring threshold (default: 0.5)
package config

import (
	"fmt"
	"time"
)

// AssessmentConfig defines the business logic configuration for effectiveness assessments.
// These values are unit-testable (pure validation, defaults, merging).
type AssessmentConfig struct {
	// StabilizationWindow is the duration to wait after remediation before assessment.
	// Allows the system to stabilize before checking health/alerts/metrics.
	// Default: 5 minutes. Range: [30s, 1h].
	// Reference: BR-EM-006
	StabilizationWindow time.Duration

	// ValidityWindow is the maximum duration after EA creation for assessment to complete.
	// After this window, remaining uncollected components are marked as timed out.
	// Default: 30 minutes. Range: [5m, 24h].
	// Reference: BR-EM-007
	ValidityWindow time.Duration

	// ScoringThreshold is the minimum score (0.0-1.0) for a "healthy" assessment.
	// Below this threshold, a Warning K8s event is emitted.
	// Default: 0.5. Range: [0.0, 1.0].
	// Reference: BR-EM-008
	ScoringThreshold float64

	// PrometheusEnabled indicates whether metric comparison is enabled.
	// When false, the metrics component is skipped in assessments.
	PrometheusEnabled bool

	// AlertManagerEnabled indicates whether alert resolution checking is enabled.
	// When false, the alert component is skipped in assessments.
	AlertManagerEnabled bool
}

// DefaultAssessmentConfig returns sensible defaults for assessment configuration.
func DefaultAssessmentConfig() AssessmentConfig {
	return AssessmentConfig{
		StabilizationWindow: 5 * time.Minute,
		ValidityWindow:      30 * time.Minute,
		ScoringThreshold:    0.5,
		PrometheusEnabled:   true,
		AlertManagerEnabled: true,
	}
}

// Validate checks the assessment configuration for invalid values.
// Returns a descriptive error if any field is out of range.
func (c *AssessmentConfig) Validate() error {
	// Stabilization window validation
	if c.StabilizationWindow < 30*time.Second {
		return fmt.Errorf("stabilizationWindow must be at least 30s, got %v", c.StabilizationWindow)
	}
	if c.StabilizationWindow > 1*time.Hour {
		return fmt.Errorf("stabilizationWindow must not exceed 1h, got %v", c.StabilizationWindow)
	}

	// Validity window validation
	if c.ValidityWindow < 5*time.Minute {
		return fmt.Errorf("validityWindow must be at least 5m, got %v", c.ValidityWindow)
	}
	if c.ValidityWindow > 24*time.Hour {
		return fmt.Errorf("validityWindow must not exceed 24h, got %v", c.ValidityWindow)
	}

	// Stabilization must be shorter than validity
	if c.StabilizationWindow >= c.ValidityWindow {
		return fmt.Errorf("stabilizationWindow (%v) must be shorter than validityWindow (%v)",
			c.StabilizationWindow, c.ValidityWindow)
	}

	// Scoring threshold validation
	if c.ScoringThreshold < 0.0 || c.ScoringThreshold > 1.0 {
		return fmt.Errorf("scoringThreshold must be between 0.0 and 1.0, got %v", c.ScoringThreshold)
	}

	return nil
}
