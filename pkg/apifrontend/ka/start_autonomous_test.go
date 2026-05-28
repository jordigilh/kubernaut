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

package ka_test

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
)

var _ = Describe("MCPClient — StartAutonomous (#1326 BR-MCP-002)", func() {

	Describe("UT-AF-1326-010: MCPClient interface includes StartAutonomous", func() {
		It("should satisfy the interface with StartAutonomous method", func() {
			var client ka.MCPClient
			mock := &ka.MockMCPClient{
				StartAutonomousFn: func(_ context.Context, args ka.StartAutonomousArgs) (*ka.StartAutonomousResult, error) {
					return &ka.StartAutonomousResult{
						SessionID: "sess-auto-001",
						Status:    "autonomous_started",
					}, nil
				},
			}
			client = mock
			Expect(client).NotTo(BeNil())

			result, err := client.StartAutonomous(context.Background(), ka.StartAutonomousArgs{
				RRID: "rr-auto-001",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.SessionID).To(Equal("sess-auto-001"))
			Expect(result.Status).To(Equal("autonomous_started"))
		})
	})

	Describe("UT-AF-1326-011: StartAutonomous returns event channel for streaming", func() {
		It("should provide an event channel when EventChannelFn is set", func() {
			eventCh := make(chan ka.InvestigationEvent, 10)
			mock := &ka.MockMCPClient{
				StartAutonomousFn: func(_ context.Context, args ka.StartAutonomousArgs) (*ka.StartAutonomousResult, error) {
					return &ka.StartAutonomousResult{
						SessionID: "sess-stream-001",
						Status:    "autonomous_started",
						Events:    eventCh,
					}, nil
				},
			}

			result, err := mock.StartAutonomous(context.Background(), ka.StartAutonomousArgs{
				RRID: "rr-stream-001",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Events).NotTo(BeNil())

			eventCh <- ka.InvestigationEvent{
				Type: "reasoning_delta",
				Turn: 1,
				Data: json.RawMessage(`{"text":"Analyzing..."}`),
			}

			evt := <-result.Events
			Expect(evt.Type).To(Equal("reasoning_delta"))
			Expect(evt.Turn).To(Equal(1))
		})
	})

	Describe("UT-AF-1326-012: StartAutonomous returns closer for cleanup", func() {
		It("should provide a closer function to terminate the streaming session", func() {
			closerCalled := false
			mock := &ka.MockMCPClient{
				StartAutonomousFn: func(_ context.Context, args ka.StartAutonomousArgs) (*ka.StartAutonomousResult, error) {
					return &ka.StartAutonomousResult{
						SessionID: "sess-closer-001",
						Status:    "autonomous_started",
						Closer: func() {
							closerCalled = true
						},
					}, nil
				},
			}

			result, err := mock.StartAutonomous(context.Background(), ka.StartAutonomousArgs{
				RRID: "rr-closer-001",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Closer).NotTo(BeNil())

			result.Closer()
			Expect(closerCalled).To(BeTrue())
		})
	})

	Describe("UT-AF-1326-013: StartAutonomous propagates error on MCP failure", func() {
		It("should return error when MCP connection fails", func() {
			mock := &ka.MockMCPClient{
				StartAutonomousFn: func(_ context.Context, args ka.StartAutonomousArgs) (*ka.StartAutonomousResult, error) {
					return nil, ka.ErrMCPUnavailable
				},
			}

			_, err := mock.StartAutonomous(context.Background(), ka.StartAutonomousArgs{
				RRID: "rr-fail-001",
			})
			Expect(err).To(MatchError(ka.ErrMCPUnavailable))
		})
	})

	Describe("UT-AF-1326-014: StartAutonomous nil mock returns error", func() {
		It("should return ErrMCPUnavailable when StartAutonomousFn is nil", func() {
			mock := &ka.MockMCPClient{}

			_, err := mock.StartAutonomous(context.Background(), ka.StartAutonomousArgs{
				RRID: "rr-nil-001",
			})
			Expect(err).To(MatchError(ka.ErrMCPUnavailable))
		})
	})
})
