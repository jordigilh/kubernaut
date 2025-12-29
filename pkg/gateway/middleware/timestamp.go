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

package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	gwerrors "github.com/jordigilh/kubernaut/pkg/gateway/errors"
)

// Constants for timestamp validation
const (
	timestampHeader    = "X-Timestamp"
	clockSkewTolerance = 2 * time.Minute // Allow 2 minutes of clock skew for future timestamps
)

// Timestamp validation error messages
const (
	errMissingTimestamp  = "missing timestamp header"
	errInvalidFormat     = "invalid timestamp format"
	errNegativeTimestamp = "invalid timestamp: negative value"
	errTimestampTooOld   = "timestamp too old: possible replay attack"
	errTimestampFuture   = "timestamp in future: possible clock skew attack"
)

// TimestampValidator creates middleware that validates webhook timestamps to prevent replay attacks.
//
// Business Requirements:
// - BR-GATEWAY-074: Webhook timestamp validation (5min window)
// - BR-GATEWAY-075: Replay attack prevention
//
// Security (Pre-Release - Mandatory Validation):
// - **MANDATORY**: X-Timestamp header is required (no backward compatibility needed)
// - Prevents replay attacks by rejecting old timestamps (> tolerance)
// - Prevents clock skew attacks by rejecting far-future timestamps
// - Allows small clock skew tolerance (2 minutes) for legitimate time differences
//
// Timestamp Validation Flow:
// 1. Extract timestamp from X-Timestamp header (REQUIRED)
// 2. Parse timestamp as Unix epoch (seconds)
// 3. Check if timestamp is within tolerance window
// 4. Reject if too old (> tolerance) or too far in future (> clockSkewTolerance)
//
// Error Handling:
// - 400 Bad Request: Missing timestamp, invalid format, timestamp out of range
//
// Design Decision (Dec 24, 2025):
// - Pre-release product: No backward compatibility requirement
// - Mandatory timestamp improves security posture from day 1
// - All webhook sources must include X-Timestamp header
func TimestampValidator(tolerance time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip validation for GET requests (health/metrics endpoints)
			// BR-GATEWAY-074: Timestamp validation only applies to write operations
			if r.Method == "GET" || r.Method == "HEAD" || r.Method == "OPTIONS" {
				next.ServeHTTP(w, r)
				return
			}

			// Skip validation for health and metrics endpoints
			// These endpoints should be accessible for monitoring without timestamps
			if r.URL.Path == "/health" || r.URL.Path == "/ready" || r.URL.Path == "/healthz" || r.URL.Path == "/metrics" {
				next.ServeHTTP(w, r)
				return
			}

			// SECURITY: Timestamp header is MANDATORY for write operations (POST/PUT/PATCH)
			// BR-GATEWAY-074/075: Pre-release product, no backward compatibility needed
			// Decision: Mandatory timestamp validation for write operations (Dec 24, 2025)
			timestamp, err := extractTimestamp(r)
			if err != nil {
				respondTimestampError(w, err.Error())
				return
			}

			// Validate timestamp is within acceptable time window
			if err := validateTimestampWindow(timestamp, tolerance); err != nil {
				respondTimestampError(w, err.Error())
				return
			}

			// Timestamp is valid, continue to next handler
			next.ServeHTTP(w, r)
		})
	}
}

// extractTimestamp extracts and parses the timestamp from the request header
func extractTimestamp(r *http.Request) (time.Time, error) {
	// Extract timestamp from header
	timestampStr := r.Header.Get(timestampHeader)
	if timestampStr == "" {
		return time.Time{}, errors.New(errMissingTimestamp)
	}

	// Parse timestamp as Unix epoch (seconds)
	timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return time.Time{}, errors.New(errInvalidFormat)
	}

	// Reject negative timestamps
	if timestamp < 0 {
		return time.Time{}, errors.New(errNegativeTimestamp)
	}

	// Convert to time.Time
	return time.Unix(timestamp, 0), nil
}

// validateTimestampWindow validates the timestamp is within acceptable time window
func validateTimestampWindow(requestTime time.Time, tolerance time.Duration) error {
	now := time.Now()

	// Check if timestamp is too old (replay attack)
	age := now.Sub(requestTime)
	if age > tolerance {
		return errors.New(errTimestampTooOld)
	}

	// Check if timestamp is in the future (clock skew attack)
	// Allow small clock skew tolerance for legitimate time differences
	if requestTime.After(now.Add(clockSkewTolerance)) {
		return errors.New(errTimestampFuture)
	}

	return nil
}

// respondTimestampError writes an RFC 7807 compliant error response for timestamp validation failures
// BR-GATEWAY-101: RFC 7807 error format
func respondTimestampError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusBadRequest)

	errorResponse := gwerrors.RFC7807Error{
		Type:   gwerrors.ErrorTypeValidationError,
		Title:  gwerrors.TitleBadRequest,
		Detail: message,
		Status: http.StatusBadRequest,
	}
	_ = json.NewEncoder(w).Encode(errorResponse)
}
