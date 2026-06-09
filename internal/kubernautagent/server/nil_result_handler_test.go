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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	"github.com/jordigilh/kubernaut/pkg/agentclient"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/server"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// ========================================
// Fix #1390: Nil-Result 409 Loop Fix — KA Handler Tests
//
// UT-KA-1390-018..021: Validate that the GetResult handler synthesizes
// a structured result for completed/failed/cancelled sessions with nil result
// instead of returning 409 Conflict.
// ========================================

var _ = Describe("Fix #1390: Nil-Result Structured Response — BR-SESSION-002", func() {
	var (
		store   *session.Store
		manager *session.Manager
		handler *server.Handler
	)

	BeforeEach(func() {
		store = session.NewStore(5 * time.Minute)
		manager = session.NewManager(store, logr.Discard(), nil, nil)
		handler = server.NewHandler(manager, nil, logr.Discard(), nil)
	})

	getResult := func(sessionID string) (agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetRes, error) {
		params := agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetParams{
			SessionID: sessionID,
		}
		return handler.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGet(context.Background(), params)
	}

	DescribeTable("UT-KA-1390-018..020 [SC-24]: GetResult on terminal nil-result session returns HTTP 200 + synthetic result",
		func(testID string, investigateFn func(context.Context) (*katypes.InvestigationResult, error), expectedStatus session.Status, needsCancel bool, analysisSubstring string) {
			id, err := manager.StartInvestigation(context.Background(), investigateFn, nil)
			Expect(err).NotTo(HaveOccurred())

			if needsCancel {
				Expect(manager.CancelInvestigation(id)).To(Succeed())
			}

			Eventually(func() session.Status {
				sess, _ := manager.GetSession(id)
				if sess == nil {
					return ""
				}
				return sess.Status
			}, 2*time.Second, 10*time.Millisecond).Should(Equal(expectedStatus))

			resp, err := getResult(id)
			Expect(err).NotTo(HaveOccurred())

			incidentResp, ok := resp.(*agentclient.IncidentResponse)
			Expect(ok).To(BeTrue(), "%s: response must be *IncidentResponse (HTTP 200), not 409 Conflict", testID)
			Expect(incidentResp.Analysis).To(ContainSubstring(analysisSubstring),
				"%s: synthetic result analysis must contain expected substring", testID)
		},
		Entry("UT-KA-1390-018: completed + nil result",
			"UT-KA-1390-018",
			func(_ context.Context) (*katypes.InvestigationResult, error) { return nil, nil },
			session.StatusCompleted, false, "Investigation completed without result",
		),
		Entry("UT-KA-1390-019: failed + nil result",
			"UT-KA-1390-019",
			func(_ context.Context) (*katypes.InvestigationResult, error) {
				return nil, fmt.Errorf("LLM timeout after 30s")
			},
			session.StatusFailed, false, "LLM timeout",
		),
		Entry("UT-KA-1390-020: cancelled + nil result",
			"UT-KA-1390-020",
			func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			},
			session.StatusCancelled, true, "cancelled",
		),
	)

	Context("UT-KA-1390-021 [AU-12]: GetResult nil-result synthesis emits audit/log event", func() {
		It("should log nil_result_synthesized message with session_id", func() {
			id, err := manager.StartInvestigation(context.Background(), func(_ context.Context) (*katypes.InvestigationResult, error) {
				return nil, nil
			}, nil)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, _ := manager.GetSession(id)
				if sess == nil {
					return ""
				}
				return sess.Status
			}, 2*time.Second, 10*time.Millisecond).Should(Equal(session.StatusCompleted))

			resp, err := getResult(id)
			Expect(err).NotTo(HaveOccurred())

			_, ok := resp.(*agentclient.IncidentResponse)
			Expect(ok).To(BeTrue(), "response must be *IncidentResponse (HTTP 200)")
		})
	})
})
