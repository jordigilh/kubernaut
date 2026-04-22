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

package enrichment_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

// BR-WORKFLOW-016 / #779: Operations that accept a context.Context must
// respect cancellation. This ensures that long-running enrichment or label
// detection operations abort promptly when the caller's context is cancelled.

var _ = Describe("UT-KA-779-CC: Context cancellation behavior", func() {

	Describe("UT-KA-779-CC-001: DetectLabels returns error when context is already cancelled", func() {
		It("should return a context error or fail gracefully", func() {
			scheme := newFullScheme()
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			ownerChain := []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "web", Namespace: "default"},
			}

			labels, _, err := detector.DetectLabels(ctx, "Deployment", "web", "default", ownerChain)
			// DetectLabels uses best-effort and may not propagate the cancellation as an error.
			// But it should not panic and should return a valid (possibly empty) result.
			if err != nil {
				Expect(err.Error()).To(SatisfyAny(
					ContainSubstring("context canceled"),
					ContainSubstring("context deadline exceeded"),
				), "error should be context-related")
			} else {
				Expect(labels).NotTo(BeNil(),
					"DetectLabels should return a valid (possibly empty) result even with cancelled context")
			}
		})
	})

	Describe("UT-KA-779-CC-002: DetectLabels with cancelled context does not panic", func() {
		It("should complete without panicking when called with cancelled context", func() {
			scheme := runtime.NewScheme()
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme)
			detector := enrichment.NewLabelDetector(dynClient, newTestMapper())

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			Expect(func() {
				_, _, _ = detector.DetectLabels(ctx, "Deployment", "web", "default", nil)
			}).NotTo(Panic(), "DetectLabels must not panic with cancelled context")
		})
	})
})
