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
// capacityBurstOvershoot deliberately creates far more concurrent
// investigations than this E2E cluster's configured KA
// runtime.session.maxConcurrentInvestigations (50 -- see the inline KA config
// template in test/infrastructure/kubernautagent.go). Investigations are
// created via concurrent goroutines (not a serial loop) so the burst arrives
// at KA's dispatcher as close to simultaneously as possible -- Mock LLM
// investigations typically complete in well under a second, so a serial,
// trickling burst could let completions free up slots fast enough to never
// genuinely exceed capacity.
const capacityBurstOvershoot = 120

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
		// 300s (raised from 180s per CI evidence, 2026-08-20): two consecutive real CI
		// runs after the RBAC fix (DD-AA-KA-001 Gap 5 amendment) showed zero Failed --
		// the retry mechanism itself is correct -- but only 90/120 had converged to
		// Completed by 180s, the rest still (correctly) retrying. 120 investigations
		// against KA's 50-slot cap means a meaningful fraction queue through multiple
		// ErrorClassifier backoff rounds (up to ~31s of backoff alone across 5 retries)
		// before a slot frees; 300s gives comfortable margin on a shared CI runner.
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
		}, 300*time.Second, 3*time.Second).Should(
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
