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
package signalprocessing

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/enricher"
)

// Unit Test: Degraded mode fallback implementation
// Per IMPLEMENTATION_PLAN_V1.21.md Day 3: degraded.go
// DD-4: K8s Enrichment Failure Handling - use degraded mode
var _ = Describe("Degraded Mode", func() {

	Describe("BuildDegradedContext", func() {

		It("should create context from signal labels", func() {
			signal := &signalprocessingv1alpha1.SignalData{
				Name: "test-signal",
				Labels: map[string]string{
					"app":         "my-app",
					"environment": "production",
					"team":        "platform",
				},
				TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "test-pod",
					Namespace: "test-namespace",
				},
			}

			ctx := enricher.BuildDegradedContext(signal)

			Expect(ctx).NotTo(BeNil())
			Expect(ctx.Namespace).NotTo(BeNil())
			Expect(ctx.Namespace.Labels).To(HaveKeyWithValue("app", "my-app"))
			Expect(ctx.Namespace.Labels).To(HaveKeyWithValue("environment", "production"))
			Expect(ctx.Namespace.Labels).To(HaveKeyWithValue("team", "platform"))
		})

		It("should set DegradedMode to true", func() {
			signal := &signalprocessingv1alpha1.SignalData{
				Name:   "test-signal",
				Labels: map[string]string{"key": "value"},
			}

			ctx := enricher.BuildDegradedContext(signal)

			Expect(ctx.DegradedMode).To(BeTrue())
		})

		It("should build degraded context successfully", func() {
			signal := &signalprocessingv1alpha1.SignalData{
				Name:   "test-signal",
				Labels: map[string]string{"key": "value"},
			}

			ctx := enricher.BuildDegradedContext(signal)

			Expect(ctx).NotTo(BeNil())
			Expect(ctx.DegradedMode).To(BeTrue())
		})

		It("should handle nil labels gracefully", func() {
			signal := &signalprocessingv1alpha1.SignalData{
				Name:   "test-signal",
				Labels: nil,
			}

			ctx := enricher.BuildDegradedContext(signal)

			Expect(ctx).NotTo(BeNil())
			Expect(ctx.Namespace).NotTo(BeNil())
			Expect(ctx.Namespace.Labels).NotTo(BeNil()) // Should be empty map, not nil
			Expect(ctx.DegradedMode).To(BeTrue())
		})

		It("should handle empty labels", func() {
			signal := &signalprocessingv1alpha1.SignalData{
				Name:   "test-signal",
				Labels: map[string]string{},
			}

			ctx := enricher.BuildDegradedContext(signal)

			Expect(ctx).NotTo(BeNil())
			Expect(ctx.Namespace).NotTo(BeNil())
			Expect(ctx.Namespace.Labels).To(BeEmpty())
			Expect(ctx.DegradedMode).To(BeTrue())
		})

		It("should copy signal annotations if present", func() {
			signal := &signalprocessingv1alpha1.SignalData{
				Name:   "test-signal",
				Labels: map[string]string{"key": "value"},
				Annotations: map[string]string{
					"description": "test annotation",
				},
			}

			ctx := enricher.BuildDegradedContext(signal)

			Expect(ctx.Namespace).NotTo(BeNil())
			Expect(ctx.Namespace.Annotations).To(HaveKeyWithValue("description", "test annotation"))
		})
	})

	Describe("Context Size Validation", func() {

		It("should accept context within size limits", func() {
			ctx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Labels: map[string]string{"key": "value"},
				},
			}

			err := enricher.ValidateContextSize(ctx)

			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject context with too many labels", func() {
			// Create context with excessive labels (>100)
			labels := make(map[string]string)
			for i := 0; i < 150; i++ {
				labels[string(rune('a'+i%26))+string(rune(i))] = "value"
			}

			ctx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Labels: labels,
				},
			}

			err := enricher.ValidateContextSize(ctx)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("labels"))
		})

		It("should reject context with label value too long", func() {
			// K8s limit is 63 chars for label values
			longValue := make([]byte, 100)
			for i := range longValue {
				longValue[i] = 'a'
			}

			ctx := &signalprocessingv1alpha1.KubernetesContext{
				Namespace: &signalprocessingv1alpha1.NamespaceContext{
					Labels: map[string]string{"key": string(longValue)},
				},
			}

			err := enricher.ValidateContextSize(ctx)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("value"))
		})

		It("should accept nil context", func() {
			err := enricher.ValidateContextSize(nil)

			Expect(err).NotTo(HaveOccurred())
		})
	})
})
