/*
Copyright 2025 Jordi Gil.

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

package aianalysis

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
)

// BR-AI-006: HolmesGPT-API client integration
var _ = Describe("HolmesGPTClient", func() {
	var (
		mockServer *httptest.Server
		hgClient   *client.HolmesGPTClient
		ctx        context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	AfterEach(func() {
		if mockServer != nil {
			mockServer.Close()
		}
	})

	Describe("Investigate", func() {
		// BR-AI-006: Successful API call
		Context("with successful response", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.URL.Path).To(Equal("/api/v1/incident/analyze"))
					Expect(r.Method).To(Equal(http.MethodPost))

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{
						"incident_id": "test-incident-001",
						"analysis": "Root cause: OOM",
						"root_cause_analysis": {
							"root_cause": "OOM detected",
							"recommendations": ["Increase memory limits"]
						},
						"target_in_owner_chain": true,
						"confidence": 0.85,
						"timestamp": "2025-12-24T15:00:00Z",
						"warnings": []
					}`))
				}))

				var err error
				hgClient, err = client.NewHolmesGPTClient(client.Config{
					BaseURL: mockServer.URL,
				})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return valid response", func() {
				resp, err := hgClient.Investigate(ctx, &client.IncidentRequest{
					IncidentID:        "test-incident-001",
					RemediationID:     "test-rem-001",
					SignalType:        "OOMKilled",
					Severity:          "critical",
					ResourceNamespace: "default",
					ResourceKind:      "Pod",
					ResourceName:      "test-pod",
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.Analysis).To(Equal("Root cause: OOM"))
				Expect(resp.TargetInOwnerChain.Set).To(BeTrue())
				Expect(resp.TargetInOwnerChain.Value).To(BeTrue())
				Expect(resp.Confidence).To(BeNumerically("~", 0.85, 0.01))
			})
		})

		// BR-AI-009: Transient error handling (503)
		Context("with 503 Service Unavailable", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusServiceUnavailable)
				}))
				var err error
				hgClient, err = client.NewHolmesGPTClient(client.Config{BaseURL: mockServer.URL})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return transient error", func() {
				_, err := hgClient.Investigate(ctx, &client.IncidentRequest{})

				Expect(err).To(HaveOccurred())
				var apiErr *client.APIError
				Expect(errors.As(err, &apiErr)).To(BeTrue())
			})
		})

		// BR-AI-009: Transient error handling (500)
		Context("with 500 Internal Server Error", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}))
				var err error
				hgClient, err = client.NewHolmesGPTClient(client.Config{BaseURL: mockServer.URL})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return transient error", func() {
				_, err := hgClient.Investigate(ctx, &client.IncidentRequest{})

				Expect(err).To(HaveOccurred())
				var apiErr *client.APIError
				Expect(errors.As(err, &apiErr)).To(BeTrue())
			})
		})

		// BR-AI-010: Permanent error handling (401)
		Context("with 401 Unauthorized", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusUnauthorized)
				}))
				var err error
				hgClient, err = client.NewHolmesGPTClient(client.Config{BaseURL: mockServer.URL})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return permanent error", func() {
				_, err := hgClient.Investigate(ctx, &client.IncidentRequest{})

				Expect(err).To(HaveOccurred())
				var apiErr *client.APIError
				Expect(errors.As(err, &apiErr)).To(BeTrue())
			})
		})

		// BR-AI-010: Permanent error handling (400)
		Context("with 400 Bad Request", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusBadRequest)
				}))
				var err error
				hgClient, err = client.NewHolmesGPTClient(client.Config{BaseURL: mockServer.URL})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return permanent error", func() {
				_, err := hgClient.Investigate(ctx, &client.IncidentRequest{})

				Expect(err).To(HaveOccurred())
				var apiErr *client.APIError
				Expect(errors.As(err, &apiErr)).To(BeTrue())
			})
		})

		// BR-AI-009: Transient error handling (429)
		Context("with 429 Too Many Requests", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusTooManyRequests)
				}))
				var err error
				hgClient, err = client.NewHolmesGPTClient(client.Config{BaseURL: mockServer.URL})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return transient error", func() {
				_, err := hgClient.Investigate(ctx, &client.IncidentRequest{})

				Expect(err).To(HaveOccurred())
				var apiErr *client.APIError
				Expect(errors.As(err, &apiErr)).To(BeTrue())
			})
		})
	})
})
