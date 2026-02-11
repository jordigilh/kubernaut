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

// Package handlers implements phase handlers for the AIAnalysis controller.
//
// P2.2 Refactoring: Consolidated constants from investigating.go for better organization.
package handlers

import "time"

// ========================================
// RETRY CONFIGURATION
// BR-AI-009: Transient error retry with exponential backoff
// BR-AI-010: Permanent error immediate failure
// ========================================

const (
	// MaxRetries for transient errors before marking as Failed
	// BR-AI-009: Maximum retry attempts for transient HAPI errors
	// After 5 attempts with exponential backoff, transition to permanent failure
	MaxRetries = 5

	// BaseDelay for exponential backoff (first retry)
	// Used with pkg/shared/backoff for retry delay calculation
	// Note: pkg/shared/backoff default is also 30s for consistency
	BaseDelay = 30 * time.Second

	// MaxDelay caps the backoff delay (maximum wait between retries)
	// Ensures retries don't become too slow for time-sensitive analysis
	// Note: Set to 8 minutes vs. backoff default 5 minutes for HAPI analysis tolerance
	MaxDelay = 480 * time.Second // 8 minutes
)

// ========================================
// KUBERNETES ANNOTATIONS
// Used for storing handler state in CRD annotations
// ========================================

const (
	// RetryCountAnnotation stores retry count in CRD annotations
	// Format: "kubernaut.ai/retry-count"
	// Used to persist retry state across reconciliation loops
	RetryCountAnnotation = "kubernaut.ai/retry-count"
)

// ========================================
// SESSION CONFIGURATION (BR-AA-HAPI-064)
// Async submit/poll session management
// ========================================

const (
	// MaxSessionRegenerations is the maximum number of session regenerations
	// before the investigation fails with SessionRegenerationExceeded.
	// BR-AA-HAPI-064.6: Cap at 5 regenerations
	MaxSessionRegenerations int32 = 5

	// DefaultPollInterval is the initial polling interval for session status.
	// BR-AA-HAPI-064.8: First poll at 10s
	DefaultPollInterval = 10 * time.Second

	// PollBackoffMultiplier is the multiplier for polling backoff.
	// BR-AA-HAPI-064.8: Double interval on each poll
	PollBackoffMultiplier = 2

	// MaxPollInterval caps the polling interval.
	// BR-AA-HAPI-064.8: Cap at 30s
	MaxPollInterval = 30 * time.Second
)














