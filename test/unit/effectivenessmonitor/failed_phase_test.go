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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/effectivenessmonitor"
	emmetrics "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/metrics"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/phase"
)

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = eav1.AddToScheme(s)
	return s
}

func makeEA(name, ns, correlationID, rrPhase string) *eav1.EffectivenessAssessment {
	return &eav1.EffectivenessAssessment{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         ns,
			CreationTimestamp: metav1.Now(),
		},
		Spec: eav1.EffectivenessAssessmentSpec{
			CorrelationID:           correlationID,
			RemediationRequestPhase: rrPhase,
			SignalTarget: eav1.TargetResource{
				Kind:      "Deployment",
				Name:      "my-deploy",
				Namespace: ns,
			},
			Config: eav1.EAConfig{
				StabilizationWindow: metav1.Duration{Duration: 30 * time.Second},
			},
		},
	}
}

var _ = Describe("EM Failed Phase (#573, ADR-EM-001 section 11)", func() {

	// UT-EM-573-001: Reconciler transitions to PhaseFailed when correlationID is empty
	Describe("Spec validation guard (UT-EM-573-001)", func() {
		It("should transition EA to PhaseFailed when correlationID is empty", func() {
			s := newTestScheme()
			ea := makeEA("ea-no-correlation", "default", "", "Verifying")

			fakeClient := fake.NewClientBuilder().
				WithScheme(s).
				WithObjects(ea).
				WithStatusSubresource(ea).
				Build()

			reconciler := controller.NewReconciler(
				fakeClient, fakeClient, s,
				record.NewFakeRecorder(10),
				emmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				nil, nil, nil, nil,
				controller.DefaultReconcilerConfig(),
			)

			_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "ea-no-correlation", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred(),
				"UT-EM-573-001: reconciliation should succeed (error handled internally)")

			updatedEA := &eav1.EffectivenessAssessment{}
			Expect(fakeClient.Get(context.Background(), types.NamespacedName{
				Name: "ea-no-correlation", Namespace: "default",
			}, updatedEA)).To(Succeed())

			Expect(updatedEA.Status.Phase).To(Equal(eav1.PhaseFailed),
				"UT-EM-573-001: EA with empty correlationID should transition to Failed")
			Expect(updatedEA.Status.AssessmentReason).To(Equal("unrecoverable"),
				"UT-EM-573-001: reason should be 'unrecoverable'")
			Expect(updatedEA.Status.Message).To(ContainSubstring("correlationID"),
				"UT-EM-573-001: message should mention correlationID")
		})
	})

	// UT-EM-573-002: Completion fields populated for Failed phase
	Describe("Completion fields for Failed phase (UT-EM-573-002)", func() {
		It("should populate completedAt, assessmentReason, and message on Failed transition", func() {
			s := newTestScheme()
			ea := makeEA("ea-fail-fields", "default", "", "Verifying")

			fakeClient := fake.NewClientBuilder().
				WithScheme(s).
				WithObjects(ea).
				WithStatusSubresource(ea).
				Build()

			reconciler := controller.NewReconciler(
				fakeClient, fakeClient, s,
				record.NewFakeRecorder(10),
				emmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
				nil, nil, nil, nil,
				controller.DefaultReconcilerConfig(),
			)

			beforeReconcile := time.Now().Add(-1 * time.Second)
			_, _ = reconciler.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "ea-fail-fields", Namespace: "default"},
			})

			updatedEA := &eav1.EffectivenessAssessment{}
			Expect(fakeClient.Get(context.Background(), types.NamespacedName{
				Name: "ea-fail-fields", Namespace: "default",
			}, updatedEA)).To(Succeed())

			Expect(updatedEA.Status.CompletedAt).NotTo(BeNil(),
				"UT-EM-573-002: completedAt should be set")
			Expect(updatedEA.Status.CompletedAt.Time).To(BeTemporally(">=", beforeReconcile),
				"UT-EM-573-002: completedAt should be after reconcile started")
			Expect(updatedEA.Status.AssessmentReason).To(Equal("unrecoverable"),
				"UT-EM-573-002: assessmentReason should be 'unrecoverable'")
			Expect(updatedEA.Status.Message).To(ContainSubstring("correlationID is required"),
				"UT-EM-573-002: message should describe the validation failure")
		})
	})

	// UT-EM-573-003: RO handles PhaseFailed (validate existing behavior)
	Describe("RO condition for Failed EA (UT-EM-573-003)", func() {
		It("should confirm phase state machine allows transitions to Failed from non-terminal phases", func() {
			// This validates the state machine that the RO relies on.
			// The RO's trackEffectivenessStatus already handles PhaseFailed
			// (internal/controller/remediationorchestrator/effectiveness_tracking.go:137-154).
			Expect(phase.CanTransition(phase.Pending, phase.Failed)).To(BeTrue(),
				"UT-EM-573-003: Pending -> Failed must be valid for RO to track")
			Expect(phase.CanTransition(phase.Assessing, phase.Failed)).To(BeTrue(),
				"UT-EM-573-003: Assessing -> Failed must be valid for RO to track")
			Expect(phase.CanTransition(phase.Stabilizing, phase.Failed)).To(BeTrue(),
				"UT-EM-573-003: Stabilizing -> Failed must be valid")
			Expect(phase.CanTransition(phase.WaitingForPropagation, phase.Failed)).To(BeTrue(),
				"UT-EM-573-003: WaitingForPropagation -> Failed must be valid")
		})
	})

	// UT-EM-573-004: CanTransition state machine for Failed
	Describe("CanTransition for Failed (UT-EM-573-004)", func() {
		It("should allow transition to Failed from all non-terminal phases", func() {
			nonTerminalPhases := []phase.Phase{
				phase.Pending,
				phase.WaitingForPropagation,
				phase.Stabilizing,
				phase.Assessing,
			}
			for _, p := range nonTerminalPhases {
				Expect(phase.CanTransition(p, phase.Failed)).To(BeTrue(),
					"UT-EM-573-004: %s -> Failed should be allowed", p)
			}
		})

		It("should disallow transition to Failed from terminal phases", func() {
			terminalPhases := []phase.Phase{
				phase.Completed,
				phase.Failed,
			}
			for _, p := range terminalPhases {
				Expect(phase.CanTransition(p, phase.Failed)).To(BeFalse(),
					"UT-EM-573-004: %s -> Failed should be disallowed", p)
			}
		})
	})
})
