package audit_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/audit"
)

type mockWriter struct {
	mu     sync.Mutex
	events []*audit.Event
	err    error
	calls  int32
}

func (m *mockWriter) WriteAuditEvents(_ context.Context, events []*audit.Event) error {
	atomic.AddInt32(&m.calls, 1)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.events = append(m.events, events...)
	return nil
}

func (m *mockWriter) eventCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

var _ = Describe("BufferedEmitter", func() {
	var (
		writer  *mockWriter
		emitter *audit.BufferedEmitter
	)

	AfterEach(func() {
		if emitter != nil {
			_ = emitter.Close(context.Background())
		}
	})

	Describe("Emit", func() {
		It("buffers and flushes events to the writer", func() {
			writer = &mockWriter{}
			emitter = audit.NewBufferedEmitter(audit.BufferConfig{
				Writer:        writer,
				Logger:        logr.Discard(),
				BufferSize:    100,
				FlushInterval: 50 * time.Millisecond,
				BatchSize:     10,
			})

			for i := 0; i < 5; i++ {
				emitter.Emit(context.Background(), &audit.Event{
					Type:   audit.EventAuthSuccess,
					UserID: "user1",
				})
			}

			Eventually(func() int {
				return writer.eventCount()
			}, 500*time.Millisecond, 10*time.Millisecond).Should(Equal(5))
		})

		It("sanitizes event detail via RedactMap", func() {
			writer = &mockWriter{}
			emitter = audit.NewBufferedEmitter(audit.BufferConfig{
				Writer:        writer,
				Logger:        logr.Discard(),
				BufferSize:    100,
				FlushInterval: 50 * time.Millisecond,
				BatchSize:     10,
			})

			emitter.Emit(context.Background(), &audit.Event{
				Type: audit.EventAuthSuccess,
				Detail: map[string]string{
					"token":    "secret-value",
					"safe_key": "visible",
				},
			})

			Eventually(func() int {
				return writer.eventCount()
			}, 500*time.Millisecond, 10*time.Millisecond).Should(Equal(1))

			writer.mu.Lock()
			evt := writer.events[0]
			writer.mu.Unlock()
			Expect(evt.Detail["token"]).To(Equal("[REDACTED]"))
			Expect(evt.Detail["safe_key"]).To(Equal("visible"))
		})

		It("drops events on buffer overflow without blocking", func() {
			writer = &mockWriter{err: errors.New("slow writer")}
			emitter = audit.NewBufferedEmitter(audit.BufferConfig{
				Writer:        writer,
				Logger:        logr.Discard(),
				BufferSize:    2,
				FlushInterval: 1 * time.Hour,
				BatchSize:     1000,
			})

			for i := 0; i < 10; i++ {
				emitter.Emit(context.Background(), &audit.Event{
					Type: audit.EventAuthSuccess,
				})
			}
			// Should not block — overflow events are dropped
		})
	})

	Describe("Close", func() {
		It("flushes remaining events on close", func() {
			writer = &mockWriter{}
			emitter = audit.NewBufferedEmitter(audit.BufferConfig{
				Writer:        writer,
				Logger:        logr.Discard(),
				BufferSize:    1000,
				FlushInterval: 1 * time.Hour,
				BatchSize:     1000,
			})

			for i := 0; i < 3; i++ {
				emitter.Emit(context.Background(), &audit.Event{
					Type:   audit.EventSessionCreated,
					UserID: "closer",
				})
			}

			time.Sleep(10 * time.Millisecond)
			err := emitter.Close(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(writer.eventCount()).To(Equal(3))
			emitter = nil
		})

		It("logs events if writer fails during close", func() {
			writer = &mockWriter{err: errors.New("DS down")}
			emitter = audit.NewBufferedEmitter(audit.BufferConfig{
				Writer:        writer,
				Logger:        logr.Discard(),
				BufferSize:    1000,
				FlushInterval: 1 * time.Hour,
				BatchSize:     1000,
			})

			emitter.Emit(context.Background(), &audit.Event{
				Type: audit.EventA2ATaskFailed,
			})

			time.Sleep(10 * time.Millisecond)
			err := emitter.Close(context.Background())
			Expect(err).NotTo(HaveOccurred())
			emitter = nil
		})

		It("handles empty buffer gracefully", func() {
			writer = &mockWriter{}
			emitter = audit.NewBufferedEmitter(audit.BufferConfig{
				Writer:        writer,
				Logger:        logr.Discard(),
				BufferSize:    100,
				FlushInterval: 50 * time.Millisecond,
				BatchSize:     10,
			})

			err := emitter.Close(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(writer.eventCount()).To(Equal(0))
			emitter = nil
		})
	})
})
