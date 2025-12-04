package signalprocessing

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/jordigilh/kubernaut/pkg/signalprocessing/classifier"
)

// BR-SP-080: Confidence Scoring
// BR-SP-081: Multi-dimensional Business Categorization
var _ = Describe("BR-SP-080, BR-SP-081: Business Classifier", func() {
	var (
		ctx                context.Context
		businessClassifier *classifier.BusinessClassifier
	)

	BeforeEach(func() {
		ctx = context.Background()
		businessClassifier = classifier.NewBusinessClassifier(ctrl.Log.WithName("test"))
	})

	// ========================================
	// BR-SP-081: Multi-dimensional Categorization Tests
	// ========================================
	Describe("Multi-dimensional Business Categorization", func() {
		// ✅ CORRECT: Use DescribeTable for similar test scenarios (per 03-testing-strategy.mdc)
		DescribeTable("should classify business context based on namespace labels",
			func(namespaceLabels map[string]string, expectedBusinessUnit string, expectedCriticality string, minConfidence float64) {
				result := businessClassifier.Classify(ctx, namespaceLabels, nil)

				// ✅ CORRECT: Value assertions (not null-testing)
				Expect(result.BusinessUnit).To(Equal(expectedBusinessUnit))
				Expect(result.Criticality).To(Equal(expectedCriticality))
				Expect(result.OverallConfidence).To(BeNumerically(">=", minConfidence))
			},
			Entry("payments team in production",
				map[string]string{"team": "payments", "environment": "production"},
				"payments", "critical", 0.9),
			Entry("platform team in staging",
				map[string]string{"team": "platform", "environment": "staging"},
				"platform", "high", 0.8),
			Entry("unknown team defaults to general",
				map[string]string{},
				"general", "low", 0.5),
			Entry("fintech team with pci-dss label",
				map[string]string{"team": "fintech", "compliance": "pci-dss"},
				"fintech", "critical", 0.95),
		)

		DescribeTable("should extract service owner from annotations",
			func(annotations map[string]string, expectedServiceOwner string) {
				result := businessClassifier.Classify(ctx, nil, annotations)

				Expect(result.ServiceOwner).To(Equal(expectedServiceOwner))
			},
			Entry("owner from standard annotation",
				map[string]string{"owner": "team-alpha@example.com"},
				"team-alpha@example.com"),
		Entry("owner from kubernaut annotation",
			map[string]string{"kubernaut.ai/owner": "team-beta@example.com"},
			"team-beta@example.com"),
			Entry("no owner annotation defaults to unknown",
				map[string]string{},
				"unknown"),
		)
	})

	// ========================================
	// BR-SP-080: Confidence Scoring Tests
	// ========================================
	Describe("Confidence Scoring", func() {
		DescribeTable("should calculate confidence based on data completeness",
			func(namespaceLabels map[string]string, annotations map[string]string, minConfidence float64, maxConfidence float64) {
				result := businessClassifier.Classify(ctx, namespaceLabels, annotations)

				Expect(result.OverallConfidence).To(BeNumerically(">=", minConfidence))
				Expect(result.OverallConfidence).To(BeNumerically("<=", maxConfidence))
			},
			Entry("full data yields high confidence",
				map[string]string{"team": "payments", "environment": "production", "compliance": "pci-dss"},
				map[string]string{"owner": "team@example.com", "sla": "gold"},
				0.9, 1.0),
			Entry("partial data yields medium confidence",
				map[string]string{"team": "payments"},
				map[string]string{},
				0.5, 0.8),
			Entry("no data yields low confidence",
				map[string]string{},
				map[string]string{},
				0.0, 0.5),
		)
	})

	// ========================================
	// BR-SP-081: SLA Requirement Extraction
	// ========================================
	Describe("SLA Requirement Extraction", func() {
		DescribeTable("should extract SLA from annotations or derive from criticality",
			func(annotations map[string]string, criticality string, expectedSLA string) {
				// Set criticality via namespace labels
				namespaceLabels := map[string]string{}
				if criticality != "" {
					namespaceLabels["criticality"] = criticality
				}

				result := businessClassifier.Classify(ctx, namespaceLabels, annotations)

				Expect(result.SLARequirement).To(Equal(expectedSLA))
			},
			Entry("explicit SLA annotation",
				map[string]string{"sla": "gold"},
				"", "gold"),
		Entry("kubernaut SLA annotation",
			map[string]string{"kubernaut.ai/sla": "platinum"},
			"", "platinum"),
			Entry("critical criticality implies gold SLA",
				map[string]string{},
				"critical", "gold"),
			Entry("high criticality implies silver SLA",
				map[string]string{},
				"high", "silver"),
			Entry("low criticality implies bronze SLA",
				map[string]string{},
				"low", "bronze"),
			Entry("no SLA or criticality defaults to bronze",
				map[string]string{},
				"", "bronze"),
		)
	})

	// ========================================
	// Edge Cases and Error Handling
	// ========================================
	Describe("Edge Cases", func() {
		It("should handle nil namespace labels gracefully", func() {
			result := businessClassifier.Classify(ctx, nil, nil)

			Expect(result.BusinessUnit).To(Equal("general"))
			Expect(result.ServiceOwner).To(Equal("unknown"))
			Expect(result.Criticality).To(Equal("low"))
			Expect(result.SLARequirement).To(Equal("bronze"))
			Expect(result.OverallConfidence).To(BeNumerically(">=", 0.0))
		})

		It("should handle empty string values in labels", func() {
			result := businessClassifier.Classify(ctx,
				map[string]string{"team": "", "environment": ""},
				map[string]string{"owner": ""})

			// Empty strings should be treated as missing values
			Expect(result.BusinessUnit).To(Equal("general"))
			Expect(result.ServiceOwner).To(Equal("unknown"))
		})
	})
})
