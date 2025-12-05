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

// Package signalprocessing contains unit tests for Signal Processing controller.
// Unit tests validate implementation correctness, not business value delivery.
// See docs/development/business-requirements/TESTING_GUIDELINES.md
package signalprocessing

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/jordigilh/kubernaut/pkg/signalprocessing/enricher"
)

func TestEnricher(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SignalProcessing Enricher Suite")
}

// Unit Test: K8sEnricher implementation correctness
var _ = Describe("K8sEnricher.Enrich", func() {
	var (
		ctx     context.Context
		e       *enricher.K8sEnricher
		scheme  *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
	})

	// Test 1: Basic enrichment with namespace labels
	Context("when enriching with valid pod reference", func() {
		It("should return namespace labels from the referenced pod", func() {
			// Create test namespace with labels
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-namespace",
					Labels: map[string]string{
						"team":        "platform",
						"environment": "production",
					},
				},
			}

			// Create test pod
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "test-namespace",
				},
			}

			// Create fake client with test objects
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(ns, pod).
				Build()

			e = enricher.NewK8sEnricher(fakeClient, ctrl.Log.WithName("test"))

			result, err := e.Enrich(ctx, "test-namespace", "test-pod")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.NamespaceLabels).To(HaveKeyWithValue("team", "platform"))
			Expect(result.NamespaceLabels).To(HaveKeyWithValue("environment", "production"))
		})
	})
})

