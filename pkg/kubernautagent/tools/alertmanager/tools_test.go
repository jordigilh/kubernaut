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

package alertmanager_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/alertmanager"
)

var _ = Describe("Alertmanager Tools Unit — #1507", func() {

	var (
		server *httptest.Server
		client *alertmanager.Client
		tools  []alertmanager.Tool
	)

	setupServer := func(handler http.HandlerFunc) {
		server = httptest.NewServer(handler)
		cfg := alertmanager.ClientConfig{URL: server.URL, SizeLimit: 30000}
		var err error
		client, err = alertmanager.NewClient(cfg)
		Expect(err).NotTo(HaveOccurred())
		tools = alertmanager.NewAllTools(client)
	}

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	findTool := func(name string) alertmanager.Tool {
		for _, t := range tools {
			if t.Name() == name {
				return t
			}
		}
		return nil
	}

	// --- get_alerts ---

	Describe("UT-KA-1507-030: get_alerts happy path (no filters)", func() {
		It("should return raw JSON from /api/v2/alerts", func() {
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Path).To(Equal("/api/v2/alerts"))
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[{"labels":{"alertname":"KubePodCrashLooping"}}]`))
			}))

			tool := findTool("get_alerts")
			Expect(tool).NotTo(BeNil())

			result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("KubePodCrashLooping"))
		})
	})

	Describe("UT-KA-1507-031: get_alerts filter active=true", func() {
		It("should send active=true query param", func() {
			var receivedQuery string
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedQuery = r.URL.RawQuery
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[]`))
			}))

			tool := findTool("get_alerts")
			_, err := tool.Execute(context.Background(), json.RawMessage(`{"active":true}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedQuery).To(ContainSubstring("active=true"))
		})
	})

	Describe("UT-KA-1507-032: get_alerts filter silenced=true", func() {
		It("should send silenced=true query param", func() {
			var receivedQuery string
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedQuery = r.URL.RawQuery
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[]`))
			}))

			tool := findTool("get_alerts")
			_, err := tool.Execute(context.Background(), json.RawMessage(`{"silenced":true}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedQuery).To(ContainSubstring("silenced=true"))
		})
	})

	Describe("UT-KA-1507-033: get_alerts filter inhibited=true", func() {
		It("should send inhibited=true query param", func() {
			var receivedQuery string
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedQuery = r.URL.RawQuery
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[]`))
			}))

			tool := findTool("get_alerts")
			_, err := tool.Execute(context.Background(), json.RawMessage(`{"inhibited":true}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedQuery).To(ContainSubstring("inhibited=true"))
		})
	})

	Describe("UT-KA-1507-034: get_alerts filter receiver name", func() {
		It("should send receiver query param", func() {
			var receivedQuery string
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedQuery = r.URL.RawQuery
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[]`))
			}))

			tool := findTool("get_alerts")
			_, err := tool.Execute(context.Background(), json.RawMessage(`{"receiver":"slack-alerts"}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedQuery).To(ContainSubstring("receiver=slack-alerts"))
		})
	})

	Describe("UT-KA-1507-035: get_alerts filter label matchers", func() {
		It("should send filter query params for matchers", func() {
			var receivedQuery string
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedQuery = r.URL.RawQuery
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[]`))
			}))

			tool := findTool("get_alerts")
			_, err := tool.Execute(context.Background(), json.RawMessage(`{"filter":["alertname=~KubePod.*"]}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedQuery).To(ContainSubstring("filter="))
		})
	})

	Describe("UT-KA-1507-036: get_alerts combined filters", func() {
		It("should compose all query params correctly", func() {
			var receivedQuery string
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedQuery = r.URL.RawQuery
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[]`))
			}))

			tool := findTool("get_alerts")
			_, err := tool.Execute(context.Background(), json.RawMessage(`{"active":true,"silenced":false,"filter":["severity=critical"]}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedQuery).To(ContainSubstring("active=true"))
			Expect(receivedQuery).To(ContainSubstring("silenced=false"))
			Expect(receivedQuery).To(ContainSubstring("filter="))
		})
	})

	Describe("UT-KA-1507-037: get_alerts HTTP error", func() {
		It("should return error with status code context", func() {
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`service unavailable`))
			}))

			tool := findTool("get_alerts")
			_, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("503"))
		})
	})

	Describe("UT-KA-1507-038: get_alerts response exceeds size limit", func() {
		It("should truncate with hint", func() {
			largeJSON := "[" + strings.Repeat(`{"labels":{"a":"b"}},`, 5000) + `{"labels":{"a":"b"}}]`
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(largeJSON))
			}))

			cfg := alertmanager.ClientConfig{URL: server.URL, SizeLimit: 500}
			var err error
			client, err = alertmanager.NewClient(cfg)
			Expect(err).NotTo(HaveOccurred())
			tools = alertmanager.NewAllTools(client)

			tool := findTool("get_alerts")
			result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("TRUNCATED"))
		})
	})

	Describe("UT-KA-1507-039: get_alerts context cancelled", func() {
		It("should propagate context error", func() {
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				time.Sleep(5 * time.Second)
				w.WriteHeader(http.StatusOK)
			}))

			tool := findTool("get_alerts")
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			_, err := tool.Execute(ctx, json.RawMessage(`{}`))
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("UT-KA-1507-040: get_alerts Name()", func() {
		It("should return 'get_alerts'", func() {
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[]`))
			}))
			tool := findTool("get_alerts")
			Expect(tool).NotTo(BeNil())
			Expect(tool.Name()).To(Equal("get_alerts"))
		})
	})

	Describe("UT-KA-1507-041: get_alerts Description()", func() {
		It("should return non-empty description", func() {
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[]`))
			}))
			tool := findTool("get_alerts")
			Expect(tool.Description()).NotTo(BeEmpty())
		})
	})

	Describe("UT-KA-1507-042: get_alerts Parameters()", func() {
		It("should return valid JSON schema", func() {
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[]`))
			}))
			tool := findTool("get_alerts")
			params := tool.Parameters()
			Expect(params).NotTo(BeNil())
			var schema map[string]interface{}
			err := json.Unmarshal(params, &schema)
			Expect(err).NotTo(HaveOccurred())
			Expect(schema["type"]).To(Equal("object"))
		})
	})

	Describe("UT-KA-1507-043: get_alerts nil args (no filters)", func() {
		It("should treat nil args as empty object and return all alerts", func() {
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[{"labels":{"alertname":"Test"}}]`))
			}))

			tool := findTool("get_alerts")
			result, err := tool.Execute(context.Background(), nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("Test"))
		})
	})

	Describe("UT-KA-1507-044: get_alerts malformed JSON args", func() {
		It("should return parsing error", func() {
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[]`))
			}))

			tool := findTool("get_alerts")
			_, err := tool.Execute(context.Background(), json.RawMessage(`{invalid`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("parsing args"))
		})
	})

	Describe("UT-KA-1507-045: get_alerts empty filter array", func() {
		It("should not send filter query param when array is empty", func() {
			var receivedQuery string
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedQuery = r.URL.RawQuery
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[]`))
			}))

			tool := findTool("get_alerts")
			_, err := tool.Execute(context.Background(), json.RawMessage(`{"filter":[]}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedQuery).NotTo(ContainSubstring("filter="))
		})
	})

	// --- get_silences ---

	Describe("UT-KA-1507-050: get_silences happy path (no filters)", func() {
		It("should return raw JSON from /api/v2/silences", func() {
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Path).To(Equal("/api/v2/silences"))
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[{"id":"abc123","matchers":[{"name":"alertname","value":"Test"}]}]`))
			}))

			tool := findTool("get_silences")
			Expect(tool).NotTo(BeNil())

			result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("abc123"))
		})
	})

	Describe("UT-KA-1507-051: get_silences filter label matchers", func() {
		It("should send filter query params", func() {
			var receivedQuery string
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedQuery = r.URL.RawQuery
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[]`))
			}))

			tool := findTool("get_silences")
			_, err := tool.Execute(context.Background(), json.RawMessage(`{"filter":["alertname=KubePodCrashLooping"]}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedQuery).To(ContainSubstring("filter="))
		})
	})

	Describe("UT-KA-1507-052: get_silences HTTP error", func() {
		It("should return error with context", func() {
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`error`))
			}))

			tool := findTool("get_silences")
			_, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("500"))
		})
	})

	Describe("UT-KA-1507-053: get_silences response exceeds size limit", func() {
		It("should truncate with hint", func() {
			largeJSON := "[" + strings.Repeat(`{"id":"x","matchers":[]},`, 5000) + `{"id":"y","matchers":[]}]`
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(largeJSON))
			}))

			cfg := alertmanager.ClientConfig{URL: server.URL, SizeLimit: 500}
			var err error
			client, err = alertmanager.NewClient(cfg)
			Expect(err).NotTo(HaveOccurred())
			tools = alertmanager.NewAllTools(client)

			tool := findTool("get_silences")
			result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("TRUNCATED"))
		})
	})

	Describe("UT-KA-1507-054: get_silences context cancelled", func() {
		It("should propagate context error", func() {
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				time.Sleep(5 * time.Second)
				w.WriteHeader(http.StatusOK)
			}))

			tool := findTool("get_silences")
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			_, err := tool.Execute(ctx, json.RawMessage(`{}`))
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("UT-KA-1507-055: get_silences Name()", func() {
		It("should return 'get_silences'", func() {
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[]`))
			}))
			tool := findTool("get_silences")
			Expect(tool).NotTo(BeNil())
			Expect(tool.Name()).To(Equal("get_silences"))
		})
	})

	Describe("UT-KA-1507-056: get_silences Description()", func() {
		It("should return non-empty description", func() {
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[]`))
			}))
			tool := findTool("get_silences")
			Expect(tool.Description()).NotTo(BeEmpty())
		})
	})

	Describe("UT-KA-1507-057: get_silences Parameters()", func() {
		It("should return valid JSON schema", func() {
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[]`))
			}))
			tool := findTool("get_silences")
			params := tool.Parameters()
			Expect(params).NotTo(BeNil())
			var schema map[string]interface{}
			err := json.Unmarshal(params, &schema)
			Expect(err).NotTo(HaveOccurred())
			Expect(schema["type"]).To(Equal("object"))
		})
	})

	Describe("UT-KA-1507-058: get_silences nil args (no filters)", func() {
		It("should treat nil as no filters", func() {
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[{"id":"silence1"}]`))
			}))

			tool := findTool("get_silences")
			result, err := tool.Execute(context.Background(), nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("silence1"))
		})
	})

	Describe("UT-KA-1507-059: get_silences malformed JSON args", func() {
		It("should return parsing error", func() {
			setupServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[]`))
			}))

			tool := findTool("get_silences")
			_, err := tool.Execute(context.Background(), json.RawMessage(`{invalid`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("parsing args"))
		})
	})
})
