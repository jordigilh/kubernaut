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

package main

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcpadapters "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/adapters"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	dsmodels "github.com/jordigilh/kubernaut/pkg/datastorage/models"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// ============================================================================
// #1654: CompleteNoActionTool / SelectWorkflowTool inactivity-timer wiring.
//
// buildMCPTools must wire each tool that can terminate an interactive session
// (complete_no_action, select_workflow) with the shared TimeoutManager so the
// tool can cancel the session's inactivity timer on completion. Without this
// wiring, the timer fires ~InactivityTimeout later regardless of the session
// already being terminal, producing spurious "failed to release expired
// session" errors and an incorrect "inactivity_timeout" audit/terminal event
// for a session that actually ended for a different reason.
// ============================================================================

// stubWorkflowCatalogFetcher implements mcpadapters.WorkflowCatalogFetcher,
// returning a fixed catalog entry for GetByID (the only method
// WorkflowCatalogAdapter.GetWorkflowByID calls). #1677 Phase 2e
// (DD-WORKFLOW-019): replaces the former DS-ogen-client-backed
// wfclient.WorkflowQuerier stub now that the adapter is catalog-backed.
type stubWorkflowCatalogFetcher struct{}

func (stubWorkflowCatalogFetcher) GetByID(_ context.Context, _ string) (*dsmodels.RemediationWorkflow, error) {
	return &dsmodels.RemediationWorkflow{
		WorkflowName:    "restart-pod",
		ExecutionEngine: "argo",
		ExecutionBundle: strPtr("oci://example/restart-pod:v1"),
	}, nil
}

func strPtr(s string) *string { return &s }

func newMCPTestLeaseSessionManager() *mcpinternal.LeaseSessionManager {
	scheme := runtime.NewScheme()
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		panic(err)
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	return mcpinternal.NewLeaseSessionManagerConcrete(fakeClient, "kubernaut-system", logr.Discard())
}

var _ = Describe("buildMCPTools — interactive-session inactivity-timer wiring (#1654)", func() {

	It("IT-KA-1654-001: complete_no_action stops the inactivity timer for the session it completes", func() {
		leaseMgr := newMCPTestLeaseSessionManager()
		autoMgr := newMCPTestAutoMgr()

		expired := make(chan string, 1)
		timeoutMgr := mcpinternal.NewTimeoutManager(30*time.Millisecond, nil, func(sessionID string) {
			expired <- sessionID
		})
		defer timeoutMgr.StopAll()

		_, _, completeNoActionTool := buildMCPTools(mcpToolsDeps{
			leaseMgr:     leaseMgr,
			autoMgr:      autoMgr,
			agentMetrics: newMCPTestAgentMetrics(),
			timeoutMgr:   timeoutMgr,
			auditStore:   audit.NopAuditStore{},
			logger:       logr.Discard(),
		})

		user := mcpinternal.UserInfo{Username: "alice"}
		driver, err := leaseMgr.Takeover(context.Background(), "rr-1654-001", user)
		Expect(err).NotTo(HaveOccurred())

		timeoutMgr.StartTracking(driver.SessionID, func(string) {})

		_, err = completeNoActionTool.Handle(context.Background(), mcptools.CompleteNoActionInput{RRID: "rr-1654-001"}, user)
		Expect(err).NotTo(HaveOccurred())

		Consistently(expired, 120*time.Millisecond, 10*time.Millisecond).ShouldNot(Receive(),
			"IT-KA-1654-001: complete_no_action must stop the inactivity timer via the production-wired "+
				"timeoutTracker; if this fires, WithCompleteNoActionTimeoutTracker is not wired into "+
				"buildMCPTools' construction of CompleteNoActionTool")
	})

	It("IT-KA-1654-002: select_workflow stops the inactivity timer for the session it completes", func() {
		leaseMgr := newMCPTestLeaseSessionManager()
		autoMgr := newMCPTestAutoMgr()

		expired := make(chan string, 1)
		timeoutMgr := mcpinternal.NewTimeoutManager(30*time.Millisecond, nil, func(sessionID string) {
			expired <- sessionID
		})
		defer timeoutMgr.StopAll()

		catalogAdapter := mcpadapters.NewWorkflowCatalogAdapter(stubWorkflowCatalogFetcher{})

		_, selectWfTool, _ := buildMCPTools(mcpToolsDeps{
			leaseMgr:       leaseMgr,
			autoMgr:        autoMgr,
			agentMetrics:   newMCPTestAgentMetrics(),
			timeoutMgr:     timeoutMgr,
			auditStore:     audit.NopAuditStore{},
			logger:         logr.Discard(),
			catalogAdapter: catalogAdapter,
		})

		user := mcpinternal.UserInfo{Username: "alice"}
		driver, err := leaseMgr.Takeover(context.Background(), "rr-1654-002", user)
		Expect(err).NotTo(HaveOccurred())
		driver.DiscoveryResult = &mcpinternal.WorkflowDiscoveryResult{
			Recommended: &mcpinternal.DiscoveredWorkflow{WorkflowID: "wf-1654"},
		}
		driver.RCAResult = &katypes.InvestigationResult{
			RCASummary: "OOM on api-server pod",
			Confidence: 0.9,
		}

		timeoutMgr.StartTracking(driver.SessionID, func(string) {})

		_, err = selectWfTool.Handle(context.Background(), mcptools.SelectWorkflowInput{
			RRID:       "rr-1654-002",
			WorkflowID: "wf-1654",
		}, user)
		Expect(err).NotTo(HaveOccurred())

		Consistently(expired, 120*time.Millisecond, 10*time.Millisecond).ShouldNot(Receive(),
			"IT-KA-1654-002: select_workflow must stop the inactivity timer via the production-wired "+
				"timeoutTracker; if this fires, SelectWorkflowTool has no way to cancel it (missing "+
				"WithSelectWorkflowTimeoutTracker wiring)")
	})
})
