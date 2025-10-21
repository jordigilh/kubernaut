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

package sqlbuilder

import (
	"testing"

	"github.com/jordigilh/kubernaut/pkg/contextapi/sqlbuilder"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSQLBuilderSchema(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SQL Builder Schema Test Suite")
}

var _ = Describe("SQL Builder Schema Alignment", func() {
	Context("Base Query Generation", func() {
		It("should use Data Storage schema with JOINs", func() {
			// BR-CONTEXT-001: Query builder must use Data Storage's authoritative schema
			builder := sqlbuilder.NewBuilder()
			query, _, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			// Must query from resource_action_traces (not remediation_audit)
			Expect(query).To(ContainSubstring("FROM resource_action_traces"))

			// Must include alias 'rat'
			Expect(query).To(ContainSubstring("resource_action_traces rat"))

			// Must JOIN with action_histories
			Expect(query).To(ContainSubstring("JOIN action_histories ah"))
			Expect(query).To(ContainSubstring("ON rat.action_history_id = ah.id"))

			// Must JOIN with resource_references
			Expect(query).To(ContainSubstring("JOIN resource_references rr"))
			Expect(query).To(ContainSubstring("ON ah.resource_id = rr.id"))
		})

		It("should select all required Context API fields", func() {
			// BR-CONTEXT-008: Must provide complete IncidentEvent data
			builder := sqlbuilder.NewBuilder()
			query, _, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			// Primary identification
			Expect(query).To(ContainSubstring("rat.id"))
			Expect(query).To(ContainSubstring("rat.alert_name"))
			Expect(query).To(ContainSubstring("rat.alert_fingerprint"))
			Expect(query).To(ContainSubstring("rat.action_id"))

			// Context fields
			Expect(query).To(ContainSubstring("rr.namespace"))
			Expect(query).To(ContainSubstring("rat.cluster_name"))
			Expect(query).To(ContainSubstring("rat.environment"))
			Expect(query).To(ContainSubstring("rr.kind"))

			// Status fields
			Expect(query).To(ContainSubstring("rat.execution_status"))
			Expect(query).To(ContainSubstring("rat.alert_severity"))
			Expect(query).To(ContainSubstring("rat.action_type"))

			// Timing fields
			Expect(query).To(ContainSubstring("rat.action_timestamp"))
			Expect(query).To(ContainSubstring("rat.execution_end_time"))
			Expect(query).To(ContainSubstring("rat.execution_duration_ms"))
		})
	})

	Context("WHERE Clause Generation with Table Aliases", func() {
		It("should use 'rr.' prefix for namespace filter", func() {
			// BR-CONTEXT-004: Namespace filtering
			builder := sqlbuilder.NewBuilder()
			builder.WithNamespace("production")
			query, args, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			// Must use resource_references table alias
			Expect(query).To(ContainSubstring("WHERE rr.namespace = $1"))
			Expect(args).To(HaveLen(3)) // namespace + limit + offset
			Expect(args[0]).To(Equal("production"))
		})

		It("should use 'rat.' prefix for severity filter", func() {
			// BR-CONTEXT-004: Severity filtering
			builder := sqlbuilder.NewBuilder()
			builder.WithSeverity("critical")
			query, args, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			// Must use resource_action_traces table alias
			Expect(query).To(ContainSubstring("rat.alert_severity = $"))
			Expect(args).To(ContainElement("critical"))
		})

		It("should use 'rat.' prefix for cluster_name filter", func() {
			// BR-CONTEXT-004: Cluster filtering (new field)
			builder := sqlbuilder.NewBuilder()
			builder.WithClusterName("prod-us-west")
			query, args, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			// Must use resource_action_traces table alias
			Expect(query).To(ContainSubstring("rat.cluster_name = $"))
			Expect(args).To(ContainElement("prod-us-west"))
		})

		It("should use 'rat.' prefix for environment filter", func() {
			// BR-CONTEXT-004: Environment filtering (new field)
			builder := sqlbuilder.NewBuilder()
			builder.WithEnvironment("production")
			query, args, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			// Must use resource_action_traces table alias
			Expect(query).To(ContainSubstring("rat.environment = $"))
			Expect(args).To(ContainElement("production"))
		})

		It("should use 'rat.' prefix for action_type filter", func() {
			// BR-CONTEXT-004: Action type filtering
			builder := sqlbuilder.NewBuilder()
			builder.WithActionType("scale_deployment")
			query, args, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			// Must use resource_action_traces table alias
			Expect(query).To(ContainSubstring("rat.action_type = $"))
			Expect(args).To(ContainElement("scale_deployment"))
		})

		It("should combine multiple filters with AND", func() {
			// BR-CONTEXT-004: Multiple filter combination
			builder := sqlbuilder.NewBuilder()
			builder.WithNamespace("production").WithSeverity("critical")
			query, args, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			// Must have both WHERE clauses with correct aliases
			Expect(query).To(ContainSubstring("WHERE"))
			Expect(query).To(ContainSubstring("rr.namespace = $"))
			Expect(query).To(ContainSubstring("rat.alert_severity = $"))
			Expect(query).To(ContainSubstring("AND"))
			Expect(args).To(HaveLen(4)) // 2 filters + limit + offset
		})
	})

	Context("Field Aliases and Mapping", func() {
		It("should alias alert_name AS name", func() {
			builder := sqlbuilder.NewBuilder()
			query, _, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			// Context API expects 'name' field
			Expect(query).To(ContainSubstring("rat.alert_name AS name"))
		})

		It("should alias action_id AS remediation_request_id", func() {
			builder := sqlbuilder.NewBuilder()
			query, _, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			// Context API expects 'remediation_request_id'
			Expect(query).To(ContainSubstring("rat.action_id AS remediation_request_id"))
		})

		It("should alias kind AS target_resource", func() {
			builder := sqlbuilder.NewBuilder()
			query, _, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			// Context API expects 'target_resource'
			Expect(query).To(ContainSubstring("rr.kind AS target_resource"))
		})

		It("should alias execution_status AS status", func() {
			builder := sqlbuilder.NewBuilder()
			query, _, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			// Context API expects 'status'
			Expect(query).To(ContainSubstring("rat.execution_status AS status"))
		})

		It("should derive phase from execution_status using CASE", func() {
			builder := sqlbuilder.NewBuilder()
			query, _, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			// Must use CASE statement to derive phase
			Expect(query).To(ContainSubstring("CASE rat.execution_status"))
			Expect(query).To(ContainSubstring("AS phase"))

			// Should handle common statuses
			Expect(query).To(ContainSubstring("completed"))
			Expect(query).To(ContainSubstring("failed"))
			Expect(query).To(ContainSubstring("pending"))
		})
	})

	Context("ORDER BY and LIMIT", func() {
		It("should use 'rat.' prefix in ORDER BY clause", func() {
			// BR-CONTEXT-007: Pagination support
			builder := sqlbuilder.NewBuilder()
			query, _, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			// Must use table alias in ORDER BY
			Expect(query).To(ContainSubstring("ORDER BY rat.action_timestamp DESC"))
		})

		It("should include LIMIT and OFFSET", func() {
			// BR-CONTEXT-007: Pagination support
			builder := sqlbuilder.NewBuilder()
			err := builder.WithLimit(10)
			Expect(err).ToNot(HaveOccurred())
			err = builder.WithOffset(20)
			Expect(err).ToNot(HaveOccurred())

			query, args, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())

			Expect(query).To(ContainSubstring("LIMIT $"))
			Expect(query).To(ContainSubstring("OFFSET $"))
			Expect(args).To(ContainElement(10))
			Expect(args).To(ContainElement(20))
		})
	})

	Context("Count Query Generation", func() {
		It("should generate count query with same JOINs", func() {
			builder := sqlbuilder.NewBuilder()
			builder.WithNamespace("production")

			countQuery, args := builder.BuildCount()

			// Must have same JOIN structure
			Expect(countQuery).To(ContainSubstring("FROM resource_action_traces rat"))
			Expect(countQuery).To(ContainSubstring("JOIN action_histories ah"))
			Expect(countQuery).To(ContainSubstring("JOIN resource_references rr"))

			// Must have same WHERE clause
			Expect(countQuery).To(ContainSubstring("WHERE rr.namespace = $1"))
			Expect(args).To(HaveLen(1))
		})

		It("should not include ORDER BY or LIMIT in count query", func() {
			builder := sqlbuilder.NewBuilder()
			err := builder.WithLimit(10)
			Expect(err).ToNot(HaveOccurred())
			err = builder.WithOffset(20)
			Expect(err).ToNot(HaveOccurred())

			countQuery, _ := builder.BuildCount()

			// Count queries don't need ordering or limits
			Expect(countQuery).NotTo(ContainSubstring("ORDER BY"))
			Expect(countQuery).NotTo(ContainSubstring("LIMIT"))
			Expect(countQuery).NotTo(ContainSubstring("OFFSET"))
		})
	})
})
