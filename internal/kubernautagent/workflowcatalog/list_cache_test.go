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

package workflowcatalog

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// UNIT TESTS: sortWorkflowsByCreatedAtDesc (Issue #1677 Phase 2b)
// ========================================
// Authority: DD-WORKFLOW-019. Ported verbatim from
// pkg/datastorage/repository/workflow/list_cache_test.go (Issue #1661
// Change 6, Phase 55 prerequisite) -- pure sort helper behind Catalog.List's
// cache-backed implementation (list_cache.go) -- mirrors List's SQL `ORDER
// BY created_at DESC, workflow_id ASC`. The cache-fetch itself
// (Catalog.listFromCache) is proven by the Phase 2a integration test,
// matching the Pyramid Invariant: "UT proves logic. IT proves wiring."
// ========================================

var _ = Describe("sortWorkflowsByCreatedAtDesc (Issue #1677 Phase 2b)", func() {
	It("UT-KA-1677-616-001: sorts by CreatedAt descending", func() {
		older := models.RemediationWorkflow{WorkflowID: "b", CreatedAt: time.Now().Add(-time.Hour)}
		newer := models.RemediationWorkflow{WorkflowID: "a", CreatedAt: time.Now()}

		workflows := []models.RemediationWorkflow{older, newer}
		sortWorkflowsByCreatedAtDesc(workflows)

		Expect(workflows[0].WorkflowID).To(Equal("a"), "newer CreatedAt must sort first")
		Expect(workflows[1].WorkflowID).To(Equal("b"))
	})

	It("UT-KA-1677-616-002: ties break on WorkflowID ascending (deterministic tiebreaker)", func() {
		now := time.Now()
		wfZ := models.RemediationWorkflow{WorkflowID: "zzz", CreatedAt: now}
		wfA := models.RemediationWorkflow{WorkflowID: "aaa", CreatedAt: now}

		workflows := []models.RemediationWorkflow{wfZ, wfA}
		sortWorkflowsByCreatedAtDesc(workflows)

		Expect(workflows[0].WorkflowID).To(Equal("aaa"))
		Expect(workflows[1].WorkflowID).To(Equal("zzz"))
	})

	It("UT-KA-1677-616-003: empty slice does not panic", func() {
		workflows := []models.RemediationWorkflow{}
		Expect(func() { sortWorkflowsByCreatedAtDesc(workflows) }).ToNot(Panic())
	})
})
