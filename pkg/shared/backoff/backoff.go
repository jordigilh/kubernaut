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

// Package backoff provides shared utilities for exponential backoff calculations.
//
// Design Decision: DD-SHARED-001 (to be created)
// Extracted from Notification service's production-proven implementation.
//
// Business Requirements Enabled:
// - BR-WE-012: WorkflowExecution - Pre-execution Failure Backoff
// - BR-NOT-052: Notification - Automatic Retry with Custom Retry Policies
// - BR-NOT-055: Notification - Graceful Degradation (jitter for anti-thundering herd)
//
// Usage Pattern:
//
//	// Flexible multiplier with jitter (Notification pattern)
//	config := backoff.Config{
//	    BasePeriod:    30 * time.Second,
//	    MaxPeriod:     5 * time.Minute,
//	    Multiplier:    2.0,      // 1.5=conservative, 2=standard, 3=aggressive
//	    JitterPercent: 10,       // ±10% variance (anti-thundering herd)
//	}
//	duration := config.Calculate(attempts)
//
// Benefits:
// - Single source of truth for backoff calculation
// - Consistent behavior across all services
// - Prevents arithmetic errors in manual calculations
// - Industry best practice (jitter prevents thundering herd)
// - Flexible strategies (configurable multiplier)
// - Battle-tested (extracted from Notification v3.1)
package backoff

import (
	"math/rand"
	"time"
)

// Config defines the exponential backoff parameters.
//
// This configuration supports flexible backoff strategies with optional jitter
// to prevent thundering herd problems in distributed systems.
type Config struct {
	// BasePeriod is the initial backoff duration (e.g., 30s)
	// Formula: duration = BasePeriod * (Multiplier ^ (attempts-1))
	BasePeriod time.Duration

	// MaxPeriod caps the exponential backoff (e.g., 5m)
	// If zero, no cap is applied (not recommended)
	MaxPeriod time.Duration

	// Multiplier for exponential growth (default: 2.0)
	// - 1.5: Conservative (slower growth)
	// - 2.0: Standard exponential (power-of-2)
	// - 3.0: Aggressive (faster growth)
	// Range: 1.0-10.0
	// If zero, defaults to 2.0
	Multiplier float64

	// JitterPercent adds random variance to prevent thundering herd (default: 0)
	// - 0: No jitter (deterministic)
	// - 10: ±10% variance (recommended for production)
	// - 20: ±20% variance (aggressive anti-thundering herd)
	// Range: 0-50
	// Jitter distributes retry attempts over time, reducing API load spikes
	JitterPercent int
}

// Calculate computes the exponential backoff duration for a given number of attempts.
//
// Formula: duration = BasePeriod * (Multiplier ^ (attempts-1))
// With jitter: duration ± (duration * JitterPercent / 100)
// Capped by: MaxPeriod
//
// Examples:
//
//	Standard (Multiplier=2, JitterPercent=0):
//	- attempts=1 → 30s (30 * 2^0)
//	- attempts=2 → 1m (30 * 2^1)
//	- attempts=3 → 2m (30 * 2^2)
//	- attempts=4 → 4m (30 * 2^3)
//	- attempts=5 → 5m (capped at MaxPeriod)
//
//	Conservative (Multiplier=1.5, JitterPercent=10):
//	- attempts=1 → 27-33s (30s ±10%)
//	- attempts=2 → 40-50s (45s ±10%)
//	- attempts=3 → 60-74s (67s ±10%)
//
//	Aggressive (Multiplier=3, JitterPercent=20):
//	- attempts=1 → 24-36s (30s ±20%)
//	- attempts=2 → 72-108s (90s ±20%)
//	- attempts=3 → 216-324s (270s ±20%)
//
// Parameters:
//   - attempts: The number of attempts made (1-based: first attempt = 1)
//
// Returns:
//   - time.Duration: The calculated backoff duration with optional jitter
//   - If attempts < 1, returns BasePeriod
//   - If BasePeriod is 0, returns 0
//
// Implementation Notes:
//   - Extracted from Notification service (lines 302-346 of notificationrequest_controller.go)
//   - Jitter prevents thundering herd when many operations fail simultaneously
//   - Multiplier enables flexible strategies (conservative/standard/aggressive)
//   - Bounds checking ensures jitter never violates [BasePeriod, MaxPeriod] range
func (c Config) Calculate(attempts int32) time.Duration {
	// Set defaults
	if c.Multiplier == 0 {
		c.Multiplier = 2.0 // Standard exponential (backward compatible with WE)
	}

	// Handle edge cases
	if c.BasePeriod == 0 {
		return 0
	}
	if attempts < 1 {
		return c.BasePeriod
	}

	// Calculate exponential backoff using configurable multiplier
	// Formula: BasePeriod * (Multiplier ^ (attempts-1))
	//
	// NT's implementation (proven in production):
	// Calculate attempts-1 iterations (first attempt = multiplier^0 = 1x base)
	backoff := c.BasePeriod
	for i := int32(0); i < attempts-1; i++ {
		backoff = time.Duration(float64(backoff) * c.Multiplier)

		// Cap during iteration to prevent overflow with high multipliers
		// This is critical when Multiplier > 2 (e.g., 10x can overflow quickly)
		if c.MaxPeriod > 0 && backoff > c.MaxPeriod {
			backoff = c.MaxPeriod
			break
		}
	}

	// Final cap at MaxPeriod (defense in depth)
	if c.MaxPeriod > 0 && backoff > c.MaxPeriod {
		backoff = c.MaxPeriod
	}

	// Add jitter if configured (NT v3.1 enhancement)
	// Jitter distributes retry attempts over time, preventing thundering herd
	//
	// Example: 100 notifications fail at same time
	// WITHOUT jitter: All 100 retry at EXACTLY 30s → API overload
	// WITH ±10% jitter: 100 retry spread over 27-33s → manageable load
	if c.JitterPercent > 0 {
		jitterRange := backoff * time.Duration(c.JitterPercent) / 100
		if jitterRange > 0 {
			// Random jitter between -JitterPercent% and +JitterPercent%
			jitter := time.Duration(rand.Int63n(int64(jitterRange)*2)) - jitterRange
			backoff += jitter

			// Ensure backoff remains within bounds after jitter
			// Never go below BasePeriod (minimum backoff)
			if backoff < c.BasePeriod {
				backoff = c.BasePeriod
			}
			// Never exceed MaxPeriod (maximum backoff)
			if c.MaxPeriod > 0 && backoff > c.MaxPeriod {
				backoff = c.MaxPeriod
			}
		}
	}

	return backoff
}

// CalculateWithDefaults is a convenience function that uses sensible defaults:
//   - BasePeriod: 30s
//   - MaxPeriod: 5m
//   - Multiplier: 2.0 (standard exponential)
//   - JitterPercent: 10 (±10% variance, recommended for production)
//
// This provides a production-ready backoff strategy with anti-thundering herd protection:
//   - Attempt 1: ~30s (27-33s with jitter)
//   - Attempt 2: ~1m (54-66s with jitter)
//   - Attempt 3: ~2m (108-132s with jitter)
//   - Attempt 4: ~4m (216-264s with jitter)
//   - Attempt 5+: ~5m (270-330s with jitter, capped at 5m)
//
// Use this for: Standard retry scenarios with external API calls or transient failures
func CalculateWithDefaults(attempts int32) time.Duration {
	config := Config{
		BasePeriod:    30 * time.Second,
		MaxPeriod:     5 * time.Minute,
		Multiplier:    2.0,
		JitterPercent: 10, // Recommended for production (NT v3.1 enhancement)
	}
	return config.Calculate(attempts)
}

// CalculateWithoutJitter is a convenience function for deterministic backoff:
//   - BasePeriod: 30s
//   - MaxPeriod: 5m
//   - Multiplier: 2.0 (standard exponential)
//   - JitterPercent: 0 (no variance)
//
// This provides deterministic backoff without jitter (useful for testing):
//   - Attempt 1: 30s (exact)
//   - Attempt 2: 1m (exact)
//   - Attempt 3: 2m (exact)
//   - Attempt 4: 4m (exact)
//   - Attempt 5+: 5m (exact, capped)
//
// Use this for: Testing, single-instance deployments, or when deterministic timing is required
//
// Note: This maintains backward compatibility with WE's original implementation
func CalculateWithoutJitter(attempts int32) time.Duration {
	config := Config{
		BasePeriod:    30 * time.Second,
		MaxPeriod:     5 * time.Minute,
		Multiplier:    2.0,
		JitterPercent: 0, // No jitter (deterministic)
	}
	return config.Calculate(attempts)
}
