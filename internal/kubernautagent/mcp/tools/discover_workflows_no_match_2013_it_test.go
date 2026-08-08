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
	"encoding/json"
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

// #2013 (main port of #2003): proves the real production dispatch path --
// InvestigateTool.Handle(action=discover_workflows) driving a REAL
// session.Manager as autoMgr (not a mock of AutonomousSessionManager) --
// actually emits session_ended through EventLogBridge when discovery
// concludes no_matching_workflows, instead of relying on the mock used by
// UT-KA-2013-001 to merely assert the call happened. This is the IT half of
// the pyramid invariant: the UT proves the auto-close logic in isolation;
// this IT proves it is wired to the real session.Manager.EmitSessionEndedByRR
// implementation and reaches a real subscriber, exactly as
// terminal_event_1438_it_test.go proves for the timeout/disconnect paths.
var _ = Describe("IT-KA-2013-001: discover_workflows auto-close reaches EventLogBridge through the real session.Manager", func() {
	It("should emit session_ended (phase=no_matching_workflows) via the real autoMgr when the tool concludes no_matching_workflows", func() {
		store := session.NewStore(30 * time.Minute)
		mgr := session.NewManager(store, logr.Discard(), audit.NopAuditStore{}, nil)

		rrID := "rr-it-2013-001"

		readyCh := make(chan struct{})
		pendingID, err := mgr.StartInteractiveSession(context.Background(),
			func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-readyCh
				return &katypes.InvestigationResult{
					InteractiveHold: true,
					RCASummary:      "pod crash loop due to OOM",
				}, nil
			},
			map[string]string{"remediation_id": rrID},
		)
		Expect(err).NotTo(HaveOccurred())

		Expect(mgr.LaunchDeferredInvestigation(pendingID)).To(Succeed())

		eventCh, subErr := mgr.Subscribe(context.Background(), pendingID)
		Expect(subErr).NotTo(HaveOccurred())

		capture := &logCapture{}
		bridge := mcptools.NewEventLogBridge(eventCh, capture.logFn, logr.Discard(), pendingID)
		bridgeCtx, bridgeCancel := context.WithCancel(context.Background())
		defer bridgeCancel()
		go bridge.Run(bridgeCtx)

		close(readyCh)

		By("wait for the deferred investigation to reach user_driving so GetLatestRCAResultByRemediationID resolves")
		Eventually(func() session.Status {
			s, _ := mgr.GetSession(pendingID)
			if s == nil {
				return ""
			}
			return s.Status
		}, 5*time.Second).Should(Equal(session.StatusUserDriving))

		By("build InvestigateTool with the real session.Manager as autoMgr")
		sessionMgr := &mockSessionManager{
			isActive: true,
			getDriverResult: &mcpinternal.InteractiveSession{
				SessionID:  pendingID,
				ActingUser: mcpinternal.UserInfo{Username: "alice"},
			},
		}
		runner := &mockInvestigatorRunner{
			workflowDiscoveryResult: &katypes.InvestigationResult{
				RCASummary:        "pod crash loop due to OOM",
				HumanReviewNeeded: true,
				HumanReviewReason: "no_matching_workflows",
			},
		}
		recon := &mockContextReconstructor{}
		completer := &orderedHTTPCompleter{found: false}

		tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, mgr,
			mcptools.WithHTTPCompleter(completer),
			mcptools.WithWorkflowCatalog(&mockWorkflowCatalog{}),
		)

		By("invoke the real dispatch path: kubernaut_investigate action=discover_workflows")
		output, handleErr := tool.Handle(context.Background(), mcptools.InvestigateInput{
			RRID:   rrID,
			Action: mcptools.ActionDiscoverWorkflows,
		}, mcpinternal.UserInfo{Username: "alice"})
		Expect(handleErr).NotTo(HaveOccurred())
		Expect(output.Status).To(Equal("workflows_discovered"))

		By("verify session_ended reached EventLogBridge without waiting for the inactivity timeout")
		Eventually(func() string {
			raw := capture.latest()
			if raw == nil {
				return ""
			}
			var env struct {
				EventType string `json:"type"`
			}
			_ = json.Unmarshal(raw, &env)
			return env.EventType
		}, 3*time.Second).Should(Equal(session.EventTypeSessionEnded),
			"#2013: AF's WatchTerminalEvents has no timer-based safety net and can only unblock "+
				"on session_ended -- this must arrive promptly, not after the 10-minute inactivity timeout")

		var env struct {
			EventType string `json:"type"`
			Phase     string `json:"phase"`
			Seq       int64  `json:"seq"`
		}
		Expect(json.Unmarshal(capture.latest(), &env)).To(Succeed())
		Expect(env.Phase).To(Equal("no_matching_workflows"),
			"AU-3: terminal event must carry the release reason")
		Expect(env.Seq).To(BeNumerically(">", 0), "SI-4: event must have a positive sequence number for ordering")
	})
})
