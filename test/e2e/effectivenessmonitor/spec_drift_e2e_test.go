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

package effectivenessmonitor

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/conditions"
)

// ============================================================================
// Spec Drift Guard E2E Tests (DD-EM-002 v1.1)
//
// Business outcomes validated:
//   - BR-EM-004: Spec drift detection aborts assessment as unreliable
//   - ADR-EM-001: DS short-circuits score to 0.0 for spec_drift
//   - DD-CRD-002: SpecIntegrity and AssessmentComplete conditions set correctly
//   - BR-AUDIT-006: Audit trail contains spec_drift completion event
//
// Strategy:
//   Same as IT-EM-SD-001: let EA complete naturally, then patch status back
//   to Assessing with a fake PostRemediationSpecHash. The real EM controller
//   in Kind detects the mismatch on re-reconcile and completes with spec_drift.
//   Unlike INT, E2E validates the FULL stack: CRD + audit trail in DS + scoring.
// ============================================================================

var _ = Describe("Spec Drift Guard E2E Tests (DD-EM-002 v1.1)", Label("e2e"), func() {
	var testNS string

	BeforeEach(func() {
		testNS = createTestNamespace("em-sd-e2e")
	})

	AfterEach(func() {
		deleteTestNamespace(testNS)
	})

	// ========================================================================
	// E2E-EM-SD-001: Spec drift detection -> EA completes with spec_drift,
	//                audit trail records it, DS scores 0.0
	// ========================================================================
	It("E2E-EM-SD-001: should detect spec drift, emit audit events, and DS should score 0.0", func() {
		By("1. Creating a target pod and letting EA complete naturally")
		createTargetPod(testNS, "target-pod")
		waitForPodReady(testNS, "target-pod")

		correlationID := uniqueName("corr-sd-drift")
		name := uniqueName("ea-sd-drift")
		createEA(testNS, name, correlationID,
			withTargetPod("target-pod"),
		)

		// Wait for the EA to complete naturally with "full" reason
		ea := waitForEAPhase(name, eav1.PhaseCompleted)
		realHash := ea.Status.Components.PostRemediationSpecHash
		Expect(realHash).ToNot(BeEmpty(), "PostRemediationSpecHash should be computed")
		GinkgoWriter.Printf("EA completed naturally: reason=%s, hash=%s\n",
			ea.Status.AssessmentReason, realHash)

		By("2. Patching EA status back to Assessing with a FAKE PostRemediationSpecHash")
		// This simulates a scenario where the target resource spec was modified
		// after the RO recorded the post-remediation hash. The EM should detect
		// the mismatch and complete with spec_drift.
		fakeOldHash := "sha256:0000000000000000000000000000000000000000000000000000000000000000"
		Expect(fakeOldHash).NotTo(Equal(realHash))

		Eventually(func(g Gomega) {
			fetchedEA := &eav1.EffectivenessAssessment{}
			g.Expect(apiReader.Get(ctx, client.ObjectKey{
				Namespace: controllerNamespace, Name: name,
			}, fetchedEA)).To(Succeed())

			deadline := metav1.NewTime(time.Now().Add(30 * time.Minute))
			checkAfter := metav1.NewTime(time.Now().Add(-1 * time.Second))

			fetchedEA.Status.Phase = eav1.PhaseAssessing
			fetchedEA.Status.CompletedAt = nil
			fetchedEA.Status.AssessmentReason = ""
			fetchedEA.Status.Message = ""
			fetchedEA.Status.ValidityDeadline = &deadline
			fetchedEA.Status.PrometheusCheckAfter = &checkAfter
			fetchedEA.Status.AlertManagerCheckAfter = &checkAfter
			fetchedEA.Status.Components.HashComputed = true
			fetchedEA.Status.Components.PostRemediationSpecHash = fakeOldHash
			fetchedEA.Status.Components.CurrentSpecHash = ""
			fetchedEA.Status.Components.HealthAssessed = false
			fetchedEA.Status.Components.AlertAssessed = false
			fetchedEA.Status.Components.MetricsAssessed = false
			fetchedEA.Status.Components.HealthScore = nil
			fetchedEA.Status.Components.AlertScore = nil
			fetchedEA.Status.Components.MetricsScore = nil
			fetchedEA.Status.Conditions = nil

			g.Expect(k8sClient.Status().Update(ctx, fetchedEA)).To(Succeed())
		}, timeout, interval).Should(Succeed())

		By("3. Waiting for EA to complete with spec_drift reason")
		var driftedEA *eav1.EffectivenessAssessment
		Eventually(func(g Gomega) {
			fetched := &eav1.EffectivenessAssessment{}
			g.Expect(apiReader.Get(ctx, client.ObjectKey{
				Namespace: testNS, Name: name,
			}, fetched)).To(Succeed())
			g.Expect(fetched.Status.Phase).To(Equal(eav1.PhaseCompleted))
			g.Expect(fetched.Status.AssessmentReason).To(Equal(eav1.AssessmentReasonSpecDrift))
			driftedEA = fetched
		}, timeout, interval).Should(Succeed())

		// ====================================================================
		// BUSINESS OUTCOME 1: CRD correctness (BR-EM-004, DD-CRD-002)
		// ====================================================================
		By("4. Verifying CRD status: CurrentSpecHash matches real hash, not fake")
		Expect(driftedEA.Status.Components.CurrentSpecHash).To(Equal(realHash),
			"CORRECTNESS: CurrentSpecHash must be the actual resource hash, not the fake")
		Expect(driftedEA.Status.Components.CurrentSpecHash).NotTo(Equal(fakeOldHash))
		Expect(driftedEA.Status.CompletedAt).NotTo(BeNil(),
			"CORRECTNESS: CompletedAt must be set on terminal phase")

		By("5. Verifying Kubernetes Conditions (DD-CRD-002)")
		specCond := meta.FindStatusCondition(driftedEA.Status.Conditions, conditions.ConditionSpecIntegrity)
		Expect(specCond).NotTo(BeNil(), "BEHAVIOR: SpecIntegrity condition must be set")
		Expect(specCond.Status).To(Equal(metav1.ConditionFalse),
			"BEHAVIOR: SpecIntegrity must be False when drift is detected")
		Expect(specCond.Reason).To(Equal(conditions.ReasonSpecDrifted),
			"ACCURACY: Reason must be SpecDrifted")

		completeCond := meta.FindStatusCondition(driftedEA.Status.Conditions, conditions.ConditionAssessmentComplete)
		Expect(completeCond).NotTo(BeNil(), "BEHAVIOR: AssessmentComplete condition must be set")
		Expect(completeCond.Status).To(Equal(metav1.ConditionTrue),
			"BEHAVIOR: AssessmentComplete must be True (assessment is terminal)")
		Expect(completeCond.Reason).To(Equal(conditions.ReasonSpecDrift),
			"ACCURACY: Reason must be SpecDrift")

		// ====================================================================
		// BUSINESS OUTCOME 2: Audit trail in DataStorage (BR-AUDIT-006)
		// ====================================================================
		By("6. Verifying effectiveness.assessment.completed audit event in DS with spec_drift reason")
		Eventually(func(g Gomega) {
			params := ogenclient.QueryAuditEventsParams{
				CorrelationID: ogenclient.NewOptString(correlationID),
				EventCategory: ogenclient.NewOptString("effectiveness"),
				Limit:         ogenclient.NewOptInt(50),
			}
			resp, err := auditClient.QueryAuditEvents(ctx, params)
			g.Expect(err).ToNot(HaveOccurred())

			// Find completed events and check for spec_drift reason in payload.
			// There will be 2 completed events: one from the natural "full" completion
			// and one from the re-triggered "spec_drift" completion.
			var hasSpecDriftEvent bool
			for _, evt := range resp.Data {
				if evt.EventType != "effectiveness.assessment.completed" {
					continue
				}
				payload, ok := evt.EventData.GetEffectivenessAssessmentAuditPayload()
				if ok && payload.Reason.IsSet() && payload.Reason.Value == "spec_drift" {
					hasSpecDriftEvent = true
					break
				}
			}
			g.Expect(hasSpecDriftEvent).To(BeTrue(),
				"BR-AUDIT-006: Audit trail must contain effectiveness.assessment.completed with reason=spec_drift")
		}, timeout, interval).Should(Succeed())

		// ====================================================================
		// BUSINESS OUTCOME 3: DS scoring short-circuit (ADR-EM-001, DD-EM-002 v1.1)
		// ====================================================================
		By("7. Verifying DS effectiveness score is 0.0 for spec_drift")
		Eventually(func(g Gomega) {
			scoreResp, err := auditClient.GetEffectivenessScore(ctx,
				ogenclient.GetEffectivenessScoreParams{
					CorrelationID: correlationID,
				})
			g.Expect(err).ToNot(HaveOccurred())

			scored, ok := scoreResp.(*ogenclient.EffectivenessScoreResponse)
			g.Expect(ok).To(BeTrue(), "Response should be *EffectivenessScoreResponse")
			g.Expect(scored.AssessmentStatus).To(Equal(
				ogenclient.EffectivenessScoreResponseAssessmentStatusSpecDrift),
				"ACCURACY: assessment_status must be spec_drift")
			g.Expect(scored.Score.Set).To(BeTrue(),
				"CORRECTNESS: Score must be set (not null) for spec_drift")
			g.Expect(scored.Score.Null).To(BeFalse(),
				"CORRECTNESS: Score must not be null for spec_drift")
			g.Expect(scored.Score.Value).To(Equal(0.0),
				"ACCURACY: Score must be 0.0 — spec drift means assessment is unreliable (DD-EM-002 v1.1)")
		}, timeout, interval).Should(Succeed())

		GinkgoWriter.Printf("E2E-EM-SD-001: PASSED — spec_drift detected, score=0.0, conditions correct\n")
	})

	// ========================================================================
	// E2E-EM-SD-002: No drift -> normal completion, DS scores normally
	// ========================================================================
	It("E2E-EM-SD-002: should complete normally without drift and DS should score > 0.0", func() {
		By("1. Creating a target pod and EA")
		createTargetPod(testNS, "target-pod")
		waitForPodReady(testNS, "target-pod")

		correlationID := uniqueName("corr-sd-nodrift")
		name := uniqueName("ea-sd-nodrift")
		createEA(testNS, name, correlationID,
			withTargetPod("target-pod"),
		)

		By("2. Waiting for EA to complete normally")
		ea := waitForEAPhase(name, eav1.PhaseCompleted)

		// ====================================================================
		// BUSINESS OUTCOME 1: No drift detected (negative test)
		// ====================================================================
		By("3. Verifying assessment did NOT complete with spec_drift")
		Expect(ea.Status.AssessmentReason).NotTo(Equal(eav1.AssessmentReasonSpecDrift),
			"CORRECTNESS: Assessment should not report spec_drift when spec is unchanged")

		By("4. Verifying SpecIntegrity condition is True")
		specCond := meta.FindStatusCondition(ea.Status.Conditions, conditions.ConditionSpecIntegrity)
		Expect(specCond).NotTo(BeNil(), "BEHAVIOR: SpecIntegrity condition must be set")
		Expect(specCond.Status).To(Equal(metav1.ConditionTrue),
			"ACCURACY: SpecIntegrity must be True when no drift")
		Expect(specCond.Reason).To(Equal(conditions.ReasonSpecUnchanged))

		// ====================================================================
		// BUSINESS OUTCOME 2: DS scores normally (not 0.0)
		// ====================================================================
		By("5. Verifying DS effectiveness score is NOT 0.0 (no drift short-circuit)")
		Eventually(func(g Gomega) {
			scoreResp, err := auditClient.GetEffectivenessScore(ctx,
				ogenclient.GetEffectivenessScoreParams{
					CorrelationID: correlationID,
				})
			g.Expect(err).ToNot(HaveOccurred())

			scored, ok := scoreResp.(*ogenclient.EffectivenessScoreResponse)
			g.Expect(ok).To(BeTrue(), "Response should be *EffectivenessScoreResponse")
			g.Expect(scored.AssessmentStatus).NotTo(Equal(
				ogenclient.EffectivenessScoreResponseAssessmentStatusSpecDrift),
				"ACCURACY: assessment_status must NOT be spec_drift for normal completion")
		}, timeout, interval).Should(Succeed())

		GinkgoWriter.Printf("E2E-EM-SD-002: PASSED — no drift, normal scoring\n")
	})
})
