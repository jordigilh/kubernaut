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

var _ = Describe("session.Manager.FindByRemediationID — BR-INTERACTIVE-004", func() {

	Describe("UT-KA-FIND-001: FindByRemediationID returns running session for given rrID", func() {
		It("should locate a running session by its remediation_id metadata", func() {
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, slog.Default(), nil, nil)

			// Start an investigation with remediation_id in metadata
			sessionID, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				// Block until cancelled
				<-ctx.Done()
				return nil, ctx.Err()
			}, map[string]string{
				"remediation_id": "rr-test-001",
				"signal_name":    "OOMKilled",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(sessionID).NotTo(BeEmpty())

			// Give goroutine time to transition to running
			Eventually(func() session.Status {
				sess, _ := mgr.GetSession(sessionID)
				if sess == nil {
					return ""
				}
				return sess.Status
			}).Should(Equal(session.StatusRunning))

			// FindByRemediationID should locate the session
			foundID, ok := mgr.FindByRemediationID("rr-test-001")
			Expect(ok).To(BeTrue(), "should find session by rrID")
			Expect(foundID).To(Equal(sessionID))

			// Non-existent rrID returns false
			_, notOK := mgr.FindByRemediationID("rr-nonexistent")
			Expect(notOK).To(BeFalse())

			// Cleanup
			_ = mgr.CancelInvestigation(sessionID)
		})
	})
})
