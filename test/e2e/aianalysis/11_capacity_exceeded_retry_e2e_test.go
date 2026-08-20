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
// the burst arrives at KA's dispatcher as close to simultaneously as possible
// -- Mock LLM investigations typically complete in well under a second, so a
// serial, trickling burst could let completions free up slots fast enough to
// never genuinely exceed capacity.
//
// 120 (2.4x overshoot) originally, reduced to 70 (1.4x overshoot, 2026-08-20)
// after DD-AA-KA-001 Gap 6's dispatch-Lease-leak fix landed: with the actual
// bug fixed, two consecutive real CI runs showed zero Failed (retry logic is
// correct) but a still-flaky tail of not-yet-Completed investigations at any
// fixed timeout -- worse (113/120) on the run with the *longer* 360s timeout
// than the prior run's 300s (116/120), proving this tail is driven by
// absolute concurrent system load on the shared CI runner (120 simultaneous
// LLM-driven investigations), not insufficient wall-clock budget. Raising the
// timeout further was also approaching this job's 20-minute CI cap (this
// single spec alone was consuming ~6 of the ~17 minutes used). 70 still
// comfortably exceeds the 50-slot cap by 20 (enough to force multiple genuine
// rejections -- the retry mechanism only needs *some* overshoot to prove
// itself, not 2.4x) while cutting total concurrent load ~42%.
const capacityBurstOvershoot = 70

var _ = Describe("E2E-AA-065: AgentSession capacity-exceeded retry", Label("e2e", "capacity-retry", "aa-065"), func() {
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
							Fingerprint:      "e2e-fp-" + suffix,
							Severity:         "warning",
							SignalName:       "CrashLoopBackOff",
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
		// 240s (2026-08-20, alongside capacityBurstOvershoot's 120->70 reduction
		// above): the earlier 180s->300s->360s timeout bumps were chasing a moving
		// target -- the real bug (DD-AA-KA-001 Gap 6's dispatch-Lease leak) is now
		// fixed (two consecutive CI runs: zero Failed), but a 120-investigation
		// burst's absolute concurrent load produced a tail that got *worse*, not
		// better, when the timeout alone was raised (113/120 at 360s vs. 116/120 at
		// 300s), while also consuming most of this job's 20-minute CI budget on this
		// one spec. With the burst now cut to 70 (still 20 over KA's 50-slot cap,
		// comfortably enough to force multiple genuine rejections), 240s gives
		// generous margin against a much smaller concurrent load.
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
		}, 240*time.Second, 3*time.Second).Should(
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
