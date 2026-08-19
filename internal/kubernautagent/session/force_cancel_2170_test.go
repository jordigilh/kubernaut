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
// #2170 (DD-AA-KA-001 Amendment): ForceCancelByRemediationID is the cleanup
// path for two production call sites that have nothing else to stop an
// orphaned investigation goroutine now that HTTP polling (and its
// CancelSession RPC) is gone:
//
//  1. Dispatcher.handleEvent's watch.Deleted case -- an AgentSession
//     disappearing (directly, or transitively via RR/AIAnalysis cascade
//     deletion) has no CRD left to poll/write to, but KA's in-memory
//     investigation goroutine for it must still be stopped, not leaked.
//  2. Dispatcher.resync's TimesOutAt self-enforcement -- KA independently
//     honors the same absolute deadline AA already enforces
//     (pkg/aianalysis/handlers/investigating.go's checkInvestigationTimeout,
//     DD-TIMEOUT-002/#2176), so a partitioned/crashed AA can never leave KA
//     running an investigation forever.
//
// Mirrors ForceCompleteByRemediationID's (#1654) iterate-then-fire-hooks-
// after-unlock pattern and multi-sibling-session semantics exactly, but
// transitions to StatusCancelled with no result instead of StatusCompleted.
// ============================================================================

var _ = Describe("session.Manager.ForceCancelByRemediationID — #2170 orphaned-goroutine cleanup", func() {

	Describe("UT-KA-2170-001: cancels every non-terminal session sharing the remediation_id", func() {
		It("should transition both sibling sessions to Cancelled, not just the first", func() {
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)

			doneA := make(chan struct{})
			idA, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-ctx.Done()
				close(doneA)
				return nil, ctx.Err()
			}, map[string]string{"remediation_id": "rr-2170-001"})
			Expect(err).NotTo(HaveOccurred())

			doneB := make(chan struct{})
			idB, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-ctx.Done()
				close(doneB)
				return nil, ctx.Err()
			}, map[string]string{"remediation_id": "rr-2170-001"})
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

			Expect(mgr.ForceCancelByRemediationID("rr-2170-001")).To(Succeed())

			sessA, errA := mgr.GetSession(idA)
			Expect(errA).NotTo(HaveOccurred())
			Expect(sessA.Status).To(Equal(session.StatusCancelled),
				"ForceCancelByRemediationID must cancel BOTH sibling sessions sharing rr-2170-001, not just the first match")

			sessB, errB := mgr.GetSession(idB)
			Expect(errB).NotTo(HaveOccurred())
			Expect(sessB.Status).To(Equal(session.StatusCancelled),
				"ForceCancelByRemediationID must cancel BOTH sibling sessions sharing rr-2170-001, not just the first match")

			Eventually(doneA, time.Second).Should(BeClosed(), "the investigation goroutine must actually be stopped, not just marked Cancelled")
			Eventually(doneB, time.Second).Should(BeClosed(), "the investigation goroutine must actually be stopped, not just marked Cancelled")
		})

		It("should return ErrSessionNotFound when no non-terminal session matches", func() {
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)

			err := mgr.ForceCancelByRemediationID("rr-2170-nonexistent")
			Expect(err).To(MatchError(session.ErrSessionNotFound))
		})

		It("should not re-cancel an already-terminal session", func() {
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)

			id, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				return &katypes.InvestigationResult{RCASummary: "done"}, nil
			}, map[string]string{"remediation_id": "rr-2170-002"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				s, _ := mgr.GetSession(id)
				if s == nil {
					return ""
				}
				return s.Status
			}, 2*time.Second, 20*time.Millisecond).Should(Equal(session.StatusCompleted))

			err = mgr.ForceCancelByRemediationID("rr-2170-002")
			Expect(err).To(MatchError(session.ErrSessionNotFound))

			sess, getErr := mgr.GetSession(id)
			Expect(getErr).NotTo(HaveOccurred())
			Expect(sess.Status).To(Equal(session.StatusCompleted),
				"an already-terminal session must never be overwritten to Cancelled")
		})
	})

	Describe("UT-KA-2170-002: emits a SessionCancelled audit event per cancelled session (SOC2 CC8.1)", func() {
		It("should emit one EventTypeSessionCancelled event for each sibling session force-cancelled", func() {
			store := session.NewStore(30 * time.Minute)
			auditSpy := &spyAuditStore{}
			mgr := session.NewManager(store, logr.Discard(), auditSpy, nil)

			idA, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}, map[string]string{"remediation_id": "rr-2170-003"})
			Expect(err).NotTo(HaveOccurred())

			idB, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}, map[string]string{"remediation_id": "rr-2170-003"})
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

			Expect(mgr.ForceCancelByRemediationID("rr-2170-003")).To(Succeed())

			events := auditSpy.Events()
			cancelledIDs := map[string]bool{}
			for _, e := range events {
				if e.EventType == audit.EventTypeSessionCancelled && e.CorrelationID == "rr-2170-003" {
					cancelledIDs[e.SessionID] = true
				}
			}
			Expect(cancelledIDs).To(HaveKey(idA),
				"ForceCancelByRemediationID must emit a SessionCancelled audit event for every cancelled session (SOC2 CC8.1)")
			Expect(cancelledIDs).To(HaveKey(idB),
				"ForceCancelByRemediationID must emit a SessionCancelled audit event for every cancelled session (SOC2 CC8.1)")
		})
	})
})
