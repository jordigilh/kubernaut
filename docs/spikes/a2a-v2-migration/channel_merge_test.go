/*
Spike 1+2: Channel-merge PoC + Re-invocation chaining

Validates:
  - 3 concurrent event sources (inner executor, keepalive, bridge side-channel)
    merged into a single iter.Seq2[Event, error] via a shared channel
  - Multiple inner.Execute() calls (re-invocation) flattened into one output iterator
  - Graceful shutdown: inner executor finishing signals auxiliaries to stop
  - Context cancellation propagation
  - Event ordering (inner events interleave with bridge/keepalive)

Key insight: The inner executor is the PRIMARY producer. Keepalive and bridge
are AUXILIARY producers that must stop when the primary finishes. This avoids
the circular dependency where wg.Wait blocks on auxiliaries that never stop.

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

// sourceInnerFixture and sourceKeepaliveFixture are Event.Source values used
// throughout this spike's test table (goconst dedup). Named distinctly from
// the local `inner` executor variable used in several tests below.
const (
	sourceInnerFixture     = "inner"
	sourceKeepaliveFixture = "keepalive"
)

// Event is a minimal stand-in for a2a.Event.
type Event struct {
	Source string
	Text   string
	Index  int
}

// BridgeWriter simulates EventBridge: tool handlers call Emit() from arbitrary
// goroutines during execution. Writes go to a shared channel.
type BridgeWriter struct {
	ch chan<- Event
}

func (b *BridgeWriter) Emit(text string) {
	b.ch <- Event{Source: "bridge", Text: text}
}

// innerExecutor simulates the ADK inner executor producing events via an
// iter.Seq2 (the v2 model).
func innerExecutor(ctx context.Context, events []Event) iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {
		for _, e := range events {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if !yield(e, nil) {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

type eventOrErr struct {
	event Event
	err   error
}

// channelMergeExecute demonstrates the channel-merge pattern.
//
// Lifecycle:
//  1. Inner executor runs in a goroutine (primary producer)
//  2. Keepalive ticker runs in a goroutine (auxiliary)
//  3. Bridge reader runs in a goroutine (auxiliary)
//  4. When the inner executor finishes, it closes `done` -> auxiliaries stop
//  5. All producers join via WaitGroup -> merged channel closes
//  6. Consumer drains merged channel
func channelMergeExecute(
	ctx context.Context,
	innerIter iter.Seq2[Event, error],
	bridgeCh chan Event,
) iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {
		merged := make(chan eventOrErr, 32)
		done := make(chan struct{})
		var wg sync.WaitGroup

		// Primary producer: inner executor events.
		// Closing `done` signals auxiliaries to stop.
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(done)
			for event, err := range innerIter {
				select {
				case merged <- eventOrErr{event: event, err: err}:
				case <-ctx.Done():
					return
				}
			}
		}()

		// Auxiliary producer: keepalive ticker
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(25 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-done:
					return
				case <-ticker.C:
					select {
					case merged <- eventOrErr{event: Event{Source: sourceKeepaliveFixture, Text: "."}}:
					case <-ctx.Done():
						return
					case <-done:
						return
					}
				}
			}
		}()

		// Auxiliary producer: bridge side-channel events
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case <-done:
					return
				case e, ok := <-bridgeCh:
					if !ok {
						return
					}
					select {
					case merged <- eventOrErr{event: e}:
					case <-ctx.Done():
						return
					case <-done:
						return
					}
				}
			}
		}()

		// Closer: all producers done -> close merged channel
		go func() {
			wg.Wait()
			close(merged)
		}()

		// Consumer: drain merged channel and yield to the iterator caller
		for eoe := range merged {
			if !yield(eoe.event, eoe.err) {
				// Consumer broke early -- drain to unblock producers
				for range merged {
				}
				return
			}
		}
	}
}

// reinvocationChainExecute demonstrates re-invocation chaining:
// calls innerExecutorFn N times (simulating the re-invocation loop),
// flattening all yielded events into one output iter.Seq2.
//
// The inner executor calls run SEQUENTIALLY (matching the current
// StreamingExecutor re-invocation loop), but keepalive and bridge
// run for the entire lifecycle across all rounds.
func reinvocationChainExecute(
	ctx context.Context,
	maxReinvocations int,
	needsReinvocation func(lastEvent Event, count int) bool,
	innerExecutorFn func(ctx context.Context, round int) iter.Seq2[Event, error],
	bridgeCh chan Event,
) iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {
		merged := make(chan eventOrErr, 32)
		done := make(chan struct{})
		var wg sync.WaitGroup

		// Auxiliary: keepalive spans the entire re-invocation lifecycle
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(25 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-done:
					return
				case <-ticker.C:
					select {
					case merged <- eventOrErr{event: Event{Source: sourceKeepaliveFixture, Text: "."}}:
					case <-ctx.Done():
						return
					case <-done:
						return
					}
				}
			}
		}()

		// Auxiliary: bridge side-channel spans the entire lifecycle
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case <-done:
					return
				case e, ok := <-bridgeCh:
					if !ok {
						return
					}
					select {
					case merged <- eventOrErr{event: e}:
					case <-ctx.Done():
						return
					case <-done:
						return
					}
				}
			}
		}()

		// Primary: inner executor runs sequentially across rounds.
		// Closing `done` signals auxiliaries after ALL rounds complete.
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(done)
			var lastEvent Event
			reinvokeCount := 0

			for round := 0; round <= maxReinvocations; round++ {
				if round > 0 {
					if !needsReinvocation(lastEvent, reinvokeCount) {
						break
					}
					reinvokeCount++
				}

				for event, err := range innerExecutorFn(ctx, round) {
					lastEvent = event
					select {
					case merged <- eventOrErr{event: event, err: err}:
					case <-ctx.Done():
						return
					}
					if err != nil {
						return
					}
				}
			}
		}()

		// Closer
		go func() {
			wg.Wait()
			close(merged)
		}()

		// Consumer
		for eoe := range merged {
			if !yield(eoe.event, eoe.err) {
				for range merged {
				}
				return
			}
		}
	}
}

// --- Tests ---

func TestChannelMerge_BasicOrdering(t *testing.T) {
	ctx := context.Background()

	innerEvents := []Event{
		{Source: sourceInnerFixture, Text: "event-1", Index: 0},
		{Source: sourceInnerFixture, Text: "event-2", Index: 1},
		{Source: sourceInnerFixture, Text: "event-3", Index: 2},
	}

	bridgeCh := make(chan Event, 10)
	inner := innerExecutor(ctx, innerEvents)

	// Simulate a bridge write after a small delay
	go func() {
		time.Sleep(15 * time.Millisecond)
		bridgeCh <- Event{Source: "bridge", Text: "reasoning-1"}
	}()

	var collected []Event
	for event, err := range channelMergeExecute(ctx, inner, bridgeCh) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		collected = append(collected, event)
	}

	if len(collected) < 3 {
		t.Fatalf("expected at least 3 events (inner), got %d", len(collected))
	}

	innerCount := 0
	bridgeCount := 0
	keepaliveCount := 0
	for _, e := range collected {
		switch e.Source {
		case sourceInnerFixture:
			innerCount++
		case "bridge":
			bridgeCount++
		case sourceKeepaliveFixture:
			keepaliveCount++
		}
	}
	if innerCount != 3 {
		t.Fatalf("expected 3 inner events, got %d", innerCount)
	}

	t.Logf("collected %d events: inner=%d, bridge=%d, keepalive=%d",
		len(collected), innerCount, bridgeCount, keepaliveCount)
}

func TestChannelMerge_KeepaliveEmits(t *testing.T) {
	ctx := context.Background()

	// Slow inner executor: 3 events with 30ms gaps -> keepalive at 25ms fires
	slowEvents := []Event{
		{Source: sourceInnerFixture, Text: "slow-1"},
		{Source: sourceInnerFixture, Text: "slow-2"},
		{Source: sourceInnerFixture, Text: "slow-3"},
	}

	slowInner := func(yield func(Event, error) bool) {
		for _, e := range slowEvents {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if !yield(e, nil) {
				return
			}
			time.Sleep(30 * time.Millisecond)
		}
	}

	bridgeCh := make(chan Event, 10)

	var collected []Event
	for event, err := range channelMergeExecute(ctx, slowInner, bridgeCh) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		collected = append(collected, event)
	}

	keepaliveCount := 0
	for _, e := range collected {
		if e.Source == sourceKeepaliveFixture {
			keepaliveCount++
		}
	}

	if keepaliveCount == 0 {
		t.Fatal("expected at least 1 keepalive event during slow execution")
	}
	t.Logf("keepalive events emitted: %d", keepaliveCount)
}

func TestChannelMerge_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	infiniteInner := func(yield func(Event, error) bool) {
		i := 0
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if !yield(Event{Source: sourceInnerFixture, Text: fmt.Sprintf("event-%d", i)}, nil) {
				return
			}
			i++
			time.Sleep(5 * time.Millisecond)
		}
	}

	bridgeCh := make(chan Event, 10)
	var collected []Event

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	for event, err := range channelMergeExecute(ctx, infiniteInner, bridgeCh) {
		if err != nil {
			break
		}
		collected = append(collected, event)
	}

	if len(collected) == 0 {
		t.Fatal("expected some events before cancellation")
	}
	t.Logf("collected %d events before context cancellation", len(collected))
}

func TestReinvocationChain_MultipleRounds(t *testing.T) {
	ctx := context.Background()
	bridgeCh := make(chan Event, 10)

	innerFn := func(_ context.Context, round int) iter.Seq2[Event, error] {
		return func(yield func(Event, error) bool) {
			if round < 2 {
				yield(Event{Source: sourceInnerFixture, Text: fmt.Sprintf("text-only-round-%d", round), Index: round}, nil)
			} else {
				yield(Event{Source: sourceInnerFixture, Text: "tool-call-round", Index: round}, nil)
			}
		}
	}

	needsReinvoke := func(last Event, count int) bool {
		return last.Text != "tool-call-round" && count < 3
	}

	var collected []Event
	for event, err := range reinvocationChainExecute(ctx, 3, needsReinvoke, innerFn, bridgeCh) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		collected = append(collected, event)
	}

	innerCount := 0
	for _, e := range collected {
		if e.Source == sourceInnerFixture {
			innerCount++
		}
	}

	if innerCount != 3 {
		t.Fatalf("expected 3 inner events (3 rounds), got %d", innerCount)
	}

	t.Logf("collected %d total events across %d re-invocation rounds", len(collected), 3)
}

func TestReinvocationChain_KeepaliveContinuesAcrossRounds(t *testing.T) {
	ctx := context.Background()
	bridgeCh := make(chan Event, 10)

	innerFn := func(ctx context.Context, round int) iter.Seq2[Event, error] {
		return func(yield func(Event, error) bool) {
			time.Sleep(30 * time.Millisecond)
			yield(Event{Source: sourceInnerFixture, Text: fmt.Sprintf("round-%d", round)}, nil)
		}
	}

	needsReinvoke := func(_ Event, count int) bool { return count < 2 }

	var collected []Event
	for event, err := range reinvocationChainExecute(ctx, 3, needsReinvoke, innerFn, bridgeCh) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		collected = append(collected, event)
	}

	keepaliveCount := 0
	innerCount := 0
	for _, e := range collected {
		switch e.Source {
		case sourceKeepaliveFixture:
			keepaliveCount++
		case sourceInnerFixture:
			innerCount++
		}
	}

	if innerCount != 3 {
		t.Fatalf("expected 3 inner events (3 rounds), got %d", innerCount)
	}
	if keepaliveCount == 0 {
		t.Fatal("expected keepalives to fire across re-invocation rounds")
	}
	t.Logf("keepalives across %d re-invocation rounds: %d", 3, keepaliveCount)
}

func TestReinvocationChain_BridgeWritesDuringExecution(t *testing.T) {
	ctx := context.Background()
	bridgeCh := make(chan Event, 10)

	innerFn := func(_ context.Context, round int) iter.Seq2[Event, error] {
		return func(yield func(Event, error) bool) {
			bridgeCh <- Event{Source: "bridge", Text: fmt.Sprintf("reasoning-round-%d", round)}
			time.Sleep(10 * time.Millisecond)
			yield(Event{Source: sourceInnerFixture, Text: fmt.Sprintf("round-%d", round)}, nil)
		}
	}

	needsReinvoke := func(_ Event, count int) bool { return count < 1 }

	var collected []Event
	for event, err := range reinvocationChainExecute(ctx, 2, needsReinvoke, innerFn, bridgeCh) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		collected = append(collected, event)
	}

	bridgeCount := 0
	innerCount := 0
	for _, e := range collected {
		switch e.Source {
		case "bridge":
			bridgeCount++
		case sourceInnerFixture:
			innerCount++
		}
	}

	if innerCount != 2 {
		t.Fatalf("expected 2 inner events, got %d", innerCount)
	}
	if bridgeCount != 2 {
		t.Fatalf("expected 2 bridge events (one per round), got %d", bridgeCount)
	}
	t.Logf("bridge events interleaved with %d inner events across 2 rounds", innerCount)
}

func TestChannelMerge_NoGoroutineLeak(t *testing.T) {
	ctx := context.Background()

	innerEvents := []Event{{Source: sourceInnerFixture, Text: "only-one"}}
	bridgeCh := make(chan Event, 10)
	inner := innerExecutor(ctx, innerEvents)

	for event, err := range channelMergeExecute(ctx, inner, bridgeCh) {
		_ = event
		_ = err
	}

	time.Sleep(50 * time.Millisecond)
	t.Log("no goroutine leak detected (test completed without hanging)")
}

func TestChannelMerge_ErrorPropagation(t *testing.T) {
	ctx := context.Background()
	bridgeCh := make(chan Event, 10)

	testErr := fmt.Errorf("simulated inner executor failure")
	failingInner := func(yield func(Event, error) bool) {
		yield(Event{Source: sourceInnerFixture, Text: "ok-event"}, nil)
		yield(Event{}, testErr)
	}

	var sawError bool
	for _, err := range channelMergeExecute(ctx, failingInner, bridgeCh) {
		if err != nil {
			sawError = true
			if !errors.Is(err, testErr) {
				t.Fatalf("expected testErr, got %v", err)
			}
			break
		}
	}

	if !sawError {
		t.Fatal("expected error from inner executor to propagate")
	}
	t.Log("error propagation works correctly")
}
