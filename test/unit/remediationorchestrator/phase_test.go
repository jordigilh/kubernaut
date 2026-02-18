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
	. "github.com/onsi/gomega"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
)

var _ = Describe("Phase Types (BR-ORCH-025, BR-ORCH-026)", func() {

	// ========================================
	// IsTerminal - Terminal State Detection
	// BR-ORCH-026: Terminal states must be correctly identified
	// ========================================
	Describe("IsTerminal (BR-ORCH-026)", func() {

		DescribeTable("terminal state detection - identifies phases where remediation has ended",
			func(p phase.Phase, expectTerminal bool, reason string) {
				// When: We check if the phase is terminal
				result := phase.IsTerminal(p)

				// Then: Result should match expected terminal status
				Expect(result).To(Equal(expectTerminal), reason)
			},
			// Terminal states - remediation has ended
			Entry("Completed is terminal (successful remediation)",
				phase.Completed, true, "Completed indicates successful end of remediation"),
			Entry("Failed is terminal (remediation error)",
				phase.Failed, true, "Failed indicates remediation cannot proceed"),
			Entry("TimedOut is terminal (BR-ORCH-027, BR-ORCH-028)",
				phase.TimedOut, true, "TimedOut indicates phase/global timeout exceeded"),
			Entry("Skipped is terminal (BR-ORCH-032 resource lock)",
				phase.Skipped, true, "Skipped indicates duplicate remediation prevented"),
			Entry("Cancelled is terminal (manual cancellation)",
				phase.Cancelled, true, "Cancelled indicates remediation was manually cancelled"),

			// Non-terminal states - remediation in progress
			Entry("Pending is NOT terminal (initial state)",
				phase.Pending, false, "Pending indicates remediation not started"),
			Entry("Processing is NOT terminal (SignalProcessing active)",
				phase.Processing, false, "Processing indicates SignalProcessing CRD in progress"),
			Entry("Analyzing is NOT terminal (AIAnalysis active)",
				phase.Analyzing, false, "Analyzing indicates AIAnalysis CRD in progress"),
			Entry("AwaitingApproval is NOT terminal (BR-ORCH-001)",
				phase.AwaitingApproval, false, "AwaitingApproval indicates human approval required"),
			Entry("Executing is NOT terminal (WorkflowExecution active)",
				phase.Executing, false, "Executing indicates WorkflowExecution CRD in progress"),
			// BR-ORCH-042.2: Blocked is non-terminal to prevent Gateway from creating new RRs
			Entry("Blocked is NOT terminal (BR-ORCH-042.2 - AC-042-2-1)",
				phase.Blocked, false, "Blocked holds RR during cooldown, Gateway sees as active"),

			// Edge cases
			Entry("Unknown phase is NOT terminal (defensive)",
				phase.Phase("Unknown"), false, "Unknown phases should not be treated as terminal"),
			Entry("Empty phase is NOT terminal (defensive)",
				phase.Phase(""), false, "Empty phases should not be treated as terminal"),
		)
	})

	// ========================================
	// CanTransition - Phase State Machine
	// BR-ORCH-025: Phase state transitions must follow defined rules
	// ========================================
	Describe("CanTransition (BR-ORCH-025)", func() {

		Context("valid forward transitions - happy path", func() {
			DescribeTable("allowed state machine transitions",
				func(from, to phase.Phase, reason string) {
					result := phase.CanTransition(from, to)
					Expect(result).To(BeTrue(), reason)
				},
				Entry("Pending → Processing (start remediation)",
					phase.Pending, phase.Processing, "First transition in state machine"),
				Entry("Processing → Analyzing (SignalProcessing complete)",
					phase.Processing, phase.Analyzing, "Move to AI analysis after enrichment"),
				Entry("Analyzing → AwaitingApproval (approval required - BR-ORCH-001)",
					phase.Analyzing, phase.AwaitingApproval, "Human approval workflow"),
				Entry("Analyzing → Executing (auto-approve path)",
					phase.Analyzing, phase.Executing, "Skip approval for low-risk operations"),
				Entry("AwaitingApproval → Executing (approval granted)",
					phase.AwaitingApproval, phase.Executing, "Proceed after human approval"),
				Entry("Executing → Completed (successful workflow)",
					phase.Executing, phase.Completed, "Workflow completed successfully"),
			)
		})

		Context("valid error transitions - failure handling", func() {
			DescribeTable("allowed transitions to error states",
				func(from, to phase.Phase, reason string) {
					result := phase.CanTransition(from, to)
					Expect(result).To(BeTrue(), reason)
				},
				// Failed transitions
				Entry("Processing → Failed (SignalProcessing error)",
					phase.Processing, phase.Failed, "Error during signal enrichment"),
				Entry("Analyzing → Failed (AIAnalysis error)",
					phase.Analyzing, phase.Failed, "Error during AI analysis"),
				Entry("AwaitingApproval → Failed (approval rejected/error)",
					phase.AwaitingApproval, phase.Failed, "Approval denied or system error"),
				Entry("Executing → Failed (WorkflowExecution error)",
					phase.Executing, phase.Failed, "Error during workflow execution"),

				// TimedOut transitions (BR-ORCH-027, BR-ORCH-028)
				Entry("Processing → TimedOut (per-phase timeout)",
					phase.Processing, phase.TimedOut, "SignalProcessing exceeded timeout"),
				Entry("Analyzing → TimedOut (per-phase timeout)",
					phase.Analyzing, phase.TimedOut, "AIAnalysis exceeded timeout"),
				Entry("AwaitingApproval → TimedOut (approval timeout)",
					phase.AwaitingApproval, phase.TimedOut, "Human approval timed out"),
				Entry("Executing → TimedOut (workflow timeout)",
					phase.Executing, phase.TimedOut, "WorkflowExecution exceeded timeout"),

				// Skipped transition (BR-ORCH-032)
				Entry("Executing → Skipped (resource lock deduplication)",
					phase.Executing, phase.Skipped, "Another workflow already executing on same target"),

				// BR-ORCH-042: Blocked transitions for consecutive failure handling
				Entry("Failed → Blocked (consecutive failures threshold met - BR-ORCH-042)",
					phase.Failed, phase.Blocked, "Block signal after 3+ consecutive failures"),
				Entry("Blocked → Failed (cooldown expired - BR-ORCH-042.3)",
					phase.Blocked, phase.Failed, "Return to terminal Failed after cooldown"),
			)
		})

		Context("invalid transitions - must be rejected", func() {
			DescribeTable("blocked state machine transitions",
				func(from, to phase.Phase, reason string) {
					result := phase.CanTransition(from, to)
					Expect(result).To(BeFalse(), reason)
				},
				// Skipping phases
				Entry("Pending → Completed (skip all phases)",
					phase.Pending, phase.Completed, "Cannot skip intermediate phases"),
				Entry("Pending → Analyzing (skip Processing)",
					phase.Pending, phase.Analyzing, "Must go through Processing first"),

				// Terminal states cannot transition (BR-ORCH-026)
				Entry("Completed → Pending (reverse from terminal)",
					phase.Completed, phase.Pending, "Terminal states cannot transition"),
				Entry("Failed → Processing (recover from failure)",
					phase.Failed, phase.Processing, "Terminal states cannot transition"),
				Entry("TimedOut → Analyzing (retry after timeout)",
					phase.TimedOut, phase.Analyzing, "Terminal states cannot transition"),
				Entry("Skipped → Executing (retry after skip)",
					phase.Skipped, phase.Executing, "Terminal states cannot transition"),

				// Same-phase transition
				Entry("Pending → Pending (no-op transition)",
					phase.Pending, phase.Pending, "Cannot transition to same phase"),

				// Unknown source phase
				Entry("Unknown → Processing (invalid source)",
					phase.Phase("Unknown"), phase.Processing, "Unknown phases have no valid transitions"),
			)
		})
	})

	// ========================================
	// Validate - Phase Value Validation
	// ========================================
	Describe("Validate", func() {

		DescribeTable("phase value validation",
			func(p phase.Phase, expectError bool, description string) {
				err := phase.Validate(p)
				if expectError {
					Expect(err).To(HaveOccurred(), description)
					Expect(err.Error()).To(ContainSubstring("invalid phase"))
				} else {
					Expect(err).ToNot(HaveOccurred(), description)
				}
			},
			// Valid phases
			Entry("Pending is valid", phase.Pending, false, "Valid initial phase"),
			Entry("Processing is valid", phase.Processing, false, "Valid active phase"),
			Entry("Analyzing is valid", phase.Analyzing, false, "Valid active phase"),
			Entry("AwaitingApproval is valid", phase.AwaitingApproval, false, "Valid active phase"),
			Entry("Executing is valid", phase.Executing, false, "Valid active phase"),
			Entry("Blocked is valid (BR-ORCH-042.2)", phase.Blocked, false, "Valid non-terminal blocking phase"),
			Entry("Completed is valid", phase.Completed, false, "Valid terminal phase"),
			Entry("Failed is valid", phase.Failed, false, "Valid terminal phase"),
			Entry("TimedOut is valid", phase.TimedOut, false, "Valid terminal phase"),
			Entry("Skipped is valid", phase.Skipped, false, "Valid terminal phase"),

			// Invalid phases
			Entry("InvalidPhase should be rejected", phase.Phase("InvalidPhase"), true, "Unknown phase value"),
			Entry("Empty string should be rejected", phase.Phase(""), true, "Empty phase value"),
		)
	})

	// ========================================
	// PhaseManager - Stateful Phase Management
	// ========================================
	Describe("PhaseManager", func() {
		var manager *phase.Manager

		BeforeEach(func() {
			manager = phase.NewManager()
		})

		Context("when determining current phase", func() {
			It("should return Pending for empty OverallPhase (initial state)", func() {
				// Given: A RemediationRequest with no phase set
				rr := &remediationv1.RemediationRequest{}

				// When: We get the current phase
				result := manager.CurrentPhase(rr)

				// Then: It should default to Pending
				Expect(result).To(Equal(phase.Pending))
			})

			It("should return actual phase from status", func() {
				// Given: A RemediationRequest with phase set
				rr := &remediationv1.RemediationRequest{}
				rr.Status.OverallPhase = "Processing"

				// When: We get the current phase
				result := manager.CurrentPhase(rr)

				// Then: It should return the set phase
				Expect(result).To(Equal(phase.Processing))
			})
		})

		Context("when transitioning phases", func() {
			It("should update status on valid transition", func() {
				// Given: A RemediationRequest in Pending phase
				rr := &remediationv1.RemediationRequest{}
				rr.Status.OverallPhase = "Pending"

				// When: We transition to Processing
				err := manager.TransitionTo(rr, phase.Processing)

				// Then: Transition should succeed and status should be updated
				Expect(err).ToNot(HaveOccurred())
				Expect(rr.Status.OverallPhase).To(Equal(remediationv1.PhaseProcessing))
			})

			It("should return error on invalid transition", func() {
				// Given: A RemediationRequest in Pending phase
				rr := &remediationv1.RemediationRequest{}
				rr.Status.OverallPhase = "Pending"

				// When: We attempt invalid transition to Completed
				err := manager.TransitionTo(rr, phase.Completed)

				// Then: Transition should fail with descriptive error
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid phase transition"))
			})
		})

		// ========================================
		// Invalid State Handling
		// Tests defensive programming for corrupted/invalid phase values
		// Business Value: State machine integrity and corruption resilience
		// ========================================
		Context("when handling invalid or corrupted phase values", func() {
			It("should handle unknown phase value gracefully", func() {
				// Scenario: RR.Status.OverallPhase contains unknown value (corruption or version skew)
				// Business Value: Resilient to status corruption, provides clear error
				// Confidence: 95% - Defensive programming essential

				// Given: RemediationRequest with unknown phase
				rr := &remediationv1.RemediationRequest{}
				rr.Status.OverallPhase = remediationv1.RemediationPhase("UnknownCorruptedPhase")

				// When: We attempt to get current phase
				current := manager.CurrentPhase(rr)

				// Then: Should return the corrupted value (not crash)
				Expect(current).To(Equal(phase.Phase("UnknownCorruptedPhase")),
					"Unknown phases should be returned as-is for debugging")

				// When: We validate the corrupted phase
				err := phase.Validate(current)

				// Then: Validation should fail with clear error
				Expect(err).To(HaveOccurred(), "Unknown phases must fail validation")
				Expect(err.Error()).To(ContainSubstring("invalid phase"),
					"Error message must be clear for operators")
			})

			It("should prevent phase regression (e.g., Executing → Pending)", func() {
				// Scenario: Attempt backward transition in state machine
				// Business Value: Enforces state machine integrity, prevents logical errors
				// Confidence: 90% - Prevents accidental state corruption

				// Given: RemediationRequest in Executing phase (late stage)
				rr := &remediationv1.RemediationRequest{}
				rr.Status.OverallPhase = remediationv1.PhaseExecuting

				// When: We attempt to regress to Pending (invalid backward transition)
				err := manager.TransitionTo(rr, phase.Pending)

				// Then: Transition must be rejected
				Expect(err).To(HaveOccurred(), "Backward transitions must be prevented")
				Expect(err.Error()).To(ContainSubstring("invalid phase transition"),
					"Error must clearly indicate invalid transition")

				// Verify: Phase remains unchanged
				Expect(rr.Status.OverallPhase).To(Equal(remediationv1.PhaseExecuting),
					"Phase must not change on invalid transition")
			})

			It("should validate that CanTransition rejects phase regression", func() {
				// Scenario: Check state machine rules prevent all backward transitions
				// Business Value: Validates state machine integrity rules
				// Confidence: 95% - Core state machine validation

				// When: We check various backward transitions
				executingToPending := phase.CanTransition(phase.Executing, phase.Pending)
				analyzingToProcessing := phase.CanTransition(phase.Analyzing, phase.Processing)
				executingToAnalyzing := phase.CanTransition(phase.Executing, phase.Analyzing)

				// Then: All backward transitions must be rejected
				Expect(executingToPending).To(BeFalse(),
					"Executing → Pending is invalid regression")
				Expect(analyzingToProcessing).To(BeFalse(),
					"Analyzing → Processing is invalid regression")
				Expect(executingToAnalyzing).To(BeFalse(),
					"Executing → Analyzing is invalid regression")
			})
		})
	})
})
