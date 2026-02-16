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
)

// alertManagerTimestamps is a minimal struct to extract timestamps from an
// AlertManager webhook payload without unmarshalling the full body.
type alertManagerTimestamps struct {
	Alerts []struct {
		StartsAt time.Time `json:"startsAt"`
	} `json:"alerts"`
}

// AlertManagerFreshnessValidator creates middleware that validates freshness for
// Prometheus AlertManager webhook requests.
//
// Business Requirements:
//   - BR-GATEWAY-074: Replay prevention for Prometheus AlertManager signals
//   - BR-GATEWAY-075: Adapter-specific replay prevention strategy
//
// Strategy: Header-first with body-fallback
//
//  1. If X-Timestamp header is present → validate header (strict TimestampValidator).
//     This path supports direct API clients (tests, integrations) that CAN set
//     custom HTTP headers.
//  2. If X-Timestamp header is absent → validate startsAt from the webhook body.
//     AlertManager's standard webhook format does NOT support custom HTTP headers,
//     so we extract freshness from the alert payload itself.
//
// Body-based validation:
//   - Extracts the most recent alerts[].startsAt from the AlertManager payload.
//   - Rejects payloads where startsAt is far in the future (clock skew attack).
//   - Does NOT enforce "too old" on body timestamps because alerts can be
//     long-running (e.g., an alert firing for hours still produces legitimate
//     re-notification webhooks with the original startsAt).
//   - Gateway deduplication (fingerprint-based) is the primary defense against
//     duplicate processing for the AlertManager path.
//
// Design Decision (Feb 2026):
//   - AlertManager cannot inject dynamic custom headers into webhook requests.
//   - Header-first preserves strict validation for direct API callers.
//   - Body-fallback enables real AlertManager deployments to reach the Gateway.
func AlertManagerFreshnessValidator(tolerance time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip validation for non-write methods (health, metrics endpoints)
			if r.Method == "GET" || r.Method == "HEAD" || r.Method == "OPTIONS" {
				next.ServeHTTP(w, r)
				return
			}

			// Skip validation for health and metrics endpoints
			if r.URL.Path == "/health" || r.URL.Path == "/ready" || r.URL.Path == "/healthz" || r.URL.Path == "/metrics" {
				next.ServeHTTP(w, r)
				return
			}

			// --- Strategy 1: Header-based (strict) ---
			// If X-Timestamp header is present, use strict TimestampValidator logic.
			// This path is used by direct API clients (E2E tests, external integrations).
			if r.Header.Get(timestampHeader) != "" {
				timestamp, err := extractTimestamp(r)
				if err != nil {
					respondTimestampError(w, err.Error())
					return
				}
				if err := validateTimestampWindow(timestamp, tolerance); err != nil {
					respondTimestampError(w, err.Error())
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			// --- Strategy 2: Body-based (AlertManager compat) ---
			// AlertManager does not set custom HTTP headers. Extract freshness
			// from alerts[].startsAt in the webhook body.
			bodyBytes, err := io.ReadAll(r.Body)
			if err != nil {
				respondFreshnessError(w, "failed to read request body")
				return
			}
			// Always rewind body for downstream handlers
			r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

			var ts alertManagerTimestamps
			if err := json.Unmarshal(bodyBytes, &ts); err != nil {
				// Can't parse JSON – let downstream handler deal with it
				next.ServeHTTP(w, r)
				return
			}

			// Find the most recent startsAt across all alerts
			var mostRecent time.Time
			for _, alert := range ts.Alerts {
				if !alert.StartsAt.IsZero() && alert.StartsAt.After(mostRecent) {
					mostRecent = alert.StartsAt
				}
			}

			if mostRecent.IsZero() {
				respondFreshnessError(w, "alert payload missing startsAt timestamp")
				return
			}

			// Reject far-future timestamps (clock skew attack)
			// Allow small clock skew tolerance (2 minutes) for legitimate time differences.
			if mostRecent.After(time.Now().Add(clockSkewTolerance)) {
				respondFreshnessError(w, "alert startsAt in future: possible clock skew attack")
				return
			}

			// NOTE: We intentionally do NOT reject old startsAt values here.
			// AlertManager re-notifies long-running alerts with the original startsAt,
			// so enforcing a strict age window would reject legitimate webhooks.
			// Gateway deduplication (fingerprint-based) prevents duplicate CRD creation.

			next.ServeHTTP(w, r)
		})
	}
}
