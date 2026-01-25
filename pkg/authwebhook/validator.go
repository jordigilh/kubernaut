package authwebhook

import (
	"fmt"
	"strings"
	"time"
)

const (
	// MaxReasonWords defines maximum word count for audit justifications
	// SOC2 CC7.4: Prevent excessively verbose justifications that reduce audit readability
	// Test Plan Reference: AUTH-007, AUTH-008
	MaxReasonWords = 100
)

// ValidateReason validates clearance/approval reason has sufficient detail
// minWords specifies minimum word count required for audit trail
//
// BR-WEBHOOK-001: Reasons must be sufficiently detailed for SOC2 audit trail
// Returns error if reason is empty, only whitespace, has fewer than minWords, or exceeds MaxReasonWords
//
// Test Plan Reference: AUTH-005 (accept valid), AUTH-006 (reject empty),
// AUTH-007 (reject too long), AUTH-008 (accept at max), AUTH-013-016 (edge cases)
func ValidateReason(reason string, minWords int) error {
	// Validate minimum words parameter
	if minWords <= 0 {
		return fmt.Errorf("minimum words must be positive, got %d", minWords)
	}

	// Trim whitespace
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return fmt.Errorf("reason cannot be empty")
	}

	// Count words (split on whitespace)
	words := strings.Fields(trimmed)
	wordCount := len(words)

	// AUTH-007: Reject overly long justifications (SOC2 CC7.4 audit readability)
	if wordCount > MaxReasonWords {
		return fmt.Errorf("reason exceeds maximum %d words for audit trail, got %d", MaxReasonWords, wordCount)
	}

	// AUTH-005, AUTH-006, AUTH-013-016: Validate minimum word count
	if wordCount < minWords {
		return fmt.Errorf("reason must have minimum %d words required for audit trail, got %d", minWords, wordCount)
	}

	return nil
}

// ValidateTimestamp validates request timestamp is not in future
// and not older than 5 minutes (replay attack prevention)
//
// BR-WEBHOOK-001: Timestamp validation prevents replay attacks and ensures timely actions
// Returns error if timestamp is zero, in future, or older than 5 minutes
func ValidateTimestamp(ts time.Time) error {
	// Check for zero time
	if ts.IsZero() {
		return fmt.Errorf("timestamp cannot be zero")
	}

	now := time.Now()

	// Check if timestamp is in the future
	if ts.After(now) {
		return fmt.Errorf("timestamp cannot be in the future (got %v, now is %v)", ts, now)
	}

	// Check if timestamp is too old (replay attack prevention)
	maxAge := 5 * time.Minute
	age := now.Sub(ts)
	if age > maxAge {
		return fmt.Errorf("timestamp too old (age: %v, maximum: %v) - possible replay attack", age.Round(time.Second), maxAge)
	}

	return nil
}
