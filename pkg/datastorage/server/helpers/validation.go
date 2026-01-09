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

package helpers

import (
	"fmt"
	"time"

	"github.com/jordigilh/kubernaut/pkg/datastorage/validation"
)

// ========================================
// BUSINESS VALIDATION HELPERS
// ========================================
// Authority: ADR-034 (Unified Audit Table)
// BR-STORAGE-033: Generic audit write API validation
// Gap 1.2: Enhanced malformed event rejection
//
// These functions provide business-specific validation that OpenAPI cannot handle:
// - Timestamp bounds (not in future, not too old)
// - Field length constraints (database/performance limits)
//
// Note: OpenAPI handles: required fields, types, enums
// ========================================

// ValidateTimestampBounds validates that an event timestamp is within acceptable bounds
//
// Rules (Gap 1.2):
// - Not in the future (with 5min clock skew tolerance)
// - Not older than 7 days (data retention policy)
//
// Parameters:
//   - timestamp: Event timestamp to validate
//
// Returns:
//   - *validation.RFC7807Problem: RFC 7807 problem details if validation fails
func ValidateTimestampBounds(timestamp time.Time) *validation.RFC7807Problem {
	now := time.Now().UTC()

	// Check if timestamp is in the future (with 1 hour clock skew tolerance for E2E/Kind environments)
	// NOTE: Kind clusters can have significant clock skew between host and container
	if timestamp.After(now.Add(60 * time.Minute)) {
		return &validation.RFC7807Problem{
			Type:   "https://kubernaut.ai/problems/validation-error",
			Title:  "Validation Error",
			Status: 400,
			Detail: fmt.Sprintf("timestamp is in the future (server time: %s, event time: %s)",
				now.Format(time.RFC3339), timestamp.Format(time.RFC3339)),
		}
	}

	// Check if timestamp is too old (7 days retention policy)
	if timestamp.Before(now.Add(-7 * 24 * time.Hour)) {
		return &validation.RFC7807Problem{
			Type:   "https://kubernaut.ai/problems/validation-error",
			Title:  "Validation Error",
			Status: 400,
			Detail: fmt.Sprintf("timestamp is too old (must be within 7 days, age: %s)",
				now.Sub(timestamp)),
		}
	}

	return nil
}

// ValidateFieldLengths validates that string fields do not exceed maximum length constraints
//
// Rules (Gap 1.2):
// - Prevent excessively long fields that could impact database performance
// - Based on database schema column constraints
//
// Parameters:
//   - fields: Map of field names to values
//   - constraints: Map of field names to maximum lengths
//
// Returns:
//   - *validation.RFC7807Problem: RFC 7807 problem details if validation fails
func ValidateFieldLengths(fields map[string]string, constraints map[string]int) *validation.RFC7807Problem {
	for field, value := range fields {
		if maxLen, ok := constraints[field]; ok && len(value) > maxLen {
			return &validation.RFC7807Problem{
				Type:   "https://kubernaut.ai/problems/validation-error",
				Title:  "Validation Error",
				Status: 400,
				Detail: fmt.Sprintf("%s exceeds maximum length of %d characters (got %d)",
					field, maxLen, len(value)),
			}
		}
	}
	return nil
}

// DefaultFieldLengthConstraints returns the default field length constraints
//
// Based on database schema (ADR-034):
// - event_type: VARCHAR(100)
// - event_category: VARCHAR(50)
// - event_action: VARCHAR(50)
// - correlation_id: VARCHAR(255)
// - actor_type: VARCHAR(50)
// - actor_id: VARCHAR(255)
// - resource_type: VARCHAR(50)
// - resource_id: VARCHAR(255)
func DefaultFieldLengthConstraints() map[string]int {
	return map[string]int{
		"event_type":     100,
		"event_category": 50,
		"event_action":   50,
		"correlation_id": 255,
		"actor_type":     50,
		"actor_id":       255,
		"resource_type":  50,
		"resource_id":    255,
	}
}
