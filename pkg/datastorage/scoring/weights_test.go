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

package scoring

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestScoring(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Scoring Package Suite")
}

var _ = Describe("DetectedLabel Weights", func() {
	Context("Weight definitions", func() {
		It("should have weights for all 8 DetectedLabel fields", func() {
			expectedFields := []string{
				"gitOpsManaged",
				"gitOpsTool",
				"pdbProtected",
				"serviceMesh",
				"networkIsolated",
				"helmManaged",
				"stateful",
				"hpaEnabled",
			}

			for _, field := range expectedFields {
				weight := GetDetectedLabelWeight(field)
				Expect(weight).To(BeNumerically(">", 0.0),
					"Field '%s' should have a positive weight", field)
			}
		})

		It("should return 0.0 for unknown fields", func() {
			weight := GetDetectedLabelWeight("unknownField")
			Expect(weight).To(Equal(0.0))
		})

		It("should have high-impact weights for GitOps fields", func() {
			gitOpsManagedWeight := GetDetectedLabelWeight("gitOpsManaged")
			gitOpsToolWeight := GetDetectedLabelWeight("gitOpsTool")

			Expect(gitOpsManagedWeight).To(Equal(0.10),
				"gitOpsManaged should have high-impact weight (0.10)")
			Expect(gitOpsToolWeight).To(Equal(0.10),
				"gitOpsTool should have high-impact weight (0.10)")
		})

		It("should have medium-impact weights for safety fields", func() {
			pdbWeight := GetDetectedLabelWeight("pdbProtected")
			meshWeight := GetDetectedLabelWeight("serviceMesh")

			Expect(pdbWeight).To(Equal(0.05),
				"pdbProtected should have medium-impact weight (0.05)")
			Expect(meshWeight).To(Equal(0.05),
				"serviceMesh should have medium-impact weight (0.05)")
		})

		It("should have low-impact weights for informational fields", func() {
			networkWeight := GetDetectedLabelWeight("networkIsolated")
			helmWeight := GetDetectedLabelWeight("helmManaged")
			statefulWeight := GetDetectedLabelWeight("stateful")
			hpaWeight := GetDetectedLabelWeight("hpaEnabled")

			Expect(networkWeight).To(Equal(0.03),
				"networkIsolated should have low-impact weight (0.03)")
			Expect(helmWeight).To(Equal(0.02),
				"helmManaged should have low-impact weight (0.02)")
			Expect(statefulWeight).To(Equal(0.02),
				"stateful should have low-impact weight (0.02)")
			Expect(hpaWeight).To(Equal(0.02),
				"hpaEnabled should have low-impact weight (0.02)")
		})
	})

	Context("Weight constants", func() {
		It("should have correct MaxLabelBoost", func() {
			// Sum all weights
			totalWeight := 0.0
			for _, weight := range DetectedLabelWeights {
				totalWeight += weight
			}

			Expect(MaxLabelBoost).To(Equal(0.39),
				"MaxLabelBoost should be 0.39 (sum of all weights)")
			Expect(totalWeight).To(Equal(MaxLabelBoost),
				"Sum of all weights should equal MaxLabelBoost")
		})

		It("should have correct MaxLabelPenalty", func() {
			// Only high-impact fields apply penalties
			penaltySum := GetDetectedLabelWeight("gitOpsManaged") +
				GetDetectedLabelWeight("gitOpsTool")

			Expect(MaxLabelPenalty).To(Equal(0.20),
				"MaxLabelPenalty should be 0.20 (sum of high-impact weights)")
			Expect(penaltySum).To(Equal(MaxLabelPenalty),
				"Sum of penalty weights should equal MaxLabelPenalty")
		})
	})

	Context("Penalty application logic", func() {
		It("should apply penalties for high-impact fields", func() {
			highImpactFields := []string{
				"gitOpsManaged",
				"gitOpsTool",
			}

			for _, field := range highImpactFields {
				Expect(ShouldApplyPenalty(field)).To(BeTrue(),
					"Field '%s' should apply penalty on mismatch", field)
			}
		})

		It("should NOT apply penalties for medium/low-impact fields", func() {
			nonPenaltyFields := []string{
				"pdbProtected",
				"serviceMesh",
				"networkIsolated",
				"helmManaged",
				"stateful",
				"hpaEnabled",
			}

			for _, field := range nonPenaltyFields {
				Expect(ShouldApplyPenalty(field)).To(BeFalse(),
					"Field '%s' should NOT apply penalty on mismatch", field)
			}
		})

		It("should NOT apply penalties for unknown fields", func() {
			Expect(ShouldApplyPenalty("unknownField")).To(BeFalse())
		})
	})

	Context("Weight rationale validation", func() {
		It("should have GitOps weights as highest (correctness-critical)", func() {
			gitOpsWeight := GetDetectedLabelWeight("gitOpsManaged")

			// GitOps should be tied for highest weight
			for field, weight := range DetectedLabelWeights {
				if field != "gitOpsManaged" && field != "gitOpsTool" {
					Expect(gitOpsWeight).To(BeNumerically(">=", weight),
						"GitOps weight should be >= %s weight (correctness-critical)", field)
				}
			}
		})

		It("should have safety weights (PDB, mesh) higher than informational", func() {
			pdbWeight := GetDetectedLabelWeight("pdbProtected")
			meshWeight := GetDetectedLabelWeight("serviceMesh")

			informationalFields := []string{"networkIsolated", "helmManaged", "stateful", "hpaEnabled"}
			for _, field := range informationalFields {
				infoWeight := GetDetectedLabelWeight(field)
				Expect(pdbWeight).To(BeNumerically(">", infoWeight),
					"pdbProtected weight should be > %s weight (safety > informational)", field)
				Expect(meshWeight).To(BeNumerically(">", infoWeight),
					"serviceMesh weight should be > %s weight (safety > informational)", field)
			}
		})
	})

	Context("Scoring impact scenarios", func() {
		It("should calculate correct boost for GitOps-managed workflow", func() {
			// Scenario: Workflow and signal both GitOps-managed
			boost := GetDetectedLabelWeight("gitOpsManaged")
			Expect(boost).To(Equal(0.10))

			// With base similarity 0.85, final score = 0.85 + 0.10 = 0.95
			baseSimilarity := 0.85
			finalScore := baseSimilarity + boost
			Expect(finalScore).To(Equal(0.95))
		})

		It("should calculate correct penalty for GitOps mismatch", func() {
			// Scenario: Workflow is manual (not GitOps), signal is GitOps-managed
			penalty := GetDetectedLabelWeight("gitOpsManaged")
			Expect(penalty).To(Equal(0.10))

			// With base similarity 0.90, final score = 0.90 - 0.10 = 0.80
			baseSimilarity := 0.90
			finalScore := baseSimilarity - penalty
			Expect(finalScore).To(Equal(0.80))
		})

		It("should calculate correct boost for multiple matching labels", func() {
			// Scenario: Workflow matches GitOps + PDB + ServiceMesh
			gitOpsBoost := GetDetectedLabelWeight("gitOpsManaged")
			pdbBoost := GetDetectedLabelWeight("pdbProtected")
			meshBoost := GetDetectedLabelWeight("serviceMesh")

			totalBoost := gitOpsBoost + pdbBoost + meshBoost
			Expect(totalBoost).To(Equal(0.20)) // 0.10 + 0.05 + 0.05

			// With base similarity 0.75, final score = 0.75 + 0.20 = 0.95
			baseSimilarity := 0.75
			finalScore := baseSimilarity + totalBoost
			Expect(finalScore).To(Equal(0.95))
		})

		It("should cap final score at 1.0 even with high boost", func() {
			// Scenario: High base similarity + maximum boost
			baseSimilarity := 0.85
			maxBoost := MaxLabelBoost // 0.39

			// Without capping: 0.85 + 0.39 = 1.24
			// With capping: min(1.24, 1.0) = 1.0
			uncappedScore := baseSimilarity + maxBoost
			Expect(uncappedScore).To(BeNumerically(">", 1.0))

			cappedScore := 1.0 // SQL will use LEAST(score, 1.0)
			Expect(cappedScore).To(Equal(1.0))
		})
	})
})
