package streaming_test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/streaming"
)

var _ = Describe("ConnectionTracker", func() {
	var tracker *streaming.ConnectionTracker

	BeforeEach(func() {
		tracker = streaming.NewConnectionTracker(nil, 0)
	})

	Describe("Add/Remove/Count", func() {
		It("tracks connections correctly", func() {
			Expect(tracker.Count()).To(Equal(0))

			_, cancel1 := context.WithCancel(context.Background())
			tracker.Add(&streaming.TrackedConnection{
				ID:     "conn-1",
				Writer: httptest.NewRecorder(),
				Cancel: cancel1,
			})
			Expect(tracker.Count()).To(Equal(1))

			_, cancel2 := context.WithCancel(context.Background())
			tracker.Add(&streaming.TrackedConnection{
				ID:     "conn-2",
				Writer: httptest.NewRecorder(),
				Cancel: cancel2,
			})
			Expect(tracker.Count()).To(Equal(2))

			tracker.Remove("conn-1")
			Expect(tracker.Count()).To(Equal(1))

			tracker.Remove("conn-2")
			Expect(tracker.Count()).To(Equal(0))
		})

		It("is safe for concurrent access", func() {
			var wg sync.WaitGroup
			for i := 0; i < 100; i++ {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()
					ctx, cancel := context.WithCancel(context.Background())
					_ = ctx
					tracker.Add(&streaming.TrackedConnection{
						ID:     connID(id),
						Writer: httptest.NewRecorder(),
						Cancel: cancel,
					})
				}(i)
			}
			wg.Wait()
			Expect(tracker.Count()).To(Equal(100))

			for i := 0; i < 100; i++ {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()
					tracker.Remove(connID(id))
				}(i)
			}
			wg.Wait()
			Expect(tracker.Count()).To(Equal(0))
		})
	})

	Describe("DrainAll", func() {
		It("closes all connections and returns force-closed count", func() {
			cancelled := make([]bool, 3)
			for i := 0; i < 3; i++ {
				idx := i
				_, cancel := context.WithCancel(context.Background())
				wrappedCancel := func() {
					cancelled[idx] = true
					cancel()
				}
				tracker.Add(&streaming.TrackedConnection{
					ID:     connID(i),
					Writer: httptest.NewRecorder(),
					Cancel: wrappedCancel,
				})
			}

			forceClosed := tracker.DrainAll(context.Background())
			Expect(forceClosed).To(Equal(3))
			Expect(tracker.Count()).To(Equal(0))
			for _, c := range cancelled {
				Expect(c).To(BeTrue())
			}
		})

		It("handles empty tracker gracefully", func() {
			forceClosed := tracker.DrainAll(context.Background())
			Expect(forceClosed).To(Equal(0))
		})
	})
})

var _ = Describe("ConnectionTracker — STREAM-04 Cap Enforcement (DOS-01, SC-5)", func() {
	It("UT-AF-STREAM04-001: Add returns false when MaxConnections is reached", func() {
		tracker := streaming.NewConnectionTracker(nil, 0)
		tracker.MaxConnections = 2

		_, c1 := context.WithCancel(context.Background())
		ok1 := tracker.Add(&streaming.TrackedConnection{ID: "c1", Writer: httptest.NewRecorder(), Cancel: c1})
		Expect(ok1).To(BeTrue())

		_, c2 := context.WithCancel(context.Background())
		ok2 := tracker.Add(&streaming.TrackedConnection{ID: "c2", Writer: httptest.NewRecorder(), Cancel: c2})
		Expect(ok2).To(BeTrue())

		_, c3 := context.WithCancel(context.Background())
		ok3 := tracker.Add(&streaming.TrackedConnection{ID: "c3", Writer: httptest.NewRecorder(), Cancel: c3})
		Expect(ok3).To(BeFalse(), "third connection must be rejected when cap is 2")

		Expect(tracker.Count()).To(Equal(2))
	})

	It("UT-AF-STREAM04-002: Add succeeds again after Remove frees a slot", func() {
		tracker := streaming.NewConnectionTracker(nil, 0)
		tracker.MaxConnections = 1

		_, c1 := context.WithCancel(context.Background())
		Expect(tracker.Add(&streaming.TrackedConnection{ID: "c1", Writer: httptest.NewRecorder(), Cancel: c1})).To(BeTrue())

		_, c2 := context.WithCancel(context.Background())
		Expect(tracker.Add(&streaming.TrackedConnection{ID: "c2", Writer: httptest.NewRecorder(), Cancel: c2})).To(BeFalse())

		tracker.Remove("c1")

		_, c3 := context.WithCancel(context.Background())
		Expect(tracker.Add(&streaming.TrackedConnection{ID: "c3", Writer: httptest.NewRecorder(), Cancel: c3})).To(BeTrue())
		Expect(tracker.Count()).To(Equal(1))
	})
})

func connID(i int) string {
	return fmt.Sprintf("conn-%d", i)
}
