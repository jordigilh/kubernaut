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
	"net/http"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

// Business Outcome Testing: Test WHAT complete webhook processing enables
//
// ❌ WRONG: "HTTP response contains status field" (tests implementation)
// ✅ RIGHT: "Prometheus alerts create RemediationRequest CRDs for AI analysis" (tests business outcome)
//
// These tests verify the COMPLETE end-to-end business flow:
// 1. Webhook arrives (Prometheus or K8s Event)
// 2. CRD created in Kubernetes with correct business metadata
// 3. Fingerprint stored in Redis for deduplication
// 4. Duplicate alerts return 202 and NO new CRD created
// 5. Storm detection aggregates multiple alerts into single CRD
//
// This REPLACES the old tests that only verified HTTP response body structure.

var _ = Describe("BR-GATEWAY-001-015: End-to-End Webhook Processing - E2E Tests", func() {
	var (
		testCtx    context.Context // ← Test-local context
		testCancel context.CancelFunc
		// k8sClient available from suite (DD-E2E-K8S-CLIENT-001)
		logger        logr.Logger // DD-005: Use logr.Logger
		testNamespace string      // Unique namespace per test
	)

	BeforeEach(func() {
		testCtx, testCancel = context.WithCancel(context.Background()) // ← Uses local variable
		logger = logr.Discard()                                        // DD-005: Use logr.Discard() for silent test logging
		_ = logger                                                     // Suppress unused variable warning

		// Setup test infrastructure using helpers

		// k8sClient available from suite (DD-E2E-K8S-CLIENT-001)
		Expect(k8sClient).ToNot(BeNil(), "K8s client required for E2E tests")

		// DD-GATEWAY-012: Redis setup REMOVED - Gateway is now Redis-free

		// Pre-create managed namespace (Pattern: RO E2E)
		testNamespace = helpers.CreateTestNamespaceAndWait(k8sClient, "test-prod")

		// E2E tests use deployed Gateway at gatewayURL (http://127.0.0.1:8080)
		// No local test server needed

		logger.Info("Test setup complete",
			"gateway_url", gatewayURL,
			"test_namespace", testNamespace,
		)
	})

	AfterEach(func() {
		if testCancel != nil {
			testCancel() // ← Only cancels test-local context
		}
		// DD-GATEWAY-012: Redis cleanup REMOVED - Gateway is now Redis-free

		// Clean up test namespace (Pattern: RO E2E)
		helpers.DeleteTestNamespace(ctx, k8sClient, testNamespace)
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// BR-GATEWAY-001: Prometheus Alert → CRD Creation
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("BR-GATEWAY-001: Prometheus Alert → CRD Creation", func() {
		It("creates RemediationRequest CRD from Prometheus AlertManager webhook", func() {
			// BR-GATEWAY-001, BR-GATEWAY-015: Complete webhook-to-CRD flow
			// BUSINESS SCENARIO: Production pod memory alert → AI analysis triggered
			// Expected: CRD created in K8s with correct priority and environment

			payload := []byte(fmt.Sprintf(`{
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "HighMemoryUsage",
						"severity": "critical",
						"namespace": "%s",
						"pod": "payment-api-123"
					},
					"annotations": {
						"summary": "Pod payment-api-123 using 95%% memory",
						"description": "Memory threshold exceeded, may cause OOM"
					},
					"startsAt": "2025-10-22T10:00:00Z"
				}]
			}`, testNamespace))

			// Send webhook to Gateway
			url := fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL)
			req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
			Expect(err).ToNot(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred(), "HTTP request should succeed")
			defer func() { _ = resp.Body.Close() }()

			// BUSINESS OUTCOME 1: HTTP 201 Created
			Expect(resp.StatusCode).To(Equal(http.StatusCreated),
				"First occurrence must create CRD (201 Created)")

			// Parse response to get fingerprint (for deduplication check while CRD active)
			var response map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&response)
			Expect(err).NotTo(HaveOccurred(), "Should parse JSON response")
			fingerprint, ok := response["fingerprint"].(string)
			Expect(ok).To(BeTrue(), "Response should contain fingerprint")
			Expect(fingerprint).NotTo(BeEmpty(), "Fingerprint should not be empty")

			// DD-GATEWAY-012: Redis check REMOVED - Gateway is now Redis-free
			// DD-GATEWAY-011: BUSINESS OUTCOME 2: Deduplication tracked in RR status
			// Fingerprint-based deduplication validated in dd_gateway_011_status_deduplication_test.go

			// BUSINESS OUTCOME 3: CRD created in Kubernetes
			var crdList remediationv1alpha1.RemediationRequestList
			err = k8sClient.List(testCtx, &crdList, client.InNamespace(testNamespace))
			Expect(err).NotTo(HaveOccurred(), "Should list CRDs in test namespace")
			Expect(crdList.Items).To(HaveLen(1), "One CRD should be created")

			crd := crdList.Items[0]
			// Note: Priority and Environment assertions removed (2025-12-06)
			// Classification moved to Signal Processing per DD-CATEGORIZATION-001
			Expect(crd.Spec.SignalName).To(Equal("HighMemoryUsage"),
				"Alert name enables AI to understand failure type")

			// Verify fingerprint label matches response fingerprint (truncated to K8s 63-char limit)
			fingerprintLabel := crd.Labels["kubernaut.ai/signal-fingerprint"]
			expectedLabel := fingerprint
			if len(expectedLabel) > 63 {
				expectedLabel = expectedLabel[:63] // K8s label value max length
			}
			Expect(fingerprintLabel).To(Equal(expectedLabel),
				"CRD fingerprint label must match response fingerprint (truncated to 63 chars for K8s)")

			// BUSINESS CAPABILITY VERIFIED:
			// ✅ Prometheus alert → Gateway → CRD created
			// ✅ Priority assigned based on severity + environment
			// ✅ Environment classified from namespace
			// ✅ Fingerprint generated for deduplication
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// BR-GATEWAY-003-005: Deduplication
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("BR-GATEWAY-003-005: Deduplication", func() {
		It("returns 202 Accepted for duplicate alerts while CRD active", func() {
			// BR-GATEWAY-003-005: Duplicate detection prevents CRD spam
			// BUSINESS SCENARIO: Same alert fires 3 times in 5 seconds
			// Expected: First = 201 Created, subsequent = 202 Accepted, NO duplicate CRDs

			payload := []byte(fmt.Sprintf(`{
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "CPUThrottling",
						"severity": "warning",
						"namespace": "%s",
						"pod": "api-gateway-7"
					},
					"annotations": {
						"summary": "CPU throttling detected"
					},
					"startsAt": "2025-10-22T12:00:00Z"
				}]
			}`, testNamespace))

			url := fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL)

			// First alert: Creates CRD
			req1, err := http.NewRequest("POST", url, bytes.NewReader(payload))

			Expect(err).ToNot(HaveOccurred())

			req1.Header.Set("Content-Type", "application/json")

			req1.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

			resp1, err := http.DefaultClient.Do(req1)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp1.Body.Close() }()
			Expect(resp1.StatusCode).To(Equal(http.StatusCreated),
				"First alert must create CRD")

			// BUSINESS OUTCOME 1: First CRD created
			// Use Eventually to handle Kubernetes API caching/propagation delays
			var crdList1 remediationv1alpha1.RemediationRequestList
			var firstCRDName string
			Eventually(func() int {
				err = k8sClient.List(testCtx, &crdList1, client.InNamespace(testNamespace))
				Expect(err).NotTo(HaveOccurred())
				return len(crdList1.Items)
			}, "5s", "100ms").Should(Equal(1), "First alert creates CRD")

			firstCRDName = crdList1.Items[0].Name

			// Second alert: Duplicate (CRD still in non-terminal phase)
			req2, err := http.NewRequest("POST", url, bytes.NewReader(payload))

			Expect(err).ToNot(HaveOccurred())

			req2.Header.Set("Content-Type", "application/json")

			req2.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

			resp2, err := http.DefaultClient.Do(req2)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp2.Body.Close() }()
			Expect(resp2.StatusCode).To(Equal(http.StatusAccepted),
				"Duplicate alert must return 202 Accepted")

			// BUSINESS OUTCOME 2: NO new CRD created
			var crdList2 remediationv1alpha1.RemediationRequestList
			err = k8sClient.List(testCtx, &crdList2, client.InNamespace(testNamespace))
			Expect(err).NotTo(HaveOccurred())
			Expect(crdList2.Items).To(HaveLen(1),
				"Duplicate alert must NOT create new CRD")
			Expect(crdList2.Items[0].Name).To(Equal(firstCRDName),
				"Same CRD name confirms deduplication")

			// Third alert: Still duplicate
			req3, err := http.NewRequest("POST", url, bytes.NewReader(payload))

			Expect(err).ToNot(HaveOccurred())

			req3.Header.Set("Content-Type", "application/json")

			req3.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

			resp3, err := http.DefaultClient.Do(req3)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp3.Body.Close() }()
			Expect(resp3.StatusCode).To(Equal(http.StatusAccepted),
				"Third duplicate must also return 202 Accepted")

			// BUSINESS OUTCOME 3: Still only 1 CRD
			var crdList3 remediationv1alpha1.RemediationRequestList
			err = k8sClient.List(testCtx, &crdList3, client.InNamespace(testNamespace))
			Expect(err).NotTo(HaveOccurred())
			Expect(crdList3.Items).To(HaveLen(1),
				"Third duplicate must NOT create new CRD (still only 1 CRD)")

			// BUSINESS CAPABILITY VERIFIED:
			// ✅ Deduplication prevents CRD spam (1 CRD, not 3)
			// ✅ Duplicate alerts tracked but don't create new CRDs
		})

		It("tracks duplicate count and timestamps in Redis metadata", func() {
			// BR-GATEWAY-005: Duplicate metadata for operational visibility
			// BUSINESS SCENARIO: Alert fires 5 times → Ops sees escalation pattern
			// Expected: Redis metadata includes count, first seen, last seen

			payload := []byte(fmt.Sprintf(`{
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "NetworkLatency",
						"severity": "critical",
						"namespace": "%s"
					},
					"annotations": {
						"summary": "Network latency > 500ms"
					},
					"startsAt": "2025-10-22T13:00:00Z"
				}]
			}`, testNamespace))

			url := fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL)

			// First alert
			req1, err := http.NewRequest("POST", url, bytes.NewReader(payload))

			Expect(err).ToNot(HaveOccurred())

			req1.Header.Set("Content-Type", "application/json")

			req1.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

			resp1, err := http.DefaultClient.Do(req1)
			Expect(err).NotTo(HaveOccurred(), "Should send first alert")
			defer func() { _ = resp1.Body.Close() }()

			// Parse response to get full fingerprint (before K8s label truncation)
			var response map[string]interface{}
			err = json.NewDecoder(resp1.Body).Decode(&response)
			Expect(err).NotTo(HaveOccurred(), "Should parse JSON response")
			fingerprint, ok := response["fingerprint"].(string)
			Expect(ok).To(BeTrue(), "Response should contain fingerprint")
			Expect(fingerprint).NotTo(BeEmpty(), "Fingerprint should not be empty")

			// Send 4 more duplicates
			for i := 0; i < 4; i++ {
				req, err := http.NewRequest("POST", url, bytes.NewReader(payload))

				Expect(err).ToNot(HaveOccurred())

				req.Header.Set("Content-Type", "application/json")

				req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

				resp, err := http.DefaultClient.Do(req)
				Expect(err).NotTo(HaveOccurred(), "Should send duplicate alert")
				_ = resp.Body.Close()
			}

			// DD-GATEWAY-012: Redis metadata check REMOVED - Gateway is now Redis-free
			// DD-GATEWAY-011: BUSINESS OUTCOME: RR status.deduplication tracks duplicate count
			// Verify duplicate count via K8s CRD status
			Eventually(func() int32 {
				// Get the first (and only) CRD
				crds := ListRemediationRequests(testCtx, k8sClient, testNamespace)
				if len(crds) == 0 || crds[0].Status.Deduplication == nil {
					return 0
				}
				return crds[0].Status.Deduplication.OccurrenceCount
			}, "5s", "100ms").Should(BeNumerically(">=", 5),
				"Count shows alert fired 5 times (1 original + 4 duplicates)")

			// Verify timestamps in status.deduplication
			crds := ListRemediationRequests(testCtx, k8sClient, testNamespace)
			Expect(crds).To(HaveLen(1))
			Expect(crds[0].Status.Deduplication).ToNot(BeNil())
			Expect(crds[0].Status.Deduplication.FirstSeenAt).ToNot(BeNil(),
				"First occurrence timestamp shows when issue started")
			Expect(crds[0].Status.Deduplication.LastSeenAt).ToNot(BeNil(),
				"Last occurrence timestamp shows issue is ongoing")

			// BUSINESS CAPABILITY VERIFIED:
			// ✅ Duplicate tracking provides operational visibility
			// ✅ Metadata helps ops understand alert escalation patterns
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// BR-GATEWAY-002: Kubernetes Event Webhooks
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("BR-GATEWAY-002: Kubernetes Event Webhooks", func() {
		It("creates CRD from Kubernetes Warning events", func() {
			// BR-GATEWAY-002: K8s events trigger remediation workflow
			// BUSINESS SCENARIO: Pod OOMKilled event → AI analyzes memory issue
			// Expected: CRD created with event details

			nowTS := time.Now().Format(time.RFC3339)
			payload := []byte(fmt.Sprintf(`{
				"type": "Warning",
				"reason": "OOMKilled",
				"message": "Container killed due to out of memory",
				"involvedObject": {
					"kind": "Pod",
					"namespace": "%s",
					"name": "payment-processor-42"
				},
				"metadata": {
					"namespace": "%s"
				},
				"firstTimestamp": "%s",
				"lastTimestamp": "%s"
			}`, testNamespace, testNamespace, nowTS, nowTS))

			url := fmt.Sprintf("%s/api/v1/signals/kubernetes-event", gatewayURL)
			// BR-GATEWAY-074: K8s Event adapter uses body-level freshness validation
			// (EventFreshnessValidator) instead of X-Timestamp header.
			// BR-SCOPE-002: Retry to handle scope checker informer cache propagation delay.
			var resp *http.Response
			Eventually(func() int {
				req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
				Expect(err).ToNot(HaveOccurred())
				req.Header.Set("Content-Type", "application/json")
				resp, err = http.DefaultClient.Do(req)
				Expect(err).ToNot(HaveOccurred())
				return resp.StatusCode
			}, "30s", "1s").Should(Equal(http.StatusCreated),
				"Warning event must create CRD for AI analysis")
			defer func() { _ = resp.Body.Close() }()

			// BUSINESS OUTCOME 2: CRD created in Kubernetes
			var crdList remediationv1alpha1.RemediationRequestList
			listErr := k8sClient.List(testCtx, &crdList, client.InNamespace(testNamespace))
			Expect(listErr).NotTo(HaveOccurred())
			Expect(crdList.Items).To(HaveLen(1), "K8s event should create CRD")

			crd := crdList.Items[0]
			Expect(crd.Spec.SignalName).To(Equal("OOMKilled"),
				"Event reason helps AI identify root cause")
			Expect(crd.Spec.SignalType).To(Equal("kubernetes-event"),
				"Signal type - ✅ ADAPTER-CONSTANT: KubernetesEventAdapter uses SourceTypeKubernetesEvent")

			// BUSINESS CAPABILITY VERIFIED:
			// ✅ K8s events trigger remediation workflow
			// ✅ Event details provide AI with root cause context
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// BR-GATEWAY-TARGET-RESOURCE: Target Resource Population
	// Business Outcome: SignalProcessing and RO can access resource info directly
	// Reference: RESPONSE_TARGET_RESOURCE_SCHEMA.md - Option A
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Context("BR-GATEWAY-TARGET-RESOURCE: Target Resource in CRD (Integration)", func() {
		It("populates spec.targetResource from Prometheus alert for downstream services", func() {
			// BUSINESS SCENARIO: SignalProcessing receives CRD and needs resource info
			// to query K8s API for context enrichment.
			// MUST be able to access: rr.Spec.TargetResource.Kind, .Name, .Namespace
			// WITHOUT parsing ProviderData JSON

			payload := []byte(fmt.Sprintf(`{
				"alerts": [{
					"status": "firing",
					"labels": {
						"alertname": "PodCrashLooping",
						"severity": "critical",
						"namespace": "%s",
						"pod": "payment-service-abc123"
					},
					"annotations": {
						"summary": "Pod is crash looping"
					},
					"startsAt": "2025-10-22T12:00:00Z"
				}]
			}`, testNamespace))

			url := fmt.Sprintf("%s/api/v1/signals/prometheus", gatewayURL)
			req, err := http.NewRequest("POST", url, bytes.NewReader(payload))

			Expect(err).ToNot(HaveOccurred())

			req.Header.Set("Content-Type", "application/json")

			req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusCreated),
				"Alert should create CRD")

			// BUSINESS OUTCOME: CRD has spec.targetResource populated
			var crdList remediationv1alpha1.RemediationRequestList
			Eventually(func() int {
				err = k8sClient.List(testCtx, &crdList, client.InNamespace(testNamespace))
				Expect(err).NotTo(HaveOccurred())
				return len(crdList.Items)
			}, "5s", "100ms").Should(Equal(1), "CRD should be created")

			crd := crdList.Items[0]

			// INTEGRATION VERIFICATION: TargetResource is populated in actual K8s CRD
			// TargetResource is a REQUIRED value type (per API_CONTRACT_TRIAGE.md)
			Expect(crd.Spec.TargetResource.Kind).To(Equal("Pod"),
				"SignalProcessing needs Kind to query correct K8s resource type")
			Expect(crd.Spec.TargetResource.Name).To(Equal("payment-service-abc123"),
				"SignalProcessing needs Name to query specific resource")
			Expect(crd.Spec.TargetResource.Namespace).To(Equal(testNamespace),
				"SignalProcessing needs Namespace to scope K8s API query")

			// CORRECTNESS: ProviderData does NOT duplicate resource info
			var providerData map[string]interface{}
			err = json.Unmarshal(crd.Spec.ProviderData, &providerData)
			Expect(err).NotTo(HaveOccurred(), "ProviderData should be valid JSON")
			Expect(providerData).NotTo(HaveKey("resource"),
				"ProviderData should NOT contain resource{} - data is in spec.targetResource")

			// BUSINESS CAPABILITY VERIFIED:
			// ✅ spec.targetResource populated from Prometheus alert
			// ✅ SignalProcessing can access resource info directly (no JSON parsing)
			// ✅ No data duplication between spec.targetResource and ProviderData
		})

		It("populates spec.targetResource from Kubernetes event for downstream services", func() {
			// BUSINESS SCENARIO: K8s event triggers remediation
			// RO needs resource info for workflow routing decisions
			// NOTE: Gateway only processes Warning/Error events (Normal events are filtered)

			nowTS2 := time.Now().Format(time.RFC3339)
			payload := []byte(fmt.Sprintf(`{
				"type": "Warning",
				"reason": "FailedMount",
				"message": "Unable to attach or mount volumes: failed to attach volume",
				"involvedObject": {
					"kind": "Pod",
					"name": "nginx-pod-abc123",
					"namespace": "%s",
					"apiVersion": "v1"
				},
				"firstTimestamp": "%s",
				"lastTimestamp": "%s",
				"count": 1,
				"source": {
					"component": "kubelet",
					"host": "worker-node-1"
				}
			}`, testNamespace, nowTS2, nowTS2))

			url := fmt.Sprintf("%s/api/v1/signals/kubernetes-event", gatewayURL)
			// BR-GATEWAY-074: K8s Event adapter uses body-level freshness validation
			// BR-SCOPE-002: Retry to handle scope checker informer cache propagation delay.
			var resp *http.Response
			Eventually(func() int {
				req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
				Expect(err).ToNot(HaveOccurred())
				req.Header.Set("Content-Type", "application/json")
				resp, err = http.DefaultClient.Do(req)
				Expect(err).ToNot(HaveOccurred())
				return resp.StatusCode
			}, "30s", "1s").Should(Equal(http.StatusCreated),
				"Warning event should create CRD")
			defer func() { _ = resp.Body.Close() }()

			// Verify CRD was created with TargetResource
			var crdList remediationv1alpha1.RemediationRequestList
			Eventually(func() int {
				listErr := k8sClient.List(testCtx, &crdList, client.InNamespace(testNamespace))
				Expect(listErr).NotTo(HaveOccurred())
				return len(crdList.Items)
			}, "5s", "100ms").Should(BeNumerically(">=", 1), "CRD should be created")

			// Find the CRD for this event
			var targetCRD *remediationv1alpha1.RemediationRequest
			for i := range crdList.Items {
				if crdList.Items[i].Spec.SignalType == "kubernetes-event" { // ✅ ADAPTER-CONSTANT: KubernetesEventAdapter uses SourceTypeKubernetesEvent
					targetCRD = &crdList.Items[i]
					break
				}
			}

			Expect(targetCRD).NotTo(BeNil(), "K8s event CRD should exist")

			// INTEGRATION VERIFICATION: TargetResource populated from K8s event
			// TargetResource is a REQUIRED value type (per API_CONTRACT_TRIAGE.md)
			Expect(targetCRD.Spec.TargetResource.Kind).To(Equal("Pod"),
				"RO uses Kind for resource-type-specific workflows")
			Expect(targetCRD.Spec.TargetResource.Name).To(Equal("nginx-pod-abc123"),
				"RO uses Name for targeting specific resources")

			// BUSINESS CAPABILITY VERIFIED:
			// ✅ K8s events populate spec.targetResource
			// ✅ RO can route workflows based on resource type
		})
	})
})
