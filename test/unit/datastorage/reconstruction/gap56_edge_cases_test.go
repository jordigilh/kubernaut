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

// Package reconstruction provides unit tests for Gap #5-6 edge cases.
//
// BR-AUDIT-005 Gap #5-6: Workflow References - Edge Case Testing
//
// Purpose: Validate parser/mapper resilience to incomplete or malformed audit data
//
// Testing Strategy:
// - Unit tests for error handling and graceful degradation
// - Focus on real-world scenarios: missing fields, empty values, nil data
// - Prevent production surprises with incomplete audit trails
//
// Test Case IDs:
// - PARSER-GAP56-EDGE-01: Missing PipelinerunName in Gap #6 event
// - PARSER-GAP56-EDGE-02: Empty WorkflowID in Gap #5 event
// - PARSER-GAP56-EDGE-03: Missing namespace in Gap #6 event
// - MAPPER-GAP56-EDGE-04: Nil SelectedWorkflowRef in parsed data
// - MAPPER-GAP56-EDGE-05: Nil ExecutionRef in parsed data
// - MAPPER-GAP56-EDGE-06: Empty ContainerImage in Gap #5
package reconstruction

import (
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	reconstructionpkg "github.com/jordigilh/kubernaut/pkg/datastorage/reconstruction"
)

var _ = Describe("Gap #5-6 Edge Cases (BR-AUDIT-005)", func() {

	// ========================================
	// PARSER EDGE CASES
	// ========================================

	Context("PARSER-GAP56-EDGE-01: Missing PipelinerunName in Gap #6 event", func() {
		It("should create ExecutionRef from WFE name even without PipelineRun", func() {
			// Given: execution.workflow.started event WITHOUT PipelinerunName
			event := ogenclient.AuditEvent{
				EventType:     "workflowexecution.execution.started",
				EventCategory: ogenclient.AuditEventEventCategoryWorkflowexecution,
				CorrelationID: "test-corr-001",
			}
			event.EventID.SetTo(uuid.New())
			event.Namespace.SetTo("test-namespace")

			payload := ogenclient.WorkflowExecutionAuditPayload{
				WorkflowID:      "workflow-123",
				WorkflowVersion: "v1.0.0",
				ContainerImage:  "ghcr.io/test/workflow:v1",
				ExecutionName:   "test-wfe-001",
				Phase:           ogenclient.WorkflowExecutionAuditPayloadPhasePending,
				TargetResource:  "Pod/test-pod",
			}
			// PipelinerunName is NOT set (OptString is unset)
			event.EventData = ogenclient.NewAuditEventEventDataWorkflowexecutionExecutionStartedAuditEventEventData(payload)

			// When: Parser processes the event
			parsed, err := reconstructionpkg.ParseAuditEvent(event)

			// Then: No error, ExecutionRef created successfully
			Expect(err).ToNot(HaveOccurred())
			Expect(parsed).ToNot(BeNil())
			Expect(parsed.EventType).To(Equal("workflowexecution.execution.started"))
			Expect(parsed.ExecutionRef).ToNot(BeNil(), "ExecutionRef should be created even without PipelinerunName")
			Expect(parsed.ExecutionRef.Kind).To(Equal("WorkflowExecution"))
			Expect(parsed.ExecutionRef.Name).To(Equal("test-wfe-001"))
			Expect(parsed.ExecutionRef.Namespace).To(Equal("test-namespace"))
		})
	})

	Context("PARSER-GAP56-EDGE-02: Empty WorkflowID in Gap #5 event", func() {
		It("should parse successfully with empty WorkflowID", func() {
			// Given: workflow.selection.completed event with EMPTY WorkflowID
			event := ogenclient.AuditEvent{
				EventType:     "workflowexecution.selection.completed",
				EventCategory: ogenclient.AuditEventEventCategoryWorkflowexecution,
				CorrelationID: "test-corr-002",
			}
			event.EventID.SetTo(uuid.New())

			payload := ogenclient.WorkflowExecutionAuditPayload{
				WorkflowID:      "", // Empty workflow ID (bad data scenario)
				WorkflowVersion: "v1.0.0",
				ContainerImage:  "ghcr.io/test/workflow:v1",
				ExecutionName:   "test-wfe-002",
				Phase:           ogenclient.WorkflowExecutionAuditPayloadPhasePending,
				TargetResource:  "Pod/test-pod",
			}
			event.EventData = ogenclient.NewAuditEventEventDataWorkflowexecutionSelectionCompletedAuditEventEventData(payload)

			// When: Parser processes the event
			parsed, err := reconstructionpkg.ParseAuditEvent(event)

			// Then: No error, SelectedWorkflowRef created with empty WorkflowID
			Expect(err).ToNot(HaveOccurred())
			Expect(parsed).ToNot(BeNil())
			Expect(parsed.SelectedWorkflowRef).ToNot(BeNil())
			Expect(parsed.SelectedWorkflowRef.WorkflowID).To(BeEmpty(), "Empty WorkflowID is preserved (not an error)")
			Expect(parsed.SelectedWorkflowRef.ContainerImage).To(Equal("ghcr.io/test/workflow:v1"))
		})
	})

	Context("PARSER-GAP56-EDGE-03: Missing namespace in Gap #6 event", func() {
		It("should create ExecutionRef with empty namespace", func() {
			// Given: execution.workflow.started event WITHOUT namespace
			event := ogenclient.AuditEvent{
				EventType:     "workflowexecution.execution.started",
				EventCategory: ogenclient.AuditEventEventCategoryWorkflowexecution,
				CorrelationID: "test-corr-003",
			}
			event.EventID.SetTo(uuid.New())
			// Namespace is NOT set (OptString is unset)

			payload := ogenclient.WorkflowExecutionAuditPayload{
				WorkflowID:      "workflow-456",
				WorkflowVersion: "v2.0.0",
				ContainerImage:  "ghcr.io/test/workflow:v2",
				ExecutionName:   "test-wfe-003",
				Phase:           ogenclient.WorkflowExecutionAuditPayloadPhasePending,
				TargetResource:  "Pod/test-pod",
			}
			payload.PipelinerunName.SetTo("test-pr-003")
			event.EventData = ogenclient.NewAuditEventEventDataWorkflowexecutionExecutionStartedAuditEventEventData(payload)

			// When: Parser processes the event
			parsed, err := reconstructionpkg.ParseAuditEvent(event)

			// Then: No error, ExecutionRef created with empty namespace
			Expect(err).ToNot(HaveOccurred())
			Expect(parsed).ToNot(BeNil())
			Expect(parsed.ExecutionRef).ToNot(BeNil())
			Expect(parsed.ExecutionRef.Namespace).To(BeEmpty(), "Empty namespace is acceptable for cross-namespace refs")
		})
	})

	Context("PARSER-GAP56-EDGE-04: Empty ContainerImage in Gap #5 event", func() {
		It("should parse successfully with empty ContainerImage", func() {
			// Given: workflow.selection.completed event with EMPTY ContainerImage
			event := ogenclient.AuditEvent{
				EventType:     "workflowexecution.selection.completed",
				EventCategory: ogenclient.AuditEventEventCategoryWorkflowexecution,
				CorrelationID: "test-corr-004",
			}
			event.EventID.SetTo(uuid.New())

			payload := ogenclient.WorkflowExecutionAuditPayload{
				WorkflowID:      "workflow-789",
				WorkflowVersion: "v3.0.0",
				ContainerImage:  "", // Empty container image (incomplete catalog data)
				ExecutionName:   "test-wfe-004",
				Phase:           ogenclient.WorkflowExecutionAuditPayloadPhasePending,
				TargetResource:  "Pod/test-pod",
			}
			event.EventData = ogenclient.NewAuditEventEventDataWorkflowexecutionSelectionCompletedAuditEventEventData(payload)

			// When: Parser processes the event
			parsed, err := reconstructionpkg.ParseAuditEvent(event)

			// Then: No error, SelectedWorkflowRef created with empty ContainerImage
			Expect(err).ToNot(HaveOccurred())
			Expect(parsed).ToNot(BeNil())
			Expect(parsed.SelectedWorkflowRef).ToNot(BeNil())
			Expect(parsed.SelectedWorkflowRef.ContainerImage).To(BeEmpty())
			Expect(parsed.SelectedWorkflowRef.WorkflowID).To(Equal("workflow-789"))
		})
	})

	// ========================================
	// MAPPER EDGE CASES
	// ========================================

	Context("MAPPER-GAP56-EDGE-05: Nil SelectedWorkflowRef in parsed data", func() {
		It("should skip mapping without error", func() {
			// Given: Parsed data with NIL SelectedWorkflowRef
			parsedData := &reconstructionpkg.ParsedAuditData{
				EventType:           "workflowexecution.selection.completed",
				CorrelationID:       "test-corr-005",
				SelectedWorkflowRef: nil, // Nil reference (parser failure scenario)
			}

			// When: Mapper processes the data
			fields, err := reconstructionpkg.MapToRRFields(parsedData)

			// Then: No error, Status.SelectedWorkflowRef remains nil
			Expect(err).ToNot(HaveOccurred())
			Expect(fields).ToNot(BeNil())
			Expect(fields.Status.SelectedWorkflowRef).To(BeNil(), "Nil input should result in nil output")
		})
	})

	Context("MAPPER-GAP56-EDGE-06: Nil ExecutionRef in parsed data", func() {
		It("should skip mapping without error", func() {
			// Given: Parsed data with NIL ExecutionRef
			parsedData := &reconstructionpkg.ParsedAuditData{
				EventType:     "workflowexecution.execution.started",
				CorrelationID: "test-corr-006",
				ExecutionRef:  nil, // Nil reference (parser failure scenario)
			}

			// When: Mapper processes the data
			fields, err := reconstructionpkg.MapToRRFields(parsedData)

			// Then: No error, Status.ExecutionRef remains nil
			Expect(err).ToNot(HaveOccurred())
			Expect(fields).ToNot(BeNil())
			Expect(fields.Status.ExecutionRef).To(BeNil(), "Nil input should result in nil output")
		})
	})

	Context("MAPPER-GAP56-EDGE-07: Empty strings in WorkflowRefData", func() {
		It("should map empty strings correctly", func() {
			// Given: Parsed data with empty strings in WorkflowRefData
			parsedData := &reconstructionpkg.ParsedAuditData{
				EventType:     "workflowexecution.selection.completed",
				CorrelationID: "test-corr-007",
				SelectedWorkflowRef: &reconstructionpkg.WorkflowRefData{
					WorkflowID:      "", // Empty
					Version:         "", // Empty
					ContainerImage:  "", // Empty
					ContainerDigest: "", // Empty
				},
			}

			// When: Mapper processes the data
			fields, err := reconstructionpkg.MapToRRFields(parsedData)

			// Then: No error, all fields mapped as empty strings
			Expect(err).ToNot(HaveOccurred())
			Expect(fields).ToNot(BeNil())
			Expect(fields.Status.SelectedWorkflowRef).ToNot(BeNil())
			Expect(fields.Status.SelectedWorkflowRef.WorkflowID).To(BeEmpty())
			Expect(fields.Status.SelectedWorkflowRef.Version).To(BeEmpty())
			Expect(fields.Status.SelectedWorkflowRef.ContainerImage).To(BeEmpty())
		})
	})

	Context("MAPPER-GAP56-EDGE-08: Empty strings in ExecutionRefData", func() {
		It("should map empty strings correctly", func() {
			// Given: Parsed data with empty strings in ExecutionRefData
			parsedData := &reconstructionpkg.ParsedAuditData{
				EventType:     "workflowexecution.execution.started",
				CorrelationID: "test-corr-008",
				ExecutionRef: &reconstructionpkg.ExecutionRefData{
					APIVersion: "", // Empty
					Kind:       "", // Empty
					Name:       "", // Empty
					Namespace:  "", // Empty
				},
			}

			// When: Mapper processes the data
			fields, err := reconstructionpkg.MapToRRFields(parsedData)

			// Then: No error, all fields mapped as empty strings
			Expect(err).ToNot(HaveOccurred())
			Expect(fields).ToNot(BeNil())
			Expect(fields.Status.ExecutionRef).ToNot(BeNil())
			Expect(fields.Status.ExecutionRef.APIVersion).To(BeEmpty())
			Expect(fields.Status.ExecutionRef.Kind).To(BeEmpty())
			Expect(fields.Status.ExecutionRef.Name).To(BeEmpty())
			Expect(fields.Status.ExecutionRef.Namespace).To(BeEmpty())
		})
	})
})
