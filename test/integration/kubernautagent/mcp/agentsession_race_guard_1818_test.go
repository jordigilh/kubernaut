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

package mcp_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// countingRunner is a minimal InvestigatorRunner test double used only to
// prove a negative: that waitForRaceyDispatch's success path never reaches
// RunFullInvestigation. It intentionally does not need to model any real
// investigation behavior -- the LLM/investigator pipeline is an external
// dependency of this race guard, not what this test is proving.
type countingRunner struct {
	fullInvestigationCalls atomic.Int32
}

func (r *countingRunner) RunInteractiveTurn(context.Context, []mcptools.LLMMessage, string) (string, error) {
	return "", nil
}

func (r *countingRunner) RunRCAExtraction(context.Context, []mcptools.LLMMessage, string) (*katypes.InvestigationResult, error) {
	return nil, nil
}

func (r *countingRunner) RunWorkflowDiscovery(context.Context, katypes.SignalContext, *katypes.InvestigationResult, *prompt.EnrichmentData, string) (*katypes.InvestigationResult, error) {
	return nil, nil
}

func (r *countingRunner) RunFullInvestigation(context.Context, katypes.SignalContext) (*katypes.InvestigationResult, error) {
	r.fullInvestigationCalls.Add(1)
	return &katypes.InvestigationResult{RCASummary: "duplicate-fresh-investigation"}, nil
}

// IT-KA-1818-RACE-001 reproduces the exact race window #1818's Gap 3
// amendment closed (CI evidence: run 32188463924, E2E-FLEET-018): AA has
// already created the AgentSession CRD for an RR, and KA's own dispatcher
// goroutine (modeled here by a direct session.Manager.StartInvestigation
// call, exactly as the real dispatcher registers a session -- see
// internal/kubernautagent/agentsession/dispatcher.go) is in the process of
// registering it in session.Manager, but hasn't finished yet, when a
// concurrent MCP action=start call for the same RR arrives.
//
// Proves the fix through the real production wiring
// (K8sAgentSessionExistenceChecker against a real envtest API server,
// exactly as cmd/kubernautagent/routes.go's buildMCPTools constructs it) --
// not a fake/mocked existence checker -- because the bug this closes was
// specifically about racing a real K8s Get against a real, concurrently
// slow in-memory registration; a fake checker would not exercise the same
// timing-sensitive code path CHECKPOINT W requires being proven for.
var _ = Describe("IT-KA-1818-RACE: AgentSession existence race guard", Label("integration", "race"), func() {

	It("IT-KA-1818-RACE-001: action=start attaches to the dispatcher's own session instead of starting a duplicate investigation", func() {
		ctx := context.Background()
		nsName := uniqueNamespace("race-1818")
		createNamespace(ctx, sharedK8sClient, nsName)

		rrID := "rr-race-1818-001"

		By("Simulating AA having already created the AgentSession CRD for this RR")
		as := &agentsessionv1.AgentSession{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("as-%s", rrID),
				Namespace: nsName,
			},
			Spec: agentsessionv1.AgentSessionSpec{
				RemediationRequestRef: agentsessionv1.ObjectRef{Name: "rr-race-1818-001", Namespace: nsName},
				IncidentID:            "aia-race-1818-001",
				RemediationID:         rrID,
				SignalName:            "OOMKilled",
				Severity:              "warning",
			},
		}
		Expect(sharedK8sClient.Create(ctx, as)).To(Succeed())

		By("Building a real InvestigateTool wired with the real K8s-backed race guard (mirrors cmd/kubernautagent/routes.go)")
		logger := logr.Discard()
		leaseMgr := mcpinternal.NewLeaseSessionManagerConcrete(sharedK8sClient, nsName, logger)
		recon := mcpinternal.NewDSContextReconstructor(sharedDSClient, 5*time.Second, logger)
		checker := mcptools.NewK8sAgentSessionExistenceChecker(sharedK8sClient, nsName)

		store := session.NewStore(30 * time.Minute)
		autoMgr := session.NewManager(store, logger, audit.NopAuditStore{}, nil)
		runner := &countingRunner{}

		investigateTool := mcptools.NewInvestigateTool(leaseMgr, runner, recon, autoMgr,
			mcptools.WithAgentSessionExistenceChecker(checker),
		)

		By("Racing a concurrent action=start against the dispatcher's own registration, which lands mid-poll")
		type startResult struct {
			out mcptools.InvestigateOutput
			err error
		}
		resultCh := make(chan startResult, 1)
		go func() {
			out, err := investigateTool.Handle(ctx, mcptools.InvestigateInput{
				RRID:   rrID,
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "alice"})
			resultCh <- startResult{out: out, err: err}
		}()

		// Registers rrID in autoMgr partway through waitForRaceyDispatch's
		// ~1.2s poll window (raceyDispatchPollInterval=200ms x
		// raceyDispatchMaxAttempts=6), modeling the real dispatcher's own
		// registration landing after the Get above already observed the
		// AgentSession CRD but before the poll gave up.
		time.Sleep(350 * time.Millisecond)
		holdInvestigation := make(chan struct{})
		registeredID, startErr := autoMgr.StartInvestigation(ctx,
			func(bgCtx context.Context) (*katypes.InvestigationResult, error) {
				<-holdInvestigation
				return &katypes.InvestigationResult{RCASummary: "dispatcher-driven RCA"}, nil
			},
			map[string]string{"remediation_id": rrID},
		)
		Expect(startErr).NotTo(HaveOccurred(), "the dispatcher's own StartInvestigation registration must succeed")

		var res startResult
		Eventually(resultCh, 3*time.Second).Should(Receive(&res))
		close(holdInvestigation)

		Expect(res.err).NotTo(HaveOccurred(),
			"IT-KA-1818-RACE-001: the race guard must let action=start attach to the AA-dispatched "+
				"session instead of exhausting every fallback and failing closed")
		Expect(res.out.InvestigationSessionID).To(Equal(registeredID),
			"IT-KA-1818-RACE-001: action=start must attach to the dispatcher's own session "+
				"(waitForRaceyDispatch's FindByRemediationID hit), not a second independent one")
		Expect(runner.fullInvestigationCalls.Load()).To(Equal(int32(0)),
			"IT-KA-1818-RACE-001: no duplicate RunFullInvestigation must be started for the same RR -- "+
				"that duplicate (running without the dispatcher's AgentSession-driven context) is exactly "+
				"what produced the generic, wrongly-scoped RCA content observed in E2E-FLEET-018")
	})
})
