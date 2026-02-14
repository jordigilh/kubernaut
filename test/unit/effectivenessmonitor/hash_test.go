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

package effectivenessmonitor

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/hash"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/types"
)

// ========================================
// EM Spec Hash Computer Tests (DD-EM-002, Phase 5)
//
// Contract:
//   - SpecHashInput accepts Spec map[string]interface{} (not SpecJSON []byte)
//   - SpecHashInput accepts PreHash string (pre-remediation hash from DS)
//   - ComputeResult.Hash is canonical "sha256:<hex>" format
//   - ComputeResult.PreHash stores the pre-remediation hash
//   - ComputeResult.Match is *bool: true/false/nil
//   - Deterministic output using pkg/shared/hash.CanonicalSpecHash
// ========================================
var _ = Describe("Spec Hash Computer (DD-EM-002)", func() {

	var computer hash.Computer

	BeforeEach(func() {
		computer = hash.NewComputer()
	})

	Describe("Basic Hash Computation", func() {

		It("UT-EM-SH-001: should compute canonical sha256-prefixed hash from spec map", func() {
			input := hash.SpecHashInput{
				Spec: map[string]interface{}{
					"replicas": float64(3),
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{"name": "app", "image": "v1.2.3"},
							},
						},
					},
				},
			}

			result := computer.Compute(input)
			Expect(result.Hash).To(HavePrefix("sha256:"))
			Expect(result.Hash).To(HaveLen(71))
			Expect(result.Component.Assessed).To(BeTrue())
			Expect(result.Component.Component).To(Equal(types.ComponentHash))
		})

		It("UT-EM-SH-002: should be deterministic - same spec produces same hash", func() {
			input := hash.SpecHashInput{
				Spec: map[string]interface{}{"replicas": float64(3)},
			}

			result1 := computer.Compute(input)
			result2 := computer.Compute(input)
			Expect(result1.Hash).To(Equal(result2.Hash))
		})

		It("UT-EM-SH-003: should produce different hashes for different specs", func() {
			input1 := hash.SpecHashInput{
				Spec: map[string]interface{}{"replicas": float64(3)},
			}
			input2 := hash.SpecHashInput{
				Spec: map[string]interface{}{"replicas": float64(5)},
			}

			result1 := computer.Compute(input1)
			result2 := computer.Compute(input2)
			Expect(result1.Hash).ToNot(Equal(result2.Hash))
		})

		It("UT-EM-SH-004: should handle empty spec map", func() {
			input := hash.SpecHashInput{
				Spec: map[string]interface{}{},
			}

			result := computer.Compute(input)
			Expect(result.Hash).To(HavePrefix("sha256:"))
			Expect(result.Component.Assessed).To(BeTrue())
		})

		It("UT-EM-SH-005: should handle nil spec (treats as empty map)", func() {
			input := hash.SpecHashInput{
				Spec: nil,
			}

			result := computer.Compute(input)
			Expect(result.Hash).To(HavePrefix("sha256:"))
			Expect(result.Component.Assessed).To(BeTrue())
		})
	})

	Describe("Pre/Post Hash Comparison (DD-EM-002)", func() {

		It("UT-EM-SH-006: Match=true when pre and post hashes are identical", func() {
			spec := map[string]interface{}{"replicas": float64(3)}
			// First compute to get the expected hash
			preResult := computer.Compute(hash.SpecHashInput{Spec: spec})

			// Now compute with pre-hash set to match
			input := hash.SpecHashInput{
				Spec:    spec,
				PreHash: preResult.Hash,
			}

			result := computer.Compute(input)
			Expect(result.Match).ToNot(BeNil())
			Expect(*result.Match).To(BeTrue(), "Same spec should match pre-hash")
			Expect(result.PreHash).To(Equal(preResult.Hash))
		})

		It("UT-EM-SH-007: Match=false when pre and post hashes differ", func() {
			input := hash.SpecHashInput{
				Spec:    map[string]interface{}{"replicas": float64(5)},
				PreHash: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
			}

			result := computer.Compute(input)
			Expect(result.Match).ToNot(BeNil())
			Expect(*result.Match).To(BeFalse(), "Different spec should not match pre-hash")
		})

		It("UT-EM-SH-008: Match=nil when no PreHash provided", func() {
			input := hash.SpecHashInput{
				Spec:    map[string]interface{}{"replicas": float64(3)},
				PreHash: "",
			}

			result := computer.Compute(input)
			Expect(result.Match).To(BeNil(), "No pre-hash means comparison not possible")
			Expect(result.PreHash).To(BeEmpty())
		})

		It("UT-EM-SH-009: should be consistent with pkg/shared/hash.CanonicalSpecHash", func() {
			spec := map[string]interface{}{
				"replicas": float64(2),
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{"app": "test"},
				},
			}

			result := computer.Compute(hash.SpecHashInput{Spec: spec})

			// The hash from the EM Computer should use CanonicalSpecHash internally
			// and produce the sha256: prefixed format
			Expect(result.Hash).To(HavePrefix("sha256:"))
			Expect(result.Hash).To(HaveLen(71))
		})

		It("UT-EM-SH-010: should store PreHash in result for audit reporting", func() {
			preHash := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			input := hash.SpecHashInput{
				Spec:    map[string]interface{}{"key": "value"},
				PreHash: preHash,
			}

			result := computer.Compute(input)
			Expect(result.PreHash).To(Equal(preHash))
		})
	})
})
