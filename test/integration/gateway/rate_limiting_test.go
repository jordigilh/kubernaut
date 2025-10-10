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

package gateway

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// Integration Tests: BR-GATEWAY-004 Extension - Rate Limiting
//
// BUSINESS FOCUS: Protect Gateway from overwhelming traffic
// - Rate limiting prevents system overload
// - Per-source isolation prevents noisy neighbor
// - Burst capacity handles legitimate alert storms

var _ = Describe("BR-GATEWAY-004 Extension: Rate Limiting", func() {
	var testNamespace string

	BeforeEach(func() {
		// Create unique namespace for test isolation
		testNamespace = fmt.Sprintf("test-rate-%d", time.Now().UnixNano())
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		}
		Expect(k8sClient.Create(context.Background(), ns)).To(Succeed())

		// Clear Redis
		Expect(redisClient.FlushDB(context.Background()).Err()).To(Succeed())
	})

	AfterEach(func() {
		// Cleanup namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		}
		_ = k8sClient.Delete(context.Background(), ns)

		// Allow rate limiters to reset
		time.Sleep(2 * time.Second)
	})

	It("enforces per-source rate limits to prevent system overload", func() {
		// BUSINESS SCENARIO: Noisy Prometheus instance sending 150 alerts/min
		// Expected: Rate limiter blocks excess (default limit: 100 alerts/min)
		//
		// WHY THIS MATTERS: Runaway alertmanager can overwhelm Gateway
		// Example: Misconfigured alert rule fires 1000 times/min
		// Rate limiter protects Gateway, Redis, and downstream controllers

		alertPayload := fmt.Sprintf(`{
			"version": "4",
			"status": "firing",
			"alerts": [{
				"labels": {
					"alertname": "TestAlert",
					"severity": "warning",
					"namespace": "%s",
					"pod": "test-pod-%d"
				}
			}]
		}`, testNamespace, time.Now().UnixNano())

		By("Sending 150 alerts rapidly (above 100/min rate limit)")
		successCount := 0
		rateLimitedCount := 0

		// Use unique X-Forwarded-For to isolate from other tests
		testSourceIP := fmt.Sprintf("10.0.0.%d", time.Now().Unix()%255)

		for i := 0; i < 150; i++ {
			req, err := http.NewRequest("POST",
				"http://localhost:8090/api/v1/signals/prometheus",
				bytes.NewBufferString(alertPayload))
			if err != nil {
				continue
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))
			req.Header.Set("X-Forwarded-For", testSourceIP) // Isolate from other tests

			resp, err := http.DefaultClient.Do(req)

			if err == nil {
				if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusAccepted {
					successCount++ // 201 Created (new) or 202 Accepted (duplicate)
				} else if resp.StatusCode == http.StatusTooManyRequests {
					rateLimitedCount++
				}
				resp.Body.Close()
			}

			// Small delay to simulate realistic traffic (not instant burst)
			time.Sleep(10 * time.Millisecond)
		}

		By("Rate limiter blocks excess alerts")
		// Math with realistic limits (100 req/min, burst 20):
		// - Burst capacity: 20 tokens
		// - Refill over 1.5s: 1.67 req/sec × 1.5s ≈ 2.5 tokens
		// - Total allowed: ~23 requests
		// - Total blocked: ~127 requests (out of 150)
		Expect(rateLimitedCount).To(BeNumerically(">", 100),
			"Rate limiter should block significant excess traffic (>100 out of 150)")

		By("Rate limiter allows legitimate traffic")
		// Expect ~20-25 requests to succeed (burst + small refill)
		Expect(successCount).To(BeNumerically(">", 15),
			"Rate limiter should allow initial burst traffic (burst capacity = 20)")

		// BUSINESS OUTCOME VERIFIED:
		// ✅ Gateway protected from overwhelming traffic
		// ✅ Legitimate alerts still processed
		// ✅ Rate limiting transparent to normal operations
	})

	It("isolates rate limits per source (noisy neighbor protection)", func() {
		// BUSINESS SCENARIO: Source 1 is noisy (150 alerts), Source 2 is normal (10 alerts)
		// Expected: Source 2 unaffected by Source 1's rate limit
		//
		// WHY THIS MATTERS: Multi-tenant environments need isolation
		// Example: Team A's noisy alerts shouldn't block Team B's critical alerts
		// Per-source rate limiting provides fair resource allocation
		//
		// IMPLEMENTATION: Use X-Forwarded-For header (standard for services behind load balancers)
		// Production: Load balancer (ALB, NGINX, HAProxy) sets this header with real client IP

		alertTemplate := `{
			"version": "4",
			"status": "firing",
			"alerts": [{
				"labels": {
					"alertname": "NoisyAlert",
					"severity": "warning",
					"namespace": "%s",
					"pod": "noisy-pod-%d",
					"source": "%s"
				}
			}]
		}`

		source1RateLimited := 0
		source1Success := 0
		source2Success := 0

		By("Sending 150 alerts from Source 1 (noisy neighbor)")
		for i := 0; i < 150; i++ {
			alertPayload := fmt.Sprintf(alertTemplate, testNamespace, i, "source1")
			req, err := http.NewRequest("POST",
				"http://localhost:8090/api/v1/signals/prometheus",
				bytes.NewBufferString(alertPayload))
			if err != nil {
				continue
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))
			req.Header.Set("X-Forwarded-For", "192.168.1.1") // Source 1 IP

			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				if resp.StatusCode == http.StatusTooManyRequests {
					source1RateLimited++
				} else if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusAccepted {
					source1Success++
				}
				resp.Body.Close()
			}

			// Small delay to simulate realistic traffic
			time.Sleep(10 * time.Millisecond)
		}

		By("Sending 10 alerts from Source 2 (legitimate traffic)")
		for i := 0; i < 10; i++ {
			alertPayload := fmt.Sprintf(alertTemplate, testNamespace, i, "source2")
			req, err := http.NewRequest("POST",
				"http://localhost:8090/api/v1/signals/prometheus",
				bytes.NewBufferString(alertPayload))
			if err != nil {
				continue
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))
			req.Header.Set("X-Forwarded-For", "192.168.1.2") // Source 2 IP (different!)

			resp, err := http.DefaultClient.Do(req)
			if err == nil && (resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusAccepted) {
				source2Success++
			}
			if err == nil {
				resp.Body.Close()
			}

			// Small delay
			time.Sleep(10 * time.Millisecond)
		}

		By("Source 1 (noisy) should be rate limited")
		// Math: 100 req/min, burst 20 → ~23 allowed (burst + small refill), ~127 blocked
		Expect(source1RateLimited).To(BeNumerically(">", 100),
			"Noisy source should be rate limited (>100 out of 150)")

		By("Source 2 (legitimate) should NOT be affected by Source 1's rate limit")
		// All 10 alerts from Source 2 should succeed (well below rate limit)
		Expect(source2Success).To(Equal(10),
			"Legitimate source unaffected by noisy neighbor")

		// BUSINESS OUTCOME VERIFIED:
		// ✅ Per-source isolation prevents noisy neighbor
		// ✅ Fair resource allocation across sources
		// ✅ Multi-tenant safety (Team A can't DoS Team B)
	})

	It("allows burst traffic within token bucket capacity", func() {
		// BUSINESS SCENARIO: Alert storm causes 50 alerts in 5 seconds
		// Expected: Token bucket burst capacity handles spike
		//
		// WHY THIS MATTERS: Legitimate alert storms (pod rollout failure)
		// Example: 20-pod deployment fails → 20 alerts in 2 seconds
		// Burst capacity prevents blocking legitimate storms

		alertTemplate := `{
			"version": "4",
			"status": "firing",
			"alerts": [{
				"labels": {
					"alertname": "BurstTestAlert",
					"severity": "critical",
					"namespace": "%s",
					"pod": "burst-pod-%d"
				}
			}]
		}`

		By("Sending 50 alerts in rapid burst (5 seconds)")
		successCount := 0

		// Use unique X-Forwarded-For to isolate from other tests
		burstSourceIP := fmt.Sprintf("10.0.1.%d", time.Now().Unix()%255)

		for i := 0; i < 50; i++ {
			alertPayload := fmt.Sprintf(alertTemplate, testNamespace, i)
			req, err := http.NewRequest("POST",
				"http://localhost:8090/api/v1/signals/prometheus",
				bytes.NewBufferString(alertPayload))
			if err != nil {
				continue
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))
			req.Header.Set("X-Forwarded-For", burstSourceIP) // Isolate from other tests

			resp, err := http.DefaultClient.Do(req)

			if err == nil && (resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusAccepted) {
				successCount++ // 201 Created (new) or 202 Accepted (duplicate)
			}
			if err == nil {
				resp.Body.Close()
			}

			// Rapid burst (100ms intervals = 50 in 5 seconds)
			time.Sleep(100 * time.Millisecond)
		}

		By("Token bucket allows burst within capacity")
		// Math with realistic limits (100 req/min, burst 20):
		// - Burst capacity: 20 tokens
		// - Refill over 5s: 1.67 req/sec × 5s ≈ 8 tokens
		// - Total allowed: ~28 requests (out of 50)
		// - Using ≥15 threshold to account for timing variations in test execution
		Expect(successCount).To(BeNumerically(">=", 15),
			"Burst capacity handles temporary spike (burst=20 + refill=8, actual may vary with timing)")

		By("Verifying CRDs created for burst traffic")
		Eventually(func() int {
			rrList := &remediationv1alpha1.RemediationRequestList{}
			k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
			return len(rrList.Items)
		}, 15*time.Second).Should(BeNumerically(">=", 15),
			"Burst traffic results in CRD creation (expecting ~20-28 unique alerts)")

		// BUSINESS OUTCOME VERIFIED:
		// ✅ Legitimate alert bursts not blocked
		// ✅ Token bucket refills for sustained traffic
		// ✅ Production deployments can fail gracefully
	})
})
