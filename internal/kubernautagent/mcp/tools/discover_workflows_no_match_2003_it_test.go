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

// IT-KA-2003-001 proves the fix through the REAL production dispatch path —
// InvestigateTool.Handle(action=discover_workflows) — wired to a real
// session.Manager (not a mock), so EmitSessionEndedByRR's real
// emitTerminalEvent -> sink.Emit path executes and the terminal event is
// forwarded by the real EventLogBridge, exactly as it is in production
// (#1438, AU-3, SI-4). This is what distinguishes it from the UT: the UT
// proves handleDiscoverWorkflows calls the right mock methods; this IT proves
// the whole KA-side signal actually reaches the point AF's WatchTerminalEvents
// is listening at.
var _ = Describe("IT #2003 — discover_workflows(no_matching_workflows) auto-closes via the real dispatch path", func() {

	It("IT-KA-2003-001 (AU-3, SI-4): session_ended propagates through EventLogBridge promptly, without waiting for inactivity timeout", func() {
		store := session.NewStore(30 * time.Minute)
		mgr := session.NewManager(store, logr.Discard(), audit.NopAuditStore{}, nil)

		rrID := "rr-it-2003-001"

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

		By("Subscribe before releasing the goroutine — activates the LazySink")
		eventCh, subErr := mgr.Subscribe(context.Background(), pendingID)
		Expect(subErr).NotTo(HaveOccurred())

		By("Start EventLogBridge (production wiring)")
		capture := &logCapture{}
		bridge := mcptools.NewEventLogBridge(eventCh, capture.logFn, logr.Discard(), pendingID)
		bridgeCtx, bridgeCancel := context.WithCancel(context.Background())
		defer bridgeCancel()
		go bridge.Run(bridgeCtx)

		close(readyCh)

		By("Wait for the session to reach StatusUserDriving (RCA stored, ready for discover_workflows)")
		Eventually(func() session.Status {
			s, _ := mgr.GetSession(pendingID)
			if s == nil {
				return ""
			}
			return s.Status
		}, 5*time.Second).Should(Equal(session.StatusUserDriving))

		Eventually(func() int { return capture.count() }, 3*time.Second).Should(BeNumerically(">=", 1),
			"drain the EventTypeComplete emitted by the investigation goroutine's exit")

		By("Drive discover_workflows through the real InvestigateTool, mirroring production wiring: " +
			"autoMgr AND httpCompleter both point at the same session.Manager (cmd/kubernautagent/main.go: WithHTTPCompleter(autoMgr))")
		leaseSessions := &mockSessionManager{
			isActive: true,
			getDriverResult: &mcpinternal.InteractiveSession{
				SessionID:     "lease-it-2003-001",
				CorrelationID: rrID,
				ActingUser:    mcpinternal.UserInfo{Username: "alice"},
			},
		}
		runner := &mockInvestigatorRunner{
			workflowDiscoveryResult: &katypes.InvestigationResult{
				RCASummary:        "OOM crash on payments-api",
				HumanReviewNeeded: true,
				HumanReviewReason: "no_matching_workflows",
			},
		}
		recon := &mockContextReconstructor{}

		tool := mcptools.NewInvestigateTool(leaseSessions, runner, recon, mgr,
			mcptools.WithHTTPCompleter(mgr),
			mcptools.WithWorkflowCatalog(&mockWorkflowCatalog{}),
		)

		output, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
			RRID:   rrID,
			Action: mcptools.ActionDiscoverWorkflows,
		}, mcpinternal.UserInfo{Username: "alice"})
		Expect(err).NotTo(HaveOccurred())
		Expect(output.Status).To(Equal("workflows_discovered"))

		By("Verify EventLogBridge forwarded session_ended promptly (no inactivity-timeout-scale wait)")
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
		}, 3*time.Second).Should(Equal(session.EventTypeSessionEnded))

		var env struct {
			EventType string `json:"type"`
			Phase     string `json:"phase"`
			Seq       int64  `json:"seq"`
		}
		Expect(json.Unmarshal(capture.latest(), &env)).To(Succeed())
		Expect(env.Phase).To(Equal("no_matching_workflows"),
			"AU-3: terminal event must carry the release reason so AF can map it to a console phase")
		Expect(env.Seq).To(BeNumerically(">", 0),
			"SI-4: event must have a positive sequence number for ordering")

		By("Verify the real session actually transitioned to terminal (StatusCompleted), " +
			"proving production dispatch drove the real session lifecycle, not just a mock call")
		Eventually(func() session.Status {
			s, _ := mgr.GetSession(pendingID)
			if s == nil {
				return ""
			}
			return s.Status
		}, 3*time.Second).Should(Equal(session.StatusCompleted))
	})
})
