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

package authwebhook

import (
	"fmt"
	"strings"
	"time"
)

// ValidateReason validates that a reason string meets minimum requirements
// Prevents operators from providing meaningless reasons like "ok" or "done"
// Implements BR-WE-013 clearance request validation
//
// Parameters:
//   - reason: Operator-provided reason for clearance/approval
//   - minLength: Minimum required character count
//
// Returns:
//   - error: If reason is invalid
func ValidateReason(reason string, minLength int) error {
	// Check if reason is provided
	if reason == "" {
		return fmt.Errorf("reason is required")
	}

	// Check minimum length
	if len(reason) < minLength {
		return fmt.Errorf("reason must be at least %d characters, got %d", minLength, len(reason))
	}

	// Check for whitespace-only reasons
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("reason cannot be only whitespace")
	}

	return nil
}

// ValidateTimestamp validates that a timestamp is present and not in the future
// Prevents time manipulation attacks and malformed requests
// Implements BR-WE-013 clearance request validation
//
// Parameters:
//   - ts: Timestamp to validate
//
// Returns:
//   - error: If timestamp is invalid
func ValidateTimestamp(ts time.Time) error {
	// Check if timestamp is provided
	if ts.IsZero() {
		return fmt.Errorf("timestamp is required")
	}

	// Check if timestamp is in the future
	if ts.After(time.Now()) {
		return fmt.Errorf("timestamp cannot be in the future")
	}

	return nil
}

