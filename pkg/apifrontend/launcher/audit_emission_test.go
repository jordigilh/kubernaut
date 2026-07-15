package launcher

import (
	"context"
	"iter"
	"sync"
	"testing"

	"github.com/go-logr/logr"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
	"google.golang.org/adk/server/adka2a"
	"google.golang.org/adk/session"
	"google.golang.org/genai"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	afsession "github.com/jordigilh/kubernaut/pkg/apifrontend/session"
)

type auditSpy struct {
	mu     sync.Mutex
	events []*audit.Event
}

func (s *auditSpy) Emit(_ context.Context, event *audit.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

func (s *auditSpy) eventsByType(t audit.EventType) []*audit.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*audit.Event
	for _, e := range s.events {
		if e.Type == t {
			out = append(out, e)
		}
	}
	return out
}

// stubExecutorContext is a minimal adka2a.ExecutorContext for unit tests.
type stubExecutorContext struct {
	context.Context
}

func (s *stubExecutorContext) SessionID() string                        { return "test-session" }
func (s *stubExecutorContext) UserID() string                           { return "test-user" }
func (s *stubExecutorContext) AgentName() string                        { return "kubernaut-apifrontend" }
func (s *stubExecutorContext) ReadonlyState() session.ReadonlyState     { return emptyState{} }
func (s *stubExecutorContext) Events() session.Events                   { return emptyEvents{} }
func (s *stubExecutorContext) UserContent() *genai.Content              { return nil }
func (s *stubExecutorContext) RequestContext() *a2asrv.RequestContext    { return nil }

type emptyState struct{}

func (emptyState) Get(string) (any, error) { return nil, nil }
func (emptyState) All() iter.Seq2[string, any] {
	return func(func(string, any) bool) {}
}

type emptyEvents struct{}

func (emptyEvents) At(int) *session.Event  { return nil }
func (emptyEvents) All() iter.Seq[*session.Event] {
	return func(func(*session.Event) bool) {}
}
func (emptyEvents) Len() int { return 0 }

var _ adka2a.ExecutorContext = (*stubExecutorContext)(nil)

// UT-AF-1156-063: emits triage.started in BeforeExecuteCallback
func TestBeforeExecuteCallback_EmitsTriageStarted(t *testing.T) {
	spy := &auditSpy{}
	cb := buildBeforeExecuteCallback(nil, spy)

	taskID := a2a.TaskID("task-triage-1")
	reqCtx := &a2asrv.RequestContext{TaskID: taskID}
	_, err := cb(context.Background(), reqCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := spy.eventsByType(audit.EventTriageStarted)
	if len(events) != 1 {
		t.Fatalf("expected 1 triage.started event, got %d", len(events))
	}
	if events[0].Detail["task_id"] != "task-triage-1" {
		t.Errorf("task_id = %q, want %q", events[0].Detail["task_id"], "task-triage-1")
	}
}

// UT-AF-1156-064: emits triage.completed in AfterExecuteCallback on success
func TestAfterExecuteCallback_EmitsTriageCompleted(t *testing.T) {
	spy := &auditSpy{}
	log := logr.Discard()
	cb := buildAfterExecuteCallback(log, spy)

	taskID := a2a.TaskID("task-triage-done")
	finalEvent := &a2a.TaskStatusUpdateEvent{TaskID: taskID}
	execCtx := &stubExecutorContext{Context: context.Background()}
	err := cb(execCtx, finalEvent, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := spy.eventsByType(audit.EventTriageCompleted)
	if len(events) != 1 {
		t.Fatalf("expected 1 triage.completed event, got %d", len(events))
	}
	if events[0].Detail["task_id"] != "task-triage-done" {
		t.Errorf("task_id = %q, want %q", events[0].Detail["task_id"], "task-triage-done")
	}
}

// IT-AF-1234-W40: BeforeExecuteCallback populates CreateContext.SessionID from ContextID
func TestBeforeExecuteCallback_SetsSessionIDFromContextID(t *testing.T) {
	t.Parallel()
	cb := buildBeforeExecuteCallback(nil, nil)

	reqCtx := &a2asrv.RequestContext{
		TaskID:    "task-sid-test",
		ContextID: "ctx-abc-123",
	}
	ctx, err := cb(context.Background(), reqCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sc := afsession.CreateContextFromContext(ctx)
	if sc == nil {
		t.Fatal("IT-AF-1234-W40: CreateContext should be injected into context")
	}
	if sc.TaskID != "task-sid-test" { //nolint:staticcheck // SA5011 false positive: sc is guaranteed non-nil by the preceding t.Fatal guard
		t.Errorf("IT-AF-1234-W40: TaskID = %q, want %q", sc.TaskID, "task-sid-test")
	}
	if sc.SessionID != "ctx-abc-123" {
		t.Errorf("IT-AF-1234-W40: SessionID = %q, want %q (from ContextID)", sc.SessionID, "ctx-abc-123")
	}
}
