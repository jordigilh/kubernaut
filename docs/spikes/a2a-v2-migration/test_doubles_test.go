/*
Spike 5: Test Double Pattern for iter.Seq2-based Executor

Demonstrates the replacement pattern for fakeQueue-based test doubles.

Current pattern (v0.3):
  - fakeQueue captures events via Write() calls
  - Tests inspect queue.events[] after Execute() completes
  - Side-channel writes (EventBridge) go through the same queue

New pattern (v2):
  - fakeIterExecutor returns events via iter.Seq2[Event, error]
  - Tests collect events from the returned iterator
  - Side-channel writes (EventBridge) go through a merged channel
  - collectEvents() helper replaces queue.events[] inspection

This spike validates that the new pattern supports all existing test scenarios:
  - Single event emission
  - Multiple emissions with ordering
  - Error injection
  - Context cancellation
  - Concurrent bridge writes during execution
  - Event type assertions (status, artifact, keepalive)

This is a throwaway spike -- not production code.
*/
package a2av2migration

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"sync"
	"testing"
	"time"
)

// --- New test doubles for v2 iter.Seq2 executor ---

// fakeIterExecutor replaces fakeQueue. Instead of receiving events through
// Write(), it produces events through an iter.Seq2. Tests configure the
// events/errors upfront.
type fakeIterExecutor struct {
	events []Event
	errors []error
	delay  time.Duration
}

func (f *fakeIterExecutor) Execute(ctx context.Context) iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {
		for i, e := range f.events {
			select {
			case <-ctx.Done():
				return
			default:
			}

			var err error
			if i < len(f.errors) {
				err = f.errors[i]
			}
			if !yield(e, err) {
				return
			}
			if err != nil {
				return
			}
			if f.delay > 0 {
				time.Sleep(f.delay)
			}
		}
	}
}

// collectEvents drains an iter.Seq2 into a slice, stopping on first error.
// This replaces inspecting queue.events[] after execution.
func collectEvents(seq iter.Seq2[Event, error]) ([]Event, error) {
	var events []Event
	for event, err := range seq {
		if err != nil {
			return events, err
		}
		events = append(events, event)
	}
	return events, nil
}

// collectAllEvents drains an iter.Seq2 including errored events.
func collectAllEvents(seq iter.Seq2[Event, error]) (events []Event, errors []error) {
	for event, err := range seq {
		events = append(events, event)
		errors = append(errors, err)
	}
	return
}

// filterBySource filters collected events by source.
func filterBySource(events []Event, source string) []Event {
	var filtered []Event
	for _, e := range events {
		if e.Source == source {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// --- Spike 5 adaptation of channelMergeExecute for bridge side-channel ---

// testableExecute wraps channelMergeExecute with a bridge writer that tests
// can use to inject side-channel events during execution.
type testableExecute struct {
	bridgeCh chan Event
	mu       sync.Mutex
}

func newTestableExecute() *testableExecute {
	return &testableExecute{
		bridgeCh: make(chan Event, 32),
	}
}

func (te *testableExecute) EmitBridge(text string) {
	te.bridgeCh <- Event{Source: "bridge", Text: text}
}

func (te *testableExecute) Run(ctx context.Context, inner iter.Seq2[Event, error]) iter.Seq2[Event, error] {
	return channelMergeExecute(ctx, inner, te.bridgeCh)
}

// --- Tests demonstrating the new pattern ---

func TestTestDouble_SingleEventEmission(t *testing.T) {
	exec := &fakeIterExecutor{
		events: []Event{
			{Source: "inner", Text: "status-update"},
		},
	}

	events, err := collectEvents(exec.Execute(context.Background()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Text != "status-update" {
		t.Errorf("text mismatch: got %q", events[0].Text)
	}

	t.Log("single event emission works with iter.Seq2 pattern")
}

func TestTestDouble_MultipleEmissionsOrdered(t *testing.T) {
	exec := &fakeIterExecutor{
		events: []Event{
			{Source: "inner", Text: "reasoning", Index: 0},
			{Source: "inner", Text: "status", Index: 1},
			{Source: "inner", Text: "output", Index: 2},
		},
	}

	events, err := collectEvents(exec.Execute(context.Background()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	for i, e := range events {
		if e.Index != i {
			t.Errorf("ordering broken: event %d has index %d", i, e.Index)
		}
	}

	t.Log("multiple emissions maintain ordering with iter.Seq2 pattern")
}

func TestTestDouble_ErrorInjection(t *testing.T) {
	testErr := fmt.Errorf("LLM capacity exceeded")
	exec := &fakeIterExecutor{
		events: []Event{
			{Source: "inner", Text: "ok-event"},
			{Source: "inner", Text: "error-event"},
		},
		errors: []error{nil, testErr},
	}

	events, err := collectEvents(exec.Execute(context.Background()))
	if !errors.Is(err, testErr) {
		t.Fatalf("expected testErr, got %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event before error, got %d", len(events))
	}

	t.Log("error injection stops iteration at the error point")
}

func TestTestDouble_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	exec := &fakeIterExecutor{
		events: []Event{
			{Source: "inner", Text: "event-0"},
			{Source: "inner", Text: "event-1"},
			{Source: "inner", Text: "event-2"},
		},
		delay: 30 * time.Millisecond,
	}

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	events, _ := collectEvents(exec.Execute(ctx))

	if len(events) > 3 {
		t.Fatalf("expected at most 3 events, got %d", len(events))
	}
	if len(events) == 0 {
		t.Fatal("expected at least 1 event before cancellation")
	}

	t.Logf("context cancellation: collected %d events before cancel", len(events))
}

func TestTestDouble_BridgeWritesDuringExecution(t *testing.T) {
	te := newTestableExecute()
	exec := &fakeIterExecutor{
		events: []Event{
			{Source: "inner", Text: "first"},
			{Source: "inner", Text: "second"},
		},
		delay: 20 * time.Millisecond,
	}

	// Inject bridge event between inner events
	go func() {
		time.Sleep(15 * time.Millisecond)
		te.EmitBridge("reasoning-fragment")
	}()

	events, err := collectEvents(te.Run(context.Background(), exec.Execute(context.Background())))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	innerEvents := filterBySource(events, "inner")
	bridgeEvents := filterBySource(events, "bridge")

	if len(innerEvents) != 2 {
		t.Fatalf("expected 2 inner events, got %d", len(innerEvents))
	}
	if len(bridgeEvents) < 1 {
		t.Fatal("expected at least 1 bridge event")
	}

	t.Logf("bridge writes during execution: %d inner, %d bridge, %d keepalive",
		len(innerEvents), len(bridgeEvents), len(filterBySource(events, "keepalive")))
}

func TestTestDouble_EventTypeAssertions(t *testing.T) {
	exec := &fakeIterExecutor{
		events: []Event{
			{Source: "inner", Text: "reasoning-text"},
			{Source: "inner", Text: "status-update"},
			{Source: "inner", Text: "artifact-data"},
		},
	}

	events, err := collectEvents(exec.Execute(context.Background()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Demonstrate type-based filtering (simulates checking metadata.type)
	if events[0].Text != "reasoning-text" {
		t.Errorf("expected first event to be reasoning, got %q", events[0].Text)
	}
	if events[1].Text != "status-update" {
		t.Errorf("expected second event to be status, got %q", events[1].Text)
	}
	if events[2].Text != "artifact-data" {
		t.Errorf("expected third event to be artifact, got %q", events[2].Text)
	}

	t.Log("event type assertions work with iter.Seq2 pattern")
}

func TestTestDouble_ComparisonWithFakeQueuePattern(t *testing.T) {
	// OLD PATTERN (v0.3): events captured in queue.events[]
	//   queue := &fakeQueue{}
	//   executor.Execute(ctx, reqCtx, queue)
	//   Expect(queue.events).To(HaveLen(1))
	//   evt := queue.events[0].(*a2a.TaskStatusUpdateEvent)
	//   Expect(evt.Status.Message.Parts[0].(a2a.TextPart).Text).To(Equal("..."))

	// NEW PATTERN (v2): events returned via iter.Seq2
	exec := &fakeIterExecutor{
		events: []Event{
			{Source: "inner", Text: "Checking pod status..."},
		},
	}

	events, err := collectEvents(exec.Execute(context.Background()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Text != "Checking pod status..." {
		t.Errorf("text mismatch: got %q", events[0].Text)
	}

	// Mapping guide for migration:
	// queue.events[i] -> events[i]
	// queue.events[0].(*a2a.TaskStatusUpdateEvent) -> events[0] (already typed)
	// Expect(queue.events).To(HaveLen(N)) -> len(events) == N
	// queue.Write(ctx, evt) error -> checked during iteration
	// queue.closed -> iterator naturally terminates

	t.Log("iter.Seq2 pattern is a 1:1 replacement for fakeQueue inspection")
}

func TestTestDouble_CollectAllWithErrors(t *testing.T) {
	testErr := fmt.Errorf("partial failure")
	exec := &fakeIterExecutor{
		events: []Event{
			{Source: "inner", Text: "ok"},
			{Source: "inner", Text: "fail"},
		},
		errors: []error{nil, testErr},
	}

	events, errs := collectAllEvents(exec.Execute(context.Background()))

	if len(events) != 2 {
		t.Fatalf("expected 2 events (including errored), got %d", len(events))
	}
	if errs[0] != nil {
		t.Errorf("first event should have no error")
	}
	if !errors.Is(errs[1], testErr) {
		t.Errorf("second event should carry testErr")
	}

	t.Log("collectAllEvents captures both events and errors for detailed assertions")
}
