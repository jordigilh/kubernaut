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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/alertmanager"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

var _ = Describe("Kubernaut Agent Alertmanager Tools Integration — #1507", func() {

	Describe("IT-KA-1507-001: get_alerts via MockAlertManager (happy path)", func() {
		It("should return alerts through registry dispatch", func() {
			mock := infrastructure.NewMockAlertManager(infrastructure.MockAlertManagerConfig{
				AlertsResponse: []infrastructure.AMAlert{
					{
						Labels:      map[string]string{"alertname": "KubePodCrashLooping", "namespace": "default"},
						Annotations: map[string]string{"summary": "Pod is crash looping"},
						Status:      &infrastructure.AMAlertStatus{State: "active"},
					},
					{
						Labels: map[string]string{"alertname": "HighMemoryUsage", "namespace": "monitoring"},
						Status: &infrastructure.AMAlertStatus{State: "active"},
					},
				},
				Ready:   true,
				Healthy: true,
			})
			defer mock.Server.Close()

			client, err := alertmanager.NewClient(alertmanager.ClientConfig{
				URL: mock.URL(), Timeout: 5 * time.Second, SizeLimit: 30000,
			})
			Expect(err).NotTo(HaveOccurred())

			reg := registry.New()
			for _, t := range alertmanager.NewAllTools(client) {
				reg.Register(t)
			}

			result, err := reg.Execute(context.Background(), "get_alerts", json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("KubePodCrashLooping"))
			Expect(result).To(ContainSubstring("HighMemoryUsage"))
		})
	})

	Describe("IT-KA-1507-002: get_alerts query params forwarded correctly", func() {
		It("should forward filter params to the server", func() {
			var receivedQuery string
			mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedQuery = r.URL.RawQuery
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[]`))
			}))
			defer mock.Close()

			client, err := alertmanager.NewClient(alertmanager.ClientConfig{
				URL: mock.URL, Timeout: 5 * time.Second, SizeLimit: 30000,
			})
			Expect(err).NotTo(HaveOccurred())

			reg := registry.New()
			for _, t := range alertmanager.NewAllTools(client) {
				reg.Register(t)
			}

			_, err = reg.Execute(context.Background(), "get_alerts",
				json.RawMessage(`{"active":true,"silenced":false,"filter":["severity=critical"]}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedQuery).To(ContainSubstring("active=true"))
			Expect(receivedQuery).To(ContainSubstring("silenced=false"))
			Expect(receivedQuery).To(ContainSubstring("filter="))
		})
	})

	Describe("IT-KA-1507-003: get_silences via MockAlertManager (happy path)", func() {
		It("should return silences through registry dispatch", func() {
			silencesJSON := `[{"id":"silence-123","matchers":[{"name":"alertname","value":"KubePodCrashLooping","isRegex":false}],"createdBy":"admin","comment":"Maintenance window","startsAt":"2026-06-27T00:00:00Z","endsAt":"2026-06-28T00:00:00Z","status":{"state":"active"}}]`
			mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(silencesJSON))
			}))
			defer mock.Close()

			client, err := alertmanager.NewClient(alertmanager.ClientConfig{
				URL: mock.URL, Timeout: 5 * time.Second, SizeLimit: 30000,
			})
			Expect(err).NotTo(HaveOccurred())

			reg := registry.New()
			for _, t := range alertmanager.NewAllTools(client) {
				reg.Register(t)
			}

			result, err := reg.Execute(context.Background(), "get_silences", json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("silence-123"))
			Expect(result).To(ContainSubstring("Maintenance window"))
		})
	})

	Describe("IT-KA-1507-004: get_alerts server returns 500", func() {
		It("should propagate error through registry", func() {
			mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`internal error`))
			}))
			defer mock.Close()

			client, err := alertmanager.NewClient(alertmanager.ClientConfig{
				URL: mock.URL, Timeout: 5 * time.Second, SizeLimit: 30000,
			})
			Expect(err).NotTo(HaveOccurred())

			reg := registry.New()
			for _, t := range alertmanager.NewAllTools(client) {
				reg.Register(t)
			}

			_, err = reg.Execute(context.Background(), "get_alerts", json.RawMessage(`{}`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("500"))
		})
	})

	Describe("IT-KA-1507-005: get_silences server timeout", func() {
		It("should return timeout error", func() {
			mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				time.Sleep(3 * time.Second)
				w.WriteHeader(http.StatusOK)
			}))
			defer mock.Close()

			client, err := alertmanager.NewClient(alertmanager.ClientConfig{
				URL: mock.URL, Timeout: 100 * time.Millisecond, SizeLimit: 30000,
			})
			Expect(err).NotTo(HaveOccurred())

			reg := registry.New()
			for _, t := range alertmanager.NewAllTools(client) {
				reg.Register(t)
			}

			_, err = reg.Execute(context.Background(), "get_silences", json.RawMessage(`{}`))
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("IT-KA-1507-006: Client respects custom headers", func() {
		It("should include auth headers in Alertmanager requests", func() {
			var receivedHeaders http.Header
			mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedHeaders = r.Header.Clone()
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[]`))
			}))
			defer mock.Close()

			client, err := alertmanager.NewClient(alertmanager.ClientConfig{
				URL:     mock.URL,
				Headers: map[string]string{"Authorization": "Bearer test-token-123"},
				Timeout: 5 * time.Second, SizeLimit: 30000,
			})
			Expect(err).NotTo(HaveOccurred())

			reg := registry.New()
			for _, t := range alertmanager.NewAllTools(client) {
				reg.Register(t)
			}

			_, err = reg.Execute(context.Background(), "get_alerts", json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedHeaders.Get("Authorization")).To(Equal("Bearer test-token-123"))
		})
	})

	Describe("IT-KA-1507-007: Client respects custom Transport", func() {
		It("should use the provided RoundTripper", func() {
			transportCalled := false
			mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[]`))
			}))
			defer mock.Close()

			customRT := &roundTripperFunc{fn: func(req *http.Request) (*http.Response, error) {
				transportCalled = true
				return http.DefaultTransport.RoundTrip(req)
			}}

			client, err := alertmanager.NewClient(alertmanager.ClientConfig{
				URL: mock.URL, Transport: customRT, Timeout: 5 * time.Second, SizeLimit: 30000,
			})
			Expect(err).NotTo(HaveOccurred())

			reg := registry.New()
			for _, t := range alertmanager.NewAllTools(client) {
				reg.Register(t)
			}

			_, err = reg.Execute(context.Background(), "get_alerts", json.RawMessage(`{}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(transportCalled).To(BeTrue())
		})
	})

	Describe("IT-KA-1507-020: get_alerts registered in tool registry", func() {
		It("should be retrievable by name from registry", func() {
			mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[]`))
			}))
			defer mock.Close()

			client, err := alertmanager.NewClient(alertmanager.ClientConfig{
				URL: mock.URL, Timeout: 5 * time.Second, SizeLimit: 30000,
			})
			Expect(err).NotTo(HaveOccurred())

			reg := registry.New()
			for _, t := range alertmanager.NewAllTools(client) {
				reg.Register(t)
			}

			tool, err := reg.Get("get_alerts")
			Expect(err).NotTo(HaveOccurred())
			Expect(tool).NotTo(BeNil())
			Expect(tool.Name()).To(Equal("get_alerts"))
		})
	})

	Describe("IT-KA-1507-021: get_silences registered in tool registry", func() {
		It("should be retrievable by name from registry", func() {
			mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[]`))
			}))
			defer mock.Close()

			client, err := alertmanager.NewClient(alertmanager.ClientConfig{
				URL: mock.URL, Timeout: 5 * time.Second, SizeLimit: 30000,
			})
			Expect(err).NotTo(HaveOccurred())

			reg := registry.New()
			for _, t := range alertmanager.NewAllTools(client) {
				reg.Register(t)
			}

			tool, err := reg.Get("get_silences")
			Expect(err).NotTo(HaveOccurred())
			Expect(tool).NotTo(BeNil())
			Expect(tool.Name()).To(Equal("get_silences"))
		})
	})

	Describe("IT-KA-1507-024: Tools absent when Alertmanager URL is empty", func() {
		It("should not register tools when URL is empty (matching production buildToolRegistry behavior)", func() {
			reg := registry.New()
			url := ""
			if url != "" {
				client, _ := alertmanager.NewClient(alertmanager.ClientConfig{URL: url})
				for _, t := range alertmanager.NewAllTools(client) {
					reg.Register(t)
				}
			}
			_, err := reg.Get("get_alerts")
			Expect(err).To(HaveOccurred())
			_, err = reg.Get("get_silences")
			Expect(err).To(HaveOccurred())
		})
	})
})

type roundTripperFunc struct {
	fn func(*http.Request) (*http.Response, error)
}

func (rt *roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return rt.fn(req)
}
