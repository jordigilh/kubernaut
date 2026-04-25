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

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
)

var _ = Describe("Manager Shutdown — #823 Hardening", func() {

	Describe("UT-KA-823-SD01: Shutdown cancels running investigations", func() {
		It("transitions running sessions to cancelled and fires cancel func", func() {
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, slog.Default(), audit.NopAuditStore{})

			cancelled := make(chan struct{})
			id, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				<-ctx.Done()
				close(cancelled)
				return nil, ctx.Err()
			}, map[string]string{"remediation_id": "rr-shutdown-test"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() string {
				s, _ := mgr.GetSession(id)
				return string(s.Status)
			}, 2*time.Second).Should(Equal("running"))

			mgr.Shutdown()

			Eventually(cancelled, 5*time.Second).Should(BeClosed(),
				"investigation context should be cancelled by Shutdown")

			Eventually(func() string {
				s, _ := mgr.GetSession(id)
				return string(s.Status)
			}, 5*time.Second).Should(SatisfyAny(
				Equal("cancelled"), Equal("failed"),
			))
		})
	})

	Describe("UT-KA-823-SD02: Shutdown is idempotent", func() {
		It("can be called multiple times without panic", func() {
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, slog.Default(), audit.NopAuditStore{})

			Expect(func() {
				mgr.Shutdown()
				mgr.Shutdown()
			}).NotTo(Panic())
		})
	})

	Describe("UT-KA-823-SD03: Shutdown skips already-terminal sessions", func() {
		It("does not affect completed/failed sessions", func() {
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, slog.Default(), audit.NopAuditStore{})

			done := make(chan string, 1)
			id, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				return "completed-result", nil
			}, map[string]string{"remediation_id": "rr-terminal-test"})
			Expect(err).NotTo(HaveOccurred())
			done <- id

			Eventually(func() string {
				s, _ := mgr.GetSession(id)
				return string(s.Status)
			}, 5*time.Second).Should(Equal("completed"))

			mgr.Shutdown()

			s, getErr := mgr.GetSession(id)
			Expect(getErr).NotTo(HaveOccurred())
			Expect(s.Status).To(Equal(session.StatusCompleted), "completed session should not be affected by Shutdown")
		})
	})
})
