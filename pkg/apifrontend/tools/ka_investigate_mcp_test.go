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
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("HandleInvestigationMCP — #1326 BR-MCP-002 non-blocking MCP investigate", func() {

	Describe("UT-AF-1326-020: starts autonomous MCP investigation and returns immediately", func() {
		It("should return session_id and autonomous_started status without blocking", func() {
			eventCh := make(chan ka.InvestigationEvent, 10)
			mockMCP := &ka.MockMCPClient{
				StartAutonomousFn: func(_ context.Context, args ka.StartAutonomousArgs) (*ka.StartAutonomousResult, error) {
					Expect(args.RRID).To(Equal("rr-mcp-001"))
					return &ka.StartAutonomousResult{
						SessionID: "sess-mcp-001",
						Status:    "autonomous_started",
						Events:    eventCh,
						Closer:    func() { close(eventCh) },
					}, nil
				},
			}

			result, err := tools.HandleInvestigationMCP(context.Background(), mockMCP, tools.InvestigateMCPArgs{
				RRID: "rr-mcp-001",
			}, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.SessionID).To(Equal("sess-mcp-001"))
			Expect(result.Status).To(Equal("autonomous_started"))
		})
	})

	Describe("UT-AF-1326-021: emits ka.delegated audit event on successful start", func() {
		It("should emit delegation audit event with session_id and rr_id", func() {
			eventCh := make(chan ka.InvestigationEvent, 10)
			mockMCP := &ka.MockMCPClient{
				StartAutonomousFn: func(_ context.Context, _ ka.StartAutonomousArgs) (*ka.StartAutonomousResult, error) {
					return &ka.StartAutonomousResult{
						SessionID: "sess-audit-001",
						Status:    "autonomous_started",
						Events:    eventCh,
						Closer:    func() { close(eventCh) },
					}, nil
				},
			}

			recorder := &auditRecorder{}
			_, err := tools.HandleInvestigationMCP(context.Background(), mockMCP, tools.InvestigateMCPArgs{
				RRID: "rr-audit-001",
			}, recorder)
			Expect(err).NotTo(HaveOccurred())

			Expect(recorder.events).To(HaveLen(1))
			Expect(recorder.events[0].Type).To(Equal(audit.EventKADelegated))
			Expect(recorder.events[0].Detail["delegation_type"]).To(Equal("autonomous_mcp"))
			Expect(recorder.events[0].Detail["session_id"]).To(Equal("sess-audit-001"))
		})
	})

	Describe("UT-AF-1326-022: propagates MCP connection errors", func() {
		It("should return error when MCPClient.StartAutonomous fails", func() {
			mockMCP := &ka.MockMCPClient{
				StartAutonomousFn: func(_ context.Context, _ ka.StartAutonomousArgs) (*ka.StartAutonomousResult, error) {
					return nil, ka.ErrMCPUnavailable
				},
			}

			_, err := tools.HandleInvestigationMCP(context.Background(), mockMCP, tools.InvestigateMCPArgs{
				RRID: "rr-fail-001",
			}, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("MCP"))
		})
	})

	Describe("UT-AF-1326-023: requires rr_id", func() {
		It("should return error when RRID is empty", func() {
			mockMCP := &ka.MockMCPClient{}

			_, err := tools.HandleInvestigationMCP(context.Background(), mockMCP, tools.InvestigateMCPArgs{
				RRID: "",
			}, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("rr_id"))
		})
	})

	Describe("UT-AF-1326-024: MonitorRegistry tracks active sessions", func() {
		It("should register the autonomous session in the monitor registry", func() {
			eventCh := make(chan ka.InvestigationEvent, 10)
			var closerCalled atomic.Int32
			mockMCP := &ka.MockMCPClient{
				StartAutonomousFn: func(_ context.Context, _ ka.StartAutonomousArgs) (*ka.StartAutonomousResult, error) {
					return &ka.StartAutonomousResult{
						SessionID: "sess-monitor-001",
						Status:    "autonomous_started",
						Events:    eventCh,
						Closer:    func() { closerCalled.Add(1) },
					}, nil
				},
			}

			registry := tools.NewMonitorRegistry()
			result, err := tools.HandleInvestigationMCPWithRegistry(context.Background(), mockMCP, tools.InvestigateMCPArgs{
				RRID: "rr-monitor-001",
			}, nil, registry)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.SessionID).To(Equal("sess-monitor-001"))

			Expect(registry.Active("sess-monitor-001")).To(BeTrue())
		})
	})

	Describe("UT-AF-1326-025: MonitorRegistry cancels session on Stop", func() {
		It("should call closer and remove from registry when stopped", func() {
			eventCh := make(chan ka.InvestigationEvent, 10)
			var closerCalled atomic.Int32
			mockMCP := &ka.MockMCPClient{
				StartAutonomousFn: func(_ context.Context, _ ka.StartAutonomousArgs) (*ka.StartAutonomousResult, error) {
					return &ka.StartAutonomousResult{
						SessionID: "sess-stop-001",
						Status:    "autonomous_started",
						Events:    eventCh,
						Closer:    func() { closerCalled.Add(1) },
					}, nil
				},
			}

			registry := tools.NewMonitorRegistry()
			_, err := tools.HandleInvestigationMCPWithRegistry(context.Background(), mockMCP, tools.InvestigateMCPArgs{
				RRID: "rr-stop-001",
			}, nil, registry)
			Expect(err).NotTo(HaveOccurred())

			registry.Stop("sess-stop-001")

			Eventually(func() int32 {
				return closerCalled.Load()
			}, 2*time.Second).Should(BeNumerically(">=", 1))

			Expect(registry.Active("sess-stop-001")).To(BeFalse())
		})
	})

	Describe("UT-AF-1326-100: audit trail completeness — delegation event has all AU-3 fields", func() {
		It("should include session_id, ka_correlation_id, delegation_type, rr_id in audit detail", func() {
			eventCh := make(chan ka.InvestigationEvent, 10)
			mockMCP := &ka.MockMCPClient{
				StartAutonomousFn: func(_ context.Context, _ ka.StartAutonomousArgs) (*ka.StartAutonomousResult, error) {
					return &ka.StartAutonomousResult{
						SessionID: "sess-au3-001",
						Status:    "autonomous_started",
						Events:    eventCh,
						Closer:    func() { close(eventCh) },
					}, nil
				},
			}

			recorder := &auditRecorder{}
			_, err := tools.HandleInvestigationMCP(context.Background(), mockMCP, tools.InvestigateMCPArgs{
				RRID: "rr-au3-001",
			}, recorder)
			Expect(err).NotTo(HaveOccurred())

			Expect(recorder.events).To(HaveLen(1))
			evt := recorder.events[0]
			Expect(evt.Detail).To(HaveKey("session_id"))
			Expect(evt.Detail).To(HaveKey("ka_correlation_id"))
			Expect(evt.Detail).To(HaveKey("delegation_type"))
			Expect(evt.Detail).To(HaveKey("rr_id"))
		})
	})
})

// auditRecorder captures audit events for test assertions.
type auditRecorder struct {
	events []*audit.Event
}

func (r *auditRecorder) Emit(_ context.Context, e *audit.Event) {
	r.events = append(r.events, e)
}

// Ensure auditRecorder satisfies audit.Emitter at compile time (if exported).
var _ audit.Emitter = (*auditRecorder)(nil)

// Suppress unused import warning for json
var _ = json.Marshal
