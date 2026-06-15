package ka_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
)

var _ = Describe("Event Priority Classification — #1433 BR-AF-STREAM-001", func() {

	Describe("UT-AF-1433-001: structural events classified correctly", func() {
		It("should classify complete as structural", func() {
			Expect(ka.IsStructuralEvent(ka.EventTypeComplete)).To(BeTrue())
		})

		It("should classify error as structural", func() {
			Expect(ka.IsStructuralEvent(ka.EventTypeError)).To(BeTrue())
		})

		It("should classify cancelled as structural", func() {
			Expect(ka.IsStructuralEvent(ka.EventTypeCancelled)).To(BeTrue())
		})

		It("should classify alignment_verdict as structural", func() {
			Expect(ka.IsStructuralEvent(ka.EventTypeAlignmentVerdict)).To(BeTrue())
		})
	})

	Describe("UT-AF-1433-002: streaming events classified correctly", func() {
		It("should classify token_delta as streaming", func() {
			Expect(ka.IsStructuralEvent(ka.EventTypeTokenDelta)).To(BeFalse())
		})

		It("should classify reasoning_delta as streaming", func() {
			Expect(ka.IsStructuralEvent(ka.EventTypeReasoningDelta)).To(BeFalse())
		})

		It("should classify tool_call_start as streaming", func() {
			Expect(ka.IsStructuralEvent(ka.EventTypeToolCallStart)).To(BeFalse())
		})

		It("should classify tool_result as streaming", func() {
			Expect(ka.IsStructuralEvent(ka.EventTypeToolResult)).To(BeFalse())
		})

		It("should classify tool_call as streaming", func() {
			Expect(ka.IsStructuralEvent(ka.EventTypeToolCall)).To(BeFalse())
		})

		It("should classify unknown types as streaming (safe default)", func() {
			Expect(ka.IsStructuralEvent("unknown_type")).To(BeFalse())
		})
	})

	Describe("UT-AF-1433-003: PrioritySend delivers structural events under backpressure", func() {
		It("should deliver structural event even when channel is full", func() {
			ch := make(chan ka.InvestigationEvent, 1)
			done := make(chan struct{})

			// Fill channel with a streaming event
			ch <- ka.InvestigationEvent{Type: ka.EventTypeTokenDelta}

			// Drain in background after short delay to simulate slow consumer
			go func() {
				defer close(done)
				<-ch // drain the blocking event
				<-ch // drain the structural event we're about to send
			}()

			evt := ka.InvestigationEvent{Type: ka.EventTypeComplete}
			dropped := ka.PrioritySend(ch, done, evt)
			Expect(dropped).To(BeFalse())

			Eventually(done).Should(BeClosed())
		})
	})

	Describe("UT-AF-1433-004: PrioritySend drops streaming events under backpressure", func() {
		It("should drop streaming event when channel is full", func() {
			ch := make(chan ka.InvestigationEvent, 1)
			done := make(chan struct{})

			// Fill channel
			ch <- ka.InvestigationEvent{Type: ka.EventTypeTokenDelta}

			evt := ka.InvestigationEvent{Type: ka.EventTypeTokenDelta}
			dropped := ka.PrioritySend(ch, done, evt)
			Expect(dropped).To(BeTrue())
		})
	})

	Describe("UT-AF-1433-005: PrioritySend respects doneCh for structural events", func() {
		It("should not block forever when doneCh is closed", func() {
			ch := make(chan ka.InvestigationEvent, 1)
			done := make(chan struct{})

			// Fill channel
			ch <- ka.InvestigationEvent{Type: ka.EventTypeTokenDelta}
			// Close done to signal shutdown
			close(done)

			evt := ka.InvestigationEvent{Type: ka.EventTypeComplete}
			dropped := ka.PrioritySend(ch, done, evt)
			Expect(dropped).To(BeTrue())
		})
	})

	Describe("UT-AF-1433-006: DefaultEventChannelBuffer size", func() {
		It("should be 128", func() {
			Expect(ka.DefaultEventChannelBuffer).To(Equal(128))
		})
	})
})
