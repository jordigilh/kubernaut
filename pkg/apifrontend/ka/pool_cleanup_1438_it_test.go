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
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
)

var _ = Describe("IT-AF-1438-040 (SI-4): Pool Release closes done channel, watcher exits deterministically", func() {

	It("should close done channel on Release so a goroutine listening on it exits", func() {
		pool := ka.NewKASessionPool(ka.PoolConfig{
			Factory: func(ctx context.Context) (ka.PoolSession, error) {
				return &mockPoolSession{}, nil
			},
			MaxEntries: 10,
			Logger:     logr.Discard(),
		})

		done := make(chan struct{})
		pool.InjectWithCleanup("rr-it-1438-040", "alice", &mockPoolSession{id: 1}, func() {
			close(done)
		})

		var watcherExited atomic.Bool
		go func() {
			select {
			case <-done:
				watcherExited.Store(true)
			case <-time.After(5 * time.Second):
			}
		}()

		time.Sleep(50 * time.Millisecond)
		Expect(watcherExited.Load()).To(BeFalse(),
			"watcher must NOT exit before pool Release")

		pool.Release("rr-it-1438-040", "alice")

		Eventually(func() bool {
			return watcherExited.Load()
		}, 2*time.Second).Should(BeTrue(),
			"SI-4: watcher goroutine must exit deterministically when pool Release closes done channel")
	})
})
