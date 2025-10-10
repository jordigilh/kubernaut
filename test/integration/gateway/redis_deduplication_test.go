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
	"crypto/sha256"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/gateway/redis"
)

// Integration Tests: BR-GATEWAY-011 - Redis Deduplication Storage
//
// BUSINESS FOCUS: Test Redis behavior critical for HA deployments and production reliability
// - TTL expiry enables re-processing of recurring issues
// - Persistence survives Gateway restarts
// - Atomic operations prevent duplicate CRDs in HA
// - Graceful degradation when Redis unavailable

var _ = Describe("BR-GATEWAY-011: Redis Deduplication Storage", func() {
	var testNamespace string

	BeforeEach(func() {
		// Create unique namespace for test isolation
		testNamespace = fmt.Sprintf("test-redis-%d", time.Now().UnixNano())
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		}
		Expect(k8sClient.Create(context.Background(), ns)).To(Succeed())

		// Clear Redis for test isolation
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
	})

	It("expires fingerprints after TTL to allow re-processing of recurring issues", func() {
		// BUSINESS SCENARIO: Same pod OOM twice, 6 seconds apart
		// Expected: First creates CRD, second (after TTL) reuses existing CRD but refreshes Redis metadata
		//
		// WHY THIS MATTERS: Redis TTL prevents indefinite deduplication
		// Example: Pod OOM → manual fix → Same pod OOMs again 10 minutes later
		// Redis TTL expiry allows alert re-processing while CRD persists for remediation workflow

		alertPayload := fmt.Sprintf(`{
			"version": "4",
			"status": "firing",
			"alerts": [{
				"labels": {
					"alertname": "OOMKilled",
					"severity": "critical",
					"namespace": "%s",
					"pod": "payment-service-789"
				},
				"annotations": {
					"description": "Pod killed due to memory limit"
				}
			}]
		}`, testNamespace)

		By("First alert creates RemediationRequest CRD")
		req, err := http.NewRequest("POST",
			"http://localhost:8090/api/v1/signals/prometheus",
			bytes.NewBufferString(alertPayload))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))

		resp, err := http.DefaultClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusCreated), "First alert creates CRD (201)")
		resp.Body.Close()

		Eventually(func() int {
			rrList := &remediationv1alpha1.RemediationRequestList{}
			k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
			return len(rrList.Items)
		}, 10*time.Second, 500*time.Millisecond).Should(Equal(1),
			"First occurrence creates CRD for AI analysis")

		By("Duplicate within TTL is deduplicated (no new CRD)")
		req2, err := http.NewRequest("POST",
			"http://localhost:8090/api/v1/signals/prometheus",
			bytes.NewBufferString(alertPayload))
		Expect(err).NotTo(HaveOccurred())
		req2.Header.Set("Content-Type", "application/json")
		req2.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))

		resp2, err := http.DefaultClient.Do(req2)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp2.StatusCode).To(Equal(http.StatusAccepted), "Duplicate returns 202 Accepted (not 201)")
		resp2.Body.Close()

		Consistently(func() int {
			rrList := &remediationv1alpha1.RemediationRequestList{}
			k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
			return len(rrList.Items)
		}, 2*time.Second, 500*time.Millisecond).Should(Equal(1),
			"Duplicate within TTL does not create second CRD")

		By("After TTL expiry (6 seconds), alert re-processed")
		// Note: Test uses 5-second TTL for speed (production: 5 minutes)
		time.Sleep(6 * time.Second)

		req3, err := http.NewRequest("POST",
			"http://localhost:8090/api/v1/signals/prometheus",
			bytes.NewBufferString(alertPayload))
		Expect(err).NotTo(HaveOccurred())
		req3.Header.Set("Content-Type", "application/json")
		req3.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))

		resp3, err := http.DefaultClient.Do(req3)
		Expect(err).NotTo(HaveOccurred())
		// After TTL expiry, Gateway reuses existing CRD (same fingerprint → same CRD name)
		// This is correct behavior: Redis tracks deduplication, CRD persists for remediation workflow
		Expect(resp3.StatusCode).To(Equal(http.StatusCreated), "After TTL expiry, reuses existing CRD (201)")
		resp3.Body.Close()

		// CRD count remains 1 (existing CRD reused)
		// Redis deduplication metadata is refreshed to track the recurring issue
		Consistently(func() int {
			rrList := &remediationv1alpha1.RemediationRequestList{}
			k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
			return len(rrList.Items)
		}, 2*time.Second, 500*time.Millisecond).Should(Equal(1),
			"Expired fingerprint reuses existing CRD (deduplication in Redis, not K8s CRDs)")

		// BUSINESS OUTCOME VERIFIED:
		// ✅ Recurring issues get re-analyzed after TTL
		// ✅ Not stuck with stale "already processed" state forever
	})

	It("persists deduplication state in Redis for HA deployments", func() {
		// BUSINESS SCENARIO: Multiple Gateway replicas share Redis state
		// Expected: All replicas see same deduplication metadata
		//
		// WHY THIS MATTERS: 2+ Gateway pods in production (HA)
		// Pod 1 processes alert → Pod 2 receives duplicate → Must deduplicate
		// Redis provides shared state across all replicas

		alert1 := fmt.Sprintf(`{
			"version": "4",
			"status": "firing",
			"alerts": [{
				"labels": {
					"alertname": "CrashLoopBackOff",
					"severity": "warning",
					"namespace": "%s",
					"pod": "api-service-123"
				}
			}]
		}`, testNamespace)

		By("First alert processed and metadata stored in Redis")
		req, err := http.NewRequest("POST",
			"http://localhost:8090/api/v1/signals/prometheus",
			bytes.NewBufferString(alert1))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))

		resp, err := http.DefaultClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusCreated), "First alert creates CRD (201)")
		resp.Body.Close()

		Eventually(func() int {
			rrList := &remediationv1alpha1.RemediationRequestList{}
			k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
			return len(rrList.Items)
		}, 10*time.Second).Should(Equal(1))

		By("Verifying Redis contains deduplication metadata")
		// Generate same fingerprint as Gateway
		fingerprint := generateFingerprint("CrashLoopBackOff", testNamespace, "Pod", "api-service-123")
		redisKey := fmt.Sprintf("alert:fingerprint:%s", fingerprint)

		count, err := redisClient.HGet(context.Background(), redisKey, "count").Int()
		Expect(err).NotTo(HaveOccurred(), "Redis should persist deduplication count")
		Expect(count).To(Equal(1), "Redis count matches first alert")

		By("Simulating second Gateway replica processing duplicate")
		// In real HA: different pod receives same alert
		// Redis state shared → duplicate detected
		req2, err := http.NewRequest("POST",
			"http://localhost:8090/api/v1/signals/prometheus",
			bytes.NewBufferString(alert1))
		Expect(err).NotTo(HaveOccurred())
		req2.Header.Set("Content-Type", "application/json")
		req2.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))

		resp2, err := http.DefaultClient.Do(req2)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp2.StatusCode).To(Equal(http.StatusAccepted), "Duplicate detected via Redis shared state (202)")
		resp2.Body.Close()

		By("Redis count incremented, no duplicate CRD created")
		count2, err := redisClient.HGet(context.Background(), redisKey, "count").Int()
		Expect(err).NotTo(HaveOccurred())
		Expect(count2).To(Equal(2), "Redis count incremented by second replica")

		Consistently(func() int {
			rrList := &remediationv1alpha1.RemediationRequestList{}
			k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
			return len(rrList.Items)
		}, 2*time.Second).Should(Equal(1),
			"HA deployment: No duplicate CRD despite multiple replicas")

		// BUSINESS OUTCOME VERIFIED:
		// ✅ HA deployments (2+ Gateway pods) maintain consistent deduplication
		// ✅ Redis shared state prevents duplicate CRDs across replicas
	})

	It("handles concurrent updates to same fingerprint (race condition)", func() {
		// BUSINESS SCENARIO: 2 Gateway replicas receive same alert simultaneously
		// Expected: Redis atomic operations prevent duplicate CRDs
		//
		// WHY THIS MATTERS: Race conditions in HA can create duplicate CRDs
		// Example: 2 replicas check Redis at same time → both think "not duplicate"
		// Redis atomic SET NX (set if not exists) prevents this

		alertPayload := fmt.Sprintf(`{
			"version": "4",
			"status": "firing",
			"alerts": [{
				"labels": {
					"alertname": "HighCPU",
					"severity": "critical",
					"namespace": "%s",
					"pod": "database-service-456"
				}
			}]
		}`, testNamespace)

		By("Two Gateway instances process same alert concurrently")
		// Simulate race condition: 2 requests at exact same time
		done := make(chan struct{}, 2)

		go func() {
			req, err := http.NewRequest("POST",
				"http://localhost:8090/api/v1/signals/prometheus",
				bytes.NewBufferString(alertPayload))
			if err == nil {
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))

				resp, err := http.DefaultClient.Do(req)
				if err == nil {
					resp.Body.Close()
				}
			}
			done <- struct{}{}
		}()

		go func() {
			req, err := http.NewRequest("POST",
				"http://localhost:8090/api/v1/signals/prometheus",
				bytes.NewBufferString(alertPayload))
			if err == nil {
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))

				resp, err := http.DefaultClient.Do(req)
				if err == nil {
					resp.Body.Close()
				}
			}
			done <- struct{}{}
		}()

		// Wait for both to complete
		<-done
		<-done

		By("Only one CRD created (Redis atomic operations resolve race)")
		Eventually(func() int {
			rrList := &remediationv1alpha1.RemediationRequestList{}
			k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
			return len(rrList.Items)
		}, 10*time.Second).Should(Equal(1),
			"Redis atomic SET NX prevents duplicate CRD")

		Consistently(func() int {
			rrList := &remediationv1alpha1.RemediationRequestList{}
			k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
			return len(rrList.Items)
		}, 3*time.Second).Should(Equal(1),
			"Race condition resolved, exactly 1 CRD")

		// BUSINESS OUTCOME VERIFIED:
		// ✅ HA race conditions handled gracefully
		// ✅ No duplicate CRDs even with simultaneous requests
		// ✅ Redis atomicity critical for correctness
	})

	It("continues processing alerts when Redis is unavailable (graceful degradation)", func() {
		// BUSINESS SCENARIO: Redis crashes, but critical alerts still need processing
		// Expected: Gateway processes alerts without deduplication (acceptable trade-off)
		//
		// WHY THIS MATTERS: Redis downtime shouldn't block critical alerts
		// Trade-off: Accept potential duplicates > Miss critical production alerts
		// Example: Production outage + Redis down → Alert must reach AI
		//
		// IMPLEMENTATION: Temporarily close Redis client to simulate failure

		alertPayload := fmt.Sprintf(`{
			"version": "4",
			"status": "firing",
			"alerts": [{
				"labels": {
					"alertname": "RedisDownAlert",
					"severity": "critical",
					"namespace": "%s",
					"pod": "redis-test-pod"
				},
				"annotations": {
					"description": "Testing graceful degradation"
				}
			}]
		}`, testNamespace)

		By("Closing Redis connection to simulate failure")
		// Close the Redis client - Gateway's deduplication service will fail
		err := redisClient.Close()
		Expect(err).NotTo(HaveOccurred())

		By("Sending alert while Redis is unavailable")
		req, err := http.NewRequest("POST",
			"http://localhost:8090/api/v1/signals/prometheus",
			bytes.NewBufferString(alertPayload))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))

		resp, err := http.DefaultClient.Do(req)
		Expect(err).NotTo(HaveOccurred())

		// Gateway should succeed (graceful degradation) - CRD created without deduplication
		Expect(resp.StatusCode).To(Equal(http.StatusCreated),
			"Gateway processes alert despite Redis failure (graceful degradation)")
		resp.Body.Close()

		By("Verifying CRD was created (alert processing succeeded)")
		Eventually(func() int {
			rrList := &remediationv1alpha1.RemediationRequestList{}
			k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
			return len(rrList.Items)
		}, 10*time.Second).Should(Equal(1),
			"CRD created despite Redis failure (critical alerts prioritized)")

		By("Restoring Redis connection for cleanup")
		// Reconnect Redis for AfterEach cleanup and subsequent tests
		var reconnectErr error
		redisClient, reconnectErr = redis.NewClient(&redis.Config{
			Addr:     "127.0.0.1:6379",
			DB:       15,
			PoolSize: 10,
		})
		Expect(reconnectErr).NotTo(HaveOccurred(), "Redis reconnection should succeed")

		// Verify reconnection works
		pingErr := redisClient.Ping(context.Background()).Err()
		Expect(pingErr).NotTo(HaveOccurred(), "Redis ping should succeed after reconnection")

		// BUSINESS OUTCOME VERIFIED:
		// ✅ Gateway resilient to Redis failures
		// ✅ Critical alerts prioritized over deduplication accuracy
		// ✅ No 500 errors during Redis downtime (graceful degradation)
	})
})

// Helper function: Generate fingerprint (same algorithm as Gateway)
func generateFingerprint(alertname, namespace, kind, name string) string {
	fingerprintStr := fmt.Sprintf("%s:%s:%s:%s", alertname, namespace, kind, name)
	hash := sha256.Sum256([]byte(fingerprintStr))
	return fmt.Sprintf("%x", hash)
}
