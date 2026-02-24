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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/rego"
)

// ========================================================================
// REGO ENGINE UNIT TESTS - BR-SP-102: CustomLabels Rego Extraction
// ========================================================================
//
// Business Requirement: BR-SP-102
// Authoritative Reference: DD-WORKFLOW-001 v1.9
//
// Test Matrix (17 tests):
//   - Happy Path: 7 tests (CL-HP-01 to CL-HP-07)
//   - Edge Cases: 6 tests (CL-EC-01 to CL-EC-06)
//   - Error Handling: 4 tests (CL-ER-01 to CL-ER-04)
//
// Validation Limits (DD-WORKFLOW-001 v1.9):
//   - Max keys (subdomains): 10
//   - Max values per key: 5
//   - Max key length: 63 chars
//   - Max value length: 100 chars
//
// Sandbox Requirements:
//   - Evaluation timeout: 5 seconds
//   - Memory limit: 128 MB (enforced at runtime)
//   - Network access: Disabled
//   - Filesystem access: Disabled
// ========================================================================

var _ = Describe("Rego Engine", func() {
	var (
		engine *rego.Engine
		ctx    context.Context
		cancel context.CancelFunc
		logger = zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true))
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		engine = rego.NewEngine(logger, "/tmp/test-policy.rego")
	})

	AfterEach(func() {
		cancel()
	})

	// Helper to create a basic RegoInput
	// Issue #113: KubernetesContext uses Namespace *NamespaceContext (not flat string/labels)
	createBasicInput := func(namespace string, labels map[string]string) *rego.RegoInput {
		return &rego.RegoInput{
			Kubernetes: &sharedtypes.KubernetesContext{
				Namespace: &sharedtypes.NamespaceContext{
					Name:   namespace,
					Labels: labels,
				},
			},
			Signal: rego.SignalContext{
				Type:     "pod_crash",
				Severity: "critical",
				Source:   "alertmanager",
			},
		}
	}

	// ========================================================================
	// CONSTRUCTOR TEST
	// ========================================================================

	It("should create a new Rego Engine with valid dependencies", func() {
		Expect(engine).To(BeAssignableToTypeOf(&rego.Engine{}))
	})

	// ========================================================================
	// HAPPY PATH TESTS (CL-HP-01 to CL-HP-05)
	// ========================================================================

	Context("Happy Path", func() {
		// CL-HP-01: Extract team from namespace label using signalprocessing.customlabels package
		It("CL-HP-01: should extract team from namespace label (BR-SP-102)", func() {
			// Arrange: Policy uses signalprocessing.customlabels — the dedicated custom labels package
			policy := `
package signalprocessing.customlabels

import rego.v1

labels["team"] := input.kubernetes.namespace.labels["kubernaut.ai/team"] if {
    input.kubernetes.namespace.labels["kubernaut.ai/team"]
}
`
			err := engine.LoadPolicy(policy)
			Expect(err).ToNot(HaveOccurred())

			input := createBasicInput("prod-payments", map[string]string{
				"kubernaut.ai/team": "payments",
			})

			// Act
			result, err := engine.EvaluatePolicy(ctx, input)

			// Assert
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKey("team"))
			Expect(result["team"]).To(ContainElement("payments"))
		})

		// CL-HP-02: Extract risk_tolerance based on severity
		It("CL-HP-02: should set risk_tolerance based on severity (BR-SP-102)", func() {
			// Arrange: Policy that sets risk_tolerance based on signal severity
			policy := `
package signalprocessing.customlabels

import rego.v1

labels["risk_tolerance"] := "low" if {
    input.signal.severity == "critical"
}

labels["risk_tolerance"] := "high" if {
    input.signal.severity != "critical"
}
`
			err := engine.LoadPolicy(policy)
			Expect(err).ToNot(HaveOccurred())

			input := createBasicInput("prod-api", map[string]string{})
			input.Signal.Severity = "critical"

			// Act
			result, err := engine.EvaluatePolicy(ctx, input)

			// Assert
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKey("risk_tolerance"))
			Expect(result["risk_tolerance"]).To(ContainElement("low"))
		})

		// CL-HP-03: Multi-subdomain extraction
		It("CL-HP-03: should extract multiple subdomains (BR-SP-102)", func() {
			// Arrange: Policy that extracts multiple subdomains
			policy := `
package signalprocessing.customlabels

import rego.v1

labels["team"] := input.kubernetes.namespace.labels["kubernaut.ai/team"] if {
    input.kubernetes.namespace.labels["kubernaut.ai/team"]
}

labels["region"] := input.kubernetes.namespace.labels["kubernaut.ai/region"] if {
    input.kubernetes.namespace.labels["kubernaut.ai/region"]
}

labels["tier"] := "critical" if {
    input.signal.severity == "critical"
}
`
			err := engine.LoadPolicy(policy)
			Expect(err).ToNot(HaveOccurred())

			input := createBasicInput("prod-payments", map[string]string{
				"kubernaut.ai/team":   "payments",
				"kubernaut.ai/region": "us-east-1",
			})
			input.Signal.Severity = "critical"

			// Act
			result, err := engine.EvaluatePolicy(ctx, input)

			// Assert
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKey("team"))
			Expect(result).To(HaveKey("region"))
			Expect(result).To(HaveKey("tier"))
		})

		// CL-HP-04: Multi-value per subdomain
		It("CL-HP-04: should handle multi-value per subdomain (BR-SP-102)", func() {
			// Arrange: Policy that returns array of constraints
			policy := `
package signalprocessing.customlabels

import rego.v1

labels["constraint"] := ["cost-aware", "stateful-safe"] if {
    input.signal.severity == "critical"
}
`
			err := engine.LoadPolicy(policy)
			Expect(err).ToNot(HaveOccurred())

			input := createBasicInput("prod-db", map[string]string{})

			// Act
			result, err := engine.EvaluatePolicy(ctx, input)

			// Assert
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKey("constraint"))
			Expect(result["constraint"]).To(ContainElements("cost-aware", "stateful-safe"))
		})

		// CL-HP-05: Hot-reload triggers callback
		It("CL-HP-05: should support policy reload (BR-SP-102)", func() {
			// Arrange: Initial policy
			initialPolicy := `
package signalprocessing.customlabels

import rego.v1

labels["version"] := "v1"
`
			err := engine.LoadPolicy(initialPolicy)
			Expect(err).ToNot(HaveOccurred())

			input := createBasicInput("test-ns", map[string]string{})

			// Act: First evaluation
			result1, err := engine.EvaluatePolicy(ctx, input)
			Expect(err).ToNot(HaveOccurred())
			Expect(result1["version"]).To(ContainElement("v1"))

			// Reload with new policy
			newPolicy := `
package signalprocessing.customlabels

import rego.v1

labels["version"] := "v2"
`
			err = engine.LoadPolicy(newPolicy)
			Expect(err).ToNot(HaveOccurred())

			// Act: Second evaluation after reload
			result2, err := engine.EvaluatePolicy(ctx, input)

			// Assert
			Expect(err).ToNot(HaveOccurred())
			Expect(result2["version"]).To(ContainElement("v2"))
		})

		// CL-HP-06: Dynamic extraction from namespace labels via kubernaut.ai/ prefix (Issue #174)
		// This is the pattern used by the suite default policy and demo ConfigMaps.
		// Validates the Rego engine path (not the fallback/degraded mode path).
		It("CL-HP-06: should dynamically extract kubernaut.ai/ namespace labels (BR-SP-102)", func() {
			policy := `
package signalprocessing.customlabels

import rego.v1

labels := result if {
    input.kubernetes.namespace.labels
    result := {key: [val] |
        some full_key, val in input.kubernetes.namespace.labels
        startswith(full_key, "kubernaut.ai/")
        key := substring(full_key, count("kubernaut.ai/"), -1)
    }
    count(result) > 0
} else := {}
`
			err := engine.LoadPolicy(policy)
			Expect(err).ToNot(HaveOccurred())

			input := createBasicInput("prod-payments", map[string]string{
				"kubernaut.ai/team":           "payments",
				"kubernaut.ai/risk-tolerance": "high",
				"unrelated-label":             "should-be-ignored",
			})

			result, err := engine.EvaluatePolicy(ctx, input)

			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveLen(2), "Should extract exactly the 2 kubernaut.ai/ labels")
			Expect(result).To(HaveKeyWithValue("team", ContainElement("payments")))
			Expect(result).To(HaveKeyWithValue("risk-tolerance", ContainElement("high")))
		})

		// CL-HP-07: Namespace with no kubernaut.ai/ labels returns empty via Rego (Issue #174)
		It("CL-HP-07: should return empty when no kubernaut.ai/ labels present (BR-SP-102)", func() {
			policy := `
package signalprocessing.customlabels

import rego.v1

labels := result if {
    input.kubernetes.namespace.labels
    result := {key: [val] |
        some full_key, val in input.kubernetes.namespace.labels
        startswith(full_key, "kubernaut.ai/")
        key := substring(full_key, count("kubernaut.ai/"), -1)
    }
    count(result) > 0
} else := {}
`
			err := engine.LoadPolicy(policy)
			Expect(err).ToNot(HaveOccurred())

			input := createBasicInput("test-ns", map[string]string{
				"app":         "my-app",
				"environment": "staging",
			})

			result, err := engine.EvaluatePolicy(ctx, input)

			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(BeEmpty(), "No kubernaut.ai/ labels should mean empty CustomLabels")
		})
	})

	// ========================================================================
	// EDGE CASE TESTS (CL-EC-01 to CL-EC-06)
	// ========================================================================

	Context("Edge Cases", func() {
		// CL-EC-01: Empty policy
		It("CL-EC-01: should return empty map for empty policy (BR-SP-102)", func() {
			// Arrange: No policy loaded (empty)
			// Don't load any policy

			input := createBasicInput("test-ns", map[string]string{})

			// Act
			result, err := engine.EvaluatePolicy(ctx, input)

			// Assert
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(BeEmpty())
		})

		// CL-EC-02: Policy returns nil/empty
		It("CL-EC-02: should return empty map when policy returns nil (BR-SP-102)", func() {
			// Arrange: Policy that produces no labels
			policy := `
package signalprocessing.customlabels

import rego.v1

# No labels rule - returns empty
`
			err := engine.LoadPolicy(policy)
			Expect(err).ToNot(HaveOccurred())

			input := createBasicInput("test-ns", map[string]string{})

			// Act
			result, err := engine.EvaluatePolicy(ctx, input)

			// Assert
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(BeEmpty())
		})

		// CL-EC-03: Max keys (10) exceeded - truncation
		It("CL-EC-03: should truncate to 10 keys when exceeded (BR-SP-102)", func() {
			// Arrange: Policy that returns 15 keys
			policy := `
package signalprocessing.customlabels

import rego.v1

labels["key01"] := "value1"
labels["key02"] := "value2"
labels["key03"] := "value3"
labels["key04"] := "value4"
labels["key05"] := "value5"
labels["key06"] := "value6"
labels["key07"] := "value7"
labels["key08"] := "value8"
labels["key09"] := "value9"
labels["key10"] := "value10"
labels["key11"] := "value11"
labels["key12"] := "value12"
labels["key13"] := "value13"
labels["key14"] := "value14"
labels["key15"] := "value15"
`
			err := engine.LoadPolicy(policy)
			Expect(err).ToNot(HaveOccurred())

			input := createBasicInput("test-ns", map[string]string{})

			// Act
			result, err := engine.EvaluatePolicy(ctx, input)

			// Assert
			Expect(err).ToNot(HaveOccurred())
			Expect(len(result)).To(Equal(10)) // Truncated to max 10 keys
		})

		// CL-EC-04: Max values (5) per key exceeded - truncation
		It("CL-EC-04: should truncate to 5 values per key when exceeded (BR-SP-102)", func() {
			// Arrange: Policy that returns 8 values for one key
			policy := `
package signalprocessing.customlabels

import rego.v1

labels["constraint"] := ["v1", "v2", "v3", "v4", "v5", "v6", "v7", "v8"]
`
			err := engine.LoadPolicy(policy)
			Expect(err).ToNot(HaveOccurred())

			input := createBasicInput("test-ns", map[string]string{})

			// Act
			result, err := engine.EvaluatePolicy(ctx, input)

			// Assert
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKey("constraint"))
			Expect(len(result["constraint"])).To(Equal(5)) // Truncated to max 5 values
		})

		// CL-EC-05: Key length > 63 chars - truncation
		It("CL-EC-05: should truncate key longer than 63 chars (BR-SP-102)", func() {
			// Arrange: Policy with key > 63 chars
			longKey := strings.Repeat("a", 70) // 70 chars
			policy := `
package signalprocessing.customlabels

import rego.v1

labels["` + longKey + `"] := "value"
`
			err := engine.LoadPolicy(policy)
			Expect(err).ToNot(HaveOccurred())

			input := createBasicInput("test-ns", map[string]string{})

			// Act
			result, err := engine.EvaluatePolicy(ctx, input)

			// Assert
			Expect(err).ToNot(HaveOccurred())
			// Key should be truncated to 63 chars
			for key := range result {
				Expect(len(key)).To(BeNumerically("<=", 63))
			}
		})

		// CL-EC-06: Value length > 100 chars - truncation
		It("CL-EC-06: should truncate value longer than 100 chars (BR-SP-102)", func() {
			// Arrange: Policy with value > 100 chars
			longValue := strings.Repeat("x", 120) // 120 chars
			policy := `
package signalprocessing.customlabels

import rego.v1

labels["key"] := "` + longValue + `"
`
			err := engine.LoadPolicy(policy)
			Expect(err).ToNot(HaveOccurred())

			input := createBasicInput("test-ns", map[string]string{})

			// Act
			result, err := engine.EvaluatePolicy(ctx, input)

			// Assert
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKey("key"))
			Expect(len(result["key"][0])).To(BeNumerically("<=", 100))
		})
	})

	// ========================================================================
	// ERROR HANDLING TESTS (CL-ER-01 to CL-ER-03)
	// ========================================================================

	Context("Error Handling", func() {
		// CL-ER-01: Rego syntax error
		It("CL-ER-01: should return error for Rego syntax error (BR-SP-102)", func() {
			// Arrange: Invalid Rego policy
			invalidPolicy := `
package signalprocessing.customlabels

import rego.v1

# Missing closing brace - syntax error
labels["key"] := { if {
`
			err := engine.LoadPolicy(invalidPolicy)
			// Note: LoadPolicy may or may not catch syntax errors
			// The error should be caught during evaluation
			if err == nil {
				input := createBasicInput("test-ns", map[string]string{})

				// Act
				_, err = engine.EvaluatePolicy(ctx, input)

				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("rego"))
			} else {
				// If LoadPolicy catches the error, that's also acceptable
				Expect(err).To(HaveOccurred())
			}
		})

		// CL-ER-02: Rego timeout (>5s)
		It("CL-ER-02: should return timeout error for slow policy (BR-SP-102)", func() {
			// Arrange: Policy that would take too long (simulate with context)
			policy := `
package signalprocessing.customlabels

import rego.v1

labels["key"] := "value"
`
			err := engine.LoadPolicy(policy)
			Expect(err).ToNot(HaveOccurred())

			// Create already-cancelled context to simulate timeout
			cancelledCtx, cancelFunc := context.WithTimeout(ctx, 1*time.Nanosecond)
			defer cancelFunc()
			time.Sleep(10 * time.Millisecond) // Ensure context is cancelled

			input := createBasicInput("test-ns", map[string]string{})

			// Act
			_, err = engine.EvaluatePolicy(cancelledCtx, input)

			// Assert
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Or(
				ContainSubstring("context"),
				ContainSubstring("deadline"),
				ContainSubstring("canceled"),
			))
		})

		// CL-ER-03: Policies must use package signalprocessing.customlabels (not signalprocessing.labels)
		// The engine queries data.signalprocessing.customlabels.labels, so policies using the
		// wrong package name should fail validation or return empty results.
		It("CL-ER-03: should reject policies using wrong package name (BR-SP-102)", func() {
			wrongPackagePolicy := `
package signalprocessing.labels

import rego.v1

labels["team"] := "should-not-work"
`
			err := engine.LoadPolicy(wrongPackagePolicy)
			if err == nil {
				input := createBasicInput("test-ns", map[string]string{})
				result, evalErr := engine.EvaluatePolicy(ctx, input)
				Expect(evalErr).ToNot(HaveOccurred())
				Expect(result).To(BeEmpty(), "Policy with wrong package should produce no labels")
			}
		})

		// CL-ER-04: Invalid output type
		It("CL-ER-04: should return error for invalid output type (BR-SP-102)", func() {
			// Arrange: Policy that returns invalid type (number instead of string)
			policy := `
package signalprocessing.customlabels

import rego.v1

# Returns a number instead of string - should cause type error
labels["invalid"] := 12345
`
			err := engine.LoadPolicy(policy)
			Expect(err).ToNot(HaveOccurred())

			input := createBasicInput("test-ns", map[string]string{})

			// Act
			result, err := engine.EvaluatePolicy(ctx, input)

			// Assert: Either returns error or filters out invalid value
			if err != nil {
				Expect(err.Error()).To(Or(
					ContainSubstring("type"),
					ContainSubstring("invalid"),
				))
			} else {
				// If no error, the invalid value should be filtered out
				Expect(result).ToNot(HaveKey("invalid"))
			}
		})
	})
})
