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

package prometheus_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/prometheus"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

var _ = Describe("Kubernaut Agent Prometheus Tools Integration — #433", func() {

	Describe("Query tools via shared MockPrometheus (IT-025, IT-026)", func() {
		var (
			mockProm *infrastructure.MockPrometheus
			reg      *registry.Registry
		)

		BeforeEach(func() {
			now := float64(time.Now().Unix())
			mockProm = infrastructure.NewMockPrometheus(infrastructure.MockPrometheusConfig{
				Ready:   true,
				Healthy: true,
				QueryResponse: infrastructure.NewPromVectorResponse(
					map[string]string{"__name__": "up", "job": "kubelet"},
					1.0, now,
				),
				QueryRangeResponse: infrastructure.NewPromMatrixResponse(
					map[string]string{"__name__": "up", "job": "kubelet"},
					[][]interface{}{{now - 60, "1"}, {now, "1"}},
				),
			})

			client, err := prometheus.NewClient(prometheus.ClientConfig{
				URL: mockProm.URL(), Timeout: 5 * time.Second, SizeLimit: 30000,
			})
			Expect(err).NotTo(HaveOccurred())

			reg = registry.New()
			for _, t := range prometheus.NewAllTools(client) {
				reg.Register(t)
			}
		})

		AfterEach(func() { mockProm.Close() })

		It("IT-KA-433-025: execute_prometheus_instant_query returns PromQL query result", func() {
			result, err := reg.Execute(context.Background(), "execute_prometheus_instant_query",
				json.RawMessage(`{"query":"up{job=\"kubelet\"}"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("up"))
		})

		It("IT-KA-433-026: execute_prometheus_range_query returns time-series data", func() {
			result, err := reg.Execute(context.Background(), "execute_prometheus_range_query",
				json.RawMessage(`{"query":"up","start":"2024-02-27T00:00:00Z","end":"2024-02-27T01:00:00Z","step":"60s"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeEmpty())
		})
	})

	Describe("Label and metadata tools via httptest (IT-027..030)", func() {
		var (
			server *httptest.Server
			reg    *registry.Registry
		)

		BeforeEach(func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/v1/label/__name__/values":
				_, _ = fmt.Fprint(w, `{"status":"success","data":["up","container_memory_usage_bytes","node_cpu_seconds_total"]}`)
			case "/api/v1/labels":
				_, _ = fmt.Fprint(w, `{"status":"success","data":["__name__","instance","job","namespace","pod"]}`)
			case "/api/v1/metadata":
				_, _ = fmt.Fprint(w, `{"status":"success","data":{"up":[{"type":"gauge","help":"Health check metric","unit":""}]}}`)
			default:
				_, _ = fmt.Fprintf(w, `{"status":"success","data":["value-a","value-b"]}`)
				}
			}))

			client, err := prometheus.NewClient(prometheus.ClientConfig{
				URL: server.URL, Timeout: 5 * time.Second, SizeLimit: 30000,
			})
			Expect(err).NotTo(HaveOccurred())

			reg = registry.New()
			for _, t := range prometheus.NewAllTools(client) {
				reg.Register(t)
			}
		})

		AfterEach(func() { server.Close() })

		It("IT-KA-433-027: get_metric_names returns available metric names", func() {
			result, err := reg.Execute(context.Background(), "get_metric_names", json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("up"))
			Expect(result).To(ContainSubstring("container_memory_usage_bytes"))
		})

		It("IT-KA-433-028: get_label_values returns label values for metric", func() {
			result, err := reg.Execute(context.Background(), "get_label_values",
				json.RawMessage(`{"label":"namespace"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeEmpty())
		})

		It("IT-KA-433-029: get_all_labels returns all label names", func() {
			result, err := reg.Execute(context.Background(), "get_all_labels", json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("__name__"))
			Expect(result).To(ContainSubstring("namespace"))
		})

		It("IT-KA-433-030: get_metric_metadata returns metric help/type info", func() {
			result, err := reg.Execute(context.Background(), "get_metric_metadata", json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("gauge"))
			Expect(result).To(ContainSubstring("Health check metric"))
		})
	})

	Describe("IT-KA-433-031: Prometheus client respects timeout configuration", func() {
		It("should fail when server takes longer than configured timeout", func() {
			slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				time.Sleep(2 * time.Second)
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
			}))
			defer slowServer.Close()

			client, err := prometheus.NewClient(prometheus.ClientConfig{
				URL: slowServer.URL, Timeout: 100 * time.Millisecond, SizeLimit: 30000,
			})
			Expect(err).NotTo(HaveOccurred())

			reg := registry.New()
			for _, t := range prometheus.NewAllTools(client) {
				reg.Register(t)
			}

			_, err = reg.Execute(context.Background(), "execute_prometheus_instant_query",
				json.RawMessage(`{"query":"up"}`))
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("IT-KA-433-032: Prometheus client sends provider-specific auth headers", func() {
		It("should include custom auth headers in requests", func() {
			var receivedHeaders http.Header
			headerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedHeaders = r.Header.Clone()
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
			}))
			defer headerServer.Close()

			client, err := prometheus.NewClient(prometheus.ClientConfig{
				URL:     headerServer.URL,
				Headers: map[string]string{"X-Custom-Auth": "provider-token-123"},
				Timeout: 5 * time.Second, SizeLimit: 30000,
			})
			Expect(err).NotTo(HaveOccurred())

			reg := registry.New()
			for _, t := range prometheus.NewAllTools(client) {
				reg.Register(t)
			}

			_, err = reg.Execute(context.Background(), "execute_prometheus_instant_query",
				json.RawMessage(`{"query":"up"}`))
			Expect(err).NotTo(HaveOccurred())

			Expect(receivedHeaders).NotTo(BeNil())
			Expect(receivedHeaders.Get("X-Custom-Auth")).To(Equal("provider-token-123"))
		})
	})

	Describe("get_series integration (IT-040..042)", func() {
		var (
			capturedQuery url.Values
			serverBody    string
			server        *httptest.Server
			reg           *registry.Registry
		)

		BeforeEach(func() {
			capturedQuery = nil
			serverBody = `{"status":"success","data":[{"__name__":"up","job":"kubelet","instance":"10.0.0.1:10250"}]}`

			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/series" {
					capturedQuery = r.URL.Query()
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, serverBody)
			}))

			client, err := prometheus.NewClient(prometheus.ClientConfig{
				URL: server.URL, Timeout: 5 * time.Second, SizeLimit: 50000,
			})
			Expect(err).NotTo(HaveOccurred())

			reg = registry.New()
			for _, t := range prometheus.NewAllTools(client) {
				reg.Register(t)
			}
		})

		AfterEach(func() { server.Close() })

		It("IT-KA-433-040: get_series sends match[], limit, start, end to /api/v1/series", func() {
			result, err := reg.Execute(context.Background(), "get_series",
				json.RawMessage(`{"match":"up{job=\"kubelet\"}","start":"1709000000","end":"1709003600"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("up"))

			Expect(capturedQuery).NotTo(BeNil(), "server should have received /api/v1/series request")
			Expect(capturedQuery["match[]"]).To(ConsistOf(`up{job="kubelet"}`))
			Expect(capturedQuery["limit"]).To(ConsistOf("100"))
			Expect(capturedQuery["start"]).To(ConsistOf("1709000000"))
			Expect(capturedQuery["end"]).To(ConsistOf("1709003600"))
		})

		It("IT-KA-433-041: get_series defaults start to 1h ago and end to now when not provided", func() {
			before := time.Now().Unix()
			_, err := reg.Execute(context.Background(), "get_series",
				json.RawMessage(`{"match":"up"}`))
			Expect(err).NotTo(HaveOccurred())
			after := time.Now().Unix()

			Expect(capturedQuery).NotTo(BeNil())

			endVal := capturedQuery.Get("end")
			Expect(endVal).NotTo(BeEmpty(), "end should be defaulted")
			endTS, err := strconv.ParseInt(endVal, 10, 64)
			Expect(err).NotTo(HaveOccurred())
			Expect(endTS).To(BeNumerically(">=", before))
			Expect(endTS).To(BeNumerically("<=", after))

			startVal := capturedQuery.Get("start")
			Expect(startVal).NotTo(BeEmpty(), "start should be defaulted to ~1h ago")
			startTS, err := strconv.ParseInt(startVal, 10, 64)
			Expect(err).NotTo(HaveOccurred())
			Expect(startTS).To(BeNumerically("~", endTS-3600, 5),
				"default start should be ~1h before end")
		})

		It("IT-KA-433-042: get_series injects _truncated hint when response hits MetadataLimit", func() {
			items := make([]map[string]string, 100)
			for i := range items {
				items[i] = map[string]string{
					"__name__": fmt.Sprintf("metric_%d", i),
					"job":      "test",
				}
			}
			body, err := json.Marshal(map[string]interface{}{
				"status": "success",
				"data":   items,
			})
			Expect(err).NotTo(HaveOccurred())
			serverBody = string(body)

			result, err := reg.Execute(context.Background(), "get_series",
				json.RawMessage(`{"match":"{job=\"test\"}"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("_truncated"))
			Expect(result).To(ContainSubstring("more specific"))
		})
	})
})
