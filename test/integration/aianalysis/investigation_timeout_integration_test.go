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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

// DD-TIMEOUT-002 / Issue #2176: proves AIAnalysis self-enforces RO's
// propagated Spec.TimesOutAt through the real production reconcile loop
// (envtest -> AIAnalysisReconciler -> InvestigatingHandler.checkInvestigationTimeout),
// taking precedence over the handler's own hardcoded maxInvestigationDuration
// default (25m, unmodified in this suite's default handler wiring), well
// before RO's own outer backstop would ever fire.
var _ = Describe("DD-TIMEOUT-002: AIAnalysis self-enforces Spec.TimesOutAt", Label("integration", "timeout"), func() {
	const (
		timeout  = 15 * time.Second
		interval = 200 * time.Millisecond
	)

	It("IT-AA-2176-001: fails the analysis via Spec.TimesOutAt long before the 25m maxInvestigationDuration default would fire", func() {
		rrName := helpers.UniqueTestName("rr-2176-timeout")
		aaName := helpers.UniqueTestName("aa-2176-timeout")

		By("creating AIAnalysis with an already-past Spec.TimesOutAt and a slow investigation scenario")
		pastDeadline := metav1.NewTime(time.Now().Add(-1 * time.Minute))
		analysis := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      aaName,
				Namespace: testNamespace,
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationRequestRef: corev1.ObjectReference{
					Name:      rrName,
					Namespace: testNamespace,
				},
				RemediationID: rrName,
				// DD-TIMEOUT-002: propagated by RO from Status.TimeoutConfig.Analyzing.
				// Already-past so the very first poll after session establishment
				// self-fails, without waiting for the "slow-investigation-test"
				// mock scenario to organically complete.
				TimesOutAt: &pastDeadline,
				AnalysisRequest: aianalysisv1.AnalysisRequest{
					SignalContext: aianalysisv1.SignalContextInput{
						Fingerprint:      "fp-2176-timeout",
						Severity:         "warning",
						SignalName:       "slow-investigation-test",
						Environment:      "staging",
						BusinessPriority: "P2",
						TargetResource: aianalysisv1.TargetResource{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: testNamespace,
						},
						EnrichmentResults: sharedtypes.EnrichmentResults{},
					},
					AnalysisTypes: []aianalysisv1.AnalysisType{aianalysisv1.AnalysisTypeInvestigation},
				},
			},
		}
		Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

		By("verifying the production reconciler self-fails it via Spec.TimesOutAt, not the 25m default")
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
			g.Expect(analysis.Status.Phase).To(Equal(aianalysisv1.PhaseFailed),
				"DD-TIMEOUT-002: an already-past Spec.TimesOutAt must fail the analysis long before the 25m maxInvestigationDuration default")
		}, timeout, interval).Should(Succeed())
	})
})
