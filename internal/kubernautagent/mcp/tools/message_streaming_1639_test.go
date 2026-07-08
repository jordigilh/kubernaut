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
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
)

// #1639: handleMessage (action=message, the handler behind kubernaut_message)
// never attached a live-event sink to ctx before calling RunInteractiveTurn,
// so KA never streamed reasoning/reasoning_content/status events during
// message turns — separate from, and deeper than, #1637's AF-side relay gap
// (AF can only relay what KA actually sends). This mirrors the wiring
// discover_workflows already had for the same purpose (#1384,
// session_handoff_1384_test.go).
var _ = Describe("kubernaut_message live event context enrichment — #1639", func() {

	Describe("UT-KA-1639-001: action=message enriches ctx with HTTP session_id + LazySink", func() {
		It("should attach session_id and an active LazySink to the ctx passed to RunInteractiveTurn", func() {
			sess := &mcpinternal.InteractiveSession{
				SessionID:     "mcp-sess-1639-001",
				CorrelationID: "rr-1639-001",
				ActingUser:    mcpinternal.UserInfo{Username: "alice"},
			}
			sessionMgr := &mockSessionManager{
				isActive:        true,
				getDriverResult: sess,
			}
			runner := &mockInvestigatorRunner{response: "the pod OOMKilled"}
			recon := &mockContextReconstructor{}

			// A LazySink with an already-activated channel, exactly as
			// production's Subscribe() would have set it up for a session
			// with a live observer (#1384's Subscribe call).
			ch := make(chan session.InvestigationEvent, 1)
			ls := &session.LazySink{}
			ls.Set(ch)

			autoMgr := &mockAutoMgrWithHTTPSession{
				httpSessionID: "http-sess-1639-001",
				lazySink:      ls,
			}
			completer := &mockHTTPCompleter{
				foundID: "http-sess-1639-001",
				found:   true,
			}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr,
				mcptools.WithHTTPCompleter(completer))

			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:    "rr-1639-001",
				Action:  mcptools.ActionMessage,
				Message: "what caused this?",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Response).To(Equal("the pod OOMKilled"))

			capturedCtx := runner.capturedCtx
			Expect(capturedCtx).NotTo(BeNil(), "RunInteractiveTurn should have been called")

			sessionID := session.SessionIDFromContext(capturedCtx)
			Expect(sessionID).To(Equal("http-sess-1639-001"),
				"UT-KA-1639-001: action=message must propagate session_id for audit correlation (AU-2/AU-3), same as discover_workflows (#1384)")

			sink := session.EventSinkFromContext(capturedCtx)
			Expect(sink).NotTo(BeNil(),
				"UT-KA-1639-001: action=message must attach the session's LazySink so live KA events "+
					"(reasoning_delta, reasoning_content_delta, etc.) can actually stream (BR-AI-086, #1639) — "+
					"this is the KA-side half; #1637 is the AF-side relay half")
			Expect(sink).To(Equal((chan<- session.InvestigationEvent)(ch)),
				"the attached sink must be the session's own event channel, not a stand-in")
		})
	})

	Describe("UT-KA-1639-002: action=message without an HTTP session degrades gracefully (no crash, no sink)", func() {
		It("should still succeed with no session context when no HTTP session exists", func() {
			sess := &mcpinternal.InteractiveSession{
				SessionID:     "mcp-sess-1639-002",
				CorrelationID: "rr-1639-002",
				ActingUser:    mcpinternal.UserInfo{Username: "alice"},
			}
			sessionMgr := &mockSessionManager{
				isActive:        true,
				getDriverResult: sess,
			}
			runner := &mockInvestigatorRunner{response: "ok"}
			recon := &mockContextReconstructor{}
			autoMgr := &mockAutoMgrWithHTTPSession{} // no httpSessionID set

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr)

			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:    "rr-1639-002",
				Action:  mcptools.ActionMessage,
				Message: "hello",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Response).To(Equal("ok"))

			Expect(session.EventSinkFromContext(runner.capturedCtx)).To(BeNil(),
				"UT-KA-1639-002: no HTTP session found must degrade gracefully with no sink attached, not panic or error")
		})
	})
})
