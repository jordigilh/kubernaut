package launcher_test

import (
	"bytes"
	"context"
	"time"

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
		registry = launcher.NewActiveContextRegistry(2 * time.Hour)
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

	It("UT-AF-SESS-020-012: Overrides msg.ContextID when active context differs (SC-7)", func() {
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
		Expect(msg.ContextID).To(Equal("ctx-active-session"),
			"ContextID must be overridden to the active context from registry")
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
		msg := &a2a.Message{ContextID: "ctx-original"}
		req := &a2asrv.Request{
			Payload: &a2a.MessageSendParams{Message: msg},
		}

		_, err := interceptor.Before(ctx, callCtx, req)
		Expect(err).NotTo(HaveOccurred())

		logOutput := logBuf.String()
		Expect(logOutput).To(ContainSubstring("ctx-original"),
			"Log must contain the original context_id")
		Expect(logOutput).To(ContainSubstring("ctx-target"),
			"Log must contain the target context_id")
	})
})

// newTestCallContext creates a minimal CallContext for interceptor testing.
// In production, this is created by the JSON-RPC handler via WithCallContext.
func newTestCallContext() *a2asrv.CallContext {
	_, callCtx := a2asrv.WithCallContext(context.Background(), nil)
	return callCtx
}
