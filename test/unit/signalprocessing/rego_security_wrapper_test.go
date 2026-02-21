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
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/rego"
)

// ========================================================================
// REGO SECURITY WRAPPER TESTS - BR-SP-104: Mandatory Label Protection
// ========================================================================
//
// Business Requirement: BR-SP-104
// Authoritative Reference: DD-WORKFLOW-001 v1.9
//
// Test Matrix (2 tests):
//   - Security: CL-SEC-01, CL-SEC-02
//
// Reserved Prefixes (labels with these prefixes are stripped from rego output):
//   - kubernaut.ai/
//   - system/
// ========================================================================

var _ = Describe("Rego Security Wrapper", func() {
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
	createBasicInput := func() *rego.RegoInput {
		return &rego.RegoInput{
			Kubernetes: &sharedtypes.KubernetesContext{
				Namespace:       "test-ns",
				NamespaceLabels: map[string]string{},
			},
			Signal: rego.SignalContext{
				Type:     "pod_crash",
				Severity: "critical",
				Source:   "alertmanager",
			},
		}
	}

	// ========================================================================
	// SECURITY TESTS (CL-SEC-01 to CL-SEC-02) - BR-SP-104
	// ========================================================================

	Context("Security - Mandatory Label Protection", func() {
		// CL-SEC-01: Policy attempts to set kubernaut.ai/ prefix - should be stripped
		It("CL-SEC-01: should strip labels with kubernaut.ai/ prefix (BR-SP-104)", func() {
			// Arrange: Malicious policy attempting to override system labels
			policy := `
package signalprocessing.labels

import rego.v1

# Attempt to override mandatory system labels - should be STRIPPED
labels["kubernaut.ai/environment"] := "hacked"
labels["kubernaut.ai/priority"] := "P0-malicious"
labels["kubernaut.ai/severity"] := "critical-override"

# Valid custom label - should be KEPT
labels["team"] := "payments"
`
			err := engine.LoadPolicy(policy)
			Expect(err).ToNot(HaveOccurred())

			input := createBasicInput()

			// Act
			result, err := engine.EvaluatePolicy(ctx, input)

			// Assert
			Expect(err).ToNot(HaveOccurred())

			// System labels should be STRIPPED (BR-SP-104)
			Expect(result).ToNot(HaveKey("kubernaut.ai/environment"))
			Expect(result).ToNot(HaveKey("kubernaut.ai/priority"))
			Expect(result).ToNot(HaveKey("kubernaut.ai/severity"))

			// Valid custom labels should be KEPT
			Expect(result).To(HaveKey("team"))
			Expect(result["team"]).To(ContainElement("payments"))
		})

		// CL-SEC-02: Policy attempts to set system/ prefix - should be stripped
		It("CL-SEC-02: should strip labels with system/ prefix (BR-SP-104)", func() {
			// Arrange: Malicious policy attempting to use system/ prefix
			policy := `
package signalprocessing.labels

import rego.v1

# Attempt to set system labels - should be STRIPPED
labels["system/internal"] := "bypass"
labels["system/admin"] := "true"

# Valid custom label - should be KEPT
labels["region"] := "us-east-1"
`
			err := engine.LoadPolicy(policy)
			Expect(err).ToNot(HaveOccurred())

			input := createBasicInput()

			// Act
			result, err := engine.EvaluatePolicy(ctx, input)

			// Assert
			Expect(err).ToNot(HaveOccurred())

			// System prefix labels should be STRIPPED (BR-SP-104)
			Expect(result).ToNot(HaveKey("system/internal"))
			Expect(result).ToNot(HaveKey("system/admin"))

			// Valid custom labels should be KEPT
			Expect(result).To(HaveKey("region"))
			Expect(result["region"]).To(ContainElement("us-east-1"))
		})
	})
})
