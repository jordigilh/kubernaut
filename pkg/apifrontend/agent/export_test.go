package agent

import (
	"context"
	"iter"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/session"
	"google.golang.org/genai"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
)

// MockReadonlyContext creates a minimal ReadonlyContext for testing
// InstructionProvider with the given user identity injected.
func MockReadonlyContext(parent context.Context, username string, groups []string) agent.ReadonlyContext {
	ctx := auth.WithUserIdentity(parent, &auth.UserIdentity{
		Username: username,
		Groups:   groups,
	})
	return &mockReadonlyCtx{Context: ctx}
}

type mockReadonlyCtx struct {
	context.Context
}

func (m *mockReadonlyCtx) UserContent() *genai.Content          { return nil }
func (m *mockReadonlyCtx) InvocationID() string                 { return "test-invocation" }
func (m *mockReadonlyCtx) AgentName() string                    { return "test-agent" }
func (m *mockReadonlyCtx) ReadonlyState() session.ReadonlyState { return &emptyState{} }
func (m *mockReadonlyCtx) UserID() string                       { return "" }
func (m *mockReadonlyCtx) AppName() string                      { return "test-app" }
func (m *mockReadonlyCtx) SessionID() string                    { return "test-session" }
func (m *mockReadonlyCtx) Branch() string                       { return "" }

type emptyState struct{}

func (e *emptyState) Get(_ string) (any, error) {
	return nil, nil
}

func (e *emptyState) All() iter.Seq2[string, any] {
	return func(yield func(string, any) bool) {}
}
