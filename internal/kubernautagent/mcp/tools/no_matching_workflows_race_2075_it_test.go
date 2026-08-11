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

// IT-KA-2075-001 proves the fix through the REAL production dispatch path for
// BOTH tools involved in the race: InvestigateTool.Handle(action=
// discover_workflows) drives the real no_matching_workflows auto-close
// goroutine (investigate_discovery.go's autoCloseOnNoMatchingWorkflows on
// main -- investigate.go on release/v1.5), which marks the shared
// AutoCloseTombstone and releases the lease; CompleteNoActionTool.Handle
// (complete_no_action.go) is then driven through its own real production
// Handle() and must consult that same tombstone instance instead of
// erroring. This is what distinguishes it from UT-KA-2075-001 (component
// logic in isolation) and UT-KA-2075-002 (complete_no_action's fallback
// logic against a fake tombstone): this IT proves the two production call
// sites are actually wired to one shared instance, exactly as
// cmd/kubernautagent/routes.go's buildMCPTools/buildAndRegisterMCPTools
// wire it (#2076, v1.6 clone of #2075).
var _ = Describe("IT #2076 (v1.6 clone of #2075) — complete_no_action races the no_matching_workflows auto-close", func() {

	It("IT-KA-2075-001 (AC-6, AU-12): complete_no_action succeeds with already_resolved when it arrives after the real auto-close already released the lease", func() {
		store := session.NewStore(30 * time.Minute)
		mgr := session.NewManager(store, logr.Discard(), audit.NopAuditStore{}, nil)

		rrID := "rr-it-2075-001"
		user := mcpinternal.UserInfo{Username: "alice"}

		By("Start and launch an interactive session so a stored RCA result exists for rrID")
		readyCh := make(chan struct{})
		pendingID, err := mgr.StartInteractiveSession(context.Background(),
			func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-readyCh
				return &katypes.InvestigationResult{
					InteractiveHold: true,
					RCASummary:      "OOM crash on payments-api",
				}, nil
			},
			map[string]string{"remediation_id": rrID},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(mgr.LaunchDeferredInvestigation(pendingID)).To(Succeed())

		By("Subscribe before releasing the goroutine — activates the LazySink, then drain in the background")
		eventCh, subErr := mgr.Subscribe(context.Background(), pendingID)
		Expect(subErr).NotTo(HaveOccurred())
		go func() {
			for range eventCh {
			}
		}()

		close(readyCh)

		By("Wait for the session to reach StatusUserDriving (RCA stored, ready for discover_workflows)")
		Eventually(func() session.Status {
			s, _ := mgr.GetSession(pendingID)
			if s == nil {
				return ""
			}
			return s.Status
		}, 5*time.Second).Should(Equal(session.StatusUserDriving))

		By("Share one real production lease-session manager stand-in and one AutoCloseTombstone across both tools, exactly as cmd/kubernautagent/routes.go wires them")
		leaseSessions := &mockSessionManager{
			isActive: true,
			getDriverResult: &mcpinternal.InteractiveSession{
				SessionID:     "lease-it-2075-001",
				CorrelationID: rrID,
				ActingUser:    user,
			},
		}
		tombstone := mcpinternal.NewAutoCloseTombstone(60 * time.Second)

		runner := &mockInvestigatorRunner{
			workflowDiscoveryResult: &katypes.InvestigationResult{
				RCASummary:        "OOM crash on payments-api",
				HumanReviewNeeded: true,
				HumanReviewReason: "no_matching_workflows",
			},
		}
		recon := &mockContextReconstructor{}

		investigateTool := mcptools.NewInvestigateTool(leaseSessions, runner, recon, mgr,
			mcptools.WithHTTPCompleter(mgr),
			mcptools.WithWorkflowCatalog(&mockWorkflowCatalog{}),
			mcptools.WithInvestigateAutoCloseTombstone(tombstone),
		)
		completeNoActionTool := mcptools.NewCompleteNoActionTool(leaseSessions,
			mcptools.WithCompleteNoActionHTTPCompleter(mgr),
			mcptools.WithCompleteNoActionAutoCloseTombstone(tombstone),
		)

		By("Drive discover_workflows through the real InvestigateTool — triggers the real no_matching_workflows auto-close goroutine")
		output, err := investigateTool.Handle(context.Background(), mcptools.InvestigateInput{
			RRID:   rrID,
			Action: mcptools.ActionDiscoverWorkflows,
		}, user)
		Expect(err).NotTo(HaveOccurred())
		Expect(output.Status).To(Equal("workflows_discovered"))

		By("Simulate the real production race: the async auto-close goroutine releases the lease (a real Release() call recorded on the shared session manager) before the console's Dismiss click arrives")
		Eventually(func() string {
			id, _ := leaseSessions.getReleased()
			return id
		}, 3*time.Second).Should(Equal("lease-it-2075-001"),
			"the real no_matching_workflows auto-close goroutine (investigate_discovery.go) must have called Release")
		leaseSessions.setActive(false)

		By("complete_no_action, called after the auto-close already released the lease, must succeed via the tombstone instead of erroring")
		completeOutput, err := completeNoActionTool.Handle(context.Background(), mcptools.CompleteNoActionInput{
			RRID: rrID,
		}, user)
		Expect(err).NotTo(HaveOccurred(), "#2075/#2076: complete_no_action must not error when it races the no_matching_workflows auto-close")
		Expect(completeOutput.Status).To(Equal("already_resolved"))

		By("Verify the real session actually reached its terminal state via the auto-close, not via this second complete_no_action call")
		Eventually(func() session.Status {
			s, _ := mgr.GetSession(pendingID)
			if s == nil {
				return ""
			}
			return s.Status
		}, 3*time.Second).Should(Equal(session.StatusCompleted))
	})
})
