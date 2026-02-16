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

// Package config provides centralized configuration constants for Effectiveness Monitor.
// Reference: REFACTOR-EM-001
package config

import "time"

// ========================================
// TIMEOUT CONSTANTS (REFACTOR-EM-001)
// ========================================

// RequeueStabilizationPending is the delay before retrying when waiting for
// the stabilization window to expire.
// WHY 30 seconds?
// - Shorter than typical stabilization window (5m default)
// - Avoids excessive API server load
// - Frequent enough to react quickly once window expires
const RequeueStabilizationPending = 30 * time.Second

// RequeueAssessmentInProgress is the delay before retrying when assessment
// is in progress but waiting for external data (e.g., Prometheus metrics).
// WHY 15 seconds?
// - Prometheus scrape interval is typically 15-30 seconds
// - Allows enough time for new metric data to arrive
// - Keeps assessment responsive
const RequeueAssessmentInProgress = 15 * time.Second

// RequeueGenericError is the default delay for transient errors.
// WHY 5 seconds?
// - Fast retry for transient errors (network blips, API server hiccups)
// - Matches RO pattern for consistency
const RequeueGenericError = 5 * time.Second

// RequeueExternalServiceDown is the delay when an external service (Prometheus, AlertManager)
// is unavailable but the assessment should continue with available components.
// WHY 30 seconds?
// - Gives external service time to recover
// - Doesn't block assessment of other components
const RequeueExternalServiceDown = 30 * time.Second
