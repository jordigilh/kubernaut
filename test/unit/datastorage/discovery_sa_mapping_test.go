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
// DISCOVERY ENTRY SA MAPPING TESTS (#481)
// ========================================
// Authority: DD-WE-005 v2.0 (Per-Workflow ServiceAccount Reference)
// Validates that WorkflowDiscoveryEntry correctly maps ServiceAccountName
// from RemediationWorkflow (same logic as HandleListWorkflowsByActionType).
// ========================================

var _ = Describe("WorkflowDiscoveryEntry SA Mapping [DD-WE-005] (#481)", func() {

	Context("serviceAccountName propagation from RemediationWorkflow", func() {

		It("UT-DS-481-005: should include serviceAccountName in discovery entry when workflow has SA", func() {
			saName := "my-workflow-sa"
			wf := models.RemediationWorkflow{
				WorkflowID:         "uuid-123",
				WorkflowName:       "test-workflow",
				Name:               "Test Workflow",
				Version:            "1.0.0",
				ServiceAccountName: &saName,
			}

			entry := models.WorkflowDiscoveryEntry{
				WorkflowID:      wf.WorkflowID,
				WorkflowName:    wf.WorkflowName,
				Name:            wf.Name,
				Version:         wf.Version,
				ExecutionEngine: string(wf.ExecutionEngine),
			}
			if wf.ServiceAccountName != nil {
				entry.ServiceAccountName = *wf.ServiceAccountName
			}

			Expect(entry.ServiceAccountName).To(Equal("my-workflow-sa"))
		})

		It("UT-DS-481-006: should omit serviceAccountName in discovery entry when workflow has no SA", func() {
			wf := models.RemediationWorkflow{
				WorkflowID:   "uuid-456",
				WorkflowName: "no-sa-workflow",
				Name:         "No SA Workflow",
				Version:      "1.0.0",
			}

			entry := models.WorkflowDiscoveryEntry{
				WorkflowID:      wf.WorkflowID,
				WorkflowName:    wf.WorkflowName,
				Name:            wf.Name,
				Version:         wf.Version,
				ExecutionEngine: string(wf.ExecutionEngine),
			}
			if wf.ServiceAccountName != nil {
				entry.ServiceAccountName = *wf.ServiceAccountName
			}

			Expect(entry.ServiceAccountName).To(BeEmpty())
		})
	})
})
