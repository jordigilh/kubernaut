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

package controller

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// TDD Phase: RED — Issue #594 Override Type Serialization Tests
// BR-ORCH-030: WorkflowOverride CRD type contract
//
// These tests validate that the WorkflowOverride struct serializes
// and deserializes correctly via JSON, and that nil values are
// properly omitted for backward compatibility.

var _ = Describe("BR-ORCH-030: WorkflowOverride Type Serialization (#594)", func() {

	Describe("UT-OV-594-001: WorkflowOverride JSON round-trip preserves all fields", func() {
		It("should preserve WorkflowName, Parameters, and Rationale through marshal/unmarshal", func() {
			original := remediationv1.RemediationApprovalRequestStatus{
				Decision: remediationv1.ApprovalDecisionApproved,
				WorkflowOverride: &remediationv1.WorkflowOverride{
					WorkflowName: "drain-restart",
					Parameters: map[string]string{
						"TIMEOUT": "30s",
						"FORCE":   "true",
					},
					Rationale: "prefer safe restart over rolling update",
				},
			}

			data, err := json.Marshal(original)
			Expect(err).NotTo(HaveOccurred())

			var restored remediationv1.RemediationApprovalRequestStatus
			err = json.Unmarshal(data, &restored)
			Expect(err).NotTo(HaveOccurred())

			Expect(restored.WorkflowOverride).NotTo(BeNil())
			Expect(restored.WorkflowOverride.WorkflowName).To(Equal("drain-restart"))
			Expect(restored.WorkflowOverride.Parameters).To(HaveLen(2))
			Expect(restored.WorkflowOverride.Parameters["TIMEOUT"]).To(Equal("30s"))
			Expect(restored.WorkflowOverride.Parameters["FORCE"]).To(Equal("true"))
			Expect(restored.WorkflowOverride.Rationale).To(Equal("prefer safe restart over rolling update"))
		})
	})

	Describe("UT-OV-594-002: Nil WorkflowOverride is omitted from JSON", func() {
		It("should not include workflowOverride key when nil (backward compat)", func() {
			status := remediationv1.RemediationApprovalRequestStatus{
				Decision:         remediationv1.ApprovalDecisionApproved,
				WorkflowOverride: nil,
			}

			data, err := json.Marshal(status)
			Expect(err).NotTo(HaveOccurred())

			var raw map[string]interface{}
			err = json.Unmarshal(data, &raw)
			Expect(err).NotTo(HaveOccurred())

			_, exists := raw["workflowOverride"]
			Expect(exists).To(BeFalse(), "workflowOverride should be omitted when nil")
		})
	})
})
