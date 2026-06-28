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

package k8s_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/k8s"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type mockNodeProxyClient struct {
	logsResult  string
	logsErr     error
	statsResult string
	statsErr    error

	capturedNode     string
	capturedLogPath  string
	capturedTailLines int
}

func (m *mockNodeProxyClient) GetNodeLogs(ctx context.Context, node, logPath string, tailLines int) (string, error) {
	m.capturedNode = node
	m.capturedLogPath = logPath
	m.capturedTailLines = tailLines
	return m.logsResult, m.logsErr
}

func (m *mockNodeProxyClient) GetNodeStats(ctx context.Context, node string) (string, error) {
	m.capturedNode = node
	return m.statsResult, m.statsErr
}

var _ = Describe("Node Proxy Tools Unit — #1507", func() {

	Describe("NewNodeProxyTools defaults sizeLimit", func() {
		It("should default sizeLimit to 30000 when zero", func() {
			mock := &mockNodeProxyClient{logsResult: "test"}
			tools := k8s.NewNodeProxyTools(mock, 0)
			Expect(tools).To(HaveLen(2))
		})
	})

	// --- nodes_log ---

	Describe("UT-KA-1507-001: nodes_log happy path", func() {
		It("should return log text from proxy client", func() {
			mock := &mockNodeProxyClient{logsResult: "Jun 27 kubelet[1234]: Started container"}
			tools := k8s.NewNodeProxyTools(mock, 30000)
			var logTool k8s.NodeTool
			for _, t := range tools {
				if t.Name() == "nodes_log" {
					logTool = t
					break
				}
			}
			Expect(logTool).NotTo(BeNil())

			result, err := logTool.Execute(context.Background(), json.RawMessage(`{"node":"worker-1","path":"kubelet.log"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("Started container"))
			Expect(mock.capturedNode).To(Equal("worker-1"))
			Expect(mock.capturedLogPath).To(Equal("kubelet.log"))
		})
	})

	Describe("UT-KA-1507-002: nodes_log optional tail_lines parameter", func() {
		It("should pass tail_lines to proxy client", func() {
			mock := &mockNodeProxyClient{logsResult: "log line"}
			tools := k8s.NewNodeProxyTools(mock, 30000)
			var logTool k8s.NodeTool
			for _, t := range tools {
				if t.Name() == "nodes_log" {
					logTool = t
					break
				}
			}

			_, err := logTool.Execute(context.Background(), json.RawMessage(`{"node":"worker-1","path":"kubelet.log","tail_lines":100}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(mock.capturedTailLines).To(Equal(100))
		})
	})

	Describe("UT-KA-1507-003: nodes_log missing required param node", func() {
		It("should return error when node is missing", func() {
			mock := &mockNodeProxyClient{}
			tools := k8s.NewNodeProxyTools(mock, 30000)
			var logTool k8s.NodeTool
			for _, t := range tools {
				if t.Name() == "nodes_log" {
					logTool = t
					break
				}
			}

			_, err := logTool.Execute(context.Background(), json.RawMessage(`{"path":"kubelet.log"}`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("node is required"))
		})
	})

	Describe("UT-KA-1507-004: nodes_log missing required param path", func() {
		It("should return error when path is missing", func() {
			mock := &mockNodeProxyClient{}
			tools := k8s.NewNodeProxyTools(mock, 30000)
			var logTool k8s.NodeTool
			for _, t := range tools {
				if t.Name() == "nodes_log" {
					logTool = t
					break
				}
			}

			_, err := logTool.Execute(context.Background(), json.RawMessage(`{"node":"worker-1"}`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("path is required"))
		})
	})

	Describe("UT-KA-1507-005: nodes_log empty node name", func() {
		It("should return error for empty string node", func() {
			mock := &mockNodeProxyClient{}
			tools := k8s.NewNodeProxyTools(mock, 30000)
			var logTool k8s.NodeTool
			for _, t := range tools {
				if t.Name() == "nodes_log" {
					logTool = t
					break
				}
			}

			_, err := logTool.Execute(context.Background(), json.RawMessage(`{"node":"","path":"kubelet.log"}`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("node is required"))
		})
	})

	Describe("UT-KA-1507-006: nodes_log proxy client returns error", func() {
		It("should wrap and propagate the proxy error", func() {
			mock := &mockNodeProxyClient{logsErr: errors.New("node not found")}
			tools := k8s.NewNodeProxyTools(mock, 30000)
			var logTool k8s.NodeTool
			for _, t := range tools {
				if t.Name() == "nodes_log" {
					logTool = t
					break
				}
			}

			_, err := logTool.Execute(context.Background(), json.RawMessage(`{"node":"nonexistent","path":"kubelet.log"}`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("node not found"))
		})
	})

	Describe("UT-KA-1507-007: nodes_log response exceeds size limit", func() {
		It("should truncate output with hint", func() {
			largeLog := strings.Repeat("log line\n", 10000)
			mock := &mockNodeProxyClient{logsResult: largeLog}
			tools := k8s.NewNodeProxyTools(mock, 500)
			var logTool k8s.NodeTool
			for _, t := range tools {
				if t.Name() == "nodes_log" {
					logTool = t
					break
				}
			}

			result, err := logTool.Execute(context.Background(), json.RawMessage(`{"node":"worker-1","path":"kubelet.log"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(len(result)).To(BeNumerically("<", len(largeLog)))
			Expect(result).To(ContainSubstring("TRUNCATED"))
		})
	})

	Describe("UT-KA-1507-009: nodes_log Name()", func() {
		It("should return 'nodes_log'", func() {
			mock := &mockNodeProxyClient{}
			tools := k8s.NewNodeProxyTools(mock, 30000)
			var logTool k8s.NodeTool
			for _, t := range tools {
				if t.Name() == "nodes_log" {
					logTool = t
					break
				}
			}
			Expect(logTool.Name()).To(Equal("nodes_log"))
		})
	})

	Describe("UT-KA-1507-010: nodes_log Description()", func() {
		It("should return non-empty description", func() {
			mock := &mockNodeProxyClient{}
			tools := k8s.NewNodeProxyTools(mock, 30000)
			var logTool k8s.NodeTool
			for _, t := range tools {
				if t.Name() == "nodes_log" {
					logTool = t
					break
				}
			}
			Expect(logTool.Description()).NotTo(BeEmpty())
		})
	})

	Describe("UT-KA-1507-011: nodes_log Parameters()", func() {
		It("should return valid JSON schema with required fields", func() {
			mock := &mockNodeProxyClient{}
			tools := k8s.NewNodeProxyTools(mock, 30000)
			var logTool k8s.NodeTool
			for _, t := range tools {
				if t.Name() == "nodes_log" {
					logTool = t
					break
				}
			}
			params := logTool.Parameters()
			var schema map[string]interface{}
			err := json.Unmarshal(params, &schema)
			Expect(err).NotTo(HaveOccurred())
			Expect(schema["type"]).To(Equal("object"))
			required, ok := schema["required"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(required).To(ContainElement("node"))
			Expect(required).To(ContainElement("path"))
		})
	})

	Describe("UT-KA-1507-012: nodes_log path traversal sanitization", func() {
		It("should reject paths containing '..'", func() {
			mock := &mockNodeProxyClient{}
			tools := k8s.NewNodeProxyTools(mock, 30000)
			var logTool k8s.NodeTool
			for _, t := range tools {
				if t.Name() == "nodes_log" {
					logTool = t
					break
				}
			}

			_, err := logTool.Execute(context.Background(), json.RawMessage(`{"node":"worker-1","path":"../../etc/passwd"}`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid path"))
		})

		It("should reject absolute paths starting with '/'", func() {
			mock := &mockNodeProxyClient{}
			tools := k8s.NewNodeProxyTools(mock, 30000)
			var logTool k8s.NodeTool
			for _, t := range tools {
				if t.Name() == "nodes_log" {
					logTool = t
					break
				}
			}

			_, err := logTool.Execute(context.Background(), json.RawMessage(`{"node":"worker-1","path":"/etc/passwd"}`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid path"))
		})
	})

	Describe("UT-KA-1507-013: nodes_log nil args", func() {
		It("should return error for nil args", func() {
			mock := &mockNodeProxyClient{}
			tools := k8s.NewNodeProxyTools(mock, 30000)
			var logTool k8s.NodeTool
			for _, t := range tools {
				if t.Name() == "nodes_log" {
					logTool = t
					break
				}
			}

			_, err := logTool.Execute(context.Background(), nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("node is required"))
		})
	})

	Describe("UT-KA-1507-014: nodes_log malformed JSON args", func() {
		It("should return parsing error", func() {
			mock := &mockNodeProxyClient{}
			tools := k8s.NewNodeProxyTools(mock, 30000)
			var logTool k8s.NodeTool
			for _, t := range tools {
				if t.Name() == "nodes_log" {
					logTool = t
					break
				}
			}

			_, err := logTool.Execute(context.Background(), json.RawMessage(`{invalid`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("parsing args"))
		})
	})

	// --- nodes_stats_summary ---

	Describe("UT-KA-1507-020: nodes_stats_summary happy path", func() {
		It("should return raw JSON from proxy client", func() {
			statsJSON := `{"node":{"nodeName":"worker-1","cpu":{"usageNanoCores":123456789}}}`
			mock := &mockNodeProxyClient{statsResult: statsJSON}
			tools := k8s.NewNodeProxyTools(mock, 30000)
			var statsTool k8s.NodeTool
			for _, t := range tools {
				if t.Name() == "nodes_stats_summary" {
					statsTool = t
					break
				}
			}
			Expect(statsTool).NotTo(BeNil())

			result, err := statsTool.Execute(context.Background(), json.RawMessage(`{"node":"worker-1"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("usageNanoCores"))
			Expect(mock.capturedNode).To(Equal("worker-1"))
		})
	})

	Describe("UT-KA-1507-021: nodes_stats_summary missing required param node", func() {
		It("should return error when node is missing", func() {
			mock := &mockNodeProxyClient{}
			tools := k8s.NewNodeProxyTools(mock, 30000)
			var statsTool k8s.NodeTool
			for _, t := range tools {
				if t.Name() == "nodes_stats_summary" {
					statsTool = t
					break
				}
			}

			_, err := statsTool.Execute(context.Background(), json.RawMessage(`{}`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("node is required"))
		})
	})

	Describe("UT-KA-1507-022: nodes_stats_summary empty node name", func() {
		It("should return error for empty string node", func() {
			mock := &mockNodeProxyClient{}
			tools := k8s.NewNodeProxyTools(mock, 30000)
			var statsTool k8s.NodeTool
			for _, t := range tools {
				if t.Name() == "nodes_stats_summary" {
					statsTool = t
					break
				}
			}

			_, err := statsTool.Execute(context.Background(), json.RawMessage(`{"node":""}`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("node is required"))
		})
	})

	Describe("UT-KA-1507-023: nodes_stats_summary proxy client returns error", func() {
		It("should wrap and propagate the proxy error", func() {
			mock := &mockNodeProxyClient{statsErr: errors.New("forbidden")}
			tools := k8s.NewNodeProxyTools(mock, 30000)
			var statsTool k8s.NodeTool
			for _, t := range tools {
				if t.Name() == "nodes_stats_summary" {
					statsTool = t
					break
				}
			}

			_, err := statsTool.Execute(context.Background(), json.RawMessage(`{"node":"worker-1"}`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("forbidden"))
		})
	})

	Describe("UT-KA-1507-024: nodes_stats_summary response exceeds size limit", func() {
		It("should truncate with hint", func() {
			largeStats := strings.Repeat(`{"pod":"x"},`, 10000)
			mock := &mockNodeProxyClient{statsResult: largeStats}
			tools := k8s.NewNodeProxyTools(mock, 500)
			var statsTool k8s.NodeTool
			for _, t := range tools {
				if t.Name() == "nodes_stats_summary" {
					statsTool = t
					break
				}
			}

			result, err := statsTool.Execute(context.Background(), json.RawMessage(`{"node":"worker-1"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(len(result)).To(BeNumerically("<", len(largeStats)))
			Expect(result).To(ContainSubstring("TRUNCATED"))
		})
	})

	Describe("UT-KA-1507-026: nodes_stats_summary Name()", func() {
		It("should return 'nodes_stats_summary'", func() {
			mock := &mockNodeProxyClient{}
			tools := k8s.NewNodeProxyTools(mock, 30000)
			var statsTool k8s.NodeTool
			for _, t := range tools {
				if t.Name() == "nodes_stats_summary" {
					statsTool = t
					break
				}
			}
			Expect(statsTool.Name()).To(Equal("nodes_stats_summary"))
		})
	})

	Describe("UT-KA-1507-027: nodes_stats_summary Description()", func() {
		It("should return non-empty description", func() {
			mock := &mockNodeProxyClient{}
			tools := k8s.NewNodeProxyTools(mock, 30000)
			var statsTool k8s.NodeTool
			for _, t := range tools {
				if t.Name() == "nodes_stats_summary" {
					statsTool = t
					break
				}
			}
			Expect(statsTool.Description()).NotTo(BeEmpty())
		})
	})

	Describe("UT-KA-1507-028: nodes_stats_summary Parameters()", func() {
		It("should return valid JSON schema with node required", func() {
			mock := &mockNodeProxyClient{}
			tools := k8s.NewNodeProxyTools(mock, 30000)
			var statsTool k8s.NodeTool
			for _, t := range tools {
				if t.Name() == "nodes_stats_summary" {
					statsTool = t
					break
				}
			}
			params := statsTool.Parameters()
			var schema map[string]interface{}
			err := json.Unmarshal(params, &schema)
			Expect(err).NotTo(HaveOccurred())
			Expect(schema["type"]).To(Equal("object"))
			required, ok := schema["required"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(required).To(ContainElement("node"))
		})
	})

	Describe("UT-KA-1507-029: nodes_stats_summary nil args", func() {
		It("should return error for nil args", func() {
			mock := &mockNodeProxyClient{}
			tools := k8s.NewNodeProxyTools(mock, 30000)
			var statsTool k8s.NodeTool
			for _, t := range tools {
				if t.Name() == "nodes_stats_summary" {
					statsTool = t
					break
				}
			}

			_, err := statsTool.Execute(context.Background(), nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("node is required"))
		})
	})

	Describe("UT-KA-1507-030-a: nodes_stats_summary malformed JSON args", func() {
		It("should return parsing error", func() {
			mock := &mockNodeProxyClient{}
			tools := k8s.NewNodeProxyTools(mock, 30000)
			var statsTool k8s.NodeTool
			for _, t := range tools {
				if t.Name() == "nodes_stats_summary" {
					statsTool = t
					break
				}
			}

			_, err := statsTool.Execute(context.Background(), json.RawMessage(`{invalid`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("parsing args"))
		})
	})
})

var _ = Describe("realNodeProxyClient Unit — #1507", func() {

	Describe("UT-KA-1507-120: GetNodeLogs successful call", func() {
		It("should return log content from the kubelet proxy endpoint", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Path).To(Equal("/api/v1/nodes/worker-1/proxy/logs/kubelet.log"))
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("Jun 27 kubelet: container started"))
			}))
			defer server.Close()

			clientset, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
			Expect(err).NotTo(HaveOccurred())

			npc := k8s.NewNodeProxyClient(clientset)
			result, err := npc.GetNodeLogs(context.Background(), "worker-1", "kubelet.log", 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("container started"))
		})
	})

	Describe("UT-KA-1507-120b: GetNodeLogs with tailLines param", func() {
		It("should include tailLines query parameter", func() {
			var receivedQuery string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedQuery = r.URL.RawQuery
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("last lines"))
			}))
			defer server.Close()

			clientset, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
			Expect(err).NotTo(HaveOccurred())

			npc := k8s.NewNodeProxyClient(clientset)
			result, err := npc.GetNodeLogs(context.Background(), "worker-1", "kubelet.log", 50)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("last lines"))
			Expect(receivedQuery).To(ContainSubstring("tailLines=50"))
		})
	})

	Describe("UT-KA-1507-121: GetNodeLogs node not found", func() {
		It("should return error when server responds 404", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"kind":"Status","status":"Failure","message":"nodes \"bad-node\" not found","reason":"NotFound","code":404}`))
			}))
			defer server.Close()

			clientset, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
			Expect(err).NotTo(HaveOccurred())

			npc := k8s.NewNodeProxyClient(clientset)
			_, err = npc.GetNodeLogs(context.Background(), "bad-node", "kubelet.log", 0)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("kubelet proxy logs request"))
		})
	})

	Describe("UT-KA-1507-122: GetNodeLogs forbidden (RBAC)", func() {
		It("should return error when server responds 403", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"kind":"Status","status":"Failure","message":"forbidden","reason":"Forbidden","code":403}`))
			}))
			defer server.Close()

			clientset, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
			Expect(err).NotTo(HaveOccurred())

			npc := k8s.NewNodeProxyClient(clientset)
			_, err = npc.GetNodeLogs(context.Background(), "worker-1", "kubelet.log", 0)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("kubelet proxy logs request"))
		})
	})

	Describe("UT-KA-1507-123: GetNodeStats successful call", func() {
		It("should return stats JSON from the kubelet proxy endpoint", func() {
			statsJSON := `{"node":{"nodeName":"worker-1","cpu":{"usageNanoCores":500000000}}}`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Path).To(Equal("/api/v1/nodes/worker-1/proxy/stats/summary"))
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(statsJSON))
			}))
			defer server.Close()

			clientset, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
			Expect(err).NotTo(HaveOccurred())

			npc := k8s.NewNodeProxyClient(clientset)
			result, err := npc.GetNodeStats(context.Background(), "worker-1")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("usageNanoCores"))
		})
	})

	Describe("UT-KA-1507-124: GetNodeStats node not found", func() {
		It("should return error when server responds 404", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"kind":"Status","status":"Failure","message":"not found","code":404}`))
			}))
			defer server.Close()

			clientset, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
			Expect(err).NotTo(HaveOccurred())

			npc := k8s.NewNodeProxyClient(clientset)
			_, err = npc.GetNodeStats(context.Background(), "bad-node")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("kubelet proxy stats request"))
		})
	})

	Describe("UT-KA-1507-125: GetNodeStats forbidden (RBAC)", func() {
		It("should return error when server responds 403", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"kind":"Status","status":"Failure","message":"forbidden","code":403}`))
			}))
			defer server.Close()

			clientset, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
			Expect(err).NotTo(HaveOccurred())

			npc := k8s.NewNodeProxyClient(clientset)
			_, err = npc.GetNodeStats(context.Background(), "worker-1")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("kubelet proxy stats request"))
		})
	})
})
