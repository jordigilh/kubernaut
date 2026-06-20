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

// Package spike_s7 validates that the FMC Writer can parse MCP list_resources responses
// and extract resource identities for writing to Valkey.
// This code is NOT production code — it lives under docs/spikes/ per project convention.
package spike_s7

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
)

func TestSpikeS7(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Spike S7 — FMC Writer MCP Response Parsing")
}

// ResourceIdentity represents a Kubernetes resource extracted from list_resources response.
type ResourceIdentity struct {
	Group     string
	Version   string
	Kind      string
	Namespace string
	Name      string
}

// BuildValkeyKey constructs the scope cache key from a resource identity and cluster.
func BuildValkeyKey(clusterID string, r ResourceIdentity) string {
	gvr := fmt.Sprintf("%s/%s/%s", r.Group, r.Version, r.Kind)
	nsName := fmt.Sprintf("%s/%s", r.Namespace, r.Name)
	return fmt.Sprintf("kubernaut:managed:%s:%s:%s", clusterID, gvr, nsName)
}

// ParseYAMLListResponse parses a YAML list response from resources_list and extracts
// resource identities. This is the primary parsing path for full YAML responses.
func ParseYAMLListResponse(yamlContent string, group, version, kind string) ([]ResourceIdentity, error) {
	var list struct {
		Items []struct {
			Metadata struct {
				Name      string `yaml:"name"`
				Namespace string `yaml:"namespace"`
			} `yaml:"metadata"`
		} `yaml:"items"`
	}

	if err := yaml.Unmarshal([]byte(yamlContent), &list); err != nil {
		return nil, fmt.Errorf("failed to parse YAML list response: %w", err)
	}

	resources := make([]ResourceIdentity, 0, len(list.Items))
	for _, item := range list.Items {
		if item.Metadata.Name == "" {
			continue
		}
		resources = append(resources, ResourceIdentity{
			Group:     group,
			Version:   version,
			Kind:      kind,
			Namespace: item.Metadata.Namespace,
			Name:      item.Metadata.Name,
		})
	}
	return resources, nil
}

// ParseGoTemplateResponse parses the minimal gotemplate response format:
// "namespace/name\n" per resource.
func ParseGoTemplateResponse(text string, group, version, kind string) ([]ResourceIdentity, error) {
	resources := []ResourceIdentity{}
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "/", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid gotemplate line format: %q (expected namespace/name)", line)
		}
		resources = append(resources, ResourceIdentity{
			Group:     group,
			Version:   version,
			Kind:      kind,
			Namespace: parts[0],
			Name:      parts[1],
		})
	}
	return resources, nil
}

// MCPToolCallResult simulates the structure of mcp.CallToolResult.Content[0].Text
type MCPToolCallResult struct {
	Content []MCPContent `json:"content"`
}

type MCPContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ExtractTextFromMCPResult extracts the text content from an MCP tool call result.
func ExtractTextFromMCPResult(resultJSON string) (string, error) {
	var result MCPToolCallResult
	if err := json.Unmarshal([]byte(resultJSON), &result); err != nil {
		return "", fmt.Errorf("failed to parse MCP result: %w", err)
	}
	for _, c := range result.Content {
		if c.Type == "text" && c.Text != "" {
			return c.Text, nil
		}
	}
	return "", fmt.Errorf("no text content in MCP result")
}

// --- Test suite ---

var _ = Describe("Spike S7 — FMC Writer MCP Response Parsing", func() {
	ctx := context.Background()
	_ = ctx

	Describe("YAML list response parsing", func() {
		It("S7-001: parses standard YAML list and extracts resource identities", func() {
			yamlResponse := `apiVersion: v1
kind: List
items:
- metadata:
    name: nginx
    namespace: default
    labels:
      kubernaut.ai/managed: "true"
      app: nginx
- metadata:
    name: redis
    namespace: prod
    labels:
      kubernaut.ai/managed: "true"
      app: redis
- metadata:
    name: postgres
    namespace: data
    labels:
      kubernaut.ai/managed: "true"
`

			resources, err := ParseYAMLListResponse(yamlResponse, "apps", "v1", "Deployment")
			Expect(err).ToNot(HaveOccurred())
			Expect(resources).To(HaveLen(3))

			Expect(resources[0]).To(Equal(ResourceIdentity{
				Group: "apps", Version: "v1", Kind: "Deployment",
				Namespace: "default", Name: "nginx",
			}))
			Expect(resources[1]).To(Equal(ResourceIdentity{
				Group: "apps", Version: "v1", Kind: "Deployment",
				Namespace: "prod", Name: "redis",
			}))
			Expect(resources[2]).To(Equal(ResourceIdentity{
				Group: "apps", Version: "v1", Kind: "Deployment",
				Namespace: "data", Name: "postgres",
			}))
		})

		It("S7-002: labelSelector filters are applied server-side (we only get managed resources)", func() {
			// The MCP call uses labelSelector=kubernaut.ai/managed=true
			// so the response only contains managed resources — no client filtering needed.
			// This test validates we can construct the correct tool call arguments.
			toolCallArgs := map[string]string{
				"apiVersion":    "apps/v1",
				"kind":          "Deployment",
				"labelSelector": "kubernaut.ai/managed=true",
			}

			argsJSON, err := json.Marshal(toolCallArgs)
			Expect(err).ToNot(HaveOccurred())

			var parsed map[string]string
			Expect(json.Unmarshal(argsJSON, &parsed)).To(Succeed())
			Expect(parsed["labelSelector"]).To(Equal("kubernaut.ai/managed=true"))
			Expect(parsed["apiVersion"]).To(Equal("apps/v1"))
			Expect(parsed["kind"]).To(Equal("Deployment"))
		})

		It("S7-003: gotemplate response parsing (namespace/name format)", func() {
			gotemplateResponse := `default/nginx
prod/redis
data/postgres
`
			resources, err := ParseGoTemplateResponse(gotemplateResponse, "apps", "v1", "Deployment")
			Expect(err).ToNot(HaveOccurred())
			Expect(resources).To(HaveLen(3))

			Expect(resources[0].Namespace).To(Equal("default"))
			Expect(resources[0].Name).To(Equal("nginx"))
			Expect(resources[2].Namespace).To(Equal("data"))
			Expect(resources[2].Name).To(Equal("postgres"))
		})
	})

	Describe("End-to-end: parse → key generation → scope check", func() {
		It("S7-004: full pipeline from MCP response to Valkey key", func() {
			yamlResponse := `apiVersion: v1
kind: List
items:
- metadata:
    name: payment-api
    namespace: production
`
			resources, err := ParseYAMLListResponse(yamlResponse, "apps", "v1", "Deployment")
			Expect(err).ToNot(HaveOccurred())
			Expect(resources).To(HaveLen(1))

			clusterID := "prod-east"
			key := BuildValkeyKey(clusterID, resources[0])
			Expect(key).To(Equal("kubernaut:managed:prod-east:apps/v1/Deployment:production/payment-api"))
		})
	})

	Describe("Error handling", func() {
		It("S7-005a: handles malformed YAML gracefully", func() {
			_, err := ParseYAMLListResponse("not: [valid: yaml: {{}", "apps", "v1", "Deployment")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse YAML"))
		})

		It("S7-005b: handles empty list response", func() {
			yamlResponse := `apiVersion: v1
kind: List
items: []
`
			resources, err := ParseYAMLListResponse(yamlResponse, "apps", "v1", "Deployment")
			Expect(err).ToNot(HaveOccurred())
			Expect(resources).To(BeEmpty())
		})

		It("S7-005c: handles malformed gotemplate line", func() {
			_, err := ParseGoTemplateResponse("invalid-no-slash", "apps", "v1", "Deployment")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid gotemplate line format"))
		})

		It("S7-005d: skips items with empty name", func() {
			yamlResponse := `apiVersion: v1
kind: List
items:
- metadata:
    name: ""
    namespace: default
- metadata:
    name: valid-resource
    namespace: prod
`
			resources, err := ParseYAMLListResponse(yamlResponse, "", "v1", "Pod")
			Expect(err).ToNot(HaveOccurred())
			Expect(resources).To(HaveLen(1))
			Expect(resources[0].Name).To(Equal("valid-resource"))
		})
	})

	Describe("MCP result extraction", func() {
		It("S7-006: extracts text content from MCP CallToolResult JSON", func() {
			mcpResultJSON := `{"content":[{"type":"text","text":"apiVersion: v1\nkind: List\nitems:\n- metadata:\n    name: nginx\n    namespace: default\n"}]}`

			text, err := ExtractTextFromMCPResult(mcpResultJSON)
			Expect(err).ToNot(HaveOccurred())
			Expect(text).To(ContainSubstring("nginx"))

			resources, err := ParseYAMLListResponse(text, "apps", "v1", "Deployment")
			Expect(err).ToNot(HaveOccurred())
			Expect(resources).To(HaveLen(1))
			Expect(resources[0].Name).To(Equal("nginx"))
		})

		It("S7-007: handles empty MCP result", func() {
			mcpResultJSON := `{"content":[{"type":"text","text":""}]}`
			_, err := ExtractTextFromMCPResult(mcpResultJSON)
			Expect(err).To(HaveOccurred())
		})
	})
})
