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

Issue #1684 (CodeQL go/sql-injection remediation): whitebox regression test
for the appendFilters-local JSONB-key guard. Build()/BuildCount() already
reject an invalid key via validateEventDataFilter() before appendFilters ever
runs, so this path is unreachable through the public API today -- this test
exists to prove the defense-in-depth guard inside appendFilters itself still
holds if that were ever bypassed (e.g. a future internal caller that invokes
appendFilters directly), since that guard is what makes the interpolation
site in appendFilters provably safe to a static analyzer, independent of
callers.
*/

package query

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("appendFilters — JSONB key guard (Issue #1684 defense-in-depth)", func() {
	It("UT-DS-1684-010: does not interpolate an unvalidated key into SQL even when called directly", func() {
		key := "task_id'; DROP TABLE audit_events;--"
		value := "val"
		b := &AuditEventsQueryBuilder{
			eventDataKey:   &key,
			eventDataValue: &value,
		}

		sql, args := b.appendFilters("SELECT 1 WHERE 1=1", nil)

		Expect(sql).NotTo(ContainSubstring("DROP TABLE"))
		Expect(sql).NotTo(ContainSubstring("event_data->>"))
		Expect(args).To(BeEmpty())
	})

	It("UT-DS-1684-011: interpolates a valid key as before", func() {
		key := "task_id"
		value := "task-abc"
		b := &AuditEventsQueryBuilder{
			eventDataKey:   &key,
			eventDataValue: &value,
		}

		sql, args := b.appendFilters("SELECT 1 WHERE 1=1", nil)

		Expect(sql).To(ContainSubstring("event_data->>'task_id' = $1"))
		Expect(args).To(ContainElement("task-abc"))
	})
})
