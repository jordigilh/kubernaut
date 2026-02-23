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

// Package validity provides validity window logic for the Effectiveness Monitor.
// It determines whether an assessment is still within its validity window
// and whether the stabilization period has expired.
//
// Business Requirements:
// - BR-EM-006: Stabilization window before assessment begins
// - BR-EM-007: Validity window for assessment expiration
//
// Timeline:
//   EA Created                Stabilization Expires          Validity Deadline
//   |--- stabilization ------>|--- assessment window -------->|--- expired --->
//
// The stabilization window prevents premature assessment when the system
// is still settling after remediation. The validity deadline ensures
// assessments don't run indefinitely.
package validity

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WindowState represents the current state of an assessment's validity window.
type WindowState int

const (
	// WindowStabilizing indicates the stabilization period has not yet expired.
	// The assessment should wait (requeue) until stabilization completes.
	WindowStabilizing WindowState = iota

	// WindowActive indicates the stabilization period has passed and the
	// assessment is within the validity window. Assessment checks should proceed.
	WindowActive

	// WindowExpired indicates the validity deadline has passed.
	// The assessment should complete with whatever data has been collected.
	WindowExpired
)

// String returns the human-readable name of the WindowState.
func (ws WindowState) String() string {
	switch ws {
	case WindowStabilizing:
		return "Stabilizing"
	case WindowActive:
		return "Active"
	case WindowExpired:
		return "Expired"
	default:
		return "Unknown"
	}
}

// Checker evaluates the validity window state for an assessment.
type Checker interface {
	// Check determines the current window state given the EA creation time,
	// stabilization window duration, and validity deadline.
	// Accepts metav1.Time (QF1008: omit embedded .Time from selector).
	Check(creationTime metav1.Time, stabilizationWindow time.Duration, validityDeadline metav1.Time) WindowState

	// TimeUntilStabilized returns the remaining time until stabilization expires.
	// Returns 0 if already stabilized.
	// Accepts metav1.Time (QF1008: omit embedded .Time from selector).
	TimeUntilStabilized(creationTime metav1.Time, stabilizationWindow time.Duration) time.Duration

	// TimeUntilExpired returns the remaining time until the validity deadline.
	// Returns 0 if already expired.
	TimeUntilExpired(validityDeadline time.Time) time.Duration
}

// checker is the concrete implementation of Checker.
type checker struct{}

// NewChecker creates a new validity window checker.
func NewChecker() Checker {
	return &checker{}
}

// Check determines the current window state.
//
// Timeline:
//
//	EA Created                Stabilization Expires          Validity Deadline
//	|--- stabilization ------>|--- assessment window -------->|--- expired --->
//
// Expired takes priority: if validity has passed, always return Expired
// even if stabilization hasn't completed (edge case: misconfigured windows).
func (c *checker) Check(creationTime metav1.Time, stabilizationWindow time.Duration, validityDeadline metav1.Time) WindowState {
	now := time.Now()

	// Expired takes priority (use promoted After to avoid QF1008)
	if !validityDeadline.After(now) {
		return WindowExpired
	}

	// Check if stabilization window has passed
	stabilizationEnd := creationTime.Add(stabilizationWindow)
	if now.Before(stabilizationEnd) {
		return WindowStabilizing
	}

	return WindowActive
}

// TimeUntilStabilized returns the remaining time until stabilization expires.
func (c *checker) TimeUntilStabilized(creationTime metav1.Time, stabilizationWindow time.Duration) time.Duration {
	stabilizationEnd := creationTime.Add(stabilizationWindow)
	remaining := time.Until(stabilizationEnd)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// TimeUntilExpired returns the remaining time until the validity deadline.
func (c *checker) TimeUntilExpired(validityDeadline time.Time) time.Duration {
	remaining := time.Until(validityDeadline)
	if remaining < 0 {
		return 0
	}
	return remaining
}
