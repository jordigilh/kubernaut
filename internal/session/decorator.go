package session

import (
	"context"
	"fmt"

	adksession "google.golang.org/adk/session"

	"github.com/jordigilh/kubernaut-apifrontend/api/apifrontend/v1alpha1"
	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
)

type sessionCreateContextKey struct{}

// SessionCreateContext carries A2A task metadata through the context for
// the decorator to inject into the session creation request.
type SessionCreateContext struct {
	TaskID         string
	RemediationRef v1alpha1.ObjectRef
}

// WithSessionCreateContext returns a context enriched with session creation metadata.
func WithSessionCreateContext(ctx context.Context, sc *SessionCreateContext) context.Context {
	return context.WithValue(ctx, sessionCreateContextKey{}, sc)
}

// SessionCreateContextFromContext extracts the SessionCreateContext. Returns nil if not set.
func SessionCreateContextFromContext(ctx context.Context) *SessionCreateContext {
	v, _ := ctx.Value(sessionCreateContextKey{}).(*SessionCreateContext)
	return v
}

// SessionServiceDecorator wraps an adksession.Service and enriches Create
// requests with CRD metadata from context. All other methods delegate unchanged.
type SessionServiceDecorator struct {
	inner adksession.Service
}

// NewSessionServiceDecorator creates a decorator that wraps the given service.
// Panics if inner is nil (programming error — caught at startup, not runtime).
func NewSessionServiceDecorator(inner adksession.Service) *SessionServiceDecorator {
	if inner == nil {
		panic("session.NewSessionServiceDecorator: inner service must not be nil")
	}
	return &SessionServiceDecorator{inner: inner}
}

// Create enriches the request State with CreateConfig extracted from context,
// then delegates to the inner service.
func (d *SessionServiceDecorator) Create(ctx context.Context, req *adksession.CreateRequest) (*adksession.CreateResponse, error) {
	sc := SessionCreateContextFromContext(ctx)
	if sc == nil {
		return d.inner.Create(ctx, req)
	}

	identity := auth.UserIdentityFromContext(ctx)
	if identity == nil || identity.Username == "" {
		return nil, fmt.Errorf("session creation requires authenticated user identity")
	}

	cfg := &CreateConfig{
		A2ATaskID: sc.TaskID,
		UserIdentity: v1alpha1.SessionUser{
			Username: identity.Username,
			Groups:   identity.Groups,
		},
		RemediationRef: sc.RemediationRef,
	}

	if req.State == nil {
		req.State = make(map[string]any)
	}
	req.State[StateKeyCreateConfig] = cfg

	return d.inner.Create(ctx, req)
}

// Get delegates to the inner service.
func (d *SessionServiceDecorator) Get(ctx context.Context, req *adksession.GetRequest) (*adksession.GetResponse, error) {
	return d.inner.Get(ctx, req)
}

// List delegates to the inner service.
func (d *SessionServiceDecorator) List(ctx context.Context, req *adksession.ListRequest) (*adksession.ListResponse, error) {
	return d.inner.List(ctx, req)
}

// Delete delegates to the inner service.
func (d *SessionServiceDecorator) Delete(ctx context.Context, req *adksession.DeleteRequest) error {
	return d.inner.Delete(ctx, req)
}

// AppendEvent delegates to the inner service.
func (d *SessionServiceDecorator) AppendEvent(ctx context.Context, sess adksession.Session, event *adksession.Event) error {
	return d.inner.AppendEvent(ctx, sess, event)
}

// Compile-time interface assertion.
var _ adksession.Service = (*SessionServiceDecorator)(nil)
