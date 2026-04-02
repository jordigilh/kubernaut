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
