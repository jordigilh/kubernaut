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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

var _ = Describe("DD-2582 SignalProcessing parent lifecycle", Label("integration", "issue-2582"), func() {
	It("IT-SP-2582-001: cancels before enrichment when the persisted RR is Failed [BR-SP-*, AC-4]", func() {
		ns := createTestNamespace(ctx, "sp-2582-parent")
		DeferCleanup(func() { deleteTestNamespace(ns) })

		target := signalprocessingv1.ResourceIdentifier{Kind: "Pod", Name: "parent-lifecycle-target", Namespace: ns}
		rr := CreateTestRemediationRequest("rr-sp-2582-parent", ns, ValidTestFingerprints["reconciler-05"], "critical", target)
		Expect(k8sClient.Create(ctx, rr)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, rr) })

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr)).To(Succeed())
		rr.Status.OverallPhase = remediationv1.PhaseFailed
		Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())

		sp := CreateTestSignalProcessingWithParent("sp-2582-parent", ns, rr, ValidTestFingerprints["reconciler-05"], target)
		Expect(k8sClient.Create(ctx, sp)).To(Succeed())
		DeferCleanup(func() { _ = deleteAndWait(sp) })

		Eventually(func() string {
			fetched := &signalprocessingv1.SignalProcessing{}
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), fetched); err != nil {
				return ""
			}
			return string(fetched.Status.Phase)
		}, timeout, interval).Should(Equal(signalprocessingv1.PhaseCancelled))
	})
})
