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

package signalprocessing

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	spconditions "github.com/jordigilh/kubernaut/pkg/signalprocessing"
)

// DD-TIMEOUT-002 / Issue #2176: proves SignalProcessing self-enforces RO's
// propagated Spec.TimesOutAt through the real production reconcile loop
// (envtest -> SignalProcessingReconciler.Reconcile -> hasTimedOut/failOnTimeout),
// well before RO's own outer backstop would ever fire.
var _ = Describe("DD-TIMEOUT-002: SignalProcessing self-enforces Spec.TimesOutAt", func() {
	It("IT-SP-2176-001: fails the SignalProcessing via Spec.TimesOutAt", func() {
		ns := createTestNamespaceWithLabels("sp-timeout", nil)
		defer deleteTestNamespace(ns)

		By("creating parent RemediationRequest")
		targetResource := signalprocessingv1alpha1.ResourceIdentifier{
			Kind:      "Pod",
			Name:      "test-pod",
			Namespace: ns,
		}
		rrName := "rr-2176-timeout"
		rr := CreateTestRemediationRequest(rrName, ns, ValidTestFingerprints["reconciler-01"], "warning", targetResource)
		Expect(k8sClient.Create(ctx, rr)).To(Succeed())

		By("creating SignalProcessing with an already-past Spec.TimesOutAt")
		sp := CreateTestSignalProcessingWithParent("sp-2176-timeout", ns, rr, ValidTestFingerprints["reconciler-01"], targetResource)
		pastDeadline := metav1.NewTime(time.Now().Add(-1 * time.Minute))
		sp.Spec.TimesOutAt = &pastDeadline
		Expect(k8sClient.Create(ctx, sp)).To(Succeed())
		defer func() { _ = deleteAndWait(sp) }()

		By("verifying the production reconciler self-fails it via Spec.TimesOutAt")
		Expect(waitForPhase(sp.Name, sp.Namespace, signalprocessingv1alpha1.PhaseFailed)).To(Succeed(),
			"DD-TIMEOUT-002: an already-past Spec.TimesOutAt must fail SP long before RO's outer backstop")

		var final signalprocessingv1alpha1.SignalProcessing
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sp.Name, Namespace: ns}, &final)).To(Succeed())

		processingComplete := spconditions.GetCondition(&final, spconditions.ConditionProcessingComplete)
		Expect(processingComplete).ToNot(BeNil())
		Expect(processingComplete.Status).To(Equal(metav1.ConditionFalse))
		Expect(processingComplete.Reason).To(Equal(spconditions.ReasonTimedOut))
	})
})
