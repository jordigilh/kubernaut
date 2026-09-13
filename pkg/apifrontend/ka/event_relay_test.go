/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ka_test

import (
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
)

// #1637 / DD-AF-015: EventRouter fans one pooled session's event stream out to
// explicitly registered subscribers. The router is session-scoped by the
// KASessionPool entry, so subscribers cannot cross the existing (rr_id,
// username) isolation boundary.
var _ = Describe("EventRouter — #1637", func() {

	It("UT-AF-1637-001: Publish does nothing when there are no subscribers", func() {
		router := ka.NewEventRouter()

		Expect(router.Publish(ka.InvestigationEvent{Type: ka.EventTypeReasoningDelta})).To(Equal(0),
			"an idle EventRouter must not report delivery")
	})

	It("UT-AF-1637-001: Subscribe receives published events and unsubscribe stops delivery", func() {
		router := ka.NewEventRouter()
		var received atomic.Int32

		unsubscribe := router.Subscribe(func(evt ka.InvestigationEvent) {
			if evt.Type == ka.EventTypeReasoningDelta {
				received.Add(1)
			}
		})

		Expect(router.Publish(ka.InvestigationEvent{Type: ka.EventTypeReasoningDelta})).To(Equal(1))
		Expect(received.Load()).To(Equal(int32(1)))

		unsubscribe()
		Expect(router.Publish(ka.InvestigationEvent{Type: ka.EventTypeReasoningDelta})).To(Equal(0))
		Expect(received.Load()).To(Equal(int32(1)),
			"unsubscribed sinks must not receive later events")
	})

	It("UT-AF-1637-001: supports multiple subscribers without last-writer-wins routing", func() {
		router := ka.NewEventRouter()
		var first, second atomic.Int32

		unsubscribeFirst := router.Subscribe(func(ka.InvestigationEvent) { first.Add(1) })
		defer unsubscribeFirst()
		unsubscribeSecond := router.Subscribe(func(ka.InvestigationEvent) { second.Add(1) })
		defer unsubscribeSecond()

		Expect(router.Publish(ka.InvestigationEvent{Type: ka.EventTypeToolCallStart})).To(Equal(2))
		Expect(first.Load()).To(Equal(int32(1)))
		Expect(second.Load()).To(Equal(int32(1)))
	})

	It("UT-AF-1637-001: concurrent subscribe, publish, and unsubscribe do not race", func() {
		router := ka.NewEventRouter()
		done := make(chan struct{})
		go func() {
			defer close(done)
			for i := 0; i < 200; i++ {
				unsubscribe := router.Subscribe(func(ka.InvestigationEvent) {})
				router.Publish(ka.InvestigationEvent{Type: ka.EventTypeReasoningDelta})
				unsubscribe()
			}
		}()
		for i := 0; i < 200; i++ {
			router.Publish(ka.InvestigationEvent{Type: ka.EventTypeReasoningDelta})
		}
		Eventually(done, 3*time.Second).Should(BeClosed())
	})
})
