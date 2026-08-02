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

package tools_test

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// auditEventsOfType filters a recordingAuditStore's captured events by type,
// for #1818's audit reconstruction assertions (BR-AUDIT-005, SOC2 CC8.1).
// recordingAuditStore is defined in takeover_test.go (same package).
func auditEventsOfType(store *recordingAuditStore, eventType string) []*audit.AuditEvent {
	store.mu.Lock()
	defer store.mu.Unlock()
	var result []*audit.AuditEvent
	for _, e := range store.events {
		if e.EventType == eventType {
			result = append(result, e)
		}
	}
	return result
}

// Fix #1818: exercises the full production wiring path — a REAL
// session.Manager, not mocks — for the exact race that orphans a completed
// autonomous investigation's RCA: the autonomous investigation completes
// with a real RCA, then AF's interactive kubernaut_investigate action=start
// call races in afterward. Before the fix, upgradeOrCreateInteractiveSession
// only looked for a StatusRunning session (FindByRemediationID), missed the
// now-StatusCompleted one, and createFallbackSession unconditionally seeded
// a hardcoded "awaiting user direction" placeholder — orphaning the real RCA
// and fragmenting the audit trail for this remediation request
// (BR-AUDIT-005, SOC2 CC8.1). autoMgr and httpCompleter are the SAME real
// *session.Manager instance, matching production wiring
// (cmd/kubernautagent/routes.go passes d.autoMgr for both).
var _ = Describe("Fix #1818: orphaned RCA on interactive reattach race", func() {

	Describe("IT-KA-1818-001: interactive action=start after autonomous completion reattaches the real RCA", func() {
		It("should seed the fresh interactive session with the real RCA instead of a placeholder, leaving the original session untouched", func() {
			store := session.NewStore(30 * time.Minute)
			auditStore := &recordingAuditStore{}
			mgr := session.NewManager(store, logr.Discard(), auditStore, nil)

			rrID := "rr-it-1818-001"
			realRCA := &katypes.InvestigationResult{
				RCASummary: "Pod OOMKilled due to memory leak in /api/v2/reports endpoint",
				Confidence: 0.91,
			}

			By("the autonomous investigation runs to completion BEFORE the interactive request arrives")
			autoID, startErr := mgr.StartInvestigation(context.Background(), func(_ context.Context) (*katypes.InvestigationResult, error) {
				return realRCA, nil
			}, map[string]string{"remediation_id": rrID})
			Expect(startErr).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				s, _ := mgr.GetSession(autoID)
				if s == nil {
					return ""
				}
				return s.Status
			}, 2*time.Second).Should(Equal(session.StatusCompleted),
				"the autonomous investigation must have already completed — this is the #1818 race window")

			By("AF's interactive kubernaut_investigate action=start races in afterward")
			takeoverSess := &mcpinternal.InteractiveSession{
				SessionID:     "mcp-lease-it-1818-001",
				CorrelationID: rrID,
				ActingUser:    mcpinternal.UserInfo{Username: "sre-alice"},
			}
			sessionMgr := &mockSessionManager{
				takeoverSession: takeoverSess,
				isActive:        true,
				getDriverResult: takeoverSess,
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mgr,
				mcptools.WithHTTPCompleter(mgr))

			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   rrID,
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "sre-alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.InvestigationSessionID).NotTo(BeEmpty())
			Expect(out.InvestigationSessionID).NotTo(Equal(autoID),
				"a fresh interactive session is created since the autonomous one is already terminal")

			By("the fresh session must carry the REAL RCA, not a disconnected placeholder (#1818)")
			var freshSess *session.Session
			Eventually(func() *katypes.InvestigationResult {
				s, _ := mgr.GetSession(out.InvestigationSessionID)
				freshSess = s
				if s == nil {
					return nil
				}
				return s.Result
			}, 2*time.Second).ShouldNot(BeNil(),
				"the seeded investigateFn runs in StartInvestigation's background goroutine — wait for it to land")
			Expect(freshSess.Result.RCASummary).To(Equal(realRCA.RCASummary),
				"#1818: the real RCA the autonomous investigation already produced must not be orphaned behind a hardcoded placeholder")
			Expect(freshSess.Metadata["mode"]).To(Equal("interactive_reattached"))

			By("the original autonomous session is left untouched — its own completed RCA is unmodified")
			origSess, origErr := mgr.GetSession(autoID)
			Expect(origErr).NotTo(HaveOccurred())
			Expect(origSess.Status).To(Equal(session.StatusCompleted))
			Expect(origSess.Result.RCASummary).To(Equal(realRCA.RCASummary))
		})
	})

	Describe("IT-KA-1818-002: audit trail reconstruction by correlation_id remains coherent across the reattach (BR-AUDIT-005, SOC2 CC8.1)", func() {
		It("should let every session_id emitted for this correlation_id resolve to the SAME real RCA content", func() {
			store := session.NewStore(30 * time.Minute)
			auditStore := &recordingAuditStore{}
			mgr := session.NewManager(store, logr.Discard(), auditStore, nil)

			rrID := "rr-it-1818-002"
			realRCA := &katypes.InvestigationResult{
				RCASummary: "CrashLoopBackOff caused by missing ConfigMap key",
				Confidence: 0.78,
			}

			autoID, startErr := mgr.StartInvestigation(context.Background(), func(_ context.Context) (*katypes.InvestigationResult, error) {
				return realRCA, nil
			}, map[string]string{"remediation_id": rrID})
			Expect(startErr).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				s, _ := mgr.GetSession(autoID)
				if s == nil {
					return ""
				}
				return s.Status
			}, 2*time.Second).Should(Equal(session.StatusCompleted))

			takeoverSess := &mcpinternal.InteractiveSession{
				SessionID:     "mcp-lease-it-1818-002",
				CorrelationID: rrID,
				ActingUser:    mcpinternal.UserInfo{Username: "sre-bob"},
			}
			sessionMgr := &mockSessionManager{
				takeoverSession: takeoverSess,
				isActive:        true,
				getDriverResult: takeoverSess,
			}
			tool := mcptools.NewInvestigateTool(sessionMgr, &mockInvestigatorRunner{}, &mockContextReconstructor{}, mgr,
				mcptools.WithHTTPCompleter(mgr))

			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   rrID,
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "sre-bob"})
			Expect(err).NotTo(HaveOccurred())

			By("BR-AUDIT-005: every session.started event for this correlation_id must resolve to the real, consistent RCA")
			started := auditEventsOfType(auditStore, audit.EventTypeSessionStarted)
			var correlatedSessionIDs []string
			for _, e := range started {
				if e.CorrelationID == rrID {
					correlatedSessionIDs = append(correlatedSessionIDs, e.SessionID)
				}
			}
			Expect(correlatedSessionIDs).To(HaveLen(2),
				"the audit trail must show both the original autonomous session and the reattached interactive session under the SAME correlation_id — this is what makes remediation request reconstruction possible from audit traces alone")
			Expect(correlatedSessionIDs).To(ContainElement(autoID))
			Expect(correlatedSessionIDs).To(ContainElement(out.InvestigationSessionID))

			for _, sid := range correlatedSessionIDs {
				var sess *session.Session
				Eventually(func() *katypes.InvestigationResult {
					s, _ := mgr.GetSession(sid)
					sess = s
					if s == nil {
						return nil
					}
					return s.Result
				}, 2*time.Second).ShouldNot(BeNil(),
					"#1818: every session_id reachable from this correlation_id's audit trail must carry real investigation content, not be a dead end")
				Expect(sess.Result.RCASummary).To(Equal(realRCA.RCASummary),
					"SOC2 CC8.1: reconstructing this remediation request from its audit trail must yield ONE coherent RCA narrative, not a fragmented mix of real and orphaned-placeholder content")
			}
		})
	})
})
