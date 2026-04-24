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
	"reflect"
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

var _ = Describe("Session Cancellation Infrastructure — #823", func() {

	Describe("UT-KA-823-001: A completed investigation is immutable", func() {
		It("should reject status changes on a completed investigation", func() {
			store := session.NewStore(30 * time.Minute)
			id, err := store.Create()
			Expect(err).NotTo(HaveOccurred())
			Expect(store.Update(id, session.StatusRunning, nil, nil)).To(Succeed())
			Expect(store.Update(id, session.StatusCompleted, "done", nil)).To(Succeed())

			err = store.Update(id, session.StatusRunning, nil, nil)
			Expect(err).To(MatchError(session.ErrSessionTerminal))

			sess, err := store.Get(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Status).To(Equal(session.StatusCompleted))
		})
	})

	Describe("UT-KA-823-002: A cancelled investigation is immutable", func() {
		It("should reject status changes on a cancelled investigation", func() {
			store := session.NewStore(30 * time.Minute)
			id, err := store.Create()
			Expect(err).NotTo(HaveOccurred())
			Expect(store.Update(id, session.StatusRunning, nil, nil)).To(Succeed())
			Expect(store.Update(id, session.StatusCancelled, nil, nil)).To(Succeed())

			err = store.Update(id, session.StatusFailed, nil, nil)
			Expect(err).To(MatchError(session.ErrSessionTerminal))

			sess, err := store.Get(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Status).To(Equal(session.StatusCancelled))
		})
	})

	Describe("UT-KA-823-003: A failed investigation is immutable", func() {
		It("should reject status changes on a failed investigation", func() {
			store := session.NewStore(30 * time.Minute)
			id, err := store.Create()
			Expect(err).NotTo(HaveOccurred())
			Expect(store.Update(id, session.StatusRunning, nil, nil)).To(Succeed())
			Expect(store.Update(id, session.StatusFailed, nil, nil)).To(Succeed())

			err = store.Update(id, session.StatusCompleted, "late-result", nil)
			Expect(err).To(MatchError(session.ErrSessionTerminal))

			sess, err := store.Get(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Status).To(Equal(session.StatusFailed))
		})
	})

	Describe("UT-KA-823-004: An active investigation can be cancelled by an operator", func() {
		It("should accept cancellation of a running investigation", func() {
			store := session.NewStore(30 * time.Minute)
			id, err := store.Create()
			Expect(err).NotTo(HaveOccurred())
			Expect(store.Update(id, session.StatusRunning, nil, nil)).To(Succeed())

			err = store.Update(id, session.StatusCancelled, nil, nil)
			Expect(err).NotTo(HaveOccurred())

			sess, err := store.Get(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Status).To(Equal(session.StatusCancelled))
		})
	})

	Describe("UT-KA-823-005: Querying a cancelled investigation reports the cancelled state", func() {
		It("should return StatusCancelled for a cancelled investigation", func() {
			store := session.NewStore(30 * time.Minute)
			id, err := store.Create()
			Expect(err).NotTo(HaveOccurred())
			Expect(store.Update(id, session.StatusRunning, nil, nil)).To(Succeed())
			Expect(store.Update(id, session.StatusCancelled, nil, nil)).To(Succeed())

			sess, err := store.Get(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Status).To(Equal(session.StatusCancelled))
		})
	})

	Describe("UT-KA-823-006: Active investigations are never removed by housekeeping", func() {
		It("should skip running sessions during cleanup even if TTL expired", func() {
			store := session.NewStore(1 * time.Millisecond)
			id, err := store.Create()
			Expect(err).NotTo(HaveOccurred())
			Expect(store.Update(id, session.StatusRunning, nil, nil)).To(Succeed())

			time.Sleep(5 * time.Millisecond)

			removed := store.Cleanup()
			Expect(removed).To(Equal(0))

			sess, err := store.Get(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Status).To(Equal(session.StatusRunning))
		})
	})

	Describe("UT-KA-823-007: Session metadata cannot interfere with active investigations", func() {
		It("should not expose internal control fields in the returned session copy", func() {
			store := session.NewStore(30 * time.Minute)
			id, err := store.Create()
			Expect(err).NotTo(HaveOccurred())
			Expect(store.Update(id, session.StatusRunning, nil, nil)).To(Succeed())

			sess, err := store.Get(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess).NotTo(BeNil())

			v := reflect.ValueOf(*sess)
			cancelField := v.FieldByName("cancel")
			eventChanField := v.FieldByName("eventChan")
			Expect(cancelField.IsValid()).To(BeTrue(), "cancel field must exist on Session struct")
			Expect(eventChanField.IsValid()).To(BeTrue(), "eventChan field must exist on Session struct")
			Expect(cancelField.IsNil()).To(BeTrue(), "cancel must be nil on cloned session")
			Expect(eventChanField.IsNil()).To(BeTrue(), "eventChan must be nil on cloned session")
		})
	})

	Describe("UT-KA-823-008: SetResult attaches result without status change (RR-2)", func() {
		It("should set the result on a cancelled session without changing status", func() {
			store := session.NewStore(30 * time.Minute)
			id, err := store.Create()
			Expect(err).NotTo(HaveOccurred())
			Expect(store.Update(id, session.StatusCancelled, nil, nil)).To(Succeed())

			partialResult := map[string]string{"rca_summary": "partial"}
			store.SetResult(id, partialResult)

			sess, err := store.Get(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Status).To(Equal(session.StatusCancelled), "status must remain cancelled")
			Expect(sess.Result).To(Equal(partialResult), "result must be attached")
		})

		It("should be a no-op for non-existent session", func() {
			store := session.NewStore(30 * time.Minute)
			Expect(func() { store.SetResult("nonexistent", "data") }).NotTo(Panic())
		})
	})
})
