package launcher_test

import (
	"context"
	"fmt"

	"github.com/a2aproject/a2a-go/a2a"
)

// fakeQueue implements eventqueue.Queue for testing event bridge and executor.
type fakeQueue struct {
	events []a2a.Event
	closed bool
}

func (q *fakeQueue) Write(_ context.Context, event a2a.Event) error {
	if q.closed {
		return fmt.Errorf("queue closed")
	}
	q.events = append(q.events, event)
	return nil
}

func (q *fakeQueue) WriteVersioned(_ context.Context, _ a2a.Event, _ a2a.TaskVersion) error {
	return fmt.Errorf("not supported")
}

func (q *fakeQueue) Read(_ context.Context) (a2a.Event, a2a.TaskVersion, error) {
	return nil, 0, fmt.Errorf("not supported")
}

func (q *fakeQueue) Close() error {
	q.closed = true
	return nil
}

// blockingQueue blocks on Write until context is done.
type blockingQueue struct{}

func (q *blockingQueue) Write(ctx context.Context, _ a2a.Event) error {
	<-ctx.Done()
	return ctx.Err()
}

func (q *blockingQueue) WriteVersioned(_ context.Context, _ a2a.Event, _ a2a.TaskVersion) error {
	return fmt.Errorf("not supported")
}

func (q *blockingQueue) Read(_ context.Context) (a2a.Event, a2a.TaskVersion, error) {
	return nil, 0, fmt.Errorf("not supported")
}

func (q *blockingQueue) Close() error { return nil }
