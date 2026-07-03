/*
Copyright 2026 Jordi Gil.

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

package middleware_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/middleware"
)

// ========================================
// Wave 6 RED phase characterization tests
// ========================================
//
// Discovery: AlertManagerFreshnessValidator and EventFreshnessValidator had NO
// dedicated test file at all (0% UT+IT coverage on their orchestration logic).
// The underlying timestamp-comparison primitives are exercised indirectly via
// TimestampValidator's own tests (timestamp_security_test.go), but the
// adapter-specific orchestration (header-vs-body strategy selection, body size
// enforcement, JSON-parse fallback, multi-alert "most recent" selection) was
// never exercised by any test.
//
// BR-GATEWAY-074: Replay prevention (reject stale/replayed signals)
// BR-GATEWAY-075: Adapter-specific replay prevention strategy (each source
// declares how it proves freshness: header vs. body timestamp)
var _ = Describe("BR-GATEWAY-074, BR-GATEWAY-075: Freshness middleware orchestration", func() {
	var passthroughHandler http.Handler

	BeforeEach(func() {
		passthroughHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	})

	Describe("AlertManagerFreshnessValidator", func() {
		var handler http.Handler

		BeforeEach(func() {
			handler = middleware.AlertManagerFreshnessValidator(5 * time.Minute)(passthroughHandler)
		})

		DescribeTable("non-write methods bypass freshness validation entirely",
			func(method string) {
				req := httptest.NewRequest(method, "/api/v1/signals/prometheus", nil)
				rr := httptest.NewRecorder()
				handler.ServeHTTP(rr, req)
				Expect(rr.Code).To(Equal(http.StatusOK),
					"BR-GATEWAY-074: read-only requests must not be blocked by freshness checks")
			},
			Entry("GET", "GET"),
			Entry("HEAD", "HEAD"),
			Entry("OPTIONS", "OPTIONS"),
		)

		DescribeTable("health/metrics endpoints bypass freshness validation regardless of method",
			func(path string) {
				req := httptest.NewRequest("POST", path, strings.NewReader("not even json"))
				rr := httptest.NewRecorder()
				handler.ServeHTTP(rr, req)
				Expect(rr.Code).To(Equal(http.StatusOK),
					"BR-GATEWAY-074: operational endpoints must remain reachable even with malformed bodies")
			},
			Entry("/health", "/health"),
			Entry("/ready", "/ready"),
			Entry("/healthz", "/healthz"),
			Entry("/metrics", "/metrics"),
		)

		It("uses Strategy 1 (header-based, strict) when X-Timestamp is present, rejecting an expired timestamp", func() {
			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", strings.NewReader("{}"))
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Add(-1*time.Hour).Unix()))

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest),
				"BR-GATEWAY-074: an X-Timestamp header older than the tolerance window must be rejected (replay prevention)")
		})

		It("uses Strategy 1 (header-based, strict) and rejects a malformed (non-numeric) X-Timestamp header", func() {
			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", strings.NewReader("{}"))
			req.Header.Set("X-Timestamp", "not-a-unix-timestamp")

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest),
				"BR-GATEWAY-074: a malformed X-Timestamp header must be rejected as a parse failure, not silently ignored")
		})

		It("uses Strategy 1 (header-based) and lets a fresh, valid X-Timestamp through untouched", func() {
			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", strings.NewReader(`{"alerts":[]}`))
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK),
				"BR-GATEWAY-074: a fresh X-Timestamp header must be accepted (no false-positive rejection)")
		})

		It("falls back to Strategy 2 (body-based) when X-Timestamp is absent, extracting startsAt from the AlertManager payload", func() {
			body := fmt.Sprintf(`{"alerts":[{"startsAt":%q}]}`, time.Now().Format(time.RFC3339))
			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", strings.NewReader(body))

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK),
				"BR-GATEWAY-075: AlertManager cannot set custom headers, so a fresh body startsAt must be accepted")
		})

		It("rejects a body-based payload whose most-recent startsAt is a future timestamp (clock skew attack)", func() {
			body := fmt.Sprintf(`{"alerts":[{"startsAt":%q}]}`, time.Now().Add(10*time.Minute).Format(time.RFC3339))
			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", strings.NewReader(body))

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest),
				"BR-GATEWAY-074: a far-future startsAt must be rejected as a possible clock-skew attack")
		})

		It("rejects a body-based payload missing startsAt on every alert", func() {
			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", strings.NewReader(`{"alerts":[{}]}`))

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest),
				"BR-GATEWAY-075: without any timestamp evidence, freshness cannot be proven and must be rejected")
		})

		It("selects the MOST RECENT startsAt across multiple alerts, accepting a fresh one even if an older one is also present", func() {
			body := fmt.Sprintf(`{"alerts":[{"startsAt":%q},{"startsAt":%q}]}`,
				time.Now().Add(-2*time.Hour).Format(time.RFC3339), // stale, legitimately re-notified alert
				time.Now().Format(time.RFC3339),                   // fresh
			)
			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", strings.NewReader(body))

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK),
				"Design Decision: AlertManager re-notifies long-running alerts with the original startsAt; "+
					"the validator must not reject the whole batch just because one alert is old")
		})

		It("passes through to the downstream handler when the body is not valid JSON (lets adapter-level parsing report the error)", func() {
			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", strings.NewReader("not json at all"))

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK),
				"malformed JSON is a parsing concern for the downstream adapter, not a freshness violation")
		})

		It("rejects an oversized body with 413 instead of buffering it into memory", func() {
			oversized := strings.NewReader(strings.Repeat("a", 11*1024*1024)) // > MaxRequestBodySize
			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", oversized)

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusRequestEntityTooLarge),
				"Issue #673 C-ADV-1: oversized bodies must be rejected before unbounded memory allocation")
		})

		It("rewinds the body so the downstream handler can still read it after freshness validation", func() {
			var bodyContentSeenDownstream string
			recordingHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				bodyContentSeenDownstream = string(b)
				w.WriteHeader(http.StatusOK)
			})
			wrapped := middleware.AlertManagerFreshnessValidator(5 * time.Minute)(recordingHandler)

			body := fmt.Sprintf(`{"alerts":[{"startsAt":%q}]}`, time.Now().Format(time.RFC3339))
			req := httptest.NewRequest("POST", "/api/v1/signals/prometheus", strings.NewReader(body))

			rr := httptest.NewRecorder()
			wrapped.ServeHTTP(rr, req)

			Expect(bodyContentSeenDownstream).To(Equal(body),
				"the middleware consumes r.Body to inspect startsAt and MUST rewind it for downstream adapters")
		})
	})

	Describe("EventFreshnessValidator", func() {
		var handler http.Handler

		BeforeEach(func() {
			handler = middleware.EventFreshnessValidator(5 * time.Minute)(passthroughHandler)
		})

		DescribeTable("non-write methods bypass freshness validation entirely",
			func(method string) {
				req := httptest.NewRequest(method, "/api/v1/signals/kubernetes-event", nil)
				rr := httptest.NewRecorder()
				handler.ServeHTTP(rr, req)
				Expect(rr.Code).To(Equal(http.StatusOK))
			},
			Entry("GET", "GET"),
			Entry("HEAD", "HEAD"),
			Entry("OPTIONS", "OPTIONS"),
		)

		It("prefers lastTimestamp over firstTimestamp when both are present", func() {
			body := fmt.Sprintf(`{"firstTimestamp":%q,"lastTimestamp":%q}`,
				time.Now().Add(-2*time.Hour).Format(time.RFC3339), // stale first occurrence
				time.Now().Format(time.RFC3339),                   // fresh recurrence
			)
			req := httptest.NewRequest("POST", "/api/v1/signals/kubernetes-event", strings.NewReader(body))

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK),
				"Design Decision: recurring events have a more recent lastTimestamp, which must take precedence")
		})

		It("falls back to firstTimestamp when lastTimestamp is absent", func() {
			body := fmt.Sprintf(`{"firstTimestamp":%q}`, time.Now().Format(time.RFC3339))
			req := httptest.NewRequest("POST", "/api/v1/signals/kubernetes-event", strings.NewReader(body))

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK))
		})

		It("rejects an event whose lastTimestamp is older than the tolerance window (stale/replayed event)", func() {
			body := fmt.Sprintf(`{"lastTimestamp":%q}`, time.Now().Add(-1*time.Hour).Format(time.RFC3339))
			req := httptest.NewRequest("POST", "/api/v1/signals/kubernetes-event", strings.NewReader(body))

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest),
				"BR-GATEWAY-074: an event older than the tolerance window must be rejected as possibly stale/replayed")
		})

		It("rejects an event with a future lastTimestamp (clock skew attack)", func() {
			body := fmt.Sprintf(`{"lastTimestamp":%q}`, time.Now().Add(10*time.Minute).Format(time.RFC3339))
			req := httptest.NewRequest("POST", "/api/v1/signals/kubernetes-event", strings.NewReader(body))

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
		})

		It("rejects an event body missing both lastTimestamp and firstTimestamp", func() {
			req := httptest.NewRequest("POST", "/api/v1/signals/kubernetes-event", strings.NewReader(`{}`))

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest),
				"BR-GATEWAY-075: without any timestamp evidence, freshness cannot be proven and must be rejected")
		})

		It("passes through when the body is not valid JSON (lets adapter-level parsing report the error)", func() {
			req := httptest.NewRequest("POST", "/api/v1/signals/kubernetes-event", strings.NewReader("not json"))

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK))
		})

		It("rejects an oversized body with 413 instead of buffering it into memory", func() {
			oversized := strings.NewReader(strings.Repeat("a", 11*1024*1024))
			req := httptest.NewRequest("POST", "/api/v1/signals/kubernetes-event", oversized)

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			Expect(rr.Code).To(Equal(http.StatusRequestEntityTooLarge))
		})

		It("rewinds the body so the downstream handler can still read it after freshness validation", func() {
			var bodyContentSeenDownstream string
			recordingHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				bodyContentSeenDownstream = string(b)
				w.WriteHeader(http.StatusOK)
			})
			wrapped := middleware.EventFreshnessValidator(5 * time.Minute)(recordingHandler)

			body := fmt.Sprintf(`{"lastTimestamp":%q}`, time.Now().Format(time.RFC3339))
			req := httptest.NewRequest("POST", "/api/v1/signals/kubernetes-event", strings.NewReader(body))

			rr := httptest.NewRecorder()
			wrapped.ServeHTTP(rr, req)

			Expect(bodyContentSeenDownstream).To(Equal(body))
		})
	})
})