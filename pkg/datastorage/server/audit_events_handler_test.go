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

package server

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kubelog "github.com/jordigilh/kubernaut/pkg/log"
)

var _ = Describe("Audit Events Handler — parseQueryFilters (Issue #1199)", func() {
	var srv *Server

	BeforeEach(func() {
		srv = &Server{
			logger: kubelog.NewLogger(kubelog.DefaultOptions()),
		}
	})

	It("UT-DS-1199-018: detail_key and detail_value both present parses correctly", func() {
		req := httptest.NewRequest(http.MethodGet,
			"/api/v1/audit/events?detail_key=task_id&detail_value=task-abc", nil)

		filters, err := srv.parseQueryFilters(req)

		Expect(err).ToNot(HaveOccurred())
		Expect(filters.detailKey).To(Equal("task_id"))
		Expect(filters.detailValue).To(Equal("task-abc"))
	})

	It("UT-DS-1199-019: neither detail_key nor detail_value results in empty strings", func() {
		req := httptest.NewRequest(http.MethodGet,
			"/api/v1/audit/events?event_type=apifrontend.a2a.task_completed", nil)

		filters, err := srv.parseQueryFilters(req)

		Expect(err).ToNot(HaveOccurred())
		Expect(filters.detailKey).To(BeEmpty())
		Expect(filters.detailValue).To(BeEmpty())
	})

	It("UT-DS-1199-020: detail_key without detail_value returns validation error", func() {
		req := httptest.NewRequest(http.MethodGet,
			"/api/v1/audit/events?detail_key=task_id", nil)

		_, err := srv.parseQueryFilters(req)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("detail_key and detail_value must both be provided"))
	})

	It("UT-DS-1199-020b: detail_value without detail_key returns validation error", func() {
		req := httptest.NewRequest(http.MethodGet,
			"/api/v1/audit/events?detail_value=task-abc", nil)

		_, err := srv.parseQueryFilters(req)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("detail_key and detail_value must both be provided"))
	})

	It("UT-DS-1199-021: buildQueryFromFilters wires detail_key/detail_value to builder", func() {
		filters := &queryFilters{
			detailKey:   "rr_name",
			detailValue: "rr-oom-web",
			limit:       100,
			offset:      0,
		}

		builder := srv.buildQueryFromFilters(filters)
		sql, args, err := builder.Build()

		Expect(err).ToNot(HaveOccurred())
		Expect(sql).To(ContainSubstring("event_data->>'rr_name'"))
		Expect(args).To(ContainElement("rr-oom-web"))
	})
})
