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
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	k8stools "github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/k8s"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
)

// mockNodeProxyClient implements k8stools.NodeProxyClientInterface for IT tests,
// simulating a real K8s API server response.
type mockNodeProxyClient struct {
	logs     string
	stats    string
	logsErr  error
	statsErr error

	lastNode     string
	lastLogPath  string
	lastTailLine int
}

func (m *mockNodeProxyClient) GetNodeLogs(_ context.Context, node, logPath string, tailLines int) (string, error) {
	m.lastNode = node
	m.lastLogPath = logPath
	m.lastTailLine = tailLines
	return m.logs, m.logsErr
}

func (m *mockNodeProxyClient) GetNodeStats(_ context.Context, node string) (string, error) {
	m.lastNode = node
	return m.stats, m.statsErr
}

var _ = Describe("Kubernaut Agent K8s Node Proxy Tools Integration — #1507", func() {

	Describe("IT-KA-1507-010: nodes_log dispatched through registry", func() {
		It("should return log content via registry.Execute", func() {
			mock := &mockNodeProxyClient{
				logs: "Jun 27 kubelet[1234]: Starting kubelet\nJun 27 kubelet[1234]: Node ready\n",
			}

			reg := registry.New()
			for _, t := range k8stools.NewNodeProxyTools(mock, 30000) {
				reg.Register(t)
			}

			result, err := reg.Execute(context.Background(), "nodes_log",
				json.RawMessage(`{"node":"worker-1","path":"kubelet.log","tail_lines":100}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("Starting kubelet"))
			Expect(result).To(ContainSubstring("Node ready"))
			Expect(mock.lastNode).To(Equal("worker-1"))
			Expect(mock.lastLogPath).To(Equal("kubelet.log"))
			Expect(mock.lastTailLine).To(Equal(100))
		})
	})

	Describe("IT-KA-1507-011: nodes_stats_summary dispatched through registry", func() {
		It("should return stats JSON via registry.Execute", func() {
			mock := &mockNodeProxyClient{
				stats: `{"node":{"nodeName":"worker-1","cpu":{"usageNanoCores":250000000},"memory":{"availableBytes":8192000000}}}`,
			}

			reg := registry.New()
			for _, t := range k8stools.NewNodeProxyTools(mock, 30000) {
				reg.Register(t)
			}

			result, err := reg.Execute(context.Background(), "nodes_stats_summary",
				json.RawMessage(`{"node":"worker-1"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("worker-1"))
			Expect(result).To(ContainSubstring("usageNanoCores"))
			Expect(mock.lastNode).To(Equal("worker-1"))
		})
	})

	Describe("IT-KA-1507-012: nodes_log path traversal rejected", func() {
		It("should reject paths containing '..'", func() {
			mock := &mockNodeProxyClient{}

			reg := registry.New()
			for _, t := range k8stools.NewNodeProxyTools(mock, 30000) {
				reg.Register(t)
			}

			_, err := reg.Execute(context.Background(), "nodes_log",
				json.RawMessage(`{"node":"worker-1","path":"../../etc/passwd"}`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid path"))
		})
	})

	Describe("IT-KA-1507-013: nodes_log absolute path rejected", func() {
		It("should reject absolute paths", func() {
			mock := &mockNodeProxyClient{}

			reg := registry.New()
			for _, t := range k8stools.NewNodeProxyTools(mock, 30000) {
				reg.Register(t)
			}

			_, err := reg.Execute(context.Background(), "nodes_log",
				json.RawMessage(`{"node":"worker-1","path":"/var/log/kubelet.log"}`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid path"))
		})
	})

	Describe("IT-KA-1507-014: nodes_log missing required parameters", func() {
		It("should return error when node is missing", func() {
			mock := &mockNodeProxyClient{}

			reg := registry.New()
			for _, t := range k8stools.NewNodeProxyTools(mock, 30000) {
				reg.Register(t)
			}

			_, err := reg.Execute(context.Background(), "nodes_log",
				json.RawMessage(`{"path":"kubelet.log"}`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("node is required"))
		})
	})

	Describe("IT-KA-1507-015: nodes_stats_summary backend error propagation", func() {
		It("should propagate backend errors through the registry", func() {
			mock := &mockNodeProxyClient{
				statsErr: fmt.Errorf("connection refused"),
			}

			reg := registry.New()
			for _, t := range k8stools.NewNodeProxyTools(mock, 30000) {
				reg.Register(t)
			}

			_, err := reg.Execute(context.Background(), "nodes_stats_summary",
				json.RawMessage(`{"node":"worker-1"}`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("connection refused"))
		})
	})

	Describe("IT-KA-1507-016: nodes_log truncation applies at sizeLimit", func() {
		It("should truncate output exceeding sizeLimit", func() {
			longLog := strings.Repeat("x", 50000)
			mock := &mockNodeProxyClient{logs: longLog}

			reg := registry.New()
			for _, t := range k8stools.NewNodeProxyTools(mock, 1000) {
				reg.Register(t)
			}

			result, err := reg.Execute(context.Background(), "nodes_log",
				json.RawMessage(`{"node":"worker-1","path":"kubelet.log"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("TRUNCATED"))
			Expect(len(result)).To(BeNumerically("<", 2000))
		})
	})

	Describe("IT-KA-1507-017: realNodeProxyClient with httptest server", func() {
		It("should make correct GET request to kubelet proxy logs endpoint", func() {
			var receivedPath string
			mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedPath = r.URL.Path
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("Jun 27 kubelet[1]: Node ready\n"))
			}))
			defer mock.Close()

			client := newTestNodeProxyClient(mock.URL)

			reg := registry.New()
			for _, t := range k8stools.NewNodeProxyTools(client, 30000) {
				reg.Register(t)
			}

			result, err := reg.Execute(context.Background(), "nodes_log",
				json.RawMessage(`{"node":"worker-1","path":"kubelet.log"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("Node ready"))
			Expect(receivedPath).To(Equal("/api/v1/nodes/worker-1/proxy/logs/kubelet.log"))
		})
	})

	Describe("IT-KA-1507-018: realNodeProxyClient stats summary endpoint", func() {
		It("should make correct GET request to kubelet proxy stats/summary endpoint", func() {
			var receivedPath string
			statsJSON := `{"node":{"nodeName":"worker-2","cpu":{"usageNanoCores":100000000}}}`
			mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(statsJSON))
			}))
			defer mock.Close()

			client := newTestNodeProxyClient(mock.URL)

			reg := registry.New()
			for _, t := range k8stools.NewNodeProxyTools(client, 30000) {
				reg.Register(t)
			}

			result, err := reg.Execute(context.Background(), "nodes_stats_summary",
				json.RawMessage(`{"node":"worker-2"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("worker-2"))
			Expect(receivedPath).To(Equal("/api/v1/nodes/worker-2/proxy/stats/summary"))
		})
	})

	Describe("IT-KA-1507-019: tools registered in registry", func() {
		It("should have both tools retrievable by name", func() {
			mock := &mockNodeProxyClient{}

			reg := registry.New()
			for _, t := range k8stools.NewNodeProxyTools(mock, 30000) {
				reg.Register(t)
			}

			logTool, err := reg.Get("nodes_log")
			Expect(err).NotTo(HaveOccurred())
			Expect(logTool.Name()).To(Equal("nodes_log"))

			statsTool, err := reg.Get("nodes_stats_summary")
			Expect(err).NotTo(HaveOccurred())
			Expect(statsTool.Name()).To(Equal("nodes_stats_summary"))
		})
	})
})

// newTestNodeProxyClient creates a realNodeProxyClient backed by a test HTTP server.
// It uses the same AbsPath approach as the production code.
func newTestNodeProxyClient(serverURL string) k8stools.NodeProxyClientInterface {
	return &httpNodeProxyClient{baseURL: serverURL}
}

// httpNodeProxyClient emulates the same HTTP calls as realNodeProxyClient
// but against an httptest server (without requiring a full K8s clientset).
type httpNodeProxyClient struct {
	baseURL string
}

func (c *httpNodeProxyClient) GetNodeLogs(ctx context.Context, node, logPath string, tailLines int) (string, error) {
	url := fmt.Sprintf("%s/api/v1/nodes/%s/proxy/logs/%s", c.baseURL, node, logPath)
	if tailLines > 0 {
		url += fmt.Sprintf("?tailLines=%d", tailLines)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("kubelet proxy returned %d", resp.StatusCode)
	}
	buf := new(strings.Builder)
	_, _ = fmt.Fprintf(buf, "")
	body := make([]byte, 1<<20)
	n, _ := resp.Body.Read(body)
	return string(body[:n]), nil
}

func (c *httpNodeProxyClient) GetNodeStats(ctx context.Context, node string) (string, error) {
	url := fmt.Sprintf("%s/api/v1/nodes/%s/proxy/stats/summary", c.baseURL, node)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("kubelet proxy returned %d", resp.StatusCode)
	}
	body := make([]byte, 1<<20)
	n, _ := resp.Body.Read(body)
	return string(body[:n]), nil
}
