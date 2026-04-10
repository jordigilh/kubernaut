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

// Spec Drift Early Exit Test
//
// Issue #254: Captures the monolith's Step 6.5 behavior — when the post-remediation
// hash has been computed and the current spec hash differs (spec drift), the
// reconciler completes early with AssessmentReasonSpecDrift.
//
// This test MUST pass against the monolith and after decomposition.
package effectivenessmonitor

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/prometheus/client_golang/prometheus"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/effectivenessmonitor"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/conditions"
	emmetrics "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/metrics"
)

var _ = Describe("Spec Drift Early Exit (UT-EM-254-005, #254)", func() {

	buildScheme := func() *runtime.Scheme {
		s := runtime.NewScheme()
		Expect(eav1.AddToScheme(s)).To(Succeed())
		Expect(corev1.AddToScheme(s)).To(Succeed())
		return s
	}

	makeReconciler := func(s *runtime.Scheme, recorder *record.FakeRecorder, objs ...client.Object) (*controller.Reconciler, client.Client) {
		fakeClient := fake.NewClientBuilder().
			WithScheme(s).
			WithObjects(objs...).
			WithStatusSubresource(&eav1.EffectivenessAssessment{}).
			Build()

		cfg := controller.DefaultReconcilerConfig()
		cfg.PrometheusEnabled = false
		cfg.AlertManagerEnabled = false

		r := controller.NewReconciler(
			fakeClient, fakeClient,
			s, recorder,
			emmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			nil, nil,
			nil, nil,
			cfg,
		)
		return r, fakeClient
	}

	// seedDriftedEA creates an EA in Assessing phase with hash already computed.
	// PostRemediationSpecHash is set to a known value that will differ from what
	// getTargetSpec produces (metadata fallback since restMapper is nil).
	seedDriftedEA := func(ns, name string) *eav1.EffectivenessAssessment {
		vd := metav1.NewTime(time.Now().Add(25 * time.Minute))
		pca := metav1.NewTime(time.Now().Add(-1 * time.Minute))
		healthScore := 1.0
		return &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name: name, Namespace: ns,
				CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute)),
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           "corr-drift-" + name,
				RemediationRequestPhase: "Completed",
				SignalTarget:            eav1.TargetResource{Kind: "Deployment", Name: "test-app", Namespace: ns},
				RemediationTarget:       eav1.TargetResource{Kind: "Deployment", Name: "test-app", Namespace: ns},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 0},
				},
			},
			Status: eav1.EffectivenessAssessmentStatus{
				Phase:                  eav1.PhaseAssessing,
				ValidityDeadline:       &vd,
				PrometheusCheckAfter:   &pca,
				AlertManagerCheckAfter: &pca,
				Components: eav1.EAComponents{
					HashComputed:            true,
					PostRemediationSpecHash: "sha256:original-hash-before-drift-000000000000000000",
					CurrentSpecHash:         "sha256:original-hash-before-drift-000000000000000000",
					HealthAssessed:          true,
					HealthScore:             &healthScore,
				},
			},
		}
	}

	It("UT-EM-254-005: Spec drift triggers early exit with correct reason and conditions", func() {
		s := buildScheme()
		recorder := record.NewFakeRecorder(100)
		ea := seedDriftedEA("default", "ea-drift-005")
		r, fc := makeReconciler(s, recorder, ea)

		ctx := context.Background()
		req := ctrl.Request{NamespacedName: types.NamespacedName{
			Name: "ea-drift-005", Namespace: "default",
		}}

		result, err := r.Reconcile(ctx, req)
		Expect(err).NotTo(HaveOccurred())

		// --- Snapshot: ctrl.Result ---
		// Spec drift completes the assessment (no requeue).
		Expect(result.Requeue).To(BeFalse(),
			"Spec drift must complete without requeue")
		Expect(result.RequeueAfter).To(BeZero(),
			"Spec drift must have zero RequeueAfter")

		// --- Snapshot: EA Status ---
		fetched := &eav1.EffectivenessAssessment{}
		Expect(fc.Get(ctx, req.NamespacedName, fetched)).To(Succeed())

		Expect(fetched.Status.Phase).To(Equal(eav1.PhaseCompleted),
			"Phase must be Completed after spec drift")
		Expect(fetched.Status.AssessmentReason).To(Equal(eav1.AssessmentReasonSpecDrift),
			"AssessmentReason must be spec_drift")
		Expect(fetched.Status.CompletedAt).NotTo(BeNil(),
			"CompletedAt must be set")

		// --- Snapshot: CurrentSpecHash updated ---
		// Step 6.5 sets CurrentSpecHash to the new (drifted) hash.
		Expect(fetched.Status.Components.CurrentSpecHash).NotTo(Equal(
			fetched.Status.Components.PostRemediationSpecHash),
			"CurrentSpecHash must differ from PostRemediationSpecHash (drift detected)")
		Expect(fetched.Status.Components.CurrentSpecHash).NotTo(BeEmpty(),
			"CurrentSpecHash must be populated")

		// --- Snapshot: Conditions ---
		// Spec drift sets SpecIntegrity=False and AssessmentComplete=True.
		specIntegrityFound := false
		assessmentCompleteFound := false
		for _, cond := range fetched.Status.Conditions {
			if cond.Type == conditions.ConditionSpecIntegrity {
				specIntegrityFound = true
				Expect(string(cond.Status)).To(Equal(string(metav1.ConditionFalse)),
					"SpecIntegrity must be False on drift")
				Expect(cond.Reason).To(Equal(conditions.ReasonSpecDrifted),
					"SpecIntegrity reason must be SpecDrifted")
			}
			if cond.Type == conditions.ConditionAssessmentComplete {
				assessmentCompleteFound = true
				Expect(string(cond.Status)).To(Equal(string(metav1.ConditionTrue)),
					"AssessmentComplete must be True")
			}
		}
		Expect(specIntegrityFound).To(BeTrue(),
			"SpecIntegrity condition must be present")
		Expect(assessmentCompleteFound).To(BeTrue(),
			"AssessmentComplete condition must be present")

		// --- Snapshot: Events ---
		// Spec drift emits a Warning event for SpecDriftDetected.
		Eventually(recorder.Events).Should(Receive(ContainSubstring("SpecDriftDetected")),
			"Spec drift must emit a SpecDriftDetected warning event")
	})
})
