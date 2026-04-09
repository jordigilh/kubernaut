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
	"log/slog"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
)

var _ = Describe("Session Metadata — #433", func() {

	Describe("UT-KA-433-METADATA-001: StartInvestigation propagates metadata to session", func() {
		It("should store metadata on the session and make it retrievable", func() {
			store := session.NewStore(5 * time.Minute)
			manager := session.NewManager(store, slog.Default())

			metadata := map[string]string{
				"incident_id": "e2e-ka-001-oom",
			}

			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				time.Sleep(50 * time.Millisecond)
				return "done", nil
			}, metadata)
			Expect(err).NotTo(HaveOccurred())
			Expect(id).NotTo(BeEmpty())

			sess, err := manager.GetSession(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Metadata).NotTo(BeNil(), "session metadata should not be nil")
			Expect(sess.Metadata["incident_id"]).To(Equal("e2e-ka-001-oom"))
		})
	})

	Describe("UT-KA-433-METADATA-002: Metadata persists after investigation completes", func() {
		It("should retain metadata on a completed session", func() {
			store := session.NewStore(5 * time.Minute)
			manager := session.NewManager(store, slog.Default())

			metadata := map[string]string{
				"incident_id": "e2e-ka-002-crash",
			}

			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				return "result", nil
			}, metadata)
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
			Expect(sess.Metadata).NotTo(BeNil(), "metadata should persist after completion")
			Expect(sess.Metadata["incident_id"]).To(Equal("e2e-ka-002-crash"))
		})
	})

	Describe("UT-KA-433-METADATA-003: Nil metadata does not cause issues", func() {
		It("should handle nil metadata gracefully", func() {
			store := session.NewStore(5 * time.Minute)
			manager := session.NewManager(store, slog.Default())

			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				return "result", nil
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
			Expect(sess.Status).To(Equal(session.StatusCompleted))
		})
	})
})
