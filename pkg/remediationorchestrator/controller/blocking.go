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

// Package controller provides the Kubernetes controller for RemediationRequest CRDs.
//
// This file implements consecutive failure blocking logic per BR-ORCH-042.
// When a signal fingerprint fails ≥3 consecutive times, RO holds the RR in a
// non-terminal Blocked phase for a cooldown period before allowing retry.
//
// Business Requirements:
// - BR-ORCH-042: Consecutive Failure Blocking with Automatic Cooldown
// - BR-GATEWAY-185 v1.1: Field selector on spec.signalFingerprint (not labels)
//
// Design Decision:
// - DD-GATEWAY-011 v1.3: Blocking logic moved from Gateway to RO
//
// TDD Implementation:
// - RED: Tests in test/unit/remediationorchestrator/blocking_test.go
// - GREEN: This file (minimal implementation to pass tests)
// - REFACTOR: After GREEN, optimize as needed
package controller

import (
	"time"
)

// ========================================
// BLOCKING CONFIGURATION CONSTANTS
// BR-ORCH-042.1, BR-ORCH-042.3, BR-GATEWAY-185 v1.1
// ========================================

// DefaultBlockThreshold is the number of consecutive failures before blocking.
// Reference: BR-ORCH-042.1
const DefaultBlockThreshold = 3

// DefaultCooldownDuration is how long to block before allowing retry.
// Reference: BR-ORCH-042.3
const DefaultCooldownDuration = 1 * time.Hour

// FingerprintFieldIndex is the field index key for spec.signalFingerprint.
// Used for O(1) lookups. Set up in SetupWithManager().
// Reference: BR-GATEWAY-185 v1.1
const FingerprintFieldIndex = "spec.signalFingerprint"

// ========================================
// BLOCK REASON CONSTANTS
// ========================================

// BlockReasonConsecutiveFailures indicates blocking due to ≥3 consecutive failures.
const BlockReasonConsecutiveFailures = "consecutive_failures_exceeded"

// ========================================
// TDD GREEN PHASE: Minimal implementation complete
// Next: Run tests to verify they pass
// Then: REFACTOR phase - add actual blocking logic methods
// ========================================

