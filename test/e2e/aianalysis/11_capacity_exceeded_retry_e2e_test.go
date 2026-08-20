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

package aianalysis

import (
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// E2E-AA-065: AgentSession Capacity-Exceeded Retry (full cross-service journey)
// Business Requirements: BR-AI-009 (retry transient errors with backoff),
// BR-AA-KA-065 (AA<->KA channel via AgentSession CRD)
// Design: DD-AA-KA-001 amendment (AgentSessionReasonCapacityExceeded retry)
//
// Purpose: prove the full KA -> AgentSession -> AA journey for a REAL
// dispatch-time capacity rejection (session.ErrMaxInvestigationsReached),
// not a synthetic/seeded one -- this is what test/integration/aianalysis/
// capacityretry (IT) deliberately does NOT cover, since that suite isolates
// AA from a real KA to avoid a real-KA-subprocess race (see its suite_test.go
// doc comment). Only this E2E test exercises KA's actual session.Store
// admission control under genuine concurrent load.
//
// capacityBurstOvershoot deliberately creates more concurrent investigations
// than this E2E cluster's configured KA runtime.session.maxConcurrentInvestigations
// (50 -- see the inline KA config template in test/infrastructure/kubernautagent.go).
// Investigations are created via concurrent goroutines (not a serial loop) so
// the burst arrives at KA's dispatcher as close to simultaneously as possible.
//
// History (2026-08-20): 120 (2.4x overshoot) originally, with the default
// fast-completing "CrashLoopBackOff" mock scenario. After DD-AA-KA-001 Gap
// 6's dispatch-Lease-leak fix landed, two consecutive CI runs showed zero
// Failed (retry logic is correct) but a flaky tail of not-yet-Completed
// investigations that got *worse*, not better, when only the timeout was
// raised (113/120 Completed at 360s vs. 116/120 at 300s) -- proving the tail
// was driven by absolute concurrent system load on the shared CI runner, not
// insufficient wall-clock budget. Cutting the burst to 70 fixed the load
// problem but introduced the opposite failure: on a lightly-loaded runner,
// all 70 completed in 18s with ZERO capacity rejections ever observed
// (KASession.Generation never advanced) -- fast-completing investigations
// simply never overlapped enough to exceed the 50-slot cap, since burst
// *creation* (rate-limited by the API server) and burst *completion* (mock
// LLM responds in well under a second) were racing each other, and on that
// run creation lost. Root cause of both failure modes: relying on wall-clock
// race timing (a fundamentally non-deterministic lever) to force overlap.
// Fixed by using the "brief-investigation-test" mock LLM scenario below
// (~9-12s deterministic investigation duration, the same mechanism already
// proven by IT-AA-1376-001 for holding KA sessions open a controlled amount
// of time) instead of racing burst-creation speed against instant
// completion -- this guarantees overlap deterministically regardless of CI
// runner speed, so the burst size no longer needs to be large enough to
// "win" a timing race. 70 (1.4x overshoot, 20 over the 50-slot cap) is ample
// to force multiple genuine rejections once overlap is guaranteed.
const capacityBurstOvershoot = 70

// Serial (2026-08-20 RCA, PR #2189 round-13): this suite runs with
// `--procs=$(TEST_PROCS)` (Makefile test-e2e-% target), so without Serial
// this spec's deliberate capacity-saturating burst runs concurrently with
// OTHER specs in sibling Ginkgo processes -- all sharing the SAME KA
// deployment's session.maxConcurrentInvestigations=50 cap. That starved
// unrelated specs (03_full_flow_test.go, 02_metrics_test.go,
// 05_audit_trail_test.go) of dispatch capacity mid-burst, pushing their
// tight (30-60s) Eventually windows past their limit even though they never
// touch capacity-retry logic themselves. Exact precedent for this class of
// fix: test/e2e/datastorage/11_connection_pool_exhaustion_test.go's own
// Serial (burst saturates a shared connection pool, interfering with
// parallel tests). Serial guarantees this spec only runs once every other
// process is idle, matching this test's already-serial internal design (one
// blocking burst-then-converge It).
var _ = Describe("E2E-AA-065: AgentSession capacity-exceeded retry", Label("e2e", "capacity-retry", "aa-065"), Serial, func() {
	It("transparently retries and eventually completes every investigation despite exceeding KA's real dispatch capacity", func() {
		analyses := make([]*aianalysisv1.AIAnalysis, capacityBurstOvershoot)
		for i := range analyses {
			suffix := randomSuffix()
			rrName := "e2e-capacity-retry-" + suffix
			analyses[i] = &aianalysisv1.AIAnalysis{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-capacity-retry-" + suffix,
					Namespace: controllerNamespace,
				},
				Spec: aianalysisv1.AIAnalysisSpec{
					RemediationRequestRef: corev1.ObjectReference{
						Name:      rrName,
						Namespace: controllerNamespace,
					},
					RemediationID: rrName,
					AnalysisRequest: aianalysisv1.AnalysisRequest{
						SignalContext: aianalysisv1.SignalContextInput{
							Fingerprint: "e2e-fp-" + suffix,
							Severity:    "warning",
							// "brief-investigation-test" routes to the mock LLM's
							// briefInvestigationConfig scenario (~9-12s deterministic
							// investigation duration) instead of the near-instant
							// default "CrashLoopBackOff" scenario -- see
							// capacityBurstOvershoot's doc comment above for why a
							// deterministic delay, not burst size, is what reliably
							// forces capacity overlap.
							SignalName:       "brief-investigation-test",
							Environment:      "staging",
							BusinessPriority: "P2",
							TargetResource: aianalysisv1.TargetResource{
								Kind:      "Pod",
								Name:      "capacity-retry-target",
								Namespace: "staging",
							},
							EnrichmentResults: sharedtypes.EnrichmentResults{},
						},
						AnalysisTypes: []aianalysisv1.AnalysisType{aianalysisv1.AnalysisTypeInvestigation, aianalysisv1.AnalysisTypeWorkflowSelection},
					},
				},
			}
		}

		defer func() {
			for _, a := range analyses {
				_ = k8sClient.Delete(ctx, a)
			}
		}()

		By("bursting far more concurrent investigations than KA's configured dispatch capacity, all at once")
		var wg sync.WaitGroup
		for _, a := range analyses {
			wg.Add(1)
			go func(a *aianalysisv1.AIAnalysis) {
				defer wg.Done()
				defer GinkgoRecover()
				Expect(k8sClient.Create(ctx, a)).To(Succeed())
			}(a)
		}
		wg.Wait()

		By("every investigation must eventually converge to Completed -- a capacity rejection is transient and must never surface as a permanent failure")
		// 120s (2026-08-20, alongside switching to the deterministic
		// "brief-investigation-test" scenario -- see capacityBurstOvershoot's doc
		// comment above): with each investigation taking a bounded ~9-12s and at
		// most a handful of ErrorClassifier retry cycles (backoff base 1s,
		// multiplier 2.0, max 5 attempts -- ~31s worst case) needed to drain 20
		// investigations past a 50-slot cap in two waves, total convergence should
		// land well under a minute; 120s leaves generous margin for CI scheduling
		// noise without re-approaching this job's 20-minute CI budget the way the
		// earlier 240s-360s timeouts (chasing a non-deterministic race) did.
		Eventually(func() map[string]int {
			counts := map[string]int{}
			for _, a := range analyses {
				fresh := &aianalysisv1.AIAnalysis{}
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(a), fresh); err != nil {
					counts["(get-error)"]++
					continue
				}
				phase := fresh.Status.Phase
				if phase == "" {
					phase = "(pending)"
				}
				counts[phase]++
			}
			return counts
		}, 120*time.Second, 3*time.Second).Should(
			SatisfyAll(
				HaveKeyWithValue("Completed", capacityBurstOvershoot),
				Not(HaveKey("Failed")),
			),
			"every burst investigation must reach Completed; any Failed indicates a capacity rejection leaked into a permanent failure instead of being retried",
		)

		By("at least one investigation in the burst must have genuinely hit KA's real capacity limit and been retried (BR-AI-009)")
		atLeastOneRetried := false
		for _, a := range analyses {
			fresh := &aianalysisv1.AIAnalysis{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(a), fresh)).To(Succeed())
			if fresh.Status.KASession != nil && fresh.Status.KASession.Generation > 0 {
				atLeastOneRetried = true
				break
			}
		}
		Expect(atLeastOneRetried).To(BeTrue(),
			"a burst of far more concurrent investigations than KA's configured capacity must trigger at least one real "+
				"AgentSessionReasonCapacityExceeded retry (KASession.Generation only advances via retryCapacityExceeded) -- "+
				"otherwise this test isn't proving what it claims")
	})
})
