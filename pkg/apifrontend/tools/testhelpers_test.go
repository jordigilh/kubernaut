package tools_test

import (
	"context"
	"fmt"
	"sync"

	"github.com/a2aproject/a2a-go/a2a"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func newForbiddenError(resource string) *errors.StatusError {
	return errors.NewForbidden(
		schema.GroupResource{Group: "kubernaut.ai", Resource: resource},
		"",
		nil,
	)
}

// bridgeQueue captures A2A events written by the EventBridge for test assertions.
type bridgeQueue struct {
	mu     sync.Mutex
	events []a2a.Event
}

func (q *bridgeQueue) Write(_ context.Context, event a2a.Event) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.events = append(q.events, event)
	return nil
}

func (q *bridgeQueue) WriteVersioned(_ context.Context, _ a2a.Event, _ a2a.TaskVersion) error {
	return fmt.Errorf("not supported")
}

func (q *bridgeQueue) Read(_ context.Context) (a2a.Event, a2a.TaskVersion, error) {
	return nil, 0, fmt.Errorf("not supported")
}

func (q *bridgeQueue) Close() error { return nil }

func (q *bridgeQueue) Events() []a2a.Event {
	q.mu.Lock()
	defer q.mu.Unlock()
	cp := make([]a2a.Event, len(q.events))
	copy(cp, q.events)
	return cp
}
