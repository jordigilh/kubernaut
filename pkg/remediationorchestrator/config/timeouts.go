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

// Package config provides centralized configuration constants for Remediation Orchestrator.
// Reference: REFACTOR-RO-003
package config

import "time"

// ========================================
// TIMEOUT CONSTANTS (REFACTOR-RO-003)
// ========================================
//
// Centralized timeout configuration to avoid magic numbers throughout the codebase.
// All timeouts are configurable via controller flags (see cmd/remediationorchestrator/main.go).
//
// WHY Centralize Timeouts?
// - ✅ Single source of truth - no magic numbers scattered across files
// - ✅ Easy to adjust - change once, affects all usages
// - ✅ Testable - can override for unit tests
// - ✅ Documented - clear rationale for each timeout value
//
// Reference: REFACTOR-RO-003 (timeout constant centralization)

// ========================================
// REQUEUE TIMEOUTS
// ========================================

// RequeueResourceBusy is the delay before retrying after ResourceBusy skip.
// WHY 30 seconds?
// - Short enough to retry quickly after resource becomes available
// - Long enough to avoid excessive reconciliation load
// - Typical workflow duration is 1-2 minutes, so 30s is reasonable
//
// Reference: BR-ORCH-032 (ResourceBusy handling)
const RequeueResourceBusy = 30 * time.Second

// RequeueRecentlyRemediated is the delay before retrying after RecentlyRemediated skip.
// WHY 1 minute?
// - Per WE Team Response Q6: RO should NOT calculate backoff, let WE re-evaluate
// - Fixed interval allows WE to determine if cooldown has expired
// - Avoids complex backoff logic in RO
//
// Reference: BR-ORCH-032 (RecentlyRemediated handling), DD-WE-004 (exponential backoff)
const RequeueRecentlyRemediated = 1 * time.Minute

// RequeueGenericError is the default delay for transient errors.
// WHY 5 seconds?
// - Fast retry for transient errors (network blips, API server hiccups)
// - Short enough to not delay remediation significantly
// - Long enough to avoid hammering the API server
//
// Reference: General error handling pattern
const RequeueGenericError = 5 * time.Second

// ========================================
// FALLBACK TIMEOUTS
// ========================================

// RequeueFallback is the default requeue delay when no specific timeout applies.
// WHY 1 minute?
// - Conservative default for unknown scenarios
// - Balances responsiveness with resource usage
// - Used by CalculateRequeueTime when NextAllowedExecution is nil
//
// Reference: DD-WE-004 (exponential backoff fallback)
const RequeueFallback = 1 * time.Minute

// ========================================
// USAGE EXAMPLES
// ========================================
//
// Example 1: ResourceBusy skip handling
//   return ctrl.Result{RequeueAfter: config.RequeueResourceBusy}, nil
//
// Example 2: Generic error handling
//   return ctrl.Result{RequeueAfter: config.RequeueGenericError}, nil
//
// Example 3: Fallback in CalculateRequeueTime
//   if nextAllowed == nil {
//       return config.RequeueFallback
//   }
