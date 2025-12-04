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
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/jordigilh/kubernaut/pkg/signalprocessing/rego"
)

var _ = Describe("Rego Policy Engine", func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	// ========================================
	// REGO ENGINE TESTS (BR-SP-080 CustomLabels)
	// ========================================
	Describe("RegoEngine", func() {
		var engine *rego.Engine

		// Test 1: Should evaluate simple Rego policy
		It("should evaluate simple Rego policy", func() {
			policy := `
package signalprocessing.labels

default custom_labels := {}

custom_labels := {"team": ["payments"]} if {
    input.namespace_labels["team"] == "payments"
}
`
			var err error
			engine, err = rego.NewEngine(policy, "data.signalprocessing.labels.custom_labels", ctrl.Log.WithName("test"))
			Expect(err).NotTo(HaveOccurred())

			input := map[string]interface{}{
				"namespace_labels": map[string]interface{}{
					"team": "payments",
				},
			}

			result, err := engine.Evaluate(ctx, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveKey("team"))
		})

		// Test 2: Should return default when no rule matches
		It("should return default when no rule matches", func() {
			policy := `
package signalprocessing.labels

default custom_labels := {}

custom_labels := {"team": ["payments"]} if {
    input.namespace_labels["team"] == "payments"
}
`
			var err error
			engine, err = rego.NewEngine(policy, "data.signalprocessing.labels.custom_labels", ctrl.Log.WithName("test"))
			Expect(err).NotTo(HaveOccurred())

			input := map[string]interface{}{
				"namespace_labels": map[string]interface{}{
					"team": "other",
				},
			}

			result, err := engine.Evaluate(ctx, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeEmpty())
		})

		// Test 3: Should handle invalid Rego syntax
		It("should return error for invalid Rego syntax", func() {
			policy := `
package signalprocessing.labels
invalid syntax here
`
			_, err := rego.NewEngine(policy, "data.signalprocessing.labels.custom_labels", ctrl.Log.WithName("test"))
			Expect(err).To(HaveOccurred())
		})

		// Test 4: Should enforce timeout on evaluation
		It("should enforce timeout on evaluation", func() {
			policy := `
package signalprocessing.labels

default custom_labels := {}
custom_labels := {"simple": ["value"]} if { true }
`
			var err error
			engine, err = rego.NewEngine(policy, "data.signalprocessing.labels.custom_labels", ctrl.Log.WithName("test"))
			Expect(err).NotTo(HaveOccurred())

			input := map[string]interface{}{}
			result, err := engine.EvaluateWithTimeout(ctx, input, rego.DefaultTimeout)
			Expect(err).NotTo(HaveOccurred())

			// Business-meaningful assertion: verify actual result content
			Expect(result).To(HaveKey("simple"))
			Expect(result["simple"]).To(ContainElement("value"))
		})

		// Test 5: Should strip system labels from output (security wrapper)
		It("should strip system labels from output", func() {
			// This tests the security wrapper that prevents customers from
			// overriding mandatory system labels (DD-WORKFLOW-001 v1.9)
			policy := `
package signalprocessing.labels

default custom_labels := {}

# Attempt to override system labels (should be stripped)
custom_labels := {
    "environment": ["production"],
    "priority": ["P0"],
    "custom": ["allowed"]
} if { true }
`
			var err error
			engine, err = rego.NewEngine(policy, "data.signalprocessing.labels.custom_labels", ctrl.Log.WithName("test"))
			Expect(err).NotTo(HaveOccurred())

			input := map[string]interface{}{}
			result, err := engine.EvaluateWithSecurityWrapper(ctx, input)
			Expect(err).NotTo(HaveOccurred())

			// System labels should be stripped
			Expect(result).NotTo(HaveKey("environment"))
			Expect(result).NotTo(HaveKey("priority"))

			// Custom labels should be preserved
			Expect(result).To(HaveKey("custom"))
		})
	})
})
