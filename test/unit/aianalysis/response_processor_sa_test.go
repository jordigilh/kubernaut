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

package aianalysis

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
)

// ========================================
// RESPONSE PROCESSOR SA MAPPING TESTS (#481)
// ========================================
// Authority: DD-WE-005 v2.0 (Per-Workflow ServiceAccount Reference)
// Validates GetStringFromMap correctly extracts service_account_name
// from HAPI response maps (same pattern used at 3 mapping sites).
// ========================================

var _ = Describe("Response Processor SA Mapping [DD-WE-005] (#481)", func() {

	Context("GetStringFromMap for service_account_name", func() {

		It("UT-AA-481-001: should extract service_account_name from selected_workflow map", func() {
			swMap := map[string]interface{}{
				"workflow_id":          "wf-uuid-123",
				"execution_bundle":     "quay.io/test:v1@sha256:abc",
				"confidence":           0.95,
				"service_account_name": "my-workflow-sa",
			}
			result := handlers.GetStringFromMap(swMap, "service_account_name")
			Expect(result).To(Equal("my-workflow-sa"))
		})

		It("UT-AA-481-002: should return empty string when service_account_name is absent", func() {
			swMap := map[string]interface{}{
				"workflow_id":      "wf-uuid-456",
				"execution_bundle": "quay.io/test:v1@sha256:def",
				"confidence":       0.85,
			}
			result := handlers.GetStringFromMap(swMap, "service_account_name")
			Expect(result).To(BeEmpty())
		})
	})
})
