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
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/signalprocessing"
)

var _ = Describe("SignalProcessing Controller", func() {
	var (
		ctx        context.Context
		reconciler *signalprocessing.SignalProcessingReconciler
		scheme     *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(signalprocessingv1alpha1.AddToScheme(scheme)).To(Succeed())
	})

	// Test 1: Reconciler should handle non-existent resource gracefully
	// ADR-004: Fake K8s Client for unit tests
	It("should handle non-existent resource gracefully", func() {
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		reconciler = &signalprocessing.SignalProcessingReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}

		result, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      "non-existent",
				Namespace: "default",
			},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Requeue).To(BeFalse())
	})

	// Test 2: Reconciler should initialize status for new resource
	It("should initialize status for new resource", func() {
		sp := &signalprocessingv1alpha1.SignalProcessing{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-sp",
				Namespace: "default",
			},
			Spec: signalprocessingv1alpha1.SignalProcessingSpec{
				SignalFingerprint: "abc123def456abc123def456abc123def456abc123def456abc123def456abcd",
				SignalName:        "HighMemoryUsage",
				Severity:          "warning",
				Environment:       "production",
				Priority:          "P2",
				SignalType:        "prometheus",
				TargetType:        "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "my-pod",
					Namespace: "default",
				},
				ReceivedTime: metav1.Now(),
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(sp).
			WithStatusSubresource(&signalprocessingv1alpha1.SignalProcessing{}).
			Build()

		reconciler = &signalprocessing.SignalProcessingReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}

		result, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      "test-sp",
				Namespace: "default",
			},
		})

		Expect(err).NotTo(HaveOccurred())

		// Fetch updated resource
		updated := &signalprocessingv1alpha1.SignalProcessing{}
		err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-sp", Namespace: "default"}, updated)
		Expect(err).NotTo(HaveOccurred())

		// Verify status was initialized
		Expect(updated.Status.Phase).To(Equal("enriching"))
		Expect(updated.Status.StartTime).NotTo(BeNil())

		// Should not requeue immediately (will continue processing)
		Expect(result.Requeue).To(BeFalse())
	})

	// Test 3: Reconciler should skip completed resources
	It("should skip completed resources", func() {
		sp := &signalprocessingv1alpha1.SignalProcessing{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "completed-sp",
				Namespace: "default",
			},
			Spec: signalprocessingv1alpha1.SignalProcessingSpec{
				SignalFingerprint: "abc123def456abc123def456abc123def456abc123def456abc123def456abcd",
				SignalName:        "HighMemoryUsage",
				Severity:          "warning",
				Environment:       "production",
				Priority:          "P2",
				SignalType:        "prometheus",
				TargetType:        "kubernetes",
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "my-pod",
					Namespace: "default",
				},
				ReceivedTime: metav1.Now(),
			},
			Status: signalprocessingv1alpha1.SignalProcessingStatus{
				Phase: "completed",
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(sp).
			WithStatusSubresource(&signalprocessingv1alpha1.SignalProcessing{}).
			Build()

		reconciler = &signalprocessing.SignalProcessingReconciler{
			Client: fakeClient,
			Scheme: scheme,
		}

		result, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      "completed-sp",
				Namespace: "default",
			},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Requeue).To(BeFalse())
	})
})

