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

// Package remediationorchestrator contains unit tests for the Remediation Orchestrator.
package remediationorchestrator

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
)

var _ = Describe("SignalProcessingCreator (BR-ORCH-025, BR-ORCH-031)", func() {
	var (
		scheme     *runtime.Scheme
		ctx        context.Context
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		_ = remediationv1.AddToScheme(scheme)
		_ = signalprocessingv1.AddToScheme(scheme)
		ctx = context.Background()
	})

	Describe("NewSignalProcessingCreator", func() {
		It("should return a non-nil creator when given valid dependencies", func() {
			// Arrange
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

			// Act
			spCreator := creator.NewSignalProcessingCreator(fakeClient, scheme)

			// Assert
			Expect(spCreator).ToNot(BeNil())
		})
	})

	Describe("Create", func() {
		Context("when creating a new SignalProcessing CRD", func() {
			It("should generate name in format 'sp-{rr.Name}'", func() {
				// Arrange
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				spCreator := creator.NewSignalProcessingCreator(fakeClient, scheme)
				rr := &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-remediation",
						Namespace: "default",
						UID:       "test-uid-123",
					},
					Spec: remediationv1.RemediationRequestSpec{
						SignalFingerprint: "abc123fingerprint",
						SignalName:        "TestSignal",
						Severity:          "warning",
						Environment:       "production",
						Priority:          "P1",
						SignalType:        "kubernetes-event",
						TargetType:        "kubernetes",
						TargetResource: remediationv1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: "default",
						},
					},
				}

				// Act
				name, err := spCreator.Create(ctx, rr)

				// Assert
				Expect(err).ToNot(HaveOccurred())
				Expect(name).To(Equal("sp-test-remediation"))
			})

			It("should set owner reference for cascade deletion (BR-ORCH-031)", func() {
				// Arrange
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				spCreator := creator.NewSignalProcessingCreator(fakeClient, scheme)
				rr := &remediationv1.RemediationRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-remediation",
						Namespace: "default",
						UID:       "test-uid-123",
					},
					Spec: remediationv1.RemediationRequestSpec{
						SignalFingerprint: "abc123fingerprint",
						SignalName:        "TestSignal",
						Severity:          "warning",
						Environment:       "production",
						Priority:          "P1",
						SignalType:        "kubernetes-event",
						TargetType:        "kubernetes",
						TargetResource: remediationv1.ResourceIdentifier{
							Kind:      "Pod",
							Name:      "test-pod",
							Namespace: "default",
						},
					},
				}

				// Act
				name, err := spCreator.Create(ctx, rr)

				// Assert
				Expect(err).ToNot(HaveOccurred())

				// Fetch the created SignalProcessing
				createdSP := &signalprocessingv1.SignalProcessing{}
				err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, createdSP)
				Expect(err).ToNot(HaveOccurred())

				// Verify owner reference
				Expect(createdSP.OwnerReferences).To(HaveLen(1))
				Expect(createdSP.OwnerReferences[0].Name).To(Equal(rr.Name))
				Expect(createdSP.OwnerReferences[0].UID).To(Equal(rr.UID))
			})
		})
	})
})

