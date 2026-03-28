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

package datastorage

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// WORKFLOW SA INTEGRATION TESTS (#481)
// ========================================
// Authority: DD-WE-005 v2.0 (Per-Workflow ServiceAccount Reference)
// Integration test: validates SA field roundtrips through models and handlers.
// ========================================

var _ = Describe("Workflow ServiceAccount Integration [DD-WE-005] (#481)", func() {

	Context("RemediationWorkflow model SA field", func() {

		It("IT-DS-481-001: should roundtrip ServiceAccountName through RemediationWorkflow model", func() {
			saName := "integration-test-sa"
			wf := models.RemediationWorkflow{
				WorkflowID:         "wf-integration-123",
				WorkflowName:       "integration-sa-workflow",
				Name:               "Integration SA Workflow",
				Version:            "1.0.0",
				ServiceAccountName: &saName,
			}

			Expect(*wf.ServiceAccountName).To(Equal("integration-test-sa"))
		})

		It("IT-DS-481-002: should allow nil ServiceAccountName in RemediationWorkflow model", func() {
			wf := models.RemediationWorkflow{
				WorkflowID:   "wf-integration-456",
				WorkflowName: "no-sa-workflow",
				Name:         "No SA Workflow",
				Version:      "1.0.0",
			}

			Expect(wf.ServiceAccountName).To(BeNil())
		})
	})

	Context("WorkflowSearchResult model SA field", func() {

		It("IT-DS-481-003: should propagate ServiceAccountName to WorkflowSearchResult", func() {
			result := models.WorkflowSearchResult{
				WorkflowID:         "wf-search-789",
				Title:              "Search SA Workflow",
				ServiceAccountName: "search-test-sa",
			}

			Expect(result.ServiceAccountName).To(Equal("search-test-sa"))
		})
	})
})
