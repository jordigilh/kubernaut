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

package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-logr/logr"

	"github.com/google/uuid"

	kaserver "github.com/jordigilh/kubernaut/internal/kubernautagent/server"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/agentclient"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func validIncidentJSON() string {
	// Mirrors required OpenAPI IncidentRequest fields (excluding optional blocks).
	return `{
  "incident_id": "inc-it-srv",
  "remediation_id": "rem-it-srv",
  "signal_name": "OOMKilled",
  "severity": "high",
  "signal_source": "prometheus",
  "resource_namespace": "default",
  "resource_kind": "Pod",
  "resource_name": "web-1",
  "error_message": "out of memory",
  "environment": "staging",
  "priority": "P1",
  "risk_tolerance": "low",
  "business_category": "platform",
  "cluster_name": "kind-local"
}`
}

type stubInvestigator struct {
	fn func(context.Context, katypes.SignalContext) (*katypes.InvestigationResult, error)
}

func (s *stubInvestigator) Investigate(ctx context.Context, sc katypes.SignalContext) (*katypes.InvestigationResult, error) {
	if s.fn != nil {
		return s.fn(ctx, sc)
	}
	return &katypes.InvestigationResult{RCASummary: "ok", Confidence: 0.85}, nil
}

func newTestAPIServer(inv kaserver.InvestigationRunner) (*httptest.Server, *session.Manager) {
	store := session.NewStore(24 * time.Hour)
	mgr := session.NewManager(store, logr.Discard(), 8)

	handler := kaserver.NewHandler(mgr, inv, logr.Discard())
	ogenSrv, err := agentclient.NewServer(handler)
	Expect(err).NotTo(HaveOccurred())

	r := chi.NewRouter()
	r.Route("/api/v1", func(api chi.Router) {
		api.Handle("/*", ogenSrv)
	})
	return httptest.NewServer(r), mgr
}

var _ = Describe("Kubernaut Agent incident API HTTP contract — BR-AI-952 / GAP-T2", func() {

	Describe("IT-SRV-001: POST /api/v1/incident/analyze accepts valid JSON and returns 202 + session_id", func() {
		It("returns 202 and a UUID session_id", func() {
			ts, mgr := newTestAPIServer(&stubInvestigator{})
			defer ts.Close()
			defer mgr.DrainAndWait(2 * time.Second)

			req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/incident/analyze", strings.NewReader(validIncidentJSON()))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusAccepted))

			var body struct {
				SessionID string `json:"session_id"`
			}
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
			Expect(body.SessionID).NotTo(BeEmpty())
			_, perr := uuid.Parse(body.SessionID)
			Expect(perr).NotTo(HaveOccurred())
		})
	})

	Describe("IT-SRV-002: POST /api/v1/incident/analyze with malformed JSON returns 400", func() {
		It("responds with decoder error envelope (ogen DefaultErrorHandler)", func() {
			ts, mgr := newTestAPIServer(&stubInvestigator{})
			defer ts.Close()
			defer mgr.DrainAndWait(2 * time.Second)

			req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/incident/analyze",
				bytes.NewBufferString("{not-valid-json"))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			Expect(strings.Contains(resp.Header.Get("Content-Type"), "application/json")).To(BeTrue())

			payload, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(payload).To(ContainSubstring("error_message"))
		})
	})

	Describe("IT-SRV-003: GET /api/v1/incident/session/{id} returns 200 + status for existing session", func() {
		It("reflects session state after POST", func() {
			ts, mgr := newTestAPIServer(&stubInvestigator{})
			defer ts.Close()
			defer mgr.DrainAndWait(3 * time.Second)

			postReq, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/incident/analyze",
				bytes.NewBufferString(validIncidentJSON()))
			Expect(err).NotTo(HaveOccurred())
			postReq.Header.Set("Content-Type", "application/json")

			postResp, err := http.DefaultClient.Do(postReq)
			Expect(err).NotTo(HaveOccurred())
			defer postResp.Body.Close()
			Expect(postResp.StatusCode).To(Equal(http.StatusAccepted))

			var sub struct {
				SessionID string `json:"session_id"`
			}
			Expect(json.NewDecoder(postResp.Body).Decode(&sub)).To(Succeed())

			statusURL := ts.URL + "/api/v1/incident/session/" + sub.SessionID
			getResp, err := http.DefaultClient.Get(statusURL)
			Expect(err).NotTo(HaveOccurred())
			defer getResp.Body.Close()

			Expect(getResp.StatusCode).To(Equal(http.StatusOK))

			var st struct {
				SessionID string `json:"session_id"`
				Status    string `json:"status"`
			}
			Expect(json.NewDecoder(getResp.Body).Decode(&st)).To(Succeed())
			Expect(st.SessionID).To(Equal(sub.SessionID))
			Expect(st.Status).NotTo(BeEmpty())
		})
	})

	Describe("IT-SRV-004: GET /api/v1/incident/session/{id} returns 404 for unknown session", func() {
		It("returns problem details for missing session", func() {
			ts, mgr := newTestAPIServer(&stubInvestigator{})
			defer ts.Close()
			defer mgr.DrainAndWait(200 * time.Millisecond)

			id := uuid.NewString()
			getResp, err := http.DefaultClient.Get(ts.URL + "/api/v1/incident/session/" + id)
			Expect(err).NotTo(HaveOccurred())
			defer getResp.Body.Close()

			Expect(getResp.StatusCode).To(Equal(http.StatusNotFound))
			Expect(getResp.Header.Get("Content-Type")).To(ContainSubstring("application/problem+json"))

			var prob struct {
				Status int    `json:"status"`
				Title  string `json:"title"`
			}
			Expect(json.NewDecoder(getResp.Body).Decode(&prob)).To(Succeed())
			Expect(prob.Status).To(Equal(404))
		})
	})

	Describe("IT-SRV-005: GET /api/v1/incident/session/{id}/result returns 200 for completed session", func() {
		It("returns incident response JSON", func() {
			ts, mgr := newTestAPIServer(&stubInvestigator{})
			defer ts.Close()
			defer mgr.DrainAndWait(3 * time.Second)

			postReq, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/incident/analyze",
				bytes.NewBufferString(validIncidentJSON()))
			Expect(err).NotTo(HaveOccurred())
			postReq.Header.Set("Content-Type", "application/json")

			postResp, err := http.DefaultClient.Do(postReq)
			Expect(err).NotTo(HaveOccurred())
			defer postResp.Body.Close()

			var sub struct {
				SessionID string `json:"session_id"`
			}
			Expect(json.NewDecoder(postResp.Body).Decode(&sub)).To(Succeed())

			var res *http.Response
			Eventually(func() int {
				var e error
				res, e = http.Get(ts.URL + "/api/v1/incident/session/" + sub.SessionID + "/result")
				if e != nil {
					return 0
				}
				return res.StatusCode
			}, 3*time.Second, 20*time.Millisecond).Should(Equal(http.StatusOK))
			defer res.Body.Close()

			var out struct {
				IncidentID string `json:"incident_id"`
				Analysis   string `json:"analysis"`
			}
			Expect(json.NewDecoder(res.Body).Decode(&out)).To(Succeed())
			Expect(out.IncidentID).To(Equal("inc-it-srv"))
			Expect(out.Analysis).NotTo(BeEmpty())
		})
	})

	Describe("IT-SRV-006: GET /api/v1/incident/session/{id}/result while session is in progress", func() {
		It("returns 409 session-not-completed (handler contract — not 202)", func() {
			resume := make(chan struct{})
			inv := &stubInvestigator{
				fn: func(ctx context.Context, _ katypes.SignalContext) (*katypes.InvestigationResult, error) {
					select {
					case <-resume:
						return &katypes.InvestigationResult{RCASummary: "late", Confidence: 1}, nil
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				},
			}

			ts, mgr := newTestAPIServer(inv)
			defer ts.Close()
			defer close(resume)
			defer mgr.DrainAndWait(3 * time.Second)

			postReq, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/incident/analyze",
				bytes.NewBufferString(validIncidentJSON()))
			Expect(err).NotTo(HaveOccurred())
			postReq.Header.Set("Content-Type", "application/json")

			postResp, err := http.DefaultClient.Do(postReq)
			Expect(err).NotTo(HaveOccurred())
			defer postResp.Body.Close()

			var sub struct {
				SessionID string `json:"session_id"`
			}
			Expect(json.NewDecoder(postResp.Body).Decode(&sub)).To(Succeed())

			res, err := http.Get(ts.URL + "/api/v1/incident/session/" + sub.SessionID + "/result")
			Expect(err).NotTo(HaveOccurred())
			defer res.Body.Close()

			Expect(res.StatusCode).To(Equal(http.StatusConflict))
			Expect(res.Header.Get("Content-Type")).To(ContainSubstring("application/problem+json"))
		})
	})

	Describe("IT-SRV-007: Content-Type enforcement for POST /api/v1/incident/analyze", func() {
		It("rejects non-JSON Content-Type with 415", func() {
			ts, mgr := newTestAPIServer(&stubInvestigator{})
			defer ts.Close()
			defer mgr.DrainAndWait(200 * time.Millisecond)

			req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/incident/analyze",
				bytes.NewBufferString(validIncidentJSON()))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "text/plain")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusUnsupportedMediaType))
			Expect(strings.Contains(resp.Header.Get("Content-Type"), "application/json")).To(BeTrue())
		})
	})
})
