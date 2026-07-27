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
	"errors"
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
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}
			validateEventBodyFreshness(w, r, next, tolerance)
		})
	}
}

// validateEventBodyFreshness reads the Kubernetes Event body, extracts its
// freshness timestamp, and validates it against tolerance. Extracted from
// EventFreshnessValidator to keep the outer closure's cognitive complexity low.
func validateEventBodyFreshness(w http.ResponseWriter, r *http.Request, next http.Handler, tolerance time.Duration) {
	// Issue #673 C-ADV-1: Cap body read to prevent unbounded memory allocation.
	bodyReader := http.MaxBytesReader(nil, r.Body, MaxRequestBodySize)
	bodyBytes, err := io.ReadAll(bodyReader)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			respondPayloadTooLarge(w)
			return
		}
		respondFreshnessError(w, "failed to read request body")
		return
	}
	// Always rewind body for downstream handlers
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	var ts eventTimestamps
	if err := json.Unmarshal(bodyBytes, &ts); err != nil {
		// Can't parse JSON - let downstream handler deal with it
		next.ServeHTTP(w, r)
		return
	}

	eventTime, ok := mostRelevantEventTimestamp(ts)
	if !ok {
		respondFreshnessError(w, "event missing lastTimestamp and firstTimestamp")
		return
	}

	if err := validateEventFreshnessWindow(eventTime, tolerance); err != nil {
		respondFreshnessError(w, err.Error())
		return
	}

	next.ServeHTTP(w, r)
}

// mostRelevantEventTimestamp determines the most relevant timestamp for
// freshness validation, preferring lastTimestamp (recurring events have a
// more recent lastTimestamp) over firstTimestamp. ok=false when neither is set.
func mostRelevantEventTimestamp(ts eventTimestamps) (time.Time, bool) {
	if ts.LastTimestamp != nil && !ts.LastTimestamp.IsZero() {
		return *ts.LastTimestamp, true
	}
	if ts.FirstTimestamp != nil && !ts.FirstTimestamp.IsZero() {
		return *ts.FirstTimestamp, true
	}
	return time.Time{}, false
}

// validateEventFreshnessWindow rejects events older than tolerance (stale
// replay) or with a future timestamp beyond the clock-skew allowance.
func validateEventFreshnessWindow(eventTime time.Time, tolerance time.Duration) error {
	now := time.Now()
	if age := now.Sub(eventTime); age > tolerance {
		return errors.New("event timestamp too old: possible stale event")
	}
	if eventTime.After(now.Add(2 * time.Minute)) {
		return errors.New("event timestamp in future: possible clock skew")
	}
	return nil
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
