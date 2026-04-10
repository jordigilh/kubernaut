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

package partition_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/partition"
)

var _ = Describe("Partition Management — Unit Tests", func() {

	// UT-DS-235-001: Month range calculator (both tables)
	// BR-AUDIT-029: Automatic partition management
	Describe("UT-DS-235-001: ComputePartitionSpecs — month range calculator", func() {

		It("should return 4 partitions per table with lookahead=3 (M0..M+3)", func() {
			now := time.Date(2026, time.April, 15, 10, 30, 0, 0, time.UTC)
			tables := partition.AllTables()
			specs := partition.ComputePartitionSpecs(now, 3, tables)

			// 2 tables × 4 months = 8 total partition specs
			Expect(specs).To(HaveLen(8))
		})

		It("should produce correct months for mid-month date (April → April, May, June, July)", func() {
			now := time.Date(2026, time.April, 15, 10, 30, 0, 0, time.UTC)
			tables := []partition.ParentTable{partition.AuditEventsTable}
			specs := partition.ComputePartitionSpecs(now, 3, tables)

			Expect(specs).To(HaveLen(4))
			Expect(specs[0].Name).To(Equal("audit_events_2026_04"))
			Expect(specs[1].Name).To(Equal("audit_events_2026_05"))
			Expect(specs[2].Name).To(Equal("audit_events_2026_06"))
			Expect(specs[3].Name).To(Equal("audit_events_2026_07"))
		})

		It("should handle year rollover (November → Nov, Dec, Jan, Feb)", func() {
			now := time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC)
			tables := []partition.ParentTable{partition.ResourceActionTracesTable}
			specs := partition.ComputePartitionSpecs(now, 3, tables)

			Expect(specs).To(HaveLen(4))
			Expect(specs[0].Name).To(Equal("resource_action_traces_2026_11"))
			Expect(specs[1].Name).To(Equal("resource_action_traces_2026_12"))
			Expect(specs[2].Name).To(Equal("resource_action_traces_2027_01"))
			Expect(specs[3].Name).To(Equal("resource_action_traces_2027_02"))
		})

		It("should use UTC for all boundary dates", func() {
			now := time.Date(2026, time.April, 15, 23, 59, 59, 0, time.UTC)
			tables := []partition.ParentTable{partition.AuditEventsTable}
			specs := partition.ComputePartitionSpecs(now, 3, tables)

			for _, spec := range specs {
				Expect(spec.RangeStart.Location()).To(Equal(time.UTC))
				Expect(spec.RangeEnd.Location()).To(Equal(time.UTC))
				Expect(spec.RangeStart.Day()).To(Equal(1), "RangeStart should be first of month")
				Expect(spec.RangeEnd.Day()).To(Equal(1), "RangeEnd should be first of next month")
			}
		})

		It("should compute correct range boundaries (first-of-month to first-of-next-month)", func() {
			now := time.Date(2026, time.April, 10, 0, 0, 0, 0, time.UTC)
			tables := []partition.ParentTable{partition.AuditEventsTable}
			specs := partition.ComputePartitionSpecs(now, 0, tables)

			Expect(specs).To(HaveLen(1))
			Expect(specs[0].RangeStart).To(Equal(time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC)))
			Expect(specs[0].RangeEnd).To(Equal(time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC)))
		})

		It("should return specs for both tables with matching month sequences", func() {
			now := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
			tables := partition.AllTables()
			specs := partition.ComputePartitionSpecs(now, 1, tables)

			// 2 tables × 2 months = 4 specs
			Expect(specs).To(HaveLen(4))

			auditSpecs := filterByParent(specs, "audit_events")
			ratSpecs := filterByParent(specs, "resource_action_traces")
			Expect(auditSpecs).To(HaveLen(2))
			Expect(ratSpecs).To(HaveLen(2))

			// Same month sequence for both
			for i := range auditSpecs {
				Expect(auditSpecs[i].RangeStart).To(Equal(ratSpecs[i].RangeStart))
				Expect(auditSpecs[i].RangeEnd).To(Equal(ratSpecs[i].RangeEnd))
			}
		})

		It("should handle UTC midnight boundary for timestamp at 2026-04-30T23:59:59Z (still April)", func() {
			now := time.Date(2026, time.April, 30, 23, 59, 59, 0, time.UTC)
			tables := []partition.ParentTable{partition.AuditEventsTable}
			specs := partition.ComputePartitionSpecs(now, 0, tables)

			Expect(specs).To(HaveLen(1))
			Expect(specs[0].Name).To(Equal("audit_events_2026_04"))
		})
	})

	// UT-DS-235-002: Partition naming
	// BR-AUDIT-029: Automatic partition management
	Describe("UT-DS-235-002: FormatPartitionName — naming conventions", func() {

		It("should format audit_events partitions as audit_events_YYYY_MM", func() {
			name := partition.FormatPartitionName("audit_events", 2026, time.April)
			Expect(name).To(Equal("audit_events_2026_04"))
		})

		It("should format resource_action_traces partitions as resource_action_traces_YYYY_MM", func() {
			name := partition.FormatPartitionName("resource_action_traces", 2026, time.November)
			Expect(name).To(Equal("resource_action_traces_2026_11"))
		})

		It("should zero-pad single-digit months", func() {
			name := partition.FormatPartitionName("audit_events", 2027, time.January)
			Expect(name).To(Equal("audit_events_2027_01"))
		})

		It("should handle December correctly", func() {
			name := partition.FormatPartitionName("resource_action_traces", 2028, time.December)
			Expect(name).To(Equal("resource_action_traces_2028_12"))
		})

		It("should match existing migration naming pattern (YYYY_MM with underscore separator)", func() {
			// Verify alignment with 001_v1_schema.sql naming: resource_action_traces_2026_03
			name := partition.FormatPartitionName("resource_action_traces", 2026, time.March)
			Expect(name).To(Equal("resource_action_traces_2026_03"))
		})
	})

	// UT-DS-235-003: Idempotency (DDL uses IF NOT EXISTS)
	// BR-AUDIT-029: Automatic partition management
	Describe("UT-DS-235-003: GenerateDDL — idempotent DDL generation", func() {

		It("should produce DDL with IF NOT EXISTS clause", func() {
			spec := partition.PartitionSpec{
				ParentTable: "audit_events",
				Name:        "audit_events_2026_04",
				RangeStart:  time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC),
				RangeEnd:    time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC),
			}
			ddl := partition.GenerateDDL(spec)
			Expect(ddl).To(ContainSubstring("IF NOT EXISTS"))
		})

		It("should produce DDL with PARTITION OF parent table", func() {
			spec := partition.PartitionSpec{
				ParentTable: "resource_action_traces",
				Name:        "resource_action_traces_2026_05",
				RangeStart:  time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC),
				RangeEnd:    time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC),
			}
			ddl := partition.GenerateDDL(spec)
			Expect(ddl).To(ContainSubstring("PARTITION OF resource_action_traces"))
		})

		It("should include correct FOR VALUES FROM ... TO ... range", func() {
			spec := partition.PartitionSpec{
				ParentTable: "audit_events",
				Name:        "audit_events_2026_04",
				RangeStart:  time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC),
				RangeEnd:    time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC),
			}
			ddl := partition.GenerateDDL(spec)
			Expect(ddl).To(ContainSubstring("FOR VALUES FROM ('2026-04-01') TO ('2026-05-01')"))
		})

		It("should use the spec Name as the child table identifier", func() {
			spec := partition.PartitionSpec{
				ParentTable: "audit_events",
				Name:        "audit_events_2027_01",
				RangeStart:  time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC),
				RangeEnd:    time.Date(2027, time.February, 1, 0, 0, 0, 0, time.UTC),
			}
			ddl := partition.GenerateDDL(spec)
			Expect(ddl).To(ContainSubstring("audit_events_2027_01"))
		})

		It("should produce valid non-empty DDL", func() {
			spec := partition.PartitionSpec{
				ParentTable: "audit_events",
				Name:        "audit_events_2026_06",
				RangeStart:  time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC),
				RangeEnd:    time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC),
			}
			ddl := partition.GenerateDDL(spec)
			Expect(ddl).NotTo(BeEmpty())
		})
	})
})

func filterByParent(specs []partition.PartitionSpec, parentTable string) []partition.PartitionSpec {
	var result []partition.PartitionSpec
	for _, s := range specs {
		if s.ParentTable == parentTable {
			result = append(result, s)
		}
	}
	return result
}
