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

package aianalysis

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
)

// BR-AA-HAPI-064: Session-based async pull communication unit tests for HolmesGPTClient.
// These tests verify the HTTP-level behavior of the 5 session methods using httptest.NewServer.
var _ = Describe("HolmesGPTClient Session Methods [BR-AA-HAPI-064]", func() {
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

	// ========================================
	// SubmitInvestigation (POST /api/v1/incident/analyze → 202)
	// ========================================
	Describe("SubmitInvestigation", func() {

		// UT-AA-064-020: Success — 202 Accepted with session_id
		Context("with successful 202 response [UT-AA-064-020]", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.URL.Path).To(Equal("/api/v1/incident/analyze"))
					Expect(r.Method).To(Equal(http.MethodPost))

					// Verify request body is valid JSON with required fields
					var body map[string]interface{}
					Expect(json.NewDecoder(r.Body).Decode(&body)).To(Succeed())
					Expect(body).To(HaveKey("incident_id"))
					Expect(body).To(HaveKey("remediation_id"))

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusAccepted)
					_, _ = w.Write([]byte(`{"session_id": "sess-abc-123"}`))
				}))

				var err error
				hgClient, err = client.NewHolmesGPTClient(client.Config{BaseURL: mockServer.URL})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return session ID", func() {
				sessionID, err := hgClient.SubmitInvestigation(ctx, &client.IncidentRequest{
					IncidentID:        "test-incident-001",
					RemediationID:     "test-rem-001",
					SignalType:        "OOMKilled",
					ResourceNamespace: "default",
					ResourceKind:      "Pod",
					ResourceName:      "test-pod",
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(sessionID).To(Equal("sess-abc-123"))
			})
		})

		// UT-AA-064-021: Transient error — 503 Service Unavailable
		Context("with 503 Service Unavailable [UT-AA-064-021]", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusServiceUnavailable)
				}))
				var err error
				hgClient, err = client.NewHolmesGPTClient(client.Config{BaseURL: mockServer.URL})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return APIError with status 503", func() {
				_, err := hgClient.SubmitInvestigation(ctx, &client.IncidentRequest{
					IncidentID:    "test-incident-001",
					RemediationID: "test-rem-001",
				})

				Expect(err).To(HaveOccurred())
				var apiErr *client.APIError
				Expect(errors.As(err, &apiErr)).To(BeTrue())
				Expect(apiErr.StatusCode).To(Equal(http.StatusServiceUnavailable))
			})
		})

		// UT-AA-064-022: Permanent error — 401 Unauthorized
		Context("with 401 Unauthorized [UT-AA-064-022]", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusUnauthorized)
				}))
				var err error
				hgClient, err = client.NewHolmesGPTClient(client.Config{BaseURL: mockServer.URL})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return APIError with status 401", func() {
				_, err := hgClient.SubmitInvestigation(ctx, &client.IncidentRequest{
					IncidentID:    "test-incident-001",
					RemediationID: "test-rem-001",
				})

				Expect(err).To(HaveOccurred())
				var apiErr *client.APIError
				Expect(errors.As(err, &apiErr)).To(BeTrue())
				Expect(apiErr.StatusCode).To(Equal(http.StatusUnauthorized))
			})
		})
	})

	// ========================================
	// SubmitRecoveryInvestigation (POST /api/v1/recovery/analyze → 202)
	// ========================================
	Describe("SubmitRecoveryInvestigation", func() {

		// UT-AA-064-023: Success — 202 Accepted with session_id
		Context("with successful 202 response [UT-AA-064-023]", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.URL.Path).To(Equal("/api/v1/recovery/analyze"))
					Expect(r.Method).To(Equal(http.MethodPost))

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusAccepted)
					_, _ = w.Write([]byte(`{"session_id": "sess-recovery-456"}`))
				}))

				var err error
				hgClient, err = client.NewHolmesGPTClient(client.Config{BaseURL: mockServer.URL})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return session ID", func() {
				sessionID, err := hgClient.SubmitRecoveryInvestigation(ctx, &client.RecoveryRequest{
					IncidentID:    "test-incident-002",
					RemediationID: "test-rem-002",
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(sessionID).To(Equal("sess-recovery-456"))
			})
		})

		// UT-AA-064-024: Transient error — 500 Internal Server Error
		Context("with 500 Internal Server Error [UT-AA-064-024]", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}))
				var err error
				hgClient, err = client.NewHolmesGPTClient(client.Config{BaseURL: mockServer.URL})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return APIError with status 500", func() {
				_, err := hgClient.SubmitRecoveryInvestigation(ctx, &client.RecoveryRequest{
					IncidentID:    "test-incident-002",
					RemediationID: "test-rem-002",
				})

				Expect(err).To(HaveOccurred())
				var apiErr *client.APIError
				Expect(errors.As(err, &apiErr)).To(BeTrue())
				Expect(apiErr.StatusCode).To(Equal(http.StatusInternalServerError))
			})
		})
	})

	// ========================================
	// PollSession (GET /api/v1/incident/session/{id})
	// ========================================
	Describe("PollSession", func() {

		// UT-AA-064-025: Status "pending" — session still processing
		Context("with status pending [UT-AA-064-025]", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.URL.Path).To(Equal("/api/v1/incident/session/sess-poll-001"))
					Expect(r.Method).To(Equal(http.MethodGet))

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"status": "pending", "created_at": "2026-02-09T10:00:00Z"}`))
				}))

				var err error
				hgClient, err = client.NewHolmesGPTClient(client.Config{BaseURL: mockServer.URL})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return SessionStatus with pending", func() {
				status, err := hgClient.PollSession(ctx, "sess-poll-001")

				Expect(err).NotTo(HaveOccurred())
				Expect(status).NotTo(BeNil())
				Expect(status.Status).To(Equal("pending"))
			})
		})

		// UT-AA-064-026: Status "completed" — investigation finished
		Context("with status completed [UT-AA-064-026]", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"status": "completed"}`))
				}))

				var err error
				hgClient, err = client.NewHolmesGPTClient(client.Config{BaseURL: mockServer.URL})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return SessionStatus with completed", func() {
				status, err := hgClient.PollSession(ctx, "sess-poll-002")

				Expect(err).NotTo(HaveOccurred())
				Expect(status).NotTo(BeNil())
				Expect(status.Status).To(Equal("completed"))
			})
		})

		// UT-AA-064-027: Session lost — 404 Not Found (HAPI restarted)
		Context("with 404 Not Found [UT-AA-064-027]", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte(`{"detail": "Session sess-lost not found"}`))
				}))

				var err error
				hgClient, err = client.NewHolmesGPTClient(client.Config{BaseURL: mockServer.URL})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return APIError with status 404", func() {
				_, err := hgClient.PollSession(ctx, "sess-lost")

				Expect(err).To(HaveOccurred())
				var apiErr *client.APIError
				Expect(errors.As(err, &apiErr)).To(BeTrue())
				Expect(apiErr.StatusCode).To(Equal(http.StatusNotFound))
			})
		})
	})

	// ========================================
	// GetSessionResult (GET /api/v1/incident/session/{id}/result)
	// ========================================
	Describe("GetSessionResult", func() {

		// UT-AA-064-028: Success — full IncidentResponse returned
		Context("with completed result [UT-AA-064-028]", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.URL.Path).To(Equal("/api/v1/incident/session/sess-result-001/result"))
					Expect(r.Method).To(Equal(http.MethodGet))

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{
						"incident_id": "test-incident-result",
						"analysis": "Root cause: memory leak",
						"root_cause_analysis": {
							"root_cause": "Memory leak detected",
							"recommendations": ["Increase memory limits"]
						},
						"target_in_owner_chain": true,
						"confidence": 0.92,
						"timestamp": "2026-02-09T10:30:00Z",
						"warnings": []
					}`))
				}))

				var err error
				hgClient, err = client.NewHolmesGPTClient(client.Config{BaseURL: mockServer.URL})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return valid IncidentResponse", func() {
				resp, err := hgClient.GetSessionResult(ctx, "sess-result-001")

				Expect(err).NotTo(HaveOccurred())
				Expect(resp).NotTo(BeNil())
				Expect(resp.IncidentID).To(Equal("test-incident-result"))
				Expect(resp.Analysis).To(Equal("Root cause: memory leak"))
				Expect(resp.Confidence).To(BeNumerically("~", 0.92, 0.01))
			})
		})

		// UT-AA-064-029: Not ready — 409 Conflict (session still in progress)
		Context("with 409 Conflict [UT-AA-064-029]", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusConflict)
					_, _ = w.Write([]byte(`{"detail": "Session not yet completed (status: investigating)"}`))
				}))

				var err error
				hgClient, err = client.NewHolmesGPTClient(client.Config{BaseURL: mockServer.URL})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return APIError with status 409", func() {
				_, err := hgClient.GetSessionResult(ctx, "sess-result-pending")

				Expect(err).To(HaveOccurred())
				var apiErr *client.APIError
				Expect(errors.As(err, &apiErr)).To(BeTrue())
				Expect(apiErr.StatusCode).To(Equal(http.StatusConflict))
			})
		})

		// UT-AA-064-030: Session not found — 404 (HAPI restarted)
		Context("with 404 Not Found [UT-AA-064-030]", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNotFound)
				}))

				var err error
				hgClient, err = client.NewHolmesGPTClient(client.Config{BaseURL: mockServer.URL})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return APIError with status 404", func() {
				_, err := hgClient.GetSessionResult(ctx, "sess-gone")

				Expect(err).To(HaveOccurred())
				var apiErr *client.APIError
				Expect(errors.As(err, &apiErr)).To(BeTrue())
				Expect(apiErr.StatusCode).To(Equal(http.StatusNotFound))
			})
		})
	})

	// ========================================
	// GetRecoverySessionResult (GET /api/v1/recovery/session/{id}/result)
	// ========================================
	Describe("GetRecoverySessionResult", func() {

		// UT-AA-064-031: Success — full RecoveryResponse returned
		Context("with completed result [UT-AA-064-031]", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.URL.Path).To(Equal("/api/v1/recovery/session/sess-recovery-result-001/result"))
					Expect(r.Method).To(Equal(http.MethodGet))

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{
						"incident_id": "test-incident-recovery",
						"can_recover": true,
						"strategies": [{
							"action_type": "increase_memory",
							"confidence": 0.88,
							"rationale": "Memory limits too low",
							"estimated_risk": "low",
							"prerequisites": []
						}],
						"analysis_confidence": 0.88,
						"warnings": []
					}`))
				}))

				var err error
				hgClient, err = client.NewHolmesGPTClient(client.Config{BaseURL: mockServer.URL})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return valid RecoveryResponse", func() {
				resp, err := hgClient.GetRecoverySessionResult(ctx, "sess-recovery-result-001")

				Expect(err).NotTo(HaveOccurred())
				Expect(resp).NotTo(BeNil())
				Expect(resp.IncidentID).To(Equal("test-incident-recovery"))
				Expect(resp.CanRecover).To(BeTrue())
				Expect(resp.AnalysisConfidence).To(BeNumerically("~", 0.88, 0.01))
			})
		})

		// UT-AA-064-032: Not ready — 409 Conflict
		Context("with 409 Conflict [UT-AA-064-032]", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusConflict)
					_, _ = w.Write([]byte(`{"detail": "Session not yet completed"}`))
				}))

				var err error
				hgClient, err = client.NewHolmesGPTClient(client.Config{BaseURL: mockServer.URL})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return APIError with status 409", func() {
				_, err := hgClient.GetRecoverySessionResult(ctx, "sess-recovery-pending")

				Expect(err).To(HaveOccurred())
				var apiErr *client.APIError
				Expect(errors.As(err, &apiErr)).To(BeTrue())
				Expect(apiErr.StatusCode).To(Equal(http.StatusConflict))
			})
		})
	})
})
