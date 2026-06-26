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
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("bridgeEventsCollectSummary exit reason — #1451 (BR-AF-MCP-003)", func() {

	Describe("Exit Path Accuracy (SI-4)", func() {

		It("UT-AF-1451-001: no events within inactivity timeout → inactivity_timeout", func() {
			events := make(chan ka.InvestigationEvent, 5)
			ctx := context.Background()

			_, _, exitReason := tools.BridgeEventsCollectSummary(ctx, events, 50*time.Millisecond)

			Expect(exitReason).To(Equal("inactivity_timeout"),
				"SI-4: bridge must report inactivity_timeout when no events arrive")
		})

		It("UT-AF-1451-002: channel closes normally → channel_closed", func() {
			events := make(chan ka.InvestigationEvent, 5)
			close(events)
			ctx := context.Background()

			_, _, exitReason := tools.BridgeEventsCollectSummary(ctx, events, 5*time.Second)

			Expect(exitReason).To(Equal("channel_closed"),
				"SI-4: bridge must report channel_closed when events channel closes normally")
		})

		It("UT-AF-1451-003: context cancelled → ctx_cancelled", func() {
			events := make(chan ka.InvestigationEvent, 5)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			_, _, exitReason := tools.BridgeEventsCollectSummary(ctx, events, 5*time.Second)

			Expect(exitReason).To(Equal("ctx_cancelled"),
				"SI-4: bridge must report ctx_cancelled when context is done")
		})
	})

	Describe("Status Mapping (AU-3)", func() {

		It("UT-AF-1451-004: inactivity_timeout maps to timed_out status", func() {
			events := make(chan ka.InvestigationEvent, 5)
			ctx := context.Background()

			_, _, exitReason := tools.BridgeEventsCollectSummary(ctx, events, 50*time.Millisecond)
			Expect(exitReason).To(Equal("inactivity_timeout"))

			status := tools.ExitReasonToStatus(exitReason)
			Expect(status).To(Equal("timed_out"),
				"AU-3: inactivity_timeout must map to timed_out so LLM knows investigation is hung")
		})

		It("UT-AF-1451-005: channel_closed maps to completed status", func() {
			events := make(chan ka.InvestigationEvent, 5)
			close(events)
			ctx := context.Background()

			_, _, exitReason := tools.BridgeEventsCollectSummary(ctx, events, 5*time.Second)
			Expect(exitReason).To(Equal("channel_closed"))

			status := tools.ExitReasonToStatus(exitReason)
			Expect(status).To(Equal("completed"),
				"AU-3: channel_closed must map to completed — natural investigation end")
		})

		It("UT-AF-1451-006: ctx_cancelled maps to timeout status", func() {
			events := make(chan ka.InvestigationEvent, 5)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			_, _, exitReason := tools.BridgeEventsCollectSummary(ctx, events, 5*time.Second)
			Expect(exitReason).To(Equal("ctx_cancelled"))

			status := tools.ExitReasonToStatus(exitReason)
			Expect(status).To(Equal("timeout"),
				"AU-3: ctx_cancelled must map to timeout — external cancellation")
		})
	})

	Describe("Timer Behavior (SI-4)", func() {

		It("UT-AF-1451-007: default BridgeInactivityTimeout is 180s", func() {
			Expect(tools.BridgeInactivityTimeout).To(Equal(180*time.Second),
				"SI-4: default inactivity timeout must be 180s to avoid premature hang detection")
		})

		It("UT-AF-1451-008: timer resets on event arrival", func() {
			events := make(chan ka.InvestigationEvent, 5)
			ctx := context.Background()

			go func() {
				time.Sleep(40 * time.Millisecond)
				events <- ka.InvestigationEvent{
					Type: ka.EventTypeReasoningDelta,
					Data: json.RawMessage(`{"text":"thinking..."}`),
				}
				time.Sleep(40 * time.Millisecond)
				close(events)
			}()

			_, _, exitReason := tools.BridgeEventsCollectSummary(ctx, events, 50*time.Millisecond)

			Expect(exitReason).To(Equal("channel_closed"),
				"SI-4: event arrival must reset inactivity timer; channel close at T+80ms with 50ms timeout should not trigger timeout")
		})
	})

	Describe("Audit Emission on Timeout (AU-3)", func() {

		It("UT-AF-1451-E01: ExitReasonToStatus returns timed_out for inactivity_timeout", func() {
			Expect(tools.ExitReasonToStatus(tools.ExitReasonInactivityTimeout)).To(Equal("timed_out"))
		})

		It("UT-AF-1451-E02: ExitReasonToStatus returns completed for channel_closed", func() {
			Expect(tools.ExitReasonToStatus(tools.ExitReasonChannelClosed)).To(Equal("completed"))
		})

		It("UT-AF-1451-E03: ExitReasonToStatus returns timeout for ctx_cancelled", func() {
			Expect(tools.ExitReasonToStatus(tools.ExitReasonCtxCancelled)).To(Equal("timeout"))
		})
	})

	Describe("Data Preservation on Timeout (AU-3)", func() {

		It("UT-AF-1451-009: summary text accumulated before timeout is preserved", func() {
			events := make(chan ka.InvestigationEvent, 5)
			ctx := context.Background()

			events <- ka.InvestigationEvent{
				Type: ka.EventTypeReasoningDelta,
				Data: json.RawMessage(`{"text":"partial analysis of the issue"}`),
			}

			summary, _, exitReason := tools.BridgeEventsCollectSummary(ctx, events, 50*time.Millisecond)

			Expect(exitReason).To(Equal("inactivity_timeout"))
			Expect(summary).To(ContainSubstring("partial analysis of the issue"),
				"AU-3: accumulated summary text must be preserved even on inactivity timeout")
		})

		It("UT-AF-1451-010: RCA data preserved on timeout", func() {
			events := make(chan ka.InvestigationEvent, 5)
			ctx := context.Background()

			rcaPayload := map[string]interface{}{
				"severity":    "critical",
				"confidence":  0.91,
				"rca_summary": "OOMKill from memory leak",
				"target":      "Deployment/api in production",
			}
			rcaJSON, err := json.Marshal(rcaPayload)
			Expect(err).NotTo(HaveOccurred())

			events <- ka.InvestigationEvent{
				Type: ka.EventTypeComplete,
				Data: rcaJSON,
			}

			_, rca, exitReason := tools.BridgeEventsCollectSummary(ctx, events, 5*time.Second)

			Expect(exitReason).To(Equal("channel_closed"),
				"EventTypeComplete triggers immediate return with channel_closed")
			Expect(rca).NotTo(BeNil(), "AU-3: RCA data must be preserved")
			Expect(rca.Severity).To(Equal("critical"))
			Expect(rca.RCASummary).To(Equal("OOMKill from memory leak"))
		})
	})
})
