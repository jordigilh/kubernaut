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
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/alertmanager"
)

var _ = Describe("Alertmanager Client Unit — #1507", func() {

	Describe("UT-KA-1507-100: NewClient with valid config", func() {
		It("should create a non-nil client without error", func() {
			cfg := alertmanager.ClientConfig{
				URL:       "http://alertmanager:9093",
				Timeout:   10 * time.Second,
				SizeLimit: 50000,
			}
			client, err := alertmanager.NewClient(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil())
		})
	})

	Describe("UT-KA-1507-101: NewClient defaults SizeLimit", func() {
		It("should default SizeLimit to 30000 when zero", func() {
			cfg := alertmanager.ClientConfig{
				URL:       "http://alertmanager:9093",
				SizeLimit: 0,
			}
			client, err := alertmanager.NewClient(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(client.Config().SizeLimit).To(Equal(30000))
		})
	})

	Describe("UT-KA-1507-102: NewClient defaults Timeout", func() {
		It("should default Timeout to 30s when zero", func() {
			cfg := alertmanager.ClientConfig{
				URL:     "http://alertmanager:9093",
				Timeout: 0,
			}
			client, err := alertmanager.NewClient(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(client.Config().Timeout).To(Equal(30 * time.Second))
		})
	})

	Describe("UT-KA-1507-103: doGet successful GET with params", func() {
		It("should compose URL with query parameters and return response body", func() {
			var receivedPath string
			var receivedQuery string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedPath = r.URL.Path
				receivedQuery = r.URL.RawQuery
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[{"labels":{"alertname":"TestAlert"}}]`))
			}))
			defer server.Close()

			cfg := alertmanager.ClientConfig{URL: server.URL}
			client, err := alertmanager.NewClient(cfg)
			Expect(err).NotTo(HaveOccurred())

			result, err := client.DoGet(context.Background(), "/api/v2/alerts", map[string][]string{
				"active": {"true"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedPath).To(Equal("/api/v2/alerts"))
			Expect(receivedQuery).To(ContainSubstring("active=true"))
			Expect(result).To(ContainSubstring("TestAlert"))
		})
	})

	Describe("UT-KA-1507-104: doGet response truncation at SizeLimit", func() {
		It("should truncate response exceeding SizeLimit and append hint", func() {
			largeBody := strings.Repeat("x", 500)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(largeBody))
			}))
			defer server.Close()

			cfg := alertmanager.ClientConfig{URL: server.URL, SizeLimit: 100}
			client, err := alertmanager.NewClient(cfg)
			Expect(err).NotTo(HaveOccurred())

			result, err := client.DoGet(context.Background(), "/api/v2/alerts", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(result)).To(BeNumerically("<", 500))
			Expect(result).To(ContainSubstring("TRUNCATED"))
		})
	})

	Describe("UT-KA-1507-105: doGet HTTP error response (4xx)", func() {
		It("should return error with HTTP status for 404", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`not found`))
			}))
			defer server.Close()

			cfg := alertmanager.ClientConfig{URL: server.URL}
			client, err := alertmanager.NewClient(cfg)
			Expect(err).NotTo(HaveOccurred())

			_, err = client.DoGet(context.Background(), "/api/v2/alerts", nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("404"))
		})
	})

	Describe("UT-KA-1507-106: doGet HTTP error response (5xx)", func() {
		It("should return error with HTTP status for 500", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`internal server error`))
			}))
			defer server.Close()

			cfg := alertmanager.ClientConfig{URL: server.URL}
			client, err := alertmanager.NewClient(cfg)
			Expect(err).NotTo(HaveOccurred())

			_, err = client.DoGet(context.Background(), "/api/v2/alerts", nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("500"))
		})
	})

	Describe("UT-KA-1507-107: doGet network error", func() {
		It("should return connection error for unreachable server", func() {
			cfg := alertmanager.ClientConfig{URL: "http://127.0.0.1:1"}
			client, err := alertmanager.NewClient(cfg)
			Expect(err).NotTo(HaveOccurred())

			_, err = client.DoGet(context.Background(), "/api/v2/alerts", nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("alertmanager request"))
		})
	})

	Describe("UT-KA-1507-108: doGet context cancellation", func() {
		It("should return context error when context is cancelled", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				time.Sleep(5 * time.Second)
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			cfg := alertmanager.ClientConfig{URL: server.URL, Timeout: 100 * time.Millisecond}
			client, err := alertmanager.NewClient(cfg)
			Expect(err).NotTo(HaveOccurred())

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			_, err = client.DoGet(ctx, "/api/v2/alerts", nil)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("UT-KA-1507-109: doGet headers set from config", func() {
		It("should set custom headers on requests", func() {
			var capturedHeaders http.Header
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedHeaders = r.Header.Clone()
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[]`))
			}))
			defer server.Close()

			cfg := alertmanager.ClientConfig{
				URL:     server.URL,
				Headers: map[string]string{"X-Custom": "test-value"},
			}
			client, err := alertmanager.NewClient(cfg)
			Expect(err).NotTo(HaveOccurred())

			_, err = client.DoGet(context.Background(), "/api/v2/alerts", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(capturedHeaders.Get("X-Custom")).To(Equal("test-value"))
		})
	})

	Describe("UT-KA-1507-110: doGet custom Transport used", func() {
		It("should use the provided Transport for HTTP calls", func() {
			transportCalled := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[]`))
			}))
			defer server.Close()

			customTransport := &roundTripperFunc{fn: func(req *http.Request) (*http.Response, error) {
				transportCalled = true
				return http.DefaultTransport.RoundTrip(req)
			}}

			cfg := alertmanager.ClientConfig{URL: server.URL, Transport: customTransport}
			client, err := alertmanager.NewClient(cfg)
			Expect(err).NotTo(HaveOccurred())

			_, err = client.DoGet(context.Background(), "/api/v2/alerts", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(transportCalled).To(BeTrue())
		})
	})

	Describe("UT-KA-1507-103b: doGet with nil params", func() {
		It("should succeed without query string when params is nil", func() {
			var receivedQuery string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedQuery = r.URL.RawQuery
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[]`))
			}))
			defer server.Close()

			cfg := alertmanager.ClientConfig{URL: server.URL}
			client, err := alertmanager.NewClient(cfg)
			Expect(err).NotTo(HaveOccurred())

			result, err := client.DoGet(context.Background(), "/api/v2/alerts", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedQuery).To(BeEmpty())
			Expect(result).To(Equal("[]"))
		})
	})

	Describe("UT-KA-1507-103c: doGet with invalid URL returns error", func() {
		It("should return error for malformed base URL", func() {
			cfg := alertmanager.ClientConfig{URL: "://invalid"}
			client, err := alertmanager.NewClient(cfg)
			Expect(err).NotTo(HaveOccurred())

			_, err = client.DoGet(context.Background(), "/api/v2/alerts", nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("building URL"))
		})
	})

	Describe("UT-KA-1507-111: Config() returns stored config", func() {
		It("should return the exact config passed to NewClient", func() {
			cfg := alertmanager.ClientConfig{
				URL:       "http://alertmanager:9093",
				Timeout:   15 * time.Second,
				SizeLimit: 45000,
			}
			client, err := alertmanager.NewClient(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(client.Config().URL).To(Equal("http://alertmanager:9093"))
			Expect(client.Config().Timeout).To(Equal(15 * time.Second))
			Expect(client.Config().SizeLimit).To(Equal(45000))
		})
	})
})

type roundTripperFunc struct {
	fn func(*http.Request) (*http.Response, error)
}

func (rt *roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return rt.fn(req)
}
