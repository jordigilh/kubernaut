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

package tools_test

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("kubernaut_watch session event routing - #1637", func() {
	It("IT-AF-1637-005: subscribes while watching and unsubscribes when the watch returns", func() {
		rr := newTypedRR("payments", "rr-router-001", "Executing")
		client := newWatchClient(rr)
		session := &mockPoolSession{}
		pool := ka.NewKASessionPool(ka.PoolConfig{
			Factory: func(_ context.Context) (ka.PoolSession, error) {
				return session, nil
			},
			MaxEntries: 10,
		})
		router, err := pool.InjectVerified(context.Background(), "rr-router-001", "alice", session)
		Expect(err).NotTo(HaveOccurred())

		queue := &bridgeQueue{}
		baseCtx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{Username: "alice"})
		ctx, cancel := context.WithCancel(launcher.WithEventBridge(baseCtx, queue, "task-1637-005", "ctx-1637-005", nil))
		defer cancel()

		resultCh := make(chan tools.WatchResult, 1)
		errorCh := make(chan error, 1)
		go func() {
			result, watchErr := tools.HandleWatchWithPool(ctx, client, tools.WatchArgs{Namespace: "payments", RRID: "rr-router-001"}, pool)
			resultCh <- result
			errorCh <- watchErr
		}()

		Eventually(func() int {
			return router.Publish(ka.InvestigationEvent{
				Type: ka.EventTypeReasoningContentDelta,
				Data: json.RawMessage(`{"text":"KA still processing","redacted":false}`),
			})
		}, 3*time.Second, 10*time.Millisecond).Should(Equal(1),
			"IT-AF-1637-005: an active kubernaut_watch call must be a session event subscriber")

		Eventually(func() []a2a.Event { return queue.Events() }, 3*time.Second, 10*time.Millisecond).ShouldNot(BeEmpty())
		cancel()
		Eventually(resultCh, 3*time.Second).Should(Receive())
		Expect(<-errorCh).NotTo(HaveOccurred())

		Expect(router.Publish(ka.InvestigationEvent{Type: ka.EventTypeReasoningContentDelta})).To(Equal(0),
			"IT-AF-1637-005: watch completion must remove its session subscription")
	})
})
