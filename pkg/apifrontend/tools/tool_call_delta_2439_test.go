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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("AF tool-call delta relay — #2439", func() {
	It("IT-AF-2439-001: preserves the structured delta fields at the A2A boundary", func() {
		queue := &bridgeQueue{}
		ctx := launcher.WithEventBridge(context.Background(), queue, "task-2439", "ctx-2439", nil)
		events := make(chan ka.InvestigationEvent, 2)
		events <- ka.InvestigationEvent{
			Type:  ka.EventTypeToolCallDelta,
			Turn:  3,
			Phase: "rca",
			Data:  json.RawMessage(`{"index":0,"id":"call-1","name":"kubectl_get","arguments_delta":"{\"kind\":\"Pod\"}"}`),
		}
		close(events)

		_, _, _, _ = tools.BridgeEventsCollectSummary(ctx, events, time.Second)

		queued := queue.Events()
		Expect(queued).NotTo(BeEmpty(), "tool-call deltas must not be silently dropped by AF")
		var found bool
		for _, event := range queued {
			raw, _ := json.Marshal(event)
			if containsAll(string(raw), "tool_call_delta", "kubectl_get", "call-1", "arguments_delta") {
				found = true
			}
		}
		Expect(found).To(BeTrue(), "A2A event must preserve the structured tool-call delta payload")
	})
})
