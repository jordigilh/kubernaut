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

package mcpclient_test

import (
	"context"
	"encoding/json"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
)

type mockSession struct {
	callResult *mcp.CallToolResult
	callErr    error
	lastParams *mcp.CallToolParams
}

func (s *mockSession) CallTool(_ context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	s.lastParams = params
	if s.callErr != nil {
		return nil, s.callErr
	}
	return s.callResult, nil
}

var _ = Describe("BridgeTool (BR-INTEGRATION-065)", func() {
	var (
		session *mockSession
		bt      *mcpclient.BridgeTool
	)

	BeforeEach(func() {
		session = &mockSession{
			callResult: &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: `{"pods":["nginx","redis"]}`}},
			},
		}
		bt = mcpclient.NewBridgeTool(mcpclient.ToolDefinition{
			Name:        "prod-east__get_pods",
			Description: "List pods in prod-east cluster",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"namespace":{"type":"string"}}}`),
		}, "prod-east", session)
	})

	Describe("Interface compliance", func() {
		It("UT-FLEET-BT-001: implements tools.Tool interface", func() {
			var _ tools.Tool = bt
		})

		It("UT-FLEET-BT-002: exposes Name, Description, Parameters", func() {
			Expect(bt.Name()).To(Equal("prod-east__get_pods"))
			Expect(bt.Description()).To(Equal("List pods in prod-east cluster"))
			Expect(bt.Parameters()).To(MatchJSON(`{"type":"object","properties":{"namespace":{"type":"string"}}}`))
		})
	})

	Describe("Execute", func() {
		It("UT-FLEET-BT-003: calls session with correct params and returns text", func() {
			args := json.RawMessage(`{"namespace":"kube-system"}`)
			result, err := bt.Execute(context.Background(), args)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(ContainSubstring("nginx"))

			Expect(session.lastParams.Name).To(Equal("prod-east__get_pods"))
			Expect(session.lastParams.Arguments).To(HaveKeyWithValue("namespace", "kube-system"))
		})

		It("UT-FLEET-BT-004: handles nil/null args gracefully", func() {
			result, err := bt.Execute(context.Background(), nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(ContainSubstring("nginx"))
			Expect(session.lastParams.Arguments).To(BeNil())
		})

		It("UT-FLEET-BT-005: returns error when session call fails", func() {
			session.callErr = errors.New("connection refused")
			_, err := bt.Execute(context.Background(), nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("prod-east"))
			Expect(err.Error()).To(ContainSubstring("connection refused"))
		})

		It("UT-FLEET-BT-006: returns error when remote tool reports error", func() {
			session.callResult = &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "resource not found"}},
			}
			_, err := bt.Execute(context.Background(), nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("resource not found"))
		})

		It("UT-FLEET-BT-007: returns error for invalid JSON args", func() {
			_, err := bt.Execute(context.Background(), json.RawMessage(`{invalid`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unmarshal"))
		})
	})
})
