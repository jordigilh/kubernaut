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

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
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
// capacityLimit is this E2E cluster's configured
// KA runtime.session.maxConcurrentInvestigations (see the inline KA config
// template in test/infrastructure/kubernautagent.go).
const capacityLimit = 50

// capacityBurstOvershoot creates 20 investigations beyond the configured cap.
// The test fills the first 50 slots and then creates the overshoot concurrently
// so capacity rejection is controlled by the KA admission boundary rather than
// by a wall-clock race between API-server creation and mock-LLM completion.
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
// Fixed by filling the configured cap with the existing
// "slow-investigation-test" mock scenario before creating the overshoot. This
// removes the API-server creation-rate versus investigation-completion race
// without changing KA configuration or the test timeout.
const capacityBurstOvershoot = capacityLimit + 20

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
							// "slow-investigation-test" keeps the first capacityLimit
							// AgentSessions in KA's Investigating phase while the
							// overshoot is admitted. This avoids relying on the
							// API-server creation rate to beat mock-LLM completion.
							SignalName:       "slow-investigation-test",
							SignalMode:       "reactive",
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

		createBatch := func(batch []*aianalysisv1.AIAnalysis) {
			var wg sync.WaitGroup
			for _, a := range batch {
				wg.Add(1)
				go func(a *aianalysisv1.AIAnalysis) {
					defer wg.Done()
					defer GinkgoRecover()
					Expect(k8sClient.Create(ctx, a)).To(Succeed())
				}(a)
			}
			wg.Wait()
		}

		By("filling KA's configured dispatch capacity with long-lived investigations")
		createBatch(analyses[:capacityLimit])
		Eventually(func(g Gomega) int {
			investigating := 0
			for _, a := range analyses[:capacityLimit] {
				sessionName := "as-" + a.Name
				session := &agentsessionv1.AgentSession{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKey{
					Name:      sessionName,
					Namespace: controllerNamespace,
				}, session)).To(Succeed(), "AgentSession %s must be readable by the E2E client", sessionName)
				if session.Status.Phase == agentsessionv1.AgentSessionPhaseInvestigating {
					investigating++
				}
			}
			return investigating
		}, 60*time.Second, 500*time.Millisecond).Should(Equal(capacityLimit),
			"the first capacityLimit AgentSessions must be running before the overshoot is created")

		By("creating the overshoot concurrently so KA must reject at least one dispatch")
		createBatch(analyses[capacityLimit:])

		By("every investigation must eventually converge to Completed -- a capacity rejection is transient and must never surface as a permanent failure")
		// 120s leaves margin for the slow fixture's first wave, capacity-retry
		// backoff, and the second wave without changing the existing CI budget.
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
