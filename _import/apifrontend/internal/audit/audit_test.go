package audit_test

import (
	"context"
	"testing"

	"github.com/go-logr/logr/funcr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/audit"
)

func TestAuditSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Audit Suite")
}

var _ = Describe("Audit", func() {
	Describe("LogEmitter", func() {
		It("UT-AF-AUD-001: emits event with all structured fields", func() {
			var captured string
			logger := funcr.New(func(prefix, args string) {
				captured = args
			}, funcr.Options{})

			emitter := audit.NewLogEmitter(logger)
			emitter.Emit(context.Background(), &audit.Event{
				Type:     audit.EventAuthSuccess,
				UserID:   "alice",
				SourceIP: "10.0.0.1",
				Detail:   map[string]string{"issuer": "https://sso.example.com"},
			})

			Expect(captured).To(ContainSubstring("auth.success"))
			Expect(captured).To(ContainSubstring("alice"))
			Expect(captured).To(ContainSubstring("10.0.0.1"))
		})

		It("UT-AF-AUD-002: sets timestamp automatically on emitted events", func() {
			event := &audit.Event{Type: audit.EventAuthFailure}
			Expect(event.Timestamp.IsZero()).To(BeTrue(), "timestamp should be zero before Emit")

			var captured string
			logger := funcr.New(func(prefix, args string) {
				captured = args
			}, funcr.Options{})
			emitter := audit.NewLogEmitter(logger)
			emitter.Emit(context.Background(), event)

			Expect(captured).To(ContainSubstring("timestamp"))
			Expect(captured).NotTo(ContainSubstring("0001-01-01"),
				"logged timestamp must not be the zero time value")
		})

		It("UT-AF-AUD-003: omits empty user_id and source_ip from log output", func() {
			var captured string
			logger := funcr.New(func(prefix, args string) {
				captured = args
			}, funcr.Options{})

			emitter := audit.NewLogEmitter(logger)
			emitter.Emit(context.Background(), &audit.Event{
				Type: audit.EventRateLimitDenied,
			})

			Expect(captured).To(ContainSubstring("ratelimit.denied"))
			Expect(captured).NotTo(ContainSubstring("user_id"))
		})

		It("UT-AF-AUD-006: redacts sensitive keys in event Detail", func() {
			var captured string
			logger := funcr.New(func(prefix, args string) {
				captured = args
			}, funcr.Options{})

			emitter := audit.NewLogEmitter(logger)
			emitter.Emit(context.Background(), &audit.Event{
				Type: audit.EventAuthSuccess,
				Detail: map[string]string{
					"user":        "alice",
					"token":       "super-secret-value",
					"api_key":     "key-12345",
					"description": "normal text",
				},
			})

			Expect(captured).To(ContainSubstring("alice"))
			Expect(captured).To(ContainSubstring("[REDACTED]"))
			Expect(captured).NotTo(ContainSubstring("super-secret-value"))
			Expect(captured).NotTo(ContainSubstring("key-12345"))
			Expect(captured).To(ContainSubstring("normal text"))
		})

		It("UT-AF-AUD-007: redacts JWT patterns in event Detail values", func() {
			var captured string
			logger := funcr.New(func(prefix, args string) {
				captured = args
			}, funcr.Options{})

			jwt := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
			emitter := audit.NewLogEmitter(logger)
			emitter.Emit(context.Background(), &audit.Event{
				Type:   audit.EventAuthFailure,
				Detail: map[string]string{"upstream_header": jwt},
			})

			Expect(captured).NotTo(ContainSubstring("eyJhbGci"))
			Expect(captured).To(ContainSubstring("[JWT_REDACTED]"))
		})

		It("UT-AF-AUD-008: handles nil Detail map without panic", func() {
			var captured string
			logger := funcr.New(func(prefix, args string) {
				captured = args
			}, funcr.Options{})

			emitter := audit.NewLogEmitter(logger)
			Expect(func() {
				emitter.Emit(context.Background(), &audit.Event{
					Type:   audit.EventToolInvoked,
					Detail: nil,
				})
			}).NotTo(Panic())
			Expect(captured).To(ContainSubstring("tool.invoked"))
		})
	})

	Describe("EmitFromContext", func() {
		It("UT-AF-AUD-004: does not panic when emitter is nil", func() {
			Expect(func() {
				audit.EmitFromContext(context.Background(), nil, audit.EventAuthSuccess, "alice", "10.0.0.1", nil)
			}).NotTo(Panic())
		})

		It("UT-AF-AUD-005: passes detail map to emitter", func() {
			var capturedEvent *audit.Event
			emitter := &fakeEmitter{onEmit: func(_ context.Context, e *audit.Event) {
				capturedEvent = e
			}}

			audit.EmitFromContext(context.Background(), emitter, audit.EventImpersonation, "bob", "10.0.0.2",
				map[string]string{"target": "pod-123"})

			Expect(capturedEvent.Type).To(Equal(audit.EventImpersonation))
			Expect(capturedEvent.Detail["target"]).To(Equal("pod-123"))
		})
	})
})

type fakeEmitter struct {
	onEmit func(context.Context, *audit.Event)
}

func (f *fakeEmitter) Emit(ctx context.Context, event *audit.Event) {
	if f.onEmit != nil {
		f.onEmit(ctx, event)
	}
}
