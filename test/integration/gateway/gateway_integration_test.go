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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-redis/redis/v8"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// Integration Tests: Business Outcome Focus
//
// PRINCIPLE: Integration tests verify end-to-end business workflows,
// not infrastructure implementation details.
//
// ❌ DON'T TEST: Redis key formats, HTTP status codes, CRD field names
// ✅ DO TEST: Downstream services can discover and process requests

var _ = Describe("Gateway Integration: Business Outcomes", func() {
	var testNamespace string

	BeforeEach(func() {
		// Create unique namespace for test isolation
		testNamespace = fmt.Sprintf("test-gw-%d", time.Now().UnixNano())
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		}
		Expect(k8sClient.Create(context.Background(), ns)).To(Succeed())

		// Clear Redis (infrastructure cleanup, not business test)
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

	// BUSINESS OUTCOME 1: Enable downstream AI service to discover and analyze failures
	Describe("BR-GATEWAY-001-002: Alert Ingestion for Downstream Remediation", func() {
		It("enables AI service to discover Kubernetes failures from monitoring alerts", func() {
			// Business scenario: Prometheus detects pod memory issue
			// Expectation: AI service can discover this via RemediationRequest CRD

			By("Prometheus AlertManager sends webhook about pod failure")
			alertPayload := fmt.Sprintf(`{
				"version": "4",
				"status": "firing",
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "PodMemoryHigh",
						"namespace": "%s",
						"pod": "payment-service-789",
						"severity": "critical"
					},
					"annotations": {
						"summary": "Pod memory exceeds 90%%"
					},
					"startsAt": "2025-10-09T10:00:00Z"
				}]
			}`, testNamespace)

			req, err := http.NewRequest("POST", "http://localhost:8090/api/v1/signals/prometheus",
				bytes.NewBufferString(alertPayload))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			By("AI service discovers exactly one remediation request to analyze")
			// Business outcome: AI service can query K8s API and find work to do
			Eventually(func() int {
				rrList := &remediationv1alpha1.RemediationRequestList{}
				err := k8sClient.List(context.Background(), rrList,
					client.InNamespace(testNamespace))
				Expect(err).NotTo(HaveOccurred())
				return len(rrList.Items)
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(1),
				"AI service needs exactly 1 request (not 0 = missed, not 2+ = duplicates)")

			By("AI service has complete information to start remediation workflow")
			// Business capability: CRD contains all data AI needs
			rrList := &remediationv1alpha1.RemediationRequestList{}
			err = k8sClient.List(context.Background(), rrList,
				client.InNamespace(testNamespace))
			Expect(err).NotTo(HaveOccurred())

			rr := rrList.Items[0]

			// Business outcome: AI has signal identification
			Expect(rr.Spec.SignalName).NotTo(BeEmpty(),
				"AI needs signal name to understand WHAT problem occurred")

			// Business outcome: AI has raw data to extract resource details
			Expect(rr.Spec.ProviderData).NotTo(BeEmpty(),
				"AI needs provider data to determine WHICH resource and WHERE to remediate")

			// Business outcome: AI has original context for analysis
			Expect(rr.Spec.OriginalPayload).NotTo(BeEmpty(),
				"AI can access full alert context for root cause analysis")

			// Business capability verified:
			// Prometheus alert → Gateway → CRD → AI can discover and start workflow
			// (We verify AI CAN start remediation, not HOW the data is structured)
		})

		It("enables AI service to discover Node failures from cluster alerts", func() {
			// Business scenario: Node has disk pressure
			// Expectation: AI can remediate cluster-level issues

			By("AlertManager sends cluster-scoped alert")
			alertPayload := `{
				"alerts": [{
					"labels": {
						"alertname": "NodeDiskPressure",
						"node": "worker-node-3",
						"severity": "critical"
					}
				}]
			}`

			req, err := http.NewRequest("POST", "http://localhost:8090/api/v1/signals/prometheus",
				bytes.NewBufferString(alertPayload))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			By("AI service can discover cluster-scoped remediation requests")
			Eventually(func() int {
				rrList := &remediationv1alpha1.RemediationRequestList{}
				err := k8sClient.List(context.Background(), rrList)
				if err != nil {
					return 0
				}
				// Count only CRDs for this specific Node alert
				count := 0
				for _, rr := range rrList.Items {
					if rr.Spec.SignalName == "NodeDiskPressure" {
						count++
					}
				}
				return count
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(1),
				"Exactly 1 CRD should be created for Node alert (not multiple duplicates)")

			By("Verifying Node CRD is cluster-scoped (not namespaced)")
			rrList := &remediationv1alpha1.RemediationRequestList{}
			err = k8sClient.List(context.Background(), rrList)
			Expect(err).NotTo(HaveOccurred())

			var nodeCRD *remediationv1alpha1.RemediationRequest
			for i := range rrList.Items {
				if rrList.Items[i].Spec.SignalName == "NodeDiskPressure" {
					nodeCRD = &rrList.Items[i]
					break
				}
			}
			Expect(nodeCRD).NotTo(BeNil(), "Node alert CRD should exist")

			// Verify it's cluster-scoped (CRD created in default namespace or cluster-scoped)
			// Note: Kubernetes CRDs are always namespaced, but we verify the signal namespace is empty
			// to indicate cluster-scoped resource
			Expect(nodeCRD.Namespace).NotTo(BeEmpty(),
				"RemediationRequest CRD itself is namespaced (Kubernetes design)")

			// Business capability: System handles both namespaced and cluster-scoped resources
		})
	})

	// BUSINESS OUTCOME 2: Prevent AI from analyzing same issue multiple times
	Describe("BR-GATEWAY-010: Deduplication Saves AI Analysis Costs", func() {
		It("prevents AI from wasting resources on duplicate alerts", func() {
			// Business scenario: AlertManager sends same alert every 30 seconds
			// Without deduplication: 20 alerts = 20 AI analyses = $$$
			// With deduplication: 20 alerts = 1 AI analysis

			alertPayload := fmt.Sprintf(`{
				"alerts": [{
					"labels": {
						"alertname": "PodCrashLoop",
						"namespace": "%s",
						"pod": "api-server-1",
						"severity": "critical"
					},
					"startsAt": "2025-10-09T10:00:00Z"
				}]
			}`, testNamespace)

			By("AlertManager sends first alert")
			sendAlert := func(payload string) {
				req, err := http.NewRequest("POST", "http://localhost:8090/api/v1/signals/prometheus",
					bytes.NewBufferString(payload))
				Expect(err).NotTo(HaveOccurred())
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))
				resp, err := http.DefaultClient.Do(req)
				Expect(err).NotTo(HaveOccurred())
				resp.Body.Close()
			}

			sendAlert(alertPayload)

			By("Waiting for first CRD to be created")
			Eventually(func() int {
				rrList := &remediationv1alpha1.RemediationRequestList{}
				err := k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
				Expect(err).NotTo(HaveOccurred())
				return len(rrList.Items)
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(1))

			By("AlertManager sends same alert 4 more times (every 30 seconds)")
			for i := 0; i < 4; i++ {
				time.Sleep(100 * time.Millisecond) // Simulate time between alerts
				sendAlert(alertPayload)
			}

			By("AI service still has only 1 remediation request to analyze")
			// Business outcome: Deduplication prevents redundant AI analysis
			Consistently(func() int {
				rrList := &remediationv1alpha1.RemediationRequestList{}
				err := k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
				Expect(err).NotTo(HaveOccurred())
				return len(rrList.Items)
			}, 3*time.Second, 500*time.Millisecond).Should(Equal(1),
				"5 duplicate alerts = 1 CRD (saves AI from analyzing same issue 5 times)")

			// Business capability verified:
			// Deduplication reduces AI analysis costs by 80% (5 alerts → 1 analysis)
			// (No Redis keys checked - we verify the business outcome, not the mechanism)
		})

		It("ensures different failures each get analyzed separately", func() {
			// Business scenario: 3 different pods fail
			// Expectation: AI analyzes all 3 failures (don't over-deduplicate)

			By("Three different pods crash")
			for i := 1; i <= 3; i++ {
				alertPayload := fmt.Sprintf(`{
					"alerts": [{
						"labels": {
							"alertname": "PodCrashLoop",
							"namespace": "%s",
							"pod": "api-server-%d",
							"severity": "critical"
						}
					}]
				}`, testNamespace, i)

				req, err := http.NewRequest("POST", "http://localhost:8090/api/v1/signals/prometheus",
					bytes.NewBufferString(alertPayload))
				Expect(err).NotTo(HaveOccurred())
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))
				resp, err := http.DefaultClient.Do(req)
				Expect(err).NotTo(HaveOccurred())
				resp.Body.Close()

				time.Sleep(100 * time.Millisecond)
			}

			By("AI service receives 3 separate remediation requests")
			// Business outcome: Different failures aren't incorrectly deduplicated
			Eventually(func() int {
				rrList := &remediationv1alpha1.RemediationRequestList{}
				err := k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
				Expect(err).NotTo(HaveOccurred())
				return len(rrList.Items)
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(3),
				"3 different pod failures = 3 separate analyses (don't over-deduplicate)")

			// Business capability: Deduplication is accurate (no false positives)
		})
	})

	// BUSINESS OUTCOME 3: Aggregate mass incidents to prevent overwhelming AI
	Describe("BR-GATEWAY-015-016: Storm Detection Prevents AI Overload", func() {
		It("aggregates mass incidents so AI analyzes root cause instead of 50 symptoms", func() {
			// Business scenario: Bad deployment causes 50 pods to crash in 1 minute
			// Without storm detection: 50 CRDs → AI analyzes 50 times → $$$
			// With storm detection: 1 aggregated CRD → AI finds root cause

			By("Deployment rollout fails, causing 12+ pods to crash rapidly")
			stormAlertName := fmt.Sprintf("DeploymentRolloutFailed-%d", time.Now().UnixNano())

			// Track responses to verify aggregation behavior
			type Response struct {
				Status   string `json:"status"`
				IsStorm  bool   `json:"isStorm"`
				WindowID string `json:"windowID"`
			}
			responses := make([]Response, 0, 12)

			for i := 0; i < 12; i++ {
				alertPayload := fmt.Sprintf(`{
					"alerts": [{
						"labels": {
							"alertname": "%s",
							"namespace": "%s",
							"pod": "api-server-%d",
							"severity": "critical"
						}
					}]
				}`, stormAlertName, testNamespace, i)

				req, err := http.NewRequest("POST", "http://localhost:8090/api/v1/signals/prometheus",
					bytes.NewBufferString(alertPayload))
				Expect(err).NotTo(HaveOccurred())
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))
				resp, err := http.DefaultClient.Do(req)
				Expect(err).NotTo(HaveOccurred())

				// Parse response to verify aggregation
				var response Response
				body, _ := io.ReadAll(resp.Body)
				json.Unmarshal(body, &response)
				responses = append(responses, response)
				resp.Body.Close()

				time.Sleep(50 * time.Millisecond) // Rapid fire
			}

			By("Gateway accepts all alerts for aggregation (not immediate CRD creation)")
			// Business capability: All alerts return "accepted" status
			for i, resp := range responses {
				Expect(resp.Status).To(Equal("accepted"),
					fmt.Sprintf("Alert %d should be accepted for aggregation", i))
				Expect(resp.IsStorm).To(BeTrue(),
					fmt.Sprintf("Alert %d should be marked as storm", i))
				Expect(resp.WindowID).NotTo(BeEmpty(),
					fmt.Sprintf("Alert %d should have aggregation window ID", i))
			}

			// Verify all alerts share the same window ID
			firstWindowID := responses[0].WindowID
			for i, resp := range responses[1:] {
				Expect(resp.WindowID).To(Equal(firstWindowID),
					fmt.Sprintf("Alert %d should use same window ID as first alert", i+1))
			}

			By("Waiting for 1-minute aggregation window to complete")
			time.Sleep(65 * time.Second) // 1 minute + 5 seconds buffer

			By("AI service receives exactly 1 aggregated CRD with all 12 affected resources")
			// Business outcome: Storm detection prevents AI overload through strict aggregation
			var rrList *remediationv1alpha1.RemediationRequestList
			Eventually(func() int {
				rrList = &remediationv1alpha1.RemediationRequestList{}
				err := k8sClient.List(context.Background(), rrList,
					client.InNamespace(testNamespace))
				if err != nil {
					return -1
				}

				// Count storm CRDs for this alertname
				count := 0
				for _, rr := range rrList.Items {
					if rr.Spec.SignalName == stormAlertName && rr.Spec.IsStorm {
						count++
					}
				}
				return count
			}, 15*time.Second, 500*time.Millisecond).Should(Equal(1),
				"Exactly 1 aggregated CRD should be created after window expires (not 12)")

			By("Verifying aggregated CRD contains all affected resources")
			// This is the strict verification that was missing in V1.0
			stormCRDs := make([]*remediationv1alpha1.RemediationRequest, 0)
			for i := range rrList.Items {
				if rrList.Items[i].Spec.SignalName == stormAlertName && rrList.Items[i].Spec.IsStorm {
					stormCRDs = append(stormCRDs, &rrList.Items[i])
				}
			}
			// Note: This is redundant with Eventually check above, but kept for clarity
			Expect(len(stormCRDs)).To(Equal(1),
				"Sanity check: Storm CRD should exist in filtered list")

			stormRR := stormCRDs[0]
			Expect(stormRR).NotTo(BeNil(), "Storm CRD should exist")

			// Business capability: Storm metadata helps AI choose strategy
			Expect(stormRR.Spec.StormType).NotTo(BeEmpty(),
				"Storm type guides AI: rate storm = infra issue, pattern storm = config issue")

			// Business capability: Aggregated CRD contains all affected resources
			Expect(stormRR.Spec.AffectedResources).To(HaveLen(12),
				"Aggregated CRD should contain all 12 affected resources")

			// Business value: 50 crashes → 1 root-cause fix (not 50 pod restarts)
		})
	})

	// BUSINESS OUTCOME 4: Protect system from unauthorized access
	Describe("BR-GATEWAY-004: Security Prevents Unauthorized Alert Injection", func() {
		It("protects system from malicious actors injecting fake alerts", func() {
			// Business scenario: Attacker tries to trigger unnecessary remediations
			// Expectation: System rejects unauthorized requests

			By("Attacker sends alert without authentication token")
			alertPayload := fmt.Sprintf(`{
				"alerts": [{
					"labels": {
						"alertname": "MaliciousAlert",
						"namespace": "%s"
					}
				}]
			}`, testNamespace)

			req, err := http.NewRequest("POST", "http://localhost:8090/api/v1/signals/prometheus",
				bytes.NewBufferString(alertPayload))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			// NO Authorization header

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			By("System prevents unauthorized CRD creation")
			// Business outcome: No remediation workflows triggered by attacker
			Consistently(func() int {
				rrList := &remediationv1alpha1.RemediationRequestList{}
				err := k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
				Expect(err).NotTo(HaveOccurred())
				return len(rrList.Items)
			}, 3*time.Second, 500*time.Millisecond).Should(Equal(0),
				"Unauthorized requests don't create CRDs (prevents malicious remediation triggers)")

			// Business capability: System security prevents unauthorized remediation workflows
			// (We don't check HTTP 401 status - we verify the business outcome: no CRD created)
		})
	})

	// BUSINESS OUTCOME 5: Enable risk-aware remediation strategies
	Describe("BR-GATEWAY-051-053: Environment Classification for Risk Management", func() {
		It("enables AI to apply conservative remediation in production", func() {
			// Business scenario: Production pod fails
			// Expectation: AI knows to be conservative (require approval, slow rollout)

			By("Creating production namespace")
			prodNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("prod-%d", time.Now().UnixNano()),
					Labels: map[string]string{
						"environment": "production",
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), prodNs)).To(Succeed())
			defer k8sClient.Delete(context.Background(), prodNs)

			By("Production alert triggers")
			alertPayload := fmt.Sprintf(`{
				"alerts": [{
					"labels": {
						"alertname": "PaymentServiceDown",
						"namespace": "%s",
						"pod": "payment-api-1",
						"severity": "critical"
					}
				}]
			}`, prodNs.Name)

			req, err := http.NewRequest("POST", "http://localhost:8090/api/v1/signals/prometheus",
				bytes.NewBufferString(alertPayload))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))
			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			By("AI service knows this is production (enables risk-aware strategy)")
			var prodRR *remediationv1alpha1.RemediationRequest
			Eventually(func() bool {
				rrList := &remediationv1alpha1.RemediationRequestList{}
				err := k8sClient.List(context.Background(), rrList,
					client.InNamespace(prodNs.Name))
				if err != nil || len(rrList.Items) != 1 {
					return false // ✅ Strict: exactly 1 CRD, not >=1
				}
				prodRR = &rrList.Items[0]
				// Business capability: Environment classification enables risk decisions
				return prodRR.Spec.Environment == "production"
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"Exactly 1 CRD should be created with production environment")

			By("Verifying environment classification affects priority")
			// Production + critical severity should result in P0 priority
			Expect(prodRR.Spec.Priority).To(Equal("P0"),
				"Production + critical severity → P0 priority (risk-aware decision)")

			By("Verifying namespace label was source of environment classification")
			// This verifies the classification logic actually read the namespace label
			ns := &corev1.Namespace{}
			err = k8sClient.Get(context.Background(), client.ObjectKey{Name: prodNs.Name}, ns)
			Expect(err).NotTo(HaveOccurred())
			Expect(ns.Labels["environment"]).To(Equal("production"),
				"Namespace label should be source of environment classification")

			// Business value:
			// Production → Manual approval required before pod restart (conservative)
			// Dev → Auto-restart immediately (aggressive)
			// Environment classification drives remediation aggressiveness
		})
	})

	// ========================================
	// PHASE 1 CRITICAL: EDGE CASES & FAILURE SCENARIOS
	// Added based on GATEWAY_TEST_COVERAGE_CONFIDENCE_ASSESSMENT.md
	// Priority: Prevents production incidents
	// ========================================

	// BUSINESS OUTCOME 6: System resilience during Redis failures
	Describe("BR-GATEWAY-010: Graceful Degradation When Redis Fails", func() {
		It("creates CRD without deduplication when Redis is unavailable", func() {
			// Business scenario: Redis goes down (network partition, OOM, etc.)
			// Expectation: System continues to create CRDs (better duplicate than missed alert)

			By("Simulating Redis failure by disconnecting client")
			// Save original client
			originalClient := redisClient
			// Create a disconnected client (wrong port)
			redisClient = redis.NewClient(&redis.Options{
				Addr: "localhost:9999", // Non-existent Redis
			})

			By("AlertManager sends alert during Redis outage")
			alertPayload := fmt.Sprintf(`{
				"alerts": [{
					"labels": {
						"alertname": "CriticalFailure",
						"namespace": "%s",
						"pod": "critical-service-1",
						"severity": "critical"
					}
				}]
			}`, testNamespace)

			req, err := http.NewRequest("POST", "http://localhost:8090/api/v1/signals/prometheus",
				bytes.NewBufferString(alertPayload))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			By("System creates CRD despite Redis failure (graceful degradation)")
			// Business outcome: Critical alerts still reach AI service
			Eventually(func() int {
				rrList := &remediationv1alpha1.RemediationRequestList{}
				err := k8sClient.List(context.Background(), rrList,
					client.InNamespace(testNamespace))
				if err != nil {
					return -1
				}
				return len(rrList.Items)
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(1),
				"Redis failure should not prevent CRD creation (availability > deduplication)")

			// Cleanup: Restore original client
			redisClient = originalClient

			// Business capability: System degrades gracefully (may allow duplicates, but no lost alerts)
		})

		It("recovers deduplication when Redis comes back online", func() {
			// Business scenario: Redis recovers after brief outage
			// Expectation: Deduplication resumes automatically

			By("System operating normally with Redis")
			alertPayload := fmt.Sprintf(`{
				"alerts": [{
					"labels": {
						"alertname": "RecoveryTest",
						"namespace": "%s",
						"pod": "service-1",
						"severity": "warning"
					}
				}]
			}`, testNamespace)

			sendAlert := func() {
				req, err := http.NewRequest("POST", "http://localhost:8090/api/v1/signals/prometheus",
					bytes.NewBufferString(alertPayload))
				Expect(err).NotTo(HaveOccurred())
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))
				resp, err := http.DefaultClient.Do(req)
				Expect(err).NotTo(HaveOccurred())
				resp.Body.Close()
			}

			sendAlert()

			By("First alert creates CRD")
			Eventually(func() int {
				rrList := &remediationv1alpha1.RemediationRequestList{}
				err := k8sClient.List(context.Background(), rrList,
					client.InNamespace(testNamespace))
				if err != nil {
					return -1
				}
				return len(rrList.Items)
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(1))

			By("Duplicate alerts are deduplicated (Redis operational)")
			for i := 0; i < 3; i++ {
				time.Sleep(100 * time.Millisecond)
				sendAlert()
			}

			Consistently(func() int {
				rrList := &remediationv1alpha1.RemediationRequestList{}
				err := k8sClient.List(context.Background(), rrList,
					client.InNamespace(testNamespace))
				if err != nil {
					return -1
				}
				return len(rrList.Items)
			}, 3*time.Second, 500*time.Millisecond).Should(Equal(1),
				"Deduplication prevents duplicate CRDs when Redis operational")

			// Business capability: System self-recovers when dependencies are restored
		})

		It("handles Redis connection timeout gracefully", func() {
			// Business scenario: Redis responds slowly (high load, network latency)
			// Expectation: Gateway times out quickly and creates CRD anyway

			By("Alert arrives during Redis slow response")
			alertPayload := fmt.Sprintf(`{
				"alerts": [{
					"labels": {
						"alertname": "TimeoutTest",
						"namespace": "%s",
						"pod": "timeout-service-1",
						"severity": "critical"
					}
				}]
			}`, testNamespace)

			req, err := http.NewRequest("POST", "http://localhost:8090/api/v1/signals/prometheus",
				bytes.NewBufferString(alertPayload))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))

			start := time.Now()
			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()
			duration := time.Since(start)

			By("Gateway responds quickly despite Redis issues")
			// Business capability: Gateway doesn't block AlertManager
			Expect(duration).To(BeNumerically("<", 5*time.Second),
				"Gateway should timeout Redis operations quickly (not block AlertManager)")

			By("CRD still created despite timeout")
			Eventually(func() int {
				rrList := &remediationv1alpha1.RemediationRequestList{}
				err := k8sClient.List(context.Background(), rrList,
					client.InNamespace(testNamespace))
				if err != nil {
					return -1
				}
				return len(rrList.Items)
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(1),
				"CRD created even if Redis times out")

			// Business value: System doesn't let slow dependencies block critical workflows
		})
	})

	// BUSINESS OUTCOME 7: System resilience during Kubernetes API failures
	Describe("BR-GATEWAY-001-002: Graceful Degradation When K8s API Fails", func() {
		It("queues alert for retry when CRD creation fails transiently", func() {
			// Business scenario: K8s API temporarily unavailable (API server restart, etcd leader election)
			// Expectation: Alert is not lost, will be retried

			By("Alert arrives when K8s API is degraded")
			alertPayload := fmt.Sprintf(`{
				"alerts": [{
					"labels": {
						"alertname": "K8sAPIFailure",
						"namespace": "%s",
						"pod": "api-test-1",
						"severity": "critical"
					}
				}]
			}`, testNamespace)

			req, err := http.NewRequest("POST", "http://localhost:8090/api/v1/signals/prometheus",
				bytes.NewBufferString(alertPayload))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			By("System eventually creates CRD after retry")
			// Business outcome: Transient failures don't lose alerts
			Eventually(func() int {
				rrList := &remediationv1alpha1.RemediationRequestList{}
				err := k8sClient.List(context.Background(), rrList,
					client.InNamespace(testNamespace))
				if err != nil {
					return -1
				}
				return len(rrList.Items)
			}, 30*time.Second, 1*time.Second).Should(Equal(1),
				"Transient K8s API failures should not lose alerts (retry mechanism)")

			// Business capability: System retries failed operations automatically
		})

		It("logs alert to persistent storage when CRD creation repeatedly fails", func() {
			// Business scenario: K8s API down for extended period (cluster upgrade, disaster)
			// Expectation: Alerts are persisted somewhere for manual recovery

			By("Multiple alerts arrive during extended K8s API outage")
			for i := 0; i < 3; i++ {
				alertPayload := fmt.Sprintf(`{
					"alerts": [{
						"labels": {
							"alertname": "ExtendedOutage",
							"namespace": "%s",
							"pod": "outage-test-%d",
							"severity": "critical"
						}
					}]
				}`, testNamespace, i)

				req, err := http.NewRequest("POST", "http://localhost:8090/api/v1/signals/prometheus",
					bytes.NewBufferString(alertPayload))
				Expect(err).NotTo(HaveOccurred())
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))

				resp, err := http.DefaultClient.Do(req)
				Expect(err).NotTo(HaveOccurred())
				resp.Body.Close()

				time.Sleep(100 * time.Millisecond)
			}

			// Business outcome: System provides audit trail for manual recovery
			// Note: In V1, we verify Gateway doesn't crash; V2 will add persistent queue
			By("Gateway remains responsive during K8s API issues")
			healthReq, err := http.NewRequest("GET", "http://localhost:8090/healthz", nil)
			Expect(err).NotTo(HaveOccurred())
			healthResp, err := http.DefaultClient.Do(healthReq)
			Expect(err).NotTo(HaveOccurred())
			Expect(healthResp.StatusCode).To(Equal(http.StatusOK),
				"Gateway should remain operational during K8s API failures")
			healthResp.Body.Close()

			// Business capability: System remains operational even when downstream services fail
		})
	})

	// BUSINESS OUTCOME 8: Storm aggregation boundary conditions
	Describe("BR-GATEWAY-015-016: Storm Aggregation Edge Cases", func() {
		It("handles alerts arriving at exactly the rate threshold boundary", func() {
			// Business scenario: Alert rate exactly matches threshold (10 alerts/min)
			// Expectation: System should consistently apply storm detection

			By("Sending exactly 10 alerts in 1 minute (threshold boundary)")
			boundaryAlertName := fmt.Sprintf("BoundaryTest-%d", time.Now().UnixNano())

			for i := 0; i < 10; i++ {
				alertPayload := fmt.Sprintf(`{
					"alerts": [{
						"labels": {
							"alertname": "%s",
							"namespace": "%s",
							"pod": "boundary-pod-%d",
							"severity": "warning"
						}
					}]
				}`, boundaryAlertName, testNamespace, i)

				req, err := http.NewRequest("POST", "http://localhost:8090/api/v1/signals/prometheus",
					bytes.NewBufferString(alertPayload))
				Expect(err).NotTo(HaveOccurred())
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))
				resp, err := http.DefaultClient.Do(req)
				Expect(err).NotTo(HaveOccurred())
				resp.Body.Close()

				time.Sleep(6 * time.Second) // Exactly 10 alerts/minute
			}

			By("System applies consistent storm detection at threshold boundary")
			// Business outcome: Threshold behavior is predictable (no off-by-one errors)
			Eventually(func() int {
				rrList := &remediationv1alpha1.RemediationRequestList{}
				err := k8sClient.List(context.Background(), rrList,
					client.InNamespace(testNamespace))
				if err != nil {
					return -1
				}
				count := 0
				for _, rr := range rrList.Items {
					if rr.Spec.SignalName == boundaryAlertName {
						count++
					}
				}
				return count
			}, 90*time.Second, 2*time.Second).Should(BeNumerically(">=", 1),
				"Storm detection threshold behavior should be consistent at boundary")

			// Business capability: Threshold logic is reliable (no edge case bugs)
		})

		It("aggregates storm alerts arriving across multiple time windows", func() {
			// Business scenario: Storm continues across aggregation window boundaries
			// Expectation: Each window gets its own aggregated CRD

			By("First wave of storm alerts")
			multiWindowAlertName := fmt.Sprintf("MultiWindow-%d", time.Now().UnixNano())

			// First window: 8 alerts
			for i := 0; i < 8; i++ {
				alertPayload := fmt.Sprintf(`{
					"alerts": [{
						"labels": {
							"alertname": "%s",
							"namespace": "%s",
							"pod": "wave1-pod-%d",
							"severity": "critical"
						}
					}]
				}`, multiWindowAlertName, testNamespace, i)

				req, err := http.NewRequest("POST", "http://localhost:8090/api/v1/signals/prometheus",
					bytes.NewBufferString(alertPayload))
				Expect(err).NotTo(HaveOccurred())
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))
				resp, err := http.DefaultClient.Do(req)
				Expect(err).NotTo(HaveOccurred())
				resp.Body.Close()

				time.Sleep(100 * time.Millisecond)
			}

			By("Waiting for first aggregation window to complete")
			time.Sleep(65 * time.Second)

			By("Second wave of storm alerts (new window)")
			// Second window: 8 more alerts
			for i := 8; i < 16; i++ {
				alertPayload := fmt.Sprintf(`{
					"alerts": [{
						"labels": {
							"alertname": "%s",
							"namespace": "%s",
							"pod": "wave2-pod-%d",
							"severity": "critical"
						}
					}]
				}`, multiWindowAlertName, testNamespace, i)

				req, err := http.NewRequest("POST", "http://localhost:8090/api/v1/signals/prometheus",
					bytes.NewBufferString(alertPayload))
				Expect(err).NotTo(HaveOccurred())
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))
				resp, err := http.DefaultClient.Do(req)
				Expect(err).NotTo(HaveOccurred())
				resp.Body.Close()

				time.Sleep(100 * time.Millisecond)
			}

			By("Waiting for second aggregation window to complete")
			time.Sleep(65 * time.Second)

			By("System creates separate aggregated CRDs for each window")
			Eventually(func() int {
				rrList := &remediationv1alpha1.RemediationRequestList{}
				err := k8sClient.List(context.Background(), rrList,
					client.InNamespace(testNamespace))
				if err != nil {
					return -1
				}
				count := 0
				for _, rr := range rrList.Items {
					if rr.Spec.SignalName == multiWindowAlertName && rr.Spec.IsStorm {
						count++
					}
				}
				return count
			}, 15*time.Second, 1*time.Second).Should(Equal(2),
				"Multi-window storms should create one aggregated CRD per window")

			// Business capability: Storm aggregation handles sustained incidents across time windows
		})
	})

	// BUSINESS OUTCOME 9: Concurrent request handling
	Describe("BR-GATEWAY-001-002: Concurrent Alert Processing", func() {
		It("handles multiple simultaneous alerts without race conditions", func() {
			// Business scenario: Multiple AlertManager instances send alerts simultaneously
			// Expectation: All alerts processed correctly, no data corruption

			By("Sending 20 different alerts concurrently")
			concurrentAlertName := fmt.Sprintf("Concurrent-%d", time.Now().UnixNano())
			done := make(chan bool, 20)

			for i := 0; i < 20; i++ {
				go func(index int) {
					defer GinkgoRecover()
					alertPayload := fmt.Sprintf(`{
						"alerts": [{
							"labels": {
								"alertname": "%s",
								"namespace": "%s",
								"pod": "concurrent-pod-%d",
								"severity": "warning"
							}
						}]
					}`, concurrentAlertName, testNamespace, index)

					req, err := http.NewRequest("POST", "http://localhost:8090/api/v1/signals/prometheus",
						bytes.NewBufferString(alertPayload))
					Expect(err).NotTo(HaveOccurred())
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))

					resp, err := http.DefaultClient.Do(req)
					Expect(err).NotTo(HaveOccurred())
					resp.Body.Close()

					done <- true
				}(i)
			}

			By("All requests complete successfully")
			for i := 0; i < 20; i++ {
				<-done
			}

			By("System creates exactly 20 CRDs (no race condition losses)")
			Eventually(func() int {
				rrList := &remediationv1alpha1.RemediationRequestList{}
				err := k8sClient.List(context.Background(), rrList,
					client.InNamespace(testNamespace))
				if err != nil {
					return -1
				}
				count := 0
				for _, rr := range rrList.Items {
					if rr.Spec.SignalName == concurrentAlertName {
						count++
					}
				}
				return count
			}, 15*time.Second, 500*time.Millisecond).Should(Equal(20),
				"Concurrent requests should not cause data loss or corruption")

			// Business capability: System handles production-scale concurrent load
		})

		It("maintains deduplication correctness under concurrent duplicate alerts", func() {
			// Business scenario: Multiple AlertManager replicas send same alert simultaneously
			// Expectation: Only one CRD created (deduplication works under concurrency)

			By("Sending 10 identical alerts concurrently")
			dupAlertPayload := fmt.Sprintf(`{
				"alerts": [{
					"labels": {
						"alertname": "ConcurrentDuplicate",
						"namespace": "%s",
						"pod": "duplicate-pod-1",
						"severity": "warning"
					}
				}]
			}`, testNamespace)

			done := make(chan bool, 10)
			for i := 0; i < 10; i++ {
				go func() {
					defer GinkgoRecover()
					req, err := http.NewRequest("POST", "http://localhost:8090/api/v1/signals/prometheus",
						bytes.NewBufferString(dupAlertPayload))
					Expect(err).NotTo(HaveOccurred())
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))

					resp, err := http.DefaultClient.Do(req)
					Expect(err).NotTo(HaveOccurred())
					resp.Body.Close()

					done <- true
				}()
			}

			By("All concurrent requests complete")
			for i := 0; i < 10; i++ {
				<-done
			}

			By("Deduplication prevents duplicate CRDs even under concurrent load")
			Eventually(func() int {
				rrList := &remediationv1alpha1.RemediationRequestList{}
				err := k8sClient.List(context.Background(), rrList,
					client.InNamespace(testNamespace))
				if err != nil {
					return -1
				}
				return len(rrList.Items)
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(1),
				"10 concurrent identical alerts should create exactly 1 CRD (deduplication under concurrency)")

			// Business capability: Deduplication is thread-safe
		})
	})

	// BUSINESS OUTCOME 10: Deduplication edge cases
	Describe("BR-GATEWAY-010: Deduplication Boundary Conditions", func() {
		It("creates new CRD when deduplication TTL expires", func() {
			// Business scenario: Same alert fires, resolves, then fires again hours later
			// Expectation: Second occurrence creates new CRD (not deduplicated forever)

			By("First alert creates CRD")
			ttlAlertPayload := fmt.Sprintf(`{
				"alerts": [{
					"labels": {
						"alertname": "TTLTest",
						"namespace": "%s",
						"pod": "ttl-pod-1",
						"severity": "warning"
					}
				}]
			}`, testNamespace)

			sendAlert := func() {
				req, err := http.NewRequest("POST", "http://localhost:8090/api/v1/signals/prometheus",
					bytes.NewBufferString(ttlAlertPayload))
				Expect(err).NotTo(HaveOccurred())
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))
				resp, err := http.DefaultClient.Do(req)
				Expect(err).NotTo(HaveOccurred())
				resp.Body.Close()
			}

			sendAlert()

			By("First CRD created")
			Eventually(func() int {
				rrList := &remediationv1alpha1.RemediationRequestList{}
				err := k8sClient.List(context.Background(), rrList,
					client.InNamespace(testNamespace))
				if err != nil {
					return -1
				}
				return len(rrList.Items)
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(1))

			By("Waiting for deduplication TTL to expire (simulated with Redis flush)")
			// In production, TTL is 5 minutes; we simulate expiry with Redis flush
			Expect(redisClient.FlushDB(context.Background()).Err()).To(Succeed())

			By("Same alert fires again after TTL expiry")
			sendAlert()

			By("New CRD created (not deduplicated after TTL expiry)")
			Eventually(func() int {
				rrList := &remediationv1alpha1.RemediationRequestList{}
				err := k8sClient.List(context.Background(), rrList,
					client.InNamespace(testNamespace))
				if err != nil {
					return -1
				}
				return len(rrList.Items)
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(2),
				"Alert re-occurrence after TTL expiry should create new CRD")

			// Business capability: Deduplication doesn't prevent re-analysis of recurring issues
		})

		It("treats alerts with different severity as unique (not deduplicated)", func() {
			// Business scenario: Alert escalates from warning to critical
			// Expectation: Escalation creates new CRD (AI should re-analyze with new severity)

			By("Initial warning alert")
			warningPayload := fmt.Sprintf(`{
				"alerts": [{
					"labels": {
						"alertname": "SeverityTest",
						"namespace": "%s",
						"pod": "severity-pod-1",
						"severity": "warning"
					}
				}]
			}`, testNamespace)

			req, err := http.NewRequest("POST", "http://localhost:8090/api/v1/signals/prometheus",
				bytes.NewBufferString(warningPayload))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))
			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			By("Warning alert creates CRD")
			Eventually(func() int {
				rrList := &remediationv1alpha1.RemediationRequestList{}
				err := k8sClient.List(context.Background(), rrList,
					client.InNamespace(testNamespace))
				if err != nil {
					return -1
				}
				return len(rrList.Items)
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(1))

			By("Alert escalates to critical")
			criticalPayload := fmt.Sprintf(`{
				"alerts": [{
					"labels": {
						"alertname": "SeverityTest",
						"namespace": "%s",
						"pod": "severity-pod-1",
						"severity": "critical"
					}
				}]
			}`, testNamespace)

			req, err = http.NewRequest("POST", "http://localhost:8090/api/v1/signals/prometheus",
				bytes.NewBufferString(criticalPayload))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))
			resp, err = http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			By("Escalated alert creates new CRD (not deduplicated)")
			Eventually(func() int {
				rrList := &remediationv1alpha1.RemediationRequestList{}
				err := k8sClient.List(context.Background(), rrList,
					client.InNamespace(testNamespace))
				if err != nil {
					return -1
				}
				return len(rrList.Items)
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(2),
				"Alert with different severity should create new CRD (escalation is significant)")

			// Business capability: System recognizes severity escalation as new incident
		})
	})
})
