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
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redis/go-redis/v9"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	rediscache "github.com/jordigilh/kubernaut/pkg/cache/redis"
	"github.com/jordigilh/kubernaut/pkg/gateway/k8s"
	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// DD-GATEWAY-009: CRD Phase-Based Deduplication Unit Tests
//
// Business Outcome Testing: Test WHAT phase-based deduplication enables
//
// ❌ WRONG: "should query K8s API" (tests implementation)
// ✅ RIGHT: "allows new CRD when previous remediation completed" (tests business outcome)
//
// PHASE BEHAVIOR:
// - Pending/Processing: Duplicate (increment occurrenceCount, no new CRD)
// - Completed/Failed/Cancelled: New incident (allow new CRD)
// - Unknown states: Conservative (treat as in-progress, no new CRD)

var _ = Describe("DD-GATEWAY-009: CRD Phase-Based Deduplication", func() {
	var (
		ctx             context.Context
		dedupService    *processing.DeduplicationService
		redisServer     *miniredis.Miniredis
		redisClient     *redis.Client
		k8sClient       *k8s.Client
		logger          logr.Logger
		testSignal      *types.NormalizedSignal
		testFingerprint string
		scheme          *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger = logr.Discard()

		// Setup miniredis server (required for deduplication service)
		var err error
		redisServer, err = miniredis.Run()
		Expect(err).NotTo(HaveOccurred())

		// Create Redis client pointing to miniredis
		redisClient = redis.NewClient(&redis.Options{
			Addr: redisServer.Addr(),
		})

		// Setup K8s scheme with RemediationRequest CRD
		scheme = runtime.NewScheme()
		err = remediationv1alpha1.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())

		// Create test signal with consistent fingerprint
		testFingerprint = "phase-test-fingerprint-12345678901234567890abcdef123456"
		testSignal = &types.NormalizedSignal{
			AlertName: "PodCrashLooping",
			Namespace: "production",
			Resource: types.ResourceIdentifier{
				Kind: "Pod",
				Name: "payment-api-789",
			},
			Severity:    "critical",
			Fingerprint: testFingerprint,
		}
	})

	AfterEach(func() {
		_ = redisClient.Close()
		redisServer.Close()
	})

	// Helper to create a RemediationRequest CRD with specific phase
	createCRDWithPhase := func(name, namespace, fingerprint, phase string) *remediationv1alpha1.RemediationRequest {
		return &remediationv1alpha1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels: map[string]string{
					"kubernaut.io/signal-fingerprint": fingerprint,
				},
			},
			Spec: remediationv1alpha1.RemediationRequestSpec{
				Deduplication: sharedtypes.DeduplicationInfo{
					OccurrenceCount:  1,
					FirstOccurrence:  metav1.NewTime(time.Now().Add(-5 * time.Minute)),
					LastOccurrence:   metav1.NewTime(time.Now()),
				},
			},
			Status: remediationv1alpha1.RemediationRequestStatus{
				OverallPhase: phase,
			},
		}
	}

	// Helper to create deduplication service with fake K8s client
	// Uses interceptor to handle MatchingFields{} which fake client doesn't support
	createServiceWithCRDs := func(crds ...*remediationv1alpha1.RemediationRequest) *processing.DeduplicationService {
		// Build fake client with existing CRDs and interceptor
		builder := fake.NewClientBuilder().WithScheme(scheme)
		for _, crd := range crds {
			builder = builder.WithObjects(crd)
		}

		// Use interceptor to handle List with MatchingFields{} (fake client doesn't support empty field selectors)
		// The interceptor removes the MatchingFields option and lets the label selector work
		builder = builder.WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				// Filter out MatchingFields options (fake client doesn't support empty field selectors)
				filteredOpts := make([]client.ListOption, 0, len(opts))
				for _, opt := range opts {
					// Skip MatchingFields options - they cause "field selector is not in one of the two supported forms" error
					if _, ok := opt.(client.MatchingFields); ok {
						continue
					}
					filteredOpts = append(filteredOpts, opt)
				}
				return c.List(ctx, list, filteredOpts...)
			},
		})

		fakeClient := builder.Build()
		k8sClient = k8s.NewClient(fakeClient)

		// Create Redis cache client
		rediscacheClient := rediscache.NewClient(&redis.Options{
			Addr: redisServer.Addr(),
		}, logger)

		return processing.NewDeduplicationService(rediscacheClient, k8sClient, logger, nil)
	}

	// BUSINESS OUTCOME: Phase-based deduplication allows retry after completion
	// Scenario: Alert fires → CRD created → Remediation completes → Same alert fires again
	// Expected: New CRD created (previous remediation finished, this might be a new incident)
	Describe("Final State Phases (Allow New CRD)", func() {

		Context("when existing CRD has Completed phase (BR-GATEWAY-009)", func() {
			It("allows new CRD creation for potential new incident", func() {
				// BUSINESS SCENARIO: Previous remediation succeeded, but alert fires again
				// This could be a new incident (e.g., different root cause)
				existingCRD := createCRDWithPhase(
					"rr-completed-12345",
					"production",
					testFingerprint,
					"Completed",
				)

				dedupService = createServiceWithCRDs(existingCRD)

				isDuplicate, metadata, err := dedupService.Check(ctx, testSignal)

				// BUSINESS OUTCOME: New CRD allowed (remediation completed, allow retry)
				Expect(err).NotTo(HaveOccurred(),
					"Phase check must succeed")
				Expect(isDuplicate).To(BeFalse(),
					"Completed phase should allow new CRD (not duplicate)")
				Expect(metadata).To(BeNil(),
					"No duplicate metadata for new incident")

				// Business capability verified:
				// Completed remediation + same alert = new incident → new CRD → AI analyzes again
			})
		})

		Context("when existing CRD has Failed phase (BR-GATEWAY-009)", func() {
			It("allows new CRD creation for retry attempt", func() {
				// BUSINESS SCENARIO: Previous remediation failed, alert persists
				// User wants system to retry remediation
				existingCRD := createCRDWithPhase(
					"rr-failed-12345",
					"production",
					testFingerprint,
					"Failed",
				)

				dedupService = createServiceWithCRDs(existingCRD)

				isDuplicate, metadata, err := dedupService.Check(ctx, testSignal)

				// BUSINESS OUTCOME: New CRD allowed (failed remediation, allow retry)
				Expect(err).NotTo(HaveOccurred(),
					"Phase check must succeed")
				Expect(isDuplicate).To(BeFalse(),
					"Failed phase should allow new CRD (retry allowed)")
				Expect(metadata).To(BeNil(),
					"No duplicate metadata for retry attempt")

				// Business capability verified:
				// Failed remediation + same alert = retry → new CRD → AI tries different approach
			})
		})

		Context("when existing CRD has Cancelled phase (BR-GATEWAY-009)", func() {
			It("allows new CRD creation after user cancellation", func() {
				// BUSINESS SCENARIO: User cancelled previous remediation, alert persists
				// User now wants to allow remediation
				existingCRD := createCRDWithPhase(
					"rr-cancelled-12345",
					"production",
					testFingerprint,
					"Cancelled",
				)

				dedupService = createServiceWithCRDs(existingCRD)

				isDuplicate, metadata, err := dedupService.Check(ctx, testSignal)

				// BUSINESS OUTCOME: New CRD allowed (user cancelled, allow new attempt)
				Expect(err).NotTo(HaveOccurred(),
					"Phase check must succeed")
				Expect(isDuplicate).To(BeFalse(),
					"Cancelled phase should allow new CRD")
				Expect(metadata).To(BeNil(),
					"No duplicate metadata after cancellation")

				// Business capability verified:
				// Cancelled + same alert = new attempt → new CRD → AI processes
			})
		})
	})

	// BUSINESS OUTCOME: Phase-based deduplication prevents duplicate processing
	// Scenario: Alert fires → CRD created → Remediation in progress → Same alert fires again
	// Expected: No new CRD (increment occurrence count on existing CRD)
	Describe("In-Progress Phases (Block New CRD)", func() {

		Context("when existing CRD has Pending phase (BR-GATEWAY-009)", func() {
			It("blocks new CRD and increments occurrence count", func() {
				// BUSINESS SCENARIO: CRD exists, waiting for AI to process
				// Same alert fires again before AI picks it up
				existingCRD := createCRDWithPhase(
					"rr-pending-12345",
					"production",
					testFingerprint,
					"Pending",
				)

				dedupService = createServiceWithCRDs(existingCRD)

				isDuplicate, metadata, err := dedupService.Check(ctx, testSignal)

				// BUSINESS OUTCOME: Duplicate detected (in-progress remediation)
				Expect(err).NotTo(HaveOccurred(),
					"Phase check must succeed")
				Expect(isDuplicate).To(BeTrue(),
					"Pending phase should block new CRD (duplicate)")
				Expect(metadata).NotTo(BeNil(),
					"Duplicate metadata should be returned")
				Expect(metadata.Count).To(Equal(2),
					"Occurrence count should be incremented (1 existing + 1 new)")
				Expect(metadata.RemediationRequestRef).To(Equal("production/rr-pending-12345"),
					"Should reference existing CRD")

				// Business capability verified:
				// Pending CRD + same alert = duplicate → no new CRD → AI processes once
			})
		})

		Context("when existing CRD has Processing phase (BR-GATEWAY-009)", func() {
			It("blocks new CRD while remediation is running", func() {
				// BUSINESS SCENARIO: AI is actively processing remediation
				// Same alert fires again during processing
				existingCRD := createCRDWithPhase(
					"rr-processing-12345",
					"production",
					testFingerprint,
					"Processing",
				)

				dedupService = createServiceWithCRDs(existingCRD)

				isDuplicate, metadata, err := dedupService.Check(ctx, testSignal)

				// BUSINESS OUTCOME: Duplicate detected (remediation in progress)
				Expect(err).NotTo(HaveOccurred(),
					"Phase check must succeed")
				Expect(isDuplicate).To(BeTrue(),
					"Processing phase should block new CRD (duplicate)")
				Expect(metadata).NotTo(BeNil(),
					"Duplicate metadata should be returned")
				Expect(metadata.Count).To(Equal(2),
					"Occurrence count should be incremented")

				// Business capability verified:
				// Processing CRD + same alert = duplicate → no new CRD → avoid duplicate work
			})
		})
	})

	// BUSINESS OUTCOME: Conservative handling of unknown phases
	// Scenario: Future CRD phase added (e.g., "Validating") → treat as in-progress
	// Expected: No new CRD (safer than creating duplicates)
	Describe("Unknown Phases (Conservative Block)", func() {

		Context("when existing CRD has unknown phase (BR-GATEWAY-009)", func() {
			It("conservatively blocks new CRD for safety", func() {
				// BUSINESS SCENARIO: Future phase like "Validating" or "WaitingForApproval"
				// System doesn't recognize it, but should be safe
				existingCRD := createCRDWithPhase(
					"rr-unknown-12345",
					"production",
					testFingerprint,
					"Validating", // Unknown future phase
				)

				dedupService = createServiceWithCRDs(existingCRD)

				isDuplicate, metadata, err := dedupService.Check(ctx, testSignal)

				// BUSINESS OUTCOME: Duplicate detected (conservative approach)
				Expect(err).NotTo(HaveOccurred(),
					"Phase check must succeed")
				Expect(isDuplicate).To(BeTrue(),
					"Unknown phase should be treated as in-progress (conservative)")
				Expect(metadata).NotTo(BeNil(),
					"Duplicate metadata should be returned")

				// Business capability verified:
				// Unknown phase = conservative → no new CRD → safer than duplicates
			})
		})

		Context("when existing CRD has empty phase (BR-GATEWAY-009)", func() {
			It("conservatively blocks new CRD for newly created CRD", func() {
				// BUSINESS SCENARIO: CRD just created, status not yet set
				existingCRD := createCRDWithPhase(
					"rr-empty-12345",
					"production",
					testFingerprint,
					"", // Empty phase (newly created)
				)

				dedupService = createServiceWithCRDs(existingCRD)

				isDuplicate, metadata, err := dedupService.Check(ctx, testSignal)

				// BUSINESS OUTCOME: Duplicate detected (CRD exists, phase pending)
				Expect(err).NotTo(HaveOccurred(),
					"Phase check must succeed")
				Expect(isDuplicate).To(BeTrue(),
					"Empty phase should be treated as in-progress")
				Expect(metadata).NotTo(BeNil(),
					"Duplicate metadata should be returned")

				// Business capability verified:
				// Empty phase (new CRD) = in-progress → no duplicate CRD
			})
		})
	})

	// BUSINESS OUTCOME: Multiple CRDs with mixed phases
	// Scenario: Multiple CRDs exist for same fingerprint (historical)
	// Expected: If ANY is in-progress, block new CRD
	Describe("Multiple CRDs with Mixed Phases", func() {

		Context("when one CRD is Completed and another is Pending (BR-GATEWAY-009)", func() {
			It("blocks new CRD because one is still in-progress", func() {
				// BUSINESS SCENARIO: Historical completed CRD + new pending CRD
				completedCRD := createCRDWithPhase(
					"rr-completed-old",
					"production",
					testFingerprint,
					"Completed",
				)
				pendingCRD := createCRDWithPhase(
					"rr-pending-new",
					"production",
					testFingerprint,
					"Pending",
				)

				dedupService = createServiceWithCRDs(completedCRD, pendingCRD)

				isDuplicate, metadata, err := dedupService.Check(ctx, testSignal)

				// BUSINESS OUTCOME: Duplicate detected (pending CRD exists)
				Expect(err).NotTo(HaveOccurred(),
					"Phase check must succeed")
				Expect(isDuplicate).To(BeTrue(),
					"Should block new CRD when any is in-progress")
				Expect(metadata).NotTo(BeNil(),
					"Duplicate metadata should reference in-progress CRD")

				// Business capability verified:
				// Mixed phases with in-progress = duplicate → no new CRD
			})
		})

		Context("when all CRDs are in final states (BR-GATEWAY-009)", func() {
			It("allows new CRD when all previous are finished", func() {
				// BUSINESS SCENARIO: Multiple historical CRDs, all completed/failed
				completedCRD1 := createCRDWithPhase(
					"rr-completed-1",
					"production",
					testFingerprint,
					"Completed",
				)
				failedCRD := createCRDWithPhase(
					"rr-failed-1",
					"production",
					testFingerprint,
					"Failed",
				)
				cancelledCRD := createCRDWithPhase(
					"rr-cancelled-1",
					"production",
					testFingerprint,
					"Cancelled",
				)

				dedupService = createServiceWithCRDs(completedCRD1, failedCRD, cancelledCRD)

				isDuplicate, metadata, err := dedupService.Check(ctx, testSignal)

				// BUSINESS OUTCOME: New CRD allowed (all previous finished)
				Expect(err).NotTo(HaveOccurred(),
					"Phase check must succeed")
				Expect(isDuplicate).To(BeFalse(),
					"Should allow new CRD when all previous are in final states")
				Expect(metadata).To(BeNil(),
					"No duplicate metadata for new incident")

				// Business capability verified:
				// All final states = new incident → new CRD → AI processes
			})
		})
	})

	// BUSINESS OUTCOME: Namespace isolation
	// Scenario: Same fingerprint in different namespaces
	// Expected: Each namespace tracks independently
	Describe("Namespace Isolation", func() {

		Context("when CRD exists in different namespace (BR-GATEWAY-009)", func() {
			It("allows new CRD in target namespace", func() {
				// BUSINESS SCENARIO: Same alert in staging (pending) and production (new)
				stagingCRD := createCRDWithPhase(
					"rr-staging-12345",
					"staging", // Different namespace
					testFingerprint,
					"Pending",
				)

				dedupService = createServiceWithCRDs(stagingCRD)

				// Test signal is in "production" namespace
				isDuplicate, metadata, err := dedupService.Check(ctx, testSignal)

				// BUSINESS OUTCOME: New CRD allowed (different namespace)
				Expect(err).NotTo(HaveOccurred(),
					"Phase check must succeed")
				Expect(isDuplicate).To(BeFalse(),
					"CRD in different namespace should not block new CRD")
				Expect(metadata).To(BeNil(),
					"No duplicate metadata for different namespace")

				// Business capability verified:
				// Different namespace = independent → new CRD → multi-tenant isolation
			})
		})
	})

	// BUSINESS OUTCOME: No existing CRD
	// Scenario: First alert for this fingerprint
	// Expected: Allow new CRD (first occurrence)
	Describe("No Existing CRD", func() {

		Context("when no CRD exists for fingerprint (BR-GATEWAY-009)", func() {
			It("allows new CRD for first occurrence", func() {
				// BUSINESS SCENARIO: First time this alert fires
				dedupService = createServiceWithCRDs() // No CRDs

				isDuplicate, metadata, err := dedupService.Check(ctx, testSignal)

				// BUSINESS OUTCOME: New CRD allowed (first occurrence)
				Expect(err).NotTo(HaveOccurred(),
					"Phase check must succeed")
				Expect(isDuplicate).To(BeFalse(),
					"No existing CRD should allow new CRD")
				Expect(metadata).To(BeNil(),
					"No duplicate metadata for first occurrence")

				// Business capability verified:
				// No existing CRD = new incident → new CRD → AI processes
			})
		})
	})
})

