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

package main

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
)

// fixedDiscoverer implements mcpclient.GatewayDiscoverer, returning a fixed
// set of ToolDefinitions for one clusterID regardless of which gateway wire
// convention those definitions carry -- lets a single test double drive both
// EAIGW-style and Kuadrant-style prefix scenarios against the real
// gatewayOverlayResolver.
type fixedDiscoverer struct {
	clusterID string
	defs      []mcpclient.ToolDefinition
}

func (d *fixedDiscoverer) ListClusters(_ context.Context, _ string) ([]mcpclient.ClusterInfo, error) {
	return nil, nil
}

func (d *fixedDiscoverer) ToolsForCluster(_ context.Context, clusterID string) ([]mcpclient.ToolDefinition, error) {
	if clusterID != d.clusterID {
		return nil, nil
	}
	return d.defs, nil
}

// capturingSession implements mcpclient.Session, recording the tool name
// each CallTool invocation used. This lets a test assert that the wire call
// reaches the gateway under the tool's ORIGINAL, gateway-prefixed name even
// though the LLM only ever sees the generic one (DD-FLEET-005).
type capturingSession struct {
	lastToolName string
}

func (s *capturingSession) CallTool(_ context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	s.lastToolName = params.Name
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil
}

// UT-KA-FLEET-024/025 [AC-4]: gatewayOverlayResolver.Overlay() must generic-ize
// a cluster's discovered tool names identically regardless of which gateway
// wire convention produced them (Issue #1756). Overlay()'s generic-name
// computation must never leak the wire prefix into the LLM-facing tool
// identity, and must never leave the wire identity used for the real gateway
// call ambiguous -- both are AC-4 (information flow enforcement) properties:
// an incorrect generic key here means an investigation's tool calls silently
// resolve against the wrong cluster boundary instead of erroring.
//
// These tests call the real, unexported gatewayOverlayResolver.Overlay()
// directly (not a re-derivation living in another package, which is exactly
// how issue #1756 went undetected) with only the GatewayDiscoverer/Session
// external dependencies faked, per the project's mock-external-only policy.
var _ = Describe("gatewayOverlayResolver.Overlay prefix resolution across gateway conventions (DD-FLEET-005, BR-INTEGRATION-054, BR-INTEGRATION-1489) [AC-4]", func() {
	DescribeTable("generic-izes the tool name regardless of the gateway's wire prefix convention",
		func(clusterID, wireName string) {
			session := &capturingSession{}
			disc := &fixedDiscoverer{
				clusterID: clusterID,
				defs:      []mcpclient.ToolDefinition{{Name: wireName}},
			}
			resolver := &gatewayOverlayResolver{discoverer: disc, session: session}

			overlay, err := resolver.Overlay(context.Background(), clusterID)
			Expect(err).ToNot(HaveOccurred())

			tool, found := overlay["resources_get"]
			Expect(found).To(BeTrue(),
				"DD-FLEET-005/AC-4: the LLM must see this cluster's tool under the generic name "+
					"'resources_get', regardless of the gateway's wire prefix convention -- got keys %v", toolKeys(overlay))
			Expect(tool.Name()).To(Equal("resources_get"))

			_, execErr := tool.Execute(context.Background(), json.RawMessage(`{}`))
			Expect(execErr).ToNot(HaveOccurred())
			Expect(session.lastToolName).To(Equal(wireName),
				"AC-4: the wire call must reach the gateway using the cluster's original prefixed "+
					"name even though the LLM only ever named the generic tool")
		},
		Entry("UT-KA-FLEET-024 [AC-4]: EAIGW convention ({clusterID}__resources_get)",
			"cluster-a", "cluster-a__resources_get"),
		Entry("UT-KA-FLEET-025 [AC-4]: Kuadrant convention (admin-set spec.prefix, NOT {clusterID}__)",
			"prod-east", "prod_east_resources_get"),
	)
})

func toolKeys(overlay map[string]tools.Tool) []string {
	keys := make([]string, 0, len(overlay))
	for k := range overlay {
		keys = append(keys, k)
	}
	return keys
}
