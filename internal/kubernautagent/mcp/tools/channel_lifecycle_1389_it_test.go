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
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// it1389Runner implements InvestigatorRunner for the #1389 IT.
type it1389Runner struct {
	rcaResult       *katypes.InvestigationResult
	discoveryResult *katypes.InvestigationResult
}

func (r *it1389Runner) RunInteractiveTurn(_ context.Context, _ []mcptools.LLMMessage, _ string) (string, error) {
	return "", nil
}

func (r *it1389Runner) RunRCAExtraction(_ context.Context, _ []mcptools.LLMMessage, _ string) (*katypes.InvestigationResult, error) {
	return r.rcaResult, nil
}

func (r *it1389Runner) RunWorkflowDiscovery(_ context.Context, _ katypes.SignalContext, _ *katypes.InvestigationResult, _ *prompt.EnrichmentData, _ string) (*katypes.InvestigationResult, error) {
	return r.discoveryResult, nil
}

func (r *it1389Runner) RunFullInvestigation(_ context.Context, _ katypes.SignalContext) (*katypes.InvestigationResult, error) {
	return r.rcaResult, nil
}

var _ = Describe("Fix #1389 — Channel lifecycle IT (Pyramid Invariant: wiring proof)", func() {

	// IT-KA-1389-001: Exercises the production wiring path:
	//   StartInteractiveSession → LaunchDeferredInvestigation → Subscribe →
	//   goroutine exits (InteractiveHold) → channel stays open →
	//   discover_workflows → select_workflow → CompleteHTTPSession →
	//   real Manager.CompleteUserDriving → closeEventChan → channel closed.
	//
	// Uses the real session.Manager as HTTPSessionCompleter (not a mock).
	It("IT-KA-1389-001: CompleteHTTPSession through real Manager closes event channel after goroutine exits", func() {
		store := session.NewStore(30 * time.Minute)
		mgr := session.NewManager(store, logr.Discard(), audit.NopAuditStore{}, nil)

		rrID := "rr-it-1389-001"
		wfID := "restart-pod-v1"

		runner := &it1389Runner{
			rcaResult: &katypes.InvestigationResult{
				RCASummary:      "OOM on api-server pod",
				Confidence:      0.9,
				InteractiveHold: true,
				RemediationTarget: katypes.RemediationTarget{
					Kind: "Pod", Name: "api-server", Namespace: "prod",
				},
			},
			discoveryResult: &katypes.InvestigationResult{
				RCASummary: "OOM on api-server pod",
				WorkflowID: wfID,
				Confidence: 0.85,
			},
		}

		By("Step 1: Create interactive session and launch investigation")
		pendingID, err := mgr.StartInteractiveSession(context.Background(),
			func(ctx context.Context) (*katypes.InvestigationResult, error) {
				return runner.rcaResult, nil
			},
			map[string]string{"remediation_id": rrID},
		)
		Expect(err).NotTo(HaveOccurred())

		err = mgr.LaunchDeferredInvestigation(pendingID)
		Expect(err).NotTo(HaveOccurred())

		By("Step 2: Subscribe to activate LazySink and event channel")
		eventCh, subErr := mgr.Subscribe(context.Background(), pendingID)
		Expect(subErr).NotTo(HaveOccurred())
		Expect(eventCh).NotTo(BeNil())

		By("Step 3: Wait for goroutine to exit — status becomes UserDriving")
		Eventually(func() session.Status {
			s, _ := mgr.GetSession(pendingID)
			if s == nil {
				return ""
			}
			return s.Status
		}, 5*time.Second).Should(Equal(session.StatusUserDriving))

		time.Sleep(50 * time.Millisecond)

		By("Step 4: Verify channel is still open (LazySink writable)")
		ls, found := mgr.GetSessionLazySink(pendingID)
		Expect(found).To(BeTrue())
		activeSink := ls.Get()
		Expect(activeSink).NotTo(BeNil(),
			"LazySink must still be writable after goroutine exits")

		By("Step 5: Call CompleteHTTPSession through production wiring (real Manager)")
		// mgr implements HTTPSessionCompleter — this is the production path.
		mcptools.CompleteHTTPSession(
			mgr, rrID,
			&katypes.InvestigationResult{WorkflowID: wfID, RCASummary: "OOM"},
			logr.Discard(), "select_workflow",
		)

		By("Step 6: Verify event channel is now closed")
		Eventually(func() bool {
			select {
			case _, ok := <-eventCh:
				return !ok
			default:
				return false
			}
		}, 2*time.Second).Should(BeTrue(),
			"IT-KA-1389-001: event channel must be closed after CompleteHTTPSession → CompleteUserDriving")

		By("Step 7: Verify LazySink is cleared (defense-in-depth)")
		clearedSink := ls.Get()
		Expect(clearedSink).To(BeNil(),
			"IT-KA-1389-001: LazySink must be nil after CompleteUserDriving")
	})
})
