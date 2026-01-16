/*
Copyright 2025 Jordi Gil.

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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

// This file contains a minimal test to verify the ENVTEST setup works.
// It will be expanded with full integration tests in reconciler_integration_test.go.

var _ = Describe("SignalProcessing Integration Setup Verification", func() {
	Context("ENVTEST Environment", func() {
		It("should have a working k8sClient", func() {
			Expect(k8sClient).ToNot(BeNil())
		})

		It("should be able to create and delete SignalProcessing CRD", func() {
			// Create unique namespace for this test
			ns := createTestNamespace("setup-verify")
			defer deleteTestNamespace(ns)

			// Create a SignalProcessing CR
			sp := &signalprocessingv1alpha1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-setup-verification",
					Namespace: ns,
				},
				Spec: signalprocessingv1alpha1.SignalProcessingSpec{
					RemediationRequestRef: signalprocessingv1alpha1.ObjectReference{
						Name: "setup-test-rr",
					},
					Signal: signalprocessingv1alpha1.SignalData{
						Fingerprint: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
						Name:        "TestSignal",
						Severity: "low",
						Type:        "kubernetes-event",
						TargetType:  "kubernetes",
						TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: ns,
						},
						ReceivedTime: metav1.Now(),
					},
				},
			}

			// Create
			err := k8sClient.Create(ctx, sp)
			Expect(err).ToNot(HaveOccurred())

			// Verify it exists
			var fetched signalprocessingv1alpha1.SignalProcessing
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), &fetched)
			Expect(err).ToNot(HaveOccurred())
			Expect(fetched.Spec.Signal.Name).To(Equal("TestSignal"))

			// Delete
			err = k8sClient.Delete(ctx, sp)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
