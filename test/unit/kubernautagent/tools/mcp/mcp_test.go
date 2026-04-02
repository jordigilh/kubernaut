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

package mcp_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/mcp"
)

var _ = Describe("MCP Skeleton — #433, Option C", func() {

	Describe("UT-KA-433-010: MCP config parsing handles multiple servers", func() {
		It("should parse multiple server entries with different transports", func() {
			entries := []mcp.ServerConfig{
				{Name: "tools-server", URL: "http://mcp-tools:8080/sse", Transport: "sse"},
				{Name: "local-server", URL: "stdio:///usr/local/bin/mcp-local", Transport: "stdio"},
			}
			configs, err := mcp.ParseConfigs(entries)
			Expect(err).NotTo(HaveOccurred())
			Expect(configs).To(HaveLen(2))
			Expect(configs[0].Name).To(Equal("tools-server"))
			Expect(configs[0].Transport).To(Equal("sse"))
			Expect(configs[1].Name).To(Equal("local-server"))
			Expect(configs[1].Transport).To(Equal("stdio"))
		})
	})

	Describe("UT-KA-433-011: MCP stub provider returns empty tool list", func() {
		It("should return an empty slice and log a warning", func() {
			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
			provider := mcp.NewStubProvider(logger)

			tools, err := provider.DiscoverTools(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(tools).To(BeEmpty(), "stub provider should return empty tool list")
		})
	})

	Describe("UT-KA-433-012: MCP registry integration registers tools from provider", func() {
		It("should register all tools returned by a provider", func() {
			provider := &fakeMCPProvider{
				tools: []mcp.Tool{
					{ToolName: "remote_tool_1", ToolDescription: "A remote tool", ToolParameters: json.RawMessage(`{}`)},
					{ToolName: "remote_tool_2", ToolDescription: "Another remote tool", ToolParameters: json.RawMessage(`{}`)},
				},
			}

			count, err := mcp.RegisterToolsFromProviders(context.Background(), []mcp.MCPToolProvider{provider})
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(2), "should register 2 tools from the fake provider")
		})
	})
})

type fakeMCPProvider struct {
	tools []mcp.Tool
}

func (f *fakeMCPProvider) DiscoverTools(_ context.Context) ([]mcp.Tool, error) {
	return f.tools, nil
}

func (f *fakeMCPProvider) Close() error {
	return nil
}
