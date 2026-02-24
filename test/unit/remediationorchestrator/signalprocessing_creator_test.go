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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

var _ = Describe("SignalProcessingCreator", func() {
	var (
		scheme *runtime.Scheme
		ctx    context.Context
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		_ = remediationv1.AddToScheme(scheme)
		_ = signalprocessingv1.AddToScheme(scheme)
		ctx = context.Background()
	})

	Describe("NewSignalProcessingCreator", func() {
		It("should return a non-nil creator to enable BR-ORCH-025 data pass-through", func() {
			// Arrange
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

			// Act
			spCreator := creator.NewSignalProcessingCreator(fakeClient, scheme, nil)

			// Assert
			Expect(spCreator).ToNot(BeNil())
		})
	})

	Describe("Create", func() {
		Context("BR-ORCH-025: Data pass-through to child CRDs", func() {
			It("should generate deterministic name in format 'sp-{rr.Name}' for reliable tracking", func() {
				// Arrange - use testutil factory
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				spCreator := creator.NewSignalProcessingCreator(fakeClient, scheme, nil)
				rr := helpers.NewRemediationRequest("test-remediation", "default")

				// Act
				name, err := spCreator.Create(ctx, rr)

				// Assert
				Expect(err).ToNot(HaveOccurred())
				Expect(name).To(Equal("sp-test-remediation"))
			})

			It("should be idempotent - return existing name on retry without creating duplicate", func() {
				// Arrange - pre-create the SignalProcessing
				existingSP := &signalprocessingv1.SignalProcessing{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "sp-test-remediation",
						Namespace: "default",
					},
				}
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingSP).Build()
				spCreator := creator.NewSignalProcessingCreator(fakeClient, scheme, nil)
				rr := helpers.NewRemediationRequest("test-remediation", "default")

				// Act
				name, err := spCreator.Create(ctx, rr)

				// Assert
				Expect(err).ToNot(HaveOccurred())
				Expect(name).To(Equal("sp-test-remediation"))
			})

			It("should build correct SignalProcessing spec with all signal data passed through", func() {
				// Arrange - use testutil factory with custom options
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				spCreator := creator.NewSignalProcessingCreator(fakeClient, scheme, nil)
				rr := helpers.NewRemediationRequest("test-remediation", "default", helpers.RemediationRequestOpts{
					Severity:    "critical",
					Priority:    "P0",
					Environment: "production",
					SignalType:  "alert",
				})
				rr.Spec.SignalLabels = map[string]string{"label1": "value1"}

				// Act
				name, err := spCreator.Create(ctx, rr)

				// Assert
				Expect(err).ToNot(HaveOccurred())

				// Fetch created SP and verify spec
				createdSP := &signalprocessingv1.SignalProcessing{}
				err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: rr.Namespace}, createdSP)
				Expect(err).ToNot(HaveOccurred())

				// Verify RemediationRequestRef
				Expect(createdSP.Spec.RemediationRequestRef.Name).To(Equal(rr.Name))
				Expect(createdSP.Spec.RemediationRequestRef.Namespace).To(Equal(rr.Namespace))
				Expect(createdSP.Spec.RemediationRequestRef.Kind).To(Equal("RemediationRequest"))

				// Verify Signal data pass-through
				Expect(createdSP.Spec.Signal.Fingerprint).To(Equal(rr.Spec.SignalFingerprint))
				Expect(createdSP.Spec.Signal.Name).To(Equal(rr.Spec.SignalName))
				Expect(createdSP.Spec.Signal.Severity).To(Equal(rr.Spec.Severity))
				Expect(createdSP.Spec.Signal.Type).To(Equal(rr.Spec.SignalType))
				Expect(createdSP.Spec.Signal.TargetType).To(Equal(rr.Spec.TargetType))

				// Verify TargetResource
				Expect(createdSP.Spec.Signal.TargetResource.Kind).To(Equal(rr.Spec.TargetResource.Kind))
				Expect(createdSP.Spec.Signal.TargetResource.Name).To(Equal(rr.Spec.TargetResource.Name))
				Expect(createdSP.Spec.Signal.TargetResource.Namespace).To(Equal(rr.Spec.TargetResource.Namespace))

				// Verify Labels pass-through
				Expect(createdSP.Spec.Signal.Labels).To(HaveKeyWithValue("label1", "value1"))
			})
		})

		Context("BR-ORCH-031: Cascade deletion via owner references", func() {
			It("should set owner reference for automatic cleanup when RemediationRequest is deleted", func() {
				// Arrange - use testutil factory
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				spCreator := creator.NewSignalProcessingCreator(fakeClient, scheme, nil)
				rr := helpers.NewRemediationRequest("test-remediation", "default")

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

		// BR-ORCH-025: Error handling ensures failures are propagated correctly
		Context("BR-ORCH-025: Error handling for infrastructure failures", func() {
			DescribeTable("should return appropriate error when client operations fail",
				func(errorType string, interceptFunc interceptor.Funcs, expectedError string) {
					// Arrange
					fakeClient := fake.NewClientBuilder().WithScheme(scheme).
						WithInterceptorFuncs(interceptFunc).Build()
					spCreator := creator.NewSignalProcessingCreator(fakeClient, scheme, nil)
					rr := helpers.NewRemediationRequest("test-remediation", "default")

					// Act
					_, err := spCreator.Create(ctx, rr)

					// Assert
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(expectedError))
				},
				Entry("Get fails with non-NotFound error - propagates to allow RO to mark RR as Failed",
					"get_error",
					interceptor.Funcs{
						Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
							return fmt.Errorf("network error")
						},
					},
					"failed to check existing SignalProcessing",
				),
				Entry("Create fails with API server error - propagates to allow RO to mark RR as Failed",
					"create_error",
					interceptor.Funcs{
						Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
							return fmt.Errorf("API server unavailable")
						},
					},
					"failed to create SignalProcessing",
				),
			)
		})
	})
})
