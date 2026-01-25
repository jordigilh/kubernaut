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

package remediationorchestrator

import (
	. "github.com/onsi/ginkgo/v2"
)

// ========================================
// Phase 2 E2E Tests - Routing Workflow Cooldown
// ========================================
//
// PHASE 2 PATTERN: RO Controller + Child Controllers Running
// - RO controller orchestrates full remediation lifecycle
// - Cooldown logic prevents duplicate workflows on same target
// - Tests validate RecentlyRemediated blocking behavior
//
// PENDING: These tests await controller deployment in E2E suite.
// See test/e2e/remediationorchestrator/suite_test.go lines 142-147 for TODO.
//
// Business Value:
// - Workflow cooldown prevents redundant remediation attempts
// - BlockedUntil expiry allows retry after cooldown period
// ========================================

var _ = Describe("Routing Workflow Cooldown E2E Tests", Label("e2e", "routing", "cooldown", "pending"), func() {

	Describe("Workflow Cooldown Blocking (RecentlyRemediated)", func() {

		It("should block RR when same workflow+target executed within cooldown period", func() {
			// Test validates:
			// - First RR completes successfully with WorkflowExecution
			// - Second RR with same workflow+target transitions to Blocked
			// - BlockReason is RecentlyRemediated
			// - BlockedUntil is set for time-based cooldown
			// - BlockingWorkflowExecution references the recent WFE
			// - NO WorkflowExecution created for blocked RR
			//
			// NOTE: This test requires full RO+child controller orchestration.
			// Integration tests use manual CRD creation (Phase 1 pattern).
			// E2E tests validate automatic orchestration (Phase 2 pattern).
		})
	})
})
