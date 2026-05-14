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

package auth

import (
	"io"
	"net/http"
	"strconv"
	"time"
)

// RetryOn429Transport is a test-only http.RoundTripper decorator that
// automatically retries requests receiving HTTP 429 (Too Many Requests).
//
// E2E suites run many tests against a rate-limited server; transient 429s
// from the per-IP token-bucket limiter are expected when Ginkgo randomises
// test order. This transport absorbs those transient rejections so that
// individual tests don't need retry logic.
//
// The production rate limiter configuration is intentionally NOT changed;
// this retry lives entirely in test infrastructure.
type RetryOn429Transport struct {
	base       http.RoundTripper
	maxRetries int
	baseDelay  time.Duration
}

// NewRetryOn429Transport wraps base with retry-on-429 behaviour.
// Defaults: up to 5 retries, starting with a 500ms delay (doubling each
// attempt, capped at 4s). Respects Retry-After header when present.
func NewRetryOn429Transport(base http.RoundTripper) *RetryOn429Transport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &RetryOn429Transport{
		base:       base,
		maxRetries: 5,
		baseDelay:  500 * time.Millisecond,
	}
}

// RoundTrip implements http.RoundTripper.
func (t *RetryOn429Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	delay := t.baseDelay
	const maxDelay = 4 * time.Second

	for attempt := 0; ; attempt++ {
		resp, err := t.base.RoundTrip(req)
		if err != nil {
			return resp, err
		}
		if resp.StatusCode != http.StatusTooManyRequests || attempt >= t.maxRetries {
			return resp, nil
		}

		// Drain body so the connection can be reused.
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()

		wait := delay
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, parseErr := strconv.Atoi(ra); parseErr == nil && secs > 0 {
				wait = time.Duration(secs) * time.Second
			}
		}

		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(wait): // ✅ APPROVED EXCEPTION: rate-limiter backoff in test transport
		}

		delay *= 2
		if delay > maxDelay {
			delay = maxDelay
		}
	}
}
