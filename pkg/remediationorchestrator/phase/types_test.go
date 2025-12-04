package phase_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
)

func TestPhase(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Phase Suite")
}

// BR-ORCH-025: Core Orchestration Phases
// BR-ORCH-026: Approval Orchestration
// BR-ORCH-027, BR-ORCH-028: Timeout Management
// BR-ORCH-032: Resource Lock Deduplication (Skipped Phase)
var _ = Describe("BR-ORCH-025: Phase State Machine Validation", func() {

	// IsTerminal validates business rule: terminal phases cannot progress further
	// Reference: SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md - DescribeTable pattern
	Describe("IsTerminal", func() {
		DescribeTable("should correctly identify terminal vs non-terminal phases",
			func(p phase.Phase, expected bool, description string) {
				Expect(phase.IsTerminal(p)).To(Equal(expected), description)
			},
			// Non-terminal phases (orchestration continues)
			Entry("Pending is not terminal", phase.Pending, false,
				"Pending allows transition to Processing"),
			Entry("Processing is not terminal", phase.Processing, false,
				"Processing allows transition to Analyzing"),
			Entry("Analyzing is not terminal", phase.Analyzing, false,
				"Analyzing allows transition to AwaitingApproval or Executing"),
			Entry("AwaitingApproval is not terminal", phase.AwaitingApproval, false,
				"AwaitingApproval allows transition to Executing (BR-ORCH-026)"),
			Entry("Executing is not terminal", phase.Executing, false,
				"Executing allows transition to Completed, Failed, TimedOut, or Skipped"),

			// Terminal phases (orchestration ends)
			Entry("Completed is terminal", phase.Completed, true,
				"Completed is a successful terminal state"),
			Entry("Failed is terminal", phase.Failed, true,
				"Failed is an error terminal state"),
			Entry("TimedOut is terminal (BR-ORCH-027)", phase.TimedOut, true,
				"TimedOut is a timeout terminal state"),
			Entry("Skipped is terminal (BR-ORCH-032)", phase.Skipped, true,
				"Skipped is a resource-lock deduplication terminal state"),
		)
	})

	// CanTransition validates the phase state machine transitions
	// Reference: SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md lines 1246-1306
	Describe("CanTransition", func() {
		DescribeTable("should validate phase transition rules",
			func(from, to phase.Phase, allowed bool, description string) {
				Expect(phase.CanTransition(from, to)).To(Equal(allowed), description)
			},
			// From Pending (BR-ORCH-025)
			Entry("Pending → Processing: allowed",
				phase.Pending, phase.Processing, true,
				"Initial phase can only progress to Processing"),
			Entry("Pending → Analyzing: NOT allowed",
				phase.Pending, phase.Analyzing, false,
				"Cannot skip Processing phase"),
			Entry("Pending → Completed: NOT allowed",
				phase.Pending, phase.Completed, false,
				"Cannot jump to terminal state"),

			// From Processing (BR-ORCH-025)
			Entry("Processing → Analyzing: allowed",
				phase.Processing, phase.Analyzing, true,
				"After SignalProcessing completes, progress to AI analysis"),
			Entry("Processing → Failed: allowed",
				phase.Processing, phase.Failed, true,
				"SignalProcessing failure triggers Failed state"),
			Entry("Processing → TimedOut: allowed (BR-ORCH-028)",
				phase.Processing, phase.TimedOut, true,
				"Per-phase timeout triggers TimedOut state"),
			Entry("Processing → Completed: NOT allowed",
				phase.Processing, phase.Completed, false,
				"Cannot skip Analyzing and Executing phases"),

			// From Analyzing (BR-ORCH-025, BR-ORCH-026)
			Entry("Analyzing → AwaitingApproval: allowed (BR-ORCH-026)",
				phase.Analyzing, phase.AwaitingApproval, true,
				"AI analysis may require human approval"),
			Entry("Analyzing → Executing: allowed (auto-approved)",
				phase.Analyzing, phase.Executing, true,
				"AI analysis may auto-approve low-risk workflows"),
			Entry("Analyzing → Failed: allowed",
				phase.Analyzing, phase.Failed, true,
				"AI analysis failure triggers Failed state"),
			Entry("Analyzing → TimedOut: allowed (BR-ORCH-028)",
				phase.Analyzing, phase.TimedOut, true,
				"Per-phase timeout triggers TimedOut state"),

			// From AwaitingApproval (BR-ORCH-001, BR-ORCH-026)
			Entry("AwaitingApproval → Executing: allowed",
				phase.AwaitingApproval, phase.Executing, true,
				"Human approval granted triggers execution"),
			Entry("AwaitingApproval → Failed: allowed (rejected)",
				phase.AwaitingApproval, phase.Failed, true,
				"Human rejection triggers Failed state"),
			Entry("AwaitingApproval → TimedOut: allowed (BR-ORCH-026)",
				phase.AwaitingApproval, phase.TimedOut, true,
				"Approval timeout triggers TimedOut state"),

			// From Executing (BR-ORCH-025, BR-ORCH-032)
			Entry("Executing → Completed: allowed",
				phase.Executing, phase.Completed, true,
				"WorkflowExecution success triggers Completed state"),
			Entry("Executing → Failed: allowed",
				phase.Executing, phase.Failed, true,
				"WorkflowExecution failure triggers Failed state"),
			Entry("Executing → TimedOut: allowed (BR-ORCH-028)",
				phase.Executing, phase.TimedOut, true,
				"Per-phase timeout triggers TimedOut state"),
			Entry("Executing → Skipped: allowed (BR-ORCH-032)",
				phase.Executing, phase.Skipped, true,
				"WorkflowExecution resource lock triggers Skipped state"),

			// From terminal phases (no transitions allowed)
			Entry("Completed → Failed: NOT allowed",
				phase.Completed, phase.Failed, false,
				"Terminal state cannot transition"),
			Entry("Completed → Processing: NOT allowed",
				phase.Completed, phase.Processing, false,
				"Terminal state cannot transition"),
			Entry("Failed → Completed: NOT allowed",
				phase.Failed, phase.Completed, false,
				"Terminal state cannot transition"),
			Entry("Failed → Processing: NOT allowed",
				phase.Failed, phase.Processing, false,
				"Terminal state cannot transition"),
			Entry("TimedOut → Processing: NOT allowed",
				phase.TimedOut, phase.Processing, false,
				"Terminal state cannot transition"),
			Entry("Skipped → Processing: NOT allowed",
				phase.Skipped, phase.Processing, false,
				"Terminal state cannot transition"),
		)
	})

	// Validate validates that phase values are from the allowed set
	Describe("Validate", func() {
		DescribeTable("should validate phase values",
			func(p phase.Phase, shouldSucceed bool) {
				err := phase.Validate(p)
				if shouldSucceed {
					Expect(err).ToNot(HaveOccurred())
				} else {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("invalid phase"))
				}
			},
			// Valid phases
			Entry("Pending is valid", phase.Pending, true),
			Entry("Processing is valid", phase.Processing, true),
			Entry("Analyzing is valid", phase.Analyzing, true),
			Entry("AwaitingApproval is valid", phase.AwaitingApproval, true),
			Entry("Executing is valid", phase.Executing, true),
			Entry("Completed is valid", phase.Completed, true),
			Entry("Failed is valid", phase.Failed, true),
			Entry("TimedOut is valid", phase.TimedOut, true),
			Entry("Skipped is valid", phase.Skipped, true),

			// Invalid phases
			Entry("InvalidPhase is invalid", phase.Phase("InvalidPhase"), false),
			Entry("Empty string is invalid", phase.Phase(""), false),
			Entry("Unknown is invalid", phase.Phase("Unknown"), false),
		)
	})
})
