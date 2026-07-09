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

// ============================================================================
// #1654: ForceCompleteByRemediationID must complete ALL non-terminal sibling
// sessions sharing the same remediation_id, not just the first one found.
//
// Root cause (E2E-FP-1456-001 RCA): MCP action=start (SC-24) can create a
// fallback interactive session for an rrID before AA's own autonomous
// investigation session for the same rrID transitions out of Running. Both
// sessions carry the same remediation_id metadata. When complete_no_action /
// select_workflow later force-completes "the" session for that rrID, only
// the first match in map iteration order was completed — leaving the other
// (the one AA is actually polling) stuck non-terminal until its own
// inactivity timer eventually fires minutes later.
// ============================================================================

var _ = Describe("session.Manager.ForceCompleteByRemediationID — #1654 sibling-session completion", func() {

	Describe("UT-KA-1654-B-001: completes every non-terminal session sharing the remediation_id", func() {
		It("should transition both sibling sessions to Completed, not just the first", func() {
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)

			doneA := make(chan struct{})
			idA, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-ctx.Done()
				close(doneA)
				return nil, ctx.Err()
			}, map[string]string{"remediation_id": "rr-1654-b-001", "mode": "interactive_fallback"})
			Expect(err).NotTo(HaveOccurred())

			doneB := make(chan struct{})
			idB, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-ctx.Done()
				close(doneB)
				return nil, ctx.Err()
			}, map[string]string{"remediation_id": "rr-1654-b-001"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				s, _ := mgr.GetSession(idA)
				if s == nil {
					return ""
				}
				return s.Status
			}, 2*time.Second, 20*time.Millisecond).Should(Equal(session.StatusRunning))
			Eventually(func() session.Status {
				s, _ := mgr.GetSession(idB)
				if s == nil {
					return ""
				}
				return s.Status
			}, 2*time.Second, 20*time.Millisecond).Should(Equal(session.StatusRunning))

			result := &katypes.InvestigationResult{RCASummary: "operator closed with no action"}
			Expect(mgr.ForceCompleteByRemediationID("rr-1654-b-001", result)).To(Succeed())

			sessA, errA := mgr.GetSession(idA)
			Expect(errA).NotTo(HaveOccurred())
			Expect(sessA.Status).To(Equal(session.StatusCompleted),
				"IT must complete BOTH sibling sessions sharing rr-1654-b-001, not just the first match (#1654)")

			sessB, errB := mgr.GetSession(idB)
			Expect(errB).NotTo(HaveOccurred())
			Expect(sessB.Status).To(Equal(session.StatusCompleted),
				"IT must complete BOTH sibling sessions sharing rr-1654-b-001, not just the first match (#1654)")
		})

		It("should return ErrSessionNotFound when no non-terminal session matches", func() {
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)

			err := mgr.ForceCompleteByRemediationID("rr-1654-b-nonexistent", nil)
			Expect(err).To(MatchError(session.ErrSessionNotFound))
		})
	})

	Describe("UT-KA-1654-B-002: emits a SessionCompleted audit event per completed session (SOC2 CC8.1)", func() {
		It("should emit one EventTypeSessionCompleted event for each sibling session force-completed", func() {
			store := session.NewStore(30 * time.Minute)
			auditSpy := &spyAuditStore{}
			mgr := session.NewManager(store, logr.Discard(), auditSpy, nil)

			idA, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}, map[string]string{"remediation_id": "rr-1654-b-002"})
			Expect(err).NotTo(HaveOccurred())

			idB, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}, map[string]string{"remediation_id": "rr-1654-b-002"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				s, _ := mgr.GetSession(idA)
				if s == nil {
					return ""
				}
				return s.Status
			}, 2*time.Second, 20*time.Millisecond).Should(Equal(session.StatusRunning))
			Eventually(func() session.Status {
				s, _ := mgr.GetSession(idB)
				if s == nil {
					return ""
				}
				return s.Status
			}, 2*time.Second, 20*time.Millisecond).Should(Equal(session.StatusRunning))

			Expect(mgr.ForceCompleteByRemediationID("rr-1654-b-002", nil)).To(Succeed())

			events := auditSpy.Events()
			completedIDs := map[string]bool{}
			for _, e := range events {
				if e.EventType == audit.EventTypeSessionCompleted && e.CorrelationID == "rr-1654-b-002" {
					completedIDs[e.SessionID] = true
				}
			}
			Expect(completedIDs).To(HaveKey(idA),
				"ForceCompleteByRemediationID must emit a SessionCompleted audit event for every completed session (#1654, SOC2 CC8.1)")
			Expect(completedIDs).To(HaveKey(idB),
				"ForceCompleteByRemediationID must emit a SessionCompleted audit event for every completed session (#1654, SOC2 CC8.1)")
		})
	})
})
