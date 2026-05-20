package query_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/query"
)

var _ = Describe("AuditEventsQueryBuilder — JSONB field filter (Issue #1199)", func() {

	It("UT-DS-1199-005: WithEventDataFilter generates correct SQL with parameterized arg", func() {
		builder := query.NewAuditEventsQueryBuilder().
			WithEventDataFilter("task_id", "task-abc")

		sql, args, err := builder.Build()
		Expect(err).NotTo(HaveOccurred())
		Expect(sql).To(ContainSubstring("event_data->>'task_id' = $"))
		Expect(args).To(ContainElement("task-abc"))
	})

	It("UT-DS-1199-006: WithEventDataFilter combined with other filters chains correctly", func() {
		builder := query.NewAuditEventsQueryBuilder().
			WithEventType("apifrontend.a2a.task_completed").
			WithEventDataFilter("task_id", "task-abc")

		sql, args, err := builder.Build()
		Expect(err).NotTo(HaveOccurred())
		Expect(sql).To(ContainSubstring("AND event_type = $1"))
		Expect(sql).To(ContainSubstring("AND event_data->>'task_id' = $2"))

		Expect(args[0]).To(Equal("apifrontend.a2a.task_completed"))
		Expect(args[1]).To(Equal("task-abc"))

		limitIdx := strings.Index(sql, "LIMIT")
		Expect(limitIdx).To(BeNumerically(">", 0))
		Expect(sql[limitIdx:]).To(ContainSubstring("$3"))
		Expect(sql[limitIdx:]).To(ContainSubstring("$4"))
	})

	It("UT-DS-1199-007: WithEventDataFilter rejects key with non-alphanumeric characters", func() {
		builder := query.NewAuditEventsQueryBuilder().
			WithEventDataFilter("task_id'; DROP TABLE--", "val")

		_, _, err := builder.Build()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid"))
	})

	It("UT-DS-1199-008: WithEventDataFilter rejects empty key", func() {
		builder := query.NewAuditEventsQueryBuilder().
			WithEventDataFilter("", "val")

		_, _, err := builder.Build()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid"))
	})

	It("UT-DS-1199-009: WithEventDataFilter with empty value does not apply filter", func() {
		builder := query.NewAuditEventsQueryBuilder().
			WithEventDataFilter("task_id", "")

		sql, _, err := builder.Build()
		Expect(err).NotTo(HaveOccurred())
		Expect(sql).NotTo(ContainSubstring("event_data->>'task_id'"))
	})

	It("UT-DS-1199-012: BuildCount with WithEventDataFilter includes JSONB filter", func() {
		builder := query.NewAuditEventsQueryBuilder().
			WithEventDataFilter("rr_name", "rr-oom-web")

		sql, args, err := builder.BuildCount()
		Expect(err).NotTo(HaveOccurred())
		Expect(sql).To(ContainSubstring("event_data->>'rr_name' = $"))
		Expect(args).To(ContainElement("rr-oom-web"))
		Expect(sql).To(HavePrefix("SELECT COUNT(*)"))
	})
})
