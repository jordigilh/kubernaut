package launcher_test

import (
	"bytes"
	"context"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
)

var _ = Describe("SessionInterceptor (BR-SESS-020, BR-SESS-021, BR-SESS-023)", func() {
	var (
		registry    *launcher.ActiveContextRegistry
		interceptor *launcher.SessionInterceptor
		logBuf      *bytes.Buffer
	)

	BeforeEach(func() {
		registry = launcher.NewActiveContextRegistry(2*time.Hour, 10*time.Minute)
		logBuf = &bytes.Buffer{}
		logger := funcr.New(func(prefix, args string) {
			logBuf.WriteString(prefix + " " + args + "\n")
		}, funcr.Options{})
		interceptor = launcher.NewSessionInterceptor(registry, logger)
	})

	It("UT-AF-SESS-020-010: Sets callCtx.User from auth context (AC-2)", func() {
		ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
			Username: "alice", Groups: []string{"sre"},
		})
		callCtx := newTestCallContext()
		req := &a2asrv.Request{
			Payload: &a2a.MessageSendParams{
				Message: &a2a.Message{ContextID: "ctx-new-123"},
			},
		}

		_, err := interceptor.Before(ctx, callCtx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(callCtx.User).NotTo(BeNil())
		Expect(callCtx.User.Name()).To(Equal("alice"))
		Expect(callCtx.User.Authenticated()).To(BeTrue())
	})

	It("UT-AF-SESS-020-011: No-op when auth identity is nil (AC-2)", func() {
		ctx := context.Background()
		callCtx := newTestCallContext()
		req := &a2asrv.Request{
			Payload: &a2a.MessageSendParams{
				Message: &a2a.Message{ContextID: "ctx-123"},
			},
		}

		_, err := interceptor.Before(ctx, callCtx, req)
		Expect(err).NotTo(HaveOccurred())
		// User should remain unauthenticated (default)
		Expect(callCtx.User.Name()).To(BeEmpty())
	})

	It("UT-AF-SESS-020-012: Preserves explicit ContextID even when active context differs (SC-7)", func() {
		registry.Set("bob", "ctx-active-session")

		ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
			Username: "bob", Groups: []string{"sre"},
		})
		callCtx := newTestCallContext()
		msg := &a2a.Message{ContextID: "ctx-new-random"}
		req := &a2asrv.Request{
			Payload: &a2a.MessageSendParams{Message: msg},
		}

		_, err := interceptor.Before(ctx, callCtx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(msg.ContextID).To(Equal("ctx-new-random"),
			"Explicit ContextID must be preserved — interceptor only overrides empty values")
	})

	It("UT-AF-SESS-020-013: No-op on ContextID when registry has no entry (SC-7)", func() {
		ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
			Username: "carol", Groups: []string{"sre"},
		})
		callCtx := newTestCallContext()
		msg := &a2a.Message{ContextID: "ctx-original"}
		req := &a2asrv.Request{
			Payload: &a2a.MessageSendParams{Message: msg},
		}

		_, err := interceptor.Before(ctx, callCtx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(msg.ContextID).To(Equal("ctx-original"),
			"ContextID must not be modified when no active context exists")
	})

	It("UT-AF-SESS-020-014: No-op on ContextID when already matches active context (SC-7)", func() {
		registry.Set("dave", "ctx-same-id")

		ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
			Username: "dave", Groups: []string{"sre"},
		})
		callCtx := newTestCallContext()
		msg := &a2a.Message{ContextID: "ctx-same-id"}
		req := &a2asrv.Request{
			Payload: &a2a.MessageSendParams{Message: msg},
		}

		_, err := interceptor.Before(ctx, callCtx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(msg.ContextID).To(Equal("ctx-same-id"),
			"ContextID must not be modified when it already matches the active context")
	})

	It("UT-AF-SESS-020-015: No-op on non-MessageSendParams payloads (SC-7)", func() {
		registry.Set("eve", "ctx-active")

		ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
			Username: "eve", Groups: []string{"sre"},
		})
		callCtx := newTestCallContext()
		req := &a2asrv.Request{
			Payload: &a2a.TaskQueryParams{ID: "task-123"},
		}

		_, err := interceptor.Before(ctx, callCtx, req)
		Expect(err).NotTo(HaveOccurred())
		// No panic, no override -- non-message payloads are untouched
	})

	It("UT-AF-SESS-020-016: Logs override with original and target context_id (AU-12)", func() {
		registry.Set("frank", "ctx-target")

		ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
			Username: "frank", Groups: []string{"sre"},
		})
		callCtx := newTestCallContext()
		msg := &a2a.Message{ContextID: ""}
		req := &a2asrv.Request{
			Payload: &a2a.MessageSendParams{Message: msg},
		}

		_, err := interceptor.Before(ctx, callCtx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(msg.ContextID).To(Equal("ctx-target"),
			"Empty ContextID must be overridden to the active context")

		logOutput := logBuf.String()
		Expect(logOutput).To(ContainSubstring("overriding context_id"),
			"Override must be logged for audit trail")
		Expect(logOutput).To(ContainSubstring("ctx-target"),
			"Log must contain the target context_id")
	})

	It("UT-AF-SESS-020-018: Explicit ContextID skips override without logging (SC-7)", func() {
		registry.Set("gina", "ctx-active")

		ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
			Username: "gina", Groups: []string{"sre"},
		})
		callCtx := newTestCallContext()
		msg := &a2a.Message{ContextID: "ctx-explicit"}
		req := &a2asrv.Request{
			Payload: &a2a.MessageSendParams{Message: msg},
		}

		_, err := interceptor.Before(ctx, callCtx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(msg.ContextID).To(Equal("ctx-explicit"),
			"Explicit ContextID must not be overridden")

		logOutput := logBuf.String()
		Expect(logOutput).NotTo(ContainSubstring("overriding context_id"),
			"No override log when explicit ContextID is provided")
	})

	It("UT-AF-1446-006: SC-7, AC-6 — Does NOT override contextId when session is idle-expired; clears stale entry (#1446)", func() {
		shortIdle := launcher.NewActiveContextRegistry(2*time.Hour, 1*time.Millisecond)
		logBuf.Reset()
		logger := funcr.New(func(prefix, args string) {
			logBuf.WriteString(prefix + " " + args + "\n")
		}, funcr.Options{})
		staleInterceptor := launcher.NewSessionInterceptor(shortIdle, logger)

		shortIdle.Set("hank", "ctx-stale-investigation")
		time.Sleep(5 * time.Millisecond)

		ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
			Username: "hank", Groups: []string{"sre"},
		})
		callCtx := newTestCallContext()
		msg := &a2a.Message{ContextID: ""}
		req := &a2asrv.Request{
			Payload: &a2a.MessageSendParams{Message: msg},
		}

		_, err := staleInterceptor.Before(ctx, callCtx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(msg.ContextID).To(BeEmpty(),
			"SC-7: idle-expired session must NOT hijack new conversations — contextId must remain empty")

		_, ok := shortIdle.Get("hank")
		Expect(ok).To(BeFalse(),
			"AC-6: stale registry entry must be cleared to prevent repeated redirect attempts")

		Expect(logBuf.String()).To(ContainSubstring("clearing stale context"),
			"AU-3: stale entry clearing must be logged for audit traceability")
	})

	It("UT-AF-SESS-020-017: Overrides empty ContextID when active context exists (#1345)", func() {
		registry.Set("grace", "ctx-active-investigation")

		ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
			Username: "grace", Groups: []string{"sre"},
		})
		callCtx := newTestCallContext()
		msg := &a2a.Message{ContextID: ""}
		req := &a2asrv.Request{
			Payload: &a2a.MessageSendParams{Message: msg},
		}

		_, err := interceptor.Before(ctx, callCtx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(msg.ContextID).To(Equal("ctx-active-investigation"),
			"Empty ContextID must be overridden when an active investigation exists for the user")

		logOutput := logBuf.String()
		Expect(logOutput).To(ContainSubstring("overriding context_id"),
			"Override must be logged for audit trail")
	})
})

var _ = Describe("SessionInterceptor stale context validation (BR-SESS-025, #1472)", func() {
	var (
		registry    *launcher.ActiveContextRegistry
		logBuf      *bytes.Buffer
	)

	BeforeEach(func() {
		registry = launcher.NewActiveContextRegistry(2*time.Hour, 10*time.Minute)
		logBuf = &bytes.Buffer{}
	})

	newLogger := func() logr.Logger {
		return funcr.New(func(prefix, args string) {
			logBuf.WriteString(prefix + " " + args + "\n")
		}, funcr.Options{})
	}

	It("UT-AF-1472-001: Validator returns false for non-existent session (SC-7, SI-10)", func() {
		validator := &fakeStaleSessionValidator{valid: false}
		logger := newLogger()
		interceptor := launcher.NewSessionInterceptor(registry, logger, launcher.WithStaleSessionValidator(validator))

		ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
			Username: "alice", Groups: []string{"sre"},
		})
		callCtx := newTestCallContext()
		msg := &a2a.Message{ContextID: "stale-ctx-from-previous-pod"}
		req := &a2asrv.Request{
			Payload: &a2a.MessageSendParams{Message: msg},
		}

		_, err := interceptor.Before(ctx, callCtx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(msg.ContextID).To(BeEmpty(),
			"SC-7: stale context_id must be cleared when validator reports session not found")
		Expect(validator.calledWith).To(Equal("stale-ctx-from-previous-pod"),
			"Validator must be called with the original context_id")
	})

	It("UT-AF-1472-002: Validator returns true for existing session", func() {
		validator := &fakeStaleSessionValidator{valid: true}
		logger := newLogger()
		interceptor := launcher.NewSessionInterceptor(registry, logger, launcher.WithStaleSessionValidator(validator))

		ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
			Username: "bob", Groups: []string{"sre"},
		})
		callCtx := newTestCallContext()
		msg := &a2a.Message{ContextID: "active-session-ctx"}
		req := &a2asrv.Request{
			Payload: &a2a.MessageSendParams{Message: msg},
		}

		_, err := interceptor.Before(ctx, callCtx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(msg.ContextID).To(Equal("active-session-ctx"),
			"Valid context_id must be preserved when session exists in memory")
	})

	It("UT-AF-1472-003: Validator returns true (fail-open) on unexpected error (SC-5)", func() {
		validator := &fakeStaleSessionValidator{valid: true}
		logger := newLogger()
		interceptor := launcher.NewSessionInterceptor(registry, logger, launcher.WithStaleSessionValidator(validator))

		ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
			Username: "carol", Groups: []string{"sre"},
		})
		callCtx := newTestCallContext()
		msg := &a2a.Message{ContextID: "any-ctx-id"}
		req := &a2asrv.Request{
			Payload: &a2a.MessageSendParams{Message: msg},
		}

		_, err := interceptor.Before(ctx, callCtx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(msg.ContextID).To(Equal("any-ctx-id"),
			"SC-5: context_id must be preserved (fail-open) when validator cannot determine validity")
	})

	It("UT-AF-1472-004: Interceptor clears stale context_id and logs (SC-10, AU-3)", func() {
		validator := &fakeStaleSessionValidator{valid: false}
		logger := newLogger()
		interceptor := launcher.NewSessionInterceptor(registry, logger, launcher.WithStaleSessionValidator(validator))

		ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
			Username: "dave", Groups: []string{"sre"},
		})
		callCtx := newTestCallContext()
		msg := &a2a.Message{ContextID: "stale-ctx-after-restart"}
		req := &a2asrv.Request{
			Payload: &a2a.MessageSendParams{Message: msg},
		}

		_, err := interceptor.Before(ctx, callCtx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(msg.ContextID).To(BeEmpty(),
			"SC-10: stale context_id must be cleared after post-restart detection")

		logOutput := logBuf.String()
		Expect(logOutput).To(ContainSubstring("stale-ctx-after-restart"),
			"AU-3: log must contain the original stale context_id for traceability")
		Expect(logOutput).To(ContainSubstring("dave"),
			"AU-3: log must contain the username for audit correlation")
	})

	It("UT-AF-1472-005: Interceptor preserves valid context_id", func() {
		validator := &fakeStaleSessionValidator{valid: true}
		logger := newLogger()
		interceptor := launcher.NewSessionInterceptor(registry, logger, launcher.WithStaleSessionValidator(validator))

		ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
			Username: "eve", Groups: []string{"sre"},
		})
		callCtx := newTestCallContext()
		msg := &a2a.Message{ContextID: "valid-active-ctx"}
		req := &a2asrv.Request{
			Payload: &a2a.MessageSendParams{Message: msg},
		}

		_, err := interceptor.Before(ctx, callCtx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(msg.ContextID).To(Equal("valid-active-ctx"),
			"Valid context_id must pass through unchanged")
	})

	It("UT-AF-1472-006: Interceptor skips validation for empty context_id", func() {
		validator := &fakeStaleSessionValidator{valid: false}
		logger := newLogger()
		interceptor := launcher.NewSessionInterceptor(registry, logger, launcher.WithStaleSessionValidator(validator))

		ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
			Username: "frank", Groups: []string{"sre"},
		})
		callCtx := newTestCallContext()
		msg := &a2a.Message{ContextID: ""}
		req := &a2asrv.Request{
			Payload: &a2a.MessageSendParams{Message: msg},
		}

		_, err := interceptor.Before(ctx, callCtx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(validator.calledWith).To(BeEmpty(),
			"Validator must NOT be called when context_id is empty — existing registry logic handles routing")
	})
})

// fakeStaleSessionValidator is a test double for StaleSessionValidator.
type fakeStaleSessionValidator struct {
	valid      bool
	calledWith string
}

func (f *fakeStaleSessionValidator) IsContextValid(_ context.Context, contextID, _ string) bool {
	f.calledWith = contextID
	return f.valid
}

// newTestCallContext creates a minimal CallContext for interceptor testing.
// In production, this is created by the JSON-RPC handler via WithCallContext.
func newTestCallContext() *a2asrv.CallContext {
	_, callCtx := a2asrv.WithCallContext(context.Background(), nil)
	return callCtx
}
