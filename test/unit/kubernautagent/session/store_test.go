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
	"context"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
)

var _ = Describe("Kubernaut Agent Session Store — #433", func() {

	Describe("UT-KA-433-006: Session store creates and retrieves session", func() {
		It("should create a session and retrieve it by ID", func() {
			store := session.NewStore(30 * time.Minute)
			id, err := store.Create()
			Expect(err).NotTo(HaveOccurred())
			Expect(id).NotTo(BeEmpty(), "session ID should not be empty")

			sess, err := store.Get(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess).NotTo(BeNil())
			Expect(sess.ID).To(Equal(id))
			Expect(sess.Status).To(Equal(session.StatusPending))
		})
	})

	Describe("UT-KA-433-007: Session store returns not-found for unknown ID", func() {
		It("should return ErrSessionNotFound for non-existent session", func() {
			store := session.NewStore(30 * time.Minute)
			sess, err := store.Get("nonexistent-id")
			Expect(err).To(MatchError(session.ErrSessionNotFound))
			Expect(sess).To(BeNil())
		})
	})

	Describe("UT-KA-433-008: Session store TTL cleanup removes expired sessions", func() {
		It("should remove sessions beyond TTL", func() {
			store := session.NewStore(1 * time.Millisecond)
			id, err := store.Create()
			Expect(err).NotTo(HaveOccurred())
			Expect(id).NotTo(BeEmpty())

			time.Sleep(5 * time.Millisecond)

			removed := store.Cleanup()
			Expect(removed).To(BeNumerically(">=", 1), "at least 1 expired session should be cleaned up")

			_, err = store.Get(id)
			Expect(err).To(MatchError(session.ErrSessionNotFound))
		})
	})

	Describe("UT-KA-HARDEN-001: StartCleanupLoop removes expired sessions automatically", func() {
		It("should remove expired sessions after the cleanup interval fires", func() {
			store := session.NewStore(1 * time.Millisecond)
			id, err := store.Create()
			Expect(err).NotTo(HaveOccurred())

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			store.StartCleanupLoop(ctx, 5*time.Millisecond)

			time.Sleep(20 * time.Millisecond)

			_, err = store.Get(id)
			Expect(err).To(MatchError(session.ErrSessionNotFound),
				"expired session should be removed by cleanup loop")
		})

		It("should stop the cleanup loop when context is cancelled", func() {
			store := session.NewStore(1 * time.Hour)
			ctx, cancel := context.WithCancel(context.Background())
			store.StartCleanupLoop(ctx, 5*time.Millisecond)
			cancel()

			time.Sleep(15 * time.Millisecond)

			id, err := store.Create()
			Expect(err).NotTo(HaveOccurred())
			sess, err := store.Get(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess).NotTo(BeNil())
		})
	})

	Describe("UT-KA-HARDEN-002: Get returns a deep copy of Metadata", func() {
		It("should not allow callers to mutate the stored session metadata", func() {
			store := session.NewStore(30 * time.Minute)
			id, err := store.Create()
			Expect(err).NotTo(HaveOccurred())
			store.SetMetadata(id, map[string]string{"key": "original"})

			snap, err := store.Get(id)
			Expect(err).NotTo(HaveOccurred())
			snap.Metadata["key"] = "mutated"

			snap2, err := store.Get(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(snap2.Metadata["key"]).To(Equal("original"),
				"mutation of returned snapshot must not affect the stored session")
		})
	})

	Describe("UT-KA-433-009: Session store concurrent access is data-race-free", func() {
		It("should handle concurrent Create/Get/Update without data races", func() {
			store := session.NewStore(30 * time.Minute)
			const goroutines = 20
			var wg sync.WaitGroup
			wg.Add(goroutines)

			for i := 0; i < goroutines; i++ {
				go func() {
					defer wg.Done()
					id, err := store.Create()
					if err != nil {
						return
					}
					_, _ = store.Get(id)
					_ = store.Update(id, session.StatusRunning, nil, nil)
					_, _ = store.Get(id)
				}()
			}

			wg.Wait()
		})
	})
})
