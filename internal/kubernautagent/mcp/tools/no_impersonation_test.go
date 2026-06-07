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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/shared/transport"
)

var _ = Describe("No Runtime Impersonation (#1288)", func() {

	Describe("UT-KA-1288-002: Enrichment hook does not inject impersonation context", func() {
		It("enrichment context should not contain Impersonate-User", func() {
			var capturedCtx context.Context
			runner := &contextCapturingEnrichmentRunner{
				captureCtx: func(ctx context.Context) { capturedCtx = ctx },
			}

			wfID := "wf-1288"
			sessions := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:       "sess-1288",
					ActingUser:      mcpinternal.UserInfo{Username: "alice", Groups: []string{"sre"}},
					RCAResult:       &katypes.InvestigationResult{RCASummary: "test"},
					DiscoveryResult: discoveryWithWorkflow(wfID),
				},
			}
			catalog := &mockWorkflowCatalog{
				workflow: &mcptools.CatalogWorkflow{WorkflowID: wfID},
			}

			tool := mcptools.NewSelectWorkflowTool(catalog, sessions,
				mcptools.WithEnrichmentRunner(runner),
			)
			_, _ = tool.Handle(context.Background(), mcptools.SelectWorkflowInput{
				RRID:       "rr-1288",
				WorkflowID: wfID,
				Kind:       "Deployment",
				Name:       "test",
				Namespace:  "default",
			}, mcpinternal.UserInfo{Username: "alice"})

			Expect(capturedCtx).NotTo(BeNil(), "enrichment should have been called")
			impUser, _ := transport.ImpersonatedUserFromContext(capturedCtx)
			Expect(impUser).To(BeEmpty(),
				"enrichment context should NOT contain impersonation headers — KA uses its own SA (#1288)")
		})
	})

	Describe("UT-KA-1288-003: Interactive turn runs without WithImpersonatedUser", func() {
		It("investigate handle does not inject impersonation context", func() {
			var capturedCtx context.Context
			runner := &contextCapturingInvestigatorRunner{
				captureCtx: func(ctx context.Context) { capturedCtx = ctx },
				response:   "LLM response",
			}

			sessions := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "sess-1288-inv",
					CorrelationID: "rr-1288-inv",
					ActingUser:    mcpinternal.UserInfo{Username: "alice", Groups: []string{"sre"}},
				},
			}

			tool := mcptools.NewInvestigateTool(sessions, runner, nil, mcptools.NopAutonomousManager{})
			_, _ = tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:    "rr-1288-inv",
				Action:  "message",
				Message: "check logs",
			}, mcpinternal.UserInfo{Username: "alice"})

			Expect(capturedCtx).NotTo(BeNil(), "investigator should have been called")
			impUser, _ := transport.ImpersonatedUserFromContext(capturedCtx)
			Expect(impUser).To(BeEmpty(),
				"interactive turn context should NOT contain impersonation headers (#1288)")
		})
	})
})

// contextCapturingEnrichmentRunner captures the context passed to Enrich.
type contextCapturingEnrichmentRunner struct {
	captureCtx func(context.Context)
}

func (r *contextCapturingEnrichmentRunner) Enrich(ctx context.Context, _, _, _, _, _, _ string) (*enrichment.EnrichmentResult, error) {
	r.captureCtx(ctx)
	return &enrichment.EnrichmentResult{}, nil
}

// contextCapturingInvestigatorRunner captures the context passed to RunInteractiveTurn.
type contextCapturingInvestigatorRunner struct {
	captureCtx func(context.Context)
	response   string
}

func (r *contextCapturingInvestigatorRunner) RunInteractiveTurn(ctx context.Context, _ []mcptools.LLMMessage, _ string) (string, error) {
	r.captureCtx(ctx)
	return r.response, nil
}

func (r *contextCapturingInvestigatorRunner) RunRCAExtraction(_ context.Context, _ []mcptools.LLMMessage, _ string) (*katypes.InvestigationResult, error) {
	return &katypes.InvestigationResult{RCASummary: "mock"}, nil
}

func (r *contextCapturingInvestigatorRunner) RunWorkflowDiscovery(_ context.Context, _ katypes.SignalContext, _ *katypes.InvestigationResult, _ *prompt.EnrichmentData, _ string) (*katypes.InvestigationResult, error) {
	return &katypes.InvestigationResult{RCASummary: "mock"}, nil
}

func (r *contextCapturingInvestigatorRunner) RunFullInvestigation(_ context.Context, _ katypes.SignalContext) (*katypes.InvestigationResult, error) {
	return &katypes.InvestigationResult{RCASummary: "mock"}, nil
}
