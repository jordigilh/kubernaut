package contextapi

import (
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/contextapi/sqlbuilder"
)

var _ = Describe("SQL Builder", func() {
	Context("NewBuilder", func() {
		It("should create a builder with default values", func() {
			builder := sqlbuilder.NewBuilder()
			Expect(builder).ToNot(BeNil())

			query, args, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())
			// Updated for Data Storage Service schema (DD-SCHEMA-001)
			Expect(query).To(ContainSubstring("FROM resource_action_traces rat"))
			Expect(query).To(ContainSubstring("JOIN action_histories ah"))
			Expect(query).To(ContainSubstring("JOIN resource_references rr"))
			Expect(query).To(ContainSubstring("ORDER BY rat.action_timestamp DESC"))
			Expect(query).To(ContainSubstring("LIMIT"))
			Expect(args).To(HaveLen(2)) // Default limit + offset
		})
	})

	Context("Boundary Value Tests for Limit", func() {
		DescribeTable("Limit validation",
			func(limit int, shouldFail bool) {
				builder := sqlbuilder.NewBuilder()
				err := builder.WithLimit(limit)

				if shouldFail {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("limit"))
				} else {
					Expect(err).ToNot(HaveOccurred())
				}
			},
			Entry("valid limit: 100", 100, false),
			Entry("minimum valid limit: 1", 1, false),
			Entry("maximum valid limit: 1000", 1000, false),
			Entry("zero limit invalid", 0, true),
			Entry("negative limit invalid", -1, true),
			Entry("negative limit invalid: -100", -100, true),
			Entry("over limit invalid: 1001", 1001, true),
			Entry("over limit invalid: 5000", 5000, true),
		)
	})

	Context("Boundary Value Tests for Offset", func() {
		DescribeTable("Offset validation",
			func(offset int, shouldFail bool) {
				builder := sqlbuilder.NewBuilder()
				err := builder.WithOffset(offset)

				if shouldFail {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("offset"))
				} else {
					Expect(err).ToNot(HaveOccurred())
				}
			},
			Entry("zero offset valid", 0, false),
			Entry("positive offset valid: 100", 100, false),
			Entry("large offset valid: 999999", 999999, false),
			Entry("negative offset invalid: -1", -1, true),
			Entry("negative offset invalid: -100", -100, true),
		)
	})

	Context("SQL Injection Protection", func() {
		DescribeTable("Namespace filter parameterization",
			func(namespace string) {
				builder := sqlbuilder.NewBuilder()
				builder.WithNamespace(namespace)

				query, args, err := builder.Build()
				Expect(err).ToNot(HaveOccurred())

				// Verify raw input is NOT in the query string
				Expect(query).ToNot(ContainSubstring(namespace))

				// Verify input is parameterized in args
				Expect(args).To(ContainElement(namespace))

				// Verify parameterized placeholder is used with table alias (Data Storage schema)
				Expect(query).To(ContainSubstring("rr.namespace = $"))
			},
			Entry("normal input", "default"),
			Entry("SQL injection attempt 1", "default' OR '1'='1"),
			Entry("SQL injection attempt 2", "default; DROP TABLE resource_action_traces;--"),
			Entry("SQL injection attempt 3", "default' UNION SELECT * FROM secrets--"),
			Entry("SQL injection attempt 4", "default') OR 1=1--"),
			Entry("special chars", "namespace-with-special_chars.123"),
		)

		DescribeTable("Severity filter parameterization",
			func(severity string) {
				builder := sqlbuilder.NewBuilder()
				builder.WithSeverity(severity)

				query, args, err := builder.Build()
				Expect(err).ToNot(HaveOccurred())

				// Verify raw input is NOT in the query string
				Expect(query).ToNot(ContainSubstring(severity))

				// Verify input is parameterized in args
				Expect(args).To(ContainElement(severity))

				// Verify parameterized placeholder is used with table alias (Data Storage schema)
				Expect(query).To(ContainSubstring("rat.alert_severity = $"))
			},
			Entry("normal input", "critical"),
			Entry("SQL injection attempt", "critical' OR '1'='1"),
		)
	})

	Context("Filter Combinations", func() {
		It("should handle multiple filters with correct parameter counting", func() {
			builder := sqlbuilder.NewBuilder()
			builder.WithNamespace("production")
			builder.WithSeverity("critical")

			query, args, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			// Verify WHERE clause exists with table aliases (Data Storage schema)
			Expect(query).To(ContainSubstring("WHERE"))
			Expect(query).To(ContainSubstring("rr.namespace = $1"))
			Expect(query).To(ContainSubstring("rat.alert_severity = $2"))
			Expect(query).To(ContainSubstring("LIMIT $3"))
			Expect(query).To(ContainSubstring("OFFSET $4"))

			// Verify args in correct order
			Expect(args).To(HaveLen(4))
			Expect(args[0]).To(Equal("production"))
			Expect(args[1]).To(Equal("critical"))
		})

		It("should handle time range filter", func() {
			builder := sqlbuilder.NewBuilder()
			start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2025, 1, 31, 23, 59, 59, 0, time.UTC)

			builder.WithTimeRange(start, end)

			query, args, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			// Verify BETWEEN clause with table alias (Data Storage schema)
			Expect(query).To(ContainSubstring("rat.action_timestamp BETWEEN $1 AND $2"))

			// Verify args contain time values
			Expect(args).To(ContainElement(start))
			Expect(args).To(ContainElement(end))
		})

		It("should handle all filters together", func() {
			builder := sqlbuilder.NewBuilder()
			start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2025, 1, 31, 23, 59, 59, 0, time.UTC)

			builder.WithNamespace("production")
			builder.WithSeverity("critical")
			builder.WithTimeRange(start, end)
			Expect(builder.WithLimit(50)).To(Succeed())
			Expect(builder.WithOffset(10)).To(Succeed())

			query, args, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			// Verify all filters are present with table aliases (Data Storage schema)
			Expect(query).To(ContainSubstring("rr.namespace = $1"))
			Expect(query).To(ContainSubstring("rat.alert_severity = $2"))
			Expect(query).To(ContainSubstring("rat.action_timestamp BETWEEN $3 AND $4"))
			Expect(query).To(ContainSubstring("LIMIT $5"))
			Expect(query).To(ContainSubstring("OFFSET $6"))

			// Verify args count and order
			Expect(args).To(HaveLen(6))
			Expect(args[0]).To(Equal("production"))
			Expect(args[1]).To(Equal("critical"))
			Expect(args[2]).To(Equal(start))
			Expect(args[3]).To(Equal(end))
			Expect(args[4]).To(Equal(50))
			Expect(args[5]).To(Equal(10))
		})
	})

	Context("Query Structure Validation", func() {
		It("should always include ORDER BY for consistent pagination", func() {
			builder := sqlbuilder.NewBuilder()

			query, _, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			// Verify ORDER BY comes before LIMIT
			orderIdx := strings.Index(query, "ORDER BY")
			limitIdx := strings.Index(query, "LIMIT")
			Expect(orderIdx).To(BeNumerically("<", limitIdx))
			// Updated for Data Storage schema
			Expect(query).To(ContainSubstring("ORDER BY rat.action_timestamp DESC"))
		})

		It("should join multiple filters with AND", func() {
			builder := sqlbuilder.NewBuilder()
			builder.WithNamespace("default")
			builder.WithSeverity("warning")

			query, _, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			// Verify AND between filters with table aliases (Data Storage schema)
			Expect(query).To(ContainSubstring("rr.namespace = $1 AND rat.alert_severity = $2"))
		})
	})

	Context("Empty Filters", func() {
		It("should work without any filters", func() {
			builder := sqlbuilder.NewBuilder()

			query, args, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			// Should have base query with only pagination
			Expect(query).ToNot(ContainSubstring("WHERE"))
			Expect(args).To(HaveLen(2)) // Just limit and offset
		})
	})

	Context("Parameter Counting Edge Cases", func() {
		It("should correctly count parameters with skip patterns", func() {
			// Test that parameter counting works even when filters are added out of order
			builder := sqlbuilder.NewBuilder()
			builder.WithSeverity("critical")    // This should be $1
			builder.WithNamespace("production") // This should be $2

			query, args, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			// Verify correct parameter assignment
			Expect(query).To(ContainSubstring("severity = $1"))
			Expect(query).To(ContainSubstring("namespace = $2"))
			Expect(args[0]).To(Equal("critical"))
			Expect(args[1]).To(Equal("production"))
		})
	})

	Context("Builder Reusability", func() {
		It("should support building multiple queries from same builder instance", func() {
			builder := sqlbuilder.NewBuilder()
			builder.WithNamespace("default")

			// First build
			query1, args1, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			// Second build (should be idempotent)
			query2, args2, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			// Both should be identical
			Expect(query1).To(Equal(query2))
			Expect(args1).To(Equal(args2))
		})
	})
})
