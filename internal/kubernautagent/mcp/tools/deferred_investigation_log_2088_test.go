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

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
)

// levelCaptureSink records every Info/Error call with its level, so tests
// can assert the exact log level chosen for a given message. Unlike
// logCaptureSink-style helpers (which discard Error calls entirely), this
// sink is needed to prove a log entry was downgraded from Error to Info,
// not merely that some Info entry exists.
type levelCaptureSink struct {
	entries []levelLogEntry
}

type levelLogEntry struct {
	level string // "info" or "error"
	msg   string
}

func (s *levelCaptureSink) Init(logr.RuntimeInfo)                  {}
func (s *levelCaptureSink) Enabled(int) bool                       { return true }
func (s *levelCaptureSink) WithValues(...interface{}) logr.LogSink { return s }
func (s *levelCaptureSink) WithName(string) logr.LogSink           { return s }
func (s *levelCaptureSink) Error(_ error, msg string, _ ...interface{}) {
	s.entries = append(s.entries, levelLogEntry{level: "error", msg: msg})
}
func (s *levelCaptureSink) Info(_ int, msg string, _ ...interface{}) {
	s.entries = append(s.entries, levelLogEntry{level: "info", msg: msg})
}

// levelFor returns the level of the first captured entry whose message
// contains msg, or ("", false) if none was captured.
func (s *levelCaptureSink) levelFor(msg string) (string, bool) {
	for _, e := range s.entries {
		if e.msg == msg {
			return e.level, true
		}
	}
	return "", false
}

// #2088 (main port of #2086, BR-INTERACTIVE-010, FedRAMP AU-3): handleStart's
// launch-deferred log line was previously always logged at Error level, even
// though session.ErrSessionNotPending is a benign, expected race -- it fires
// when the driving agent retries kubernaut_investigate action=start after a
// prior call already launched the deferred session (exactly the kind of
// retry #2086's false-"completed" report could trigger). An Error-level log
// for an expected, harmless race pollutes on-call alerting/dashboards with
// false-positive noise. Genuine failures (e.g. the deferred function itself
// is missing) must remain at Error.
var _ = Describe("#2088: LaunchDeferredInvestigation benign-retry log level", func() {

	Describe("UT-KA-2088-006: benign ErrSessionNotPending race logs at Info, not Error", func() {
		It("should log the launch-deferred failure at Info level when the session was already launched by a concurrent retry", func() {
			sink := &levelCaptureSink{}
			logger := logr.New(sink)

			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "sess-2088-006",
					CorrelationID: "rr-2088-006",
				},
			}
			autoMgr := &interactiveAutoMgr{
				pendingResult: "http-sess-2088-006",
				pendingOK:     true,
				launchErr:     session.ErrSessionNotPending,
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}
			auditStore := &recordingAuditStore{}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr,
				mcptools.WithAuditStore(auditStore, logger),
			)

			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-2088-006",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "charlie"})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("started"))

			level, found := sink.levelFor("start: failed to launch deferred investigation")
			Expect(found).To(BeTrue(),
				"UT-KA-2088-006: expected a launch-deferred log line to be emitted")
			Expect(level).To(Equal("info"),
				"UT-KA-2088-006: session.ErrSessionNotPending is a benign, expected retry race (#2088) "+
					"and must not be logged at Error level, or on-call alerting/dashboards accumulate "+
					"false-positive noise from routine driving-agent retries")
		})
	})

	Describe("UT-KA-2088-006b: genuine launch failures remain at Error level (regression guard)", func() {
		It("should still log at Error level for a non-benign launch failure", func() {
			sink := &levelCaptureSink{}
			logger := logr.New(sink)

			sessionMgr := &mockSessionManager{
				takeoverSession: &mcpinternal.InteractiveSession{
					SessionID:     "sess-2088-006b",
					CorrelationID: "rr-2088-006b",
				},
			}
			autoMgr := &interactiveAutoMgr{
				pendingResult: "http-sess-2088-006b",
				pendingOK:     true,
				launchErr:     errors.New("deferred investigation function unexpectedly nil"),
			}
			runner := &mockInvestigatorRunner{}
			recon := &mockContextReconstructor{turns: []mcpinternal.ConversationTurn{}}
			auditStore := &recordingAuditStore{}

			tool := mcptools.NewInvestigateTool(sessionMgr, runner, recon, autoMgr,
				mcptools.WithAuditStore(auditStore, logger),
			)

			out, err := tool.Handle(context.Background(), mcptools.InvestigateInput{
				RRID:   "rr-2088-006b",
				Action: mcptools.ActionStart,
			}, mcpinternal.UserInfo{Username: "charlie"})
			Expect(err).NotTo(HaveOccurred())
			Expect(out.Status).To(Equal("started"))

			level, found := sink.levelFor("start: failed to launch deferred investigation")
			Expect(found).To(BeTrue(),
				"UT-KA-2088-006b: expected a launch-deferred log line to be emitted")
			Expect(level).To(Equal("error"),
				"UT-KA-2088-006b: a genuine, unexpected launch failure must remain at Error level so "+
					"real incidents are not silently downgraded alongside the benign retry race")
		})
	})
})
