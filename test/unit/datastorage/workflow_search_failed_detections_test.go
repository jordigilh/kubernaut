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
// BR-STORAGE-020: failedDetections Support in Workflow Search
// ========================================
//
// TDD RED Phase: Write failing tests FIRST
//
// Business Requirement:
// When matching incident DetectedLabels against workflow catalog detected_labels,
// Data Storage MUST skip fields that are in failedDetections.
//
// Per DD-WORKFLOW-001 v2.1:
// - failedDetections []string tracks which detections failed (RBAC, timeout)
// - If a field is in failedDetections, its value should be ignored
// - Empty array = all detections succeeded
//
// Testing Strategy (per TESTING_GUIDELINES.md):
// - Unit tests: Validate implementation correctness (model validation, field presence)
// - Integration tests: Validate behavior (SQL filtering actually skips fields)
//
// ========================================

var _ = Describe("BR-STORAGE-020: FailedDetections Support", func() {

	// ========================================
	// Unit Test 1: Model Field Existence
	// ========================================
	// Tests that the FailedDetections field exists in the DetectedLabels model
	// This is implementation correctness - does the field exist?

	Describe("DetectedLabels Model", func() {
		Context("when FailedDetections field is present", func() {
			// BEHAVIOR: DetectedLabels model supports FailedDetections field
			// CORRECTNESS: Field is accessible and stores correct values
			It("should allow setting and getting FailedDetections", func() {
				// ARRANGE: Create DetectedLabels with FailedDetections
				dl := &models.DetectedLabels{
					FailedDetections: []string{"pdbProtected", "hpaEnabled"},
				}

				// ASSERT: Field is accessible and has correct values
				Expect(dl.FailedDetections).To(HaveLen(2))
				Expect(dl.FailedDetections).To(ContainElements("pdbProtected", "hpaEnabled"))
			})

			It("should allow empty FailedDetections (all detections succeeded)", func() {
				// ARRANGE: Create DetectedLabels with empty FailedDetections
				dl := &models.DetectedLabels{
					FailedDetections: []string{},
				}

				// ASSERT: Empty array is valid
				Expect(dl.FailedDetections).To(BeEmpty())
			})

			It("should allow nil FailedDetections (backwards compatibility)", func() {
				// ARRANGE: Create DetectedLabels without FailedDetections
				gitOpsTool := "argocd"
				dl := &models.DetectedLabels{
					GitOpsTool: gitOpsTool,
				}

				// ASSERT: nil FailedDetections is valid (treated as empty)
				Expect(dl.FailedDetections).To(BeNil())
			})
		})
	})

	// ========================================
	// Unit Test 2: WorkflowSearchFilters with FailedDetections
	// ========================================
	// Tests that WorkflowSearchFilters can include DetectedLabels with FailedDetections

	Describe("WorkflowSearchFilters", func() {
		Context("when DetectedLabels includes FailedDetections", func() {
			It("should include FailedDetections in search filters", func() {
				// ARRANGE: Create search filters with failed detections
				filters := &models.WorkflowSearchFilters{
					SignalName: "OOMKilled",
					Severity:   "critical",
					DetectedLabels: models.DetectedLabels{
						GitOpsTool:       "argocd",
						PDBProtected:     true,
						FailedDetections: []string{"hpaEnabled", "stateful"},
					},
				}

				// ASSERT: FailedDetections is accessible through filters
				Expect(filters.DetectedLabels.FailedDetections).To(HaveLen(2))
				Expect(filters.DetectedLabels.FailedDetections).To(ContainElements("hpaEnabled", "stateful"))

				Expect(filters.DetectedLabels.GitOpsTool).To(Equal("argocd"))
				Expect(filters.DetectedLabels.PDBProtected).To(BeTrue())
			})
		})
	})

	// ========================================
	// Unit Test 3: FailedDetections Validation
	// ========================================
	// Tests that FailedDetections only accepts valid field names

	Describe("FailedDetections Validation", func() {
		Context("when validating field names", func() {
			It("should accept valid DetectedLabels field names", func() {
				// ARRANGE: Valid field names per DD-WORKFLOW-001 v2.2
				// Using constants from models package
				// NOTE: podSecurityLevel removed in v2.2 (PSP deprecated, PSS is namespace-level)
				validFields := []string{"gitOpsManaged", "gitOpsTool", "pdbProtected", "hpaEnabled", "stateful", "helmManaged", "networkIsolated", "serviceMesh"}

				dl := &models.DetectedLabels{
					FailedDetections: validFields,
				}

				// ASSERT: All valid fields are accepted (8 fields per DD-WORKFLOW-001 v2.2)
				// podSecurityLevel removed in v2.2
				Expect(dl.FailedDetections).To(HaveLen(8))
				for _, field := range validFields {
					Expect(dl.FailedDetections).To(ContainElement(field))
				}
			})

			It("should validate field names using IsValidFailedDetectionField", func() {
				// ASSERT: Valid field names return true
				Expect(models.IsValidFailedDetectionField("gitOpsManaged")).To(BeTrue())
				Expect(models.IsValidFailedDetectionField("pdbProtected")).To(BeTrue())
				Expect(models.IsValidFailedDetectionField("hpaEnabled")).To(BeTrue())

				// ASSERT: Invalid field names return false
				Expect(models.IsValidFailedDetectionField("invalidField")).To(BeFalse())
				Expect(models.IsValidFailedDetectionField("")).To(BeFalse())
			})
		})
	})

	// ========================================
	// Unit Test 4: Helper Function for Skip Logic
	// ========================================
	// Tests the helper function that determines if a field should be skipped

	Describe("ShouldSkipField helper", func() {
		Context("when checking if a field should be skipped", func() {
			It("should return true for fields in FailedDetections", func() {
				failedDetections := []string{"pdbProtected", "hpaEnabled"}

				// ASSERT: Fields in the list should be skipped
				Expect(models.ShouldSkipDetectedLabel("pdbProtected", failedDetections)).To(BeTrue())
				Expect(models.ShouldSkipDetectedLabel("hpaEnabled", failedDetections)).To(BeTrue())
			})

			It("should return false for fields NOT in FailedDetections", func() {
				failedDetections := []string{"pdbProtected", "hpaEnabled"}

				// ASSERT: Fields not in the list should NOT be skipped
				Expect(models.ShouldSkipDetectedLabel("gitOpsManaged", failedDetections)).To(BeFalse())
				Expect(models.ShouldSkipDetectedLabel("stateful", failedDetections)).To(BeFalse())
				Expect(models.ShouldSkipDetectedLabel("helmManaged", failedDetections)).To(BeFalse())
			})

			It("should return false when FailedDetections is empty", func() {
				failedDetections := []string{}

				// ASSERT: No fields should be skipped when list is empty
				Expect(models.ShouldSkipDetectedLabel("pdbProtected", failedDetections)).To(BeFalse())
				Expect(models.ShouldSkipDetectedLabel("gitOpsManaged", failedDetections)).To(BeFalse())
			})

			It("should return false when FailedDetections is nil", func() {
				var failedDetections []string = nil

				// ASSERT: No fields should be skipped when list is nil
				Expect(models.ShouldSkipDetectedLabel("pdbProtected", failedDetections)).To(BeFalse())
			})
		})
	})
})
