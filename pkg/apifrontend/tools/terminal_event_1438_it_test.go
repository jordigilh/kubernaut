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

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("IT #1438 — AF terminal event wiring through production path", func() {

	It("IT-AF-1438-020 (AU-3, SC-7): Pool InjectWithCleanup -> watchTerminalEvents -> session_ended -> A2A queue", func() {
		queue := &bridgeQueue{}
		ctx := launcher.WithEventBridge(context.Background(), queue, "task-it-1438-020", "ctx-it-020", nil)

		eventCh := make(chan ka.InvestigationEvent, 5)
		done := make(chan struct{})

		pool := ka.NewKASessionPool(ka.PoolConfig{
			Factory: func(_ context.Context) (ka.PoolSession, error) {
				return &mockPoolSession{}, nil
			},
			MaxEntries: 10,
			Logger:     logr.Discard(),
		})

		pool.InjectWithCleanup("rr-it-1438-020", "alice", &mockPoolSession{}, func() {
			close(done)
		})

		watchCtx := context.WithoutCancel(ctx)
		exited := make(chan struct{})
		go func() {
			tools.WatchTerminalEvents(watchCtx, eventCh, "rr-it-1438-020", done)
			close(exited)
		}()

		time.Sleep(50 * time.Millisecond)

		eventCh <- ka.InvestigationEvent{
			Type:  ka.EventTypeSessionEnded,
			Phase: "inactivity_timeout",
		}

		Eventually(exited, 3*time.Second).Should(BeClosed(),
			"watcher must exit after session_ended")

		allEvents := queue.Events()
		Expect(allEvents).NotTo(BeEmpty(),
			"AU-3: A2A queue must contain terminal status event")

		var foundTerminal bool
		for _, evt := range allEvents {
			statusEvt, ok := evt.(*a2a.TaskStatusUpdateEvent)
			if !ok {
				continue
			}
			if statusEvt.Metadata == nil {
				continue
			}
			metaType, _ := statusEvt.Metadata["type"].(string)
			terminal, _ := statusEvt.Metadata["terminal"].(bool)
			if metaType == launcher.MetaTypeInvestigation && terminal {
				foundTerminal = true
				Expect(statusEvt.Metadata["phase"]).To(Equal("TimedOut"),
					"AU-3: phase must be mapped from inactivity_timeout -> TimedOut")
				Expect(statusEvt.Metadata["reason"]).To(Equal("inactivity_timeout"),
					"AU-3: reason must carry the raw release reason")
				tp, ok := statusEvt.Status.Message.Parts[0].(a2a.TextPart)
				Expect(ok).To(BeTrue())
				Expect(tp.Text).To(ContainSubstring("Session ended"),
					"user-facing text must include session ended")
				break
			}
		}
		Expect(foundTerminal).To(BeTrue(),
			"IT-AF-1438-020: terminal status event with metadata.type=investigation and terminal=true must reach A2A queue")
	})

	It("IT-AF-1438-030 (SI-4, AU-3): Full blocking investigate -> simulated timeout -> A2A queue receives terminal event", func() {
		eventCh := make(chan ka.InvestigationEvent, 10)
		sess := &mockPoolSession{}

		eventCh <- ka.InvestigationEvent{
			Type: ka.EventTypeComplete,
			Data: json.RawMessage(`{"severity":"warning","confidence":0.85,"rca_summary":"pod crash loop","target":"Deployment/api"}`),
		}

		mockMCP := &ka.MockMCPClient{
			StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
				return &ka.StartInvestigationResult{
					SessionID: "sess-it-1438-030",
					Status:    "started",
					Events:    eventCh,
					Closer:    func() {},
					Session:   sess,
				}, nil
			},
		}

		pool := ka.NewKASessionPool(ka.PoolConfig{
			Factory: func(_ context.Context) (ka.PoolSession, error) {
				return &mockPoolSession{}, nil
			},
			MaxEntries: 10,
			Logger:     logr.Discard(),
		})

		queue := &bridgeQueue{}
		ctx := launcher.WithEventBridge(context.Background(), queue, "task-it-1438-030", "ctx-it-030", nil)
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		_, err := tools.HandleInvestigationMCPWithRegistry(
			ctx, mockMCP, nil, "",
			tools.InvestigateMCPArgs{RRID: "rr-it-1438-030"},
			nil, nil, nil, true, pool, "alice", nil, nil,
		)
		Expect(err).NotTo(HaveOccurred())

		time.Sleep(50 * time.Millisecond)

		eventCh <- ka.InvestigationEvent{
			Type:  ka.EventTypeSessionEnded,
			Phase: "inactivity_timeout",
		}

		Eventually(func() bool {
			for _, evt := range queue.Events() {
				statusEvt, ok := evt.(*a2a.TaskStatusUpdateEvent)
				if !ok {
					continue
				}
				if statusEvt.Metadata == nil {
					continue
				}
				metaType, _ := statusEvt.Metadata["type"].(string)
				terminal, _ := statusEvt.Metadata["terminal"].(bool)
				if metaType == launcher.MetaTypeInvestigation && terminal {
					return true
				}
			}
			return false
		}, 5*time.Second).Should(BeTrue(),
			"IT-AF-1438-030: terminal status event with phase=TimedOut must appear in A2A queue after simulated timeout")
	})
})
