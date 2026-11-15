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

package notification

import (
	"sync"
	"sync/atomic"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestNotificationControllerUnit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Notification Controller Unit Suite")
}

var _ = Describe("Notification reconcile lock manager (DD-NOT-008)", func() {
	It("UT-NOT-008-001: serializes same-key reconciles and cleans idle locks", func() {
		manager := notificationReconcileLockManager{}
		var active atomic.Int32
		var sameKeyOverlap atomic.Bool
		ready := make(chan struct{}, 2)
		entered := make(chan struct{}, 2)
		release := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)

		for i := 0; i < 2; i++ {
			go func() {
				defer wg.Done()
				ready <- struct{}{}

				unlock := manager.lock("same-key")
				if active.Add(1) > 1 {
					sameKeyOverlap.Store(true)
				}
				entered <- struct{}{}
				<-release
				active.Add(-1)
				unlock()
			}()
		}

		Eventually(func() int { return len(ready) }).Should(Equal(2))
		Eventually(func() int {
			manager.mu.Lock()
			defer manager.mu.Unlock()
			if lock, exists := manager.locks["same-key"]; exists {
				return lock.refs
			}
			return 0
		}).Should(Equal(2))
		Expect(entered).To(HaveLen(1), "same-key critical sections must be serialized")
		Expect(sameKeyOverlap.Load()).To(BeFalse())

		close(release)
		wg.Wait()
		Expect(manager.locks).To(BeEmpty(), "all idle lock entries must be removed")
	})

	It("UT-NOT-008-002: allows different-key reconciles to run concurrently", func() {
		manager := notificationReconcileLockManager{}
		var active atomic.Int32
		entered := make(chan struct{}, 2)
		release := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)

		for _, key := range []string{"first-key", "second-key"} {
			go func(key string) {
				defer wg.Done()
				unlock := manager.lock(key)
				active.Add(1)
				entered <- struct{}{}
				<-release
				active.Add(-1)
				unlock()
			}(key)
		}

		Eventually(func() int { return len(entered) }).Should(Equal(2),
			"different-key critical sections must not block each other")
		Expect(active.Load()).To(Equal(int32(2)))

		close(release)
		wg.Wait()
		Expect(manager.locks).To(BeEmpty(), "all idle lock entries must be removed")
	})
})
