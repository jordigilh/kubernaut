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

package session_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
)

var _ = Describe("Two-Tier TTL Eviction — #1078", func() {

	Describe("UT-KA-1078-TTL-001: StatusRunning session NOT evicted at TTL", func() {
		It("should NOT evict a running session when CreatedAt + TTL has elapsed", func() {
			store := session.NewStore(1*time.Millisecond, session.WithMaxSessionAge(100*time.Millisecond))
			id, err := store.Create()
			Expect(err).NotTo(HaveOccurred())

			// Transition to Running
			Expect(store.Update(id, session.StatusRunning, nil, nil)).To(Succeed())

			// Wait for TTL to expire
			time.Sleep(5 * time.Millisecond)

			// Cleanup should NOT evict running session at TTL
			removed := store.Cleanup()
			Expect(removed).To(Equal(0), "running session must NOT be evicted at TTL")

			sess, err := store.Get(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Status).To(Equal(session.StatusRunning))
		})
	})

	Describe("UT-KA-1078-TTL-002: StatusRunning session IS evicted at MaxSessionAge", func() {
		It("should evict a running session when CreatedAt + MaxSessionAge has elapsed", func() {
			store := session.NewStore(1*time.Millisecond, session.WithMaxSessionAge(5*time.Millisecond))
			id, err := store.Create()
			Expect(err).NotTo(HaveOccurred())

			Expect(store.Update(id, session.StatusRunning, nil, nil)).To(Succeed())

			Eventually(func() int {
				return store.Cleanup()
			}, 100*time.Millisecond, time.Millisecond).Should(BeNumerically(">=", 1),
				"running session must be evicted after MaxSessionAge")

			_, err = store.Get(id)
			Expect(err).To(MatchError(session.ErrSessionNotFound))
		})
	})

	Describe("UT-KA-1078-TTL-003: StatusCompleted session IS evicted at TTL", func() {
		It("should evict a completed session at TTL (existing behavior preserved)", func() {
			store := session.NewStore(1*time.Millisecond, session.WithMaxSessionAge(100*time.Millisecond))
			id, err := store.Create()
			Expect(err).NotTo(HaveOccurred())

			Expect(store.Update(id, session.StatusCompleted, nil, nil)).To(Succeed())

			Eventually(func() int {
				return store.Cleanup()
			}, 100*time.Millisecond, time.Millisecond).Should(BeNumerically(">=", 1),
				"completed session must be evicted at TTL")

			_, err = store.Get(id)
			Expect(err).To(MatchError(session.ErrSessionNotFound))
		})
	})

	Describe("UT-KA-1078-TTL-004: StatusFailed session IS evicted at TTL", func() {
		It("should evict a failed session at TTL (existing behavior preserved)", func() {
			store := session.NewStore(1*time.Millisecond, session.WithMaxSessionAge(100*time.Millisecond))
			id, err := store.Create()
			Expect(err).NotTo(HaveOccurred())

			Expect(store.Update(id, session.StatusFailed, nil, nil)).To(Succeed())

			Eventually(func() int {
				return store.Cleanup()
			}, 100*time.Millisecond, time.Millisecond).Should(BeNumerically(">=", 1),
				"failed session must be evicted at TTL")

			_, err = store.Get(id)
			Expect(err).To(MatchError(session.ErrSessionNotFound))
		})
	})

	Describe("UT-KA-1078-TTL-005: MaxSessionAge < TTL config is rejected", func() {
		It("should reject MaxSessionAge smaller than TTL", func() {
			Expect(func() {
				session.NewStore(30*time.Minute, session.WithMaxSessionAge(10*time.Minute))
			}).To(Panic(), "MaxSessionAge < TTL must be rejected")
		})
	})
})
