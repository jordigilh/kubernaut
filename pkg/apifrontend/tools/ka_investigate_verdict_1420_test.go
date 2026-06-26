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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

var _ = Describe("Issue #1420: AF Bridge Alignment Verdict Handling (SC-7)", func() {

	It("IT-AF-1420-001: bridgeEventsCollectSummary emits structured alignment_check_failed event", func() {
		events := make(chan ka.InvestigationEvent, 5)

		verdictPayload := katypes.AlignmentVerdictResult{
			Result:                  "suspicious",
			CircuitBreakerActivated: true,
			Summary:                 "prompt injection detected in step 7",
			Flagged:                 1,
			Total:                   9,
			Findings: []katypes.AlignmentFinding{
				{StepIndex: 7, StepKind: "tool_output", Tool: "kubectl_get_by_name", Explanation: "authority impersonation"},
			},
		}
		verdictJSON, err := json.Marshal(verdictPayload)
		Expect(err).NotTo(HaveOccurred())

		events <- ka.InvestigationEvent{
			Type: ka.EventTypeAlignmentVerdict,
			Data: verdictJSON,
		}
		events <- ka.InvestigationEvent{
			Type: ka.EventTypeComplete,
		}

		q := &bridgeQueue{}
		taskID := a2a.NewTaskID()
		ctx := launcher.WithEventBridge(context.Background(), q, taskID, "ctx-1420", nil)
		ctx = tools.WithRRID(ctx, "rr-1420-001")

		summary, _, _ := tools.BridgeEventsCollectSummary(ctx, events, 5*time.Second)
		_ = summary

		found := false
		for _, evt := range q.Events() {
			statusEvt, ok := evt.(*a2a.TaskStatusUpdateEvent)
			if !ok {
				continue
			}
			meta := statusEvt.Metadata
			if meta == nil {
				continue
			}
			if metaType, ok := meta["type"].(string); ok && metaType == launcher.MetaTypeAlignmentCheckFailed {
				found = true
				Expect(meta["rr_id"]).To(Equal("rr-1420-001"))
			}
		}

		Expect(found).To(BeTrue(),
			"SC-7.a: bridgeEventsCollectSummary must emit alignment_check_failed structured event")
	})

	It("IT-AF-1420-002: structured event includes rr_id in metadata", func() {
		events := make(chan ka.InvestigationEvent, 5)

		verdictPayload := katypes.AlignmentVerdictResult{
			Result:                  "suspicious",
			CircuitBreakerActivated: true,
			Summary:                 "prompt injection detected",
			Flagged:                 1,
			Total:                   5,
		}
		verdictJSON, err := json.Marshal(verdictPayload)
		Expect(err).NotTo(HaveOccurred())

		events <- ka.InvestigationEvent{
			Type: ka.EventTypeAlignmentVerdict,
			Data: verdictJSON,
		}
		events <- ka.InvestigationEvent{
			Type: ka.EventTypeComplete,
		}

		q := &bridgeQueue{}
		taskID := a2a.NewTaskID()
		ctx := launcher.WithEventBridge(context.Background(), q, taskID, "ctx-1420-002", nil)
		ctx = tools.WithRRID(ctx, "rr-1420-002")

		_, _, _ = tools.BridgeEventsCollectSummary(ctx, events, 5*time.Second)

		found := false
		for _, evt := range q.Events() {
			statusEvt, ok := evt.(*a2a.TaskStatusUpdateEvent)
			if !ok {
				continue
			}
			meta := statusEvt.Metadata
			if meta == nil {
				continue
			}
			if metaType, ok := meta["type"].(string); ok && metaType == launcher.MetaTypeAlignmentCheckFailed {
				found = true
				Expect(meta).To(HaveKey("rr_id"))
				Expect(meta["rr_id"]).To(Equal("rr-1420-002"))
			}
		}

		Expect(found).To(BeTrue(),
			"SC-7.a: structured event must include rr_id for console context association")
	})

	It("IT-AF-1420-003: MetaTypeAlignmentCheckFailed constant matches console contract", func() {
		Expect(launcher.MetaTypeAlignmentCheckFailed).To(Equal("alignment_check_failed"),
			"SC-7.a: constant must match the value agreed with console team in issue #1420")
	})
})
