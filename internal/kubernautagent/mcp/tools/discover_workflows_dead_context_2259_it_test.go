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

// Fix #2259: reproduces the dead-context race deterministically against a
// REAL session.Manager (not mocks) -- a takeover cancels the autonomous
// investigation before it ever reaches its first LLM call (the
// InvestigateFunc below observes ctx.Done() and returns immediately with no
// result, matching production's runRCA/emitLLMRequestAudit never having
// been reached), so zero audit events exist to reconstruct. Before the fix,
// discover_workflows would then hit the permanent "no conversation context
// available" dead end (see this file's sibling reconstruction_test.go for
// the RED proof against the old behavior); after the fix it self-heals via
// a fresh investigation (createFreshInteractiveSession, same pipeline
// action=start's fallback chain already uses) instead of failing
// permanently.
var _ = Describe("Fix #2259: discover_workflows self-heals a takeover that raced ahead of the first LLM call", func() {

	Describe("IT-KA-2259-001: takeover cancels before any audit events exist, discover_workflows self-heals instead of permanently failing", func() {
		It("should self-heal via a fresh investigation and leave a fresh, correlation_id-keyed audit trail behind", func() {
			store := session.NewStore(30 * time.Minute)
			auditStore := &recordingAuditStore{}
			mgr := session.NewManager(store, logr.Discard(), auditStore, nil)

			rrID := "rr-it-2259-001"

			By("starting an autonomous investigation that is cancelled before producing anything -- #2259's dead-context race")
			autoID, startErr := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}, map[string]string{"remediation_id": rrID})
			Expect(startErr).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				s, _ := mgr.GetSession(autoID)
				if s == nil {
					return ""
				}
				return s.Status
			}, 2*time.Second).Should(Equal(session.StatusRunning))

			By("racing a takeover in before the investigation's first LLM call ever lands")
			takeoverSess := &mcpinternal.InteractiveSession{
				SessionID:     "mcp-lease-it-2259-001",
				CorrelationID: rrID,
				ActingUser:    mcpinternal.UserInfo{Username: "sre-erin"},
			}
			sessionMgr := &mockSessionManager{
				takeoverSession: takeoverSess,
				isActive:        true,
				getDriverResult: takeoverSess,
			}
			runner := &mockInvestigatorRunner{
				rcaResult: &katypes.InvestigationResult{
					RCASummary: "self-healed: OOMKilled on api-server",
					Confidence: 0.88,
					RemediationTarget: katypes.RemediationTarget{
						Kind: "Deployment", Name: "api-server", Namespace: "production",
					},
				},
				workflowDiscoveryResult: &katypes.InvestigationResult{
					RCASummary: "self-healed: OOMKilled on api-server",
					WorkflowID: "restart-pod-v1",
					Confidence: 0.88,
				},
			}
			recon := &mockContextReconstructor{} // no reconstructable turns: the goroutine never produced anything to audit
			catalog := &mockWorkflowCatalog{
				workflow: &mcptools.CatalogWorkflow{WorkflowID: "restart-pod-v1", WorkflowName: "Restart Pod"},
			}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mgr,
				mcptools.WithHTTPCompleter(mgr),
				mcptools.WithSignalContextResolver(&mockSignalResolver{}),
				mcptools.WithWorkflowCatalog(catalog))

			user := mcpinternal.UserInfo{Username: "sre-erin"}
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   rrID,
				Action: mcptools.ActionTakeover,
			}, user)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("takeover_started"))
			Expect(out.Response).To(Equal("0 prior turns reconstructed"),
				"#2259: confirms the dead end genuinely exists before self-healing -- zero audit events, nothing to reconstruct")

			By("calling discover_workflows against the dead-context state -- must self-heal instead of permanently failing")
			dwOut, dwErr := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   rrID,
				Action: mcptools.ActionDiscoverWorkflows,
			}, user)
			Expect(dwErr).NotTo(HaveOccurred(),
				"#2259: discover_workflows must self-heal via a fresh investigation instead of permanently failing when zero audit events exist to reconstruct")
			Expect(dwOut.Status).To(Equal("workflows_discovered"))

			By("BR-AUDIT-005/SOC2 CC7.2: a fresh, correlation_id-keyed audit trail must now exist where none did before")
			started := auditEventsOfType(auditStore, audit.EventTypeSessionStarted)
			var correlatedSessionIDs []string
			for _, e := range started {
				if e.CorrelationID == rrID {
					correlatedSessionIDs = append(correlatedSessionIDs, e.SessionID)
				}
			}
			Expect(correlatedSessionIDs).To(ContainElement(autoID),
				"the original (cancelled) autonomous session's start event must remain part of the trail")
			Expect(len(correlatedSessionIDs)).To(BeNumerically(">=", 2),
				"#2259: the self-healing fresh investigation must have its own session.started audit event under the "+
					"same correlation_id, proving a reconstructable trail now exists where the cancelled investigation left none")
		})
	})
})
