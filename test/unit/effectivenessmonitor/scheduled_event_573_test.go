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
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/effectivenessmonitor"
	emmetrics "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/metrics"
)

var _ = Describe("EM Scheduled Event Timing (#573, ADR-EM-001 section 9.2.0)", func() {

	// UT-EM-573-005: Guard against duplicate emission
	Describe("emitScheduledEventIfFirst guard (UT-EM-573-005)", func() {
		It("should not re-emit scheduled event when ValidityDeadline is already set", func() {
			s := newTestScheme()
			deadline := metav1.NewTime(time.Now().Add(30 * time.Minute))
			ea := &eav1.EffectivenessAssessment{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "ea-already-scheduled",
					Namespace:         "default",
					CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute)),
				},
				Spec: eav1.EffectivenessAssessmentSpec{
					CorrelationID:           "rr-existing",
					RemediationRequestPhase: "Verifying",
					SignalTarget: eav1.TargetResource{
						Kind:      "Deployment",
						Name:      "my-deploy",
						Namespace: "default",
					},
					Config: eav1.EAConfig{
						StabilizationWindow: metav1.Duration{Duration: 30 * time.Second},
					},
				},
				Status: eav1.EffectivenessAssessmentStatus{
					Phase:             eav1.PhaseAssessing,
					ValidityDeadline:  &deadline,
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(s).
				WithObjects(ea).
				WithStatusSubresource(ea).
				Build()

			fakeRecorder := record.NewFakeRecorder(10)
			reconciler := controller.NewReconciler(
				fakeClient, fakeClient, s,
				fakeRecorder,
				emmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				nil, nil, nil, nil,
				controller.DefaultReconcilerConfig(),
			)

			_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "ea-already-scheduled", Namespace: "default"},
			})
			// The reconcile may return an error due to missing Prometheus/AlertManager,
			// but it should NOT emit a duplicate "AssessmentScheduled" event
			_ = err

			// Drain the recorder channel — no "AssessmentScheduled" event should appear
			hasScheduledEvent := false
			for {
				select {
				case event := <-fakeRecorder.Events:
					if event == "Normal AssessmentScheduled" || containsScheduled(event) {
						hasScheduledEvent = true
					}
				default:
					goto done
				}
			}
		done:
			Expect(hasScheduledEvent).To(BeFalse(),
				"UT-EM-573-005: RecordAssessmentScheduled should NOT be re-emitted when ValidityDeadline is already set")
		})
	})

	// IT-EM-573-011: WFP transition emits scheduled event
	Describe("WFP transition scheduled event (IT-EM-573-011)", func() {
		It("should emit assessment.scheduled K8s event on WaitingForPropagation transition", func() {
			s := newTestScheme()
			ea := &eav1.EffectivenessAssessment{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "ea-wfp-scheduled",
					Namespace:         "default",
					CreationTimestamp: metav1.Now(),
				},
				Spec: eav1.EffectivenessAssessmentSpec{
					CorrelationID:           "rr-wfp-test",
					RemediationRequestPhase: "Verifying",
					SignalTarget: eav1.TargetResource{
						Kind:      "Deployment",
						Name:      "my-deploy",
						Namespace: "default",
					},
					Config: eav1.EAConfig{
						StabilizationWindow: metav1.Duration{Duration: 5 * time.Minute},
						HashComputeDelay:    &metav1.Duration{Duration: 10 * time.Minute},
					},
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(s).
				WithObjects(ea).
				WithStatusSubresource(ea).
				Build()

			fakeRecorder := record.NewFakeRecorder(10)
			reconciler := controller.NewReconciler(
				fakeClient, fakeClient, s,
				fakeRecorder,
				emmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				nil, nil, nil, nil,
				controller.DefaultReconcilerConfig(),
			)

			_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "ea-wfp-scheduled", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())

			updatedEA := &eav1.EffectivenessAssessment{}
			Expect(fakeClient.Get(context.Background(), types.NamespacedName{
				Name: "ea-wfp-scheduled", Namespace: "default",
			}, updatedEA)).To(Succeed())

			Expect(updatedEA.Status.Phase).To(Equal(eav1.PhaseWaitingForPropagation),
				"IT-EM-573-011: EA should transition to WaitingForPropagation")
			Expect(updatedEA.Status.ValidityDeadline).NotTo(BeNil(),
				"IT-EM-573-011: ValidityDeadline should be set on WFP transition")

			hasScheduledEvent := false
			for {
				select {
				case event := <-fakeRecorder.Events:
					if containsScheduled(event) {
						hasScheduledEvent = true
					}
				default:
					goto done2
				}
			}
		done2:
			Expect(hasScheduledEvent).To(BeTrue(),
				"IT-EM-573-011: AssessmentScheduled event should be emitted on WFP transition")
		})
	})
})

func containsScheduled(event string) bool {
	return len(event) > 0 && (contains(event, "AssessmentScheduled") || contains(event, "assessment.scheduled") || contains(event, "Assessment scheduled"))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
