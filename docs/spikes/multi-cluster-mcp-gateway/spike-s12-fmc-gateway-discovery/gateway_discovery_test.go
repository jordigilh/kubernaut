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

// Package spike_s12 validates that FMC can discover managed clusters by calling
// tools/list on the MCP Gateway and parsing cluster IDs from the
// "{clusterID}__tool_name" prefix convention.
// This code is NOT production code — it lives under docs/spikes/ per project convention.
package spike_s12

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
	gwTestutil "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
)

func TestSpikeS12(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Spike S12 — FMC Gateway-Based Cluster Discovery")
}

// parseClusterIDs extracts unique cluster IDs from an MCP tools/list response
// by splitting tool names on the "__" delimiter. Tools without the prefix
// (gateway-native tools) are ignored.
func parseClusterIDs(tools []*mcp.Tool) []string {
	seen := make(map[string]struct{})
	for _, t := range tools {
		if idx := strings.Index(t.Name, "__"); idx > 0 {
			seen[t.Name[:idx]] = struct{}{}
		}
	}
	result := make([]string, 0, len(seen))
	for id := range seen {
		result = append(result, id)
	}
	return result
}

// clusterToolMap groups tools by cluster ID.
func clusterToolMap(tools []*mcp.Tool) map[string][]string {
	m := make(map[string][]string)
	for _, t := range tools {
		if idx := strings.Index(t.Name, "__"); idx > 0 {
			clusterID := t.Name[:idx]
			toolName := t.Name[idx+2:]
			m[clusterID] = append(m[clusterID], toolName)
		}
	}
	return m
}

func mustConnect(ctx context.Context, url string) *mcp.ClientSession {
	client := mcp.NewClient(&mcp.Implementation{Name: "spike-s12-test", Version: "v1.0.0"}, nil)
	transport := &mcp.StreamableClientTransport{Endpoint: url + "/mcp"}
	session, err := client.Connect(ctx, transport, nil)
	Expect(err).ToNot(HaveOccurred(), "failed to connect to mock gateway at %s", url)
	return session
}

var _ = Describe("Spike S12 — FMC Gateway-Based Cluster Discovery", Ordered, func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	})

	AfterEach(func() {
		cancel()
	})

	// --- S12-001: tools/list returns all cluster-prefixed tools ---

	It("S12-001: tools/list returns all cluster-prefixed tools from the gateway", func() {
		gw := gwTestutil.NewMockGateway(
			gwTestutil.WithMultiCluster("prod_east", "prod_west", "staging"),
		)
		defer gw.Close()

		session := mustConnect(ctx, gw.URL())
		defer session.Close()

		result, err := session.ListTools(ctx, nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Tools).ToNot(BeEmpty())

		toolNames := make([]string, len(result.Tools))
		for i, t := range result.Tools {
			toolNames[i] = t.Name
		}
		GinkgoWriter.Printf("S12-001: tools/list returned %d tools: %v\n", len(toolNames), toolNames)

		Expect(toolNames).To(ContainElement("prod_east__get_resource"),
			"gateway must expose cluster-prefixed get_resource for prod_east")
		Expect(toolNames).To(ContainElement("prod_east__list_resources"),
			"gateway must expose cluster-prefixed list_resources for prod_east")
		Expect(toolNames).To(ContainElement("prod_west__get_resource"),
			"gateway must expose cluster-prefixed get_resource for prod_west")
		Expect(toolNames).To(ContainElement("prod_west__list_resources"),
			"gateway must expose cluster-prefixed list_resources for prod_west")
		Expect(toolNames).To(ContainElement("staging__get_resource"),
			"gateway must expose cluster-prefixed get_resource for staging")
		Expect(toolNames).To(ContainElement("staging__list_resources"),
			"gateway must expose cluster-prefixed list_resources for staging")

		Expect(result.Tools).To(HaveLen(6),
			"3 clusters × 2 tools (get_resource, list_resources) = 6 total tools")
	})

	// --- S12-002: Parse unique cluster IDs from tool names ---

	It("S12-002: extracts unique cluster IDs from cluster-prefixed tool names", func() {
		gw := gwTestutil.NewMockGateway(
			gwTestutil.WithMultiCluster("alpha", "bravo", "charlie"),
		)
		defer gw.Close()

		session := mustConnect(ctx, gw.URL())
		defer session.Close()

		result, err := session.ListTools(ctx, nil)
		Expect(err).ToNot(HaveOccurred())

		clusterIDs := parseClusterIDs(result.Tools)
		GinkgoWriter.Printf("S12-002: Parsed cluster IDs: %v\n", clusterIDs)

		Expect(clusterIDs).To(ConsistOf("alpha", "bravo", "charlie"),
			"parseClusterIDs must return exactly the 3 registered clusters")

		byCluster := clusterToolMap(result.Tools)
		for _, id := range clusterIDs {
			tools, ok := byCluster[id]
			Expect(ok).To(BeTrue(), "cluster %s must have tools", id)
			Expect(tools).To(ContainElement("get_resource"),
				"cluster %s must expose get_resource", id)
			Expect(tools).To(ContainElement("list_resources"),
				"cluster %s must expose list_resources", id)
			GinkgoWriter.Printf("  Cluster %s: tools = %v\n", id, tools)
		}
	})

	// --- S12-003: Scale — tools/list at 50 clusters ---

	It("S12-003: tools/list scales to 50 clusters with acceptable parse time", func() {
		clusterNames := make([]string, 50)
		for i := range clusterNames {
			clusterNames[i] = fmt.Sprintf("cluster_%03d", i)
		}

		gw := gwTestutil.NewMockGateway(
			gwTestutil.WithMultiCluster(clusterNames...),
		)
		defer gw.Close()

		session := mustConnect(ctx, gw.URL())
		defer session.Close()

		start := time.Now()
		result, err := session.ListTools(ctx, nil)
		listDuration := time.Since(start)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Tools).To(HaveLen(100),
			"50 clusters × 2 tools = 100 total tools")

		var memBefore, memAfter runtime.MemStats
		runtime.ReadMemStats(&memBefore)

		parseStart := time.Now()
		clusterIDs := parseClusterIDs(result.Tools)
		parseDuration := time.Since(parseStart)

		runtime.ReadMemStats(&memAfter)
		allocDelta := memAfter.TotalAlloc - memBefore.TotalAlloc

		GinkgoWriter.Printf("S12-003: Scale metrics (50 clusters, 100 tools):\n")
		GinkgoWriter.Printf("  tools/list RPC:    %v\n", listDuration)
		GinkgoWriter.Printf("  Cluster ID parse:  %v\n", parseDuration)
		GinkgoWriter.Printf("  Memory alloc:      %d bytes\n", allocDelta)
		GinkgoWriter.Printf("  Tool count:        %d\n", len(result.Tools))
		GinkgoWriter.Printf("  Cluster count:     %d\n", len(clusterIDs))

		toolsJSON, _ := json.Marshal(result.Tools)
		GinkgoWriter.Printf("  Payload size:      %d bytes (~%.1f KB)\n",
			len(toolsJSON), float64(len(toolsJSON))/1024)

		Expect(clusterIDs).To(HaveLen(50),
			"must discover exactly 50 unique clusters")
		Expect(parseDuration).To(BeNumerically("<", 10*time.Millisecond),
			"cluster ID parsing must complete in <10ms for 50 clusters")
	})

	// --- S12-004: Full pipeline — tools/list → clusters → list_resources → scope keys ---

	It("S12-004: full pipeline builds scope keys from gateway tool discovery", func() {
		gw := gwTestutil.NewMockGateway(
			gwTestutil.WithMultiCluster("hub_east", "hub_west"),
		)
		defer gw.Close()

		session := mustConnect(ctx, gw.URL())
		defer session.Close()

		// Step 1: Discover clusters via tools/list
		toolsResult, err := session.ListTools(ctx, nil)
		Expect(err).ToNot(HaveOccurred())

		clusterIDs := parseClusterIDs(toolsResult.Tools)
		Expect(clusterIDs).To(ConsistOf("hub_east", "hub_west"))

		// Step 2: For each cluster, call {clusterID}__list_resources
		var allKeys []string
		for _, clusterID := range clusterIDs {
			toolName := clusterID + "__list_resources"

			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name: toolName,
				Arguments: map[string]any{
					"kind":          "Deployment",
					"namespace":     "default",
					"labelSelector": "kubernaut.ai/managed=true",
				},
			})
			Expect(err).ToNot(HaveOccurred(),
				"calling %s should not fail", toolName)

			text := extractText(result)
			Expect(text).ToNot(BeEmpty(),
				"list_resources response from %s should contain items", clusterID)

			GinkgoWriter.Printf("S12-004: %s response (truncated): %s\n",
				toolName, truncate(text, 200))

			// Step 3: Parse items and build scope keys
			var listResp struct {
				Items []struct {
					APIVersion string `json:"apiVersion"`
					Kind       string `json:"kind"`
					Metadata   struct {
						Name      string `json:"name"`
						Namespace string `json:"namespace"`
					} `json:"metadata"`
				} `json:"items"`
			}
			err = json.Unmarshal([]byte(text), &listResp)
			Expect(err).ToNot(HaveOccurred(), "list_resources response must be valid JSON")

			for _, item := range listResp.Items {
				parts := strings.SplitN(item.APIVersion, "/", 2)
				group, version := "", parts[0]
				if len(parts) == 2 {
					group, version = parts[0], parts[1]
				}

				key, keyErr := scopecache.BuildKey(
					clusterID, group, version, item.Kind,
					item.Metadata.Namespace, item.Metadata.Name,
				)
				Expect(keyErr).ToNot(HaveOccurred())
				allKeys = append(allKeys, key)
				GinkgoWriter.Printf("  Scope key: %s\n", key)
			}
		}

		Expect(allKeys).ToNot(BeEmpty(),
			"full pipeline must produce at least one scope key")
		for _, key := range allKeys {
			Expect(key).To(HavePrefix("kubernaut:managed:"),
				"all scope keys must follow the kubernaut:managed: prefix convention")
			Expect(key).To(MatchRegexp(`hub_(east|west):`),
				"scope keys must contain one of the discovered cluster IDs")
		}

		GinkgoWriter.Printf("S12-004: Total scope keys built: %d\n", len(allKeys))
	})

	// --- S12-005: Polling-based refresh detects added/removed clusters ---

	It("S12-005: polling detects added and removed clusters across tools/list calls", func() {
		// Start with 2 clusters
		gw := gwTestutil.NewMockGateway(
			gwTestutil.WithMultiCluster("cluster_a", "cluster_b"),
		)
		defer gw.Close()

		session := mustConnect(ctx, gw.URL())
		defer session.Close()

		result1, err := session.ListTools(ctx, nil)
		Expect(err).ToNot(HaveOccurred())
		clusters1 := parseClusterIDs(result1.Tools)
		Expect(clusters1).To(ConsistOf("cluster_a", "cluster_b"),
			"initial poll should find 2 clusters")

		GinkgoWriter.Println("S12-005: Initial cluster set:", clusters1)

		// Simulate adding a third cluster by creating a new gateway
		// (in production, the Gateway adds new backend registrations dynamically)
		session.Close()
		gw.Close()

		gw2 := gwTestutil.NewMockGateway(
			gwTestutil.WithMultiCluster("cluster_a", "cluster_b", "cluster_c"),
		)
		defer gw2.Close()

		session2 := mustConnect(ctx, gw2.URL())
		defer session2.Close()

		result2, err := session2.ListTools(ctx, nil)
		Expect(err).ToNot(HaveOccurred())
		clusters2 := parseClusterIDs(result2.Tools)
		Expect(clusters2).To(ConsistOf("cluster_a", "cluster_b", "cluster_c"),
			"second poll should find 3 clusters after addition")

		// Detect added clusters
		added := diffClusters(clusters2, clusters1)
		Expect(added).To(ConsistOf("cluster_c"),
			"diff should detect cluster_c as newly added")
		GinkgoWriter.Println("S12-005: Added clusters:", added)

		// Simulate removal by creating a gateway with only 2 of the 3
		session2.Close()
		gw2.Close()

		gw3 := gwTestutil.NewMockGateway(
			gwTestutil.WithMultiCluster("cluster_a", "cluster_c"),
		)
		defer gw3.Close()

		session3 := mustConnect(ctx, gw3.URL())
		defer session3.Close()

		result3, err := session3.ListTools(ctx, nil)
		Expect(err).ToNot(HaveOccurred())
		clusters3 := parseClusterIDs(result3.Tools)
		Expect(clusters3).To(ConsistOf("cluster_a", "cluster_c"),
			"third poll should find 2 clusters after removal")

		removed := diffClusters(clusters2, clusters3)
		Expect(removed).To(ConsistOf("cluster_b"),
			"diff should detect cluster_b as removed")
		GinkgoWriter.Println("S12-005: Removed clusters:", removed)
	})
})

func extractText(result *mcp.CallToolResult) string {
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok && tc.Text != "" {
			return tc.Text
		}
	}
	return ""
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// diffClusters returns elements in a that are not in b (set difference a - b).
func diffClusters(a, b []string) []string {
	bSet := make(map[string]struct{}, len(b))
	for _, id := range b {
		bSet[id] = struct{}{}
	}
	var diff []string
	for _, id := range a {
		if _, ok := bSet[id]; !ok {
			diff = append(diff, id)
		}
	}
	return diff
}
