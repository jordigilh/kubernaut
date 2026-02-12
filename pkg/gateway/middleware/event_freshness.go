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
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"

	gwerrors "github.com/jordigilh/kubernaut/pkg/gateway/errors"
)

// eventTimestamps extracts timestamp fields from a Kubernetes Event body.
// Only the fields needed for freshness validation are unmarshaled.
type eventTimestamps struct {
	LastTimestamp  *time.Time `json:"lastTimestamp,omitempty"`
	FirstTimestamp *time.Time `json:"firstTimestamp,omitempty"`
}

// EventFreshnessValidator creates middleware that validates event freshness from
// the request body's timestamp fields instead of HTTP headers.
//
// Business Requirements:
// - BR-GATEWAY-074: Replay prevention for Kubernetes Events
// - BR-GATEWAY-075: Separation of concerns - each adapter declares its own strategy
//
// This middleware is designed for signal sources (e.g., kubernetes-event-exporter) that
// cannot dynamically set HTTP headers like X-Timestamp. Instead, it extracts the event's
// lastTimestamp (preferred) or firstTimestamp from the JSON body and validates freshness.
//
// Flow:
// 1. Read request body into buffer
// 2. Extract lastTimestamp / firstTimestamp from JSON
// 3. Validate the most recent timestamp is within the tolerance window
// 4. Rewind the body so downstream handlers can re-read it
// 5. Reject stale or future events with HTTP 400 (RFC 7807)
//
// Design Decision:
// - Prefers lastTimestamp over firstTimestamp (recurring events have a more recent lastTimestamp)
// - GET/HEAD/OPTIONS requests are exempt (health/metrics endpoints)
// - Body is always rewound for downstream consumption
func EventFreshnessValidator(tolerance time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip validation for non-write methods (health, metrics endpoints)
			if r.Method == "GET" || r.Method == "HEAD" || r.Method == "OPTIONS" {
				next.ServeHTTP(w, r)
				return
			}

			// Read body into buffer
			bodyBytes, err := io.ReadAll(r.Body)
			if err != nil {
				respondFreshnessError(w, "failed to read request body")
				return
			}
			// Always rewind body for downstream handlers
			r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

			// Extract timestamps from body
			var ts eventTimestamps
			if err := json.Unmarshal(bodyBytes, &ts); err != nil {
				// Can't parse JSON - let downstream handler deal with it
				next.ServeHTTP(w, r)
				return
			}

			// Determine the most relevant timestamp (prefer lastTimestamp)
			var eventTime time.Time
			if ts.LastTimestamp != nil && !ts.LastTimestamp.IsZero() {
				eventTime = *ts.LastTimestamp
			} else if ts.FirstTimestamp != nil && !ts.FirstTimestamp.IsZero() {
				eventTime = *ts.FirstTimestamp
			} else {
				respondFreshnessError(w, "event missing lastTimestamp and firstTimestamp")
				return
			}

			// Validate freshness
			now := time.Now()
			age := now.Sub(eventTime)
			if age > tolerance {
				respondFreshnessError(w, "event timestamp too old: possible stale event")
				return
			}
			if eventTime.After(now.Add(2 * time.Minute)) {
				respondFreshnessError(w, "event timestamp in future: possible clock skew")
				return
			}

			// Event is fresh - continue to next handler
			next.ServeHTTP(w, r)
		})
	}
}

// respondFreshnessError writes an RFC 7807 compliant error response for event freshness failures.
func respondFreshnessError(w http.ResponseWriter, message string) {
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
