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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// UT-DS-464-007: ValidateMandatoryLabels severity wildcard
// ========================================
// Authority: DD-WORKFLOW-001 v2.8 (severity wildcard restored)
// Issue: #464 (defensive fixes for wildcard label handling)
// ========================================

var _ = Describe("ValidateMandatoryLabels severity wildcard", func() {
	Context("UT-DS-464-007: severity '*' accepted per DD-WORKFLOW-001 v2.8", func() {
		It("should accept severity ['*'] as a valid wildcard value", func() {
			labels := &models.WorkflowSchemaLabels{
				Severity:    []string{"*"},
				Component:   []string{"pod"},
				Environment: []string{"production"},
				Priority:    "P1",
			}

			err := labels.ValidateMandatoryLabels()
			Expect(err).ToNot(HaveOccurred(),
				"severity ['*'] must be accepted per DD-WORKFLOW-001 v2.8")
		})

		It("should accept severity ['*'] alongside explicit values", func() {
			labels := &models.WorkflowSchemaLabels{
				Severity:    []string{"critical", "*"},
				Component:   []string{"pod"},
				Environment: []string{"production"},
				Priority:    "P1",
			}

			err := labels.ValidateMandatoryLabels()
			Expect(err).ToNot(HaveOccurred(),
				"severity ['critical', '*'] must be accepted")
		})

		It("should still reject invalid severity values", func() {
			labels := &models.WorkflowSchemaLabels{
				Severity:    []string{"invalid-value"},
				Component:   []string{"pod"},
				Environment: []string{"production"},
				Priority:    "P1",
			}

			err := labels.ValidateMandatoryLabels()
			Expect(err).To(HaveOccurred(),
				"invalid severity values must still be rejected")
		})

		It("should still accept standard severity values", func() {
			labels := &models.WorkflowSchemaLabels{
				Severity:    []string{"critical", "high", "medium", "low"},
				Component:   []string{"pod"},
				Environment: []string{"production"},
				Priority:    "P1",
			}

			err := labels.ValidateMandatoryLabels()
			Expect(err).ToNot(HaveOccurred(),
				"standard severity values must remain valid")
		})
	})
})
