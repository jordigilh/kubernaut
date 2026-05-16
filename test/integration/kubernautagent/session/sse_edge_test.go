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
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

var _ = Describe("SSE Edge Cases — GAP-7 / BR-SESSION-002", Label("integration", "sse"), func() {

	// ---------------------------------------------------------------
	// IT-KA-SSE-004: Subscribe after terminal returns ErrSessionTerminal
	//
	// BR: BR-SESSION-002
	// The handler maps ErrSessionTerminal to 404 with
	// type="session-terminal" and detail containing "use the snapshot endpoint".
	// This test validates the session manager level (handler mapping tested at E2E).
	// ---------------------------------------------------------------
	Describe("IT-KA-SSE-004: Subscribe after terminal returns ErrSessionTerminal", func() {
		It("should return ErrSessionTerminal when subscribing to completed session", func() {
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, logr.Discard(), audit.NopAuditStore{}, nil)

			By("Starting and completing an investigation")
			sessionID, err := mgr.StartInvestigation(context.Background(), func(_ context.Context) (*katypes.InvestigationResult, error) {
				return &katypes.InvestigationResult{RCASummary: "completed result"}, nil
			}, map[string]string{"remediation_id": "rr-sse-edge-001"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				s, _ := mgr.GetSession(sessionID)
				if s == nil {
					return ""
				}
				return s.Status
			}, 5*time.Second, 100*time.Millisecond).Should(Equal(session.StatusCompleted))

			By("Subscribing to the completed session")
			_, subErr := mgr.Subscribe(context.Background(), sessionID)
			Expect(subErr).To(MatchError(session.ErrSessionTerminal),
				"subscribing to a completed session must return ErrSessionTerminal")
		})

		It("should return ErrSessionTerminal when subscribing to cancelled session", func() {
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, logr.Discard(), audit.NopAuditStore{}, nil)

			By("Starting an investigation that blocks")
			sessionID, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}, map[string]string{"remediation_id": "rr-sse-edge-002"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				s, _ := mgr.GetSession(sessionID)
				if s == nil {
					return ""
				}
				return s.Status
			}, 5*time.Second, 100*time.Millisecond).Should(Equal(session.StatusRunning))

			By("Cancelling the investigation")
			Expect(mgr.CancelInvestigation(sessionID)).To(Succeed())

			Eventually(func() session.Status {
				s, _ := mgr.GetSession(sessionID)
				if s == nil {
					return ""
				}
				return s.Status
			}, 5*time.Second, 100*time.Millisecond).Should(Equal(session.StatusCancelled))

			By("Subscribing to the cancelled session")
			_, subErr := mgr.Subscribe(context.Background(), sessionID)
			Expect(subErr).To(MatchError(session.ErrSessionTerminal),
				"subscribing to a cancelled session must return ErrSessionTerminal")
		})
	})
})
