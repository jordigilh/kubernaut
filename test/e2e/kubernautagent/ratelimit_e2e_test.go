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

package kubernautagent

import (
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// xffTransport wraps an http.RoundTripper and injects X-Forwarded-For
// to isolate rate-limit tests into their own per-IP bucket, preventing
// interference with other E2E tests that share the same source IP.
type xffTransport struct {
	base http.RoundTripper
	ip   string
}

func (t *xffTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set("X-Forwarded-For", t.ip)
	return t.base.RoundTrip(clone)
}

// E2E Rate Limiting Tests — BR-KA-OBSERVABILITY-001 / #823
//
// Validates that the per-IP rate limiter returns HTTP 429 when the burst
// is exhausted, and recovers after the token bucket refills.
//
// Configuration: DefaultRateLimitConfig() → 5 req/s, burst 10 (hardcoded).
// PoC confirmed 11 sequential requests exhaust the burst in <4ms.
//
// Isolation strategy:
//   - Uses X-Forwarded-For with a synthetic IP ("198.51.100.99") so the
//     rate-limit bucket is independent from all other E2E tests
//   - Uses Ordered container so RL-001 runs before RL-002
//   - Hits a non-existent session ID (cheap 404, no investigation cost)

var _ = Describe("E2E-KA-RL: Rate Limiting", Ordered, Label("e2e", "ka", "rate-limit"), func() {

	// rlClient uses a dedicated X-Forwarded-For IP so the rate-limit
	// bucket is isolated from other E2E tests sharing the same source.
	// 198.51.100.0/24 is TEST-NET-2 (RFC 5737) — safe for test fixtures.
	var rlClient *http.Client

	BeforeAll(func() {
		rlClient = &http.Client{
			Transport: &xffTransport{
				base: authHTTPClient.Transport,
				ip:   "198.51.100.99",
			},
			Timeout: 30 * time.Second,
		}
	})

	targetURL := func() string {
		return fmt.Sprintf("%s/api/v1/incident/session/rl-nonexistent/status", kaURL)
	}

	// -----------------------------------------------------------------
	// E2E-KA-RL-001: Burst+1 request returns 429
	// -----------------------------------------------------------------

	It("E2E-KA-RL-001: Exceeding burst limit returns HTTP 429", func() {
		By("Sending burst+1 (11) rapid requests via isolated IP bucket")

		const totalRequests = 15
		statuses := make([]int, totalRequests)

		for i := 0; i < totalRequests; i++ {
			req, err := http.NewRequestWithContext(ctx, "GET", targetURL(), nil)
			Expect(err).ToNot(HaveOccurred())
			resp, err := rlClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			_ = resp.Body.Close()
			statuses[i] = resp.StatusCode
		}

		By("Asserting at least one 429 was received")
		count429 := 0
		first429 := -1
		for i, code := range statuses {
			if code == http.StatusTooManyRequests {
				count429++
				if first429 == -1 {
					first429 = i + 1
				}
			}
		}

		GinkgoWriter.Printf("Rate limit results: %v\n", statuses)
		GinkgoWriter.Printf("429 count: %d, first at request #%d\n", count429, first429)

		Expect(count429).To(BeNumerically(">=", 1),
			"at least one request should be rate-limited (429)")
		Expect(first429).To(BeNumerically("<=", 12),
			"first 429 should appear within burst+2 requests (accounting for refill)")
	})

	// -----------------------------------------------------------------
	// E2E-KA-RL-002: Recovery after rate limit window
	// -----------------------------------------------------------------

	It("E2E-KA-RL-002: Requests succeed after token bucket refills", func() {
		By("Waiting for token bucket to refill (burst=10 at 5/s → 2s full refill)")
		time.Sleep(3 * time.Second) // ✅ APPROVED EXCEPTION: intentional rate-limiter refill wait

		By("Sending a single request — should not be 429")
		req, err := http.NewRequestWithContext(ctx, "GET", targetURL(), nil)
		Expect(err).ToNot(HaveOccurred())
		resp, err := rlClient.Do(req)
		Expect(err).ToNot(HaveOccurred())
		_ = resp.Body.Close()

		Expect(resp.StatusCode).ToNot(Equal(http.StatusTooManyRequests),
			"request should not be rate-limited after bucket refill")
		Expect(resp.StatusCode).To(Equal(http.StatusNotFound),
			"non-existent session should return 404 (not 429)")
	})
})
