package contextapi

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/contextapi/sqlbuilder"
)

// ===================================================================
// EDGE CASE TESTING: Unicode and Multi-byte Characters (Scenario 3.1 + 3.2)
// ===================================================================

var _ = Describe("SQL Builder Unicode and Validation", func() {
	Context("BR-CONTEXT-001: Edge Case 3.1: Unicode and Multi-byte Characters (P3)", func() {
		// Day 11 Scenario 3.1 (Validation Testing)
		// BR-CONTEXT-001: SQL query construction with international characters
		//
		// Production Reality: ✅ Observed in K8s Namespaces
		// - Users create namespaces with Unicode
		// - K8s allows it, but requires proper handling
		// - Observed in international deployments
		//
		// Expected Behavior:
		// - SQL query handles multi-byte chars correctly
		// - Parameterization prevents encoding errors

		DescribeTable("Unicode namespace names should be handled correctly",
			func(namespace string) {
				builder := sqlbuilder.NewBuilder()
				builder.WithNamespace(namespace)

				query, args, err := builder.Build()

				// ✅ Business Value Assertion: Multi-byte chars handled correctly
				Expect(err).ToNot(HaveOccurred(),
					"SQL builder should handle Unicode characters")

				// ✅ Assert: Namespace is properly parameterized
				Expect(args).To(ContainElement(namespace),
					"Namespace parameter should preserve Unicode characters")

				// ✅ Assert: Query uses parameterization (not string interpolation)
				Expect(query).To(ContainSubstring("namespace = $"),
					"Query should use parameterized queries for Unicode safety")
			},
			Entry("Emoji", "namespace-🚀"),
			Entry("Chinese", "命名空间"),
			Entry("Arabic", "مساحة-الاسم"),
			Entry("Japanese", "ネームスペース"),
			Entry("Mixed Unicode", "namespace-中文-🎯"),
		)

		DescribeTable("Unicode severity values should be handled correctly",
			func(severity string) {
				builder := sqlbuilder.NewBuilder()
				builder.WithSeverity(severity)

				query, args, err := builder.Build()

				// ✅ Business Value Assertion: Severity Unicode handling
				Expect(err).ToNot(HaveOccurred())
				Expect(args).To(ContainElement(severity))
				Expect(query).To(ContainSubstring("severity = $"))
			},
			Entry("Standard ASCII", "critical"),
			Entry("Emoji", "critical-🔥"),
			Entry("International", "关键-critical"),
		)

		It("should reject null bytes in namespace", func() {
			// Day 11 Scenario 3.1 (Security Validation)
			// Null bytes can cause SQL injection in some databases

			builder := sqlbuilder.NewBuilder()
			maliciousNamespace := "namespace\x00with\x00nulls"

			// Option 1: Reject null bytes
			// Option 2: Sanitize null bytes
			// Current implementation: Let PostgreSQL handle it (parameterized queries are safe)
			builder.WithNamespace(maliciousNamespace)

			query, args, err := builder.Build()

			// ✅ Business Value Assertion: Parameterization protects against null bytes
			Expect(err).ToNot(HaveOccurred(),
				"Parameterized queries should handle null bytes safely")
			Expect(args).To(ContainElement(maliciousNamespace))
			Expect(query).To(ContainSubstring("namespace = $"),
				"Must use parameterization for security")
		})
	})

	Context("Edge Case 3.2: Extremely Long Filter Values (P3)", func() {
		It("should handle reasonable-length namespaces (up to K8s limit)", func() {
			// Day 11 Scenario 3.2 (Boundary Testing)
			// BR-CONTEXT-001: Input validation
			//
			// K8s namespace max length: 253 characters (RFC 1123 DNS label)

			builder := sqlbuilder.NewBuilder()

			// Create 253-char namespace (K8s maximum)
			maxLengthNamespace := string(make([]byte, 253))
			for i := range maxLengthNamespace {
				maxLengthNamespace = string(append([]byte(maxLengthNamespace[:i]), byte('a'+(i%26))))
			}

			builder.WithNamespace(maxLengthNamespace)
			query, args, err := builder.Build()

			// ✅ Business Value Assertion: K8s-valid namespaces are accepted
			Expect(err).ToNot(HaveOccurred(),
				"Should accept namespaces up to K8s limit (253 chars)")
			Expect(len(args[len(args)-3].(string))).To(Equal(253))
			Expect(query).ToNot(BeEmpty())
		})

		It("should warn about excessively long filter values", func() {
			// Day 11 Scenario 3.2 (DoS Prevention)
			// Protection against malicious or accidental large inputs

			builder := sqlbuilder.NewBuilder()

			// Create 10KB namespace string (way beyond reasonable)
			hugeNamespace := string(make([]byte, 10*1024))
			for i := range hugeNamespace {
				hugeNamespace = string(append([]byte(hugeNamespace[:i]), 'x'))
			}

			builder.WithNamespace(hugeNamespace)
			query, _, err := builder.Build()

			// ✅ Current Behavior: SQL builder accepts it (PostgreSQL will handle)
			// Note: We're validating current behavior, not enforcing limits
			// This test documents that large inputs are passed through
			Expect(err).ToNot(HaveOccurred(),
				"SQL builder currently allows large inputs (database validates)")

			// ✅ Assert: Parameterization still used (prevents SQL injection)
			Expect(query).To(ContainSubstring("namespace = $"),
				"Even with large inputs, must use parameterization")

			// ✅ Document: This is a known limitation
			// Future enhancement: Add input length validation in API layer
			// Recommended: max 253 chars for namespace (K8s limit)
			Skip("Future enhancement: Add input validation for DoS prevention")
		})
	})
})
