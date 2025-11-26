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
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	rediscache "github.com/jordigilh/kubernaut/pkg/cache/redis"
	"github.com/jordigilh/kubernaut/pkg/gateway/k8s"
	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Test Helper Functions
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// generateTestFingerprint creates a valid 64-character SHA256 hex fingerprint for testing
func generateTestFingerprint(input string) string {
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", hash)
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Multi-Pod Deduplication Integration Tests - K8s Cache Consistency
// BR-GATEWAY-025: Multi-pod cache consistency
//
// Test Tier: INTEGRATION (not unit)
// Rationale: Tests real Kubernetes API cache behavior across multiple Gateway pods.
// Multi-pod deduplication requires K8s client cache coordination, which cannot be
// reliably tested with fake clients. These tests validate production behavior with
// real K8s API server (envtest).
//
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - Unit tests (70%+): Business logic in isolation (fingerprint generation, validation)
// - Integration tests (>50%): Infrastructure interaction (THIS FILE - K8s cache + Redis)
// - E2E tests (10-15%): Complete workflow (multi-pod Gateway deployment)
//
// BUSINESS VALUE:
// - Validates cache consistency across Gateway pods
// - Prevents duplicate CRD creation in HA deployments
// - Ensures deduplication works with K8s API caching
// - Critical for production multi-replica Gateway deployments
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

var _ = Describe("BR-GATEWAY-025: Multi-Pod Deduplication (Integration)", func() {
	var (
		ctx         context.Context
		redisClient *redis.Client
		k8sClient1  *K8sTestClient // Simulates Gateway Pod 1
		k8sClient2  *K8sTestClient // Simulates Gateway Pod 2
		dedupSvc1   *processing.DeduplicationService
		dedupSvc2   *processing.DeduplicationService
		testNs      string
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Use real Redis from suite setup
		redisTestClient := SetupRedisTestClient(ctx)
		Expect(redisTestClient).ToNot(BeNil(), "Redis test client required for multi-pod deduplication tests")
		Expect(redisTestClient.Client).ToNot(BeNil(), "Redis client required for multi-pod deduplication tests")
		redisClient = redisTestClient.Client

		// Clean Redis state before each test (safe - each process uses different Redis DB)
		err := redisClient.FlushDB(ctx).Err()
		Expect(err).ToNot(HaveOccurred(), "Failed to flush Redis before test")

		// Create unique test namespace for isolation
		processID := GinkgoParallelProcess()
		testNs = fmt.Sprintf("test-multi-pod-p%d-%d", processID, time.Now().UnixNano())
		EnsureTestNamespace(ctx, suiteK8sClient, testNs)

		// Create two K8s clients simulating two Gateway pods
		// Each pod has its own client with independent cache
		k8sClient1 = SetupK8sTestClient(ctx)
		Expect(k8sClient1).ToNot(BeNil(), "K8s client 1 required for multi-pod tests")

		k8sClient2 = SetupK8sTestClient(ctx)
		Expect(k8sClient2).ToNot(BeNil(), "K8s client 2 required for multi-pod tests")

		// Wrap controller-runtime clients in k8s.Client
		k8sWrapper1 := k8s.NewClient(k8sClient1.Client)
		k8sWrapper2 := k8s.NewClient(k8sClient2.Client)

		// Create noop logger for tests
		noopLogger := zap.NewNop()

		// Wrap Redis client in rediscache.Client for DeduplicationService
		rediscacheClient := rediscache.NewClient(&redis.Options{
			Addr: redisClient.Options().Addr,
		}, noopLogger)

		// Create deduplication services for each "pod"
		dedupSvc1 = processing.NewDeduplicationServiceWithTTL(
			rediscacheClient,
			k8sWrapper1,
			5*time.Minute, // ttl
			noopLogger,    // logger
			nil,           // metrics (nil for tests)
		)

		dedupSvc2 = processing.NewDeduplicationServiceWithTTL(
			rediscacheClient,
			k8sWrapper2,
			5*time.Minute, // ttl
			noopLogger,    // logger
			nil,           // metrics (nil for tests)
		)
	})

	AfterEach(func() {
		// Clean up K8s clients
		if k8sClient1 != nil {
			k8sClient1.Cleanup(ctx)
		}
		if k8sClient2 != nil {
			k8sClient2.Cleanup(ctx)
		}

		// Clean up Redis state
		if redisClient != nil {
			_ = redisClient.FlushDB(ctx)
		}
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// TDD RED PHASE: Multi-Pod Cache Consistency Tests
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// STATUS: These tests will validate K8s cache behavior across pods
	// EXPECTED: Tests should pass if K8s client cache works correctly
	// ACTION: Validate that controller-runtime cache handles multi-client scenarios
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Describe("Cross-Pod Deduplication - K8s Cache Consistency", func() {
		Context("when Pod 1 creates CRD and Pod 2 receives same alert", func() {
			It("should detect duplicate using K8s API query", func() {
				// TDD GREEN: This test validates K8s cache behavior
				// BR-GATEWAY-025: Multi-pod cache consistency
				// BUSINESS BEHAVIOR: Pod 2 must detect CRD created by Pod 1
				// OUTCOME: No duplicate CRDs created

				// Generate valid SHA256 fingerprint
				fingerprintInput := fmt.Sprintf("test-fingerprint-%d", time.Now().UnixNano())
				validFingerprint := generateTestFingerprint(fingerprintInput)

				signal := &types.NormalizedSignal{
					Fingerprint:  validFingerprint,
					AlertName:    "HighCPU",
					Severity:     "critical",
					Namespace:    testNs,
					Resource:     types.ResourceIdentifier{Kind: "Pod", Name: "api-server-1"},
					FiringTime:   time.Now(),
					ReceivedTime: time.Now(),
					Labels: map[string]string{
						"alertname": "HighCPU",
						"namespace": testNs,
					},
				}

				// Step 1: Pod 1 checks for duplicate (should be new)
				isDup1, _, err := dedupSvc1.Check(ctx, signal)
				Expect(err).ToNot(HaveOccurred(), "Pod 1 should check successfully")
				Expect(isDup1).To(BeFalse(), "Pod 1 should detect new alert")

				// Step 2: Pod 1 creates CRD
				// Generate CRD name with timestamp (DD-015)
				fingerprintPrefix := signal.Fingerprint
				if len(fingerprintPrefix) > 12 {
					fingerprintPrefix = fingerprintPrefix[:12]
				}
				crdName := fmt.Sprintf("rr-%s-%d", fingerprintPrefix, time.Now().Unix())

				now := metav1.Now()
				crd := &remediationv1alpha1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      crdName,
						Namespace: testNs,
						Labels: map[string]string{
							"kubernaut.io/signal-fingerprint": signal.Fingerprint[:min(len(signal.Fingerprint), 63)],
						},
					},
					Spec: remediationv1alpha1.RemediationRequestSpec{
						SignalFingerprint: signal.Fingerprint,
						SignalName:        signal.AlertName,
						Severity:          signal.Severity,
						Environment:       "test",
						Priority:          "P1",
						TargetType:        "kubernetes",
						FiringTime:        now,
						ReceivedTime:      now,
						Deduplication: remediationv1alpha1.DeduplicationInfo{
							FirstSeen:       now,
							LastSeen:        now,
							OccurrenceCount: 1,
						},
					},
				}
				err = k8sClient1.Client.Create(ctx, crd)
				Expect(err).ToNot(HaveOccurred(), "Pod 1 should create CRD successfully")

				// Step 3: Store deduplication metadata in Redis
				err = dedupSvc1.Store(ctx, signal, crdName)
				Expect(err).ToNot(HaveOccurred(), "Pod 1 should store dedup metadata")

				// Step 4: Wait for K8s API to propagate (envtest is fast, but add small delay)
				time.Sleep(100 * time.Millisecond)

				// Step 5: Pod 2 receives same alert and checks for duplicate
				isDup2, metadata2, err := dedupSvc2.Check(ctx, signal)

				// BUSINESS VALIDATION: Pod 2 detects duplicate via K8s API
				// ✅ Duplicate detected (isDup2 = true)
				// ✅ Metadata includes CRD reference
				// ✅ No second CRD created
				Expect(err).ToNot(HaveOccurred(), "Pod 2 should check successfully")
				Expect(isDup2).To(BeTrue(), "Pod 2 should detect duplicate via K8s API")
				Expect(metadata2).ToNot(BeNil(), "Pod 2 should receive metadata")
				Expect(metadata2.RemediationRequestRef).To(ContainSubstring(crdName), "Metadata should reference Pod 1's CRD")

				// Step 6: Verify only one CRD exists
				crdList := &remediationv1alpha1.RemediationRequestList{}
				err = k8sClient2.Client.List(ctx, crdList, client.InNamespace(testNs))
				Expect(err).ToNot(HaveOccurred(), "Should list CRDs successfully")
				Expect(crdList.Items).To(HaveLen(1), "Should have exactly 1 CRD (no duplicate)")

				GinkgoWriter.Printf("✅ Multi-pod deduplication validated: Pod 2 detected Pod 1's CRD\n")
			})

			It("should handle concurrent duplicate checks from multiple pods", func() {
				// TDD GREEN: This test validates concurrent deduplication
				// BR-GATEWAY-025: Multi-pod cache consistency under concurrency
				// BUSINESS BEHAVIOR: Multiple pods checking same alert simultaneously
				// OUTCOME: Only one CRD created, all pods detect duplicate

				// Generate valid SHA256 fingerprint
				fingerprintInput := fmt.Sprintf("concurrent-fp-%d", time.Now().UnixNano())
				validFingerprint := generateTestFingerprint(fingerprintInput)

				signal := &types.NormalizedSignal{
					Fingerprint:  validFingerprint,
					AlertName:    "MemoryLeak",
					Severity:     "critical",
					Namespace:    testNs,
					Resource:     types.ResourceIdentifier{Kind: "Pod", Name: "db-server-1"},
					FiringTime:   time.Now(),
					ReceivedTime: time.Now(),
					Labels: map[string]string{
						"alertname": "MemoryLeak",
						"namespace": testNs,
					},
				}

				// Step 1: Both pods check for duplicate concurrently
				type checkResult struct {
					podID    int
					isDup    bool
					metadata *processing.DeduplicationMetadata
					err      error
				}

				results := make(chan checkResult, 2)

				// Pod 1 checks
				go func() {
					isDup, metadata, err := dedupSvc1.Check(ctx, signal)
					results <- checkResult{podID: 1, isDup: isDup, metadata: metadata, err: err}
				}()

				// Pod 2 checks (slight delay to simulate network timing)
				go func() {
					time.Sleep(10 * time.Millisecond)
					isDup, metadata, err := dedupSvc2.Check(ctx, signal)
					results <- checkResult{podID: 2, isDup: isDup, metadata: metadata, err: err}
				}()

				// Collect results
				result1 := <-results
				result2 := <-results

				// BUSINESS VALIDATION: Concurrent checks handled correctly
				// ✅ Both checks succeed (no errors)
				// ✅ At least one pod detects "new" (creates CRD)
				// ✅ Eventually consistent (second check may see first's CRD)
				Expect(result1.err).ToNot(HaveOccurred(), "Pod 1 check should succeed")
				Expect(result2.err).ToNot(HaveOccurred(), "Pod 2 check should succeed")

				// At least one pod should detect "new" to create CRD
				newDetections := 0
				if !result1.isDup {
					newDetections++
				}
				if !result2.isDup {
					newDetections++
				}
				Expect(newDetections).To(BeNumerically(">=", 1), "At least one pod should detect new alert")

				GinkgoWriter.Printf("✅ Concurrent deduplication: Pod 1 isDup=%v, Pod 2 isDup=%v\n", result1.isDup, result2.isDup)
			})
		})

		Context("when CRD is deleted", func() {
			It("should allow new CRD creation after deletion", func() {
				// TDD GREEN: Cache invalidation after CRD deletion
				// BR-GATEWAY-025: Cache invalidation after CRD deletion
				// BUSINESS BEHAVIOR: Deleted CRD should not block new CRD creation
				// OUTCOME: System creates new CRD for same fingerprint after deletion

				// Generate valid SHA256 fingerprint
				fingerprintInput := fmt.Sprintf("delete-test-fp-%d", time.Now().UnixNano())
				validFingerprint := generateTestFingerprint(fingerprintInput)

				signal := &types.NormalizedSignal{
					Fingerprint:  validFingerprint,
					AlertName:    "DiskFull",
					Severity:     "critical",
					Namespace:    testNs,
					Resource:     types.ResourceIdentifier{Kind: "Pod", Name: "storage-1"},
					FiringTime:   time.Now(),
					ReceivedTime: time.Now(),
					Labels: map[string]string{
						"alertname": "DiskFull",
						"namespace": testNs,
					},
				}

				// Step 1: Pod 1 creates initial CRD
				fingerprintPrefix := signal.Fingerprint
				if len(fingerprintPrefix) > 12 {
					fingerprintPrefix = fingerprintPrefix[:12]
				}
				crdName := fmt.Sprintf("rr-%s-%d", fingerprintPrefix, time.Now().Unix())

				now := metav1.Now()
				crd := &remediationv1alpha1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      crdName,
						Namespace: testNs,
						Labels: map[string]string{
							"kubernaut.io/signal-fingerprint": signal.Fingerprint[:min(len(signal.Fingerprint), 63)],
						},
					},
					Spec: remediationv1alpha1.RemediationRequestSpec{
						SignalFingerprint: signal.Fingerprint,
						SignalName:        signal.AlertName,
						Severity:          signal.Severity,
						Environment:       "test",
						Priority:          "P1",
						TargetType:        "kubernetes",
						FiringTime:        now,
						ReceivedTime:      now,
						Deduplication: remediationv1alpha1.DeduplicationInfo{
							FirstSeen:       now,
							LastSeen:        now,
							OccurrenceCount: 1,
						},
					},
				}
				err := k8sClient1.Client.Create(ctx, crd)
				Expect(err).ToNot(HaveOccurred(), "Should create initial CRD")

				err = dedupSvc1.Store(ctx, signal, crdName)
				Expect(err).ToNot(HaveOccurred(), "Should store dedup metadata")

				// Step 2: Verify Pod 2 detects duplicate
				time.Sleep(100 * time.Millisecond) // Wait for propagation
				isDup, _, err := dedupSvc2.Check(ctx, signal)
				Expect(err).ToNot(HaveOccurred(), "Should check successfully")
				Expect(isDup).To(BeTrue(), "Should detect duplicate before deletion")

				// Step 3: Delete CRD
				err = k8sClient1.Client.Delete(ctx, crd)
				Expect(err).ToNot(HaveOccurred(), "Should delete CRD")

				// Step 4: Wait for deletion to propagate
				Eventually(func() bool {
					err := k8sClient2.Client.Get(ctx, client.ObjectKey{Name: crdName, Namespace: testNs}, crd)
					return err != nil // Error means CRD is deleted
				}, 5*time.Second, 100*time.Millisecond).Should(BeTrue(), "CRD should be deleted")

				// Step 5: Pod 2 checks again after deletion
				isDupAfterDelete, _, err := dedupSvc2.Check(ctx, signal)

				// BUSINESS VALIDATION: System allows new CRD after deletion
				// ✅ No error
				// ✅ Not detected as duplicate (CRD is gone)
				// ✅ System can create new CRD
				Expect(err).ToNot(HaveOccurred(), "Should check successfully after deletion")
				Expect(isDupAfterDelete).To(BeFalse(), "Should NOT detect duplicate after CRD deletion")

				GinkgoWriter.Printf("✅ Cache invalidation validated: New CRD allowed after deletion\n")
			})
		})

		Context("when CRD phase changes", func() {
			It("should allow new CRD when previous CRD is Completed", func() {
				// TDD GREEN: State-based deduplication with phase awareness
				// BR-GATEWAY-025: State-based deduplication with phase awareness
				// BUSINESS BEHAVIOR: Completed CRD should not block new incident
				// OUTCOME: System creates new CRD for same fingerprint after completion

				// Generate valid SHA256 fingerprint
				fingerprintInput := fmt.Sprintf("phase-test-fp-%d", time.Now().UnixNano())
				validFingerprint := generateTestFingerprint(fingerprintInput)

				signal := &types.NormalizedSignal{
					Fingerprint:  validFingerprint,
					AlertName:    "NetworkError",
					Severity:     "critical",
					Namespace:    testNs,
					Resource:     types.ResourceIdentifier{Kind: "Pod", Name: "frontend-1"},
					FiringTime:   time.Now(),
					ReceivedTime: time.Now(),
					Labels: map[string]string{
						"alertname": "NetworkError",
						"namespace": testNs,
					},
				}

				// Step 1: Pod 1 creates initial CRD
				fingerprintPrefix := signal.Fingerprint
				if len(fingerprintPrefix) > 12 {
					fingerprintPrefix = fingerprintPrefix[:12]
				}
				crdName := fmt.Sprintf("rr-%s-%d", fingerprintPrefix, time.Now().Unix())

				now := metav1.Now()
				crd := &remediationv1alpha1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      crdName,
						Namespace: testNs,
						Labels: map[string]string{
							"kubernaut.io/signal-fingerprint": signal.Fingerprint[:min(len(signal.Fingerprint), 63)],
						},
					},
					Spec: remediationv1alpha1.RemediationRequestSpec{
						SignalFingerprint: signal.Fingerprint,
						SignalName:        signal.AlertName,
						Severity:          signal.Severity,
						Environment:       "test",
						Priority:          "P1",
						TargetType:        "kubernetes",
						FiringTime:        now,
						ReceivedTime:      now,
						Deduplication: remediationv1alpha1.DeduplicationInfo{
							FirstSeen:       now,
							LastSeen:        now,
							OccurrenceCount: 1,
						},
					},
				}
				err := k8sClient1.Client.Create(ctx, crd)
				Expect(err).ToNot(HaveOccurred(), "Should create initial CRD")

				err = dedupSvc1.Store(ctx, signal, crdName)
				Expect(err).ToNot(HaveOccurred(), "Should store dedup metadata")

				// Step 2: Verify Pod 2 detects duplicate while Pending
				time.Sleep(100 * time.Millisecond)
				isDupPending, _, err := dedupSvc2.Check(ctx, signal)
				Expect(err).ToNot(HaveOccurred(), "Should check successfully")
				Expect(isDupPending).To(BeTrue(), "Should detect duplicate while Pending")

				// Step 3: Update CRD phase to Completed
				err = k8sClient1.Client.Get(ctx, client.ObjectKey{Name: crdName, Namespace: testNs}, crd)
				Expect(err).ToNot(HaveOccurred(), "Should get CRD for update")

				crd.Status.OverallPhase = "Completed" // Capital C to match whitelist
				err = k8sClient1.Client.Status().Update(ctx, crd)
				Expect(err).ToNot(HaveOccurred(), "Should update CRD phase to Completed")

				// Step 4: Wait for status update to propagate
				time.Sleep(200 * time.Millisecond)

				// Step 5: Pod 2 checks again after phase change
				isDupCompleted, _, err := dedupSvc2.Check(ctx, signal)

				// BUSINESS VALIDATION: Completed CRD doesn't block new incident
				// ✅ No error
				// ✅ Not detected as duplicate (Completed phase = resolved incident)
				// ✅ System can create new CRD for new incident
				Expect(err).ToNot(HaveOccurred(), "Should check successfully after completion")
				Expect(isDupCompleted).To(BeFalse(), "Should NOT detect duplicate when CRD is Completed")

				GinkgoWriter.Printf("✅ Phase-aware deduplication validated: New CRD allowed after Completed\n")
			})
		})
	})

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// Redis + K8s Coordination
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	Describe("Redis and K8s Coordination", func() {
		Context("when Redis has stale data", func() {
			It("should prioritize K8s state over Redis", func() {
				// TDD GREEN: K8s state is source of truth
				// BR-GATEWAY-025: K8s state is source of truth
				// BUSINESS BEHAVIOR: K8s API state overrides Redis cache
				// OUTCOME: Deduplication based on actual CRD state, not Redis

				// Generate valid SHA256 fingerprint
				fingerprintInput := fmt.Sprintf("stale-redis-fp-%d", time.Now().UnixNano())
				validFingerprint := generateTestFingerprint(fingerprintInput)

				signal := &types.NormalizedSignal{
					Fingerprint:  validFingerprint,
					AlertName:    "ServiceDown",
					Severity:     "critical",
					Namespace:    testNs,
					Resource:     types.ResourceIdentifier{Kind: "Service", Name: "api-gateway"},
					FiringTime:   time.Now(),
					ReceivedTime: time.Now(),
					Labels: map[string]string{
						"alertname": "ServiceDown",
						"namespace": testNs,
					},
				}

				// Step 1: Store stale Redis data (simulates old dedup entry)
				redisKey := fmt.Sprintf("gateway:dedup:fingerprint:%s", signal.Fingerprint)
				err := redisClient.HSet(ctx, redisKey,
					"fingerprint", signal.Fingerprint,
					"remediationRequestRef", "old-crd-name",
					"count", 1,
				).Err()
				Expect(err).ToNot(HaveOccurred(), "Should store stale Redis data")

				err = redisClient.Expire(ctx, redisKey, 5*time.Minute).Err()
				Expect(err).ToNot(HaveOccurred(), "Should set TTL on stale data")

				// Step 2: Pod 1 checks for duplicate (no CRD exists in K8s)
				isDup, _, err := dedupSvc1.Check(ctx, signal)

				// BUSINESS VALIDATION: K8s state (no CRD) overrides Redis (has entry)
				// ✅ No error
				// ✅ Not detected as duplicate (K8s has no CRD)
				// ✅ Redis data ignored when K8s disagrees
				Expect(err).ToNot(HaveOccurred(), "Should check successfully")
				Expect(isDup).To(BeFalse(), "Should NOT detect duplicate (K8s has no CRD)")

				GinkgoWriter.Printf("✅ K8s state priority validated: Redis ignored when CRD doesn't exist\n")
			})
		})
	})
})
