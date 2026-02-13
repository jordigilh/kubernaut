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

var _ = Describe("Spec Hash Computer (BR-EM-004)", func() {

	var computer hash.Computer

	BeforeEach(func() {
		computer = hash.NewComputer()
	})

	// ========================================
	// UT-EM-SH-001: Compute valid hash from spec JSON
	// ========================================
	Describe("Compute (UT-EM-SH-001 through UT-EM-SH-005)", func() {

		It("UT-EM-SH-001: should compute a valid SHA-256 hash from spec JSON", func() {
			input := hash.SpecHashInput{
				SpecJSON: []byte(`{"replicas":3,"template":{"spec":{"containers":[{"name":"app","image":"v1.2.3"}]}}}`),
			}

			result := computer.Compute(input)
			Expect(result.Hash).ToNot(BeEmpty())
			Expect(result.Hash).To(HaveLen(64)) // SHA-256 hex string is 64 chars
			Expect(result.Component.Assessed).To(BeTrue())
			Expect(result.Component.Component).To(Equal(types.ComponentHash))
		})

		// UT-EM-SH-002: Deterministic - same input always produces same hash
		It("UT-EM-SH-002: should be deterministic - same spec produces same hash", func() {
			input := hash.SpecHashInput{
				SpecJSON: []byte(`{"replicas":3,"template":{"spec":{"containers":[{"name":"app","image":"v1.2.3"}]}}}`),
			}

			result1 := computer.Compute(input)
			result2 := computer.Compute(input)
			Expect(result1.Hash).To(Equal(result2.Hash))
		})

		// UT-EM-SH-003: Different specs produce different hashes
		It("UT-EM-SH-003: should produce different hashes for different specs", func() {
			input1 := hash.SpecHashInput{
				SpecJSON: []byte(`{"replicas":3}`),
			}
			input2 := hash.SpecHashInput{
				SpecJSON: []byte(`{"replicas":5}`),
			}

			result1 := computer.Compute(input1)
			result2 := computer.Compute(input2)
			Expect(result1.Hash).ToNot(Equal(result2.Hash))
		})

		// UT-EM-SH-004: Empty spec JSON produces valid hash
		It("UT-EM-SH-004: should handle empty spec JSON", func() {
			input := hash.SpecHashInput{
				SpecJSON: []byte(`{}`),
			}

			result := computer.Compute(input)
			Expect(result.Hash).ToNot(BeEmpty())
			Expect(result.Hash).To(HaveLen(64))
			Expect(result.Component.Assessed).To(BeTrue())
		})

		// UT-EM-SH-005: Nil spec JSON produces valid (empty) hash
		It("UT-EM-SH-005: should handle nil spec JSON", func() {
			input := hash.SpecHashInput{
				SpecJSON: nil,
			}

			result := computer.Compute(input)
			// Nil input should still produce a deterministic hash (hash of empty bytes)
			Expect(result.Hash).ToNot(BeEmpty())
			Expect(result.Component.Assessed).To(BeTrue())
		})

		// Edge case: large spec
		It("should handle large spec JSON", func() {
			// Build a large JSON string
			largeJSON := `{"data":"`
			for i := 0; i < 10000; i++ {
				largeJSON += "a"
			}
			largeJSON += `"}`

			input := hash.SpecHashInput{
				SpecJSON: []byte(largeJSON),
			}

			result := computer.Compute(input)
			Expect(result.Hash).To(HaveLen(64))
			Expect(result.Component.Assessed).To(BeTrue())
		})
	})
})
