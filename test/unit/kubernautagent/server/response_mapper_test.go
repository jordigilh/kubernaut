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
	"context"
	"log/slog"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/agentclient"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/server"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
)

var _ = Describe("Response Mapper — #433", func() {

	var (
		store   *session.Store
		manager *session.Manager
		handler *server.Handler
		logger  *slog.Logger
	)

	BeforeEach(func() {
		store = session.NewStore(5 * time.Minute)
		logger = slog.Default()
		manager = session.NewManager(store, logger)
		handler = server.NewHandler(manager, nil, logger)
	})

	Describe("UT-KA-433-MAPPER-001: IncidentID is populated from session metadata", func() {
		It("should set IncidentID in the response from session metadata", func() {
			metadata := map[string]string{"incident_id": "e2e-ka-001-oom"}
			id, err := manager.StartInvestigation(context.Background(), func(_ context.Context) (interface{}, error) {
				return &katypes.InvestigationResult{
					RCASummary: "OOMKilled due to memory limit",
					Confidence: 0.85,
				}, nil
			}, metadata)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, _ := manager.GetSession(id)
				if sess == nil {
					return ""
				}
				return sess.Status
			}, 2*time.Second, 10*time.Millisecond).Should(Equal(session.StatusCompleted))

			params := agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetParams{
				SessionID: id,
			}
			resp, err := handler.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGet(nil, params)
			Expect(err).NotTo(HaveOccurred())

			incidentResp, ok := resp.(*agentclient.IncidentResponse)
			Expect(ok).To(BeTrue(), "response should be *IncidentResponse")
			Expect(incidentResp.IncidentID).To(Equal("e2e-ka-001-oom"))
		})
	})

	Describe("UT-KA-433-MAPPER-002: Timestamp is set to a non-empty RFC3339 value", func() {
		It("should set a valid Timestamp on the response", func() {
			id, err := manager.StartInvestigation(context.Background(), func(_ context.Context) (interface{}, error) {
				return &katypes.InvestigationResult{
					RCASummary: "CrashLoopBackOff",
					Confidence: 0.70,
				}, nil
			}, nil)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, _ := manager.GetSession(id)
				if sess == nil {
					return ""
				}
				return sess.Status
			}, 2*time.Second, 10*time.Millisecond).Should(Equal(session.StatusCompleted))

			params := agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetParams{
				SessionID: id,
			}
			resp, err := handler.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGet(nil, params)
			Expect(err).NotTo(HaveOccurred())

			incidentResp, ok := resp.(*agentclient.IncidentResponse)
			Expect(ok).To(BeTrue(), "response should be *IncidentResponse")
			Expect(incidentResp.Timestamp).NotTo(BeEmpty(), "timestamp must be set")
			_, parseErr := time.Parse(time.RFC3339, incidentResp.Timestamp)
			Expect(parseErr).NotTo(HaveOccurred(), "timestamp should be valid RFC3339")
		})
	})

	Describe("UT-KA-433-MAPPER-003: RootCauseAnalysis is populated from RCASummary", func() {
		It("should set RootCauseAnalysis as a structured map", func() {
			id, err := manager.StartInvestigation(context.Background(), func(_ context.Context) (interface{}, error) {
				return &katypes.InvestigationResult{
					RCASummary: "Pod killed due to exceeding memory limits",
					Confidence: 0.90,
				}, nil
			}, nil)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, _ := manager.GetSession(id)
				if sess == nil {
					return ""
				}
				return sess.Status
			}, 2*time.Second, 10*time.Millisecond).Should(Equal(session.StatusCompleted))

			params := agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetParams{
				SessionID: id,
			}
			resp, err := handler.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGet(nil, params)
			Expect(err).NotTo(HaveOccurred())

			incidentResp, ok := resp.(*agentclient.IncidentResponse)
			Expect(ok).To(BeTrue(), "response should be *IncidentResponse")
			Expect(incidentResp.RootCauseAnalysis).NotTo(BeEmpty(), "root_cause_analysis must not be empty")
		})
	})
})
