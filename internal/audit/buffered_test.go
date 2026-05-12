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
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

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
			emitter.Start()

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
			emitter.Start()

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

		It("drops events on buffer overflow without blocking and increments counter", func() {
			overflowCounter := prometheus.NewCounter(prometheus.CounterOpts{
				Name: "test_overflow_total",
			})
			writer = &mockWriter{err: errors.New("slow writer")}
			emitter = audit.NewBufferedEmitter(audit.BufferConfig{
				Writer:          writer,
				Logger:          logr.Discard(),
				BufferSize:      2,
				FlushInterval:   1 * time.Hour,
				BatchSize:       1000,
				OverflowCounter: overflowCounter,
			})
			// Deliberately NOT calling Start() — no flush goroutine competing
			// for channel reads, making the overflow count deterministic.

			for i := 0; i < 10; i++ {
				emitter.Emit(context.Background(), &audit.Event{
					Type: audit.EventAuthSuccess,
				})
			}
			// Buffer holds exactly 2, no consumer running, so exactly 8 overflow.
			Expect(testutil.ToFloat64(overflowCounter)).To(BeNumerically("==", 8))
		})
	})

	Describe("EmitBlocking", func() {
		It("buffers security-critical events with backpressure", func() {
			writer = &mockWriter{}
			emitter = audit.NewBufferedEmitter(audit.BufferConfig{
				Writer:        writer,
				Logger:        logr.Discard(),
				BufferSize:    100,
				FlushInterval: 50 * time.Millisecond,
				BatchSize:     10,
			})
			emitter.Start()

			emitter.EmitBlocking(context.Background(), &audit.Event{
				Type:   audit.EventAuthFailure,
				UserID: "attacker",
			})

			Eventually(func() int {
				return writer.eventCount()
			}, 500*time.Millisecond, 10*time.Millisecond).Should(Equal(1))
		})

		It("respects context cancellation on full buffer", func() {
			overflowCounter := prometheus.NewCounter(prometheus.CounterOpts{
				Name: "test_blocking_overflow",
			})
			writer = &mockWriter{}
			emitter = audit.NewBufferedEmitter(audit.BufferConfig{
				Writer:          writer,
				Logger:          logr.Discard(),
				BufferSize:      1,
				FlushInterval:   1 * time.Hour,
				BatchSize:       1000,
				OverflowCounter: overflowCounter,
			})

			emitter.EmitBlocking(context.Background(), &audit.Event{Type: audit.EventAuthSuccess})

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()
			emitter.EmitBlocking(ctx, &audit.Event{Type: audit.EventAuthFailure})

			Expect(testutil.ToFloat64(overflowCounter)).To(BeNumerically("==", 1))
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
			emitter.Start()

			for i := 0; i < 3; i++ {
				emitter.Emit(context.Background(), &audit.Event{
					Type:   audit.EventSessionCreated,
					UserID: "closer",
				})
			}

			// Close drains the buffer synchronously — no sleep needed
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
			emitter.Start()

			emitter.Emit(context.Background(), &audit.Event{
				Type: audit.EventA2ATaskFailed,
			})

			// Close drains the buffer synchronously — no sleep needed
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
			emitter.Start()

			err := emitter.Close(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(writer.eventCount()).To(Equal(0))
			emitter = nil
		})
	})
})
