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

package mcpclient

import (
	"encoding/json"

	"github.com/containers/kubernetes-mcp-server/pkg/output"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// UT-FLEET-MCP-PARSE: parseUnstructured / populateObject
// Authority: BR-FLEET-002 (MCP Gateway client), ADR-068
// FedRAMP: SI-10 (Information Input Validation) -- response parsing correctness
var _ = Describe("UT-FLEET-MCP-PARSE: MCP response parsing", func() {

	Describe("parseUnstructured", func() {
		It("UT-FLEET-MCP-PARSE-001: parses valid JSON into Unstructured", func() {
			obj, err := parseUnstructured(`{"kind":"Pod","metadata":{"name":"nginx","namespace":"default"}}`)
			Expect(err).ToNot(HaveOccurred())
			Expect(obj.GetKind()).To(Equal("Pod"))
			Expect(obj.GetName()).To(Equal("nginx"))
			Expect(obj.GetNamespace()).To(Equal("default"))
		})

		It("UT-FLEET-MCP-PARSE-002: returns error for empty string", func() {
			_, err := parseUnstructured("")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("empty response"))
		})

		It("UT-FLEET-MCP-PARSE-003: returns error for unparseable content", func() {
			_, err := parseUnstructured("\x00\x01binary-garbage\x02\x03")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unmarshaling resource"))
		})

		It("UT-FLEET-MCP-PARSE-003b: parses YAML resource response", func() {
			yamlText := "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: nginx\n  namespace: default\n"
			obj, err := parseUnstructured(yamlText)
			Expect(err).ToNot(HaveOccurred())
			Expect(obj).ToNot(BeNil())
			Expect(obj.GetKind()).To(Equal("Deployment"))
			Expect(obj.GetName()).To(Equal("nginx"))
			Expect(obj.GetNamespace()).To(Equal("default"))
		})
	})

	Describe("parseSelectorToMap", func() {
		It("UT-FLEET-MCP-PARSE-009: parses single key=value", func() {
			m := parseSelectorToMap("app=nginx")
			Expect(m).To(HaveLen(1))
			Expect(m).To(HaveKeyWithValue("app", "nginx"))
		})

		It("UT-FLEET-MCP-PARSE-010: parses multi key=value", func() {
			m := parseSelectorToMap("app=nginx,tier=frontend")
			Expect(m).To(HaveLen(2))
			Expect(m).To(HaveKeyWithValue("app", "nginx"))
			Expect(m).To(HaveKeyWithValue("tier", "frontend"))
		})

		It("UT-FLEET-MCP-PARSE-011: returns nil for empty string", func() {
			m := parseSelectorToMap("")
			Expect(m).To(BeNil())
		})
	})

	Describe("formatLabelSelector", func() {
		It("UT-FLEET-MCP-PARSE-012: formats single label", func() {
			s := formatLabelSelector(map[string]string{"app": "nginx"})
			Expect(s).To(Equal("app=nginx"))
		})
	})

	Describe("ClusterTool", func() {
		It("UT-FLEET-MCP-PARSE-013: prefixes tool with cluster ID", func() {
			Expect(ClusterTool("prod-east", ToolGet)).To(Equal("prod-east__resources_get"))
		})

		It("UT-FLEET-MCP-PARSE-014: prefixes list tool with cluster ID", func() {
			Expect(ClusterTool("staging", ToolList)).To(Equal("staging__resources_list"))
		})
	})

	Describe("ExtractText", func() {
		It("UT-FLEET-MCP-PARSE-015: returns empty for nil result", func() {
			Expect(ExtractText(nil)).To(Equal(""))
		})
	})
})

// UT-FLEET-PARSE: structured content extraction and normalization
// Authority: BR-FLEET-002, ADR-068
// FedRAMP: SI-10 (Information Input Validation) -- validates MCP response parsing
var _ = Describe("UT-FLEET-PARSE: Structured content extraction (BR-FLEET-002)", func() {

	Describe("extractStructuredList [SI-10]", func() {
		It("UT-FLEET-PARSE-028 [SI-10]: returns nil for nil result", func() {
			Expect(extractStructuredList(nil)).To(BeNil())
		})

		It("UT-FLEET-PARSE-028b [SI-10]: returns nil for nil StructuredContent", func() {
			result := &mcp.CallToolResult{}
			Expect(extractStructuredList(result)).To(BeNil())
		})

		It("UT-FLEET-PARSE-028c [SI-10]: returns nil for non-slice StructuredContent", func() {
			result := &mcp.CallToolResult{StructuredContent: "not a slice"}
			Expect(extractStructuredList(result)).To(BeNil())
		})

		It("UT-FLEET-PARSE-028d [SI-10]: extracts valid []any of maps (legacy format)", func() {
			data := []any{
				map[string]any{"kind": "Pod", "metadata": map[string]any{"name": "p1"}},
				map[string]any{"kind": "Pod", "metadata": map[string]any{"name": "p2"}},
			}
			result := &mcp.CallToolResult{StructuredContent: data}
			items := extractStructuredList(result)
			Expect(items).To(HaveLen(2))
			Expect(items[0]["kind"]).To(Equal("Pod"))
		})

		It("UT-FLEET-PARSE-028e [SI-10]: skips non-map entries in []any", func() {
			data := []any{
				map[string]any{"kind": "Pod"},
				"not a map",
				42,
			}
			result := &mcp.CallToolResult{StructuredContent: data}
			items := extractStructuredList(result)
			Expect(items).To(HaveLen(1))
		})

		It("UT-FLEET-PARSE-028f [SI-10]: extracts items from map envelope (kube-mcp-server)", func() {
			data := map[string]any{
				"items": []any{
					map[string]any{
						"apiVersion": "v1",
						"kind":       "Namespace",
						"metadata":   map[string]any{"name": "default"},
					},
					map[string]any{
						"apiVersion": "v1",
						"kind":       "Namespace",
						"metadata":   map[string]any{"name": "kube-system"},
					},
				},
			}
			result := &mcp.CallToolResult{StructuredContent: data}
			items := extractStructuredList(result)
			Expect(items).To(HaveLen(2))
			Expect(items[0]["kind"]).To(Equal("Namespace"))
			meta, ok := items[0]["metadata"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(meta["name"]).To(Equal("default"))
		})

		It("UT-FLEET-PARSE-028g [SI-10]: returns nil for map envelope without items key", func() {
			data := map[string]any{"kind": "Namespace", "metadata": map[string]any{"name": "x"}}
			result := &mcp.CallToolResult{StructuredContent: data}
			Expect(extractStructuredList(result)).To(BeNil())
		})

		It("UT-FLEET-PARSE-028h [SI-10]: returns nil for map envelope with non-array items", func() {
			data := map[string]any{"items": "not an array"}
			result := &mcp.CallToolResult{StructuredContent: data}
			Expect(extractStructuredList(result)).To(BeNil())
		})
	})

	Describe("extractStructuredGet [SI-10]", func() {
		It("UT-FLEET-PARSE-029 [SI-10]: returns nil for nil result", func() {
			Expect(extractStructuredGet(nil)).To(BeNil())
		})

		It("UT-FLEET-PARSE-029b [SI-10]: returns nil for nil StructuredContent", func() {
			result := &mcp.CallToolResult{}
			Expect(extractStructuredGet(result)).To(BeNil())
		})

		It("UT-FLEET-PARSE-029c [SI-10]: returns nil for non-map StructuredContent", func() {
			result := &mcp.CallToolResult{StructuredContent: "not a map"}
			Expect(extractStructuredGet(result)).To(BeNil())
		})

		It("UT-FLEET-PARSE-029d [SI-10]: returns full K8s object from map with kind", func() {
			data := map[string]any{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata":   map[string]any{"name": "default", "uid": "abc-123"},
				"spec":       map[string]any{"finalizers": []any{"kubernetes"}},
				"status":     map[string]any{"phase": "Active"},
			}
			result := &mcp.CallToolResult{StructuredContent: data}
			obj := extractStructuredGet(result)
			Expect(obj).ToNot(BeNil())
			Expect(obj["kind"]).To(Equal("Namespace"))
			meta, ok := obj["metadata"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(meta["name"]).To(Equal("default"))
		})

		It("UT-FLEET-PARSE-029e [SI-10]: returns nil for map without kind key", func() {
			data := map[string]any{"metadata": map[string]any{"name": "x"}}
			result := &mcp.CallToolResult{StructuredContent: data}
			Expect(extractStructuredGet(result)).To(BeNil())
		})
	})

	Describe("normalizeTableItems [SI-10]", func() {
		It("UT-FLEET-NORM-001 [SI-10]: normalizes namespaced flat table-row map", func() {
			flatMaps := []map[string]any{
				{"Name": "pod-1", "Namespace": "prod", "Ready": "1/1", "Status": "Running", "Age": "5m"},
			}
			items := normalizeTableItems(flatMaps, "Pod", "v1")
			Expect(items).To(HaveLen(1))
			Expect(items[0].GetName()).To(Equal("pod-1"))
			Expect(items[0].GetNamespace()).To(Equal("prod"))
			Expect(items[0].GetKind()).To(Equal("Pod"))
			Expect(items[0].GetAPIVersion()).To(Equal("v1"))
		})

		It("UT-FLEET-NORM-002 [SI-10]: normalizes cluster-scoped flat map (no Namespace)", func() {
			flatMaps := []map[string]any{
				{"Name": "worker-1.example.com", "Ready": "1/1", "Status": "Running", "Age": "3d"},
			}
			items := normalizeTableItems(flatMaps, "Node", "v1")
			Expect(items).To(HaveLen(1))
			Expect(items[0].GetName()).To(Equal("worker-1.example.com"))
			Expect(items[0].GetNamespace()).To(BeEmpty())
			Expect(items[0].GetKind()).To(Equal("Node"))
			Expect(items[0].GetAPIVersion()).To(Equal("v1"))
		})

		It("UT-FLEET-NORM-003 [SI-10]: normalizes multiple items", func() {
			flatMaps := []map[string]any{
				{"Name": "pod-a", "Namespace": "ns-1"},
				{"Name": "pod-b", "Namespace": "ns-1"},
				{"Name": "pod-c", "Namespace": "ns-2"},
			}
			items := normalizeTableItems(flatMaps, "Pod", "v1")
			Expect(items).To(HaveLen(3))
			Expect(items[0].GetName()).To(Equal("pod-a"))
			Expect(items[1].GetName()).To(Equal("pod-b"))
			Expect(items[2].GetName()).To(Equal("pod-c"))
			Expect(items[2].GetNamespace()).To(Equal("ns-2"))
		})

		It("UT-FLEET-NORM-004 [SI-10]: passes through full K8s objects unchanged", func() {
			fullMaps := []map[string]any{
				{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata":   map[string]any{"name": "web", "namespace": "prod"},
					"spec":       map[string]any{"replicas": int64(3)},
				},
			}
			items := normalizeTableItems(fullMaps, "Deployment", "apps/v1")
			Expect(items).To(HaveLen(1))
			Expect(items[0].GetName()).To(Equal("web"))
			Expect(items[0].GetNamespace()).To(Equal("prod"))
			Expect(items[0].GetKind()).To(Equal("Deployment"))
			spec, ok := items[0].Object["spec"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(spec["replicas"]).To(Equal(int64(3)))
		})

		It("UT-FLEET-NORM-005 [SI-10]: handles empty items slice", func() {
			items := normalizeTableItems(nil, "Pod", "v1")
			Expect(items).To(BeEmpty())
		})

		It("UT-FLEET-NORM-006 [SI-10]: handles flat map with empty Namespace", func() {
			flatMaps := []map[string]any{
				{"Name": "my-ns", "Namespace": "", "Status": "Active"},
			}
			items := normalizeTableItems(flatMaps, "Namespace", "v1")
			Expect(items).To(HaveLen(1))
			Expect(items[0].GetName()).To(Equal("my-ns"))
			Expect(items[0].GetNamespace()).To(BeEmpty())
		})
	})

	Describe("normalizeTableItems contract with upstream [SI-10]", func() {
		It("UT-FLEET-NORM-CONTRACT-001 [SI-10]: normalizes output.Table.PrintObjStructured Pod (namespaced)", func() {
			pod := testPod("nginx-abc", "prod", map[string]string{"app": "nginx"})
			result := buildKMCPTableStructured(pod)
			Expect(result).ToNot(BeNil())

			flatItems, ok := result.([]map[string]any)
			Expect(ok).To(BeTrue(), "upstream table structured must be []map[string]any")

			items := normalizeTableItems(flatItems, "Pod", "v1")
			Expect(items).To(HaveLen(1))
			Expect(items[0].GetName()).To(Equal("nginx-abc"))
			Expect(items[0].GetNamespace()).To(Equal("prod"))
			Expect(items[0].GetKind()).To(Equal("Pod"))
			Expect(items[0].GetAPIVersion()).To(Equal("v1"))
		})

		It("UT-FLEET-NORM-CONTRACT-002 [SI-10]: normalizes output.Table.PrintObjStructured Node (cluster-scoped)", func() {
			node := testNode("worker-1", map[string]string{"kubernetes.io/arch": "amd64"})
			result := buildKMCPTableStructured(node)
			Expect(result).ToNot(BeNil())

			flatItems, ok := result.([]map[string]any)
			Expect(ok).To(BeTrue())

			items := normalizeTableItems(flatItems, "Node", "v1")
			Expect(items).To(HaveLen(1))
			Expect(items[0].GetName()).To(Equal("worker-1"))
			Expect(items[0].GetNamespace()).To(BeEmpty())
			Expect(items[0].GetKind()).To(Equal("Node"))
		})

		It("UT-FLEET-NORM-CONTRACT-003 [SI-10]: normalizes output.Table.PrintObjStructured Deployment (namespaced)", func() {
			dep := testDeployment("web-app", "staging", map[string]string{"app": "web"})
			result := buildKMCPTableStructured(dep)
			Expect(result).ToNot(BeNil())

			flatItems, ok := result.([]map[string]any)
			Expect(ok).To(BeTrue())

			items := normalizeTableItems(flatItems, "Deployment", "apps/v1")
			Expect(items).To(HaveLen(1))
			Expect(items[0].GetName()).To(Equal("web-app"))
			Expect(items[0].GetNamespace()).To(Equal("staging"))
			Expect(items[0].GetKind()).To(Equal("Deployment"))
			Expect(items[0].GetAPIVersion()).To(Equal("apps/v1"))
		})
	})
})

// buildKMCPTableStructured constructs table-format structuredContent using the
// upstream output.Table.PrintObjStructured, matching what kube-mcp-server
// returns with --list-output=table (default). Returns the Structured field only.
func buildKMCPTableStructured(objs ...*unstructured.Unstructured) any {
	columns := []metav1.TableColumnDefinition{
		{Name: "Name", Type: "string", Format: "name"},
		{Name: "Ready", Type: "string"},
		{Name: "Status", Type: "string"},
		{Name: "Restarts", Type: "string"},
		{Name: "Age", Type: "string"},
	}

	rows := make([]metav1.TableRow, 0, len(objs))
	for _, obj := range objs {
		raw, _ := json.Marshal(obj.Object)
		rows = append(rows, metav1.TableRow{
			Cells:  []any{obj.GetName(), "1/1", "Running", "0", "5m"},
			Object: runtime.RawExtension{Raw: raw},
		})
	}

	table := &metav1.Table{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "meta.k8s.io/v1",
			Kind:       "Table",
		},
		ColumnDefinitions: columns,
		Rows:              rows,
	}

	data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(table)
	if err != nil {
		panic("buildKMCPTableStructured: ToUnstructured: " + err.Error())
	}
	u := &unstructured.Unstructured{Object: data}
	u.SetGroupVersionKind(metav1.SchemeGroupVersion.WithKind("Table"))

	result, err := output.Table.PrintObjStructured(u)
	if err != nil {
		panic("buildKMCPTableStructured: PrintObjStructured: " + err.Error())
	}
	return result.Structured
}

// testDeployment creates a test Deployment as Unstructured.
func testDeployment(name, namespace string, labels map[string]string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]any{
			"replicas": int64(1),
		},
	}}
	if len(labels) > 0 {
		anyLabels := make(map[string]any, len(labels))
		for k, v := range labels {
			anyLabels[k] = v
		}
		obj.Object["metadata"].(map[string]any)["labels"] = anyLabels
	}
	return obj
}

// testNode creates a test Node as Unstructured (cluster-scoped).
func testNode(name string, labels map[string]string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Node",
		"metadata": map[string]any{
			"name": name,
		},
	}}
	if len(labels) > 0 {
		anyLabels := make(map[string]any, len(labels))
		for k, v := range labels {
			anyLabels[k] = v
		}
		obj.Object["metadata"].(map[string]any)["labels"] = anyLabels
	}
	return obj
}

// testPod creates a test Pod as Unstructured.
func testPod(name, namespace string, labels map[string]string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
		},
	}}
	if len(labels) > 0 {
		anyLabels := make(map[string]any, len(labels))
		for k, v := range labels {
			anyLabels[k] = v
		}
		obj.Object["metadata"].(map[string]any)["labels"] = anyLabels
	}
	return obj
}
