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
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
)

// #1637: EventRelay lets a pooled session's background event-watcher
// goroutine discover, for the duration of a specific pooled MCP call,
// which A2A call's context (and therefore EventBridge) should receive KA
// events arriving mid-call. See DD-AF-009.
var _ = Describe("EventRelay — #1637", func() {

	It("UT-AF-1637-001: Current returns nil when idle (no call attached)", func() {
		relay := &ka.EventRelay{}
		Expect(relay.Current()).To(BeNil(),
			"an EventRelay with no attached call must report idle (nil)")
	})

	It("UT-AF-1637-001: Attach records ctx, Current returns it while attached", func() {
		relay := &ka.EventRelay{}
		ctx := context.WithValue(context.Background(), ctxKeyTest{}, "call-1")

		detach := relay.Attach(ctx)
		Expect(relay.Current()).To(Equal(ctx),
			"Current must return the exact ctx passed to Attach")

		detach()
		Expect(relay.Current()).To(BeNil(),
			"Current must return nil after detach")
	})

	It("UT-AF-1637-001: an outer Attach is not clobbered by an inner call's stale detach", func() {
		relay := &ka.EventRelay{}
		outerCtx := context.WithValue(context.Background(), ctxKeyTest{}, "outer")
		innerCtx := context.WithValue(context.Background(), ctxKeyTest{}, "inner")

		relay.Attach(outerCtx)

		detachInner := relay.Attach(innerCtx)
		detachInner()
		Expect(relay.Current()).To(BeNil(),
			"inner detach clears Current since it was the last attach")

		// Re-attach outer to simulate the realistic nested-call ordering:
		// outer attaches, inner attaches+detaches, outer's own detach must
		// still only clear its own ctx (never someone else's).
		detachOuter := relay.Attach(outerCtx)
		detachInner = relay.Attach(innerCtx)
		detachOuter() // stale — outerCtx is no longer Current
		Expect(relay.Current()).To(Equal(innerCtx),
			"a stale outer detach firing after an inner Attach must not clobber the inner (current) ctx")

		detachInner()
		Expect(relay.Current()).To(BeNil())
	})

	It("UT-AF-1637-001: concurrent Attach/Current/detach do not race", func() {
		relay := &ka.EventRelay{}
		done := make(chan struct{})
		go func() {
			defer close(done)
			for i := 0; i < 200; i++ {
				ctx := context.WithValue(context.Background(), ctxKeyTest{}, i)
				detach := relay.Attach(ctx)
				_ = relay.Current()
				detach()
			}
		}()
		for i := 0; i < 200; i++ {
			_ = relay.Current()
		}
		Eventually(done, 3*time.Second).Should(BeClosed())
	})
})

type ctxKeyTest struct{}
