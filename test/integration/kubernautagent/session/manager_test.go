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
	"errors"
	"log/slog"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
)

var _ = Describe("Kubernaut Agent Session Manager — #433", func() {

	var (
		store   *session.Store
		manager *session.Manager
	)

	BeforeEach(func() {
		store = session.NewStore(5 * time.Minute)
		manager = session.NewManager(store, slog.Default())
	})

	Describe("IT-KA-433-001: Session manager starts background investigation", func() {
		It("should return a session ID immediately", func() {
			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				time.Sleep(100 * time.Millisecond)
				return "result", nil
			}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(id).NotTo(BeEmpty(), "session ID should be returned immediately")
		})
	})

	Describe("IT-KA-433-002: Session manager reports in-progress status", func() {
		It("should show running status while investigation is active", func() {
			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				time.Sleep(200 * time.Millisecond)
				return "done", nil
			}, nil)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, err := manager.GetSession(id)
				if err != nil {
					return ""
				}
				return sess.Status
			}, 1*time.Second, 10*time.Millisecond).Should(Equal(session.StatusRunning))
		})
	})

	Describe("IT-KA-433-003: Session manager delivers completed result", func() {
		It("should transition to completed with result after investigation finishes", func() {
			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				return map[string]string{"workflow_id": "oom-increase-memory"}, nil
			}, nil)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, err := manager.GetSession(id)
				if err != nil {
					return ""
				}
				return sess.Status
			}, 2*time.Second, 10*time.Millisecond).Should(Equal(session.StatusCompleted))

			sess, err := manager.GetSession(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Result).NotTo(BeNil())
			result, ok := sess.Result.(map[string]string)
			Expect(ok).To(BeTrue())
			Expect(result["workflow_id"]).To(Equal("oom-increase-memory"))
		})
	})

	Describe("IT-KA-433-004: Session manager captures investigation failure", func() {
		It("should transition to failed with error when investigation errors", func() {
			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				return nil, errors.New("LLM provider unavailable")
			}, nil)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, err := manager.GetSession(id)
				if err != nil {
					return ""
				}
				return sess.Status
			}, 2*time.Second, 10*time.Millisecond).Should(Equal(session.StatusFailed))

			sess, err := manager.GetSession(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Error).To(HaveOccurred())
			Expect(sess.Error.Error()).To(ContainSubstring("LLM provider unavailable"))
		})
	})
})

var _ = Describe("Session Cancellation Infrastructure — #823", func() {

	var (
		store   *session.Store
		manager *session.Manager
	)

	BeforeEach(func() {
		store = session.NewStore(5 * time.Minute)
		manager = session.NewManager(store, slog.Default())
	})

	Describe("IT-KA-823-001: Cancelling an investigation stops the running LLM analysis", func() {
		It("should propagate cancellation and transition to cancelled", func() {
			cancelled := make(chan struct{})

			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				<-ctx.Done()
				close(cancelled)
				return nil, ctx.Err()
			}, nil)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, _ := manager.GetSession(id)
				if sess == nil {
					return ""
				}
				return sess.Status
			}, 1*time.Second, 10*time.Millisecond).Should(Equal(session.StatusRunning))

			err = manager.CancelInvestigation(id)
			Expect(err).NotTo(HaveOccurred())

			Eventually(cancelled, 2*time.Second).Should(BeClosed(),
				"investigation function should receive context cancellation")

			Eventually(func() session.Status {
				sess, _ := manager.GetSession(id)
				if sess == nil {
					return ""
				}
				return sess.Status
			}, 2*time.Second, 10*time.Millisecond).Should(Equal(session.StatusCancelled))
		})
	})

	Describe("IT-KA-823-002: Cancelling a nonexistent investigation returns a clear error", func() {
		It("should return ErrSessionNotFound for unknown session ID", func() {
			err := manager.CancelInvestigation("nonexistent-id")
			Expect(err).To(MatchError(session.ErrSessionNotFound))
		})
	})

	Describe("IT-KA-823-003: Cancelling an already-completed investigation returns a clear error", func() {
		It("should return ErrSessionTerminal for a completed session", func() {
			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				return "result", nil
			}, nil)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, _ := manager.GetSession(id)
				if sess == nil {
					return ""
				}
				return sess.Status
			}, 2*time.Second, 10*time.Millisecond).Should(Equal(session.StatusCompleted))

			err = manager.CancelInvestigation(id)
			Expect(err).To(MatchError(session.ErrSessionTerminal))
		})
	})

	Describe("IT-KA-823-004: After cancellation, the investigation cannot retroactively report failure", func() {
		It("should keep StatusCancelled even when goroutine returns an error", func() {
			started := make(chan struct{})

			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				close(started)
				<-ctx.Done()
				return nil, errors.New("investigation aborted")
			}, nil)
			Expect(err).NotTo(HaveOccurred())

			Eventually(started, 1*time.Second).Should(BeClosed())
			Eventually(func() session.Status {
				sess, _ := manager.GetSession(id)
				if sess == nil {
					return ""
				}
				return sess.Status
			}, 1*time.Second, 10*time.Millisecond).Should(Equal(session.StatusRunning))

			err = manager.CancelInvestigation(id)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, _ := manager.GetSession(id)
				if sess == nil {
					return ""
				}
				return sess.Status
			}, 2*time.Second, 10*time.Millisecond).Should(Equal(session.StatusCancelled))

			Consistently(func() session.Status {
				sess, _ := manager.GetSession(id)
				if sess == nil {
					return ""
				}
				return sess.Status
			}, 200*time.Millisecond, 20*time.Millisecond).Should(Equal(session.StatusCancelled),
				"status must remain cancelled, not transition to failed")
		})
	})

	Describe("IT-KA-823-005: An observer can receive live events from an active investigation", func() {
		It("should return a readable event channel for a running investigation", func() {
			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}, nil)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, _ := manager.GetSession(id)
				if sess == nil {
					return ""
				}
				return sess.Status
			}, 1*time.Second, 10*time.Millisecond).Should(Equal(session.StatusRunning))

			ch, err := manager.Subscribe(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(ch).NotTo(BeNil(), "event channel must be non-nil for a running investigation")

			Expect(manager.CancelInvestigation(id)).To(Succeed())
		})
	})

	Describe("IT-KA-823-006: Multiple subscriptions share a single event stream", func() {
		It("should return the same channel on repeated subscribe calls", func() {
			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}, nil)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, _ := manager.GetSession(id)
				if sess == nil {
					return ""
				}
				return sess.Status
			}, 1*time.Second, 10*time.Millisecond).Should(Equal(session.StatusRunning))

			ch1, err := manager.Subscribe(id)
			Expect(err).NotTo(HaveOccurred())
			ch2, err := manager.Subscribe(id)
			Expect(err).NotTo(HaveOccurred())

			Expect(ch1).To(BeIdenticalTo(ch2),
				"repeated subscriptions must return the same channel")

			Expect(manager.CancelInvestigation(id)).To(Succeed())
		})
	})

	Describe("IT-KA-823-007: Observers are notified when an investigation concludes", func() {
		It("should close the event channel when the investigation function returns", func() {
			proceed := make(chan struct{})

			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				<-proceed
				return "done", nil
			}, nil)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, _ := manager.GetSession(id)
				if sess == nil {
					return ""
				}
				return sess.Status
			}, 1*time.Second, 10*time.Millisecond).Should(Equal(session.StatusRunning))

			ch, err := manager.Subscribe(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(ch).NotTo(BeNil())

			close(proceed)

			Eventually(func() bool {
				select {
				case _, ok := <-ch:
					return !ok
				default:
					return false
				}
			}, 2*time.Second, 10*time.Millisecond).Should(BeTrue(),
				"event channel should be closed when investigation completes")
		})
	})

	Describe("IT-KA-823-008: Subscribing to a nonexistent investigation returns a clear error", func() {
		It("should return ErrSessionNotFound for unknown session ID", func() {
			ch, err := manager.Subscribe("nonexistent-id")
			Expect(err).To(MatchError(session.ErrSessionNotFound))
			Expect(ch).To(BeNil())
		})
	})

	Describe("IT-KA-823-009: Subscribing to a concluded investigation returns a clear error", func() {
		It("should return ErrSessionTerminal after the investigation has completed", func() {
			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				return "result", nil
			}, nil)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, _ := manager.GetSession(id)
				if sess == nil {
					return ""
				}
				return sess.Status
			}, 2*time.Second, 10*time.Millisecond).Should(Equal(session.StatusCompleted))

			ch, err := manager.Subscribe(id)
			Expect(err).To(MatchError(session.ErrSessionTerminal))
			Expect(ch).To(BeNil())
		})
	})
})
