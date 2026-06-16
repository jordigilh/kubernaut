package ka

import "time"

// DefaultEventChannelBuffer is the buffer size for the MCP investigation event
// channel. Sized to reduce drop frequency under normal LLM output rates while
// the priority send mechanism prevents structural event loss.
// DD-AF-008: Increased from 64 to 128.
const DefaultEventChannelBuffer = 128

// structuralSendTimeout is the maximum time PrioritySend will block waiting to
// deliver a structural event. If the channel remains full after this duration,
// the event is considered dropped (implies consumer is dead).
const structuralSendTimeout = 5 * time.Second

// IsStructuralEvent returns true for event types that represent lifecycle state
// transitions and MUST NOT be silently dropped. These events control Console
// state machines (phase banners, timers) and FedRAMP audit trail completeness.
//
// DD-AF-008: Structural events use blocking send with timeout.
// BR-AF-STREAM-001.1: complete, error, cancelled, alignment_verdict.
// #1438: session_ended is terminal and must reach Console for phase transition.
func IsStructuralEvent(eventType string) bool {
	switch eventType {
	case EventTypeComplete, EventTypeError, EventTypeCancelled, EventTypeAlignmentVerdict, EventTypeSessionEnded:
		return true
	default:
		return false
	}
}

// PrioritySend sends an event to the channel using priority-based delivery:
//   - Structural events (complete, error, cancelled, alignment_verdict,
//     session_ended) use a blocking send with a bounded timeout — they are
//     never silently dropped.
//   - Streaming events (token_delta, reasoning_delta, etc.) use a non-blocking
//     send — they are dropped when the channel is full.
//
// Returns true if the event was dropped, false if delivered.
// The doneCh parameter allows early exit on session shutdown.
//
// DD-AF-008 Alternative 3: Priority Send at Producer.
func PrioritySend(ch chan<- InvestigationEvent, doneCh <-chan struct{}, evt InvestigationEvent) (dropped bool) {
	if IsStructuralEvent(evt.Type) {
		t := time.NewTimer(structuralSendTimeout)
		defer t.Stop()
		select {
		case ch <- evt:
			return false
		case <-doneCh:
			return true
		case <-t.C:
			return true
		}
	}

	select {
	case ch <- evt:
		return false
	case <-doneCh:
		return true
	default:
		return true
	}
}
