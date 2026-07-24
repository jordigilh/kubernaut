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

package models

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Relocated from pkg/datastorage/workflow_discovery_handler_test.go (#1677
// Phase 2g, DD-WORKFLOW-019): that file's ParseDiscoveryFilters/ParsePagination
// coverage was deleted alongside the DS HTTP handlers retiring workflow
// discovery to KubernautAgent. HasContextFilters itself is a model-level
// method with no DS-specific behavior -- still exercised by both KA's
// workflowcatalog discovery path and its custom MCP tools -- so its unit
// coverage moves here, alongside the model it belongs to.
//
// Test Plan: docs/testing/DD-HAPI-017/TEST_PLAN.md
// Test ID: UT-DS-017-005-001.
var _ = Describe("WorkflowDiscoveryFilters.HasContextFilters", func() {
	Context("UT-DS-017-005-001: remediationId propagated correctly", func() {
		It("should detect when context filters are present", func() {
			filters := &WorkflowDiscoveryFilters{
				Severity:    "critical",
				Component:   "v1/Pod",
				Environment: "production",
				Priority:    "P0",
			}
			Expect(filters.HasContextFilters()).To(BeTrue())
		})

		It("should detect when no context filters are present", func() {
			filters := &WorkflowDiscoveryFilters{
				RemediationID: "rem-uuid-123",
			}
			Expect(filters.HasContextFilters()).To(BeFalse())
		})

		It("should detect partial context filters", func() {
			filters := &WorkflowDiscoveryFilters{
				Severity: "critical",
			}
			Expect(filters.HasContextFilters()).To(BeTrue())
		})
	})
})
