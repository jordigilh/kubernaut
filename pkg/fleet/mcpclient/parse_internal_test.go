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
	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

		It("UT-FLEET-MCP-PARSE-003: returns error for invalid JSON", func() {
			_, err := parseUnstructured("{not-valid-json}")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unmarshaling resource"))
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

// Real captured output from kube-mcp-server v0.0.63 against OCP dev cluster.
// Authority: spike-s15-fmc-multiformat-parse/parse_spike_test.go
const tableDeploymentNamespaced = `NAMESPACE         APIVERSION   KIND         NAME                READY   UP-TO-DATE   AVAILABLE   AGE   CONTAINERS   IMAGES       SELECTOR        LABELS
kubernaut-spike   apps/v1      Deployment   spike-managed-web   0/1     1            0           89s   nginx        nginx:1.27   app=spike-web   app=spike-web,kubernaut.ai/managed=true`

const tableNodeClusterScoped = `APIVERSION   KIND   NAME                               STATUS   ROLES    AGE    VERSION    INTERNAL-IP       EXTERNAL-IP   OS-IMAGE                                                KERNEL-VERSION                  CONTAINER-RUNTIME                             LABELS
v1           Node   dev-worker-0.redhat-internal.com   Ready    worker   3d9h   v1.31.14   192.168.122.228   <none>        Red Hat Enterprise Linux CoreOS 418.94.202606051320-0   5.14.0-427.130.1.el9_4.x86_64   cri-o://1.31.13-10.rhaos4.18.git817a650.el9   beta.kubernetes.io/arch=amd64,beta.kubernetes.io/os=linux,kubernaut.ai/managed=true,kubernetes.io/arch=amd64,kubernetes.io/hostname=dev-worker-0.redhat-internal.com,kubernetes.io/os=linux,node-role.kubernetes.io/worker=,node.openshift.io/os_id=rhcos,topology.topolvm.io/node=dev-worker-0.redhat-internal.com`

const tableServiceNamespaced = `NAMESPACE         APIVERSION   KIND      NAME                TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)   AGE   SELECTOR        LABELS
kubernaut-spike   v1           Service   spike-managed-svc   ClusterIP   172.30.47.244   <none>        80/TCP    89s   app=spike-web   app=spike-svc,kubernaut.ai/managed=true`

const tableMultiRow = `NAMESPACE         APIVERSION   KIND   NAME    READY   STATUS    RESTARTS   AGE   IP           NODE       LABELS
kubernaut-spike   v1           Pod    pod-a   1/1     Running   0          5m    10.0.0.1     node-1     app=web,kubernaut.ai/managed=true
kubernaut-spike   v1           Pod    pod-b   1/1     Running   0          3m    10.0.0.2     node-1     app=api,kubernaut.ai/managed=true
kubernaut-spike   v1           Pod    pod-c   1/1     Running   0          1m    10.0.0.3     node-2     tier=backend`

const yamlDeploymentList = `- apiVersion: apps/v1
  kind: Deployment
  metadata:
    labels:
      app: spike-web
      kubernaut.ai/managed: "true"
    name: spike-managed-web
    namespace: kubernaut-spike
    resourceVersion: "3512410"
    uid: ef7214f7-6501-4770-9290-9593c9986d86
  spec:
    replicas: 1
  status:
    observedGeneration: 1`

const yamlSingleGet = `apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: spike-web
    kubernaut.ai/managed: "true"
  name: spike-managed-web
  namespace: kubernaut-spike
spec:
  replicas: 1`

// UT-FLEET-PARSE-020 through 046: Multi-format table parser tests
// Authority: BR-FLEET-002, ADR-068
// FedRAMP: SI-10 (Information Input Validation) -- validates MCP response parsing
var _ = Describe("UT-FLEET-PARSE: Multi-format MCP response parsing (BR-FLEET-002)", func() {

	Describe("parseTableText [SI-10]", func() {
		It("UT-FLEET-PARSE-020 [SI-10]: extracts metadata from namespaced Deployment table", func() {
			rows, err := parseTableText(tableDeploymentNamespaced)
			Expect(err).ToNot(HaveOccurred())
			Expect(rows).To(HaveLen(1))
			Expect(rows[0].Namespace).To(Equal("kubernaut-spike"))
			Expect(rows[0].Name).To(Equal("spike-managed-web"))
			Expect(rows[0].Kind).To(Equal("Deployment"))
			Expect(rows[0].APIVersion).To(Equal("apps/v1"))
		})

		It("UT-FLEET-PARSE-021 [SI-10]: handles cluster-scoped Node table (no NAMESPACE column)", func() {
			rows, err := parseTableText(tableNodeClusterScoped)
			Expect(err).ToNot(HaveOccurred())
			Expect(rows).To(HaveLen(1))
			Expect(rows[0].Namespace).To(BeEmpty())
			Expect(rows[0].Name).To(Equal("dev-worker-0.redhat-internal.com"))
			Expect(rows[0].Kind).To(Equal("Node"))
			Expect(rows[0].APIVersion).To(Equal("v1"))
		})

		It("UT-FLEET-PARSE-022 [SI-10]: extracts labels from table row", func() {
			rows, err := parseTableText(tableDeploymentNamespaced)
			Expect(err).ToNot(HaveOccurred())
			Expect(rows).To(HaveLen(1))
			Expect(rows[0].Labels).To(HaveKeyWithValue("kubernaut.ai/managed", "true"))
			Expect(rows[0].Labels).To(HaveKeyWithValue("app", "spike-web"))
		})

		It("UT-FLEET-PARSE-023 [SI-10]: returns error for header-only input (no data rows)", func() {
			_, err := parseTableText("NAME   KIND   APIVERSION")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("at least"))
		})

		It("UT-FLEET-PARSE-046 [SI-10]: parses multi-row table with multiple data rows", func() {
			rows, err := parseTableText(tableMultiRow)
			Expect(err).ToNot(HaveOccurred())
			Expect(rows).To(HaveLen(3))
			Expect(rows[0].Name).To(Equal("pod-a"))
			Expect(rows[1].Name).To(Equal("pod-b"))
			Expect(rows[2].Name).To(Equal("pod-c"))
			Expect(rows[0].Namespace).To(Equal("kubernaut-spike"))
			Expect(rows[2].Labels).To(HaveKeyWithValue("tier", "backend"))
		})
	})

	Describe("parseLabels [SI-10]", func() {
		It("UT-FLEET-PARSE-026 [SI-10]: handles empty, <none>, and key-only labels", func() {
			Expect(parseLabels("")).To(BeNil())
			Expect(parseLabels("<none>")).To(BeNil())

			m := parseLabels("node-role.kubernetes.io/worker=")
			Expect(m).To(HaveKeyWithValue("node-role.kubernetes.io/worker", ""))
		})

		It("UT-FLEET-PARSE-045 [SI-10]: handles value containing equals sign", func() {
			m := parseLabels("config=key=value,app=nginx")
			Expect(m).To(HaveKeyWithValue("config", "key=value"))
			Expect(m).To(HaveKeyWithValue("app", "nginx"))
		})
	})

	Describe("looksLikeTable [SI-10]", func() {
		It("UT-FLEET-PARSE-027 [SI-10]: detects table and non-table formats", func() {
			Expect(looksLikeTable(tableDeploymentNamespaced)).To(BeTrue())
			Expect(looksLikeTable(tableNodeClusterScoped)).To(BeTrue())
			Expect(looksLikeTable(tableServiceNamespaced)).To(BeTrue())

			Expect(looksLikeTable(yamlDeploymentList)).To(BeFalse())
			Expect(looksLikeTable(yamlSingleGet)).To(BeFalse())
			Expect(looksLikeTable(`{"items":[]}`)).To(BeFalse())
			Expect(looksLikeTable("")).To(BeFalse())
		})
	})

	Describe("parseTableColumns [SI-10]", func() {
		It("UT-FLEET-PARSE-040 [SI-10]: handles empty header, single column, trailing spaces", func() {
			Expect(parseTableColumns("")).To(BeEmpty())

			cols := parseTableColumns("NAME")
			Expect(cols).To(HaveLen(1))
			Expect(cols[0].name).To(Equal("NAME"))
			Expect(cols[0].end).To(Equal(-1))

			cols = parseTableColumns("NAME   KIND   ")
			Expect(cols).To(HaveLen(2))
			Expect(cols[0].name).To(Equal("NAME"))
			Expect(cols[1].name).To(Equal("KIND"))
			Expect(cols[1].end).To(Equal(-1))
		})
	})

	Describe("extractTableField [SI-10]", func() {
		It("UT-FLEET-PARSE-041 [SI-10]: handles row shorter than column start and boundary", func() {
			col := tableColumn{name: "NAME", start: 20, end: 30}
			Expect(extractTableField("short", col)).To(BeEmpty())

			col = tableColumn{name: "LABELS", start: 5, end: -1}
			Expect(extractTableField("HELLO WORLD", col)).To(Equal("WORLD"))
		})
	})

	Describe("findColumn [SI-10]", func() {
		It("UT-FLEET-PARSE-042 [SI-10]: case-insensitive match, missing column returns false", func() {
			cols := []tableColumn{
				{name: "NAME", start: 0, end: 10},
				{name: "KIND", start: 10, end: 20},
			}
			col, ok := findColumn(cols, "name")
			Expect(ok).To(BeTrue())
			Expect(col.name).To(Equal("NAME"))

			_, ok = findColumn(cols, "MISSING")
			Expect(ok).To(BeFalse())
		})
	})

	Describe("tableRowsToUnstructured [SI-10]", func() {
		It("UT-FLEET-PARSE-043 [SI-10]: handles empty rows and partial metadata", func() {
			items := tableRowsToUnstructured(nil)
			Expect(items).To(BeEmpty())

			rows := []parsedTableRow{{Kind: "Pod", APIVersion: "v1"}}
			items = tableRowsToUnstructured(rows)
			Expect(items).To(HaveLen(1))
			Expect(items[0].GetKind()).To(Equal("Pod"))
			Expect(items[0].GetName()).To(BeEmpty())
		})

		It("UT-FLEET-PARSE-044 [SI-10]: row with no labels produces no labels key in metadata", func() {
			rows := []parsedTableRow{{Name: "test", Kind: "Pod", APIVersion: "v1"}}
			items := tableRowsToUnstructured(rows)
			Expect(items).To(HaveLen(1))
			Expect(items[0].GetLabels()).To(BeNil())
		})
	})

	Describe("parseMultiFormat YAML fallback [SI-10]", func() {
		It("UT-FLEET-PARSE-024 [SI-10]: parses YAML array list", func() {
			items, err := parseMultiFormat(yamlDeploymentList)
			Expect(err).ToNot(HaveOccurred())
			Expect(items).To(HaveLen(1))
			Expect(items[0].GetKind()).To(Equal("Deployment"))
			Expect(items[0].GetAPIVersion()).To(Equal("apps/v1"))
			Expect(items[0].GetName()).To(Equal("spike-managed-web"))
			Expect(items[0].GetNamespace()).To(Equal("kubernaut-spike"))
			Expect(items[0].GetLabels()).To(HaveKeyWithValue("kubernaut.ai/managed", "true"))
		})

		It("UT-FLEET-PARSE-025 [SI-10]: parses YAML single-object", func() {
			items, err := parseMultiFormat(yamlSingleGet)
			Expect(err).ToNot(HaveOccurred())
			Expect(items).To(HaveLen(1))
			Expect(items[0].GetKind()).To(Equal("Deployment"))
			Expect(items[0].GetName()).To(Equal("spike-managed-web"))
		})
	})

	Describe("extractStructured [SI-10]", func() {
		It("UT-FLEET-PARSE-028 [SI-10]: returns nil for nil result", func() {
			Expect(extractStructured(nil)).To(BeNil())
		})

		It("UT-FLEET-PARSE-028b [SI-10]: returns nil for nil StructuredContent", func() {
			result := &mcp.CallToolResult{}
			Expect(extractStructured(result)).To(BeNil())
		})

		It("UT-FLEET-PARSE-028c [SI-10]: returns nil for non-slice StructuredContent", func() {
			result := &mcp.CallToolResult{StructuredContent: "not a slice"}
			Expect(extractStructured(result)).To(BeNil())
		})

		It("UT-FLEET-PARSE-028d [SI-10]: extracts valid []any of maps", func() {
			data := []any{
				map[string]any{"kind": "Pod", "metadata": map[string]any{"name": "p1"}},
				map[string]any{"kind": "Pod", "metadata": map[string]any{"name": "p2"}},
			}
			result := &mcp.CallToolResult{StructuredContent: data}
			items := extractStructured(result)
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
			items := extractStructured(result)
			Expect(items).To(HaveLen(1))
		})
	})

	Describe("parseMultiFormat priority chain [CM-6] [SI-4]", func() {
		It("UT-FLEET-PARSE-030 [SI-10]: JSON K8s list with items has highest priority", func() {
			jsonList := `{"items":[{"kind":"Pod","metadata":{"name":"a"}},{"kind":"Pod","metadata":{"name":"b"}}]}`
			items, err := parseMultiFormat(jsonList)
			Expect(err).ToNot(HaveOccurred())
			Expect(items).To(HaveLen(2))
			Expect(items[0].GetName()).To(Equal("a"))
		})

		It("UT-FLEET-PARSE-030b [SI-10]: JSON raw array", func() {
			jsonArray := `[{"kind":"Pod","metadata":{"name":"x"}}]`
			items, err := parseMultiFormat(jsonArray)
			Expect(err).ToNot(HaveOccurred())
			Expect(items).To(HaveLen(1))
			Expect(items[0].GetName()).To(Equal("x"))
		})

		It("UT-FLEET-PARSE-030c [SI-10]: JSON single object", func() {
			jsonSingle := `{"kind":"ConfigMap","metadata":{"name":"solo"}}`
			items, err := parseMultiFormat(jsonSingle)
			Expect(err).ToNot(HaveOccurred())
			Expect(items).To(HaveLen(1))
			Expect(items[0].GetName()).To(Equal("solo"))
		})

		It("UT-FLEET-PARSE-031 [SI-10]: YAML fallback when JSON fails", func() {
			items, err := parseMultiFormat(yamlDeploymentList)
			Expect(err).ToNot(HaveOccurred())
			Expect(items).To(HaveLen(1))
			Expect(items[0].GetKind()).To(Equal("Deployment"))
		})

		It("UT-FLEET-PARSE-032 [SI-10]: table fallback when JSON and YAML fail", func() {
			items, err := parseMultiFormat(tableDeploymentNamespaced)
			Expect(err).ToNot(HaveOccurred())
			Expect(items).To(HaveLen(1))
			Expect(items[0].GetKind()).To(Equal("Deployment"))
			Expect(items[0].GetName()).To(Equal("spike-managed-web"))
		})

		It("UT-FLEET-PARSE-033 [SI-10]: cluster-scoped table (no NAMESPACE column)", func() {
			items, err := parseMultiFormat(tableNodeClusterScoped)
			Expect(err).ToNot(HaveOccurred())
			Expect(items).To(HaveLen(1))
			Expect(items[0].GetKind()).To(Equal("Node"))
			Expect(items[0].GetNamespace()).To(BeEmpty())
		})

		It("UT-FLEET-PARSE-034 [SI-10]: empty input returns nil without error", func() {
			items, err := parseMultiFormat("")
			Expect(err).ToNot(HaveOccurred())
			Expect(items).To(BeNil())
		})

		It("UT-FLEET-PARSE-035 [SI-4]: unparseable input returns error", func() {
			_, err := parseMultiFormat("@@@totally garbage@@@")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unable to parse"))
		})

		It("UT-FLEET-PARSE-036 [CM-6]: valid JSON is never parsed as table even if it contains NAME", func() {
			jsonWithNAME := `{"kind":"Pod","metadata":{"name":"NAME"}}`
			items, err := parseMultiFormat(jsonWithNAME)
			Expect(err).ToNot(HaveOccurred())
			Expect(items).To(HaveLen(1))
			Expect(items[0].GetName()).To(Equal("NAME"))
		})

		It("UT-FLEET-PARSE-037 [SI-4]: malformed table returns error, not panic", func() {
			malformed := "NAME   KIND\n"
			_, err := parseMultiFormat(malformed)
			Expect(err).To(HaveOccurred())
		})

		It("UT-FLEET-PARSE-038 [SI-4]: binary/garbage input returns error, not panic", func() {
			Expect(func() {
				_, _ = parseMultiFormat("\x00\x01\x02\xff")
			}).ToNot(Panic())
		})
	})
})
