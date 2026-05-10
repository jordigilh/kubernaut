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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/server"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	"github.com/jordigilh/kubernaut/pkg/agentclient"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

var _ = Describe("Fix 7: UT-KA-SCHEMA-001 — AlignmentVerdict mapping in handler", func() {

	var (
		store   *session.Store
		manager *session.Manager
		handler *server.Handler
	)

	BeforeEach(func() {
		store = session.NewStore(5 * time.Minute)
		manager = session.NewManager(store, logr.Discard())
		handler = server.NewHandler(manager, nil, logr.Discard())
	})

	It("UT-KA-SCHEMA-001: maps AlignmentVerdict from InvestigationResult to IncidentResponse", func() {
		metadata := map[string]string{"incident_id": "schema-test-001"}
		id, err := manager.StartInvestigation(context.Background(), func(_ context.Context) (*katypes.InvestigationResult, error) {
			return &katypes.InvestigationResult{
				RCASummary: "Test RCA",
				Confidence: 0.85,
				AlignmentVerdict: &katypes.AlignmentVerdictResult{
					Result:                  "suspicious",
					CircuitBreakerActivated: true,
					Summary:                 "Suspicious content detected in tool call",
					Flagged:                 2,
					Total:                   5,
					Findings: []katypes.AlignmentFinding{
						{
							StepIndex:   1,
							StepKind:    "tool_result",
							Tool:        "kubectl_get",
							Explanation: "Unexpected data exfiltration pattern",
						},
						{
							StepIndex:   3,
							StepKind:    "llm_reasoning",
							Explanation: "Model attempted to access restricted resources",
						},
					},
				},
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

		// AlignmentVerdict must be mapped to ogen type in the response
		Expect(incidentResp.AlignmentVerdict.Set).To(BeTrue(), "AlignmentVerdict must be set")
		Expect(incidentResp.AlignmentVerdict.Null).To(BeFalse(), "AlignmentVerdict must not be null")

		av := incidentResp.AlignmentVerdict.Value
		Expect(string(av.Result)).To(Equal("suspicious"))
		Expect(av.CircuitBreakerActivated.Value).To(BeTrue())
		Expect(av.Flagged).To(Equal(2))
		Expect(av.Total).To(Equal(5))
		Expect(av.Findings).To(HaveLen(2))
		Expect(string(av.Findings[0].StepKind)).To(Equal("tool_result"))
		Expect(av.Findings[0].Tool.Value).To(Equal("kubectl_get"))
		Expect(av.Findings[0].Explanation).To(Equal("Unexpected data exfiltration pattern"))
	})
})
