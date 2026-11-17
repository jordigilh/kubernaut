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
	"sigs.k8s.io/controller-runtime/pkg/client"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/cancellation"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("Issue #2588 EffectivenessAssessment cancellation", func() {
	It("IT-EM-2588-001: cancels an existing assessment after the parent enters Cancelling", func() {
		ns := createTestNamespace(ctx, "em-cancel-2588")
		DeferCleanup(func() { deleteTestNamespace(ns) })

		parent := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rr-ea-cancel-2588",
				Namespace: ns,
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: "0000000000000000000000000000000000000000000000000000000000000000",
				SignalName:        "EffectivenessCancellation",
				Severity:          "critical",
				SignalType:        "alert",
				TargetType:        "kubernetes",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind: "Deployment", Name: "test-app", Namespace: ns,
				},
				FiringTime:   metav1.Now(),
				ReceivedTime: metav1.Now(),
			},
		}
		Expect(k8sClient.Create(ctx, parent)).To(Succeed())
		parent.Status.OverallPhase = remediationv1.PhaseCancelling
		Expect(k8sClient.Status().Update(ctx, parent)).To(Succeed())

		ea := &eav1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ea-rr-ea-cancel-2588",
				Namespace: ns,
			},
			Spec: eav1.EffectivenessAssessmentSpec{
				CorrelationID:           parent.Name,
				RemediationRequestPhase: string(remediationv1.PhaseVerifying),
				SignalTarget: eav1.TargetResource{
					Kind: "Deployment", Name: "test-app", Namespace: ns,
				},
				RemediationTarget: eav1.TargetResource{
					Kind: "Deployment", Name: "test-app", Namespace: ns,
				},
				Config: eav1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: time.Second},
				},
			},
		}
		Expect(controllerutil.SetControllerReference(parent, ea, k8sClient.Scheme())).To(Succeed())
		Expect(k8sClient.Create(ctx, ea)).To(Succeed())

		Eventually(func() string {
			current := &eav1.EffectivenessAssessment{}
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(ea), current); err != nil {
				return ""
			}
			return current.Status.Phase
		}, timeout, interval).Should(Equal(eav1.PhaseCancelled))

		current := &eav1.EffectivenessAssessment{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ea), current)).To(Succeed())
		Expect(cancellation.IsAcknowledged(current.Status.Conditions)).To(BeTrue())
		Expect(current.Status.CompletedAt).NotTo(BeNil())
	})
})
