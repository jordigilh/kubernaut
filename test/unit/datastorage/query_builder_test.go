package datastorage

import (
	"github.com/jordigilh/kubernaut/pkg/datastorage/query"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("SQL Query Builder - BR-STORAGE-021, BR-STORAGE-022", func() {
	// BR-STORAGE-022: Query filtering
	DescribeTable("should build queries with filters",
		func(params query.QueryParams, expectedSQL string, filterArgIndex int, expectedFilterValue interface{}) {
			builder := query.NewBuilder()
			sql, args, err := builder.WithParams(params).Build()

			Expect(err).ToNot(HaveOccurred())
			Expect(sql).To(ContainSubstring(expectedSQL))
			// Args always include limit and offset at the end, so filter values are at the beginning
			Expect(args[filterArgIndex]).To(Equal(expectedFilterValue))
		},
		Entry("namespace filter", query.QueryParams{Namespace: "production"}, "namespace = ?", 0, "production"),
		Entry("severity filter", query.QueryParams{Severity: "high"}, "severity = ?", 0, "high"),
		Entry("multiple filters", query.QueryParams{Namespace: "prod", Severity: "high"}, "namespace = ? AND severity = ?", 0, "prod"),
		Entry("cluster filter", query.QueryParams{Cluster: "us-east-1"}, "cluster_name = ?", 0, "us-east-1"),
		Entry("environment filter", query.QueryParams{Environment: "production"}, "environment = ?", 0, "production"),
		Entry("action_type filter", query.QueryParams{ActionType: "scale_deployment"}, "action_type = ?", 0, "scale_deployment"),
	)

	// BR-STORAGE-023: Pagination
	DescribeTable("should handle pagination",
		func(limit, offset int, expectError bool) {
			builder := query.NewBuilder().WithLimit(limit).WithOffset(offset)
			_, _, err := builder.Build()

			if expectError {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		},
		Entry("valid pagination", 100, 0, false),
		Entry("boundary: limit=1", 1, 0, false),
		Entry("boundary: limit=1000", 1000, 0, false),
		Entry("boundary: offset=0", 100, 0, false),
		Entry("invalid: limit=0", 0, 0, true),            // BR-STORAGE-023: Must reject limit=0
		Entry("invalid: limit=1001", 1001, 0, true),      // BR-STORAGE-023: Must reject limit>1000
		Entry("invalid: negative offset", 100, -1, true), // BR-STORAGE-023: Must reject negative offset
	)

	// BR-STORAGE-025: SQL injection prevention
	DescribeTable("should prevent SQL injection",
		func(maliciousInput string) {
			builder := query.NewBuilder().WithNamespace(maliciousInput)
			sql, args, err := builder.Build()

			Expect(err).ToNot(HaveOccurred())
			// Parameterized query should use placeholders, not inject SQL
			Expect(sql).To(ContainSubstring("?"))
			Expect(sql).ToNot(ContainSubstring("DROP"))
			Expect(sql).ToNot(ContainSubstring("--"))
			Expect(args[0]).To(Equal(maliciousInput)) // Value in args, not SQL
		},
		Entry("DROP TABLE attempt", "'; DROP TABLE resource_action_traces--"),
		Entry("OR 1=1 attempt", "' OR '1'='1"),
		Entry("comment injection", "test'; --"),
		Entry("union select", "' UNION SELECT * FROM users--"),
	)

	// BR-STORAGE-026: Unicode support
	DescribeTable("should handle unicode",
		func(unicodeValue string) {
			builder := query.NewBuilder().WithNamespace(unicodeValue)
			_, args, err := builder.Build()

			Expect(err).ToNot(HaveOccurred())
			Expect(args[0]).To(Equal(unicodeValue))
		},
		Entry("Arabic", "مساحة-الإنتاج"),
		Entry("Chinese", "生产环境"),
		Entry("Emoji", "prod-🚀"),
		Entry("Mixed", "prod-环境-🔥"),
	)

	// BR-STORAGE-021: Read endpoint query construction
	Describe("base query construction", func() {
		It("should build query for incident listing", func() {
			builder := query.NewBuilder()
			sql, _, err := builder.Build()

			Expect(err).ToNot(HaveOccurred())
			Expect(sql).To(ContainSubstring("SELECT"))
			Expect(sql).To(ContainSubstring("FROM resource_action_traces"))
		})

		It("should include ORDER BY for consistent ordering", func() {
			builder := query.NewBuilder()
			sql, _, err := builder.Build()

			Expect(err).ToNot(HaveOccurred())
			Expect(sql).To(ContainSubstring("ORDER BY"))
		})
	})
})
