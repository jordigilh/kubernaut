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
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// reconAutoMgr extends interactiveAutoMgr with configurable RCA summary lookup.
type reconAutoMgr struct {
	interactiveAutoMgr
	rcaSummary string
	rcaOK      bool
}

func (m *reconAutoMgr) GetLatestRCASummaryByRemediationID(_ string) (string, bool) {
	return m.rcaSummary, m.rcaOK
}
func (m *reconAutoMgr) GetLatestRCAResultByRemediationID(_ string) (*katypes.InvestigationResult, bool) {
	return nil, false
}

// messagesCapturingInvestigatorRunner records LLM messages passed to RunInteractiveTurn.
type messagesCapturingInvestigatorRunner struct {
	response         string
	capturedMessages []mcptools.LLMMessage
}

func (r *messagesCapturingInvestigatorRunner) RunInteractiveTurn(_ context.Context, messages []mcptools.LLMMessage, _ string) (string, error) {
	r.capturedMessages = messages
	return r.response, nil
}

func (r *messagesCapturingInvestigatorRunner) RunRCAExtraction(_ context.Context, _ []mcptools.LLMMessage, _ string) (*katypes.InvestigationResult, error) {
	return &katypes.InvestigationResult{RCASummary: "mock RCA"}, nil
}

func (r *messagesCapturingInvestigatorRunner) RunWorkflowDiscovery(_ context.Context, _ katypes.SignalContext, _ *katypes.InvestigationResult, _ *prompt.EnrichmentData, _ string) (*katypes.InvestigationResult, error) {
	return &katypes.InvestigationResult{RCASummary: "mock RCA"}, nil
}

func (r *messagesCapturingInvestigatorRunner) RunFullInvestigation(_ context.Context, _ katypes.SignalContext) (*katypes.InvestigationResult, error) {
	return &katypes.InvestigationResult{RCASummary: "mock RCA"}, nil
}

func newReconSessionMgr(rrID, sessionID string) *mockSessionManager {
	sess := &mcpinternal.InteractiveSession{
		SessionID:     sessionID,
		CorrelationID: rrID,
		ActingUser:    mcpinternal.UserInfo{Username: "alice"},
	}
	return &mockSessionManager{
		takeoverSession: sess,
		isActive:        true,
		getDriverResult: sess,
	}
}

var _ = Describe("BR-INTERACTIVE-010: Context reconstruction from audit trail", func() {

	Describe("UT-KA-1293-008: RCA summary available → single turn", func() {
		It("should store one assistant turn from RCA summary without calling Reconstruct", func() {
			const rrID = "rr-recon-008"
			sessionMgr := newReconSessionMgr(rrID, "sess-recon-008")
			autoMgr := &reconAutoMgr{
				rcaSummary: "Pod OOMKilled due to memory limit",
				rcaOK:      true,
			}
			recon := &mockContextReconstructor{
				turns: []mcpinternal.ConversationTurn{
					{Role: "user", Content: "should not be used"},
				},
			}
			runner := &messagesCapturingInvestigatorRunner{response: "continuing investigation"}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr)
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   rrID,
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("started"))
			Expect(recon.reconstructCalls.Load()).To(Equal(int32(0)),
				"Reconstruct must not be called when RCA summary is available")

			_, err = tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:    rrID,
				Action:  mcptools.ActionMessage,
				Message: "continue",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(runner.capturedMessages).To(HaveLen(2),
				"one RCA assistant turn plus current user message")
			Expect(runner.capturedMessages[0].Role).To(Equal("assistant"))
			Expect(runner.capturedMessages[0].Content).To(Equal(
				"Previous investigation RCA summary: Pod OOMKilled due to memory limit"))
			Expect(runner.capturedMessages[1].Role).To(Equal("user"))
			Expect(runner.capturedMessages[1].Content).To(Equal("continue"))
		})
	})

	Describe("UT-KA-1293-009: no RCA, DS returns turns → multiple turns stored", func() {
		It("should store reconstructed turns from DS when no RCA summary exists", func() {
			const rrID = "rr-recon-009"
			sessionMgr := newReconSessionMgr(rrID, "sess-recon-009")
			autoMgr := &interactiveAutoMgr{}
			recon := &mockContextReconstructor{
				turns: []mcpinternal.ConversationTurn{
					{Role: "user", Content: "first question"},
					{Role: "assistant", Content: "first answer"},
				},
			}
			runner := &messagesCapturingInvestigatorRunner{response: "follow-up response"}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr)
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   rrID,
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("started"))
			Expect(recon.reconstructCalls.Load()).To(Equal(int32(1)),
				"Reconstruct must be called when no RCA summary is available")

			_, err = tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:    rrID,
				Action:  mcptools.ActionMessage,
				Message: "next step",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(runner.capturedMessages).To(HaveLen(3),
				"two reconstructed turns plus current user message")
			Expect(runner.capturedMessages[0]).To(Equal(mcptools.LLMMessage{
				Role: "user", Content: "first question",
			}))
			Expect(runner.capturedMessages[1]).To(Equal(mcptools.LLMMessage{
				Role: "assistant", Content: "first answer",
			}))
			Expect(runner.capturedMessages[2]).To(Equal(mcptools.LLMMessage{
				Role: "user", Content: "next step",
			}))
		})
	})

	Describe("UT-KA-1293-010: no RCA, DS error → empty context", func() {
		It("should proceed with empty context when DS reconstruction fails", func() {
			const rrID = "rr-recon-010"
			sessionMgr := newReconSessionMgr(rrID, "sess-recon-010")
			autoMgr := &interactiveAutoMgr{}
			recon := &mockContextReconstructor{
				err: errors.New("DS unavailable"),
			}
			runner := &messagesCapturingInvestigatorRunner{response: "fresh start response"}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr)
			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   rrID,
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("started"))
			Expect(recon.reconstructCalls.Load()).To(Equal(int32(1)),
				"Reconstruct should still be attempted when no RCA summary exists")

			_, err = tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:    rrID,
				Action:  mcptools.ActionMessage,
				Message: "start fresh",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(runner.capturedMessages).To(HaveLen(1),
				"no prior context should be prepended when reconstruction fails")
			Expect(runner.capturedMessages[0]).To(Equal(mcptools.LLMMessage{
				Role: "user", Content: "start fresh",
			}))
		})
	})
})
