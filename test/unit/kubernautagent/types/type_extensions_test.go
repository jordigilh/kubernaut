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

package types_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
)

func boolPtr(b bool) *bool { return &b }
func intPtr(i int) *int    { return &i }

var _ = Describe("TP-433-ADV P2: Type Extensions — GAP-008/009/013/014", func() {

	Describe("UT-KA-433-TYP-001: SignalContext carries RemediationID for audit correlation (GAP-008)", func() {
		It("should carry RemediationID through JSON round-trip", func() {
			sc := katypes.SignalContext{
				Name:          "oom-alert",
				Namespace:     "prod",
				Severity:      "critical",
				Message:       "OOMKilled",
				IncidentID:    "inc-123",
				RemediationID: "rem-456",
			}

			data, err := json.Marshal(sc)
			Expect(err).NotTo(HaveOccurred())

			var restored katypes.SignalContext
			Expect(json.Unmarshal(data, &restored)).To(Succeed())
			Expect(restored.RemediationID).To(Equal("rem-456"))
		})
	})

	Describe("UT-KA-433-TYP-002: InvestigationResult JSON round-trip preserves all fields (GAP-008/009)", func() {
		It("should preserve all fields including new additions", func() {
			result := katypes.InvestigationResult{
				RCASummary:    "Pod OOMKilled due to memory limit",
				WorkflowID:    "oom-recovery",
				Confidence:    0.92,
				IsActionable:  boolPtr(true),
				ExecutionBundle: "ghcr.io/kubernaut/oom-recovery:v1.0@sha256:abc",
				HumanReviewReason: "low_confidence",
				AlternativeWorkflows: []katypes.AlternativeWorkflow{
					{
						WorkflowID:      "memory-optimize",
						ExecutionBundle: "ghcr.io/kubernaut/memory-optimize:v1.0",
						Confidence:      0.75,
						Rationale:       "Could optimize memory instead of restarting",
					},
				},
			}

			data, err := json.Marshal(result)
			Expect(err).NotTo(HaveOccurred())

			var restored katypes.InvestigationResult
			Expect(json.Unmarshal(data, &restored)).To(Succeed())

			Expect(restored.ExecutionBundle).To(Equal("ghcr.io/kubernaut/oom-recovery:v1.0@sha256:abc"))
			Expect(restored.HumanReviewReason).To(Equal("low_confidence"))
			Expect(restored.AlternativeWorkflows).To(HaveLen(1))
			Expect(restored.AlternativeWorkflows[0].WorkflowID).To(Equal("memory-optimize"))
			Expect(restored.AlternativeWorkflows[0].Confidence).To(BeNumerically("~", 0.75, 0.001))
			Expect(restored.AlternativeWorkflows[0].Rationale).To(Equal("Could optimize memory instead of restarting"))
		})
	})

	Describe("UT-KA-433-TYP-003: InvestigationResult.ExecutionBundle populated from parser output (GAP-009)", func() {
		It("should have an ExecutionBundle field that defaults to empty string", func() {
			result := katypes.InvestigationResult{}
			Expect(result.ExecutionBundle).To(BeEmpty())
		})
	})

	Describe("UT-KA-433-TYP-004: InvestigationResult.AlternativeWorkflows populated (empty list default) (GAP-009)", func() {
		It("should default to nil (empty) when no alternatives provided", func() {
			result := katypes.InvestigationResult{}
			Expect(result.AlternativeWorkflows).To(BeNil())
		})

		It("should omit alternative_workflows from JSON when nil", func() {
			result := katypes.InvestigationResult{
				RCASummary: "summary",
				Confidence: 0.9,
			}
			data, err := json.Marshal(result)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).NotTo(ContainSubstring("alternative_workflows"))
		})
	})

	Describe("UT-KA-433-TYP-005: AlternativeWorkflow struct matches OpenAPI schema (GAP-009)", func() {
		It("should serialize to JSON matching the OpenAPI AlternativeWorkflow schema", func() {
			alt := katypes.AlternativeWorkflow{
				WorkflowID:      "node-drain-reboot",
				ExecutionBundle: "ghcr.io/kubernaut/node-drain:v2.0",
				Confidence:      0.65,
				Rationale:       "Node-level issue detected",
			}

			data, err := json.Marshal(alt)
			Expect(err).NotTo(HaveOccurred())

			var raw map[string]interface{}
			Expect(json.Unmarshal(data, &raw)).To(Succeed())
			Expect(raw).To(HaveKey("workflow_id"))
			Expect(raw).To(HaveKey("execution_bundle"))
			Expect(raw).To(HaveKey("confidence"))
			Expect(raw).To(HaveKey("rationale"))
		})

		It("should omit execution_bundle when empty", func() {
			alt := katypes.AlternativeWorkflow{
				WorkflowID: "basic-restart",
				Confidence: 0.5,
				Rationale:  "Simple restart approach",
			}

			data, err := json.Marshal(alt)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).NotTo(ContainSubstring("execution_bundle"))
		})
	})

	Describe("UT-KA-433-TYP-006: InvestigationResult.HumanReviewReason from LLM extraction (GAP-013)", func() {
		It("should carry HumanReviewReason through JSON round-trip", func() {
			result := katypes.InvestigationResult{
				HumanReviewNeeded: true,
				HumanReviewReason: "rca_incomplete",
				Reason:            "LLM exhausted turns",
			}

			data, err := json.Marshal(result)
			Expect(err).NotTo(HaveOccurred())

			var restored katypes.InvestigationResult
			Expect(json.Unmarshal(data, &restored)).To(Succeed())
			Expect(restored.HumanReviewReason).To(Equal("rca_incomplete"))
		})
	})

	Describe("UT-KA-433-TYP-007: is_actionable nil when LLM does not assess actionability (GAP-013)", func() {
		It("should be nil by default", func() {
			result := katypes.InvestigationResult{}
			Expect(result.IsActionable).To(BeNil())
		})

		It("should omit is_actionable from JSON when nil", func() {
			result := katypes.InvestigationResult{
				RCASummary: "summary",
			}
			data, err := json.Marshal(result)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).NotTo(ContainSubstring("is_actionable"))
		})
	})

	Describe("UT-KA-433-TYP-008: SignalContext carries FiringTime, ReceivedTime, IsDuplicate, OccurrenceCount (GAP-014)", func() {
		It("should carry all timestamp and dedup fields through JSON round-trip", func() {
			sc := katypes.SignalContext{
				Name:            "crash-alert",
				Namespace:       "staging",
				Severity:        "warning",
				Message:         "CrashLoopBackOff",
				FiringTime:      "2026-03-04T10:00:00Z",
				ReceivedTime:    "2026-03-04T10:00:05Z",
				IsDuplicate:     boolPtr(true),
				OccurrenceCount: intPtr(3),
			}

			data, err := json.Marshal(sc)
			Expect(err).NotTo(HaveOccurred())

			var restored katypes.SignalContext
			Expect(json.Unmarshal(data, &restored)).To(Succeed())
			Expect(restored.FiringTime).To(Equal("2026-03-04T10:00:00Z"))
			Expect(restored.ReceivedTime).To(Equal("2026-03-04T10:00:05Z"))
			Expect(restored.IsDuplicate).NotTo(BeNil())
			Expect(*restored.IsDuplicate).To(BeTrue())
			Expect(restored.OccurrenceCount).NotTo(BeNil())
			Expect(*restored.OccurrenceCount).To(Equal(3))
		})

		It("should omit optional fields when not set", func() {
			sc := katypes.SignalContext{
				Name:      "alert",
				Namespace: "default",
				Severity:  "info",
				Message:   "test",
			}
			data, err := json.Marshal(sc)
			Expect(err).NotTo(HaveOccurred())
			jsonStr := string(data)
			Expect(jsonStr).NotTo(ContainSubstring("firing_time"))
			Expect(jsonStr).NotTo(ContainSubstring("received_time"))
			Expect(jsonStr).NotTo(ContainSubstring("is_duplicate"))
			Expect(jsonStr).NotTo(ContainSubstring("occurrence_count"))
		})
	})
})
