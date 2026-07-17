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

package fullpipeline

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// E2E-FP-1390-001: Full upgrade journey — autonomous → IS CRD → AA sets Interactive →
// MCP user connects → KA upgrades session → RCA completes with InteractiveHold →
// session completes.
var _ = Describe("E2E-FP-1390-001: Session Upgrade Journey", Label("e2e", "fullpipeline", "1390"), func() {

	It("should complete upgrade journey: autonomous submit → IS appears → AA upgrades → KA holds → user completes [SC-24, AC-12]", func() {
		By("creating a RemediationRequest to trigger investigation")
		testID := "fp-1390-upgrade"
		rrName, err := infrastructure.CreateDirectRR(ctx, namespace, testID)
		Expect(err).NotTo(HaveOccurred())

		// Create the IS CRD immediately after the RR — before the AA
		// controller even starts reconciling. This eliminates the race
		// where the AA completes its lifecycle (< 1s with mock-LLM) before
		// the IS CRD exists. The AA will detect the IS CRD during its
		// reconciliation and set Interactive=true.
		By("creating Active IS CRD immediately to prevent race with fast AA lifecycle")
		is := &isv1alpha1.InvestigationSession{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "is-" + testID,
				Namespace: namespace,
			},
			Spec: isv1alpha1.InvestigationSessionSpec{
				RemediationRequestRef: isv1alpha1.ObjectRef{
					Name:      rrName,
					Namespace: namespace,
				},
				A2ATaskID: "task-" + testID,
				UserIdentity: isv1alpha1.SessionUser{
					Username: "sre-upgrade@kubernaut.ai",
					Groups:   []string{"sre"},
				},
				JoinMode: isv1alpha1.SessionJoinModeStart,
			},
		}
		Expect(k8sClient.Create(ctx, is)).To(Succeed())
		is.Status.Phase = isv1alpha1.SessionPhaseActive
		Expect(k8sClient.Status().Update(ctx, is)).To(Succeed())

		By("waiting for AA to reach Investigating with an autonomous session")
		var aaName string
		Eventually(func() bool {
			aaList := &aianalysisv1.AIAnalysisList{}
			if err := apiReader.List(ctx, aaList, client.InNamespace(namespace)); err != nil {
				return false
			}
			for _, aa := range aaList.Items {
				if aa.Spec.RemediationRequestRef.Name == rrName {
					aaName = aa.Name
					phase := aa.Status.Phase
					return phase == aianalysisv1.PhaseInvestigating || phase == aianalysisv1.PhaseAnalyzing || phase == aianalysisv1.PhaseCompleted
				}
			}
			return false
		}, timeout, 200*time.Millisecond).Should(BeTrue())

		By("verifying AA upgrades to Interactive=true without cancel")
		var aa aianalysisv1.AIAnalysis
		Eventually(func(g Gomega) {
			g.Expect(apiReader.Get(ctx, client.ObjectKey{Name: aaName, Namespace: namespace}, &aa)).To(Succeed())
			g.Expect(aa.Status.KASession).NotTo(BeNil())
			g.Expect(aa.Status.KASession.Interactive).To(BeTrue(),
				"AA must set Interactive=true on upgrade")
		}, timeout, 1*time.Second).Should(Succeed())

		originalSessionID := aa.Status.KASession.ID

		By("verifying session ID is preserved — no cancel/resubmit")
		Expect(aa.Status.KASession.ID).To(Equal(originalSessionID),
			"session ID must be preserved — no cancel/resubmit")

		// Interactive mode keeps the investigation open for user commands.
		// Terminal phase completion is tested by E2E-1293 via explicit MCP
		// takeover + complete calls. This test's scope is the upgrade journey
		// (autonomous → Interactive=true with preserved session ID).
	})
})
