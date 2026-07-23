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

package datastorage

import (
	"context"
	"io"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ========================================
// E2E-DS-1677-RETIRE: Workflow discovery REST surface fully retired
// ========================================
//
// Business Requirements:
//   - N/A (pure regression protection for a deletion, not new behavior)
//
// Design Decisions:
//   - DD-WORKFLOW-019: workflow/action-type discovery ownership moves from
//     DataStorage to KubernautAgent. DS's `/api/v1/workflows*` REST surface
//     (three-step discovery: list actions, list workflows by action type,
//     get workflow by ID) and `/api/v1/action-types/{name}/workflow-count`
//     were deleted as dead code once KubernautAgent's own informer-backed
//     workflow catalog (internal/kubernautagent/workflowcatalog) became the
//     sole discovery path -- see Issue #1677 Phase 2g.
//
// This suite replaces 04_workflow_discovery_test.go and
// 06_workflow_discovery_audit_test.go, which previously exercised these
// endpoints directly. Equivalent (and stronger) coverage of the discovery
// protocol itself -- including the context-filter security gate
// (IT-KA-1677-DHAPI017-006) and the 4 catalog audit events
// (IT-KA-1677-AUDIT-001..004) -- now lives against KubernautAgent:
//   - test/integration/kubernautagent/workflowcatalog/discovery_edge_cases_test.go
//   - internal/kubernautagent/tools/custom/discovery_audit_test.go
//   - test/e2e/kubernautagent/three_step_discovery_test.go
//
// This file's sole purpose is a permanent regression guard: assert the
// retired routes stay retired (404/405), so a future change can't
// accidentally resurrect a DS-side discovery path that would fork from
// KubernautAgent's ownership.
var _ = Describe("E2E-DS-1677-RETIRE: DS workflow discovery REST surface stays retired (DD-WORKFLOW-019)", Label("e2e", "datastorage", "discovery", "retirement"), func() {
	var (
		testCtx    context.Context
		testCancel context.CancelFunc
	)

	BeforeEach(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 30*time.Second)
		DeferCleanup(testCancel)
	})

	DescribeTable("retired endpoint returns 404 or 405, never 200",
		func(method, path string) {
			req, err := http.NewRequestWithContext(testCtx, method, dataStorageURL+path, nil)
			Expect(err).ToNot(HaveOccurred())
			resp, err := AuthHTTPClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer func() {
				_, _ = io.ReadAll(resp.Body)
				_ = resp.Body.Close()
			}()

			Expect(resp.StatusCode).To(SatisfyAny(
				Equal(http.StatusNotFound),
				Equal(http.StatusMethodNotAllowed),
			), "%s %s must stay retired (404/405), got %d", method, path, resp.StatusCode)
		},
		Entry("E2E-DS-1677-RETIRE-001: GET /api/v1/workflows (old list endpoint)", http.MethodGet, "/api/v1/workflows"),
		Entry("E2E-DS-1677-RETIRE-002: GET /api/v1/workflows/actions (step 1)", http.MethodGet, "/api/v1/workflows/actions"),
		Entry("E2E-DS-1677-RETIRE-003: GET /api/v1/workflows/actions/ScaleReplicas (step 2)", http.MethodGet, "/api/v1/workflows/actions/ScaleReplicas"),
		Entry("E2E-DS-1677-RETIRE-004: GET /api/v1/workflows/00000000-0000-0000-0000-000000000000 (step 3)", http.MethodGet, "/api/v1/workflows/00000000-0000-0000-0000-000000000000"),
		Entry("E2E-DS-1677-RETIRE-005: GET /api/v1/action-types/ScaleReplicas/workflow-count", http.MethodGet, "/api/v1/action-types/ScaleReplicas/workflow-count"),
		Entry("E2E-DS-1677-RETIRE-006: POST /api/v1/workflows/search (removed earlier, #1661)", http.MethodPost, "/api/v1/workflows/search"),
	)
})
